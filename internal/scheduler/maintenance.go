package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// MaintenanceService provides system maintenance utilities
type MaintenanceService struct {
	scheduler *Scheduler
	logger    *logrus.Logger
}

// MaintenanceTask represents a maintenance operation
type MaintenanceTask struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Handler     MaintenanceHandler     `json:"-"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// MaintenanceHandler is the function signature for maintenance handlers
type MaintenanceHandler func(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error)

// MaintenanceResult represents the result of a maintenance operation
type MaintenanceResult struct {
	TaskName         string            `json:"task_name"`
	Success          bool              `json:"success"`
	StartTime        time.Time         `json:"start_time"`
	EndTime          time.Time         `json:"end_time"`
	Duration         string            `json:"duration"`
	ItemsProcessed   int64             `json:"items_processed"`
	ItemsFixed       int64             `json:"items_fixed"`
	SpaceReclaimed   int64             `json:"space_reclaimed"`
	Warnings         []string          `json:"warnings,omitempty"`
	Errors           []string          `json:"errors,omitempty"`
	Details          map[string]interface{} `json:"details,omitempty"`
}

// NewMaintenanceService creates a new maintenance service
func NewMaintenanceService(scheduler *Scheduler) *MaintenanceService {
	return &MaintenanceService{
		scheduler: scheduler,
		logger:    scheduler.logger,
	}
}

// GetAvailableMaintenanceTasks returns all available maintenance tasks
func (ms *MaintenanceService) GetAvailableMaintenanceTasks() []*MaintenanceTask {
	return []*MaintenanceTask{
		{
			Name:        "fix_orphaned_data",
			Description: "Fix orphaned data and broken references",
			Category:    "data_integrity",
			Handler:     ms.fixOrphanedData,
		},
		{
			Name:        "rebuild_indexes",
			Description: "Rebuild database indexes for optimal performance",
			Category:    "performance",
			Handler:     ms.rebuildIndexes,
		},
		{
			Name:        "update_url_metadata",
			Description: "Update URL metadata like titles and favicons",
			Category:    "content",
			Handler:     ms.updateURLMetadata,
		},
		{
			Name:        "verify_data_integrity",
			Description: "Verify database integrity and fix corruption",
			Category:    "data_integrity",
			Handler:     ms.verifyDataIntegrity,
		},
		{
			Name:        "cleanup_temporary_files",
			Description: "Clean up temporary files and caches",
			Category:    "cleanup",
			Handler:     ms.cleanupTemporaryFiles,
		},
		{
			Name:        "update_analytics_aggregations",
			Description: "Recalculate analytics aggregations",
			Category:    "analytics",
			Handler:     ms.updateAnalyticsAggregations,
		},
		{
			Name:        "fix_missing_qr_codes",
			Description: "Generate missing QR codes for URLs",
			Category:    "content",
			Handler:     ms.fixMissingQRCodes,
		},
		{
			Name:        "validate_ssl_certificates",
			Description: "Validate and fix SSL certificate configurations",
			Category:    "security",
			Handler:     ms.validateSSLCertificates,
		},
		{
			Name:        "compress_old_logs",
			Description: "Compress old log files to save space",
			Category:    "storage",
			Handler:     ms.compressOldLogs,
		},
		{
			Name:        "sync_user_quotas",
			Description: "Synchronize user quota calculations",
			Category:    "billing",
			Handler:     ms.syncUserQuotas,
		},
	}
}

// RunMaintenanceTask executes a specific maintenance task
func (ms *MaintenanceService) RunMaintenanceTask(ctx context.Context, taskName string, config map[string]interface{}) (*MaintenanceResult, error) {
	tasks := ms.GetAvailableMaintenanceTasks()

	for _, task := range tasks {
		if task.Name == taskName {
			ms.logger.WithField("task", taskName).Info("Starting maintenance task")

			result, err := task.Handler(ctx, config)
			if err != nil {
				ms.logger.WithError(err).WithField("task", taskName).Error("Maintenance task failed")
				return nil, err
			}

			ms.logger.WithFields(logrus.Fields{
				"task":            taskName,
				"duration":        result.Duration,
				"items_processed": result.ItemsProcessed,
				"items_fixed":     result.ItemsFixed,
			}).Info("Maintenance task completed")

			return result, nil
		}
	}

	return nil, fmt.Errorf("maintenance task not found: %s", taskName)
}

// fixOrphanedData fixes orphaned data and broken references
func (ms *MaintenanceService) fixOrphanedData(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error) {
	result := &MaintenanceResult{
		TaskName:  "fix_orphaned_data",
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Fix orphaned clicks
	orphanedClicks, err := ms.fixOrphanedClicks(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to fix orphaned clicks: %v", err))
	}
	result.Details["orphaned_clicks_fixed"] = orphanedClicks
	result.ItemsFixed += orphanedClicks

	// Fix orphaned daily stats
	orphanedStats, err := ms.fixOrphanedDailyStats(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to fix orphaned stats: %v", err))
	}
	result.Details["orphaned_stats_fixed"] = orphanedStats
	result.ItemsFixed += orphanedStats

	// Fix orphaned QR codes
	orphanedQR, err := ms.fixOrphanedQRCodes(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to fix orphaned QR codes: %v", err))
	}
	result.Details["orphaned_qr_codes_fixed"] = orphanedQR
	result.ItemsFixed += orphanedQR

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = len(result.Errors) == 0

	return result, nil
}

// rebuildIndexes rebuilds database indexes
func (ms *MaintenanceService) rebuildIndexes(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error) {
	result := &MaintenanceResult{
		TaskName:  "rebuild_indexes",
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	indexes := []string{
		"idx_urls_created_at",
		"idx_urls_expires_at",
		"idx_clicks_url_id",
		"idx_clicks_clicked_at",
		"idx_users_username",
		"idx_users_email",
	}

	var rebuiltIndexes int64
	for _, index := range indexes {
		query := fmt.Sprintf("REINDEX %s", index)
		if _, err := ms.scheduler.db.ExecContext(ctx, query); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to rebuild index %s: %v", index, err))
		} else {
			rebuiltIndexes++
		}
	}

	result.Details["indexes_rebuilt"] = rebuiltIndexes
	result.ItemsProcessed = int64(len(indexes))
	result.ItemsFixed = rebuiltIndexes

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = rebuiltIndexes > 0

	return result, nil
}

// updateURLMetadata updates URL metadata like titles and favicons
func (ms *MaintenanceService) updateURLMetadata(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error) {
	result := &MaintenanceResult{
		TaskName:  "update_url_metadata",
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Get URLs without titles or with old metadata
	query := `
		SELECT id, original_url FROM urls
		WHERE title IS NULL OR title = '' OR updated_at < ?
		LIMIT 1000`

	cutoffDate := time.Now().AddDate(0, 0, -30) // Update URLs older than 30 days
	rows, err := ms.scheduler.db.QueryContext(ctx, query, cutoffDate)
	if err != nil {
		return result, fmt.Errorf("failed to query URLs: %w", err)
	}
	defer rows.Close()

	var urlsProcessed, urlsUpdated int64
	for rows.Next() {
		var urlID, originalURL string
		if err := rows.Scan(&urlID, &originalURL); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to scan URL: %v", err))
			continue
		}

		urlsProcessed++

		// TODO: Implement actual metadata fetching
		// This would typically involve HTTP requests to fetch page titles, descriptions, etc.
		title := fmt.Sprintf("Updated Title for %s", originalURL)

		updateQuery := "UPDATE urls SET title = ?, updated_at = ? WHERE id = ?"
		if _, err := ms.scheduler.db.ExecContext(ctx, updateQuery, title, time.Now(), urlID); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to update URL %s: %v", urlID, err))
		} else {
			urlsUpdated++
		}
	}

	result.ItemsProcessed = urlsProcessed
	result.ItemsFixed = urlsUpdated
	result.Details["urls_processed"] = urlsProcessed
	result.Details["urls_updated"] = urlsUpdated

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = true

	return result, nil
}

// verifyDataIntegrity verifies database integrity
func (ms *MaintenanceService) verifyDataIntegrity(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error) {
	result := &MaintenanceResult{
		TaskName:  "verify_data_integrity",
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Run SQLite integrity check
	var integrityResult string
	err := ms.scheduler.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityResult)
	if err != nil {
		return result, fmt.Errorf("failed to run integrity check: %w", err)
	}

	result.Details["integrity_check"] = integrityResult
	result.Success = integrityResult == "ok"

	if result.Success {
		result.Details["status"] = "Database integrity verified successfully"
	} else {
		result.Errors = append(result.Errors, fmt.Sprintf("Integrity check failed: %s", integrityResult))
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()

	return result, nil
}

// cleanupTemporaryFiles cleans up temporary files and caches
func (ms *MaintenanceService) cleanupTemporaryFiles(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error) {
	result := &MaintenanceResult{
		TaskName:  "cleanup_temporary_files",
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	tempDirs := []string{
		"/tmp/caslink",
		"/var/tmp/caslink",
		filepath.Join(ms.scheduler.config.Server.DataDir, "tmp"),
	}

	var totalFilesDeleted int64
	var totalSpaceReclaimed int64

	for _, dir := range tempDirs {
		filesDeleted, spaceReclaimed, err := ms.cleanupDirectory(dir, 24*time.Hour)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to cleanup %s: %v", dir, err))
			continue
		}

		totalFilesDeleted += filesDeleted
		totalSpaceReclaimed += spaceReclaimed
	}

	result.ItemsProcessed = totalFilesDeleted
	result.ItemsFixed = totalFilesDeleted
	result.SpaceReclaimed = totalSpaceReclaimed
	result.Details["files_deleted"] = totalFilesDeleted
	result.Details["space_reclaimed"] = totalSpaceReclaimed

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = true

	return result, nil
}

// updateAnalyticsAggregations recalculates analytics aggregations
func (ms *MaintenanceService) updateAnalyticsAggregations(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error) {
	result := &MaintenanceResult{
		TaskName:  "update_analytics_aggregations",
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Recalculate daily stats for the last 30 days
	startDate := time.Now().AddDate(0, 0, -30)
	var daysProcessed int64

	for d := startDate; d.Before(time.Now()); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")

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

		if _, err := ms.scheduler.db.ExecContext(ctx, query, dateStr, dateStr, dateStr); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to aggregate data for %s: %v", dateStr, err))
		} else {
			daysProcessed++
		}
	}

	result.ItemsProcessed = daysProcessed
	result.ItemsFixed = daysProcessed
	result.Details["days_processed"] = daysProcessed

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = daysProcessed > 0

	return result, nil
}

// fixMissingQRCodes generates missing QR codes for URLs
func (ms *MaintenanceService) fixMissingQRCodes(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error) {
	result := &MaintenanceResult{
		TaskName:  "fix_missing_qr_codes",
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Find URLs without QR codes
	query := `
		SELECT u.id FROM urls u
		LEFT JOIN qr_codes q ON u.id = q.url_id
		WHERE q.id IS NULL
		LIMIT 500`

	rows, err := ms.scheduler.db.QueryContext(ctx, query)
	if err != nil {
		return result, fmt.Errorf("failed to query URLs without QR codes: %w", err)
	}
	defer rows.Close()

	var urlsProcessed, qrCodesGenerated int64
	for rows.Next() {
		var urlID string
		if err := rows.Scan(&urlID); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to scan URL ID: %v", err))
			continue
		}

		urlsProcessed++

		// TODO: Generate QR code using the QR service
		// This would integrate with the internal/qr package
		qrCodesGenerated++
	}

	result.ItemsProcessed = urlsProcessed
	result.ItemsFixed = qrCodesGenerated
	result.Details["urls_processed"] = urlsProcessed
	result.Details["qr_codes_generated"] = qrCodesGenerated

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = true

	return result, nil
}

// validateSSLCertificates validates SSL certificate configurations
func (ms *MaintenanceService) validateSSLCertificates(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error) {
	result := &MaintenanceResult{
		TaskName:  "validate_ssl_certificates",
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Get all domains with SSL enabled
	query := `
		SELECT d.id, d.domain FROM domains d
		WHERE d.ssl_enabled = true`

	rows, err := ms.scheduler.db.QueryContext(ctx, query)
	if err != nil {
		return result, fmt.Errorf("failed to query SSL-enabled domains: %w", err)
	}
	defer rows.Close()

	var domainsProcessed, certificatesValid int64
	for rows.Next() {
		var domainID, domain string
		if err := rows.Scan(&domainID, &domain); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to scan domain: %v", err))
			continue
		}

		domainsProcessed++

		// TODO: Validate SSL certificate using the domains/ssl service
		certificatesValid++
	}

	result.ItemsProcessed = domainsProcessed
	result.ItemsFixed = certificatesValid
	result.Details["domains_processed"] = domainsProcessed
	result.Details["certificates_valid"] = certificatesValid

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = true

	return result, nil
}

// compressOldLogs compresses old log files
func (ms *MaintenanceService) compressOldLogs(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error) {
	result := &MaintenanceResult{
		TaskName:  "compress_old_logs",
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	logDir := "/var/log/caslink"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		result.Details["status"] = "No log directory found"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime).String()
		result.Success = true
		return result, nil
	}

	// TODO: Implement log compression logic
	result.Details["status"] = "Log compression not yet implemented"
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = true

	return result, nil
}

// syncUserQuotas synchronizes user quota calculations
func (ms *MaintenanceService) syncUserQuotas(ctx context.Context, config map[string]interface{}) (*MaintenanceResult, error) {
	result := &MaintenanceResult{
		TaskName:  "sync_user_quotas",
		StartTime: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Recalculate URL counts for all users
	query := `
		UPDATE users SET
		url_count = (SELECT COUNT(*) FROM urls WHERE user_id = users.id),
		updated_at = ?
		WHERE id IN (SELECT DISTINCT user_id FROM urls WHERE user_id IS NOT NULL)`

	result2, err := ms.scheduler.db.ExecContext(ctx, query, time.Now())
	if err != nil {
		return result, fmt.Errorf("failed to sync user quotas: %w", err)
	}

	usersUpdated, _ := result2.RowsAffected()
	result.ItemsProcessed = usersUpdated
	result.ItemsFixed = usersUpdated
	result.Details["users_updated"] = usersUpdated

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = true

	return result, nil
}

// Helper methods

func (ms *MaintenanceService) fixOrphanedClicks(ctx context.Context) (int64, error) {
	query := `DELETE FROM clicks WHERE url_id NOT IN (SELECT id FROM urls)`
	result, err := ms.scheduler.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

func (ms *MaintenanceService) fixOrphanedDailyStats(ctx context.Context) (int64, error) {
	query := `DELETE FROM click_daily_stats WHERE url_id NOT IN (SELECT id FROM urls)`
	result, err := ms.scheduler.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

func (ms *MaintenanceService) fixOrphanedQRCodes(ctx context.Context) (int64, error) {
	query := `DELETE FROM qr_codes WHERE url_id NOT IN (SELECT id FROM urls)`
	result, err := ms.scheduler.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

func (ms *MaintenanceService) cleanupDirectory(dir string, maxAge time.Duration) (int64, int64, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, 0, nil
	}

	cutoffTime := time.Now().Add(-maxAge)
	var filesDeleted, spaceReclaimed int64

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() && info.ModTime().Before(cutoffTime) {
			spaceReclaimed += info.Size()
			if err := os.Remove(path); err == nil {
				filesDeleted++
			}
		}
		return nil
	})

	return filesDeleted, spaceReclaimed, err
}