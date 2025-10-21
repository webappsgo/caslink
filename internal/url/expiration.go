package url

import (
	"context"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// ExpirationManager handles URL expiration logic
type ExpirationManager struct {
	db     *db.DB
	logger *logrus.Logger
}

// NewExpirationManager creates a new expiration manager
func NewExpirationManager(database *db.DB, logger *logrus.Logger) *ExpirationManager {
	return &ExpirationManager{
		db:     database,
		logger: logger,
	}
}

// CheckExpired checks if a URL has expired
func (m *ExpirationManager) CheckExpired(urlRecord *db.URL) bool {
	if urlRecord.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*urlRecord.ExpiresAt)
}

// GetTimeUntilExpiration returns the duration until expiration
func (m *ExpirationManager) GetTimeUntilExpiration(urlRecord *db.URL) *time.Duration {
	if urlRecord.ExpiresAt == nil {
		return nil // Never expires
	}

	duration := time.Until(*urlRecord.ExpiresAt)
	return &duration
}

// GetExpirationStatus returns detailed expiration status
func (m *ExpirationManager) GetExpirationStatus(urlRecord *db.URL) ExpirationStatus {
	if urlRecord.ExpiresAt == nil {
		return ExpirationStatus{
			IsExpired:    false,
			ExpiresAt:    nil,
			TimeLeft:     nil,
			Status:       "never_expires",
			StatusText:   "Never expires",
		}
	}

	now := time.Now()
	expiresAt := *urlRecord.ExpiresAt
	timeLeft := time.Until(expiresAt)

	if now.After(expiresAt) {
		return ExpirationStatus{
			IsExpired:    true,
			ExpiresAt:    &expiresAt,
			TimeLeft:     nil,
			Status:       "expired",
			StatusText:   fmt.Sprintf("Expired %s ago", formatDuration(time.Since(expiresAt))),
		}
	}

	// Determine status based on time left
	var status, statusText string
	switch {
	case timeLeft <= time.Hour:
		status = "expiring_soon"
		statusText = fmt.Sprintf("Expires in %s", formatDuration(timeLeft))
	case timeLeft <= 24*time.Hour:
		status = "expiring_today"
		statusText = fmt.Sprintf("Expires in %s", formatDuration(timeLeft))
	case timeLeft <= 7*24*time.Hour:
		status = "expiring_this_week"
		statusText = fmt.Sprintf("Expires in %s", formatDuration(timeLeft))
	default:
		status = "active"
		statusText = fmt.Sprintf("Expires in %s", formatDuration(timeLeft))
	}

	return ExpirationStatus{
		IsExpired:    false,
		ExpiresAt:    &expiresAt,
		TimeLeft:     &timeLeft,
		Status:       status,
		StatusText:   statusText,
	}
}

// CleanupExpiredURLs removes or deactivates expired URLs
func (m *ExpirationManager) CleanupExpiredURLs(ctx context.Context, deleteMode bool) (*CleanupResult, error) {
	result := &CleanupResult{
		StartTime: time.Now(),
	}

	// Find expired URLs
	query := `
		SELECT id, original_url, expires_at, created_at, clicks
		FROM urls
		WHERE expires_at IS NOT NULL
		  AND expires_at <= ?
		  AND active = true
	`

	if m.db.Type() == "postgres" {
		query = `
			SELECT id, original_url, expires_at, created_at, clicks
			FROM urls
			WHERE expires_at IS NOT NULL
			  AND expires_at <= $1
			  AND active = true
		`
	}

	rows, err := m.db.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to query expired URLs: %w", err)
	}
	defer rows.Close()

	var expiredURLs []ExpiredURL
	for rows.Next() {
		var expired ExpiredURL
		err := rows.Scan(
			&expired.ID,
			&expired.OriginalURL,
			&expired.ExpiresAt,
			&expired.CreatedAt,
			&expired.Clicks,
		)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to scan expired URL: %v", err))
			continue
		}
		expiredURLs = append(expiredURLs, expired)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate expired URLs: %w", err)
	}

	result.Found = len(expiredURLs)

	// Process expired URLs
	for _, expired := range expiredURLs {
		if deleteMode {
			err := m.deleteExpiredURL(ctx, expired.ID)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to delete URL %s: %v", expired.ID, err))
				result.Failed++
			} else {
				result.Deleted++
			}
		} else {
			err := m.deactivateExpiredURL(ctx, expired.ID)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to deactivate URL %s: %v", expired.ID, err))
				result.Failed++
			} else {
				result.Deactivated++
			}
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	m.logger.WithFields(logrus.Fields{
		"found":       result.Found,
		"deleted":     result.Deleted,
		"deactivated": result.Deactivated,
		"failed":      result.Failed,
		"duration":    result.Duration,
		"delete_mode": deleteMode,
	}).Info("Expired URL cleanup completed")

	return result, nil
}

// deleteExpiredURL permanently deletes an expired URL
func (m *ExpirationManager) deleteExpiredURL(ctx context.Context, urlID string) error {
	query := "DELETE FROM urls WHERE id = ?"
	if m.db.Type() == "postgres" {
		query = "DELETE FROM urls WHERE id = $1"
	}

	_, err := m.db.Exec(ctx, query, urlID)
	return err
}

// deactivateExpiredURL marks an expired URL as inactive
func (m *ExpirationManager) deactivateExpiredURL(ctx context.Context, urlID string) error {
	query := "UPDATE urls SET active = false, updated_at = ? WHERE id = ?"
	if m.db.Type() == "postgres" {
		query = "UPDATE urls SET active = false, updated_at = $1 WHERE id = $2"
	}

	_, err := m.db.Exec(ctx, query, time.Now(), urlID)
	return err
}

// GetExpiringURLs returns URLs that will expire within the specified duration
func (m *ExpirationManager) GetExpiringURLs(ctx context.Context, within time.Duration) ([]*db.URL, error) {
	expiryThreshold := time.Now().Add(within)

	query := `
		SELECT id, original_url, is_custom, title, description, favicon_url,
		       created_at, updated_at, expires_at, clicks, unique_clicks,
		       user_id, domain_id, active, tags
		FROM urls
		WHERE expires_at IS NOT NULL
		  AND expires_at <= ?
		  AND expires_at > ?
		  AND active = true
		ORDER BY expires_at ASC
	`

	if m.db.Type() == "postgres" {
		query = `
			SELECT id, original_url, is_custom, title, description, favicon_url,
			       created_at, updated_at, expires_at, clicks, unique_clicks,
			       user_id, domain_id, active, tags
			FROM urls
			WHERE expires_at IS NOT NULL
			  AND expires_at <= $1
			  AND expires_at > $2
			  AND active = true
			ORDER BY expires_at ASC
		`
	}

	rows, err := m.db.Query(ctx, query, expiryThreshold, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to query expiring URLs: %w", err)
	}
	defer rows.Close()

	var urls []*db.URL
	for rows.Next() {
		url := &db.URL{}
		var title, description, faviconURL, userID, domainID, tags *string
		var expiresAt *time.Time

		err := rows.Scan(
			&url.ID,
			&url.OriginalURL,
			&url.IsCustom,
			&title,
			&description,
			&faviconURL,
			&url.CreatedAt,
			&url.UpdatedAt,
			&expiresAt,
			&url.Clicks,
			&url.UniqueClicks,
			&userID,
			&domainID,
			&url.Active,
			&tags,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan URL: %w", err)
		}

		// Set nullable fields
		url.Title = title
		url.Description = description
		url.FaviconURL = faviconURL
		url.UserID = userID
		url.DomainID = domainID
		url.Tags = tags
		url.ExpiresAt = expiresAt

		urls = append(urls, url)
	}

	return urls, rows.Err()
}

// ExtendExpiration extends the expiration time of a URL
func (m *ExpirationManager) ExtendExpiration(ctx context.Context, urlID string, extension time.Duration) error {
	// Get current URL
	query := "SELECT expires_at FROM urls WHERE id = ?"
	if m.db.Type() == "postgres" {
		query = "SELECT expires_at FROM urls WHERE id = $1"
	}

	var currentExpiry *time.Time
	err := m.db.QueryRow(ctx, query, urlID).Scan(&currentExpiry)
	if err != nil {
		return fmt.Errorf("failed to get current expiration: %w", err)
	}

	// Calculate new expiry time
	var newExpiry time.Time
	if currentExpiry != nil {
		newExpiry = currentExpiry.Add(extension)
	} else {
		newExpiry = time.Now().Add(extension)
	}

	// Update expiration
	updateQuery := "UPDATE urls SET expires_at = ?, updated_at = ? WHERE id = ?"
	if m.db.Type() == "postgres" {
		updateQuery = "UPDATE urls SET expires_at = $1, updated_at = $2 WHERE id = $3"
	}

	_, err = m.db.Exec(ctx, updateQuery, newExpiry, time.Now(), urlID)
	if err != nil {
		return fmt.Errorf("failed to update expiration: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"url_id":     urlID,
		"extension":  extension,
		"new_expiry": newExpiry,
	}).Info("URL expiration extended")

	return nil
}

// RemoveExpiration removes the expiration from a URL (makes it permanent)
func (m *ExpirationManager) RemoveExpiration(ctx context.Context, urlID string) error {
	query := "UPDATE urls SET expires_at = NULL, updated_at = ? WHERE id = ?"
	if m.db.Type() == "postgres" {
		query = "UPDATE urls SET expires_at = NULL, updated_at = $1 WHERE id = $2"
	}

	_, err := m.db.Exec(ctx, query, time.Now(), urlID)
	if err != nil {
		return fmt.Errorf("failed to remove expiration: %w", err)
	}

	m.logger.WithField("url_id", urlID).Info("URL expiration removed")
	return nil
}

// GetExpirationStats returns statistics about URL expirations
func (m *ExpirationManager) GetExpirationStats(ctx context.Context) (*ExpirationStats, error) {
	stats := &ExpirationStats{}

	// Count total URLs
	err := m.db.QueryRow(ctx, "SELECT COUNT(*) FROM urls WHERE active = true").Scan(&stats.TotalActiveURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to count total URLs: %w", err)
	}

	// Count URLs with expiration
	err = m.db.QueryRow(ctx, "SELECT COUNT(*) FROM urls WHERE expires_at IS NOT NULL AND active = true").Scan(&stats.URLsWithExpiration)
	if err != nil {
		return nil, fmt.Errorf("failed to count URLs with expiration: %w", err)
	}

	// Count expired URLs
	query := "SELECT COUNT(*) FROM urls WHERE expires_at IS NOT NULL AND expires_at <= ? AND active = true"
	if m.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM urls WHERE expires_at IS NOT NULL AND expires_at <= $1 AND active = true"
	}
	err = m.db.QueryRow(ctx, query, time.Now()).Scan(&stats.ExpiredURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to count expired URLs: %w", err)
	}

	// Count URLs expiring in next 24 hours
	tomorrow := time.Now().Add(24 * time.Hour)
	query = "SELECT COUNT(*) FROM urls WHERE expires_at IS NOT NULL AND expires_at <= ? AND expires_at > ? AND active = true"
	if m.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM urls WHERE expires_at IS NOT NULL AND expires_at <= $1 AND expires_at > $2 AND active = true"
	}
	err = m.db.QueryRow(ctx, query, tomorrow, time.Now()).Scan(&stats.ExpiringIn24Hours)
	if err != nil {
		return nil, fmt.Errorf("failed to count URLs expiring in 24 hours: %w", err)
	}

	// Count URLs expiring in next 7 days
	nextWeek := time.Now().Add(7 * 24 * time.Hour)
	query = "SELECT COUNT(*) FROM urls WHERE expires_at IS NOT NULL AND expires_at <= ? AND expires_at > ? AND active = true"
	if m.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM urls WHERE expires_at IS NOT NULL AND expires_at <= $1 AND expires_at > $2 AND active = true"
	}
	err = m.db.QueryRow(ctx, query, nextWeek, time.Now()).Scan(&stats.ExpiringIn7Days)
	if err != nil {
		return nil, fmt.Errorf("failed to count URLs expiring in 7 days: %w", err)
	}

	stats.NeverExpireURLs = stats.TotalActiveURLs - stats.URLsWithExpiration

	return stats, nil
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}

	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}

	if hours > 0 {
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}

	if minutes > 0 {
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}

	return "less than a minute"
}

// ExpirationStatus represents the expiration status of a URL
type ExpirationStatus struct {
	IsExpired  bool           `json:"is_expired"`
	ExpiresAt  *time.Time     `json:"expires_at"`
	TimeLeft   *time.Duration `json:"time_left"`
	Status     string         `json:"status"`
	StatusText string         `json:"status_text"`
}

// ExpiredURL represents an expired URL
type ExpiredURL struct {
	ID          string    `json:"id"`
	OriginalURL string    `json:"original_url"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	Clicks      int64     `json:"clicks"`
}

// CleanupResult represents the result of cleanup operation
type CleanupResult struct {
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration"`
	Found        int           `json:"found"`
	Deleted      int           `json:"deleted"`
	Deactivated  int           `json:"deactivated"`
	Failed       int           `json:"failed"`
	Errors       []string      `json:"errors"`
}

// ExpirationStats represents expiration statistics
type ExpirationStats struct {
	TotalActiveURLs     int64 `json:"total_active_urls"`
	URLsWithExpiration  int64 `json:"urls_with_expiration"`
	NeverExpireURLs     int64 `json:"never_expire_urls"`
	ExpiredURLs         int64 `json:"expired_urls"`
	ExpiringIn24Hours   int64 `json:"expiring_in_24_hours"`
	ExpiringIn7Days     int64 `json:"expiring_in_7_days"`
}