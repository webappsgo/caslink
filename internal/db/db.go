package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/lib/pq"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/denisenkom/go-mssqldb"
)

// ErrNoRows is returned when a query returns no rows
var ErrNoRows = sql.ErrNoRows

// DB represents the database connection and operations
type DB struct {
	db       *sql.DB
	dbType   string
	config   *config.DatabaseConfig
	logger   *logrus.Logger
}

// New creates a new database connection
func New(cfg *config.DatabaseConfig, logger *logrus.Logger) (*DB, error) {
	dbType, dsn, err := parseConnectionConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Open database connection
	db, err := sql.Open(dbType, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"type": dbType,
		"host": cfg.Host,
		"name": cfg.Name,
	}).Info("Database connection established")

	return &DB{
		db:     db,
		dbType: dbType,
		config: cfg,
		logger: logger,
	}, nil
}

// parseConnectionConfig parses the database configuration and returns the driver and DSN
func parseConnectionConfig(cfg *config.DatabaseConfig) (string, string, error) {
	// If URL is provided, parse it
	if cfg.URL != "" {
		return parseConnectionURL(cfg.URL)
	}

	// Otherwise, build from individual fields
	return buildConnectionString(cfg)
}

// parseConnectionURL parses a database URL
func parseConnectionURL(url string) (string, string, error) {
	parts := strings.SplitN(url, "://", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid database URL format")
	}

	scheme := parts[0]
	dsn := parts[1]

	switch scheme {
	case "sqlite", "sqlite3":
		return "sqlite3", dsn, nil
	case "postgresql", "postgres":
		return "postgres", url, nil
	case "mysql":
		return "mysql", dsn, nil
	case "sqlserver":
		return "sqlserver", url, nil
	default:
		return "", "", fmt.Errorf("unsupported database scheme: %s", scheme)
	}
}

// buildConnectionString builds a connection string from individual config fields
func buildConnectionString(cfg *config.DatabaseConfig) (string, string, error) {
	switch cfg.Type {
	case "sqlite":
		return "sqlite3", cfg.Name, nil

	case "postgresql", "postgres":
		port := cfg.Port
		if port == 0 {
			port = 5432
		}

		sslMode := cfg.SSLMode
		if sslMode == "auto" {
			sslMode = "prefer"
		}

		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			cfg.Host, port, cfg.Username, cfg.Password, cfg.Name, sslMode)
		return "postgres", dsn, nil

	case "mysql", "mariadb":
		port := cfg.Port
		if port == 0 {
			port = 3306
		}

		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4",
			cfg.Username, cfg.Password, cfg.Host, port, cfg.Name)
		return "mysql", dsn, nil

	case "sqlserver":
		port := cfg.Port
		if port == 0 {
			port = 1433
		}

		dsn := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%d;database=%s",
			cfg.Host, cfg.Username, cfg.Password, port, cfg.Name)
		return "sqlserver", dsn, nil

	default:
		return "", "", fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
}

// Close closes the database connection
func (d *DB) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// DB returns the underlying *sql.DB
func (d *DB) DB() *sql.DB {
	return d.db
}

// Type returns the database type
func (d *DB) Type() string {
	return d.dbType
}

// Ping tests the database connection
func (d *DB) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

// Health returns database health information
func (d *DB) Health(ctx context.Context) map[string]interface{} {
	health := map[string]interface{}{
		"type": d.dbType,
		"status": "healthy",
	}

	// Test connection
	start := time.Now()
	if err := d.Ping(ctx); err != nil {
		health["status"] = "unhealthy"
		health["error"] = err.Error()
		return health
	}
	health["response_time"] = time.Since(start).String()

	// Get connection stats
	stats := d.db.Stats()
	health["connections"] = map[string]interface{}{
		"open":        stats.OpenConnections,
		"in_use":      stats.InUse,
		"idle":        stats.Idle,
		"max_open":    stats.MaxOpenConnections,
		"max_idle":    d.config.MaxIdleConns,
		"max_lifetime": d.config.ConnMaxLifetime.String(),
	}

	return health
}

// Begin starts a new transaction
func (d *DB) Begin(ctx context.Context) (*sql.Tx, error) {
	return d.db.BeginTx(ctx, nil)
}

// BeginTx starts a new transaction with options
func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return d.db.BeginTx(ctx, opts)
}

// Exec executes a query without returning any rows
func (d *DB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	result, err := d.db.ExecContext(ctx, query, args...)

	duration := time.Since(start)
	if duration > d.config.SlowQueryThreshold {
		d.logger.WithFields(logrus.Fields{
			"query":    query,
			"duration": duration,
			"args":     args,
		}).Warn("Slow query detected")
	}

	if d.config.LogQueries {
		d.logger.WithFields(logrus.Fields{
			"query":    query,
			"duration": duration,
			"args":     args,
		}).Debug("Query executed")
	}

	return result, err
}

// Query executes a query that returns rows
func (d *DB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := d.db.QueryContext(ctx, query, args...)

	duration := time.Since(start)
	if duration > d.config.SlowQueryThreshold {
		d.logger.WithFields(logrus.Fields{
			"query":    query,
			"duration": duration,
			"args":     args,
		}).Warn("Slow query detected")
	}

	if d.config.LogQueries {
		d.logger.WithFields(logrus.Fields{
			"query":    query,
			"duration": duration,
			"args":     args,
		}).Debug("Query executed")
	}

	return rows, err
}

// QueryRow executes a query that is expected to return at most one row
func (d *DB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	start := time.Now()
	row := d.db.QueryRowContext(ctx, query, args...)

	duration := time.Since(start)
	if duration > d.config.SlowQueryThreshold {
		d.logger.WithFields(logrus.Fields{
			"query":    query,
			"duration": duration,
			"args":     args,
		}).Warn("Slow query detected")
	}

	if d.config.LogQueries {
		d.logger.WithFields(logrus.Fields{
			"query":    query,
			"duration": duration,
			"args":     args,
		}).Debug("Query executed")
	}

	return row
}

// Prepare creates a prepared statement
func (d *DB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return d.db.PrepareContext(ctx, query)
}