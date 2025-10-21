package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/casjaysdevdocker/caslink/internal/config"
)

// SQLServerDB represents SQL Server-specific database operations
type SQLServerDB struct {
	*DB
}

// NewSQLServer creates a new SQL Server database connection
func NewSQLServer(cfg *config.DatabaseConfig) (*SQLServerDB, error) {
	db, err := New(cfg, nil)
	if err != nil {
		return nil, err
	}

	sqlServerDB := &SQLServerDB{DB: db}

	// Run SQL Server-specific optimizations
	if err := sqlServerDB.optimize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to optimize SQL Server: %w", err)
	}

	return sqlServerDB, nil
}

// optimize applies SQL Server-specific optimizations
func (s *SQLServerDB) optimize(ctx context.Context) error {
	// Set connection options for better performance
	optimizations := []string{
		"SET ANSI_NULLS ON",
		"SET QUOTED_IDENTIFIER ON",
		"SET ARITHABORT ON",
		"SET CONCAT_NULL_YIELDS_NULL ON",
		"SET NUMERIC_ROUNDABORT OFF",
	}

	for _, query := range optimizations {
		if _, err := s.Exec(ctx, query); err != nil {
			// Log the error but don't fail
			s.logger.WithField("query", query).Warn("Failed to apply SQL Server optimization")
		}
	}

	return nil
}

// UpdateStatistics updates statistics for a table or index
func (s *SQLServerDB) UpdateStatistics(ctx context.Context, table string, index string) error {
	var query string
	if index != "" {
		query = fmt.Sprintf("UPDATE STATISTICS %s %s", table, index)
	} else {
		query = fmt.Sprintf("UPDATE STATISTICS %s", table)
	}
	_, err := s.Exec(ctx, query)
	return err
}

// RebuildIndex rebuilds an index
func (s *SQLServerDB) RebuildIndex(ctx context.Context, table string, index string) error {
	query := fmt.Sprintf("ALTER INDEX %s ON %s REBUILD", index, table)
	_, err := s.Exec(ctx, query)
	return err
}

// ReorganizeIndex reorganizes an index
func (s *SQLServerDB) ReorganizeIndex(ctx context.Context, table string, index string) error {
	query := fmt.Sprintf("ALTER INDEX %s ON %s REORGANIZE", index, table)
	_, err := s.Exec(ctx, query)
	return err
}

// GetTableSizes returns the size of each table in the database
func (s *SQLServerDB) GetTableSizes(ctx context.Context) (map[string]int64, error) {
	query := `
		SELECT
			t.name AS table_name,
			SUM(a.total_pages) * 8 * 1024 AS size_bytes
		FROM sys.tables t
		INNER JOIN sys.indexes i ON t.object_id = i.object_id
		INNER JOIN sys.partitions p ON i.object_id = p.object_id AND i.index_id = p.index_id
		INNER JOIN sys.allocation_units a ON p.partition_id = a.container_id
		LEFT OUTER JOIN sys.schemas s ON t.schema_id = s.schema_id
		WHERE t.is_ms_shipped = 0
		GROUP BY t.name
		ORDER BY size_bytes DESC
	`

	rows, err := s.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sizes := make(map[string]int64)
	for rows.Next() {
		var tableName string
		var sizeBytes int64
		if err := rows.Scan(&tableName, &sizeBytes); err != nil {
			return nil, err
		}
		sizes[tableName] = sizeBytes
	}

	return sizes, rows.Err()
}

// GetIndexSizes returns the size of each index in the database
func (s *SQLServerDB) GetIndexSizes(ctx context.Context) (map[string]int64, error) {
	query := `
		SELECT
			i.name AS index_name,
			t.name AS table_name,
			SUM(a.total_pages) * 8 * 1024 AS size_bytes
		FROM sys.tables t
		INNER JOIN sys.indexes i ON t.object_id = i.object_id
		INNER JOIN sys.partitions p ON i.object_id = p.object_id AND i.index_id = p.index_id
		INNER JOIN sys.allocation_units a ON p.partition_id = a.container_id
		WHERE t.is_ms_shipped = 0 AND i.name IS NOT NULL
		GROUP BY i.name, t.name
		ORDER BY size_bytes DESC
	`

	rows, err := s.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sizes := make(map[string]int64)
	for rows.Next() {
		var indexName, tableName string
		var sizeBytes int64
		if err := rows.Scan(&indexName, &tableName, &sizeBytes); err != nil {
			return nil, err
		}
		fullName := fmt.Sprintf("%s.%s", tableName, indexName)
		sizes[fullName] = sizeBytes
	}

	return sizes, rows.Err()
}

// GetDatabaseStats returns SQL Server database statistics
func (s *SQLServerDB) GetDatabaseStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Database size
	row := s.QueryRow(ctx, `
		SELECT
			SUM(size * 8.0 / 1024 / 1024) AS size_gb
		FROM sys.master_files
		WHERE database_id = DB_ID()
	`)
	var sizeGB float64
	if err := row.Scan(&sizeGB); err == nil {
		stats["database_size"] = int64(sizeGB * 1024 * 1024 * 1024)
	}

	// Connection count
	row = s.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM sys.dm_exec_sessions
		WHERE is_user_process = 1
	`)
	var connCount int64
	if err := row.Scan(&connCount); err == nil {
		stats["connection_count"] = connCount
	}

	// Buffer pool statistics
	row = s.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM sys.dm_os_buffer_descriptors) AS total_pages,
			(SELECT COUNT(*) FROM sys.dm_os_buffer_descriptors WHERE is_modified = 1) AS dirty_pages
	`)
	var totalPages, dirtyPages int64
	if err := row.Scan(&totalPages, &dirtyPages); err == nil {
		stats["buffer_pool"] = map[string]int64{
			"total_pages": totalPages,
			"dirty_pages": dirtyPages,
		}
	}

	// Wait statistics (top 10)
	waitRows, err := s.Query(ctx, `
		SELECT TOP 10
			wait_type,
			waiting_tasks_count,
			wait_time_ms,
			max_wait_time_ms,
			signal_wait_time_ms
		FROM sys.dm_os_wait_stats
		WHERE wait_type NOT IN (
			'CLR_SEMAPHORE', 'LAZYWRITER_SLEEP', 'RESOURCE_QUEUE', 'SLEEP_TASK',
			'SLEEP_SYSTEMTASK', 'SQLTRACE_BUFFER_FLUSH', 'WAITFOR', 'LOGMGR_QUEUE',
			'CHECKPOINT_QUEUE', 'REQUEST_FOR_DEADLOCK_SEARCH', 'XE_TIMER_EVENT',
			'BROKER_TO_FLUSH', 'BROKER_TASK_STOP', 'CLR_MANUAL_EVENT', 'CLR_AUTO_EVENT',
			'DISPATCHER_QUEUE_SEMAPHORE', 'FT_IFTS_SCHEDULER_IDLE_WAIT',
			'XE_DISPATCHER_WAIT', 'XE_DISPATCHER_JOIN', 'SQLTRACE_INCREMENTAL_FLUSH_SLEEP'
		)
		ORDER BY wait_time_ms DESC
	`)

	if err == nil {
		defer waitRows.Close()
		var waitStats []map[string]interface{}

		for waitRows.Next() {
			var waitType string
			var waitingTasks, waitTime, maxWaitTime, signalWaitTime int64

			if err := waitRows.Scan(&waitType, &waitingTasks, &waitTime, &maxWaitTime, &signalWaitTime); err == nil {
				waitStats = append(waitStats, map[string]interface{}{
					"wait_type":          waitType,
					"waiting_tasks":      waitingTasks,
					"wait_time_ms":       waitTime,
					"max_wait_time_ms":   maxWaitTime,
					"signal_wait_time_ms": signalWaitTime,
				})
			}
		}
		stats["wait_statistics"] = waitStats
	}

	return stats, nil
}

// GetSlowQueries returns slow queries from Query Store
func (s *SQLServerDB) GetSlowQueries(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	query := fmt.Sprintf(`
		SELECT TOP %d
			qt.query_sql_text,
			rs.count_executions,
			rs.avg_duration / 1000.0 AS avg_duration_ms,
			rs.avg_cpu_time / 1000.0 AS avg_cpu_time_ms,
			rs.avg_logical_io_reads,
			rs.avg_physical_io_reads
		FROM sys.query_store_query_text qt
		JOIN sys.query_store_query q ON qt.query_text_id = q.query_text_id
		JOIN sys.query_store_plan p ON q.query_id = p.query_id
		JOIN sys.query_store_runtime_stats rs ON p.plan_id = rs.plan_id
		WHERE rs.last_execution_time > DATEADD(day, -7, GETUTCDATE())
		ORDER BY rs.avg_duration DESC
	`, limit)

	rows, err := s.Query(ctx, query)
	if err != nil {
		// Query Store might not be enabled
		return nil, fmt.Errorf("Query Store not available or not enabled: %w", err)
	}
	defer rows.Close()

	var queries []map[string]interface{}
	for rows.Next() {
		var sqlText string
		var executions int64
		var avgDuration, avgCPU float64
		var avgLogicalReads, avgPhysicalReads int64

		if err := rows.Scan(&sqlText, &executions, &avgDuration, &avgCPU, &avgLogicalReads, &avgPhysicalReads); err != nil {
			return nil, err
		}

		queries = append(queries, map[string]interface{}{
			"query":               strings.TrimSpace(sqlText),
			"executions":          executions,
			"avg_duration_ms":     avgDuration,
			"avg_cpu_time_ms":     avgCPU,
			"avg_logical_reads":   avgLogicalReads,
			"avg_physical_reads":  avgPhysicalReads,
		})
	}

	return queries, rows.Err()
}

// GetActiveQueries returns currently executing queries
func (s *SQLServerDB) GetActiveQueries(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT
			s.session_id,
			s.login_name,
			s.host_name,
			s.program_name,
			r.command,
			r.status,
			r.wait_type,
			r.wait_time,
			r.cpu_time,
			r.logical_reads,
			r.reads,
			r.writes,
			t.text AS query_text
		FROM sys.dm_exec_sessions s
		INNER JOIN sys.dm_exec_requests r ON s.session_id = r.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE s.is_user_process = 1
		ORDER BY r.cpu_time DESC
	`

	rows, err := s.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activeQueries []map[string]interface{}
	for rows.Next() {
		var sessionID int64
		var loginName, hostName, programName, command, status string
		var waitType *string
		var waitTime, cpuTime, logicalReads, reads, writes int64
		var queryText string

		if err := rows.Scan(&sessionID, &loginName, &hostName, &programName,
			&command, &status, &waitType, &waitTime, &cpuTime,
			&logicalReads, &reads, &writes, &queryText); err != nil {
			return nil, err
		}

		query := map[string]interface{}{
			"session_id":     sessionID,
			"login_name":     loginName,
			"host_name":      hostName,
			"program_name":   programName,
			"command":        command,
			"status":         status,
			"wait_time":      waitTime,
			"cpu_time":       cpuTime,
			"logical_reads":  logicalReads,
			"reads":          reads,
			"writes":         writes,
			"query_text":     strings.TrimSpace(queryText),
		}

		if waitType != nil {
			query["wait_type"] = *waitType
		}

		activeQueries = append(activeQueries, query)
	}

	return activeQueries, rows.Err()
}

// KillSession terminates a session by session ID
func (s *SQLServerDB) KillSession(ctx context.Context, sessionID int64) error {
	_, err := s.Exec(ctx, fmt.Sprintf("KILL %d", sessionID))
	return err
}

// Backup creates a backup using T-SQL BACKUP command
func (s *SQLServerDB) Backup(ctx context.Context, destPath string) error {
	query := fmt.Sprintf("BACKUP DATABASE [%s] TO DISK = '%s'", s.getCurrentDatabase(ctx), destPath)
	_, err := s.Exec(ctx, query)
	return err
}

// Restore restores from a backup using T-SQL RESTORE command
func (s *SQLServerDB) Restore(ctx context.Context, sourcePath string) error {
	dbName := s.getCurrentDatabase(ctx)
	query := fmt.Sprintf("RESTORE DATABASE [%s] FROM DISK = '%s' WITH REPLACE", dbName, sourcePath)
	_, err := s.Exec(ctx, query)
	return err
}

// CheckDB performs DBCC CHECKDB
func (s *SQLServerDB) CheckDB(ctx context.Context) ([]string, error) {
	dbName := s.getCurrentDatabase(ctx)
	query := fmt.Sprintf("DBCC CHECKDB('%s') WITH NO_INFOMSGS", dbName)

	rows, err := s.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// getCurrentDatabase returns the current database name
func (s *SQLServerDB) getCurrentDatabase(ctx context.Context) string {
	row := s.QueryRow(ctx, "SELECT DB_NAME()")
	var dbName string
	if err := row.Scan(&dbName); err != nil {
		return "master" // fallback
	}
	return dbName
}