package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// ReportGenerator handles analytics report generation
type ReportGenerator struct {
	db     *db.DB
	config *config.AnalyticsConfig
	logger *logrus.Logger
}

// NewReportGenerator creates a new report generator
func NewReportGenerator(database *db.DB, cfg *config.AnalyticsConfig, logger *logrus.Logger) *ReportGenerator {
	return &ReportGenerator{
		db:     database,
		config: cfg,
		logger: logger,
	}
}

// GenerateReport generates a report based on the request
func (rg *ReportGenerator) GenerateReport(ctx context.Context, req *ReportRequest) (*ReportResponse, error) {
	rg.logger.WithFields(logrus.Fields{
		"report_type": req.Type,
		"format":      req.Format,
		"start_date":  req.Filter.StartDate,
		"end_date":    req.Filter.EndDate,
	}).Info("Generating analytics report")

	var content []byte
	var contentType string
	var err error

	switch req.Type {
	case ReportTypeOverview:
		content, err = rg.generateOverviewReport(ctx, req)
		contentType = getContentType(req.Format)
	case ReportTypeTraffic:
		content, err = rg.generateTrafficReport(ctx, req)
		contentType = getContentType(req.Format)
	case ReportTypeGeographic:
		content, err = rg.generateGeographicReport(ctx, req)
		contentType = getContentType(req.Format)
	case ReportTypeTechnology:
		content, err = rg.generateTechnologyReport(ctx, req)
		contentType = getContentType(req.Format)
	case ReportTypeReferrers:
		content, err = rg.generateReferrersReport(ctx, req)
		contentType = getContentType(req.Format)
	case ReportTypeCustom:
		content, err = rg.generateCustomReport(ctx, req)
		contentType = getContentType(req.Format)
	default:
		return nil, fmt.Errorf("unsupported report type: %s", req.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate %s report: %w", req.Type, err)
	}

	filename := fmt.Sprintf("%s_report_%s_%s.%s",
		req.Type,
		req.Filter.StartDate.Format("2006-01-02"),
		req.Filter.EndDate.Format("2006-01-02"),
		getFileExtension(req.Format))

	return &ReportResponse{
		Type:        req.Type,
		GeneratedAt: time.Now(),
		Content:     content,
		ContentType: contentType,
		Filename:    filename,
	}, nil
}

// generateOverviewReport generates an overview report
func (rg *ReportGenerator) generateOverviewReport(ctx context.Context, req *ReportRequest) ([]byte, error) {
	// Get overall statistics
	totalClicks, uniqueClicks, err := rg.getOverallStats(ctx, req.Filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get overall stats: %w", err)
	}

	// Get top URLs
	topURLs, err := rg.getTopURLs(ctx, req.Filter, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to get top URLs: %w", err)
	}

	// Get daily trends
	dailyTrends, err := rg.getDailyTrends(ctx, req.Filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily trends: %w", err)
	}

	overview := struct {
		TotalClicks   int64       `json:"total_clicks"`
		UniqueClicks  int64       `json:"unique_clicks"`
		TopURLs       []URLStat   `json:"top_urls"`
		DailyTrends   []DailyStat `json:"daily_trends"`
		GeneratedAt   time.Time   `json:"generated_at"`
		DateRange     string      `json:"date_range"`
	}{
		TotalClicks:  totalClicks,
		UniqueClicks: uniqueClicks,
		TopURLs:      topURLs,
		DailyTrends:  dailyTrends,
		GeneratedAt:  time.Now(),
		DateRange:    fmt.Sprintf("%s to %s", req.Filter.StartDate.Format("2006-01-02"), req.Filter.EndDate.Format("2006-01-02")),
	}

	return formatReportContent(overview, req.Format)
}

// generateTrafficReport generates a traffic analysis report
func (rg *ReportGenerator) generateTrafficReport(ctx context.Context, req *ReportRequest) ([]byte, error) {
	// Get hourly traffic patterns
	hourlyTraffic, err := rg.getHourlyTraffic(ctx, req.Filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get hourly traffic: %w", err)
	}

	// Get daily traffic patterns
	dailyTraffic, err := rg.getDailyTraffic(ctx, req.Filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily traffic: %w", err)
	}

	// Get peak hours and days
	peakHour, peakDay, err := rg.getPeakTimes(ctx, req.Filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get peak times: %w", err)
	}

	traffic := struct {
		HourlyTraffic map[int]int64     `json:"hourly_traffic"`
		DailyTraffic  map[string]int64  `json:"daily_traffic"`
		PeakHour      int               `json:"peak_hour"`
		PeakDay       string            `json:"peak_day"`
		GeneratedAt   time.Time         `json:"generated_at"`
		DateRange     string            `json:"date_range"`
	}{
		HourlyTraffic: hourlyTraffic,
		DailyTraffic:  dailyTraffic,
		PeakHour:      peakHour,
		PeakDay:       peakDay,
		GeneratedAt:   time.Now(),
		DateRange:     fmt.Sprintf("%s to %s", req.Filter.StartDate.Format("2006-01-02"), req.Filter.EndDate.Format("2006-01-02")),
	}

	return formatReportContent(traffic, req.Format)
}

// generateGeographicReport generates a geographic analysis report
func (rg *ReportGenerator) generateGeographicReport(ctx context.Context, req *ReportRequest) ([]byte, error) {
	// Get top countries
	topCountries, err := rg.getTopCountriesReport(ctx, req.Filter, 20)
	if err != nil {
		return nil, fmt.Errorf("failed to get top countries: %w", err)
	}

	// Get geographic distribution
	geoDistribution, err := rg.getGeographicDistribution(ctx, req.Filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get geographic distribution: %w", err)
	}

	geographic := struct {
		TopCountries        []CountryStat            `json:"top_countries"`
		GeographicDistribution map[string]int64      `json:"geographic_distribution"`
		GeneratedAt         time.Time                `json:"generated_at"`
		DateRange           string                   `json:"date_range"`
	}{
		TopCountries:           topCountries,
		GeographicDistribution: geoDistribution,
		GeneratedAt:            time.Now(),
		DateRange:              fmt.Sprintf("%s to %s", req.Filter.StartDate.Format("2006-01-02"), req.Filter.EndDate.Format("2006-01-02")),
	}

	return formatReportContent(geographic, req.Format)
}

// generateTechnologyReport generates a technology analysis report
func (rg *ReportGenerator) generateTechnologyReport(ctx context.Context, req *ReportRequest) ([]byte, error) {
	// Get browser statistics
	browsers, err := rg.getBrowsersReport(ctx, req.Filter, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to get browsers: %w", err)
	}

	// Get operating system statistics
	operatingSystems, err := rg.getOperatingSystemsReport(ctx, req.Filter, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to get operating systems: %w", err)
	}

	// Get device statistics
	devices, err := rg.getDevicesReport(ctx, req.Filter, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to get devices: %w", err)
	}

	technology := struct {
		Browsers         []BrowserStat `json:"browsers"`
		OperatingSystems []OStat       `json:"operating_systems"`
		Devices          []DeviceStat  `json:"devices"`
		GeneratedAt      time.Time     `json:"generated_at"`
		DateRange        string        `json:"date_range"`
	}{
		Browsers:         browsers,
		OperatingSystems: operatingSystems,
		Devices:          devices,
		GeneratedAt:      time.Now(),
		DateRange:        fmt.Sprintf("%s to %s", req.Filter.StartDate.Format("2006-01-02"), req.Filter.EndDate.Format("2006-01-02")),
	}

	return formatReportContent(technology, req.Format)
}

// generateReferrersReport generates a referrers analysis report
func (rg *ReportGenerator) generateReferrersReport(ctx context.Context, req *ReportRequest) ([]byte, error) {
	// Get top referrers
	topReferrers, err := rg.getTopReferrersReport(ctx, req.Filter, 20)
	if err != nil {
		return nil, fmt.Errorf("failed to get top referrers: %w", err)
	}

	// Get referrer categories
	referrerCategories, err := rg.getReferrerCategories(ctx, req.Filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get referrer categories: %w", err)
	}

	referrers := struct {
		TopReferrers       []ReferrerStat     `json:"top_referrers"`
		ReferrerCategories map[string]int64   `json:"referrer_categories"`
		GeneratedAt        time.Time          `json:"generated_at"`
		DateRange          string             `json:"date_range"`
	}{
		TopReferrers:       topReferrers,
		ReferrerCategories: referrerCategories,
		GeneratedAt:        time.Now(),
		DateRange:          fmt.Sprintf("%s to %s", req.Filter.StartDate.Format("2006-01-02"), req.Filter.EndDate.Format("2006-01-02")),
	}

	return formatReportContent(referrers, req.Format)
}

// generateCustomReport generates a custom report based on options
func (rg *ReportGenerator) generateCustomReport(ctx context.Context, req *ReportRequest) ([]byte, error) {
	// Custom reports can include specific metrics based on options
	metrics := make(map[string]interface{})

	if includeOverview, ok := req.Options["include_overview"].(bool); ok && includeOverview {
		totalClicks, uniqueClicks, _ := rg.getOverallStats(ctx, req.Filter)
		metrics["total_clicks"] = totalClicks
		metrics["unique_clicks"] = uniqueClicks
	}

	if includeGeographic, ok := req.Options["include_geographic"].(bool); ok && includeGeographic {
		topCountries, _ := rg.getTopCountriesReport(ctx, req.Filter, 10)
		metrics["top_countries"] = topCountries
	}

	if includeTechnology, ok := req.Options["include_technology"].(bool); ok && includeTechnology {
		browsers, _ := rg.getBrowsersReport(ctx, req.Filter, 10)
		devices, _ := rg.getDevicesReport(ctx, req.Filter, 10)
		metrics["browsers"] = browsers
		metrics["devices"] = devices
	}

	custom := struct {
		Metrics     map[string]interface{} `json:"metrics"`
		Options     map[string]interface{} `json:"options"`
		GeneratedAt time.Time              `json:"generated_at"`
		DateRange   string                 `json:"date_range"`
	}{
		Metrics:     metrics,
		Options:     req.Options,
		GeneratedAt: time.Now(),
		DateRange:   fmt.Sprintf("%s to %s", req.Filter.StartDate.Format("2006-01-02"), req.Filter.EndDate.Format("2006-01-02")),
	}

	return formatReportContent(custom, req.Format)
}

// Helper functions for report generation

func (rg *ReportGenerator) getOverallStats(ctx context.Context, filter ClickFilter) (int64, int64, error) {
	query := `
		SELECT
			COUNT(*) as total_clicks,
			COUNT(DISTINCT ip_hash) as unique_clicks
		FROM clicks
		WHERE clicked_at >= ? AND clicked_at < ?`

	conditions := []interface{}{filter.StartDate, filter.EndDate}

	if filter.ExcludeBots {
		query += " AND is_bot = false"
	}

	if len(filter.URLIDs) > 0 {
		placeholders := strings.Repeat("?,", len(filter.URLIDs))
		placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma
		query += fmt.Sprintf(" AND url_id IN (%s)", placeholders)
		for _, urlID := range filter.URLIDs {
			conditions = append(conditions, urlID)
		}
	}

	if rg.db.Type() == "postgres" {
		query = convertToPostgresQuery(query, len(conditions))
	}

	var totalClicks, uniqueClicks int64
	err := rg.db.QueryRow(ctx, query, conditions...).Scan(&totalClicks, &uniqueClicks)
	return totalClicks, uniqueClicks, err
}

func (rg *ReportGenerator) getTopURLs(ctx context.Context, filter ClickFilter, limit int) ([]URLStat, error) {
	query := `
		SELECT
			u.id, u.original_url, u.title, COUNT(c.id) as clicks,
			COUNT(DISTINCT c.ip_hash) as unique_clicks, u.created_at
		FROM urls u
		LEFT JOIN clicks c ON u.id = c.url_id AND c.clicked_at >= ? AND c.clicked_at < ?`

	conditions := []interface{}{filter.StartDate, filter.EndDate}

	if filter.ExcludeBots {
		query += " AND (c.is_bot = false OR c.is_bot IS NULL)"
	}

	query += ` GROUP BY u.id, u.original_url, u.title, u.created_at
		ORDER BY clicks DESC
		LIMIT ?`

	conditions = append(conditions, limit)

	if rg.db.Type() == "postgres" {
		query = convertToPostgresQuery(query, len(conditions))
	}

	rows, err := rg.db.Query(ctx, query, conditions...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []URLStat
	for rows.Next() {
		var url URLStat
		err := rows.Scan(&url.URLID, &url.OriginalURL, &url.Title, &url.Clicks, &url.UniqueClicks, &url.CreatedAt)
		if err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}

	return urls, rows.Err()
}

func (rg *ReportGenerator) getDailyTrends(ctx context.Context, filter ClickFilter) ([]DailyStat, error) {
	// Implementation for daily trends
	query := `
		SELECT
			DATE(clicked_at) as date,
			COUNT(*) as clicks,
			COUNT(DISTINCT ip_hash) as unique_clicks
		FROM clicks
		WHERE clicked_at >= ? AND clicked_at < ?`

	conditions := []interface{}{filter.StartDate, filter.EndDate}

	if filter.ExcludeBots {
		query += " AND is_bot = false"
	}

	query += " GROUP BY DATE(clicked_at) ORDER BY date"

	if rg.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "DATE(clicked_at)", "DATE(clicked_at)")
		query = convertToPostgresQuery(query, len(conditions))
	}

	rows, err := rg.db.Query(ctx, query, conditions...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []DailyStat
	for rows.Next() {
		var trend DailyStat
		err := rows.Scan(&trend.Date, &trend.Clicks, &trend.UniqueClicks)
		if err != nil {
			return nil, err
		}
		trends = append(trends, trend)
	}

	return trends, rows.Err()
}

// Additional helper types for reports
type OStat struct {
	OS         string  `json:"os"`
	Clicks     int64   `json:"clicks"`
	Percentage float64 `json:"percentage"`
}

// Helper functions
func getContentType(format string) string {
	switch format {
	case "json":
		return "application/json"
	case "csv":
		return "text/csv"
	case "pdf":
		return "application/pdf"
	case "html":
		return "text/html"
	default:
		return "application/octet-stream"
	}
}

func getFileExtension(format string) string {
	switch format {
	case "json":
		return "json"
	case "csv":
		return "csv"
	case "pdf":
		return "pdf"
	case "html":
		return "html"
	default:
		return "txt"
	}
}

func formatReportContent(data interface{}, format string) ([]byte, error) {
	switch format {
	case "json":
		return json.Marshal(data)
	case "csv":
		// Simplified CSV generation - in production use a proper CSV library
		return []byte(fmt.Sprintf("Report Data: %+v", data)), nil
	case "pdf":
		// Simplified PDF generation - in production use a proper PDF library
		return []byte(fmt.Sprintf("PDF Report Data: %+v", data)), nil
	case "html":
		// Simplified HTML generation - in production use proper templating
		return []byte(fmt.Sprintf("<html><body><h1>Report</h1><pre>%+v</pre></body></html>", data)), nil
	default:
		return json.Marshal(data)
	}
}

func convertToPostgresQuery(query string, paramCount int) string {
	for i := 1; i <= paramCount; i++ {
		query = strings.Replace(query, "?", fmt.Sprintf("$%d", i), 1)
	}
	return query
}

// Placeholder implementations for missing functions
func (rg *ReportGenerator) getHourlyTraffic(ctx context.Context, filter ClickFilter) (map[int]int64, error) {
	// Implementation would analyze clicks by hour of day
	return make(map[int]int64), nil
}

func (rg *ReportGenerator) getDailyTraffic(ctx context.Context, filter ClickFilter) (map[string]int64, error) {
	// Implementation would analyze clicks by day of week
	return make(map[string]int64), nil
}

func (rg *ReportGenerator) getPeakTimes(ctx context.Context, filter ClickFilter) (int, string, error) {
	// Implementation would find peak hour and day
	return 14, "Monday", nil // Placeholder
}

func (rg *ReportGenerator) getTopCountriesReport(ctx context.Context, filter ClickFilter, limit int) ([]CountryStat, error) {
	// Implementation would get top countries with detailed stats
	return []CountryStat{}, nil
}

func (rg *ReportGenerator) getGeographicDistribution(ctx context.Context, filter ClickFilter) (map[string]int64, error) {
	// Implementation would analyze geographic distribution
	return make(map[string]int64), nil
}

func (rg *ReportGenerator) getBrowsersReport(ctx context.Context, filter ClickFilter, limit int) ([]BrowserStat, error) {
	// Implementation would get browser statistics
	return []BrowserStat{}, nil
}

func (rg *ReportGenerator) getOperatingSystemsReport(ctx context.Context, filter ClickFilter, limit int) ([]OStat, error) {
	// Implementation would get OS statistics
	return []OStat{}, nil
}

func (rg *ReportGenerator) getDevicesReport(ctx context.Context, filter ClickFilter, limit int) ([]DeviceStat, error) {
	// Implementation would get device statistics
	return []DeviceStat{}, nil
}

func (rg *ReportGenerator) getTopReferrersReport(ctx context.Context, filter ClickFilter, limit int) ([]ReferrerStat, error) {
	// Implementation would get referrer statistics
	return []ReferrerStat{}, nil
}

func (rg *ReportGenerator) getReferrerCategories(ctx context.Context, filter ClickFilter) (map[string]int64, error) {
	// Implementation would categorize referrers (social, search, direct, etc.)
	return make(map[string]int64), nil
}