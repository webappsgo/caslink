package migrations

import (
	"context"
	"crypto/md5"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// MigrationService manages database migrations
type MigrationService struct {
	db       *db.DB
	logger   *logrus.Logger
	runner   *Runner
	validator *Validator
}

// Migration represents a database migration
type Migration struct {
	ID           string                         `json:"id"`
	Description  string                         `json:"description"`
	Dependencies []string                       `json:"dependencies"`
	Up           map[string][]string           `json:"up"`
	Down         map[string][]string           `json:"down"`
	Validate     func(*db.DB) error            `json:"-"`
	PostMigrate  func(*db.DB) error            `json:"-"`
}

// MigrationRecord represents a migration record in the database
type MigrationRecord struct {
	ID          string    `json:"id" db:"id"`
	Description string    `json:"description" db:"description"`
	Checksum    string    `json:"checksum" db:"checksum"`
	AppliedAt   time.Time `json:"applied_at" db:"applied_at"`
	AppliedBy   string    `json:"applied_by" db:"applied_by"`
	Success     bool      `json:"success" db:"success"`
	ErrorMsg    *string   `json:"error_msg" db:"error_msg"`
	Duration    int64     `json:"duration" db:"duration"`
}

// NewMigrationService creates a new migration service
func NewMigrationService(database *db.DB, logger *logrus.Logger) (*MigrationService, error) {
	service := &MigrationService{
		db:     database,
		logger: logger,
	}

	service.runner = NewRunner(database, logger)
	service.validator = NewValidator(database, logger)

	// Initialize migration tracking table
	if err := service.initMigrationTable(); err != nil {
		return nil, fmt.Errorf("failed to initialize migration table: %w", err)
	}

	return service, nil
}

// GetPendingMigrations returns migrations that haven't been applied
func (s *MigrationService) GetPendingMigrations(ctx context.Context) ([]*Migration, error) {
	appliedMigrations, err := s.getAppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	appliedMap := make(map[string]bool)
	for _, migration := range appliedMigrations {
		appliedMap[migration.ID] = true
	}

	var pending []*Migration
	for _, migration := range GetAllMigrations() {
		if !appliedMap[migration.ID] {
			pending = append(pending, migration)
		}
	}

	// Sort by ID to ensure proper order
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].ID < pending[j].ID
	})

	return pending, nil
}

// GetAppliedMigrations returns all applied migrations
func (s *MigrationService) GetAppliedMigrations(ctx context.Context) ([]*MigrationRecord, error) {
	return s.getAppliedMigrations(ctx)
}

// Migrate runs all pending migrations
func (s *MigrationService) Migrate(ctx context.Context) error {
	pending, err := s.GetPendingMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending migrations: %w", err)
	}

	if len(pending) == 0 {
		s.logger.Info("No pending migrations")
		return nil
	}

	s.logger.WithField("count", len(pending)).Info("Running pending migrations")

	for _, migration := range pending {
		if err := s.runMigration(ctx, migration, true); err != nil {
			return fmt.Errorf("migration %s failed: %w", migration.ID, err)
		}
	}

	s.logger.Info("All migrations completed successfully")
	return nil
}

// MigrateUp runs a specific migration up
func (s *MigrationService) MigrateUp(ctx context.Context, migrationID string) error {
	migration := GetMigrationByID(migrationID)
	if migration == nil {
		return fmt.Errorf("migration %s not found", migrationID)
	}

	// Check if already applied
	applied, err := s.isMigrationApplied(ctx, migrationID)
	if err != nil {
		return err
	}
	if applied {
		return fmt.Errorf("migration %s is already applied", migrationID)
	}

	return s.runMigration(ctx, migration, true)
}

// MigrateDown runs a specific migration down
func (s *MigrationService) MigrateDown(ctx context.Context, migrationID string) error {
	migration := GetMigrationByID(migrationID)
	if migration == nil {
		return fmt.Errorf("migration %s not found", migrationID)
	}

	// Check if migration is applied
	applied, err := s.isMigrationApplied(ctx, migrationID)
	if err != nil {
		return err
	}
	if !applied {
		return fmt.Errorf("migration %s is not applied", migrationID)
	}

	return s.runMigration(ctx, migration, false)
}

// Rollback rolls back the last applied migration
func (s *MigrationService) Rollback(ctx context.Context) error {
	applied, err := s.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	// Get the latest migration
	latest := applied[len(applied)-1]
	return s.MigrateDown(ctx, latest.ID)
}

// RollbackTo rolls back to a specific migration
func (s *MigrationService) RollbackTo(ctx context.Context, migrationID string) error {
	applied, err := s.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Find the target migration
	targetIndex := -1
	for i, migration := range applied {
		if migration.ID == migrationID {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return fmt.Errorf("migration %s is not applied", migrationID)
	}

	// Rollback migrations in reverse order
	for i := len(applied) - 1; i > targetIndex; i-- {
		if err := s.MigrateDown(ctx, applied[i].ID); err != nil {
			return fmt.Errorf("failed to rollback migration %s: %w", applied[i].ID, err)
		}
	}

	return nil
}

// ValidateMigrations validates all migrations without applying them
func (s *MigrationService) ValidateMigrations(ctx context.Context) error {
	migrations := GetAllMigrations()

	for _, migration := range migrations {
		if err := s.validator.ValidateMigration(ctx, migration); err != nil {
			return fmt.Errorf("validation failed for migration %s: %w", migration.ID, err)
		}
	}

	s.logger.Info("All migrations validated successfully")
	return nil
}

// GetMigrationStatus returns the status of all migrations
func (s *MigrationService) GetMigrationStatus(ctx context.Context) (map[string]string, error) {
	applied, err := s.getAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	appliedMap := make(map[string]bool)
	for _, migration := range applied {
		appliedMap[migration.ID] = true
	}

	status := make(map[string]string)
	for _, migration := range GetAllMigrations() {
		if appliedMap[migration.ID] {
			status[migration.ID] = "applied"
		} else {
			status[migration.ID] = "pending"
		}
	}

	return status, nil
}

// runMigration executes a single migration
func (s *MigrationService) runMigration(ctx context.Context, migration *Migration, up bool) error {
	start := time.Now()
	direction := "up"
	if !up {
		direction = "down"
	}

	s.logger.WithFields(logrus.Fields{
		"migration": migration.ID,
		"direction": direction,
	}).Info("Running migration")

	// Validate before running
	if err := s.validator.ValidateMigration(ctx, migration); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Start transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Run the migration
	var migrationErr error
	if up {
		migrationErr = s.runner.RunUp(ctx, tx, migration)
	} else {
		migrationErr = s.runner.RunDown(ctx, tx, migration)
	}

	duration := time.Since(start)

	// Record the migration attempt
	recordErr := s.recordMigration(ctx, tx, migration, up, migrationErr, duration)
	if recordErr != nil {
		s.logger.WithError(recordErr).Error("Failed to record migration")
	}

	if migrationErr != nil {
		return migrationErr
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	// Run post-migration hooks
	if up && migration.PostMigrate != nil {
		if err := migration.PostMigrate(s.db); err != nil {
			s.logger.WithError(err).Warn("Post-migration hook failed")
		}
	}

	s.logger.WithFields(logrus.Fields{
		"migration": migration.ID,
		"direction": direction,
		"duration":  duration,
	}).Info("Migration completed successfully")

	return nil
}

// initMigrationTable creates the migration tracking table
func (s *MigrationService) initMigrationTable() error {
	var query string

	switch s.db.Type() {
	case "sqlite3":
		query = `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				id TEXT PRIMARY KEY,
				description TEXT NOT NULL,
				checksum TEXT NOT NULL,
				applied_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				applied_by TEXT DEFAULT 'system',
				success BOOLEAN DEFAULT TRUE,
				error_msg TEXT,
				duration INTEGER DEFAULT 0
			)
		`
	case "postgres":
		query = `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				id TEXT PRIMARY KEY,
				description TEXT NOT NULL,
				checksum TEXT NOT NULL,
				applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				applied_by TEXT DEFAULT 'system',
				success BOOLEAN DEFAULT TRUE,
				error_msg TEXT,
				duration BIGINT DEFAULT 0
			)
		`
	case "mysql":
		query = `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				id VARCHAR(255) PRIMARY KEY,
				description TEXT NOT NULL,
				checksum VARCHAR(32) NOT NULL,
				applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				applied_by VARCHAR(255) DEFAULT 'system',
				success BOOLEAN DEFAULT TRUE,
				error_msg TEXT,
				duration BIGINT DEFAULT 0
			)
		`
	case "sqlserver":
		query = `
			IF NOT EXISTS (SELECT * FROM sysobjects WHERE name='schema_migrations' AND xtype='U')
			CREATE TABLE schema_migrations (
				id NVARCHAR(255) PRIMARY KEY,
				description NVARCHAR(MAX) NOT NULL,
				checksum NVARCHAR(32) NOT NULL,
				applied_at DATETIME2 DEFAULT GETUTCDATE(),
				applied_by NVARCHAR(255) DEFAULT 'system',
				success BIT DEFAULT 1,
				error_msg NVARCHAR(MAX),
				duration BIGINT DEFAULT 0
			)
		`
	default:
		return fmt.Errorf("unsupported database type: %s", s.db.Type())
	}

	_, err := s.db.Exec(context.Background(), query)
	return err
}

// getAppliedMigrations retrieves all applied migrations from the database
func (s *MigrationService) getAppliedMigrations(ctx context.Context) ([]*MigrationRecord, error) {
	query := `
		SELECT id, description, checksum, applied_at, applied_by, success, error_msg, duration
		FROM schema_migrations
		WHERE success = true
		ORDER BY applied_at
	`

	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []*MigrationRecord
	for rows.Next() {
		var migration MigrationRecord
		var errorMsg sql.NullString

		err := rows.Scan(
			&migration.ID,
			&migration.Description,
			&migration.Checksum,
			&migration.AppliedAt,
			&migration.AppliedBy,
			&migration.Success,
			&errorMsg,
			&migration.Duration,
		)
		if err != nil {
			return nil, err
		}

		if errorMsg.Valid {
			migration.ErrorMsg = &errorMsg.String
		}

		migrations = append(migrations, &migration)
	}

	return migrations, rows.Err()
}

// isMigrationApplied checks if a migration has been applied
func (s *MigrationService) isMigrationApplied(ctx context.Context, migrationID string) (bool, error) {
	query := "SELECT COUNT(*) FROM schema_migrations WHERE id = ? AND success = true"
	if s.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM schema_migrations WHERE id = $1 AND success = true"
	}

	var count int
	err := s.db.QueryRow(ctx, query, migrationID).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// recordMigration records a migration attempt in the database
func (s *MigrationService) recordMigration(ctx context.Context, tx *sql.Tx, migration *Migration, up bool, migrationErr error, duration time.Duration) error {
	checksum := s.calculateChecksum(migration)
	success := migrationErr == nil
	var errorMsg *string
	if migrationErr != nil {
		msg := migrationErr.Error()
		errorMsg = &msg
	}

	var query string
	switch s.db.Type() {
	case "postgres":
		if up {
			query = `
				INSERT INTO schema_migrations (id, description, checksum, success, error_msg, duration)
				VALUES ($1, $2, $3, $4, $5, $6)
			`
		} else {
			query = "DELETE FROM schema_migrations WHERE id = $1"
		}
	default:
		if up {
			query = `
				INSERT INTO schema_migrations (id, description, checksum, success, error_msg, duration)
				VALUES (?, ?, ?, ?, ?, ?)
			`
		} else {
			query = "DELETE FROM schema_migrations WHERE id = ?"
		}
	}

	if up {
		_, err := tx.ExecContext(ctx, query,
			migration.ID,
			migration.Description,
			checksum,
			success,
			errorMsg,
			duration.Milliseconds(),
		)
		return err
	} else {
		// For rollback, just delete the record
		_, err := tx.ExecContext(ctx, query, migration.ID)
		return err
	}
}

// calculateChecksum calculates a checksum for the migration
func (s *MigrationService) calculateChecksum(migration *Migration) string {
	content := fmt.Sprintf("%s:%s", migration.ID, migration.Description)
	for dbType, queries := range migration.Up {
		content += fmt.Sprintf(":%s:%s", dbType, strings.Join(queries, ";"))
	}
	for dbType, queries := range migration.Down {
		content += fmt.Sprintf(":%s:%s", dbType, strings.Join(queries, ";"))
	}

	hash := md5.Sum([]byte(content))
	return fmt.Sprintf("%x", hash)
}