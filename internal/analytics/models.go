package analytics

import (
	"time"
)

// Click represents a single click event
type Click struct {
	ID              string        `json:"id"`
	URLID           string        `json:"url_id"`
	ClickedAt       time.Time     `json:"clicked_at"`
	IPAddress       string        `json:"ip_address"`
	IPHash          string        `json:"ip_hash"`
	UserAgent       string        `json:"user_agent"`
	Referrer        string        `json:"referrer"`
	ReferrerDomain  string        `json:"referrer_domain"`
	IsUnique        bool          `json:"is_unique"`
	IsBot           bool          `json:"is_bot"`
	Location        *LocationData `json:"location,omitempty"`
	DeviceInfo      *DeviceInfo   `json:"device_info,omitempty"`
}

// LocationData represents geographic information
type LocationData struct {
	CountryCode string  `json:"country_code"`
	CountryName string  `json:"country_name"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Timezone    string  `json:"timezone"`
}

// DeviceInfo represents device and browser information
type DeviceInfo struct {
	UserAgent    string `json:"user_agent"`
	Browser      string `json:"browser"`
	BrowserVersion string `json:"browser_version"`
	OS           string `json:"os"`
	OSVersion    string `json:"os_version"`
	DeviceType   string `json:"device_type"`
	DeviceBrand  string `json:"device_brand"`
	DeviceModel  string `json:"device_model"`
}

// DailyStat represents aggregated statistics for a single day
type DailyStat struct {
	Date         time.Time `json:"date"`
	URLID        string    `json:"url_id"`
	Clicks       int64     `json:"clicks"`
	UniqueClicks int64     `json:"unique_clicks"`
	TopCountries []CountryStat `json:"top_countries"`
	TopReferrers []ReferrerStat `json:"top_referrers"`
	TopBrowsers  []BrowserStat `json:"top_browsers"`
	TopDevices   []DeviceStat `json:"top_devices"`
}

// CountryStat represents click statistics by country
type CountryStat struct {
	CountryCode string `json:"country_code"`
	CountryName string `json:"country_name"`
	Clicks      int64  `json:"clicks"`
	Percentage  float64 `json:"percentage"`
}

// ReferrerStat represents click statistics by referrer
type ReferrerStat struct {
	Domain     string  `json:"domain"`
	Clicks     int64   `json:"clicks"`
	Percentage float64 `json:"percentage"`
}

// BrowserStat represents click statistics by browser
type BrowserStat struct {
	Browser    string  `json:"browser"`
	Clicks     int64   `json:"clicks"`
	Percentage float64 `json:"percentage"`
}

// DeviceStat represents click statistics by device type
type DeviceStat struct {
	DeviceType string  `json:"device_type"`
	Clicks     int64   `json:"clicks"`
	Percentage float64 `json:"percentage"`
}

// URLStat represents statistics for a single URL
type URLStat struct {
	URLID        string    `json:"url_id"`
	OriginalURL  string    `json:"original_url"`
	Title        string    `json:"title"`
	Clicks       int64     `json:"clicks"`
	UniqueClicks int64     `json:"unique_clicks"`
	CreatedAt    time.Time `json:"created_at"`
	LastClickAt  *time.Time `json:"last_click_at"`
}

// Request/Response types

// ClickRequest represents a request to track a click
type ClickRequest struct {
	URLID     string `json:"url_id"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
	Referrer  string `json:"referrer"`
}

// ClickResponse represents the response to a click tracking request
type ClickResponse struct {
	Tracked  bool   `json:"tracked"`
	ClickID  string `json:"click_id,omitempty"`
	IsUnique bool   `json:"is_unique"`
	Reason   string `json:"reason,omitempty"`
}

// URLAnalyticsRequest represents a request for URL analytics
type URLAnalyticsRequest struct {
	URLID     string    `json:"url_id"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

// URLAnalyticsResponse represents URL analytics data
type URLAnalyticsResponse struct {
	URLID        string         `json:"url_id"`
	StartDate    time.Time      `json:"start_date"`
	EndDate      time.Time      `json:"end_date"`
	TotalClicks  int64          `json:"total_clicks"`
	UniqueClicks int64          `json:"unique_clicks"`
	DailyStats   []DailyStat    `json:"daily_stats"`
	TopCountries []CountryStat  `json:"top_countries"`
	TopReferrers []ReferrerStat `json:"top_referrers"`
	TopBrowsers  []BrowserStat  `json:"top_browsers"`
	TopDevices   []DeviceStat   `json:"top_devices"`
	RecentClicks []Click        `json:"recent_clicks"`
}

// UserAnalyticsRequest represents a request for user analytics
type UserAnalyticsRequest struct {
	UserID    string    `json:"user_id"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

// UserAnalyticsResponse represents user analytics data
type UserAnalyticsResponse struct {
	UserID       string    `json:"user_id"`
	StartDate    time.Time `json:"start_date"`
	EndDate      time.Time `json:"end_date"`
	TotalClicks  int64     `json:"total_clicks"`
	UniqueClicks int64     `json:"unique_clicks"`
	TotalURLs    int64     `json:"total_urls"`
	URLStats     []URLStat `json:"url_stats"`
}

// GlobalAnalyticsRequest represents a request for global analytics
type GlobalAnalyticsRequest struct {
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

// GlobalAnalyticsResponse represents global analytics data
type GlobalAnalyticsResponse struct {
	StartDate    time.Time   `json:"start_date"`
	EndDate      time.Time   `json:"end_date"`
	TotalClicks  int64       `json:"total_clicks"`
	UniqueClicks int64       `json:"unique_clicks"`
	TotalURLs    int64       `json:"total_urls"`
	TotalUsers   int64       `json:"total_users"`
	DailyStats   []DailyStat `json:"daily_stats"`
}

// RealTimeStatsRequest represents a request for real-time statistics
type RealTimeStatsRequest struct {
	TimeWindow time.Duration `json:"time_window"`
}

// RealTimeStatsResponse represents real-time statistics
type RealTimeStatsResponse struct {
	TimeWindow     time.Duration `json:"time_window"`
	ClicksInWindow int64         `json:"clicks_in_window"`
	ActiveURLs     int64         `json:"active_urls"`
	TopURLs        []URLStat     `json:"top_urls"`
	TopCountries   []CountryStat `json:"top_countries"`
	RecentClicks   []Click       `json:"recent_clicks"`
}

// ExportRequest represents a request to export analytics data
type ExportRequest struct {
	UserID    *string   `json:"user_id,omitempty"` // For user-specific exports
	URLIDs    []string  `json:"url_ids,omitempty"` // For specific URLs
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	Format    string    `json:"format"` // csv, json, pdf
	Fields    []string  `json:"fields,omitempty"` // Specific fields to include
}

// ExportResponse represents the response to an export request
type ExportResponse struct {
	Content     []byte `json:"content"`
	ContentType string `json:"content_type"`
	Filename    string `json:"filename"`
}

// CleanupResult represents the result of a data cleanup operation
type CleanupResult struct {
	Cleaned       bool      `json:"cleaned"`
	ClicksDeleted int64     `json:"clicks_deleted"`
	StatsDeleted  int64     `json:"stats_deleted"`
	Reason        string    `json:"reason,omitempty"`
	Duration      time.Duration `json:"duration"`
}

// Analytics aggregation types

// URLAnalytics represents analytics data for a URL
type URLAnalytics struct {
	URLID        string  `json:"url_id"`
	TotalClicks  int64   `json:"total_clicks"`
	UniqueClicks int64   `json:"unique_clicks"`
	RecentClicks []Click `json:"recent_clicks"`
}

// UserAnalytics represents analytics data for a user
type UserAnalytics struct {
	UserID       string `json:"user_id"`
	TotalClicks  int64  `json:"total_clicks"`
	UniqueClicks int64  `json:"unique_clicks"`
	TotalURLs    int64  `json:"total_urls"`
}

// GlobalAnalytics represents global analytics data
type GlobalAnalytics struct {
	TotalClicks  int64       `json:"total_clicks"`
	UniqueClicks int64       `json:"unique_clicks"`
	TotalURLs    int64       `json:"total_urls"`
	TotalUsers   int64       `json:"total_users"`
	DailyStats   []DailyStat `json:"daily_stats"`
}

// TopData represents top statistics
type TopData struct {
	Countries []CountryStat  `json:"countries"`
	Referrers []ReferrerStat `json:"referrers"`
	Browsers  []BrowserStat  `json:"browsers"`
	Devices   []DeviceStat   `json:"devices"`
}

// Geographic data types

// Country represents a country with click statistics
type Country struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Clicks int64 `json:"clicks"`
}

// Referrer represents a referrer domain with click statistics
type Referrer struct {
	Domain string `json:"domain"`
	Clicks int64  `json:"clicks"`
}

// Browser represents a browser with click statistics
type Browser struct {
	Name   string `json:"name"`
	Clicks int64  `json:"clicks"`
}

// Device represents a device type with click statistics
type Device struct {
	Type   string `json:"type"`
	Clicks int64  `json:"clicks"`
}

// Filter types for analytics queries

// ClickFilter represents filters for click queries
type ClickFilter struct {
	URLIDs      []string  `json:"url_ids,omitempty"`
	UserID      *string   `json:"user_id,omitempty"`
	StartDate   time.Time `json:"start_date"`
	EndDate     time.Time `json:"end_date"`
	Countries   []string  `json:"countries,omitempty"`
	Referrers   []string  `json:"referrers,omitempty"`
	Browsers    []string  `json:"browsers,omitempty"`
	DeviceTypes []string  `json:"device_types,omitempty"`
	ExcludeBots bool      `json:"exclude_bots"`
	UniqueOnly  bool      `json:"unique_only"`
}

// Aggregation types

// TimeInterval represents different time intervals for aggregation
type TimeInterval string

const (
	IntervalHour  TimeInterval = "hour"
	IntervalDay   TimeInterval = "day"
	IntervalWeek  TimeInterval = "week"
	IntervalMonth TimeInterval = "month"
	IntervalYear  TimeInterval = "year"
)

// AggregationRequest represents a request for aggregated data
type AggregationRequest struct {
	Filter   ClickFilter  `json:"filter"`
	Interval TimeInterval `json:"interval"`
	GroupBy  []string     `json:"group_by,omitempty"` // country, referrer, browser, device
	Limit    int          `json:"limit,omitempty"`
}

// AggregationResponse represents aggregated analytics data
type AggregationResponse struct {
	Interval TimeInterval         `json:"interval"`
	GroupBy  []string             `json:"group_by"`
	Data     []AggregationDataPoint `json:"data"`
}

// AggregationDataPoint represents a single data point in aggregated data
type AggregationDataPoint struct {
	Timestamp    time.Time            `json:"timestamp"`
	Clicks       int64                `json:"clicks"`
	UniqueClicks int64                `json:"unique_clicks"`
	Groups       map[string]int64     `json:"groups,omitempty"` // Group name -> click count
}

// Report types

// ReportType represents different types of reports
type ReportType string

const (
	ReportTypeOverview    ReportType = "overview"
	ReportTypeTraffic     ReportType = "traffic"
	ReportTypeGeographic  ReportType = "geographic"
	ReportTypeTechnology  ReportType = "technology"
	ReportTypeReferrers   ReportType = "referrers"
	ReportTypeCustom      ReportType = "custom"
)

// ReportRequest represents a request for a report
type ReportRequest struct {
	Type      ReportType  `json:"type"`
	Filter    ClickFilter `json:"filter"`
	Format    string      `json:"format"` // html, pdf, csv, json
	Options   map[string]interface{} `json:"options,omitempty"`
}

// ReportResponse represents a generated report
type ReportResponse struct {
	Type        ReportType `json:"type"`
	GeneratedAt time.Time  `json:"generated_at"`
	Content     []byte     `json:"content"`
	ContentType string     `json:"content_type"`
	Filename    string     `json:"filename"`
}

// Event tracking types

// EventType represents different types of events to track
type EventType string

const (
	EventClick     EventType = "click"
	EventView      EventType = "view"
	EventDownload  EventType = "download"
	EventShare     EventType = "share"
	EventConversion EventType = "conversion"
)

// Event represents a trackable event
type Event struct {
	ID         string                 `json:"id"`
	Type       EventType              `json:"type"`
	URLID      string                 `json:"url_id"`
	Timestamp  time.Time              `json:"timestamp"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	UserID     *string                `json:"user_id,omitempty"`
	SessionID  *string                `json:"session_id,omitempty"`
	IPAddress  string                 `json:"ip_address"`
	UserAgent  string                 `json:"user_agent"`
}

// EventRequest represents a request to track an event
type EventRequest struct {
	Type       EventType              `json:"type"`
	URLID      string                 `json:"url_id"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	UserID     *string                `json:"user_id,omitempty"`
	SessionID  *string                `json:"session_id,omitempty"`
}

// EventResponse represents the response to an event tracking request
type EventResponse struct {
	Tracked bool   `json:"tracked"`
	EventID string `json:"event_id,omitempty"`
	Reason  string `json:"reason,omitempty"`
}