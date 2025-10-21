package migrations

import "github.com/casjaysdevdocker/caslink/internal/db/migrations"

func init() {
	migrations.RegisterMigration(&migrations.Migration{
		ID:          "20240101_000001_initial_schema",
		Description: "Create initial database schema for URLs, users, and core tables",
		Dependencies: []string{},
		Up: map[string][]string{
			"sqlite": {
				// URLs table - core URL shortening functionality
				`CREATE TABLE urls (
					id TEXT PRIMARY KEY,
					original_url TEXT NOT NULL,
					is_custom BOOLEAN DEFAULT FALSE,
					title TEXT,
					description TEXT,
					favicon_url TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					expires_at DATETIME,
					clicks INTEGER DEFAULT 0,
					unique_clicks INTEGER DEFAULT 0,
					user_id TEXT,
					domain_id TEXT,
					active BOOLEAN DEFAULT TRUE,
					password TEXT,
					tags TEXT,
					utm_source TEXT,
					utm_medium TEXT,
					utm_campaign TEXT,
					utm_term TEXT,
					utm_content TEXT
				)`,
				`CREATE INDEX idx_urls_created_at ON urls(created_at DESC)`,
				`CREATE INDEX idx_urls_expires_at ON urls(expires_at) WHERE expires_at IS NOT NULL`,
				`CREATE INDEX idx_urls_user_id ON urls(user_id) WHERE user_id IS NOT NULL`,
				`CREATE INDEX idx_urls_domain_id ON urls(domain_id) WHERE domain_id IS NOT NULL`,
				`CREATE INDEX idx_urls_active ON urls(active) WHERE active = TRUE`,
				`CREATE INDEX idx_urls_tags ON urls(tags) WHERE tags IS NOT NULL AND tags != ''`,

				// Uploads table - file attachments
				`CREATE TABLE uploads (
					id TEXT PRIMARY KEY,
					url_id TEXT NOT NULL,
					filename TEXT NOT NULL,
					mime_type TEXT NOT NULL,
					size INTEGER NOT NULL,
					data BLOB NOT NULL,
					uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_uploads_url_id ON uploads(url_id)`,

				// QR codes table - generated QR codes cache
				`CREATE TABLE qr_codes (
					id TEXT PRIMARY KEY,
					url_id TEXT NOT NULL,
					format TEXT NOT NULL,
					size INTEGER NOT NULL,
					style TEXT NOT NULL,
					foreground_color TEXT,
					background_color TEXT,
					logo_url TEXT,
					data BLOB NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_qr_codes_url_id ON qr_codes(url_id)`,

				// Users table - user management and authentication
				`CREATE TABLE users (
					id TEXT PRIMARY KEY,
					username TEXT UNIQUE NOT NULL,
					email TEXT UNIQUE,
					password_hash TEXT NOT NULL,
					is_admin BOOLEAN DEFAULT FALSE,
					is_premium BOOLEAN DEFAULT FALSE,
					premium_expires_at DATETIME,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					last_login DATETIME,
					last_active DATETIME,
					two_fa_secret TEXT DEFAULT '',
					two_fa_enabled BOOLEAN DEFAULT FALSE,
					webauthn_credentials TEXT DEFAULT '',
					api_rate_limit INTEGER DEFAULT 1000,
					url_limit INTEGER DEFAULT -1,
					timezone TEXT DEFAULT 'UTC',
					language TEXT DEFAULT 'en',
					theme TEXT DEFAULT 'dark'
				)`,
				`CREATE INDEX idx_users_username ON users(username)`,
				`CREATE INDEX idx_users_email ON users(email) WHERE email IS NOT NULL`,
				`CREATE INDEX idx_users_created_at ON users(created_at DESC)`,
				`CREATE INDEX idx_users_last_login ON users(last_login DESC) WHERE last_login IS NOT NULL`,

				// Domains table - custom domain management
				`CREATE TABLE domains (
					id TEXT PRIMARY KEY,
					domain TEXT UNIQUE NOT NULL,
					user_id TEXT NOT NULL,
					is_default BOOLEAN DEFAULT FALSE,
					ssl_enabled BOOLEAN DEFAULT FALSE,
					ssl_cert_path TEXT,
					ssl_key_path TEXT,
					verified BOOLEAN DEFAULT FALSE,
					verification_token TEXT,
					verification_method TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					verified_at DATETIME,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_domains_user_id ON domains(user_id)`,
				`CREATE INDEX idx_domains_verified ON domains(verified) WHERE verified = TRUE`,

				// API tokens table - API authentication
				`CREATE TABLE api_tokens (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					name TEXT NOT NULL,
					token TEXT UNIQUE NOT NULL,
					permissions TEXT NOT NULL,
					rate_limit INTEGER DEFAULT 1000,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					expires_at DATETIME,
					last_used DATETIME,
					last_used_ip TEXT,
					active BOOLEAN DEFAULT TRUE,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_api_tokens_token ON api_tokens(token) WHERE active = TRUE`,
				`CREATE INDEX idx_api_tokens_user_id ON api_tokens(user_id) WHERE active = TRUE`,

				// Sessions table - session management
				`CREATE TABLE sessions (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					data TEXT NOT NULL,
					expires_at DATETIME NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP,
					ip_address TEXT,
					user_agent TEXT,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_sessions_expires_at ON sessions(expires_at)`,
				`CREATE INDEX idx_sessions_user_id ON sessions(user_id)`,

				// Server config table - runtime configuration
				`CREATE TABLE server_config (
					key TEXT PRIMARY KEY,
					value TEXT NOT NULL,
					type TEXT NOT NULL,
					description TEXT,
					updated_by TEXT,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)`,
			},
			"postgresql": {
				// URLs table - core URL shortening functionality
				`CREATE TABLE urls (
					id TEXT PRIMARY KEY,
					original_url TEXT NOT NULL,
					is_custom BOOLEAN DEFAULT FALSE,
					title TEXT,
					description TEXT,
					favicon_url TEXT,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					expires_at TIMESTAMP,
					clicks BIGINT DEFAULT 0,
					unique_clicks BIGINT DEFAULT 0,
					user_id TEXT,
					domain_id TEXT,
					active BOOLEAN DEFAULT TRUE,
					password TEXT,
					tags TEXT,
					utm_source TEXT,
					utm_medium TEXT,
					utm_campaign TEXT,
					utm_term TEXT,
					utm_content TEXT
				)`,
				`CREATE INDEX idx_urls_created_at ON urls(created_at DESC)`,
				`CREATE INDEX idx_urls_expires_at ON urls(expires_at) WHERE expires_at IS NOT NULL`,
				`CREATE INDEX idx_urls_user_id ON urls(user_id) WHERE user_id IS NOT NULL`,
				`CREATE INDEX idx_urls_domain_id ON urls(domain_id) WHERE domain_id IS NOT NULL`,
				`CREATE INDEX idx_urls_active ON urls(active) WHERE active = TRUE`,
				`CREATE INDEX idx_urls_tags ON urls(tags) WHERE tags IS NOT NULL AND tags != ''`,

				// Uploads table - file attachments
				`CREATE TABLE uploads (
					id TEXT PRIMARY KEY,
					url_id TEXT NOT NULL,
					filename TEXT NOT NULL,
					mime_type TEXT NOT NULL,
					size BIGINT NOT NULL,
					data BYTEA NOT NULL,
					uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_uploads_url_id ON uploads(url_id)`,

				// QR codes table - generated QR codes cache
				`CREATE TABLE qr_codes (
					id TEXT PRIMARY KEY,
					url_id TEXT NOT NULL,
					format TEXT NOT NULL,
					size INTEGER NOT NULL,
					style TEXT NOT NULL,
					foreground_color TEXT,
					background_color TEXT,
					logo_url TEXT,
					data BYTEA NOT NULL,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_qr_codes_url_id ON qr_codes(url_id)`,

				// Users table - user management and authentication
				`CREATE TABLE users (
					id TEXT PRIMARY KEY,
					username TEXT UNIQUE NOT NULL,
					email TEXT UNIQUE,
					password_hash TEXT NOT NULL,
					is_admin BOOLEAN DEFAULT FALSE,
					is_premium BOOLEAN DEFAULT FALSE,
					premium_expires_at TIMESTAMP,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					last_login TIMESTAMP,
					last_active TIMESTAMP,
					two_fa_secret TEXT DEFAULT '',
					two_fa_enabled BOOLEAN DEFAULT FALSE,
					webauthn_credentials TEXT DEFAULT '',
					api_rate_limit INTEGER DEFAULT 1000,
					url_limit INTEGER DEFAULT -1,
					timezone TEXT DEFAULT 'UTC',
					language TEXT DEFAULT 'en',
					theme TEXT DEFAULT 'dark'
				)`,
				`CREATE INDEX idx_users_username ON users(username)`,
				`CREATE INDEX idx_users_email ON users(email) WHERE email IS NOT NULL`,
				`CREATE INDEX idx_users_created_at ON users(created_at DESC)`,
				`CREATE INDEX idx_users_last_login ON users(last_login DESC) WHERE last_login IS NOT NULL`,

				// Domains table - custom domain management
				`CREATE TABLE domains (
					id TEXT PRIMARY KEY,
					domain TEXT UNIQUE NOT NULL,
					user_id TEXT NOT NULL,
					is_default BOOLEAN DEFAULT FALSE,
					ssl_enabled BOOLEAN DEFAULT FALSE,
					ssl_cert_path TEXT,
					ssl_key_path TEXT,
					verified BOOLEAN DEFAULT FALSE,
					verification_token TEXT,
					verification_method TEXT,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					verified_at TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_domains_user_id ON domains(user_id)`,
				`CREATE INDEX idx_domains_verified ON domains(verified) WHERE verified = TRUE`,

				// API tokens table - API authentication
				`CREATE TABLE api_tokens (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					name TEXT NOT NULL,
					token TEXT UNIQUE NOT NULL,
					permissions JSONB NOT NULL,
					rate_limit INTEGER DEFAULT 1000,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					expires_at TIMESTAMP,
					last_used TIMESTAMP,
					last_used_ip TEXT,
					active BOOLEAN DEFAULT TRUE,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_api_tokens_token ON api_tokens(token) WHERE active = TRUE`,
				`CREATE INDEX idx_api_tokens_user_id ON api_tokens(user_id) WHERE active = TRUE`,

				// Sessions table - session management
				`CREATE TABLE sessions (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					data TEXT NOT NULL,
					expires_at TIMESTAMP NOT NULL,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					last_accessed TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					ip_address TEXT,
					user_agent TEXT,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_sessions_expires_at ON sessions(expires_at)`,
				`CREATE INDEX idx_sessions_user_id ON sessions(user_id)`,

				// Server config table - runtime configuration
				`CREATE TABLE server_config (
					key TEXT PRIMARY KEY,
					value TEXT NOT NULL,
					type TEXT NOT NULL,
					description TEXT,
					updated_by TEXT,
					updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
				)`,
			},
			"mysql": {
				// URLs table - core URL shortening functionality
				`CREATE TABLE urls (
					id VARCHAR(255) PRIMARY KEY,
					original_url TEXT NOT NULL,
					is_custom BOOLEAN DEFAULT FALSE,
					title VARCHAR(500),
					description TEXT,
					favicon_url TEXT,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
					expires_at TIMESTAMP NULL,
					clicks BIGINT DEFAULT 0,
					unique_clicks BIGINT DEFAULT 0,
					user_id VARCHAR(255),
					domain_id VARCHAR(255),
					active BOOLEAN DEFAULT TRUE,
					password VARCHAR(255),
					tags TEXT,
					utm_source VARCHAR(255),
					utm_medium VARCHAR(255),
					utm_campaign VARCHAR(255),
					utm_term VARCHAR(255),
					utm_content VARCHAR(255)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_urls_created_at ON urls(created_at DESC)`,
				`CREATE INDEX idx_urls_expires_at ON urls(expires_at)`,
				`CREATE INDEX idx_urls_user_id ON urls(user_id)`,
				`CREATE INDEX idx_urls_domain_id ON urls(domain_id)`,
				`CREATE INDEX idx_urls_active ON urls(active)`,

				// Uploads table - file attachments
				`CREATE TABLE uploads (
					id VARCHAR(255) PRIMARY KEY,
					url_id VARCHAR(255) NOT NULL,
					filename VARCHAR(500) NOT NULL,
					mime_type VARCHAR(100) NOT NULL,
					size BIGINT NOT NULL,
					data LONGBLOB NOT NULL,
					uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_uploads_url_id ON uploads(url_id)`,

				// QR codes table - generated QR codes cache
				`CREATE TABLE qr_codes (
					id VARCHAR(255) PRIMARY KEY,
					url_id VARCHAR(255) NOT NULL,
					format VARCHAR(10) NOT NULL,
					size INT NOT NULL,
					style VARCHAR(20) NOT NULL,
					foreground_color VARCHAR(7),
					background_color VARCHAR(7),
					logo_url TEXT,
					data LONGBLOB NOT NULL,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_qr_codes_url_id ON qr_codes(url_id)`,

				// Users table - user management and authentication
				`CREATE TABLE users (
					id VARCHAR(255) PRIMARY KEY,
					username VARCHAR(255) UNIQUE NOT NULL,
					email VARCHAR(255) UNIQUE,
					password_hash VARCHAR(255) NOT NULL,
					is_admin BOOLEAN DEFAULT FALSE,
					is_premium BOOLEAN DEFAULT FALSE,
					premium_expires_at TIMESTAMP NULL,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					last_login TIMESTAMP NULL,
					last_active TIMESTAMP NULL,
					two_fa_secret VARCHAR(255) DEFAULT '',
					two_fa_enabled BOOLEAN DEFAULT FALSE,
					webauthn_credentials TEXT DEFAULT '',
					api_rate_limit INT DEFAULT 1000,
					url_limit INT DEFAULT -1,
					timezone VARCHAR(50) DEFAULT 'UTC',
					language VARCHAR(10) DEFAULT 'en',
					theme VARCHAR(20) DEFAULT 'dark'
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_users_username ON users(username)`,
				`CREATE INDEX idx_users_email ON users(email)`,
				`CREATE INDEX idx_users_created_at ON users(created_at DESC)`,
				`CREATE INDEX idx_users_last_login ON users(last_login DESC)`,

				// Domains table - custom domain management
				`CREATE TABLE domains (
					id VARCHAR(255) PRIMARY KEY,
					domain VARCHAR(255) UNIQUE NOT NULL,
					user_id VARCHAR(255) NOT NULL,
					is_default BOOLEAN DEFAULT FALSE,
					ssl_enabled BOOLEAN DEFAULT FALSE,
					ssl_cert_path TEXT,
					ssl_key_path TEXT,
					verified BOOLEAN DEFAULT FALSE,
					verification_token VARCHAR(255),
					verification_method VARCHAR(50),
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					verified_at TIMESTAMP NULL,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_domains_user_id ON domains(user_id)`,
				`CREATE INDEX idx_domains_verified ON domains(verified)`,

				// API tokens table - API authentication
				`CREATE TABLE api_tokens (
					id VARCHAR(255) PRIMARY KEY,
					user_id VARCHAR(255) NOT NULL,
					name VARCHAR(255) NOT NULL,
					token VARCHAR(255) UNIQUE NOT NULL,
					permissions JSON NOT NULL,
					rate_limit INT DEFAULT 1000,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					expires_at TIMESTAMP NULL,
					last_used TIMESTAMP NULL,
					last_used_ip VARCHAR(45),
					active BOOLEAN DEFAULT TRUE,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_api_tokens_token ON api_tokens(token, active)`,
				`CREATE INDEX idx_api_tokens_user_id ON api_tokens(user_id, active)`,

				// Sessions table - session management
				`CREATE TABLE sessions (
					id VARCHAR(255) PRIMARY KEY,
					user_id VARCHAR(255) NOT NULL,
					data TEXT NOT NULL,
					expires_at TIMESTAMP NOT NULL,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					last_accessed TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					ip_address VARCHAR(45),
					user_agent TEXT,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_sessions_expires_at ON sessions(expires_at)`,
				`CREATE INDEX idx_sessions_user_id ON sessions(user_id)`,

				// Server config table - runtime configuration
				"CREATE TABLE server_config (" +
					"`key` VARCHAR(255) PRIMARY KEY," +
					"`value` TEXT NOT NULL," +
					"`type` VARCHAR(50) NOT NULL," +
					"description TEXT," +
					"updated_by VARCHAR(255)," +
					"updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP" +
				") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
			},
		},
		Down: map[string][]string{
			"all": {
				"DROP TABLE IF EXISTS server_config",
				"DROP TABLE IF EXISTS sessions",
				"DROP TABLE IF EXISTS api_tokens",
				"DROP TABLE IF EXISTS domains",
				"DROP TABLE IF EXISTS users",
				"DROP TABLE IF EXISTS qr_codes",
				"DROP TABLE IF EXISTS uploads",
				"DROP TABLE IF EXISTS urls",
			},
		},
	})
}