package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// openSQLite opens a SQLite database file
// Uses modernc.org/sqlite (pure Go, CGO_ENABLED=0 compatible)
func openSQLite(dbPath string) (*sql.DB, error) {
	// Ensure database directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Connection string with options
	// Driver name is "sqlite" for modernc.org/sqlite
	connStr := fmt.Sprintf("file:%s?_journal_mode=WAL&_timeout=5000&_fk=1", dbPath)

	// Open database
	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(0)

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable foreign keys and WAL mode
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -64000", // 64MB cache
		"PRAGMA temp_store = MEMORY",
		"PRAGMA busy_timeout = 5000",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	return db, nil
}

// DBType returns the normalised database driver name (sqlite, postgres, mysql, sqlserver).
func (s *Store) DBType() string {
	if s.driver != "" {
		return s.driver
	}
	return "sqlite"
}

// DBLocality returns "local" for SQLite and "remote" for network databases.
func (s *Store) DBLocality() string {
	switch s.DBType() {
	case "postgres", "postgresql", "mysql", "mariadb", "sqlserver", "mssql":
		return "remote"
	default:
		return "local"
	}
}

// Ping checks if the database connection is alive
func (s *Store) Ping() error {
	if err := s.ServerDB.Ping(); err != nil {
		return fmt.Errorf("server.db ping failed: %w", err)
	}

	if err := s.UsersDB.Ping(); err != nil {
		return fmt.Errorf("users.db ping failed: %w", err)
	}

	return nil
}

// Stats returns database statistics
func (s *Store) Stats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Server DB stats
	var urlCount, clickCount int
	if err := s.ServerDB.QueryRow("SELECT COUNT(*) FROM urls").Scan(&urlCount); err != nil {
		urlCount = 0
	}
	if err := s.ServerDB.QueryRow("SELECT COUNT(*) FROM clicks").Scan(&clickCount); err != nil {
		clickCount = 0
	}

	// Users DB stats
	var userCount, adminCount int
	if err := s.UsersDB.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		userCount = 0
	}
	if err := s.UsersDB.QueryRow("SELECT COUNT(*) FROM admins").Scan(&adminCount); err != nil {
		adminCount = 0
	}

	stats["urls"] = urlCount
	stats["clicks"] = clickCount
	stats["users"] = userCount
	stats["admins"] = adminCount
	stats["server_db_open_conns"] = s.ServerDB.Stats().OpenConnections
	stats["users_db_open_conns"] = s.UsersDB.Stats().OpenConnections

	return stats, nil
}
