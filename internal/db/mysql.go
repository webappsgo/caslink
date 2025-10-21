package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/casjaysdevdocker/caslink/internal/config"
)

// MySQLDB represents MySQL/MariaDB-specific database operations
type MySQLDB struct {
	*DB
}

// NewMySQL creates a new MySQL/MariaDB database connection
func NewMySQL(cfg *config.DatabaseConfig) (*MySQLDB, error) {
	db, err := New(cfg, nil)
	if err != nil {
		return nil, err
	}

	mysqlDB := &MySQLDB{DB: db}

	// Run MySQL-specific optimizations
	if err := mysqlDB.optimize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to optimize MySQL: %w", err)
	}

	return mysqlDB, nil
}

// optimize applies MySQL-specific optimizations
func (m *MySQLDB) optimize(ctx context.Context) error {
	// Set session variables for better performance
	optimizations := []string{
		"SET SESSION sql_mode = 'STRICT_TRANS_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO'",
		"SET SESSION innodb_lock_wait_timeout = 50",
		"SET SESSION wait_timeout = 28800",
		"SET SESSION interactive_timeout = 28800",
	}

	for _, query := range optimizations {
		if _, err := m.Exec(ctx, query); err != nil {
			// Log the error but don't fail
			m.logger.WithField("query", query).Warn("Failed to apply MySQL optimization")
		}
	}

	return nil
}

// Optimize performs MySQL OPTIMIZE TABLE operation
func (m *MySQLDB) Optimize(ctx context.Context, table string) error {
	query := fmt.Sprintf("OPTIMIZE TABLE %s", table)
	_, err := m.Exec(ctx, query)
	return err
}

// Analyze performs MySQL ANALYZE TABLE operation
func (m *MySQLDB) Analyze(ctx context.Context, table string) error {
	query := fmt.Sprintf("ANALYZE TABLE %s", table)
	_, err := m.Exec(ctx, query)
	return err
}

// Check performs MySQL CHECK TABLE operation
func (m *MySQLDB) Check(ctx context.Context, table string) error {
	query := fmt.Sprintf("CHECK TABLE %s", table)
	_, err := m.Exec(ctx, query)
	return err
}

// Repair performs MySQL REPAIR TABLE operation
func (m *MySQLDB) Repair(ctx context.Context, table string) error {
	query := fmt.Sprintf("REPAIR TABLE %s", table)
	_, err := m.Exec(ctx, query)
	return err
}

// GetTableSizes returns the size of each table in the database
func (m *MySQLDB) GetTableSizes(ctx context.Context) (map[string]int64, error) {
	query := `
		SELECT
			table_name,
			ROUND(((data_length + index_length) / 1024 / 1024), 2) AS 'size_mb'
		FROM information_schema.TABLES
		WHERE table_schema = DATABASE()
		ORDER BY (data_length + index_length) DESC
	`

	rows, err := m.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sizes := make(map[string]int64)
	for rows.Next() {
		var tableName string
		var sizeMB float64
		if err := rows.Scan(&tableName, &sizeMB); err != nil {
			return nil, err
		}
		// Convert MB to bytes
		sizes[tableName] = int64(sizeMB * 1024 * 1024)
	}

	return sizes, rows.Err()
}

// GetDatabaseStats returns MySQL database statistics
func (m *MySQLDB) GetDatabaseStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Database size
	row := m.QueryRow(ctx, `
		SELECT ROUND(SUM(data_length + index_length) / 1024 / 1024, 2) AS 'size_mb'
		FROM information_schema.TABLES
		WHERE table_schema = DATABASE()
	`)
	var sizeMB float64
	if err := row.Scan(&sizeMB); err == nil {
		stats["database_size"] = int64(sizeMB * 1024 * 1024)
	}

	// Connection count
	row = m.QueryRow(ctx, "SHOW STATUS LIKE 'Threads_connected'")
	var variable string
	var connCount int64
	if err := row.Scan(&variable, &connCount); err == nil {
		stats["connection_count"] = connCount
	}

	// Query cache statistics
	cacheStats := make(map[string]interface{})
	cacheQueries := []string{
		"Qcache_hits",
		"Qcache_inserts",
		"Qcache_queries_in_cache",
		"Qcache_free_memory",
	}

	for _, cacheVar := range cacheQueries {
		row = m.QueryRow(ctx, fmt.Sprintf("SHOW STATUS LIKE '%s'", cacheVar))
		var variable string
		var value int64
		if err := row.Scan(&variable, &value); err == nil {
			cacheStats[strings.ToLower(cacheVar)] = value
		}
	}
	stats["query_cache"] = cacheStats

	// InnoDB statistics
	innodbStats := make(map[string]interface{})
	innodbQueries := []string{
		"Innodb_buffer_pool_read_requests",
		"Innodb_buffer_pool_reads",
		"Innodb_buffer_pool_pages_total",
		"Innodb_buffer_pool_pages_free",
		"Innodb_buffer_pool_pages_dirty",
	}

	for _, innodbVar := range innodbQueries {
		row = m.QueryRow(ctx, fmt.Sprintf("SHOW STATUS LIKE '%s'", innodbVar))
		var variable string
		var value int64
		if err := row.Scan(&variable, &value); err == nil {
			innodbStats[strings.ToLower(innodbVar)] = value
		}
	}

	// Calculate buffer pool hit ratio
	if readRequests, ok := innodbStats["innodb_buffer_pool_read_requests"].(int64); ok {
		if reads, ok := innodbStats["innodb_buffer_pool_reads"].(int64); ok && readRequests > 0 {
			hitRatio := float64(readRequests-reads) / float64(readRequests) * 100
			innodbStats["buffer_pool_hit_ratio"] = hitRatio
		}
	}

	stats["innodb"] = innodbStats

	return stats, nil
}

// GetSlowQueries returns slow queries from slow query log
func (m *MySQLDB) GetSlowQueries(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	// Check if slow query log is enabled
	row := m.QueryRow(ctx, "SHOW VARIABLES LIKE 'slow_query_log'")
	var variable, value string
	if err := row.Scan(&variable, &value); err != nil || value != "ON" {
		return nil, fmt.Errorf("slow query log is not enabled")
	}

	// This is a simplified version - in practice, you'd parse the slow query log file
	// or use pt-query-digest from Percona Toolkit
	query := `
		SELECT
			sql_text,
			count_star as count,
			avg_timer_wait / 1000000000000 as avg_time_seconds,
			sum_timer_wait / 1000000000000 as total_time_seconds
		FROM performance_schema.events_statements_summary_by_digest
		WHERE schema_name = DATABASE()
		ORDER BY avg_timer_wait DESC
		LIMIT ?
	`

	rows, err := m.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queries []map[string]interface{}
	for rows.Next() {
		var sqlText string
		var count int64
		var avgTime, totalTime float64

		if err := rows.Scan(&sqlText, &count, &avgTime, &totalTime); err != nil {
			return nil, err
		}

		queries = append(queries, map[string]interface{}{
			"query":      strings.TrimSpace(sqlText),
			"count":      count,
			"avg_time":   avgTime,
			"total_time": totalTime,
		})
	}

	return queries, rows.Err()
}

// GetProcessList returns current MySQL process list
func (m *MySQLDB) GetProcessList(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT
			id,
			user,
			host,
			db,
			command,
			time,
			state,
			info
		FROM information_schema.PROCESSLIST
		WHERE id != CONNECTION_ID()
		ORDER BY time DESC
	`

	rows, err := m.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var processes []map[string]interface{}
	for rows.Next() {
		var id int64
		var user, host, command, state string
		var db, info *string
		var time int64

		if err := rows.Scan(&id, &user, &host, &db, &command, &time, &state, &info); err != nil {
			return nil, err
		}

		process := map[string]interface{}{
			"id":      id,
			"user":    user,
			"host":    host,
			"command": command,
			"time":    time,
			"state":   state,
		}

		if db != nil {
			process["database"] = *db
		}
		if info != nil {
			process["query"] = strings.TrimSpace(*info)
		}

		processes = append(processes, process)
	}

	return processes, rows.Err()
}

// KillQuery terminates a running query by process ID
func (m *MySQLDB) KillQuery(ctx context.Context, processID int64) error {
	_, err := m.Exec(ctx, "KILL QUERY ?", processID)
	return err
}

// KillConnection terminates a connection by process ID
func (m *MySQLDB) KillConnection(ctx context.Context, processID int64) error {
	_, err := m.Exec(ctx, "KILL CONNECTION ?", processID)
	return err
}

// GetReplicationStatus returns MySQL replication status
func (m *MySQLDB) GetReplicationStatus(ctx context.Context) (map[string]interface{}, error) {
	// Check if this is a slave
	row := m.QueryRow(ctx, "SHOW SLAVE STATUS")
	var status map[string]interface{}

	// This is simplified - SHOW SLAVE STATUS returns many columns
	// In practice, you'd need to handle all the columns properly
	if err := row.Err(); err != nil {
		// Not a slave or no replication configured
		return map[string]interface{}{"type": "master"}, nil
	}

	status = map[string]interface{}{"type": "slave"}
	// Add slave status details here

	return status, nil
}

// Backup creates a logical backup using mysqldump
func (m *MySQLDB) Backup(ctx context.Context, destPath string) error {
	// This would typically call mysqldump externally
	return fmt.Errorf("backup requires external mysqldump tool")
}

// Restore restores from a mysqldump backup
func (m *MySQLDB) Restore(ctx context.Context, sourcePath string) error {
	// This would typically call mysql client externally
	return fmt.Errorf("restore requires external mysql client tool")
}