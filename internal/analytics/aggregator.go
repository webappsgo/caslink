package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Aggregator handles data aggregation and statistics generation
type Aggregator struct {
	db     *db.DB
	config *config.AnalyticsConfig
	logger *logrus.Logger
}

// NewAggregator creates a new aggregator instance
func NewAggregator(database *db.DB, cfg *config.AnalyticsConfig, logger *logrus.Logger) (*Aggregator, error) {
	return &Aggregator{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// RunDailyAggregation processes daily statistics aggregation
func (a *Aggregator) RunDailyAggregation(ctx context.Context) error {
	a.logger.Info("Starting daily aggregation process")
	start := time.Now()

	// Get date range to aggregate (yesterday by default)
	yesterday := time.Now().AddDate(0, 0, -1)
	startDate := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 0, 1)

	// Get list of URLs that had clicks yesterday
	urls, err := a.getURLsWithClicksInRange(ctx, startDate, endDate)
	if err != nil {
		return fmt.Errorf("failed to get URLs with clicks: %w", err)
	}

	a.logger.WithField("urls_count", len(urls)).Info("Processing daily aggregation for URLs")

	// Process each URL
	for _, urlID := range urls {
		if err := a.aggregateURLDay(ctx, urlID, startDate); err != nil {
			a.logger.WithError(err).WithFields(logrus.Fields{
				"url_id": urlID,
				"date":   startDate,
			}).Error("Failed to aggregate URL day")
			continue
		}
	}

	duration := time.Since(start)
	a.logger.WithFields(logrus.Fields{
		"duration":   duration,
		"urls_count": len(urls),
		"date":       startDate.Format("2006-01-02"),
	}).Info("Daily aggregation completed")

	return nil
}

// GetDailyStats retrieves daily statistics for a URL
func (a *Aggregator) GetDailyStats(ctx context.Context, urlID string, startDate, endDate time.Time) ([]DailyStat, error) {
	query := `
		SELECT
			date, url_id, clicks, unique_clicks,
			top_countries, top_referrers, top_browsers, top_devices
		FROM click_daily_stats
		WHERE url_id = ? AND date >= ? AND date < ?
		ORDER BY date ASC`

	if a.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		query = strings.Replace(query, "$", "$1", 1)
		query = strings.Replace(query, "$", "$2", 1)
		query = strings.Replace(query, "$", "$3", 1)
	}

	rows, err := a.db.Query(ctx, query, urlID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily stats: %w", err)
	}
	defer rows.Close()

	var stats []DailyStat
	for rows.Next() {
		var stat DailyStat
		var topCountriesJSON, topReferrersJSON, topBrowsersJSON, topDevicesJSON sql.NullString

		if err := rows.Scan(
			&stat.Date, &stat.URLID, &stat.Clicks, &stat.UniqueClicks,
			&topCountriesJSON, &topReferrersJSON, &topBrowsersJSON, &topDevicesJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan daily stat: %w", err)
		}

		// Parse JSON fields
		if topCountriesJSON.Valid {
			if err := parseJSONToCountryStats(topCountriesJSON.String, &stat.TopCountries); err != nil {
				a.logger.WithError(err).Warn("Failed to parse top countries JSON")
			}
		}
		if topReferrersJSON.Valid {
			if err := parseJSONToReferrerStats(topReferrersJSON.String, &stat.TopReferrers); err != nil {
				a.logger.WithError(err).Warn("Failed to parse top referrers JSON")
			}
		}
		if topBrowsersJSON.Valid {
			if err := parseJSONToBrowserStats(topBrowsersJSON.String, &stat.TopBrowsers); err != nil {
				a.logger.WithError(err).Warn("Failed to parse top browsers JSON")
			}
		}
		if topDevicesJSON.Valid {
			if err := parseJSONToDeviceStats(topDevicesJSON.String, &stat.TopDevices); err != nil {
				a.logger.WithError(err).Warn("Failed to parse top devices JSON")
			}
		}

		stats = append(stats, stat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating daily stats: %w", err)
	}

	return stats, nil
}

// GetUserURLStats retrieves URL statistics for a user
func (a *Aggregator) GetUserURLStats(ctx context.Context, userID string, startDate, endDate time.Time) ([]URLStat, error) {
	query := `
		SELECT
			u.id, u.original_url, u.title, u.clicks, u.unique_clicks,
			u.created_at, MAX(c.clicked_at) as last_click_at
		FROM urls u
		LEFT JOIN clicks c ON u.id = c.url_id AND c.clicked_at >= ? AND c.clicked_at < ?
		WHERE u.user_id = ?
		GROUP BY u.id, u.original_url, u.title, u.clicks, u.unique_clicks, u.created_at
		ORDER BY u.created_at DESC`

	if a.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		query = strings.Replace(query, "$", "$1", 1)
		query = strings.Replace(query, "$", "$2", 1)
		query = strings.Replace(query, "$", "$3", 1)
	}

	rows, err := a.db.Query(ctx, query, startDate, endDate, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user URL stats: %w", err)
	}
	defer rows.Close()

	var stats []URLStat
	for rows.Next() {
		var stat URLStat
		var lastClickAt sql.NullTime

		if err := rows.Scan(
			&stat.URLID, &stat.OriginalURL, &stat.Title,
			&stat.Clicks, &stat.UniqueClicks, &stat.CreatedAt, &lastClickAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan URL stat: %w", err)
		}

		if lastClickAt.Valid {
			stat.LastClickAt = &lastClickAt.Time
		}

		stats = append(stats, stat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating URL stats: %w", err)
	}

	return stats, nil
}

// Helper methods for aggregation

func (a *Aggregator) getURLsWithClicksInRange(ctx context.Context, startDate, endDate time.Time) ([]string, error) {
	query := `
		SELECT DISTINCT url_id
		FROM clicks
		WHERE clicked_at >= ? AND clicked_at < ?`

	if a.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		query = strings.Replace(query, "$", "$1", 1)
		query = strings.Replace(query, "$", "$2", 1)
	}

	rows, err := a.db.Query(ctx, query, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query URLs with clicks: %w", err)
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var urlID string
		if err := rows.Scan(&urlID); err != nil {
			return nil, fmt.Errorf("failed to scan URL ID: %w", err)
		}
		urls = append(urls, urlID)
	}

	return urls, rows.Err()
}

func (a *Aggregator) aggregateURLDay(ctx context.Context, urlID string, date time.Time) error {
	endDate := date.AddDate(0, 0, 1)

	// Calculate basic stats
	totalClicks, uniqueClicks, err := a.calculateDayClicks(ctx, urlID, date, endDate)
	if err != nil {
		return fmt.Errorf("failed to calculate day clicks: %w", err)
	}

	// Get top data
	topCountries, err := a.getTopCountriesForDay(ctx, urlID, date, endDate, 5)
	if err != nil {
		return fmt.Errorf("failed to get top countries: %w", err)
	}

	topReferrers, err := a.getTopReferrersForDay(ctx, urlID, date, endDate, 5)
	if err != nil {
		return fmt.Errorf("failed to get top referrers: %w", err)
	}

	topBrowsers, err := a.getTopBrowsersForDay(ctx, urlID, date, endDate, 5)
	if err != nil {
		return fmt.Errorf("failed to get top browsers: %w", err)
	}

	topDevices, err := a.getTopDevicesForDay(ctx, urlID, date, endDate, 5)
	if err != nil {
		return fmt.Errorf("failed to get top devices: %w", err)
	}

	// Convert to JSON strings
	topCountriesJSON, _ := convertCountryStatsToJSON(topCountries)
	topReferrersJSON, _ := convertReferrerStatsToJSON(topReferrers)
	topBrowsersJSON, _ := convertBrowserStatsToJSON(topBrowsers)
	topDevicesJSON, _ := convertDeviceStatsToJSON(topDevices)

	// Upsert daily stat record
	return a.upsertDailyStat(ctx, DailyStat{
		URLID:        urlID,
		Date:         date,
		Clicks:       totalClicks,
		UniqueClicks: uniqueClicks,
		TopCountries: topCountries,
		TopReferrers: topReferrers,
		TopBrowsers:  topBrowsers,
		TopDevices:   topDevices,
	}, topCountriesJSON, topReferrersJSON, topBrowsersJSON, topDevicesJSON)
}

func (a *Aggregator) calculateDayClicks(ctx context.Context, urlID string, startDate, endDate time.Time) (int64, int64, error) {
	query := `
		SELECT
			COUNT(*) as total_clicks,
			COUNT(DISTINCT ip_hash) as unique_clicks
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at < ? AND is_bot = false`

	if a.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		query = strings.Replace(query, "$", "$1", 1)
		query = strings.Replace(query, "$", "$2", 1)
		query = strings.Replace(query, "$", "$3", 1)
	}

	var totalClicks, uniqueClicks int64
	err := a.db.QueryRow(ctx, query, urlID, startDate, endDate).Scan(&totalClicks, &uniqueClicks)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to calculate day clicks: %w", err)
	}

	return totalClicks, uniqueClicks, nil
}

func (a *Aggregator) getTopCountriesForDay(ctx context.Context, urlID string, startDate, endDate time.Time, limit int) ([]CountryStat, error) {
	query := `
		SELECT
			country_code, country_name, COUNT(*) as clicks
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at < ?
			AND is_bot = false AND country_code IS NOT NULL AND country_code != ''
		GROUP BY country_code, country_name
		ORDER BY clicks DESC
		LIMIT ?`

	if a.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		query = strings.Replace(query, "$", "$1", 1)
		query = strings.Replace(query, "$", "$2", 1)
		query = strings.Replace(query, "$", "$3", 1)
		query = strings.Replace(query, "$", "$4", 1)
	}

	rows, err := a.db.Query(ctx, query, urlID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top countries: %w", err)
	}
	defer rows.Close()

	var countries []CountryStat
	var totalClicks int64
	for rows.Next() {
		var country CountryStat
		if err := rows.Scan(&country.CountryCode, &country.CountryName, &country.Clicks); err != nil {
			return nil, fmt.Errorf("failed to scan country stat: %w", err)
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

func (a *Aggregator) getTopReferrersForDay(ctx context.Context, urlID string, startDate, endDate time.Time, limit int) ([]ReferrerStat, error) {
	query := `
		SELECT
			COALESCE(referrer_domain, 'Direct') as domain, COUNT(*) as clicks
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at < ? AND is_bot = false
		GROUP BY referrer_domain
		ORDER BY clicks DESC
		LIMIT ?`

	if a.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		query = strings.Replace(query, "$", "$1", 1)
		query = strings.Replace(query, "$", "$2", 1)
		query = strings.Replace(query, "$", "$3", 1)
		query = strings.Replace(query, "$", "$4", 1)
	}

	rows, err := a.db.Query(ctx, query, urlID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top referrers: %w", err)
	}
	defer rows.Close()

	var referrers []ReferrerStat
	var totalClicks int64
	for rows.Next() {
		var referrer ReferrerStat
		if err := rows.Scan(&referrer.Domain, &referrer.Clicks); err != nil {
			return nil, fmt.Errorf("failed to scan referrer stat: %w", err)
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

func (a *Aggregator) getTopBrowsersForDay(ctx context.Context, urlID string, startDate, endDate time.Time, limit int) ([]BrowserStat, error) {
	query := `
		SELECT
			COALESCE(parsed_browser, 'Unknown') as browser, COUNT(*) as clicks
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at < ? AND is_bot = false
		GROUP BY parsed_browser
		ORDER BY clicks DESC
		LIMIT ?`

	if a.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		query = strings.Replace(query, "$", "$1", 1)
		query = strings.Replace(query, "$", "$2", 1)
		query = strings.Replace(query, "$", "$3", 1)
		query = strings.Replace(query, "$", "$4", 1)
	}

	rows, err := a.db.Query(ctx, query, urlID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top browsers: %w", err)
	}
	defer rows.Close()

	var browsers []BrowserStat
	var totalClicks int64
	for rows.Next() {
		var browser BrowserStat
		if err := rows.Scan(&browser.Browser, &browser.Clicks); err != nil {
			return nil, fmt.Errorf("failed to scan browser stat: %w", err)
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

func (a *Aggregator) getTopDevicesForDay(ctx context.Context, urlID string, startDate, endDate time.Time, limit int) ([]DeviceStat, error) {
	query := `
		SELECT
			COALESCE(parsed_device, 'Unknown') as device_type, COUNT(*) as clicks
		FROM clicks
		WHERE url_id = ? AND clicked_at >= ? AND clicked_at < ? AND is_bot = false
		GROUP BY parsed_device
		ORDER BY clicks DESC
		LIMIT ?`

	if a.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		query = strings.Replace(query, "$", "$1", 1)
		query = strings.Replace(query, "$", "$2", 1)
		query = strings.Replace(query, "$", "$3", 1)
		query = strings.Replace(query, "$", "$4", 1)
	}

	rows, err := a.db.Query(ctx, query, urlID, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top devices: %w", err)
	}
	defer rows.Close()

	var devices []DeviceStat
	var totalClicks int64
	for rows.Next() {
		var device DeviceStat
		if err := rows.Scan(&device.DeviceType, &device.Clicks); err != nil {
			return nil, fmt.Errorf("failed to scan device stat: %w", err)
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

func (a *Aggregator) upsertDailyStat(ctx context.Context, stat DailyStat, topCountriesJSON, topReferrersJSON, topBrowsersJSON, topDevicesJSON string) error {
	id := fmt.Sprintf("%s_%s", stat.URLID, stat.Date.Format("2006-01-02"))

	query := `
		INSERT OR REPLACE INTO click_daily_stats
		(id, url_id, date, clicks, unique_clicks, top_countries, top_referrers, top_browsers, top_devices, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`

	if a.db.Type() == "postgres" {
		query = `
			INSERT INTO click_daily_stats
			(id, url_id, date, clicks, unique_clicks, top_countries, top_referrers, top_browsers, top_devices, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, CURRENT_TIMESTAMP)
			ON CONFLICT (id) DO UPDATE SET
				clicks = EXCLUDED.clicks,
				unique_clicks = EXCLUDED.unique_clicks,
				top_countries = EXCLUDED.top_countries,
				top_referrers = EXCLUDED.top_referrers,
				top_browsers = EXCLUDED.top_browsers,
				top_devices = EXCLUDED.top_devices`
	} else if a.db.Type() == "mysql" {
		query = `
			INSERT INTO click_daily_stats
			(id, url_id, date, clicks, unique_clicks, top_countries, top_referrers, top_browsers, top_devices, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
			ON DUPLICATE KEY UPDATE
				clicks = VALUES(clicks),
				unique_clicks = VALUES(unique_clicks),
				top_countries = VALUES(top_countries),
				top_referrers = VALUES(top_referrers),
				top_browsers = VALUES(top_browsers),
				top_devices = VALUES(top_devices)`
	}

	_, err := a.db.Exec(ctx, query, id, stat.URLID, stat.Date, stat.Clicks, stat.UniqueClicks,
		topCountriesJSON, topReferrersJSON, topBrowsersJSON, topDevicesJSON)
	if err != nil {
		return fmt.Errorf("failed to upsert daily stat: %w", err)
	}

	return nil
}