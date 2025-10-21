package migrations

import "github.com/casjaysdevdocker/caslink/internal/db/migrations"

func init() {
	migrations.RegisterMigration(&migrations.Migration{
		ID:          "20240101_000003_add_billing",
		Description: "Add billing system tables for subscriptions and usage tracking",
		Dependencies: []string{"20240101_000002_add_analytics"},
		Up: map[string][]string{
			"sqlite": {
				// Billing plans table
				`CREATE TABLE billing_plans (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					display_name TEXT NOT NULL,
					description TEXT,
					price_monthly INTEGER NOT NULL,
					price_yearly INTEGER NOT NULL,
					currency TEXT DEFAULT 'USD',
					features TEXT NOT NULL,
					limits TEXT NOT NULL,
					trial_days INTEGER DEFAULT 0,
					active BOOLEAN DEFAULT TRUE,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				)`,

				// Subscriptions table
				`CREATE TABLE subscriptions (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					plan_id TEXT NOT NULL,
					provider_subscription_id TEXT,
					status TEXT NOT NULL,
					billing_cycle TEXT NOT NULL,
					current_period_start DATETIME NOT NULL,
					current_period_end DATETIME NOT NULL,
					trial_start DATETIME,
					trial_end DATETIME,
					cancel_at_period_end BOOLEAN DEFAULT FALSE,
					canceled_at DATETIME,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					FOREIGN KEY (plan_id) REFERENCES billing_plans(id) ON DELETE RESTRICT
				)`,
				`CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id)`,
				`CREATE INDEX idx_subscriptions_status ON subscriptions(status)`,
				`CREATE INDEX idx_subscriptions_current_period_end ON subscriptions(current_period_end)`,

				// Usage records table
				`CREATE TABLE usage_records (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					subscription_id TEXT NOT NULL,
					metric_name TEXT NOT NULL,
					quantity INTEGER NOT NULL,
					timestamp DATETIME NOT NULL,
					billing_period TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_usage_records_user_id ON usage_records(user_id)`,
				`CREATE INDEX idx_usage_records_subscription_id ON usage_records(subscription_id)`,
				`CREATE INDEX idx_usage_records_billing_period ON usage_records(billing_period)`,
			},
			"postgresql": {
				// Billing plans table
				`CREATE TABLE billing_plans (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					display_name TEXT NOT NULL,
					description TEXT,
					price_monthly INTEGER NOT NULL,
					price_yearly INTEGER NOT NULL,
					currency TEXT DEFAULT 'USD',
					features JSONB NOT NULL,
					limits JSONB NOT NULL,
					trial_days INTEGER DEFAULT 0,
					active BOOLEAN DEFAULT TRUE,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
				)`,

				// Subscriptions table
				`CREATE TABLE subscriptions (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					plan_id TEXT NOT NULL,
					provider_subscription_id TEXT,
					status TEXT NOT NULL,
					billing_cycle TEXT NOT NULL,
					current_period_start TIMESTAMP NOT NULL,
					current_period_end TIMESTAMP NOT NULL,
					trial_start TIMESTAMP,
					trial_end TIMESTAMP,
					cancel_at_period_end BOOLEAN DEFAULT FALSE,
					canceled_at TIMESTAMP,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					FOREIGN KEY (plan_id) REFERENCES billing_plans(id) ON DELETE RESTRICT
				)`,
				`CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id)`,
				`CREATE INDEX idx_subscriptions_status ON subscriptions(status)`,
				`CREATE INDEX idx_subscriptions_current_period_end ON subscriptions(current_period_end)`,

				// Usage records table
				`CREATE TABLE usage_records (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL,
					subscription_id TEXT NOT NULL,
					metric_name TEXT NOT NULL,
					quantity INTEGER NOT NULL,
					timestamp TIMESTAMP NOT NULL,
					billing_period TEXT NOT NULL,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_usage_records_user_id ON usage_records(user_id)`,
				`CREATE INDEX idx_usage_records_subscription_id ON usage_records(subscription_id)`,
				`CREATE INDEX idx_usage_records_billing_period ON usage_records(billing_period)`,
			},
			"mysql": {
				// Billing plans table
				`CREATE TABLE billing_plans (
					id VARCHAR(255) PRIMARY KEY,
					name VARCHAR(255) NOT NULL,
					display_name VARCHAR(255) NOT NULL,
					description TEXT,
					price_monthly INT NOT NULL,
					price_yearly INT NOT NULL,
					currency VARCHAR(10) DEFAULT 'USD',
					features JSON NOT NULL,
					limits JSON NOT NULL,
					trial_days INT DEFAULT 0,
					active BOOLEAN DEFAULT TRUE,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

				// Subscriptions table
				`CREATE TABLE subscriptions (
					id VARCHAR(255) PRIMARY KEY,
					user_id VARCHAR(255) NOT NULL,
					plan_id VARCHAR(255) NOT NULL,
					provider_subscription_id VARCHAR(255),
					status VARCHAR(50) NOT NULL,
					billing_cycle VARCHAR(20) NOT NULL,
					current_period_start TIMESTAMP NOT NULL,
					current_period_end TIMESTAMP NOT NULL,
					trial_start TIMESTAMP NULL,
					trial_end TIMESTAMP NULL,
					cancel_at_period_end BOOLEAN DEFAULT FALSE,
					canceled_at TIMESTAMP NULL,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					FOREIGN KEY (plan_id) REFERENCES billing_plans(id) ON DELETE RESTRICT
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id)`,
				`CREATE INDEX idx_subscriptions_status ON subscriptions(status)`,
				`CREATE INDEX idx_subscriptions_current_period_end ON subscriptions(current_period_end)`,

				// Usage records table
				`CREATE TABLE usage_records (
					id VARCHAR(255) PRIMARY KEY,
					user_id VARCHAR(255) NOT NULL,
					subscription_id VARCHAR(255) NOT NULL,
					metric_name VARCHAR(255) NOT NULL,
					quantity INT NOT NULL,
					timestamp TIMESTAMP NOT NULL,
					billing_period VARCHAR(20) NOT NULL,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
					FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_usage_records_user_id ON usage_records(user_id)`,
				`CREATE INDEX idx_usage_records_subscription_id ON usage_records(subscription_id)`,
				`CREATE INDEX idx_usage_records_billing_period ON usage_records(billing_period)`,
			},
		},
		Down: map[string][]string{
			"all": {
				"DROP TABLE IF EXISTS usage_records",
				"DROP TABLE IF EXISTS subscriptions",
				"DROP TABLE IF EXISTS billing_plans",
			},
		},
	})
}
