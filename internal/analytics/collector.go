package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Collector handles click data collection and storage
type Collector struct {
	db     *db.DB
	config *config.AnalyticsConfig
	logger *logrus.Logger
}

// NewCollector creates a new analytics collector
func NewCollector(database *db.DB, cfg *config.AnalyticsConfig, logger *logrus.Logger) (*Collector, error) {
	return &Collector{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// RecordClick stores a click event in the database
func (c *Collector) RecordClick(ctx context.Context, click *Click) error {
	query := `
		INSERT INTO clicks (id, url_id, clicked_at, ip_address, ip_hash, user_agent,
		                   referrer, referrer_domain, is_unique, is_bot,
		                   country_code, country_name, region, city, latitude, longitude, timezone,
		                   browser, browser_version, os, os_version, device_type, device_brand, device_model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	if c.db.Type() == "postgres" {
		query = `
			INSERT INTO clicks (id, url_id, clicked_at, ip_address, ip_hash, user_agent,
			                   referrer, referrer_domain, is_unique, is_bot,
			                   country_code, country_name, region, city, latitude, longitude, timezone,
			                   browser, browser_version, os, os_version, device_type, device_brand, device_model)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
		`
	}

	// Extract location and device info
	var countryCode, countryName, region, city, timezone *string
	var latitude, longitude *float64
	var browser, browserVersion, os, osVersion, deviceType, deviceBrand, deviceModel *string

	if click.Location != nil {
		if click.Location.CountryCode != "" {
			countryCode = &click.Location.CountryCode
		}
		if click.Location.CountryName != "" {
			countryName = &click.Location.CountryName
		}
		if click.Location.Region != "" {
			region = &click.Location.Region
		}
		if click.Location.City != "" {
			city = &click.Location.City
		}
		if click.Location.Timezone != "" {
			timezone = &click.Location.Timezone
		}
		if click.Location.Latitude != 0 {
			latitude = &click.Location.Latitude
		}
		if click.Location.Longitude != 0 {
			longitude = &click.Location.Longitude
		}
	}

	if click.DeviceInfo != nil {
		if click.DeviceInfo.Browser != "" {
			browser = &click.DeviceInfo.Browser
		}
		if click.DeviceInfo.BrowserVersion != "" {
			browserVersion = &click.DeviceInfo.BrowserVersion
		}
		if click.DeviceInfo.OS != "" {
			os = &click.DeviceInfo.OS
		}
		if click.DeviceInfo.OSVersion != "" {
			osVersion = &click.DeviceInfo.OSVersion
		}
		if click.DeviceInfo.DeviceType != "" {
			deviceType = &click.DeviceInfo.DeviceType
		}
		if click.DeviceInfo.DeviceBrand != "" {
			deviceBrand = &click.DeviceInfo.DeviceBrand
		}
		if click.DeviceInfo.DeviceModel != "" {
			deviceModel = &click.DeviceInfo.DeviceModel
		}
	}

	_, err := c.db.Exec(ctx, query,
		click.ID,
		click.URLID,
		click.ClickedAt,
		click.IPAddress,
		click.IPHash,
		click.UserAgent,
		nullString(click.Referrer),
		nullString(click.ReferrerDomain),
		click.IsUnique,
		click.IsBot,
		countryCode,
		countryName,
		region,
		city,
		latitude,
		longitude,
		timezone,
		browser,
		browserVersion,
		os,
		osVersion,
		deviceType,
		deviceBrand,
		deviceModel,
	)

	return err
}

// IsUniqueClick checks if this is the first click from this IP for this URL
func (c *Collector) IsUniqueClick(ctx context.Context, urlID, ipHash string) (bool, error) {
	query := `
		SELECT 1 FROM clicks
		WHERE url_id = ? AND ip_hash = ?
		LIMIT 1
	`

	if c.db.Type() == "postgres" {
		query = `
			SELECT 1 FROM clicks
			WHERE url_id = $1 AND ip_hash = $2
			LIMIT 1
		`
	}

	var exists int
	err := c.db.QueryRow(ctx, query, urlID, ipHash).Scan(&exists)
	if err != nil {
		if err == db.ErrNoRows {
			return true, nil // No previous click found, so it's unique
		}
		return false, err
	}

	return false, nil // Previous click found, so it's not unique
}

// GetURLAnalytics retrieves analytics data for a specific URL
func (c *Collector) GetURLAnalytics(ctx context.Context, urlID string, startDate, endDate time.Time) (*URLAnalytics, error) {
	analytics := &URLAnalytics{
		URLID: urlID,
	}

	// Get total and unique click counts
	countQuery := `
		SELECT
			COUNT(*) as total_clicks,
			COUNT(DISTINCT CASE WHEN is_unique = true THEN ip_hash END) as unique_clicks
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at <= ? AND is_bot = false
	`

	if c.db.Type() == "postgres" {
		countQuery = `
			SELECT
				COUNT(*) as total_clicks,
				COUNT(DISTINCT CASE WHEN is_unique = true THEN ip_hash END) as unique_clicks
			FROM clicks
			WHERE url_id = $1 AND clicked_at >= $2 AND clicked_at <= $3 AND is_bot = false
		`
	}

	err := c.db.QueryRow(ctx, countQuery, urlID, startDate, endDate).Scan(
		&analytics.TotalClicks,
		&analytics.UniqueClicks,
	)
	if err != nil && err != db.ErrNoRows {
		return nil, fmt.Errorf("failed to get click counts: %w", err)
	}

	// Get recent clicks
	recentQuery := `
		SELECT id, clicked_at, ip_address, user_agent, referrer, referrer_domain,
		       country_code, country_name, browser, device_type
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at <= ? AND is_bot = false
		ORDER BY clicked_at DESC
		LIMIT 100
	`

	if c.db.Type() == "postgres" {
		recentQuery = `
			SELECT id, clicked_at, ip_address, user_agent, referrer, referrer_domain,
			       country_code, country_name, browser, device_type
			FROM clicks
			WHERE url_id = $1 AND clicked_at >= $2 AND clicked_at <= $3 AND is_bot = false
			ORDER BY clicked_at DESC
			LIMIT 100
		`
	}

	rows, err := c.db.Query(ctx, recentQuery, urlID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent clicks: %w", err)
	}
	defer rows.Close()

	var recentClicks []Click
	for rows.Next() {
		var click Click
		var countryCode, countryName, browser, deviceType *string

		err := rows.Scan(
			&click.ID,
			&click.ClickedAt,
			&click.IPAddress,
			&click.UserAgent,
			&click.Referrer,
			&click.ReferrerDomain,
			&countryCode,
			&countryName,
			&browser,
			&deviceType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan click: %w", err)
		}

		click.URLID = urlID

		// Set location data if available
		if countryCode != nil || countryName != nil {
			click.Location = &LocationData{
				CountryCode: stringValue(countryCode),
				CountryName: stringValue(countryName),
			}
		}

		// Set device data if available
		if browser != nil || deviceType != nil {
			click.DeviceInfo = &DeviceInfo{
				Browser:    stringValue(browser),
				DeviceType: stringValue(deviceType),
			}
		}

		recentClicks = append(recentClicks, click)
	}

	analytics.RecentClicks = recentClicks
	return analytics, rows.Err()
}

// GetUserAnalytics retrieves analytics data for all URLs belonging to a user
func (c *Collector) GetUserAnalytics(ctx context.Context, userID string, startDate, endDate time.Time) (*UserAnalytics, error) {
	analytics := &UserAnalytics{
		UserID: userID,
	}

	// Get user's URL analytics
	query := `
		SELECT
			COUNT(DISTINCT c.url_id) as total_urls,
			COUNT(*) as total_clicks,
			COUNT(DISTINCT CASE WHEN c.is_unique = true THEN c.ip_hash END) as unique_clicks
		FROM clicks c
		JOIN urls u ON c.url_id = u.id
		WHERE u.user_id = ? AND c.clicked_at >= ? AND c.clicked_at <= ? AND c.is_bot = false
	`

	if c.db.Type() == "postgres" {
		query = `
			SELECT
				COUNT(DISTINCT c.url_id) as total_urls,
				COUNT(*) as total_clicks,
				COUNT(DISTINCT CASE WHEN c.is_unique = true THEN c.ip_hash END) as unique_clicks
			FROM clicks c
			JOIN urls u ON c.url_id = u.id
			WHERE u.user_id = $1 AND c.clicked_at >= $2 AND c.clicked_at <= $3 AND c.is_bot = false
		`
	}

	err := c.db.QueryRow(ctx, query, userID, startDate, endDate).Scan(
		&analytics.TotalURLs,
		&analytics.TotalClicks,
		&analytics.UniqueClicks,
	)
	if err != nil && err != db.ErrNoRows {
		return nil, fmt.Errorf("failed to get user analytics: %w", err)
	}

	return analytics, nil
}

// GetGlobalAnalytics retrieves global analytics data
func (c *Collector) GetGlobalAnalytics(ctx context.Context, startDate, endDate time.Time) (*GlobalAnalytics, error) {
	analytics := &GlobalAnalytics{}

	// Get global counts
	query := `
		SELECT
			COUNT(DISTINCT c.url_id) as total_urls,
			COUNT(DISTINCT u.user_id) as total_users,
			COUNT(*) as total_clicks,
			COUNT(DISTINCT CASE WHEN c.is_unique = true THEN c.ip_hash END) as unique_clicks
		FROM clicks c
		LEFT JOIN urls u ON c.url_id = u.id
		WHERE c.clicked_at >= ? AND c.clicked_at <= ? AND c.is_bot = false
	`

	if c.db.Type() == "postgres" {
		query = `
			SELECT
				COUNT(DISTINCT c.url_id) as total_urls,
				COUNT(DISTINCT u.user_id) as total_users,
				COUNT(*) as total_clicks,
				COUNT(DISTINCT CASE WHEN c.is_unique = true THEN c.ip_hash END) as unique_clicks
			FROM clicks c
			LEFT JOIN urls u ON c.url_id = u.id
			WHERE c.clicked_at >= $1 AND c.clicked_at <= $2 AND c.is_bot = false
		`
	}

	err := c.db.QueryRow(ctx, query, startDate, endDate).Scan(
		&analytics.TotalURLs,
		&analytics.TotalUsers,
		&analytics.TotalClicks,
		&analytics.UniqueClicks,
	)
	if err != nil && err != db.ErrNoRows {
		return nil, fmt.Errorf("failed to get global analytics: %w", err)
	}

	return analytics, nil
}

// GetTopCountries retrieves top countries by click count
func (c *Collector) GetTopCountries(ctx context.Context, urlID string, startDate, endDate time.Time, limit int) ([]CountryStat, error) {
	query := `
		SELECT country_code, country_name, COUNT(*) as clicks
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at <= ?
		  AND country_code IS NOT NULL AND is_bot = false
		GROUP BY country_code, country_name
		ORDER BY clicks DESC
		LIMIT ?
	`

	if c.db.Type() == "postgres" {
		query = `
			SELECT country_code, country_name, COUNT(*) as clicks
			FROM clicks
			WHERE url_id = $1 AND clicked_at >= $2 AND clicked_at <= $3
			  AND country_code IS NOT NULL AND is_bot = false
			GROUP BY country_code, country_name
			ORDER BY clicks DESC
			LIMIT $4
		`
	}

	rows, err := c.db.Query(ctx, query, urlID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top countries: %w", err)
	}
	defer rows.Close()

	var countries []CountryStat
	var totalClicks int64

	for rows.Next() {
		var country CountryStat
		err := rows.Scan(&country.CountryCode, &country.CountryName, &country.Clicks)
		if err != nil {
			return nil, fmt.Errorf("failed to scan country: %w", err)
		}
		countries = append(countries, country)
		totalClicks += country.Clicks
	}

	// Calculate percentages
	for i := range countries {
		if totalClicks > 0 {
			countries[i].Percentage = float64(countries[i].Clicks) / float64(totalClicks) * 100
		}
	}

	return countries, rows.Err()
}

// GetTopReferrers retrieves top referrers by click count
func (c *Collector) GetTopReferrers(ctx context.Context, urlID string, startDate, endDate time.Time, limit int) ([]ReferrerStat, error) {
	query := `
		SELECT referrer_domain, COUNT(*) as clicks
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at <= ?
		  AND referrer_domain IS NOT NULL AND referrer_domain != '' AND is_bot = false
		GROUP BY referrer_domain
		ORDER BY clicks DESC
		LIMIT ?
	`

	if c.db.Type() == "postgres" {
		query = `
			SELECT referrer_domain, COUNT(*) as clicks
			FROM clicks
			WHERE url_id = $1 AND clicked_at >= $2 AND clicked_at <= $3
			  AND referrer_domain IS NOT NULL AND referrer_domain != '' AND is_bot = false
			GROUP BY referrer_domain
			ORDER BY clicks DESC
			LIMIT $4
		`
	}

	rows, err := c.db.Query(ctx, query, urlID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top referrers: %w", err)
	}
	defer rows.Close()

	var referrers []ReferrerStat
	var totalClicks int64

	for rows.Next() {
		var referrer ReferrerStat
		err := rows.Scan(&referrer.Domain, &referrer.Clicks)
		if err != nil {
			return nil, fmt.Errorf("failed to scan referrer: %w", err)
		}
		referrers = append(referrers, referrer)
		totalClicks += referrer.Clicks
	}

	// Calculate percentages
	for i := range referrers {
		if totalClicks > 0 {
			referrers[i].Percentage = float64(referrers[i].Clicks) / float64(totalClicks) * 100
		}
	}

	return referrers, rows.Err()
}

// GetTopBrowsers retrieves top browsers by click count
func (c *Collector) GetTopBrowsers(ctx context.Context, urlID string, startDate, endDate time.Time, limit int) ([]BrowserStat, error) {
	query := `
		SELECT browser, COUNT(*) as clicks
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at <= ?
		  AND browser IS NOT NULL AND is_bot = false
		GROUP BY browser
		ORDER BY clicks DESC
		LIMIT ?
	`

	if c.db.Type() == "postgres" {
		query = `
			SELECT browser, COUNT(*) as clicks
			FROM clicks
			WHERE url_id = $1 AND clicked_at >= $2 AND clicked_at <= $3
			  AND browser IS NOT NULL AND is_bot = false
			GROUP BY browser
			ORDER BY clicks DESC
			LIMIT $4
		`
	}

	rows, err := c.db.Query(ctx, query, urlID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top browsers: %w", err)
	}
	defer rows.Close()

	var browsers []BrowserStat
	var totalClicks int64

	for rows.Next() {
		var browser BrowserStat
		err := rows.Scan(&browser.Browser, &browser.Clicks)
		if err != nil {
			return nil, fmt.Errorf("failed to scan browser: %w", err)
		}
		browsers = append(browsers, browser)
		totalClicks += browser.Clicks
	}

	// Calculate percentages
	for i := range browsers {
		if totalClicks > 0 {
			browsers[i].Percentage = float64(browsers[i].Clicks) / float64(totalClicks) * 100
		}
	}

	return browsers, rows.Err()
}

// GetTopDevices retrieves top device types by click count
func (c *Collector) GetTopDevices(ctx context.Context, urlID string, startDate, endDate time.Time, limit int) ([]DeviceStat, error) {
	query := `
		SELECT device_type, COUNT(*) as clicks
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at <= ?
		  AND device_type IS NOT NULL AND is_bot = false
		GROUP BY device_type
		ORDER BY clicks DESC
		LIMIT ?
	`

	if c.db.Type() == "postgres" {
		query = `
			SELECT device_type, COUNT(*) as clicks
			FROM clicks
			WHERE url_id = $1 AND clicked_at >= $2 AND clicked_at <= $3
			  AND device_type IS NOT NULL AND is_bot = false
			GROUP BY device_type
			ORDER BY clicks DESC
			LIMIT $4
		`
	}

	rows, err := c.db.Query(ctx, query, urlID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top devices: %w", err)
	}
	defer rows.Close()

	var devices []DeviceStat
	var totalClicks int64

	for rows.Next() {
		var device DeviceStat
		err := rows.Scan(&device.DeviceType, &device.Clicks)
		if err != nil {
			return nil, fmt.Errorf("failed to scan device: %w", err)
		}
		devices = append(devices, device)
		totalClicks += device.Clicks
	}

	// Calculate percentages
	for i := range devices {
		if totalClicks > 0 {
			devices[i].Percentage = float64(devices[i].Clicks) / float64(totalClicks) * 100
		}
	}

	return devices, rows.Err()
}

// GetRealTimeStats retrieves real-time statistics
func (c *Collector) GetRealTimeStats(ctx context.Context, timeWindow time.Duration) (*RealTimeStatsResponse, error) {
	windowStart := time.Now().Add(-timeWindow)

	stats := &RealTimeStatsResponse{
		TimeWindow: timeWindow,
	}

	// Get clicks in time window
	clickQuery := `
		SELECT COUNT(*) FROM clicks
		WHERE clicked_at >= ? AND is_bot = false
	`

	if c.db.Type() == "postgres" {
		clickQuery = `
			SELECT COUNT(*) FROM clicks
			WHERE clicked_at >= $1 AND is_bot = false
		`
	}

	err := c.db.QueryRow(ctx, clickQuery, windowStart).Scan(&stats.ClicksInWindow)
	if err != nil && err != db.ErrNoRows {
		return nil, fmt.Errorf("failed to get clicks in window: %w", err)
	}

	// Get active URLs
	urlQuery := `
		SELECT COUNT(DISTINCT url_id) FROM clicks
		WHERE clicked_at >= ? AND is_bot = false
	`

	if c.db.Type() == "postgres" {
		urlQuery = `
			SELECT COUNT(DISTINCT url_id) FROM clicks
			WHERE clicked_at >= $1 AND is_bot = false
		`
	}

	err = c.db.QueryRow(ctx, urlQuery, windowStart).Scan(&stats.ActiveURLs)
	if err != nil && err != db.ErrNoRows {
		return nil, fmt.Errorf("failed to get active URLs: %w", err)
	}

	// Get recent clicks
	recentQuery := `
		SELECT id, url_id, clicked_at, country_code, browser, device_type
		FROM clicks
		WHERE clicked_at >= ? AND is_bot = false
		ORDER BY clicked_at DESC
		LIMIT 50
	`

	if c.db.Type() == "postgres" {
		recentQuery = `
			SELECT id, url_id, clicked_at, country_code, browser, device_type
			FROM clicks
			WHERE clicked_at >= $1 AND is_bot = false
			ORDER BY clicked_at DESC
			LIMIT 50
		`
	}

	rows, err := c.db.Query(ctx, recentQuery, windowStart)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent clicks: %w", err)
	}
	defer rows.Close()

	var recentClicks []Click
	for rows.Next() {
		var click Click
		var countryCode, browser, deviceType *string

		err := rows.Scan(
			&click.ID,
			&click.URLID,
			&click.ClickedAt,
			&countryCode,
			&browser,
			&deviceType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan recent click: %w", err)
		}

		if countryCode != nil {
			click.Location = &LocationData{CountryCode: *countryCode}
		}

		if browser != nil || deviceType != nil {
			click.DeviceInfo = &DeviceInfo{
				Browser:    stringValue(browser),
				DeviceType: stringValue(deviceType),
			}
		}

		recentClicks = append(recentClicks, click)
	}

	stats.RecentClicks = recentClicks
	return stats, rows.Err()
}

// GetExportData retrieves data for export
func (c *Collector) GetExportData(ctx context.Context, req *ExportRequest) ([]map[string]interface{}, error) {
	// Build query based on request
	selectFields := []string{
		"c.id", "c.url_id", "c.clicked_at", "c.ip_address", "c.user_agent",
		"c.referrer", "c.referrer_domain", "c.country_code", "c.country_name",
		"c.browser", "c.device_type", "c.is_unique", "c.is_bot",
	}

	if len(req.Fields) > 0 {
		selectFields = req.Fields
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM clicks c
		LEFT JOIN urls u ON c.url_id = u.id
		WHERE c.clicked_at >= ? AND c.clicked_at <= ? AND c.is_bot = false
	`, strings.Join(selectFields, ", "))

	args := []interface{}{req.StartDate, req.EndDate}
	argIndex := 3

	if req.UserID != nil {
		query += fmt.Sprintf(" AND u.user_id = ?")
		if c.db.Type() == "postgres" {
			query = strings.ReplaceAll(query, "?", fmt.Sprintf("$%d", argIndex))
			argIndex++
		}
		args = append(args, *req.UserID)
	}

	if len(req.URLIDs) > 0 {
		placeholders := make([]string, len(req.URLIDs))
		for i := range req.URLIDs {
			if c.db.Type() == "postgres" {
				placeholders[i] = fmt.Sprintf("$%d", argIndex)
				argIndex++
			} else {
				placeholders[i] = "?"
			}
			args = append(args, req.URLIDs[i])
		}
		query += fmt.Sprintf(" AND c.url_id IN (%s)", strings.Join(placeholders, ","))
	}

	query += " ORDER BY c.clicked_at DESC"

	// Convert MySQL placeholders to PostgreSQL if needed
	if c.db.Type() == "postgres" {
		query = strings.Replace(query, "?", "$1", 1)
		query = strings.Replace(query, "?", "$2", 1)
	}

	rows, err := c.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get export data: %w", err)
	}
	defer rows.Close()

	var data []map[string]interface{}
	for rows.Next() {
		// This is a simplified version - in production, you'd need proper
		// column scanning based on the selected fields
		record := make(map[string]interface{})
		// Scan into record based on selectFields
		// This would need dynamic scanning implementation
		data = append(data, record)
	}

	return data, rows.Err()
}

// CleanupOldData removes old analytics data
func (c *Collector) CleanupOldData(ctx context.Context, cutoffDate time.Time) (*CleanupResult, error) {
	result := &CleanupResult{}
	startTime := time.Now()

	// Delete old clicks
	clickQuery := "DELETE FROM clicks WHERE clicked_at < ?"
	if c.db.Type() == "postgres" {
		clickQuery = "DELETE FROM clicks WHERE clicked_at < $1"
	}

	clickResult, err := c.db.Exec(ctx, clickQuery, cutoffDate)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old clicks: %w", err)
	}

	result.ClicksDeleted, _ = clickResult.RowsAffected()

	// Delete old daily stats
	statsQuery := "DELETE FROM click_daily_stats WHERE date < ?"
	if c.db.Type() == "postgres" {
		statsQuery = "DELETE FROM click_daily_stats WHERE date < $1"
	}

	statsResult, err := c.db.Exec(ctx, statsQuery, cutoffDate)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old stats: %w", err)
	}

	result.StatsDeleted, _ = statsResult.RowsAffected()
	result.Cleaned = true
	result.Duration = time.Since(startTime)

	return result, nil
}

// Helper functions

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}