package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/casjaysdevdocker/caslink/internal/config"
)

// PostgreSQLDB represents PostgreSQL-specific database operations
type PostgreSQLDB struct {
	*DB
}

// NewPostgreSQL creates a new PostgreSQL database connection
func NewPostgreSQL(cfg *config.DatabaseConfig) (*PostgreSQLDB, error) {
	db, err := New(cfg, nil)
	if err != nil {
		return nil, err
	}

	pgDB := &PostgreSQLDB{DB: db}

	// Run PostgreSQL-specific optimizations
	if err := pgDB.optimize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to optimize PostgreSQL: %w", err)
	}

	return pgDB, nil
}

// optimize applies PostgreSQL-specific optimizations
func (p *PostgreSQLDB) optimize(ctx context.Context) error {
	// Enable useful extensions
	extensions := []string{
		"CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"",
		"CREATE EXTENSION IF NOT EXISTS \"pgcrypto\"",
		"CREATE EXTENSION IF NOT EXISTS \"pg_stat_statements\"",
	}

	for _, ext := range extensions {
		if _, err := p.Exec(ctx, ext); err != nil {
			// Log the error but don't fail - extensions might not be available
			p.logger.WithField("extension", ext).Warn("Failed to create extension")
		}
	}

	return nil
}

// Vacuum performs PostgreSQL VACUUM operation
func (p *PostgreSQLDB) Vacuum(ctx context.Context, table string) error {
	query := "VACUUM"
	if table != "" {
		query += " " + table
	}
	_, err := p.Exec(ctx, query)
	return err
}

// VacuumAnalyze performs PostgreSQL VACUUM ANALYZE operation
func (p *PostgreSQLDB) VacuumAnalyze(ctx context.Context, table string) error {
	query := "VACUUM ANALYZE"
	if table != "" {
		query += " " + table
	}
	_, err := p.Exec(ctx, query)
	return err
}

// Analyze performs PostgreSQL ANALYZE operation
func (p *PostgreSQLDB) Analyze(ctx context.Context, table string) error {
	query := "ANALYZE"
	if table != "" {
		query += " " + table
	}
	_, err := p.Exec(ctx, query)
	return err
}

// Reindex performs PostgreSQL REINDEX operation
func (p *PostgreSQLDB) Reindex(ctx context.Context, target string) error {
	query := fmt.Sprintf("REINDEX %s", target)
	_, err := p.Exec(ctx, query)
	return err
}

// GetTableSizes returns the size of each table in the database
func (p *PostgreSQLDB) GetTableSizes(ctx context.Context) (map[string]int64, error) {
	query := `
		SELECT
			schemaname,
			tablename,
			pg_total_relation_size(schemaname||'.'||tablename) as size
		FROM pg_tables
		WHERE schemaname NOT IN ('information_schema', 'pg_catalog')
		ORDER BY size DESC
	`

	rows, err := p.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sizes := make(map[string]int64)
	for rows.Next() {
		var schema, table string
		var size int64
		if err := rows.Scan(&schema, &table, &size); err != nil {
			return nil, err
		}

		tableName := table
		if schema != "public" {
			tableName = schema + "." + table
		}
		sizes[tableName] = size
	}

	return sizes, rows.Err()
}

// GetIndexSizes returns the size of each index in the database
func (p *PostgreSQLDB) GetIndexSizes(ctx context.Context) (map[string]int64, error) {
	query := `
		SELECT
			schemaname,
			indexname,
			pg_relation_size(schemaname||'.'||indexname) as size
		FROM pg_indexes
		WHERE schemaname NOT IN ('information_schema', 'pg_catalog')
		ORDER BY size DESC
	`

	rows, err := p.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sizes := make(map[string]int64)
	for rows.Next() {
		var schema, index string
		var size int64
		if err := rows.Scan(&schema, &index, &size); err != nil {
			return nil, err
		}

		indexName := index
		if schema != "public" {
			indexName = schema + "." + index
		}
		sizes[indexName] = size
	}

	return sizes, rows.Err()
}

// GetDatabaseStats returns PostgreSQL database statistics
func (p *PostgreSQLDB) GetDatabaseStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Database size
	row := p.QueryRow(ctx, "SELECT pg_database_size(current_database())")
	var dbSize int64
	if err := row.Scan(&dbSize); err == nil {
		stats["database_size"] = dbSize
	}

	// Connection count
	row = p.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity")
	var connCount int64
	if err := row.Scan(&connCount); err == nil {
		stats["connection_count"] = connCount
	}

	// Transaction statistics
	row = p.QueryRow(ctx, `
		SELECT
			xact_commit,
			xact_rollback,
			blks_read,
			blks_hit,
			tup_returned,
			tup_fetched,
			tup_inserted,
			tup_updated,
			tup_deleted
		FROM pg_stat_database
		WHERE datname = current_database()
	`)

	var xactCommit, xactRollback, blksRead, blksHit int64
	var tupReturned, tupFetched, tupInserted, tupUpdated, tupDeleted int64

	if err := row.Scan(&xactCommit, &xactRollback, &blksRead, &blksHit,
		&tupReturned, &tupFetched, &tupInserted, &tupUpdated, &tupDeleted); err == nil {

		stats["transactions"] = map[string]int64{
			"committed": xactCommit,
			"rolledback": xactRollback,
		}

		stats["blocks"] = map[string]int64{
			"read": blksRead,
			"hit": blksHit,
		}

		if blksRead+blksHit > 0 {
			stats["cache_hit_ratio"] = float64(blksHit) / float64(blksRead+blksHit) * 100
		}

		stats["tuples"] = map[string]int64{
			"returned": tupReturned,
			"fetched": tupFetched,
			"inserted": tupInserted,
			"updated": tupUpdated,
			"deleted": tupDeleted,
		}
	}

	return stats, nil
}

// GetSlowQueries returns slow queries from pg_stat_statements
func (p *PostgreSQLDB) GetSlowQueries(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT
			query,
			calls,
			total_time,
			mean_time,
			rows
		FROM pg_stat_statements
		ORDER BY mean_time DESC
		LIMIT $1
	`

	rows, err := p.Query(ctx, query, limit)
	if err != nil {
		// pg_stat_statements might not be available
		return nil, fmt.Errorf("pg_stat_statements extension not available: %w", err)
	}
	defer rows.Close()

	var queries []map[string]interface{}
	for rows.Next() {
		var query string
		var calls int64
		var totalTime, meanTime float64
		var rowsAffected int64

		if err := rows.Scan(&query, &calls, &totalTime, &meanTime, &rowsAffected); err != nil {
			return nil, err
		}

		queries = append(queries, map[string]interface{}{
			"query":        strings.TrimSpace(query),
			"calls":        calls,
			"total_time":   totalTime,
			"mean_time":    meanTime,
			"rows":         rowsAffected,
		})
	}

	return queries, rows.Err()
}

// GetLocks returns current locks in the database
func (p *PostgreSQLDB) GetLocks(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT
			pl.pid,
			pl.mode,
			pl.locktype,
			pl.granted,
			pa.query,
			pa.state,
			pa.query_start
		FROM pg_locks pl
		LEFT JOIN pg_stat_activity pa ON pl.pid = pa.pid
		WHERE pl.pid != pg_backend_pid()
		ORDER BY pl.granted ASC, pa.query_start ASC
	`

	rows, err := p.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locks []map[string]interface{}
	for rows.Next() {
		var pid int64
		var mode, locktype, state string
		var granted bool
		var query *string
		var queryStart *string

		if err := rows.Scan(&pid, &mode, &locktype, &granted, &query, &state, &queryStart); err != nil {
			return nil, err
		}

		lock := map[string]interface{}{
			"pid":      pid,
			"mode":     mode,
			"locktype": locktype,
			"granted":  granted,
			"state":    state,
		}

		if query != nil {
			lock["query"] = strings.TrimSpace(*query)
		}
		if queryStart != nil {
			lock["query_start"] = *queryStart
		}

		locks = append(locks, lock)
	}

	return locks, rows.Err()
}

// KillQuery terminates a running query by PID
func (p *PostgreSQLDB) KillQuery(ctx context.Context, pid int64) error {
	_, err := p.Exec(ctx, "SELECT pg_cancel_backend($1)", pid)
	return err
}

// KillConnection terminates a connection by PID
func (p *PostgreSQLDB) KillConnection(ctx context.Context, pid int64) error {
	_, err := p.Exec(ctx, "SELECT pg_terminate_backend($1)", pid)
	return err
}

// Backup creates a logical backup using pg_dump
func (p *PostgreSQLDB) Backup(ctx context.Context, destPath string) error {
	// This would typically call pg_dump externally
	// For now, we'll return an error indicating external tool needed
	return fmt.Errorf("backup requires external pg_dump tool")
}

// Restore restores from a pg_dump backup
func (p *PostgreSQLDB) Restore(ctx context.Context, sourcePath string) error {
	// This would typically call psql or pg_restore externally
	// For now, we'll return an error indicating external tool needed
	return fmt.Errorf("restore requires external psql or pg_restore tool")
}