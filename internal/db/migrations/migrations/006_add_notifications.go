package migrations

import "github.com/casjaysdevdocker/caslink/internal/db/migrations"

func init() {
	migrations.RegisterMigration(&migrations.Migration{
		ID:          "20240101_000006_add_notifications",
		Description: "Add notification system tables for email, SMS, and webhook notifications",
		Dependencies: []string{"20240101_000005_add_audit_logs"},
		Up: map[string][]string{
			"sqlite": {
				// Notifications table
				`CREATE TABLE notifications (
					id TEXT PRIMARY KEY,
					user_id TEXT,
					type TEXT NOT NULL,
					channel TEXT NOT NULL,
					subject TEXT NOT NULL,
					content TEXT NOT NULL,
					data TEXT DEFAULT '{}',
					status TEXT NOT NULL DEFAULT 'pending',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					sent_at DATETIME,
					read_at DATETIME,
					expires_at DATETIME,
					priority INTEGER NOT NULL DEFAULT 0,
					retries INTEGER NOT NULL DEFAULT 0,
					max_retries INTEGER NOT NULL DEFAULT 3,
					last_error TEXT
				)`,
				`CREATE INDEX idx_notifications_user_id ON notifications(user_id)`,
				`CREATE INDEX idx_notifications_status ON notifications(status)`,
				`CREATE INDEX idx_notifications_created_at ON notifications(created_at DESC)`,
				`CREATE INDEX idx_notifications_expires_at ON notifications(expires_at) WHERE expires_at IS NOT NULL`,

				// Notification preferences table
				`CREATE TABLE notification_preferences (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL UNIQUE,
					email_address TEXT NOT NULL,
					phone_number TEXT,
					push_token TEXT,
					webhook_url TEXT,
					enable_email BOOLEAN NOT NULL DEFAULT TRUE,
					enable_sms BOOLEAN NOT NULL DEFAULT FALSE,
					enable_push BOOLEAN NOT NULL DEFAULT FALSE,
					enable_webhook BOOLEAN NOT NULL DEFAULT FALSE,
					notification_types TEXT DEFAULT '[]',
					quiet_hours_start TEXT,
					quiet_hours_end TEXT,
					timezone TEXT NOT NULL DEFAULT 'UTC',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX idx_notification_preferences_user_id ON notification_preferences(user_id)`,

				// Notification templates table
				`CREATE TABLE notification_templates (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL UNIQUE,
					type TEXT NOT NULL,
					channel TEXT NOT NULL,
					subject TEXT NOT NULL,
					body_text TEXT NOT NULL,
					body_html TEXT,
					variables TEXT DEFAULT '[]',
					metadata TEXT DEFAULT '{}',
					active BOOLEAN NOT NULL DEFAULT TRUE,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX idx_notification_templates_type ON notification_templates(type)`,
				`CREATE INDEX idx_notification_templates_channel ON notification_templates(channel)`,
				`CREATE INDEX idx_notification_templates_active ON notification_templates(active) WHERE active = TRUE`,

				// Notification logs table
				`CREATE TABLE notification_logs (
					id TEXT PRIMARY KEY,
					notification_id TEXT NOT NULL,
					channel TEXT NOT NULL,
					provider TEXT NOT NULL,
					status TEXT NOT NULL,
					delivery_id TEXT,
					response TEXT,
					error_message TEXT,
					attempt_number INTEGER NOT NULL DEFAULT 1,
					delivered_at DATETIME,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (notification_id) REFERENCES notifications(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_notification_logs_notification_id ON notification_logs(notification_id)`,
				`CREATE INDEX idx_notification_logs_status ON notification_logs(status)`,
				`CREATE INDEX idx_notification_logs_created_at ON notification_logs(created_at DESC)`,
			},
			"postgresql": {
				// Notifications table
				`CREATE TABLE notifications (
					id TEXT PRIMARY KEY,
					user_id TEXT,
					type TEXT NOT NULL,
					channel TEXT NOT NULL,
					subject TEXT NOT NULL,
					content TEXT NOT NULL,
					data JSONB DEFAULT '{}',
					status TEXT NOT NULL DEFAULT 'pending',
					created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
					sent_at TIMESTAMP WITH TIME ZONE,
					read_at TIMESTAMP WITH TIME ZONE,
					expires_at TIMESTAMP WITH TIME ZONE,
					priority INTEGER NOT NULL DEFAULT 0,
					retries INTEGER NOT NULL DEFAULT 0,
					max_retries INTEGER NOT NULL DEFAULT 3,
					last_error TEXT
				)`,
				`CREATE INDEX idx_notifications_user_id ON notifications(user_id)`,
				`CREATE INDEX idx_notifications_status ON notifications(status)`,
				`CREATE INDEX idx_notifications_created_at ON notifications(created_at DESC)`,
				`CREATE INDEX idx_notifications_expires_at ON notifications(expires_at) WHERE expires_at IS NOT NULL`,

				// Notification preferences table
				`CREATE TABLE notification_preferences (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL UNIQUE,
					email_address TEXT NOT NULL,
					phone_number TEXT,
					push_token TEXT,
					webhook_url TEXT,
					enable_email BOOLEAN NOT NULL DEFAULT TRUE,
					enable_sms BOOLEAN NOT NULL DEFAULT FALSE,
					enable_push BOOLEAN NOT NULL DEFAULT FALSE,
					enable_webhook BOOLEAN NOT NULL DEFAULT FALSE,
					notification_types JSONB DEFAULT '[]',
					quiet_hours_start TEXT,
					quiet_hours_end TEXT,
					timezone TEXT NOT NULL DEFAULT 'UTC',
					created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
				)`,
				`CREATE INDEX idx_notification_preferences_user_id ON notification_preferences(user_id)`,

				// Notification templates table
				`CREATE TABLE notification_templates (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL UNIQUE,
					type TEXT NOT NULL,
					channel TEXT NOT NULL,
					subject TEXT NOT NULL,
					body_text TEXT NOT NULL,
					body_html TEXT,
					variables JSONB DEFAULT '[]',
					metadata JSONB DEFAULT '{}',
					active BOOLEAN NOT NULL DEFAULT TRUE,
					created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
				)`,
				`CREATE INDEX idx_notification_templates_type ON notification_templates(type)`,
				`CREATE INDEX idx_notification_templates_channel ON notification_templates(channel)`,
				`CREATE INDEX idx_notification_templates_active ON notification_templates(active) WHERE active = TRUE`,

				// Notification logs table
				`CREATE TABLE notification_logs (
					id TEXT PRIMARY KEY,
					notification_id TEXT NOT NULL,
					channel TEXT NOT NULL,
					provider TEXT NOT NULL,
					status TEXT NOT NULL,
					delivery_id TEXT,
					response TEXT,
					error_message TEXT,
					attempt_number INTEGER NOT NULL DEFAULT 1,
					delivered_at TIMESTAMP WITH TIME ZONE,
					created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
					FOREIGN KEY (notification_id) REFERENCES notifications(id) ON DELETE CASCADE
				)`,
				`CREATE INDEX idx_notification_logs_notification_id ON notification_logs(notification_id)`,
				`CREATE INDEX idx_notification_logs_status ON notification_logs(status)`,
				`CREATE INDEX idx_notification_logs_created_at ON notification_logs(created_at DESC)`,
			},
			"mysql": {
				// Notifications table
				`CREATE TABLE notifications (
					id VARCHAR(255) PRIMARY KEY,
					user_id VARCHAR(255),
					type VARCHAR(255) NOT NULL,
					channel VARCHAR(255) NOT NULL,
					subject TEXT NOT NULL,
					content TEXT NOT NULL,
					data JSON DEFAULT ('{}'),
					status VARCHAR(50) NOT NULL DEFAULT 'pending',
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					sent_at TIMESTAMP NULL,
					read_at TIMESTAMP NULL,
					expires_at TIMESTAMP NULL,
					priority INT NOT NULL DEFAULT 0,
					retries INT NOT NULL DEFAULT 0,
					max_retries INT NOT NULL DEFAULT 3,
					last_error TEXT
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_notifications_user_id ON notifications(user_id)`,
				`CREATE INDEX idx_notifications_status ON notifications(status)`,
				`CREATE INDEX idx_notifications_created_at ON notifications(created_at DESC)`,
				`CREATE INDEX idx_notifications_expires_at ON notifications(expires_at)`,

				// Notification preferences table
				`CREATE TABLE notification_preferences (
					id VARCHAR(255) PRIMARY KEY,
					user_id VARCHAR(255) NOT NULL UNIQUE,
					email_address VARCHAR(255) NOT NULL,
					phone_number VARCHAR(50),
					push_token TEXT,
					webhook_url TEXT,
					enable_email BOOLEAN NOT NULL DEFAULT TRUE,
					enable_sms BOOLEAN NOT NULL DEFAULT FALSE,
					enable_push BOOLEAN NOT NULL DEFAULT FALSE,
					enable_webhook BOOLEAN NOT NULL DEFAULT FALSE,
					notification_types JSON DEFAULT ('[]'),
					quiet_hours_start VARCHAR(10),
					quiet_hours_end VARCHAR(10),
					timezone VARCHAR(50) NOT NULL DEFAULT 'UTC',
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_notification_preferences_user_id ON notification_preferences(user_id)`,

				// Notification templates table
				`CREATE TABLE notification_templates (
					id VARCHAR(255) PRIMARY KEY,
					name VARCHAR(255) NOT NULL UNIQUE,
					type VARCHAR(255) NOT NULL,
					channel VARCHAR(255) NOT NULL,
					subject TEXT NOT NULL,
					body_text TEXT NOT NULL,
					body_html TEXT,
					variables JSON DEFAULT ('[]'),
					metadata JSON DEFAULT ('{}'),
					active BOOLEAN NOT NULL DEFAULT TRUE,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_notification_templates_type ON notification_templates(type)`,
				`CREATE INDEX idx_notification_templates_channel ON notification_templates(channel)`,
				`CREATE INDEX idx_notification_templates_active ON notification_templates(active)`,

				// Notification logs table
				`CREATE TABLE notification_logs (
					id VARCHAR(255) PRIMARY KEY,
					notification_id VARCHAR(255) NOT NULL,
					channel VARCHAR(255) NOT NULL,
					provider VARCHAR(255) NOT NULL,
					status VARCHAR(50) NOT NULL,
					delivery_id VARCHAR(255),
					response TEXT,
					error_message TEXT,
					attempt_number INT NOT NULL DEFAULT 1,
					delivered_at TIMESTAMP NULL,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (notification_id) REFERENCES notifications(id) ON DELETE CASCADE
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_notification_logs_notification_id ON notification_logs(notification_id)`,
				`CREATE INDEX idx_notification_logs_status ON notification_logs(status)`,
				`CREATE INDEX idx_notification_logs_created_at ON notification_logs(created_at DESC)`,
			},
		},
		Down: map[string][]string{
			"all": {
				"DROP TABLE IF EXISTS notification_logs",
				"DROP TABLE IF EXISTS notification_templates",
				"DROP TABLE IF EXISTS notification_preferences",
				"DROP TABLE IF EXISTS notifications",
			},
		},
	})
}
