package migrations

import "github.com/casjaysdevdocker/caslink/internal/db/migrations"

func init() {
	migrations.RegisterMigration(&migrations.Migration{
		ID:          "20240101_000005_add_audit_logs",
		Description: "Add audit logging table for comprehensive activity tracking",
		Dependencies: []string{"20240101_000004_add_federation"},
		Up: map[string][]string{
			"sqlite": {
				// Audit logs table - comprehensive audit trail
				`CREATE TABLE audit_logs (
					id TEXT PRIMARY KEY,
					user_id TEXT,
					action TEXT NOT NULL,
					resource_type TEXT NOT NULL,
					resource_id TEXT,
					old_values TEXT,
					new_values TEXT,
					ip_address TEXT,
					user_agent TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					success BOOLEAN DEFAULT TRUE,
					error_message TEXT,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
				)`,
				`CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id) WHERE user_id IS NOT NULL`,
				`CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC)`,
				`CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id)`,
				`CREATE INDEX idx_audit_logs_action ON audit_logs(action)`,
			},
			"postgresql": {
				// Audit logs table - comprehensive audit trail
				`CREATE TABLE audit_logs (
					id TEXT PRIMARY KEY,
					user_id TEXT,
					action TEXT NOT NULL,
					resource_type TEXT NOT NULL,
					resource_id TEXT,
					old_values JSONB,
					new_values JSONB,
					ip_address TEXT,
					user_agent TEXT,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					success BOOLEAN DEFAULT TRUE,
					error_message TEXT,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
				)`,
				`CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id) WHERE user_id IS NOT NULL`,
				`CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC)`,
				`CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id)`,
				`CREATE INDEX idx_audit_logs_action ON audit_logs(action)`,
			},
			"mysql": {
				// Audit logs table - comprehensive audit trail
				`CREATE TABLE audit_logs (
					id VARCHAR(255) PRIMARY KEY,
					user_id VARCHAR(255),
					action VARCHAR(255) NOT NULL,
					resource_type VARCHAR(100) NOT NULL,
					resource_id VARCHAR(255),
					old_values JSON,
					new_values JSON,
					ip_address VARCHAR(45),
					user_agent TEXT,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					success BOOLEAN DEFAULT TRUE,
					error_message TEXT,
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
				) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
				`CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id)`,
				`CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC)`,
				`CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id)`,
				`CREATE INDEX idx_audit_logs_action ON audit_logs(action)`,
			},
		},
		Down: map[string][]string{
			"all": {
				"DROP TABLE IF EXISTS audit_logs",
			},
		},
	})
}
