package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// CleanupService provides database cleanup utilities
type CleanupService struct {
	scheduler *Scheduler
	logger    *logrus.Logger
}

// CleanupConfig contains cleanup configuration
type CleanupConfig struct {
	AnalyticsRetentionDays    int `json:"analytics_retention_days"`
	SessionRetentionDays      int `json:"session_retention_days"`
	WebhookRetentionDays      int `json:"webhook_retention_days"`
	AuditLogRetentionDays     int `json:"audit_log_retention_days"`
	TempFileRetentionHours    int `json:"temp_file_retention_hours"`
	OrphanedDataRetentionDays int `json:"orphaned_data_retention_days"`
}

// CleanupStats represents cleanup operation statistics
type CleanupStats struct {
	Operation        string    `json:"operation"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	Duration         string    `json:"duration"`
	RecordsProcessed int64     `json:"records_processed"`
	RecordsDeleted   int64     `json:"records_deleted"`
	SpaceFreed       int64     `json:"space_freed_bytes"`
	Errors           []string  `json:"errors,omitempty"`
}

// NewCleanupService creates a new cleanup service
func NewCleanupService(scheduler *Scheduler) *CleanupService {
	return &CleanupService{
		scheduler: scheduler,
		logger:    scheduler.logger,
	}
}

// persistTask persists a task to the database
func (s *Scheduler) persistTask(task *ScheduledTask) error {
	query := `
		INSERT OR REPLACE INTO scheduled_tasks
		(id, name, description, schedule, enabled, last_run, next_run, run_count, error_count, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(context.Background(), query,
		task.ID, task.Name, task.Description, task.Schedule, task.Enabled,
		task.LastRun, task.NextRun, task.RunCount, task.ErrorCount, task.LastError,
		task.CreatedAt, task.UpdatedAt,
	)

	return err
}

// loadPersistedTasks loads tasks from the database
func (s *Scheduler) loadPersistedTasks() error {
	query := `
		SELECT id, name, description, schedule, enabled, last_run, next_run, run_count, error_count, last_error, created_at, updated_at
		FROM scheduled_tasks`

	rows, err := s.db.QueryContext(context.Background(), query)
	if err != nil {
		// If table doesn't exist, that's okay - no persisted tasks yet
		if err.Error() == "no such table: scheduled_tasks" {
			return nil
		}
		return fmt.Errorf("failed to query persisted tasks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		task := &ScheduledTask{}
		var lastRun, nextRun sql.NullTime

		err := rows.Scan(
			&task.ID, &task.Name, &task.Description, &task.Schedule, &task.Enabled,
			&lastRun, &nextRun, &task.RunCount, &task.ErrorCount, &task.LastError,
			&task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to scan persisted task")
			continue
		}

		if lastRun.Valid {
			task.LastRun = &lastRun.Time
		}
		if nextRun.Valid {
			task.NextRun = &nextRun.Time
		}

		// Only load tasks that don't have handlers (won't override default tasks)
		if _, exists := s.tasks[task.ID]; !exists {
			s.tasks[task.ID] = task
		}
	}

	return nil
}

// removePersistedTask removes a task from the database
func (s *Scheduler) removePersistedTask(taskID string) error {
	_, err := s.db.ExecContext(context.Background(), "DELETE FROM scheduled_tasks WHERE id = ?", taskID)
	return err
}

// logTaskExecution logs task execution details
func (s *Scheduler) logTaskExecution(taskID string, startTime, endTime time.Time, err error) {
	status := "completed"
	errorMsg := ""
	if err != nil {
		status = "failed"
		errorMsg = err.Error()
	}

	query := `
		INSERT INTO task_executions (id, task_id, status, started_at, completed_at, duration, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	duration := endTime.Sub(startTime)
	executionID := fmt.Sprintf("%s_%d", taskID, startTime.Unix())

	_, dbErr := s.db.ExecContext(context.Background(), query,
		executionID, taskID, status, startTime, endTime, duration.String(), errorMsg,
	)

	if dbErr != nil {
		s.logger.WithError(dbErr).Error("Failed to log task execution")
	}
}

// PerformDeepCleanup performs a comprehensive cleanup of all old data
func (cs *CleanupService) PerformDeepCleanup(ctx context.Context, config CleanupConfig) (*CleanupStats, error) {
	stats := &CleanupStats{
		Operation: "deep_cleanup",
		StartTime: time.Now(),
	}

	cs.logger.Info("Starting deep cleanup operation")

	// Clean up old analytics data
	if err := cs.cleanupOldAnalyticsData(ctx, config.AnalyticsRetentionDays, stats); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("Analytics cleanup: %v", err))
	}

	// Clean up expired sessions
	if err := cs.cleanupExpiredSessionsData(ctx, config.SessionRetentionDays, stats); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("Session cleanup: %v", err))
	}

	// Clean up old webhook deliveries
	if err := cs.cleanupOldWebhookData(ctx, config.WebhookRetentionDays, stats); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("Webhook cleanup: %v", err))
	}

	// Clean up old audit logs
	if err := cs.cleanupOldAuditLogs(ctx, config.AuditLogRetentionDays, stats); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("Audit log cleanup: %v", err))
	}

	// Clean up temporary files
	if err := cs.cleanupTempFiles(ctx, config.TempFileRetentionHours, stats); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("Temp file cleanup: %v", err))
	}

	// Clean up orphaned data
	if err := cs.cleanupOrphanedData(ctx, config.OrphanedDataRetentionDays, stats); err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("Orphaned data cleanup: %v", err))
	}

	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime).String()

	cs.logger.WithFields(logrus.Fields{
		"duration":          stats.Duration,
		"records_deleted":   stats.RecordsDeleted,
		"space_freed":       stats.SpaceFreed,
		"errors":            len(stats.Errors),
	}).Info("Deep cleanup completed")

	return stats, nil
}

// cleanupOldAnalyticsData removes old analytics data
func (cs *CleanupService) cleanupOldAnalyticsData(ctx context.Context, retentionDays int, stats *CleanupStats) error {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	// Delete old clicks
	result, err := cs.scheduler.db.ExecContext(ctx, "DELETE FROM clicks WHERE clicked_at < ?", cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old clicks: %w", err)
	}
	clicksDeleted, _ := result.RowsAffected()
	stats.RecordsDeleted += clicksDeleted

	// Delete old daily stats
	result, err = cs.scheduler.db.ExecContext(ctx, "DELETE FROM click_daily_stats WHERE date < ?", cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old daily stats: %w", err)
	}
	statsDeleted, _ := result.RowsAffected()
	stats.RecordsDeleted += statsDeleted

	cs.logger.WithFields(logrus.Fields{
		"clicks_deleted": clicksDeleted,
		"stats_deleted":  statsDeleted,
		"cutoff_date":    cutoffDate,
	}).Info("Old analytics data cleaned up")

	return nil
}

// cleanupExpiredSessionsData removes expired sessions
func (cs *CleanupService) cleanupExpiredSessionsData(ctx context.Context, retentionDays int, stats *CleanupStats) error {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	result, err := cs.scheduler.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at < ? OR last_accessed < ?", time.Now(), cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	sessionsDeleted, _ := result.RowsAffected()
	stats.RecordsDeleted += sessionsDeleted

	cs.logger.WithField("sessions_deleted", sessionsDeleted).Info("Expired sessions cleaned up")
	return nil
}

// cleanupOldWebhookData removes old webhook delivery records
func (cs *CleanupService) cleanupOldWebhookData(ctx context.Context, retentionDays int, stats *CleanupStats) error {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	// Delete old webhook deliveries
	result, err := cs.scheduler.db.ExecContext(ctx, "DELETE FROM webhook_deliveries WHERE created_at < ?", cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old webhook deliveries: %w", err)
	}
	deliveriesDeleted, _ := result.RowsAffected()
	stats.RecordsDeleted += deliveriesDeleted

	// Delete old webhook events
	result, err = cs.scheduler.db.ExecContext(ctx, "DELETE FROM webhook_events WHERE created_at < ?", cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old webhook events: %w", err)
	}
	eventsDeleted, _ := result.RowsAffected()
	stats.RecordsDeleted += eventsDeleted

	cs.logger.WithFields(logrus.Fields{
		"deliveries_deleted": deliveriesDeleted,
		"events_deleted":     eventsDeleted,
	}).Info("Old webhook data cleaned up")

	return nil
}

// cleanupOldAuditLogs removes old audit log entries
func (cs *CleanupService) cleanupOldAuditLogs(ctx context.Context, retentionDays int, stats *CleanupStats) error {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	result, err := cs.scheduler.db.ExecContext(ctx, "DELETE FROM audit_logs WHERE created_at < ?", cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete old audit logs: %w", err)
	}

	logsDeleted, _ := result.RowsAffected()
	stats.RecordsDeleted += logsDeleted

	cs.logger.WithField("logs_deleted", logsDeleted).Info("Old audit logs cleaned up")
	return nil
}

// cleanupTempFiles removes old temporary files
func (cs *CleanupService) cleanupTempFiles(ctx context.Context, retentionHours int, stats *CleanupStats) error {
	tempDir := "/tmp/caslink"
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		return nil // No temp directory
	}

	cutoffTime := time.Now().Add(-time.Duration(retentionHours) * time.Hour)
	var filesDeleted int64
	var spaceFreed int64

	err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		if !info.IsDir() && info.ModTime().Before(cutoffTime) {
			spaceFreed += info.Size()
			if err := os.Remove(path); err != nil {
				cs.logger.WithError(err).WithField("file", path).Warn("Failed to delete temp file")
			} else {
				filesDeleted++
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk temp directory: %w", err)
	}

	stats.RecordsDeleted += filesDeleted
	stats.SpaceFreed += spaceFreed

	cs.logger.WithFields(logrus.Fields{
		"files_deleted": filesDeleted,
		"space_freed":   spaceFreed,
	}).Info("Temporary files cleaned up")

	return nil
}

// cleanupOrphanedData removes data that has lost its parent references
func (cs *CleanupService) cleanupOrphanedData(ctx context.Context, retentionDays int, stats *CleanupStats) error {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	// Clean up clicks for non-existent URLs
	result, err := cs.scheduler.db.ExecContext(ctx, `
		DELETE FROM clicks
		WHERE url_id NOT IN (SELECT id FROM urls)
		AND clicked_at < ?`, cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete orphaned clicks: %w", err)
	}
	orphanedClicks, _ := result.RowsAffected()

	// Clean up daily stats for non-existent URLs
	result, err = cs.scheduler.db.ExecContext(ctx, `
		DELETE FROM click_daily_stats
		WHERE url_id NOT IN (SELECT id FROM urls)
		AND date < ?`, cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete orphaned daily stats: %w", err)
	}
	orphanedStats, _ := result.RowsAffected()

	// Clean up QR codes for non-existent URLs
	result, err = cs.scheduler.db.ExecContext(ctx, `
		DELETE FROM qr_codes
		WHERE url_id NOT IN (SELECT id FROM urls)
		AND created_at < ?`, cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete orphaned QR codes: %w", err)
	}
	orphanedQR, _ := result.RowsAffected()

	// Clean up uploads for non-existent URLs
	result, err = cs.scheduler.db.ExecContext(ctx, `
		DELETE FROM uploads
		WHERE url_id NOT IN (SELECT id FROM urls)
		AND uploaded_at < ?`, cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to delete orphaned uploads: %w", err)
	}
	orphanedUploads, _ := result.RowsAffected()

	totalOrphaned := orphanedClicks + orphanedStats + orphanedQR + orphanedUploads
	stats.RecordsDeleted += totalOrphaned

	cs.logger.WithFields(logrus.Fields{
		"orphaned_clicks":     orphanedClicks,
		"orphaned_stats":      orphanedStats,
		"orphaned_qr_codes":   orphanedQR,
		"orphaned_uploads":    orphanedUploads,
		"total_orphaned":      totalOrphaned,
	}).Info("Orphaned data cleaned up")

	return nil
}

// OptimizeDatabase performs database optimization
func (cs *CleanupService) OptimizeDatabase(ctx context.Context) (*CleanupStats, error) {
	stats := &CleanupStats{
		Operation: "database_optimization",
		StartTime: time.Now(),
	}

	cs.logger.Info("Starting database optimization")

	// Get database size before optimization
	var sizeBefore int64
	err := cs.scheduler.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&sizeBefore)
	if err != nil {
		cs.logger.WithError(err).Warn("Failed to get database size before optimization")
	}

	// Run VACUUM to reclaim space
	_, err = cs.scheduler.db.ExecContext(ctx, "VACUUM")
	if err != nil {
		return stats, fmt.Errorf("failed to run VACUUM: %w", err)
	}

	// Run ANALYZE to update statistics
	_, err = cs.scheduler.db.ExecContext(ctx, "ANALYZE")
	if err != nil {
		cs.logger.WithError(err).Warn("Failed to run ANALYZE")
	}

	// Reindex all tables
	_, err = cs.scheduler.db.ExecContext(ctx, "REINDEX")
	if err != nil {
		cs.logger.WithError(err).Warn("Failed to run REINDEX")
	}

	// Get database size after optimization
	var sizeAfter int64
	err = cs.scheduler.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&sizeAfter)
	if err != nil {
		cs.logger.WithError(err).Warn("Failed to get database size after optimization")
	}

	stats.SpaceFreed = (sizeBefore - sizeAfter) * 4096 // Assuming 4KB page size
	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime).String()

	cs.logger.WithFields(logrus.Fields{
		"duration":    stats.Duration,
		"space_freed": stats.SpaceFreed,
	}).Info("Database optimization completed")

	return stats, nil
}

// GetDefaultCleanupConfig returns default cleanup configuration
func GetDefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		AnalyticsRetentionDays:    365, // 1 year
		SessionRetentionDays:      30,  // 1 month
		WebhookRetentionDays:      90,  // 3 months
		AuditLogRetentionDays:     180, // 6 months
		TempFileRetentionHours:    24,  // 1 day
		OrphanedDataRetentionDays: 7,   // 1 week
	}
}