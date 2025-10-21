package migrations

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Validator validates migrations before execution
type Validator struct {
	db     *db.DB
	logger *logrus.Logger
}

// NewValidator creates a new migration validator
func NewValidator(database *db.DB, logger *logrus.Logger) *Validator {
	return &Validator{
		db:     database,
		logger: logger,
	}
}

// ValidationError represents a validation error
type ValidationError struct {
	Migration string
	Issue     string
	Severity  string // "error", "warning", "info"
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Severity, e.Migration, e.Issue)
}

// ValidationResult represents the result of migration validation
type ValidationResult struct {
	Migration *Migration
	Errors    []*ValidationError
	Warnings  []*ValidationError
	Valid     bool
}

// ValidateMigration validates a single migration
func (v *Validator) ValidateMigration(ctx context.Context, migration *Migration) error {
	result := v.ValidateMigrationDetailed(ctx, migration)

	if !result.Valid {
		var issues []string
		for _, err := range result.Errors {
			issues = append(issues, err.Issue)
		}
		return fmt.Errorf("validation failed: %s", strings.Join(issues, "; "))
	}

	// Log warnings
	for _, warning := range result.Warnings {
		v.logger.WithField("migration", migration.ID).Warn(warning.Issue)
	}

	return nil
}

// ValidateMigrationDetailed performs detailed validation and returns all issues
func (v *Validator) ValidateMigrationDetailed(ctx context.Context, migration *Migration) *ValidationResult {
	result := &ValidationResult{
		Migration: migration,
		Errors:    []*ValidationError{},
		Warnings:  []*ValidationError{},
		Valid:     true,
	}

	// Validate structure
	v.validateStructure(migration, result)

	// Validate SQL syntax
	v.validateSQLSyntax(migration, result)

	// Validate dependencies
	v.validateDependencies(ctx, migration, result)

	// Validate rollback capability
	v.validateRollback(migration, result)

	// Run custom validation if provided
	if migration.Validate != nil {
		if err := migration.Validate(v.db); err != nil {
			result.Errors = append(result.Errors, &ValidationError{
				Migration: migration.ID,
				Issue:     fmt.Sprintf("custom validation failed: %v", err),
				Severity:  "error",
			})
		}
	}

	// Set overall validity
	result.Valid = len(result.Errors) == 0

	return result
}

// validateStructure validates the basic structure of a migration
func (v *Validator) validateStructure(migration *Migration, result *ValidationResult) {
	// Check ID format
	if migration.ID == "" {
		result.Errors = append(result.Errors, &ValidationError{
			Migration: migration.ID,
			Issue:     "migration ID cannot be empty",
			Severity:  "error",
		})
	} else {
		// Check ID format (should be YYYYMMDD_HHMMSS_description)
		matched, _ := regexp.MatchString(`^\d{8}_\d{6}_.+$`, migration.ID)
		if !matched {
			result.Warnings = append(result.Warnings, &ValidationError{
				Migration: migration.ID,
				Issue:     "migration ID should follow format YYYYMMDD_HHMMSS_description",
				Severity:  "warning",
			})
		}
	}

	// Check description
	if migration.Description == "" {
		result.Warnings = append(result.Warnings, &ValidationError{
			Migration: migration.ID,
			Issue:     "migration description is empty",
			Severity:  "warning",
		})
	}

	// Check that we have queries for at least one database type
	if len(migration.Up) == 0 {
		result.Errors = append(result.Errors, &ValidationError{
			Migration: migration.ID,
			Issue:     "no up queries defined",
			Severity:  "error",
		})
	}

	// Check that we have rollback queries
	if len(migration.Down) == 0 {
		result.Warnings = append(result.Warnings, &ValidationError{
			Migration: migration.ID,
			Issue:     "no down queries defined - rollback not possible",
			Severity:  "warning",
		})
	}

	// Check for current database type
	dbType := v.normalizeDBType(v.db.Type())
	if _, exists := migration.Up[dbType]; !exists {
		if _, hasAll := migration.Up["all"]; !hasAll {
			result.Errors = append(result.Errors, &ValidationError{
				Migration: migration.ID,
				Issue:     fmt.Sprintf("no up queries for database type %s", dbType),
				Severity:  "error",
			})
		}
	}
}

// validateSQLSyntax validates SQL syntax for the current database type
func (v *Validator) validateSQLSyntax(migration *Migration, result *ValidationResult) {
	dbType := v.normalizeDBType(v.db.Type())

	// Get queries for current database type
	upQueries := v.getQueriesForDB(migration.Up, dbType)
	downQueries := v.getQueriesForDB(migration.Down, dbType)

	// Validate up queries
	for i, query := range upQueries {
		if err := v.validateQuery(query, dbType); err != nil {
			result.Errors = append(result.Errors, &ValidationError{
				Migration: migration.ID,
				Issue:     fmt.Sprintf("invalid up query %d: %v", i+1, err),
				Severity:  "error",
			})
		}
	}

	// Validate down queries
	for i, query := range downQueries {
		if err := v.validateQuery(query, dbType); err != nil {
			result.Errors = append(result.Errors, &ValidationError{
				Migration: migration.ID,
				Issue:     fmt.Sprintf("invalid down query %d: %v", i+1, err),
				Severity:  "error",
			})
		}
	}
}

// validateQuery validates individual SQL query syntax
func (v *Validator) validateQuery(query, dbType string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("empty query")
	}

	// Basic SQL validation patterns
	switch dbType {
	case "sqlite":
		return v.validateSQLiteQuery(query)
	case "postgresql":
		return v.validatePostgreSQLQuery(query)
	case "mysql":
		return v.validateMySQLQuery(query)
	case "sqlserver":
		return v.validateSQLServerQuery(query)
	default:
		return v.validateGenericQuery(query)
	}
}

// validateSQLiteQuery validates SQLite-specific syntax
func (v *Validator) validateSQLiteQuery(query string) error {
	// Check for SQLite-specific syntax issues
	upperQuery := strings.ToUpper(query)

	// SQLite doesn't support some operations
	if strings.Contains(upperQuery, "DROP COLUMN") {
		return fmt.Errorf("SQLite doesn't support DROP COLUMN")
	}

	if strings.Contains(upperQuery, "ALTER COLUMN") {
		return fmt.Errorf("SQLite doesn't support ALTER COLUMN")
	}

	return v.validateGenericQuery(query)
}

// validatePostgreSQLQuery validates PostgreSQL-specific syntax
func (v *Validator) validatePostgreSQLQuery(query string) error {
	// PostgreSQL-specific validations
	return v.validateGenericQuery(query)
}

// validateMySQLQuery validates MySQL-specific syntax
func (v *Validator) validateMySQLQuery(query string) error {
	// MySQL-specific validations
	return v.validateGenericQuery(query)
}

// validateSQLServerQuery validates SQL Server-specific syntax
func (v *Validator) validateSQLServerQuery(query string) error {
	// SQL Server-specific validations
	return v.validateGenericQuery(query)
}

// validateGenericQuery validates basic SQL syntax
func (v *Validator) validateGenericQuery(query string) error {
	query = strings.TrimSpace(query)

	// Check for balanced parentheses
	if !v.hasBalancedParentheses(query) {
		return fmt.Errorf("unbalanced parentheses")
	}

	// Check for SQL injection patterns (basic check)
	suspiciousPatterns := []string{
		"';",
		"--",
		"/*",
		"*/",
		"xp_",
		"sp_",
	}

	upperQuery := strings.ToUpper(query)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(upperQuery, strings.ToUpper(pattern)) {
			return fmt.Errorf("potentially unsafe pattern detected: %s", pattern)
		}
	}

	return nil
}

// hasBalancedParentheses checks if parentheses are balanced
func (v *Validator) hasBalancedParentheses(query string) bool {
	count := 0
	inString := false
	escape := false

	for _, char := range query {
		if escape {
			escape = false
			continue
		}

		switch char {
		case '\\':
			escape = true
		case '\'', '"':
			inString = !inString
		case '(':
			if !inString {
				count++
			}
		case ')':
			if !inString {
				count--
				if count < 0 {
					return false
				}
			}
		}
	}

	return count == 0
}

// validateDependencies validates migration dependencies
func (v *Validator) validateDependencies(ctx context.Context, migration *Migration, result *ValidationResult) {
	for _, depID := range migration.Dependencies {
		// Check if dependency exists
		dep := GetMigrationByID(depID)
		if dep == nil {
			result.Errors = append(result.Errors, &ValidationError{
				Migration: migration.ID,
				Issue:     fmt.Sprintf("dependency not found: %s", depID),
				Severity:  "error",
			})
			continue
		}

		// Check if dependency is applied (if we're validating for execution)
		// This is a simplified check - in production, you might want more sophisticated dependency tracking
		if depID >= migration.ID {
			result.Warnings = append(result.Warnings, &ValidationError{
				Migration: migration.ID,
				Issue:     fmt.Sprintf("dependency %s has a later ID than current migration", depID),
				Severity:  "warning",
			})
		}
	}
}

// validateRollback validates that rollback is possible
func (v *Validator) validateRollback(migration *Migration, result *ValidationResult) {
	dbType := v.normalizeDBType(v.db.Type())

	upQueries := v.getQueriesForDB(migration.Up, dbType)
	downQueries := v.getQueriesForDB(migration.Down, dbType)

	if len(upQueries) > 0 && len(downQueries) == 0 {
		result.Warnings = append(result.Warnings, &ValidationError{
			Migration: migration.ID,
			Issue:     "no rollback queries defined",
			Severity:  "warning",
		})
		return
	}

	// Analyze if rollback is actually possible
	for _, upQuery := range upQueries {
		upperQuery := strings.ToUpper(strings.TrimSpace(upQuery))

		// Operations that cannot be rolled back
		irreversibleOps := []string{
			"DROP TABLE",
			"DROP COLUMN",
			"DELETE FROM",
			"TRUNCATE",
		}

		for _, op := range irreversibleOps {
			if strings.Contains(upperQuery, op) {
				result.Warnings = append(result.Warnings, &ValidationError{
					Migration: migration.ID,
					Issue:     fmt.Sprintf("operation '%s' may not be fully reversible", op),
					Severity:  "warning",
				})
				break
			}
		}
	}
}

// getQueriesForDB gets queries for a specific database type
func (v *Validator) getQueriesForDB(queries map[string][]string, dbType string) []string {
	if dbQueries, exists := queries[dbType]; exists {
		return dbQueries
	}
	if allQueries, exists := queries["all"]; exists {
		return allQueries
	}
	return []string{}
}

// normalizeDBType normalizes database type names
func (v *Validator) normalizeDBType(dbType string) string {
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