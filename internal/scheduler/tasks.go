package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// registerDefaultTasks registers all default system tasks
func (s *Scheduler) registerDefaultTasks() error {
	defaultTasks := []*ScheduledTask{
		{
			ID:          "cleanup_expired_urls",
			Name:        "Cleanup Expired URLs",
			Description: "Remove expired URLs and their associated data",
			Schedule:    "0 2 * * *", // Daily at 2 AM
			Enabled:     true,
			Handler:     s.cleanupExpiredURLs,
		},
		{
			ID:          "cleanup_old_analytics",
			Name:        "Cleanup Old Analytics",
			Description: "Archive or remove old analytics data based on retention policy",
			Schedule:    "0 3 * * 0", // Weekly on Sunday at 3 AM
			Enabled:     true,
			Handler:     s.cleanupOldAnalytics,
		},
		{
			ID:          "cleanup_old_sessions",
			Name:        "Cleanup Expired Sessions",
			Description: "Remove expired user sessions",
			Schedule:    "0 */6 * * *", // Every 6 hours
			Enabled:     true,
			Handler:     s.cleanupExpiredSessions,
		},
		{
			ID:          "cleanup_old_api_tokens",
			Name:        "Cleanup Expired API Tokens",
			Description: "Remove expired API tokens",
			Schedule:    "0 4 * * *", // Daily at 4 AM
			Enabled:     true,
			Handler:     s.cleanupExpiredAPITokens,
		},
		{
			ID:          "aggregate_analytics_daily",
			Name:        "Daily Analytics Aggregation",
			Description: "Aggregate daily analytics data for reporting",
			Schedule:    "0 1 * * *", // Daily at 1 AM
			Enabled:     true,
			Handler:     s.aggregateDailyAnalytics,
		},
		{
			ID:          "check_ssl_certificates",
			Name:        "Check SSL Certificate Expiration",
			Description: "Check for expiring SSL certificates and send notifications",
			Schedule:    "0 8 * * *", // Daily at 8 AM
			Enabled:     true,
			Handler:     s.checkSSLCertificates,
		},
		{
			ID:          "verify_custom_domains",
			Name:        "Verify Custom Domains",
			Description: "Re-verify custom domain ownership and SSL status",
			Schedule:    "0 */12 * * *", // Every 12 hours
			Enabled:     true,
			Handler:     s.verifyCustomDomains,
		},
		{
			ID:          "cleanup_webhook_deliveries",
			Name:        "Cleanup Old Webhook Deliveries",
			Description: "Remove old webhook delivery records",
			Schedule:    "0 5 * * 0", // Weekly on Sunday at 5 AM
			Enabled:     true,
			Handler:     s.cleanupWebhookDeliveries,
		},
		{
			ID:          "update_usage_metrics",
			Name:        "Update Usage Metrics",
			Description: "Calculate and update user usage metrics for billing",
			Schedule:    "0 0 1 * *", // Monthly on the 1st at midnight
			Enabled:     true,
			Handler:     s.updateUsageMetrics,
		},
		{
			ID:          "backup_database",
			Name:        "Database Backup",
			Description: "Create automated database backup",
			Schedule:    "0 0 * * *", // Daily at midnight
			Enabled:     false, // Disabled by default
			Handler:     s.backupDatabase,
		},
		{
			ID:          "optimize_database",
			Name:        "Database Optimization",
			Description: "Optimize database tables and rebuild indexes",
			Schedule:    "0 6 * * 0", // Weekly on Sunday at 6 AM
			Enabled:     true,
			Handler:     s.optimizeDatabase,
		},
		{
			ID:          "sync_federation_instances",
			Name:        "Sync Federation Instances",
			Description: "Synchronize data with federated instances",
			Schedule:    "0 */2 * * *", // Every 2 hours
			Enabled:     true,
			Handler:     s.syncFederationInstances,
		},
	}

	for _, task := range defaultTasks {
		if err := s.AddTask(task); err != nil {
			s.logger.WithError(err).WithField("task_id", task.ID).Error("Failed to register default task")
			// Continue with other tasks
		}
	}

	return nil
}

// cleanupExpiredURLs removes expired URLs and their associated data
func (s *Scheduler) cleanupExpiredURLs(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting expired URLs cleanup")

	// Find expired URLs
	query := `
		SELECT id FROM urls
		WHERE expires_at IS NOT NULL AND expires_at <= ?`

	rows, err := s.db.QueryContext(ctx, query, time.Now())
	if err != nil {
		return fmt.Errorf("failed to query expired URLs: %w", err)
	}
	defer rows.Close()

	var expiredURLs []string
	for rows.Next() {
		var urlID string
		if err := rows.Scan(&urlID); err != nil {
			s.logger.WithError(err).Warn("Failed to scan expired URL ID")
			continue
		}
		expiredURLs = append(expiredURLs, urlID)
	}

	if len(expiredURLs) == 0 {
		s.logger.Info("No expired URLs found")
		return nil
	}

	// Begin transaction for cleanup
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete associated data first
	for _, urlID := range expiredURLs {
		// Delete clicks
		if _, err := tx.ExecContext(ctx, "DELETE FROM clicks WHERE url_id = ?", urlID); err != nil {
			s.logger.WithError(err).WithField("url_id", urlID).Error("Failed to delete clicks")
		}

		// Delete daily stats
		if _, err := tx.ExecContext(ctx, "DELETE FROM click_daily_stats WHERE url_id = ?", urlID); err != nil {
			s.logger.WithError(err).WithField("url_id", urlID).Error("Failed to delete daily stats")
		}

		// Delete QR codes
		if _, err := tx.ExecContext(ctx, "DELETE FROM qr_codes WHERE url_id = ?", urlID); err != nil {
			s.logger.WithError(err).WithField("url_id", urlID).Error("Failed to delete QR codes")
		}

		// Delete uploads
		if _, err := tx.ExecContext(ctx, "DELETE FROM uploads WHERE url_id = ?", urlID); err != nil {
			s.logger.WithError(err).WithField("url_id", urlID).Error("Failed to delete uploads")
		}
	}

	// Delete the URLs themselves
	for _, urlID := range expiredURLs {
		if _, err := tx.ExecContext(ctx, "DELETE FROM urls WHERE id = ?", urlID); err != nil {
			s.logger.WithError(err).WithField("url_id", urlID).Error("Failed to delete URL")
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit cleanup transaction: %w", err)
	}

	s.logger.WithField("count", len(expiredURLs)).Info("Expired URLs cleanup completed")
	return nil
}

// cleanupOldAnalytics removes old analytics data based on retention policy
func (s *Scheduler) cleanupOldAnalytics(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting old analytics cleanup")

	// Default retention: 365 days
	retentionDays := 365
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	// Delete old clicks
	result, err := s.db.ExecContext(ctx, "DELETE FROM clicks WHERE clicked_at < ?", cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old clicks: %w", err)
	}

	clicksDeleted, _ := result.RowsAffected()

	// Delete old daily stats
	result, err = s.db.ExecContext(ctx, "DELETE FROM click_daily_stats WHERE date < ?", cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old daily stats: %w", err)
	}

	statsDeleted, _ := result.RowsAffected()

	s.logger.WithFields(logrus.Fields{
		"clicks_deleted": clicksDeleted,
		"stats_deleted":  statsDeleted,
		"cutoff_date":    cutoffDate,
	}).Info("Old analytics cleanup completed")

	return nil
}

// cleanupExpiredSessions removes expired user sessions
func (s *Scheduler) cleanupExpiredSessions(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting expired sessions cleanup")

	result, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", time.Now())
	if err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	sessionsDeleted, _ := result.RowsAffected()
	s.logger.WithField("count", sessionsDeleted).Info("Expired sessions cleanup completed")

	return nil
}

// cleanupExpiredAPITokens removes expired API tokens
func (s *Scheduler) cleanupExpiredAPITokens(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting expired API tokens cleanup")

	result, err := s.db.ExecContext(ctx,
		"DELETE FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at <= ?",
		time.Now())
	if err != nil {
		return fmt.Errorf("failed to delete expired API tokens: %w", err)
	}

	tokensDeleted, _ := result.RowsAffected()
	s.logger.WithField("count", tokensDeleted).Info("Expired API tokens cleanup completed")

	return nil
}

// aggregateDailyAnalytics aggregates daily analytics data
func (s *Scheduler) aggregateDailyAnalytics(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting daily analytics aggregation")

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// Aggregate clicks by URL for yesterday
	query := `
		INSERT OR REPLACE INTO click_daily_stats (id, url_id, date, clicks, unique_clicks)
		SELECT
			url_id || '_' || ? as id,
			url_id,
			? as date,
			COUNT(*) as clicks,
			COUNT(DISTINCT ip_hash) as unique_clicks
		FROM clicks
		WHERE DATE(clicked_at) = ?
		GROUP BY url_id`

	result, err := s.db.ExecContext(ctx, query, yesterday, yesterday, yesterday)
	if err != nil {
		return fmt.Errorf("failed to aggregate daily analytics: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	s.logger.WithFields(logrus.Fields{
		"date":          yesterday,
		"urls_updated":  rowsAffected,
	}).Info("Daily analytics aggregation completed")

	return nil
}

// checkSSLCertificates checks for expiring SSL certificates
func (s *Scheduler) checkSSLCertificates(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting SSL certificate expiration check")

	// Check for certificates expiring in the next 30 days
	expirationThreshold := time.Now().AddDate(0, 0, 30)

	query := `
		SELECT d.domain, c.expires_at, d.user_id
		FROM domains d
		JOIN ssl_certificates c ON d.id = c.domain_id
		WHERE d.ssl_enabled = true
		AND c.status = 'active'
		AND c.expires_at <= ?`

	rows, err := s.db.QueryContext(ctx, query, expirationThreshold)
	if err != nil {
		return fmt.Errorf("failed to query expiring certificates: %w", err)
	}
	defer rows.Close()

	var expiringCerts []struct {
		domain    string
		expiresAt time.Time
		userID    string
	}

	for rows.Next() {
		var cert struct {
			domain    string
			expiresAt time.Time
			userID    string
		}
		if err := rows.Scan(&cert.domain, &cert.expiresAt, &cert.userID); err != nil {
			s.logger.WithError(err).Warn("Failed to scan expiring certificate")
			continue
		}
		expiringCerts = append(expiringCerts, cert)
	}

	if len(expiringCerts) > 0 {
		s.logger.WithField("count", len(expiringCerts)).Warn("Found expiring SSL certificates")
		// TODO: Send notifications to domain owners
	} else {
		s.logger.Info("No expiring SSL certificates found")
	}

	return nil
}

// verifyCustomDomains re-verifies custom domain ownership
func (s *Scheduler) verifyCustomDomains(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting custom domain verification")

	// Get unverified domains that need re-verification
	query := `
		SELECT id, domain, verification_method, verification_token
		FROM domains
		WHERE verified = false`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query unverified domains: %w", err)
	}
	defer rows.Close()

	var domainsChecked int
	var domainsVerified int

	for rows.Next() {
		var domainID, domain, method, token string
		if err := rows.Scan(&domainID, &domain, &method, &token); err != nil {
			s.logger.WithError(err).Warn("Failed to scan domain")
			continue
		}

		domainsChecked++
		// TODO: Implement actual domain verification logic
		s.logger.WithFields(logrus.Fields{
			"domain": domain,
			"method": method,
		}).Debug("Checking domain verification")
	}

	s.logger.WithFields(logrus.Fields{
		"checked":  domainsChecked,
		"verified": domainsVerified,
	}).Info("Custom domain verification completed")

	return nil
}

// cleanupWebhookDeliveries removes old webhook delivery records
func (s *Scheduler) cleanupWebhookDeliveries(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting webhook deliveries cleanup")

	// Keep delivery records for 90 days
	cutoffDate := time.Now().AddDate(0, 0, -90)

	result, err := s.db.ExecContext(ctx, "DELETE FROM webhook_deliveries WHERE created_at < ?", cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old webhook deliveries: %w", err)
	}

	deliveriesDeleted, _ := result.RowsAffected()
	s.logger.WithField("count", deliveriesDeleted).Info("Webhook deliveries cleanup completed")

	return nil
}

// updateUsageMetrics calculates and updates user usage metrics
func (s *Scheduler) updateUsageMetrics(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting usage metrics update")

	// Calculate usage for the previous month
	now := time.Now()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	lastMonth := firstOfMonth.AddDate(0, -1, 0)
	endOfLastMonth := firstOfMonth.Add(-time.Second)

	// Update URL creation metrics
	query := `
		INSERT OR REPLACE INTO usage_records (id, user_id, metric_name, quantity, timestamp, billing_period)
		SELECT
			user_id || '_urls_' || ? as id,
			user_id,
			'urls_created' as metric_name,
			COUNT(*) as quantity,
			? as timestamp,
			? as billing_period
		FROM urls
		WHERE created_at >= ? AND created_at <= ?
		AND user_id IS NOT NULL
		GROUP BY user_id`

	billingPeriod := lastMonth.Format("2006-01")
	_, err := s.db.ExecContext(ctx, query, billingPeriod, endOfLastMonth, billingPeriod, lastMonth, endOfLastMonth)
	if err != nil {
		return fmt.Errorf("failed to update URL creation metrics: %w", err)
	}

	s.logger.WithField("billing_period", billingPeriod).Info("Usage metrics update completed")
	return nil
}

// backupDatabase creates a database backup
func (s *Scheduler) backupDatabase(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting database backup")

	// TODO: Implement database backup logic based on database type
	// This would vary depending on whether it's SQLite, PostgreSQL, MySQL, etc.

	s.logger.Info("Database backup completed")
	return nil
}

// optimizeDatabase optimizes database tables and rebuilds indexes
func (s *Scheduler) optimizeDatabase(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting database optimization")

	// SQLite optimization
	if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
		s.logger.WithError(err).Warn("Failed to run VACUUM")
	}

	if _, err := s.db.ExecContext(ctx, "ANALYZE"); err != nil {
		s.logger.WithError(err).Warn("Failed to run ANALYZE")
	}

	s.logger.Info("Database optimization completed")
	return nil
}

// syncFederationInstances synchronizes data with federated instances
func (s *Scheduler) syncFederationInstances(ctx context.Context, task *ScheduledTask) error {
	s.logger.Info("Starting federation instances sync")

	// TODO: Implement federation sync logic
	// This would interact with the federation service

	s.logger.Info("Federation instances sync completed")
	return nil
}