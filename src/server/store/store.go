package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// queryTimeout is the per-statement deadline applied to all schema DDL operations.
const queryTimeout = 30 * time.Second

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
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		_, err := s.ServerDB.ExecContext(ctx, query)
		cancel()
		if err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Idempotent schema updates — safe to run on every startup.
	serverUpdates := []string{
		`CREATE INDEX IF NOT EXISTS idx_urls_short_code ON urls(short_code)`,
		`CREATE INDEX IF NOT EXISTS idx_urls_user_id ON urls(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_urls_org_id ON urls(org_id)`,
		`CREATE INDEX IF NOT EXISTS idx_urls_expires_at ON urls(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_clicks_url_id ON clicks(url_id)`,
		`CREATE INDEX IF NOT EXISTS idx_clicks_clicked_at ON clicks(clicked_at)`,
		`CREATE INDEX IF NOT EXISTS idx_click_daily_stats_url_id ON click_daily_stats(url_id)`,
		`CREATE INDEX IF NOT EXISTS idx_click_daily_stats_date ON click_daily_stats(date)`,
	}
	for _, query := range serverUpdates {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		_, err := s.ServerDB.ExecContext(ctx, query)
		cancel()
		if err != nil {
			return fmt.Errorf("failed to apply schema update: %w", err)
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

		// Sessions — ip_address, user_agent, last_activity required by the
		// active-sessions UI (spec PART 23 security/sessions page).
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			user_type TEXT NOT NULL,
			data TEXT,
			ip_address TEXT,
			user_agent TEXT,
			last_activity DATETIME DEFAULT CURRENT_TIMESTAMP,
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

		// Passkey credentials (WebAuthn v3 per PART 34)
		`CREATE TABLE IF NOT EXISTS passkey_credentials (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			credential_id TEXT NOT NULL UNIQUE,
			public_key BLOB NOT NULL,
			attestation_type TEXT NOT NULL DEFAULT '',
			aaguid TEXT NOT NULL DEFAULT '',
			sign_count INTEGER NOT NULL DEFAULT 0,
			user_verified INTEGER NOT NULL DEFAULT 0,
			backup_eligible INTEGER NOT NULL DEFAULT 0,
			backup_state INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,

		// Recovery keys for 2FA/passkey recovery (hashed, single-use, per PART 34)
		`CREATE TABLE IF NOT EXISTS recovery_keys (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			key_hash TEXT NOT NULL,
			used INTEGER NOT NULL DEFAULT 0,
			used_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
	}

	for _, query := range queries {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		_, err := s.UsersDB.ExecContext(ctx, query)
		cancel()
		if err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Idempotent schema updates — safe to run on every startup.
	usersUpdates := []string{
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_user_id ON audit_log(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_domains_domain ON custom_domains(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_domains_owner ON custom_domains(owner_type, owner_id)`,
		// Org-scoped API tokens (PART 35)
		`CREATE TABLE IF NOT EXISTS org_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id INTEGER NOT NULL,
			created_by INTEGER NOT NULL,
			name TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			permissions TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME,
			last_used_at DATETIME,
			active INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE,
			FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_org_members_org_id ON org_members(org_id)`,
		`CREATE INDEX IF NOT EXISTS idx_org_members_user_id ON org_members(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_org_tokens_org_id ON org_tokens(org_id) WHERE active = 1`,
		`CREATE INDEX IF NOT EXISTS idx_org_tokens_token_hash ON org_tokens(token_hash) WHERE active = 1`,
		`CREATE INDEX IF NOT EXISTS idx_password_resets_token_hash ON password_resets(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_email_verifications_token_hash ON email_verifications(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_passkey_credentials_user_id ON passkey_credentials(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_passkey_credentials_credential_id ON passkey_credentials(credential_id)`,
		`CREATE INDEX IF NOT EXISTS idx_recovery_keys_user_id ON recovery_keys(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_recovery_keys_hash ON recovery_keys(key_hash) WHERE used = 0`,
	}
	for _, query := range usersUpdates {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		_, err := s.UsersDB.ExecContext(ctx, query)
		cancel()
		if err != nil {
			return fmt.Errorf("failed to apply schema update: %w", err)
		}
	}

	return nil
}
