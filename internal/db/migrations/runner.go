package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Runner executes migration scripts
type Runner struct {
	db     *db.DB
	logger *logrus.Logger
}

// NewRunner creates a new migration runner
func NewRunner(database *db.DB, logger *logrus.Logger) *Runner {
	return &Runner{
		db:     database,
		logger: logger,
	}
}

// RunUp executes the up migration
func (r *Runner) RunUp(ctx context.Context, tx *sql.Tx, migration *Migration) error {
	return r.runMigration(ctx, tx, migration, migration.Up, "up")
}

// RunDown executes the down migration
func (r *Runner) RunDown(ctx context.Context, tx *sql.Tx, migration *Migration) error {
	return r.runMigration(ctx, tx, migration, migration.Down, "down")
}

// runMigration executes migration queries for the current database type
func (r *Runner) runMigration(ctx context.Context, tx *sql.Tx, migration *Migration, queries map[string][]string, direction string) error {
	dbType := r.normalizeDBType(r.db.Type())

	// Get queries for current database type
	dbQueries, exists := queries[dbType]
	if !exists {
		// Try fallback to 'all' if specific type not found
		if allQueries, hasAll := queries["all"]; hasAll {
			dbQueries = allQueries
		} else {
			return fmt.Errorf("no %s queries found for database type %s", direction, dbType)
		}
	}

	// Execute each query
	for i, query := range dbQueries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}

		r.logger.WithFields(logrus.Fields{
			"migration": migration.ID,
			"direction": direction,
			"query_num": i + 1,
			"db_type":   dbType,
		}).Debug("Executing query")

		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to execute query %d: %w\nQuery: %s", i+1, err, query)
		}
	}

	return nil
}

// normalizeDBType normalizes database type names
func (r *Runner) normalizeDBType(dbType string) string {
	switch dbType {
	case "sqlite3":
		return "sqlite"
	case "postgres":
		return "postgresql"
	case "mysql":
		return "mysql"
	case "sqlserver":
		return "sqlserver"
	default:
		return dbType
	}
}

// TestMigration tests a migration in a separate transaction that gets rolled back
func (r *Runner) TestMigration(ctx context.Context, migration *Migration, direction string) error {
	// Start a test transaction
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start test transaction: %w", err)
	}
	defer tx.Rollback() // Always rollback test transactions

	r.logger.WithFields(logrus.Fields{
		"migration": migration.ID,
		"direction": direction,
	}).Debug("Testing migration")

	// Run the migration
	if direction == "up" {
		err = r.RunUp(ctx, tx, migration)
	} else {
		err = r.RunDown(ctx, tx, migration)
	}

	if err != nil {
		return fmt.Errorf("migration test failed: %w", err)
	}

	r.logger.WithFields(logrus.Fields{
		"migration": migration.ID,
		"direction": direction,
	}).Debug("Migration test successful")

	return nil
}

// GeneratePlan generates an execution plan for migrations
func (r *Runner) GeneratePlan(migrations []*Migration, direction string) *ExecutionPlan {
	plan := &ExecutionPlan{
		Direction:  direction,
		Migrations: make([]*PlannedMigration, 0, len(migrations)),
	}

	dbType := r.normalizeDBType(r.db.Type())

	for _, migration := range migrations {
		planned := &PlannedMigration{
			Migration: migration,
			Queries:   []string{},
			Warnings:  []string{},
		}

		// Get queries for this migration
		var queries map[string][]string
		if direction == "up" {
			queries = migration.Up
		} else {
			queries = migration.Down
		}

		// Get queries for current database type
		if dbQueries, exists := queries[dbType]; exists {
			planned.Queries = dbQueries
		} else if allQueries, hasAll := queries["all"]; hasAll {
			planned.Queries = allQueries
		} else {
			planned.Warnings = append(planned.Warnings,
				fmt.Sprintf("No %s queries found for database type %s", direction, dbType))
		}

		// Check for potentially dangerous operations
		planned.Warnings = append(planned.Warnings, r.analyzeQueries(planned.Queries)...)

		plan.Migrations = append(plan.Migrations, planned)
	}

	return plan
}

// analyzeQueries analyzes queries for potentially dangerous operations
func (r *Runner) analyzeQueries(queries []string) []string {
	var warnings []string

	dangerousPatterns := []struct {
		pattern string
		warning string
	}{
		{"DROP TABLE", "Drops table - data loss possible"},
		{"DROP COLUMN", "Drops column - data loss possible"},
		{"DROP INDEX", "Drops index - performance impact possible"},
		{"ALTER TABLE.*DROP", "Alters table structure - data loss possible"},
		{"DELETE FROM", "Deletes data - data loss possible"},
		{"TRUNCATE", "Truncates table - data loss possible"},
		{"UPDATE.*SET", "Updates data - verify WHERE clause"},
	}

	for _, query := range queries {
		upperQuery := strings.ToUpper(strings.TrimSpace(query))
		for _, pattern := range dangerousPatterns {
			if strings.Contains(upperQuery, pattern.pattern) {
				warnings = append(warnings, pattern.warning)
				break
			}
		}
	}

	return warnings
}

// ExecutionPlan represents a migration execution plan
type ExecutionPlan struct {
	Direction  string              `json:"direction"`
	Migrations []*PlannedMigration `json:"migrations"`
}

// PlannedMigration represents a migration in an execution plan
type PlannedMigration struct {
	Migration *Migration `json:"migration"`
	Queries   []string   `json:"queries"`
	Warnings  []string   `json:"warnings"`
}

// HasWarnings returns true if any migration has warnings
func (p *ExecutionPlan) HasWarnings() bool {
	for _, migration := range p.Migrations {
		if len(migration.Warnings) > 0 {
			return true
		}
	}
	return false
}

// GetWarningCount returns the total number of warnings
func (p *ExecutionPlan) GetWarningCount() int {
	count := 0
	for _, migration := range p.Migrations {
		count += len(migration.Warnings)
	}
	return count
}