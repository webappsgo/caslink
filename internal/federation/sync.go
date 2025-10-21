package federation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// SyncService handles URL synchronization with federated instances
type SyncService struct {
	db     *db.DB
	config *config.FederationConfig
	logger *logrus.Logger
	client *FederationClient
	server *FederationServer
}

// SyncStatus represents the status of a sync operation
type SyncStatus struct {
	InstanceDomain string    `json:"instance_domain"`
	LastSync       time.Time `json:"last_sync"`
	Status         string    `json:"status"`
	URLsReceived   int       `json:"urls_received"`
	URLsShared     int       `json:"urls_shared"`
	Error          string    `json:"error,omitempty"`
}

// NewSyncService creates a new sync service
func NewSyncService(database *db.DB, cfg *config.FederationConfig, logger *logrus.Logger, client *FederationClient, server *FederationServer) (*SyncService, error) {
	return &SyncService{
		db:     database,
		config: cfg,
		logger: logger,
		client: client,
		server: server,
	}, nil
}

// ShareURL shares a URL with all federated instances
func (s *SyncService) ShareURL(ctx context.Context, url *FederatedURL) error {
	if !s.config.SharePublicURLs {
		return nil // Sharing disabled
	}

	s.logger.WithField("url", url.ShortCode).Debug("Sharing URL with federated instances")

	// Get all active instances
	instances, err := s.getActiveInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active instances: %w", err)
	}

	if len(instances) == 0 {
		s.logger.Debug("No active instances to share with")
		return nil
	}

	// Convert FederatedURL to map for client
	urlData := map[string]interface{}{
		"original_url": url.OriginalURL,
		"short_code":   url.ShortCode,
		"title":        url.Title,
		"created_at":   url.CreatedAt,
	}

	// Share with each instance in parallel
	var wg sync.WaitGroup
	errorChan := make(chan error, len(instances))

	for _, instance := range instances {
		wg.Add(1)
		go func(inst *FederatedInstance) {
			defer wg.Done()

			if err := s.client.ShareURL(ctx, inst, urlData); err != nil {
				s.logger.WithError(err).WithField("instance", inst.Domain).Warn("Failed to share URL with instance")
				errorChan <- fmt.Errorf("failed to share with %s: %w", inst.Domain, err)
			} else {
				s.logger.WithField("instance", inst.Domain).Debug("Successfully shared URL")
			}
		}(instance)
	}

	wg.Wait()
	close(errorChan)

	// Collect errors
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		s.logger.WithField("error_count", len(errors)).Warn("Some instances failed to receive URL")
		// Don't return error unless all instances failed
		if len(errors) == len(instances) {
			return fmt.Errorf("failed to share URL with any instance")
		}
	}

	s.logger.WithFields(logrus.Fields{
		"url":       url.ShortCode,
		"instances": len(instances),
		"errors":    len(errors),
	}).Info("URL sharing completed")

	return nil
}

// SyncFromInstance synchronizes URLs from a specific instance
func (s *SyncService) SyncFromInstance(ctx context.Context, instanceDomain string) error {
	s.logger.WithField("instance", instanceDomain).Debug("Starting sync from instance")

	// Get instance
	instance, err := s.getInstanceByDomain(ctx, instanceDomain)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	if instance.Blocked {
		return fmt.Errorf("instance %s is blocked", instanceDomain)
	}

	if !instance.SyncEnabled {
		return fmt.Errorf("sync disabled for instance %s", instanceDomain)
	}

	// Fetch URLs from instance
	urls, err := s.client.FetchURLs(ctx, instance)
	if err != nil {
		return fmt.Errorf("failed to fetch URLs from instance: %w", err)
	}

	// Process each URL
	processedCount := 0
	for _, url := range urls {
		if err := s.processFederatedURL(ctx, instance, url); err != nil {
			s.logger.WithError(err).WithField("url", url.ShortCode).Warn("Failed to process federated URL")
			continue
		}
		processedCount++
	}

	// Update last sync time
	now := time.Now()
	if err := s.updateInstanceLastSync(ctx, instance.ID, &now); err != nil {
		s.logger.WithError(err).Warn("Failed to update last sync time")
	}

	s.logger.WithFields(logrus.Fields{
		"instance":  instanceDomain,
		"total":     len(urls),
		"processed": processedCount,
	}).Info("Sync from instance completed")

	return nil
}

// SyncWithAllInstances synchronizes with all active instances
func (s *SyncService) SyncWithAllInstances(ctx context.Context) error {
	s.logger.Debug("Starting sync with all instances")

	// Get all active instances
	instances, err := s.getActiveInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to get active instances: %w", err)
	}

	if len(instances) == 0 {
		s.logger.Debug("No active instances to sync with")
		return nil
	}

	// Sync with each instance in parallel
	var wg sync.WaitGroup
	statusChan := make(chan *SyncStatus, len(instances))

	for _, instance := range instances {
		wg.Add(1)
		go func(inst *FederatedInstance) {
			defer wg.Done()

			status := &SyncStatus{
				InstanceDomain: inst.Domain,
				LastSync:       time.Now(),
				Status:         "success",
			}

			if err := s.syncWithSingleInstance(ctx, inst, status); err != nil {
				status.Status = "error"
				status.Error = err.Error()
				s.logger.WithError(err).WithField("instance", inst.Domain).Error("Sync failed")
			}

			statusChan <- status
		}(instance)
	}

	wg.Wait()
	close(statusChan)

	// Collect sync results
	var statuses []*SyncStatus
	successCount := 0
	for status := range statusChan {
		statuses = append(statuses, status)
		if status.Status == "success" {
			successCount++
		}
	}

	s.logger.WithFields(logrus.Fields{
		"total":     len(instances),
		"succeeded": successCount,
		"failed":    len(instances) - successCount,
	}).Info("Sync with all instances completed")

	if successCount == 0 {
		return fmt.Errorf("failed to sync with any instance")
	}

	return nil
}

// syncWithSingleInstance syncs with a single instance and updates status
func (s *SyncService) syncWithSingleInstance(ctx context.Context, instance *FederatedInstance, status *SyncStatus) error {
	// Skip blocked or sync-disabled instances
	if instance.Blocked || !instance.SyncEnabled {
		return fmt.Errorf("instance sync disabled or blocked")
	}

	// Fetch URLs from instance
	urls, err := s.client.FetchURLs(ctx, instance)
	if err != nil {
		return fmt.Errorf("failed to fetch URLs: %w", err)
	}

	// Process each URL
	processedCount := 0
	for _, url := range urls {
		if err := s.processFederatedURL(ctx, instance, url); err != nil {
			s.logger.WithError(err).WithField("url", url.ShortCode).Debug("Failed to process federated URL")
			continue
		}
		processedCount++
	}

	status.URLsReceived = processedCount

	// Share our URLs with the instance if sharing is enabled
	if s.config.SharePublicURLs {
		sharedCount, err := s.shareURLsWithInstance(ctx, instance)
		if err != nil {
			s.logger.WithError(err).WithField("instance", instance.Domain).Warn("Failed to share URLs with instance")
		} else {
			status.URLsShared = sharedCount
		}
	}

	// Update last sync time
	now := time.Now()
	if err := s.updateInstanceLastSync(ctx, instance.ID, &now); err != nil {
		s.logger.WithError(err).Warn("Failed to update last sync time")
	}

	return nil
}

// shareURLsWithInstance shares our URLs with a specific instance
func (s *SyncService) shareURLsWithInstance(ctx context.Context, instance *FederatedInstance) (int, error) {
	// Get URLs to share (created since last sync)
	urls, err := s.getURLsToShare(ctx, instance.LastSync)
	if err != nil {
		return 0, fmt.Errorf("failed to get URLs to share: %w", err)
	}

	if len(urls) == 0 {
		return 0, nil // No URLs to share
	}

	// Convert to map format expected by client
	urlData := make([]map[string]interface{}, len(urls))
	for i, url := range urls {
		urlData[i] = map[string]interface{}{
			"original_url": url.OriginalURL,
			"short_code":   url.ShortCode,
			"title":        url.Title,
			"created_at":   url.CreatedAt,
		}
	}

	// Use batch sharing if available, otherwise share individually
	if len(urlData) > 1 {
		if err := s.client.BatchShareURLs(ctx, instance, urlData); err != nil {
			s.logger.WithError(err).Debug("Batch sharing failed, falling back to individual sharing")
			// Fallback to individual sharing
			successCount := 0
			for _, data := range urlData {
				if err := s.client.ShareURL(ctx, instance, data); err != nil {
					s.logger.WithError(err).WithField("url", data["short_code"]).Debug("Failed to share individual URL")
					continue
				}
				successCount++
			}
			return successCount, nil
		}
	} else if len(urlData) == 1 {
		if err := s.client.ShareURL(ctx, instance, urlData[0]); err != nil {
			return 0, err
		}
	}

	return len(urlData), nil
}

// processFederatedURL processes a URL received from federation
func (s *SyncService) processFederatedURL(ctx context.Context, instance *FederatedInstance, url *FederatedURL) error {
	// Check if we already have this URL
	exists, err := s.federatedURLExists(ctx, instance.Domain, url.OriginalID)
	if err != nil {
		return fmt.Errorf("failed to check if URL exists: %w", err)
	}

	if exists {
		s.logger.WithField("url", url.ShortCode).Debug("Federated URL already exists, skipping")
		return nil
	}

	// Insert the federated URL
	if err := s.saveFederatedURL(ctx, url); err != nil {
		return fmt.Errorf("failed to save federated URL: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"instance":   instance.Domain,
		"short_code": url.ShortCode,
		"url":        url.OriginalURL,
	}).Debug("Saved federated URL")

	return nil
}

// getActiveInstances retrieves all active federated instances
func (s *SyncService) getActiveInstances(ctx context.Context) ([]*FederatedInstance, error) {
	query := `
		SELECT id, domain, public_key, discovered_at, last_sync, active, blocked, sync_enabled
		FROM federation_instances
		WHERE active = true AND blocked = false AND sync_enabled = true`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query instances: %w", err)
	}
	defer rows.Close()

	var instances []*FederatedInstance
	for rows.Next() {
		instance := &FederatedInstance{}
		err := rows.Scan(
			&instance.ID, &instance.Domain, &instance.PublicKey,
			&instance.DiscoveredAt, &instance.LastSync,
			&instance.Active, &instance.Blocked, &instance.SyncEnabled,
		)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to scan instance")
			continue
		}
		instances = append(instances, instance)
	}

	return instances, nil
}

// getInstanceByDomain retrieves an instance by domain
func (s *SyncService) getInstanceByDomain(ctx context.Context, domain string) (*FederatedInstance, error) {
	query := `
		SELECT id, domain, public_key, discovered_at, last_sync, active, blocked, sync_enabled
		FROM federation_instances
		WHERE domain = ?`

	row := s.db.QueryRowContext(ctx, query, domain)

	instance := &FederatedInstance{}
	err := row.Scan(
		&instance.ID, &instance.Domain, &instance.PublicKey,
		&instance.DiscoveredAt, &instance.LastSync,
		&instance.Active, &instance.Blocked, &instance.SyncEnabled,
	)

	if err != nil {
		return nil, fmt.Errorf("instance not found: %w", err)
	}

	return instance, nil
}

// federatedURLExists checks if a federated URL already exists
func (s *SyncService) federatedURLExists(ctx context.Context, sourceInstance, originalID string) (bool, error) {
	query := "SELECT 1 FROM federated_urls WHERE source_instance = ? AND original_id = ?"

	var exists int
	err := s.db.QueryRowContext(ctx, query, sourceInstance, originalID).Scan(&exists)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// saveFederatedURL saves a federated URL to the database
func (s *SyncService) saveFederatedURL(ctx context.Context, url *FederatedURL) error {
	query := `
		INSERT OR IGNORE INTO federated_urls (id, original_id, source_instance, original_url, short_code, title, created_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		s.generateFederatedURLID(), url.OriginalID, url.SourceInstance,
		url.OriginalURL, url.ShortCode, url.Title, url.CreatedAt, time.Now(),
	)

	return err
}

// getURLsToShare retrieves URLs that should be shared with federated instances
func (s *SyncService) getURLsToShare(ctx context.Context, since *time.Time) ([]*FederatedURL, error) {
	query := `
		SELECT id, original_url, short_code, title, created_at
		FROM urls
		WHERE active = true AND user_id IS NULL` // Only share anonymous/public URLs

	args := []interface{}{}

	if since != nil {
		query += " AND created_at > ?"
		args = append(args, *since)
	}

	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, s.config.MaxURLsPerSync)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query URLs: %w", err)
	}
	defer rows.Close()

	var urls []*FederatedURL
	for rows.Next() {
		var id, originalURL, shortCode, title string
		var createdAt time.Time

		err := rows.Scan(&id, &originalURL, &shortCode, &title, &createdAt)
		if err != nil {
			continue
		}

		url := &FederatedURL{
			ID:             id,
			OriginalID:     id,
			SourceInstance: s.config.Domain,
			OriginalURL:    originalURL,
			ShortCode:      shortCode,
			Title:          title,
			CreatedAt:      createdAt,
			SyncedAt:       time.Now(),
		}

		urls = append(urls, url)
	}

	return urls, nil
}

// updateInstanceLastSync updates the last sync time for an instance
func (s *SyncService) updateInstanceLastSync(ctx context.Context, instanceID string, lastSync *time.Time) error {
	query := "UPDATE federation_instances SET last_sync = ? WHERE id = ?"
	_, err := s.db.ExecContext(ctx, query, lastSync, instanceID)
	return err
}

// generateFederatedURLID generates a unique ID for federated URLs
func (s *SyncService) generateFederatedURLID() string {
	return fmt.Sprintf("sync_%d", time.Now().UnixNano())
}

// GetSyncStatus returns the sync status for all instances
func (s *SyncService) GetSyncStatus(ctx context.Context) ([]*SyncStatus, error) {
	instances, err := s.getActiveInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %w", err)
	}

	statuses := make([]*SyncStatus, len(instances))
	for i, instance := range instances {
		status := &SyncStatus{
			InstanceDomain: instance.Domain,
			Status:         "idle",
		}

		if instance.LastSync != nil {
			status.LastSync = *instance.LastSync
		}

		// Get URL counts for this instance
		urlCount, err := s.getFederatedURLCount(ctx, instance.Domain)
		if err == nil {
			status.URLsReceived = urlCount
		}

		statuses[i] = status
	}

	return statuses, nil
}

// getFederatedURLCount gets the count of federated URLs from an instance
func (s *SyncService) getFederatedURLCount(ctx context.Context, sourceInstance string) (int, error) {
	query := "SELECT COUNT(*) FROM federated_urls WHERE source_instance = ?"

	var count int
	err := s.db.QueryRowContext(ctx, query, sourceInstance).Scan(&count)
	return count, err
}

// CleanupOldFederatedURLs removes old federated URLs based on retention policy
func (s *SyncService) CleanupOldFederatedURLs(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)

	query := "DELETE FROM federated_urls WHERE synced_at < ?"
	result, err := s.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup old federated URLs: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	s.logger.WithFields(logrus.Fields{
		"cutoff":        cutoff,
		"rows_affected": rowsAffected,
	}).Info("Cleaned up old federated URLs")

	return nil
}