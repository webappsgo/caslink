package db

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/casjaysdevdocker/caslink/internal/config"
)

// SQLiteDB represents SQLite-specific database operations
type SQLiteDB struct {
	*DB
}

// NewSQLite creates a new SQLite database connection
func NewSQLite(cfg *config.DatabaseConfig) (*SQLiteDB, error) {
	// Ensure SQLite-specific configuration
	if cfg.SQLiteWAL {
		cfg.URL = addSQLiteParam(cfg.URL, "_journal_mode", "WAL")
	}

	if cfg.SQLiteCacheSize != "" {
		cfg.URL = addSQLiteParam(cfg.URL, "_cache_size", cfg.SQLiteCacheSize)
	}

	if cfg.SQLiteBusyTimeout > 0 {
		cfg.URL = addSQLiteParam(cfg.URL, "_busy_timeout", fmt.Sprintf("%d", int(cfg.SQLiteBusyTimeout.Milliseconds())))
	}

	// Add other SQLite optimizations
	cfg.URL = addSQLiteParam(cfg.URL, "_foreign_keys", "on")
	cfg.URL = addSQLiteParam(cfg.URL, "_synchronous", "NORMAL")
	cfg.URL = addSQLiteParam(cfg.URL, "_temp_store", "memory")

	db, err := New(cfg, nil)
	if err != nil {
		return nil, err
	}

	sqliteDB := &SQLiteDB{DB: db}

	// Run SQLite-specific optimizations
	if err := sqliteDB.optimize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to optimize SQLite: %w", err)
	}

	return sqliteDB, nil
}

// addSQLiteParam adds a parameter to the SQLite connection string
func addSQLiteParam(url, key, value string) string {
	if strings.Contains(url, "?") {
		return fmt.Sprintf("%s&%s=%s", url, key, value)
	}
	return fmt.Sprintf("%s?%s=%s", url, key, value)
}

// optimize applies SQLite-specific optimizations
func (s *SQLiteDB) optimize(ctx context.Context) error {
	optimizations := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = memory",
		"PRAGMA mmap_size = 268435456", // 256MB
		"PRAGMA foreign_keys = ON",
		"PRAGMA auto_vacuum = INCREMENTAL",
	}

	for _, pragma := range optimizations {
		if _, err := s.Exec(ctx, pragma); err != nil {
			return fmt.Errorf("failed to execute pragma %s: %w", pragma, err)
		}
	}

	return nil
}

// Vacuum performs SQLite VACUUM operation
func (s *SQLiteDB) Vacuum(ctx context.Context) error {
	_, err := s.Exec(ctx, "VACUUM")
	return err
}

// Analyze performs SQLite ANALYZE operation
func (s *SQLiteDB) Analyze(ctx context.Context) error {
	_, err := s.Exec(ctx, "ANALYZE")
	return err
}

// CheckIntegrity checks SQLite database integrity
func (s *SQLiteDB) CheckIntegrity(ctx context.Context) ([]string, error) {
	rows, err := s.Query(ctx, "PRAGMA integrity_check")
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

// GetWALInfo returns WAL mode information
func (s *SQLiteDB) GetWALInfo(ctx context.Context) (map[string]interface{}, error) {
	info := make(map[string]interface{})

	// Check journal mode
	row := s.QueryRow(ctx, "PRAGMA journal_mode")
	var journalMode string
	if err := row.Scan(&journalMode); err != nil {
		return nil, err
	}
	info["journal_mode"] = journalMode

	if journalMode == "wal" {
		// Get WAL checkpoint info
		row = s.QueryRow(ctx, "PRAGMA wal_checkpoint")
		var busy, log, checkpointed int
		if err := row.Scan(&busy, &log, &checkpointed); err != nil {
			return nil, err
		}
		info["wal_checkpoint"] = map[string]int{
			"busy":         busy,
			"log":          log,
			"checkpointed": checkpointed,
		}
	}

	return info, nil
}

// Backup creates a backup of the SQLite database
func (s *SQLiteDB) Backup(ctx context.Context, destPath string) error {
	// Create destination directory if it doesn't exist
	if err := createDir(filepath.Dir(destPath)); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Use SQLite backup API
	query := fmt.Sprintf("VACUUM INTO '%s'", destPath)
	_, err := s.Exec(ctx, query)
	return err
}

// Restore restores a SQLite database from backup
func (s *SQLiteDB) Restore(ctx context.Context, sourcePath string) error {
	// This would typically involve replacing the current database file
	// For now, we'll implement a simple copy operation
	return fmt.Errorf("restore operation not implemented for SQLite")
}

// GetTableSizes returns the size of each table in the database
func (s *SQLiteDB) GetTableSizes(ctx context.Context) (map[string]int64, error) {
	query := `
		SELECT name, SUM("pgsize") as size
		FROM "dbstat"
		WHERE name NOT LIKE 'sqlite_%'
		GROUP BY name
		ORDER BY size DESC
	`

	rows, err := s.Query(ctx, query)
	if err != nil {
		// Fallback to simpler method if dbstat is not available
		return s.getTableSizesFallback(ctx)
	}
	defer rows.Close()

	sizes := make(map[string]int64)
	for rows.Next() {
		var tableName string
		var size int64
		if err := rows.Scan(&tableName, &size); err != nil {
			return nil, err
		}
		sizes[tableName] = size
	}

	return sizes, rows.Err()
}

// getTableSizesFallback provides a fallback method for getting table sizes
func (s *SQLiteDB) getTableSizesFallback(ctx context.Context) (map[string]int64, error) {
	// Get all table names
	rows, err := s.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}

	// Get count for each table (approximate size)
	sizes := make(map[string]int64)
	for _, table := range tables {
		row := s.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table))
		var count int64
		if err := row.Scan(&count); err != nil {
			continue // Skip tables we can't count
		}
		sizes[table] = count
	}

	return sizes, nil
}

// GetDatabaseStats returns SQLite database statistics
func (s *SQLiteDB) GetDatabaseStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Page count and size
	row := s.QueryRow(ctx, "PRAGMA page_count")
	var pageCount int64
	if err := row.Scan(&pageCount); err == nil {
		stats["page_count"] = pageCount
	}

	row = s.QueryRow(ctx, "PRAGMA page_size")
	var pageSize int64
	if err := row.Scan(&pageSize); err == nil {
		stats["page_size"] = pageSize
		if pageCount > 0 {
			stats["database_size"] = pageCount * pageSize
		}
	}

	// Free pages
	row = s.QueryRow(ctx, "PRAGMA freelist_count")
	var freePages int64
	if err := row.Scan(&freePages); err == nil {
		stats["free_pages"] = freePages
	}

	// Cache size
	row = s.QueryRow(ctx, "PRAGMA cache_size")
	var cacheSize int64
	if err := row.Scan(&cacheSize); err == nil {
		stats["cache_size"] = cacheSize
	}

	// User version
	row = s.QueryRow(ctx, "PRAGMA user_version")
	var userVersion int64
	if err := row.Scan(&userVersion); err == nil {
		stats["user_version"] = userVersion
	}

	return stats, nil
}

// createDir creates a directory if it doesn't exist
func createDir(path string) error {
	if path == "" {
		return nil
	}
	// This is a placeholder - in a real implementation, you'd use os.MkdirAll
	return nil
}