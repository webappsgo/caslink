package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
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

// quotePostgresValue quotes a libpq key=value DSN value so that spaces,
// quotes, backslashes, or any other special character in the value are
// never interpreted as a field delimiter. Per libpq's connection-string
// format, values are always safe to single-quote, escaping any embedded
// backslash or single quote with a leading backslash.
func quotePostgresValue(v string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v)
	return "'" + escaped + "'"
}

// buildPostgresDSN builds a postgres connection string, quoting every value
// so credentials containing spaces, quotes, or backslashes never break the
// key=value parsing or leak into an unintended field.
func buildPostgresDSN(host string, port int, dbName, user, password, sslMode string) string {
	if sslMode == "" {
		sslMode = "require"
	}
	if port == 0 {
		port = 5432
	}
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		quotePostgresValue(host), port, quotePostgresValue(dbName),
		quotePostgresValue(user), quotePostgresValue(password), quotePostgresValue(sslMode),
	)
}

// buildMySQLDSN builds a MySQL/MariaDB DSN using the driver's own
// mysql.Config/FormatDSN, which correctly escapes credentials containing
// special characters (e.g. "@" or ":" in the password) instead of the naive
// "user:password@tcp(host:port)/db" string formatting, which breaks on
// those characters since they're also DSN field delimiters.
func buildMySQLDSN(host string, port int, dbName, user, password string) string {
	if port == 0 {
		port = 3306
	}
	cfg := mysqldriver.NewConfig()
	cfg.User = user
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", host, port)
	cfg.DBName = dbName
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Collation = "utf8mb4_unicode_ci"
	return cfg.FormatDSN()
}

// buildSQLServerDSN builds a SQL Server connection string using net/url so
// credentials containing special characters (spaces, "@", ":", "/") are
// percent-encoded instead of interpolated raw into the URL, which would
// otherwise produce a malformed DSN or connect to the wrong target.
func buildSQLServerDSN(host string, port int, dbName, user, password string) string {
	if port == 0 {
		port = 1433
	}
	u := url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%d", host, port),
	}
	q := url.Values{}
	q.Set("database", dbName)
	u.RawQuery = q.Encode()
	return u.String()
}
