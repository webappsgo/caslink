package store

import (
	"database/sql"
	"fmt"
)

// Store represents the dual database system
type Store struct {
	ServerDB *sql.DB // URLs, analytics, clicks, QR codes, uploads
	UsersDB  *sql.DB // Users, sessions, tokens, domains, config, audit logs
	driver   string  // normalised driver name: sqlite, postgres, mysql, sqlserver
}

// Open opens both databases using SQLite (default, backward-compatible entry point).
// Use OpenStoreWithConfig for other database drivers.
func Open(dataDir string) (*Store, error) {
	return OpenStoreWithConfig("sqlite", "", 0, "", "", "", "", dataDir)
}

// NewTestStore creates a Store from a pre-opened *sql.DB, using it for both
// ServerDB and UsersDB. Intended for unit tests only; do not use in production.
func NewTestStore(db *sql.DB) *Store {
	return &Store{
		ServerDB: db,
		UsersDB:  db,
		driver:   "sqlite",
	}
}

// Close closes both databases
func (s *Store) Close() error {
	var err1, err2 error

	if s.ServerDB != nil {
		err1 = s.ServerDB.Close()
	}

	if s.UsersDB != nil {
		err2 = s.UsersDB.Close()
	}

	if err1 != nil {
		return err1
	}
	return err2
}

// InitSchema initializes database schemas using CREATE TABLE IF NOT EXISTS
func (s *Store) InitSchema() error {
	// Initialize server.db schema
	if err := s.initServerSchema(); err != nil {
		return fmt.Errorf("server.db schema init failed: %w", err)
	}

	// Initialize users.db schema
	if err := s.initUsersSchema(); err != nil {
		return fmt.Errorf("users.db schema init failed: %w", err)
	}

	return nil
}

// initServerSchema initializes server.db tables
func (s *Store) initServerSchema() error {
	queries := []string{
		// URLs table
		`CREATE TABLE IF NOT EXISTS urls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			short_code TEXT NOT NULL UNIQUE,
			long_url TEXT NOT NULL,
			title TEXT,
			description TEXT,
			user_id INTEGER,
			org_id INTEGER,
			custom_code BOOLEAN DEFAULT 0,
			password_hash TEXT,
			expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
			FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE
		)`,

		// Clicks table (analytics)
		`CREATE TABLE IF NOT EXISTS clicks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url_id INTEGER NOT NULL,
			ip_hash TEXT,
			country TEXT,
			city TEXT,
			user_agent TEXT,
			referrer TEXT,
			browser TEXT,
			os TEXT,
			device TEXT,
			is_bot BOOLEAN DEFAULT 0,
			clicked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
		)`,

		// Daily stats aggregation
		`CREATE TABLE IF NOT EXISTS click_daily_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url_id INTEGER NOT NULL,
			date DATE NOT NULL,
			clicks INTEGER DEFAULT 0,
			unique_ips INTEGER DEFAULT 0,
			UNIQUE(url_id, date),
			FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
		)`,

		// QR codes cache
		`CREATE TABLE IF NOT EXISTS qr_codes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url_id INTEGER NOT NULL,
			format TEXT NOT NULL,
			size INTEGER NOT NULL,
			style TEXT,
			data BLOB NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
		)`,

		// Uploads (logos, favicons)
		`CREATE TABLE IF NOT EXISTS uploads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			filename TEXT NOT NULL,
			content_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			data BLOB NOT NULL,
			uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, query := range queries {
		if _, err := s.ServerDB.Exec(query); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}

// initUsersSchema initializes users.db tables
func (s *Store) initUsersSchema() error {
	queries := []string{
		// Admin accounts
		`CREATE TABLE IF NOT EXISTS admins (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			is_primary BOOLEAN DEFAULT 0,
			totp_secret TEXT,
			totp_enabled BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_login DATETIME
		)`,

		// Regular users (if multi-user mode enabled)
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			email_verified BOOLEAN DEFAULT 0,
			totp_secret TEXT,
			totp_enabled BOOLEAN DEFAULT 0,
			suspended BOOLEAN DEFAULT 0,
			suspend_reason TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_login DATETIME
		)`,

		// Sessions
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			user_type TEXT NOT NULL,
			data TEXT,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// API tokens
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			user_type TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			name TEXT,
			permissions TEXT,
			last_used DATETIME,
			expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Server configuration (key-value store)
		`CREATE TABLE IF NOT EXISTS server_config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_by TEXT
		)`,

		// Audit log
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			user_type TEXT,
			action TEXT NOT NULL,
			resource TEXT,
			details TEXT,
			ip_address TEXT,
			user_agent TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Organizations
		`CREATE TABLE IF NOT EXISTS organizations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			owner_id INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE
		)`,

		// Organization members
		`CREATE TABLE IF NOT EXISTS org_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			role TEXT NOT NULL DEFAULT 'member',
			joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(org_id, user_id),
			FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,

		// Custom domains (updated per PART 35 spec)
		`CREATE TABLE IF NOT EXISTS custom_domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_type TEXT NOT NULL,
			owner_id INTEGER NOT NULL,
			domain TEXT NOT NULL UNIQUE,
			is_apex BOOLEAN DEFAULT 0,
			is_wildcard BOOLEAN DEFAULT 0,
			verification_status TEXT NOT NULL DEFAULT 'pending',
			verified_at DATETIME,
			verified_ip TEXT,
			last_check_at DATETIME,
			check_count INTEGER DEFAULT 0,
			ssl_enabled BOOLEAN DEFAULT 0,
			ssl_status TEXT NOT NULL DEFAULT 'none',
			ssl_challenge TEXT,
			ssl_provider TEXT,
			ssl_credentials TEXT,
			ssl_cert_pem TEXT,
			ssl_key_pem TEXT,
			ssl_issued_at DATETIME,
			ssl_expires_at DATETIME,
			ssl_last_error TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			suspended_reason TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Custom domain audit log
		`CREATE TABLE IF NOT EXISTS custom_domain_audit (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain_id INTEGER NOT NULL,
			action TEXT NOT NULL,
			actor_type TEXT NOT NULL,
			actor_id INTEGER,
			details TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (domain_id) REFERENCES custom_domains(id) ON DELETE CASCADE
		)`,

		// Password reset tokens per PART 23
		`CREATE TABLE IF NOT EXISTS password_resets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			token_hash TEXT NOT NULL UNIQUE,
			user_type TEXT NOT NULL,
			user_id INTEGER NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			expires_at INTEGER NOT NULL,
			used_at INTEGER
		)`,

		// Email verification tokens per PART 23
		`CREATE TABLE IF NOT EXISTS email_verifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			token_hash TEXT NOT NULL UNIQUE,
			user_type TEXT NOT NULL,
			user_id INTEGER NOT NULL,
			email TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			expires_at INTEGER NOT NULL,
			verified_at INTEGER
		)`,

		// TOTP secrets per PART 23
		`CREATE TABLE IF NOT EXISTS totp_secrets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_type TEXT NOT NULL,
			user_id INTEGER NOT NULL,
			secret TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			backup_codes TEXT,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			last_used INTEGER,
			UNIQUE(user_type, user_id)
		)`,
	}

	for _, query := range queries {
		if _, err := s.UsersDB.Exec(query); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}
