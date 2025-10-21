package migrations

import "github.com/casjaysdevdocker/caslink/internal/db/migrations"

func init() {
	migrations.RegisterMigration(&migrations.Migration{
		ID:          "20240101_000002_add_analytics",
		Description: "Add analytics tables for click tracking and statistics",
		Dependencies: []string{"20240101_000001_initial_schema"},
		Up: map[string][]string{
			"sqlite": {
				// Clicks table - individual click tracking
				`CREATE TABLE clicks (
					id TEXT PRIMARY KEY,
					url_id TEXT NOT NULL,
					clicked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					ip_address TEXT,
					ip_hash TEXT,
					user_agent TEXT,
					parsed_browser TEXT,
					parsed_os TEXT,
					parsed_device TEXT,
					referrer TEXT,
					referrer_domain TEXT,
					country_code TEXT,
					country_name TEXT,
					region TEXT,
					city TEXT,
					latitude REAL,
					longitude REAL,
					timezone TEXT,
					is_bot BOOLEAN DEFAULT FALSE,
					is_unique BOOLEAN DEFAULT TRUE,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_clicks_url_id ON clicks(url_id)`,
				`CREATE INDEX idx_clicks_clicked_at ON clicks(clicked_at DESC)`,
				`CREATE INDEX idx_clicks_country_code ON clicks(country_code) WHERE country_code IS NOT NULL`,
				`CREATE INDEX idx_clicks_is_bot ON clicks(is_bot) WHERE is_bot = FALSE`,
				`CREATE INDEX idx_clicks_ip_hash_url ON clicks(ip_hash, url_id)`,

				// Click daily stats table - aggregated analytics
				`CREATE TABLE click_daily_stats (
					id TEXT PRIMARY KEY,
					url_id TEXT NOT NULL,
					date DATE NOT NULL,
					clicks INTEGER DEFAULT 0,
					unique_clicks INTEGER DEFAULT 0,
					top_countries TEXT,
					top_referrers TEXT,
					top_browsers TEXT,
					top_devices TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE,
					UNIQUE(url_id, date)
				)`,
				`CREATE INDEX idx_click_daily_stats_url_date ON click_daily_stats(url_id, date DESC)`,
			},
			"postgresql": {
				// Clicks table - individual click tracking
				`CREATE TABLE clicks (
					id TEXT PRIMARY KEY,
					url_id TEXT NOT NULL,
					clicked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					ip_address TEXT,
					ip_hash TEXT,
					user_agent TEXT,
					parsed_browser TEXT,
					parsed_os TEXT,
					parsed_device TEXT,
					referrer TEXT,
					referrer_domain TEXT,
					country_code TEXT,
					country_name TEXT,
					region TEXT,
					city TEXT,
					latitude DECIMAL(10,8),
					longitude DECIMAL(11,8),
					timezone TEXT,
					is_bot BOOLEAN DEFAULT FALSE,
					is_unique BOOLEAN DEFAULT TRUE,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_clicks_url_id ON clicks(url_id)`,
				`CREATE INDEX idx_clicks_clicked_at ON clicks(clicked_at DESC)`,
				`CREATE INDEX idx_clicks_country_code ON clicks(country_code) WHERE country_code IS NOT NULL`,
				`CREATE INDEX idx_clicks_is_bot ON clicks(is_bot) WHERE is_bot = FALSE`,
				`CREATE INDEX idx_clicks_ip_hash_url ON clicks(ip_hash, url_id)`,

				// Click daily stats table - aggregated analytics
				`CREATE TABLE click_daily_stats (
					id TEXT PRIMARY KEY,
					url_id TEXT NOT NULL,
					date DATE NOT NULL,
					clicks BIGINT DEFAULT 0,
					unique_clicks BIGINT DEFAULT 0,
					top_countries JSONB,
					top_referrers JSONB,
					top_browsers JSONB,
					top_devices JSONB,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE,
					UNIQUE(url_id, date)
				)`,
				`CREATE INDEX idx_click_daily_stats_url_date ON click_daily_stats(url_id, date DESC)`,
			},
			"mysql": {
				// Clicks table - individual click tracking
				`CREATE TABLE clicks (
					id VARCHAR(255) PRIMARY KEY,
					url_id VARCHAR(255) NOT NULL,
					clicked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					ip_address VARCHAR(45),
					ip_hash VARCHAR(64),
					user_agent TEXT,
					parsed_browser VARCHAR(100),
					parsed_os VARCHAR(100),
					parsed_device VARCHAR(100),
					referrer TEXT,
					referrer_domain VARCHAR(255),
					country_code VARCHAR(2),
					country_name VARCHAR(100),
					region VARCHAR(100),
					city VARCHAR(100),
					latitude DECIMAL(10,8),
					longitude DECIMAL(11,8),
					timezone VARCHAR(50),
					is_bot BOOLEAN DEFAULT FALSE,
					is_unique BOOLEAN DEFAULT TRUE,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_clicks_url_id ON clicks(url_id)`,
				`CREATE INDEX idx_clicks_clicked_at ON clicks(clicked_at DESC)`,
				`CREATE INDEX idx_clicks_country_code ON clicks(country_code)`,
				`CREATE INDEX idx_clicks_is_bot ON clicks(is_bot)`,
				`CREATE INDEX idx_clicks_ip_hash_url ON clicks(ip_hash, url_id)`,

				// Click daily stats table - aggregated analytics
				`CREATE TABLE click_daily_stats (
					id VARCHAR(255) PRIMARY KEY,
					url_id VARCHAR(255) NOT NULL,
					date DATE NOT NULL,
					clicks BIGINT DEFAULT 0,
					unique_clicks BIGINT DEFAULT 0,
					top_countries JSON,
					top_referrers JSON,
					top_browsers JSON,
					top_devices JSON,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (url_id) REFERENCES urls(id) ON DELETE CASCADE,
					UNIQUE KEY unique_url_date (url_id, date)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_click_daily_stats_url_date ON click_daily_stats(url_id, date DESC)`,
			},
		},
		Down: map[string][]string{
			"all": {
				"DROP TABLE IF EXISTS click_daily_stats",
				"DROP TABLE IF EXISTS clicks",
			},
		},
	})
}
