package analytics

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Service handles analytics collection and reporting
type Service struct {
	db         *db.DB
	config     *config.AnalyticsConfig
	logger     *logrus.Logger
	collector  *Collector
	aggregator *Aggregator
	geolocator *GeoLocator
}

// NewService creates a new analytics service
func NewService(database *db.DB, cfg *config.AnalyticsConfig, logger *logrus.Logger) (*Service, error) {
	collector, err := NewCollector(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create collector: %w", err)
	}

	aggregator, err := NewAggregator(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create aggregator: %w", err)
	}

	geolocator, err := NewGeoLocator(cfg.GeoIPDatabasePath, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create geolocator: %w", err)
	}

	return &Service{
		db:         database,
		config:     cfg,
		logger:     logger,
		collector:  collector,
		aggregator: aggregator,
		geolocator: geolocator,
	}, nil
}

// TrackClick records a click event
func (s *Service) TrackClick(ctx context.Context, req *ClickRequest) (*ClickResponse, error) {
	if !s.config.Enabled {
		return &ClickResponse{Tracked: false}, nil
	}

	// Check if this is bot traffic and should be excluded
	if s.config.ExcludeBots && s.isBot(req.UserAgent) {
		s.logger.WithFields(logrus.Fields{
			"url_id":     req.URLID,
			"user_agent": req.UserAgent,
		}).Debug("Excluded bot traffic from analytics")
		return &ClickResponse{Tracked: false, Reason: "Bot traffic excluded"}, nil
	}

	// Anonymize IP if configured
	ipAddress := req.IPAddress
	if s.config.AnonymizeIPs {
		ipAddress = s.anonymizeIP(ipAddress)
	}

	// Get geolocation data
	var location *LocationData
	if s.config.EnableGeolocation && s.geolocator != nil {
		location = s.geolocator.GetLocation(req.IPAddress)
	}

	// Parse user agent
	var deviceInfo *DeviceInfo
	if req.UserAgent != "" {
		deviceInfo = s.parseUserAgent(req.UserAgent)
	}

	// Create click record
	clickID, err := generateClickID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate click ID: %w", err)
	}

	// Generate IP hash for unique counting
	ipHash := hashIP(req.IPAddress)

	// Check if this is a unique click
	isUnique, err := s.collector.IsUniqueClick(ctx, req.URLID, ipHash)
	if err != nil {
		s.logger.WithError(err).Error("Failed to check click uniqueness")
		isUnique = true // Default to unique if check fails
	}

	click := &Click{
		ID:           clickID,
		URLID:        req.URLID,
		ClickedAt:    time.Now(),
		IPAddress:    ipAddress,
		IPHash:       ipHash,
		UserAgent:    req.UserAgent,
		Referrer:     req.Referrer,
		IsUnique:     isUnique,
		IsBot:        s.isBot(req.UserAgent),
		Location:     location,
		DeviceInfo:   deviceInfo,
	}

	// Extract referrer domain
	if click.Referrer != "" {
		click.ReferrerDomain = extractDomain(click.Referrer)
	}

	// Record the click
	if err := s.collector.RecordClick(ctx, click); err != nil {
		return nil, fmt.Errorf("failed to record click: %w", err)
	}

	// Update URL click counts asynchronously
	go func() {
		if err := s.updateURLCounts(context.Background(), req.URLID, isUnique); err != nil {
			s.logger.WithError(err).WithField("url_id", req.URLID).Error("Failed to update URL click counts")
		}
	}()

	s.logger.WithFields(logrus.Fields{
		"click_id": clickID,
		"url_id":   req.URLID,
		"is_unique": isUnique,
		"country":  location.CountryCode,
		"browser":  deviceInfo.Browser,
	}).Info("Click tracked")

	return &ClickResponse{
		Tracked:  true,
		ClickID:  clickID,
		IsUnique: isUnique,
	}, nil
}

// GetURLAnalytics retrieves analytics for a specific URL
func (s *Service) GetURLAnalytics(ctx context.Context, req *URLAnalyticsRequest) (*URLAnalyticsResponse, error) {
	analytics, err := s.collector.GetURLAnalytics(ctx, req.URLID, req.StartDate, req.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get URL analytics: %w", err)
	}

	// Get aggregated daily stats
	dailyStats, err := s.aggregator.GetDailyStats(ctx, req.URLID, req.StartDate, req.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily stats: %w", err)
	}

	// Get top countries, referrers, etc.
	topData, err := s.getTopData(ctx, req.URLID, req.StartDate, req.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get top data: %w", err)
	}

	response := &URLAnalyticsResponse{
		URLID:       req.URLID,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		TotalClicks: analytics.TotalClicks,
		UniqueClicks: analytics.UniqueClicks,
		DailyStats:  dailyStats,
		TopCountries: topData.Countries,
		TopReferrers: topData.Referrers,
		TopBrowsers:  topData.Browsers,
		TopDevices:   topData.Devices,
		RecentClicks: analytics.RecentClicks,
	}

	return response, nil
}

// GetUserAnalytics retrieves analytics for all URLs belonging to a user
func (s *Service) GetUserAnalytics(ctx context.Context, req *UserAnalyticsRequest) (*UserAnalyticsResponse, error) {
	analytics, err := s.collector.GetUserAnalytics(ctx, req.UserID, req.StartDate, req.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get user analytics: %w", err)
	}

	// Get URL-specific stats
	urlStats, err := s.aggregator.GetUserURLStats(ctx, req.UserID, req.StartDate, req.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get URL stats: %w", err)
	}

	response := &UserAnalyticsResponse{
		UserID:       req.UserID,
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
		TotalClicks:  analytics.TotalClicks,
		UniqueClicks: analytics.UniqueClicks,
		TotalURLs:    analytics.TotalURLs,
		URLStats:     urlStats,
	}

	return response, nil
}

// GetGlobalAnalytics retrieves global analytics (admin only)
func (s *Service) GetGlobalAnalytics(ctx context.Context, req *GlobalAnalyticsRequest) (*GlobalAnalyticsResponse, error) {
	analytics, err := s.collector.GetGlobalAnalytics(ctx, req.StartDate, req.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get global analytics: %w", err)
	}

	response := &GlobalAnalyticsResponse{
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
		TotalClicks:  analytics.TotalClicks,
		UniqueClicks: analytics.UniqueClicks,
		TotalURLs:    analytics.TotalURLs,
		TotalUsers:   analytics.TotalUsers,
		DailyStats:   analytics.DailyStats,
	}

	return response, nil
}

// GetRealTimeStats retrieves real-time analytics
func (s *Service) GetRealTimeStats(ctx context.Context, req *RealTimeStatsRequest) (*RealTimeStatsResponse, error) {
	stats, err := s.collector.GetRealTimeStats(ctx, req.TimeWindow)
	if err != nil {
		return nil, fmt.Errorf("failed to get real-time stats: %w", err)
	}

	return stats, nil
}

// ExportAnalytics exports analytics data in various formats
func (s *Service) ExportAnalytics(ctx context.Context, req *ExportRequest) (*ExportResponse, error) {
	data, err := s.collector.GetExportData(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get export data: %w", err)
	}

	var content []byte
	var contentType string

	switch req.Format {
	case "csv":
		content, err = s.generateCSV(data)
		contentType = "text/csv"
	case "json":
		content, err = json.MarshalIndent(data, "", "  ")
		contentType = "application/json"
	case "pdf":
		content, err = s.generatePDF(data)
		contentType = "application/pdf"
	default:
		return nil, fmt.Errorf("unsupported export format: %s", req.Format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate export content: %w", err)
	}

	filename := fmt.Sprintf("analytics_%s_%s.%s",
		req.StartDate.Format("2006-01-02"),
		req.EndDate.Format("2006-01-02"),
		req.Format)

	return &ExportResponse{
		Content:     content,
		ContentType: contentType,
		Filename:    filename,
	}, nil
}

// RunAggregation runs data aggregation for daily stats
func (s *Service) RunAggregation(ctx context.Context) error {
	return s.aggregator.RunDailyAggregation(ctx)
}

// CleanupOldData removes old analytics data based on retention policy
func (s *Service) CleanupOldData(ctx context.Context) (*CleanupResult, error) {
	if s.config.RetentionDays <= 0 {
		return &CleanupResult{Cleaned: false, Reason: "Unlimited retention configured"}, nil
	}

	cutoffDate := time.Now().AddDate(0, 0, -s.config.RetentionDays)
	result, err := s.collector.CleanupOldData(ctx, cutoffDate)
	if err != nil {
		return nil, fmt.Errorf("failed to cleanup old data: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"cutoff_date":   cutoffDate,
		"clicks_deleted": result.ClicksDeleted,
		"stats_deleted":  result.StatsDeleted,
	}).Info("Analytics data cleanup completed")

	return result, nil
}

// Helper methods

func (s *Service) updateURLCounts(ctx context.Context, urlID string, isUnique bool) error {
	query := "UPDATE urls SET clicks = clicks + 1"
	if isUnique {
		query += ", unique_clicks = unique_clicks + 1"
	}
	query += " WHERE id = ?"

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$1")
	}

	_, err := s.db.Exec(ctx, query, urlID)
	return err
}

func (s *Service) isBot(userAgent string) bool {
	if userAgent == "" {
		return false
	}

	userAgent = strings.ToLower(userAgent)
	botKeywords := []string{
		"bot", "crawler", "spider", "scraper", "wget", "curl",
		"http", "python", "java", "go-http", "okhttp",
		"googlebot", "bingbot", "slurp", "duckduckbot",
		"facebookexternalhit", "twitterbot", "linkedinbot",
		"whatsapp", "telegram", "slack", "discord",
	}

	for _, keyword := range botKeywords {
		if strings.Contains(userAgent, keyword) {
			return true
		}
	}

	return false
}

func (s *Service) anonymizeIP(ip string) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ip
	}

	if parsedIP.To4() != nil {
		// IPv4: mask last octet
		parts := strings.Split(ip, ".")
		if len(parts) == 4 {
			return fmt.Sprintf("%s.%s.%s.0", parts[0], parts[1], parts[2])
		}
	} else {
		// IPv6: mask last 64 bits
		parts := strings.Split(ip, ":")
		if len(parts) >= 4 {
			return strings.Join(parts[:4], ":") + "::"
		}
	}

	return ip
}

func (s *Service) parseUserAgent(userAgent string) *DeviceInfo {
	// Simple user agent parsing - in production, use a proper UA parsing library
	device := &DeviceInfo{
		UserAgent: userAgent,
	}

	ua := strings.ToLower(userAgent)

	// Detect browser
	switch {
	case strings.Contains(ua, "chrome"):
		device.Browser = "Chrome"
	case strings.Contains(ua, "firefox"):
		device.Browser = "Firefox"
	case strings.Contains(ua, "safari") && !strings.Contains(ua, "chrome"):
		device.Browser = "Safari"
	case strings.Contains(ua, "edge"):
		device.Browser = "Edge"
	case strings.Contains(ua, "opera"):
		device.Browser = "Opera"
	default:
		device.Browser = "Other"
	}

	// Detect OS
	switch {
	case strings.Contains(ua, "windows"):
		device.OS = "Windows"
	case strings.Contains(ua, "mac"):
		device.OS = "macOS"
	case strings.Contains(ua, "linux"):
		device.OS = "Linux"
	case strings.Contains(ua, "android"):
		device.OS = "Android"
	case strings.Contains(ua, "ios") || strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad"):
		device.OS = "iOS"
	default:
		device.OS = "Other"
	}

	// Detect device type
	switch {
	case strings.Contains(ua, "mobile") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone"):
		device.DeviceType = "Mobile"
	case strings.Contains(ua, "tablet") || strings.Contains(ua, "ipad"):
		device.DeviceType = "Tablet"
	default:
		device.DeviceType = "Desktop"
	}

	return device
}

func (s *Service) getTopData(ctx context.Context, urlID string, startDate, endDate time.Time) (*TopData, error) {
	// Get top countries
	countries, err := s.collector.GetTopCountries(ctx, urlID, startDate, endDate, 10)
	if err != nil {
		return nil, err
	}

	// Get top referrers
	referrers, err := s.collector.GetTopReferrers(ctx, urlID, startDate, endDate, 10)
	if err != nil {
		return nil, err
	}

	// Get top browsers
	browsers, err := s.collector.GetTopBrowsers(ctx, urlID, startDate, endDate, 10)
	if err != nil {
		return nil, err
	}

	// Get top devices
	devices, err := s.collector.GetTopDevices(ctx, urlID, startDate, endDate, 10)
	if err != nil {
		return nil, err
	}

	return &TopData{
		Countries: countries,
		Referrers: referrers,
		Browsers:  browsers,
		Devices:   devices,
	}, nil
}

func extractDomain(url string) string {
	if url == "" {
		return ""
	}

	// Remove protocol
	if strings.HasPrefix(url, "http://") {
		url = url[7:]
	} else if strings.HasPrefix(url, "https://") {
		url = url[8:]
	}

	// Get domain part
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		domain := parts[0]
		// Remove port if present
		if colonIndex := strings.Index(domain, ":"); colonIndex != -1 {
			domain = domain[:colonIndex]
		}
		return domain
	}

	return ""
}

func generateClickID() (string, error) {
	return generateUUID()
}

func hashIP(ip string) string {
	// Simple hash function for IP addresses
	// In production, use a cryptographic hash function
	h := 0
	for _, c := range ip {
		h = h*31 + int(c)
	}
	return fmt.Sprintf("%x", h)
}

func generateUUID() (string, error) {
	// Simple UUID generation - in production, use a proper UUID library
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Set version (4) and variant bits
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}