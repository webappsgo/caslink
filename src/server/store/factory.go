package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	_ "modernc.org/sqlite"
)

// configurePool applies the spec-canonical connection pool settings to db.
// Per AI.md PART 10: all drivers must set all four pool parameters.
//
//   - maxOpen: 25 — allows concurrent query throughput without starving the server.
//   - maxIdle: 10 — keep a hot pool without holding unnecessary connections.
//   - connMaxLifetime: 30 min — recycle connections before they hit server-side
//     idle timeouts (typically 60 min for MySQL, 1 h for PostgreSQL default).
//   - connMaxIdleTime: 5 min — release connections that are idle but not expired.
func configurePool(db *sql.DB) {
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
}

// OpenDB opens a *sql.DB using driver and DSN derived from config values.
// Supported drivers: sqlite (default), postgres, mysql, sqlserver.
func OpenDB(driver, host string, port int, name, username, password, sslMode, filePath string) (*sql.DB, error) {
	switch strings.ToLower(driver) {
	case "postgres", "postgresql":
		dsn := buildPostgresDSN(host, port, name, username, password, sslMode)
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return nil, fmt.Errorf("postgres: open failed: %w", err)
		}
		configurePool(db)
		return db, nil

	case "mysql", "mariadb":
		dsn := buildMySQLDSN(host, port, name, username, password)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, fmt.Errorf("mysql: open failed: %w", err)
		}
		configurePool(db)
		return db, nil

	case "sqlserver", "mssql":
		dsn := buildSQLServerDSN(host, port, name, username, password)
		db, err := sql.Open("sqlserver", dsn)
		if err != nil {
			return nil, fmt.Errorf("sqlserver: open failed: %w", err)
		}
		configurePool(db)
		return db, nil

	default:
		// sqlite — filePath must be a directory; we append the filename.
		if filePath == "" {
			return nil, fmt.Errorf("sqlite requires a file path")
		}
		return openSQLite(filePath)
	}
}

// OpenStoreWithConfig opens both ServerDB and UsersDB using the given driver
// configuration. For SQLite, serverFile and usersFile are full paths to the
// .db files. For remote drivers the same host/port/user/pass is used and the
// database name has "_server" and "_users" appended.
func OpenStoreWithConfig(
	driver, host string, port int,
	baseName, username, password, sslMode string,
	dataDir string,
) (*Store, error) {
	var serverDB, usersDB *sql.DB
	var err error

	drv := strings.ToLower(driver)
	switch drv {
	case "postgres", "postgresql", "mysql", "mariadb", "sqlserver", "mssql":
		serverDB, err = OpenDB(drv, host, port, baseName+"_server", username, password, sslMode, "")
		if err != nil {
			return nil, fmt.Errorf("failed to open server db: %w", err)
		}
		usersDB, err = OpenDB(drv, host, port, baseName+"_users", username, password, sslMode, "")
		if err != nil {
			serverDB.Close()
			return nil, fmt.Errorf("failed to open users db: %w", err)
		}

	default:
		// SQLite — use files inside dataDir/db/
		dbDir := filepath.Join(dataDir, "db")
		serverDB, err = OpenDB("sqlite", "", 0, "", "", "", "", filepath.Join(dbDir, "server.db"))
		if err != nil {
			return nil, fmt.Errorf("failed to open server.db: %w", err)
		}
		usersDB, err = OpenDB("sqlite", "", 0, "", "", "", "", filepath.Join(dbDir, "users.db"))
		if err != nil {
			serverDB.Close()
			return nil, fmt.Errorf("failed to open users.db: %w", err)
		}
	}

	st := &Store{
		ServerDB: serverDB,
		UsersDB:  usersDB,
		driver:   drv,
	}
	if err := st.InitSchema(); err != nil {
		st.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return st, nil
}

// buildPostgresDSN builds a postgres connection string.
func buildPostgresDSN(host string, port int, dbName, user, password, sslMode string) string {
	if sslMode == "" {
		sslMode = "require"
	}
	if port == 0 {
		port = 5432
	}
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		host, port, dbName, user, password, sslMode,
	)
}

// buildMySQLDSN builds a MySQL/MariaDB DSN.
func buildMySQLDSN(host string, port int, dbName, user, password string) string {
	if port == 0 {
		port = 3306
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_unicode_ci",
		user, password, host, port, dbName,
	)
}

// buildSQLServerDSN builds a SQL Server connection string.
func buildSQLServerDSN(host string, port int, dbName, user, password string) string {
	if port == 0 {
		port = 1433
	}
	return fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
		user, password, host, port, dbName,
	)
}
