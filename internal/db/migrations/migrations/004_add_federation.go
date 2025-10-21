package migrations

import "github.com/casjaysdevdocker/caslink/internal/db/migrations"

func init() {
	migrations.RegisterMigration(&migrations.Migration{
		ID:          "20240101_000004_add_federation",
		Description: "Add federation tables for distributed URL sharing",
		Dependencies: []string{"20240101_000003_add_billing"},
		Up: map[string][]string{
			"sqlite": {
				// Federation instances table
				`CREATE TABLE federation_instances (
					id TEXT PRIMARY KEY,
					domain TEXT UNIQUE NOT NULL,
					public_key TEXT NOT NULL,
					discovered_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					last_sync DATETIME,
					active BOOLEAN DEFAULT TRUE,
					blocked BOOLEAN DEFAULT FALSE,
					sync_enabled BOOLEAN DEFAULT TRUE
				)`,
				`CREATE INDEX idx_federation_instances_domain ON federation_instances(domain)`,
				`CREATE INDEX idx_federation_instances_active ON federation_instances(active) WHERE active = TRUE`,
				`CREATE INDEX idx_federation_instances_last_sync ON federation_instances(last_sync DESC)`,

				// Federated URLs table
				`CREATE TABLE federated_urls (
					id TEXT PRIMARY KEY,
					original_id TEXT NOT NULL,
					source_instance TEXT NOT NULL,
					original_url TEXT NOT NULL,
					short_code TEXT NOT NULL,
					title TEXT,
					created_at DATETIME NOT NULL,
					synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (source_instance) REFERENCES federation_instances(domain) ON DELETE CASCADE,
					UNIQUE(source_instance, original_id)
				)`,
				`CREATE INDEX idx_federated_urls_source_instance ON federated_urls(source_instance)`,
				`CREATE INDEX idx_federated_urls_short_code ON federated_urls(short_code)`,
				`CREATE INDEX idx_federated_urls_synced_at ON federated_urls(synced_at DESC)`,
			},
			"postgresql": {
				// Federation instances table
				`CREATE TABLE federation_instances (
					id TEXT PRIMARY KEY,
					domain TEXT UNIQUE NOT NULL,
					public_key TEXT NOT NULL,
					discovered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					last_sync TIMESTAMP,
					active BOOLEAN DEFAULT TRUE,
					blocked BOOLEAN DEFAULT FALSE,
					sync_enabled BOOLEAN DEFAULT TRUE
				)`,
				`CREATE INDEX idx_federation_instances_domain ON federation_instances(domain)`,
				`CREATE INDEX idx_federation_instances_active ON federation_instances(active) WHERE active = TRUE`,
				`CREATE INDEX idx_federation_instances_last_sync ON federation_instances(last_sync DESC)`,

				// Federated URLs table
				`CREATE TABLE federated_urls (
					id TEXT PRIMARY KEY,
					original_id TEXT NOT NULL,
					source_instance TEXT NOT NULL,
					original_url TEXT NOT NULL,
					short_code TEXT NOT NULL,
					title TEXT,
					created_at TIMESTAMP NOT NULL,
					synced_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (source_instance) REFERENCES federation_instances(domain) ON DELETE CASCADE,
					UNIQUE(source_instance, original_id)
				)`,
				`CREATE INDEX idx_federated_urls_source_instance ON federated_urls(source_instance)`,
				`CREATE INDEX idx_federated_urls_short_code ON federated_urls(short_code)`,
				`CREATE INDEX idx_federated_urls_synced_at ON federated_urls(synced_at DESC)`,
			},
			"mysql": {
				// Federation instances table
				`CREATE TABLE federation_instances (
					id VARCHAR(255) PRIMARY KEY,
					domain VARCHAR(255) UNIQUE NOT NULL,
					public_key TEXT NOT NULL,
					discovered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					last_sync TIMESTAMP NULL,
					active BOOLEAN DEFAULT TRUE,
					blocked BOOLEAN DEFAULT FALSE,
					sync_enabled BOOLEAN DEFAULT TRUE
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_federation_instances_domain ON federation_instances(domain)`,
				`CREATE INDEX idx_federation_instances_active ON federation_instances(active)`,
				`CREATE INDEX idx_federation_instances_last_sync ON federation_instances(last_sync DESC)`,

				// Federated URLs table
				`CREATE TABLE federated_urls (
					id VARCHAR(255) PRIMARY KEY,
					original_id VARCHAR(255) NOT NULL,
					source_instance VARCHAR(255) NOT NULL,
					original_url TEXT NOT NULL,
					short_code VARCHAR(255) NOT NULL,
					title TEXT,
					created_at TIMESTAMP NOT NULL,
					synced_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (source_instance) REFERENCES federation_instances(domain) ON DELETE CASCADE,
					UNIQUE KEY unique_source_original (source_instance, original_id)
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_federated_urls_source_instance ON federated_urls(source_instance)`,
				`CREATE INDEX idx_federated_urls_short_code ON federated_urls(short_code)`,
				`CREATE INDEX idx_federated_urls_synced_at ON federated_urls(synced_at DESC)`,
			},
		},
		Down: map[string][]string{
			"all": {
				"DROP TABLE IF EXISTS federated_urls",
				"DROP TABLE IF EXISTS federation_instances",
			},
		},
	})
}
