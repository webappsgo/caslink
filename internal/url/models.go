package url

import (
	"errors"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/db"
)

// Error definitions
var (
	ErrURLNotFound          = errors.New("URL not found")
	ErrCodeAlreadyExists    = errors.New("short code already exists")
	ErrPasswordNotRequired  = errors.New("password not required for this URL")
	ErrInvalidPassword      = errors.New("invalid password")
	ErrURLExpired          = errors.New("URL has expired")
	ErrURLInactive         = errors.New("URL is inactive")
)

// CreateURLRequest represents a request to create a new URL
type CreateURLRequest struct {
	OriginalURL string     `json:"original_url" validate:"required,url,max=2048"`
	CustomCode  string     `json:"custom_code,omitempty" validate:"omitempty,min=3,max=50"`
	Title       *string    `json:"title,omitempty" validate:"omitempty,max=255"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=500"`
	Password    string     `json:"password,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ExpireAfter string     `json:"expire_after,omitempty" validate:"omitempty,oneof=1h 24h 7d 30d never"`
	Tags        *string    `json:"tags,omitempty"`
	UserID      *string    `json:"user_id,omitempty"`
	DomainID    *string    `json:"domain_id,omitempty"`

	// UTM parameters
	UTMSource   *string `json:"utm_source,omitempty"`
	UTMMedium   *string `json:"utm_medium,omitempty"`
	UTMCampaign *string `json:"utm_campaign,omitempty"`
	UTMTerm     *string `json:"utm_term,omitempty"`
	UTMContent  *string `json:"utm_content,omitempty"`
}

// UpdateURLRequest represents a request to update an existing URL
type UpdateURLRequest struct {
	OriginalURL *string    `json:"original_url,omitempty" validate:"omitempty,url,max=2048"`
	Title       *string    `json:"title,omitempty" validate:"omitempty,max=255"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=500"`
	Password    *string    `json:"password,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Tags        *string    `json:"tags,omitempty"`
	Active      *bool      `json:"active,omitempty"`

	// UTM parameters
	UTMSource   *string `json:"utm_source,omitempty"`
	UTMMedium   *string `json:"utm_medium,omitempty"`
	UTMCampaign *string `json:"utm_campaign,omitempty"`
	UTMTerm     *string `json:"utm_term,omitempty"`
	UTMContent  *string `json:"utm_content,omitempty"`
}

// ListURLsRequest represents a request to list URLs with filtering and pagination
type ListURLsRequest struct {
	UserID        *string `json:"user_id,omitempty"`
	DomainID      *string `json:"domain_id,omitempty"`
	Active        *bool   `json:"active,omitempty"`
	Search        *string `json:"search,omitempty"`
	Tags          *string `json:"tags,omitempty"`
	SortBy        *string `json:"sort_by,omitempty" validate:"omitempty,oneof=created_at updated_at clicks unique_clicks"`
	SortDirection *string `json:"sort_direction,omitempty" validate:"omitempty,oneof=asc desc"`
	Limit         *int64  `json:"limit,omitempty" validate:"omitempty,min=1,max=100"`
	Offset        *int64  `json:"offset,omitempty" validate:"omitempty,min=0"`
}

// ListURLsResponse represents the response for listing URLs
type ListURLsResponse struct {
	URLs   []*db.URL `json:"urls"`
	Total  int64     `json:"total"`
	Limit  int64     `json:"limit"`
	Offset int64     `json:"offset"`
}

// URLStats represents statistics for a URL
type URLStats struct {
	URL          *db.URL           `json:"url"`
	TotalClicks  int64            `json:"total_clicks"`
	UniqueClicks int64            `json:"unique_clicks"`
	ClicksByDay  map[string]int64 `json:"clicks_by_day"`
	TopCountries []CountryStats   `json:"top_countries"`
	TopReferrers []ReferrerStats  `json:"top_referrers"`
	TopBrowsers  []BrowserStats   `json:"top_browsers"`
	TopDevices   []DeviceStats    `json:"top_devices"`
}

// CountryStats represents click statistics by country
type CountryStats struct {
	CountryCode string `json:"country_code"`
	CountryName string `json:"country_name"`
	Clicks      int64  `json:"clicks"`
	Percentage  float64 `json:"percentage"`
}

// ReferrerStats represents click statistics by referrer
type ReferrerStats struct {
	Domain     string  `json:"domain"`
	Clicks     int64   `json:"clicks"`
	Percentage float64 `json:"percentage"`
}

// BrowserStats represents click statistics by browser
type BrowserStats struct {
	Browser    string  `json:"browser"`
	Clicks     int64   `json:"clicks"`
	Percentage float64 `json:"percentage"`
}

// DeviceStats represents click statistics by device type
type DeviceStats struct {
	Device     string  `json:"device"`
	Clicks     int64   `json:"clicks"`
	Percentage float64 `json:"percentage"`
}

// ValidatePasswordRequest represents a password validation request
type ValidatePasswordRequest struct {
	Password string `json:"password" validate:"required"`
}

// URLResponse represents a URL in API responses
type URLResponse struct {
	ID           string     `json:"id"`
	OriginalURL  string     `json:"original_url"`
	ShortURL     string     `json:"short_url"`
	IsCustom     bool       `json:"is_custom"`
	Title        *string    `json:"title"`
	Description  *string    `json:"description"`
	FaviconURL   *string    `json:"favicon_url"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
	Clicks       int64      `json:"clicks"`
	UniqueClicks int64      `json:"unique_clicks"`
	Active       bool       `json:"active"`
	Protected    bool       `json:"protected"`
	Tags         *string    `json:"tags"`

	// UTM parameters
	UTMSource   *string `json:"utm_source"`
	UTMMedium   *string `json:"utm_medium"`
	UTMCampaign *string `json:"utm_campaign"`
	UTMTerm     *string `json:"utm_term"`
	UTMContent  *string `json:"utm_content"`
}

// ToURLResponse converts a db.URL to URLResponse
func ToURLResponse(urlRecord *db.URL, baseURL string) *URLResponse {
	shortURL := baseURL + "/" + urlRecord.ID

	return &URLResponse{
		ID:           urlRecord.ID,
		OriginalURL:  urlRecord.OriginalURL,
		ShortURL:     shortURL,
		IsCustom:     urlRecord.IsCustom,
		Title:        urlRecord.Title,
		Description:  urlRecord.Description,
		FaviconURL:   urlRecord.FaviconURL,
		CreatedAt:    urlRecord.CreatedAt,
		UpdatedAt:    urlRecord.UpdatedAt,
		ExpiresAt:    urlRecord.ExpiresAt,
		Clicks:       urlRecord.Clicks,
		UniqueClicks: urlRecord.UniqueClicks,
		Active:       urlRecord.Active,
		Protected:    urlRecord.Password != nil,
		Tags:         urlRecord.Tags,
		UTMSource:    urlRecord.UTMSource,
		UTMMedium:    urlRecord.UTMMedium,
		UTMCampaign:  urlRecord.UTMCampaign,
		UTMTerm:      urlRecord.UTMTerm,
		UTMContent:   urlRecord.UTMContent,
	}
}

// BulkCreateURLRequest represents a request to create multiple URLs
type BulkCreateURLRequest struct {
	URLs []CreateURLRequest `json:"urls" validate:"required,min=1,max=1000"`
}

// BulkCreateURLResponse represents the response for bulk URL creation
type BulkCreateURLResponse struct {
	Created []*URLResponse       `json:"created"`
	Failed  []BulkCreateError    `json:"failed"`
	Total   int                  `json:"total"`
	Success int                  `json:"success"`
	Errors  int                  `json:"errors"`
}

// BulkCreateError represents an error in bulk creation
type BulkCreateError struct {
	Index       int    `json:"index"`
	OriginalURL string `json:"original_url"`
	Error       string `json:"error"`
}

// URLPreview represents a preview of a URL's metadata
type URLPreview struct {
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	FaviconURL  string  `json:"favicon_url"`
	ImageURL    string  `json:"image_url"`
	SiteName    string  `json:"site_name"`
}

// SuggestionRequest represents a request for short code suggestions
type SuggestionRequest struct {
	OriginalURL string `json:"original_url" validate:"required,url"`
	Count       int    `json:"count,omitempty" validate:"omitempty,min=1,max=10"`
}

// SuggestionResponse represents suggested short codes
type SuggestionResponse struct {
	Suggestions []string `json:"suggestions"`
}

// URLHealth represents the health status of a URL
type URLHealth struct {
	URL          string    `json:"url"`
	ShortCode    string    `json:"short_code"`
	Status       string    `json:"status"` // "active", "expired", "inactive", "unreachable"
	StatusCode   int       `json:"status_code"`
	ResponseTime int64     `json:"response_time_ms"`
	LastChecked  time.Time `json:"last_checked"`
	Error        string    `json:"error,omitempty"`
}

// ExportFormat represents supported export formats
type ExportFormat string

const (
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatJSON ExportFormat = "json"
	ExportFormatXML  ExportFormat = "xml"
)

// ExportRequest represents a request to export URLs
type ExportRequest struct {
	Format    ExportFormat `json:"format" validate:"required,oneof=csv json xml"`
	UserID    *string      `json:"user_id,omitempty"`
	DomainID  *string      `json:"domain_id,omitempty"`
	Active    *bool        `json:"active,omitempty"`
	DateFrom  *time.Time   `json:"date_from,omitempty"`
	DateTo    *time.Time   `json:"date_to,omitempty"`
	IncludeStats bool      `json:"include_stats,omitempty"`
}

// ImportRequest represents a request to import URLs
type ImportRequest struct {
	Format    ExportFormat `json:"format" validate:"required,oneof=csv json"`
	Data      string       `json:"data" validate:"required"`
	UserID    *string      `json:"user_id,omitempty"`
	DomainID  *string      `json:"domain_id,omitempty"`
	Overwrite bool         `json:"overwrite,omitempty"`
}

// ImportResponse represents the response for URL import
type ImportResponse struct {
	Imported []*URLResponse    `json:"imported"`
	Failed   []BulkCreateError `json:"failed"`
	Total    int               `json:"total"`
	Success  int               `json:"success"`
	Errors   int               `json:"errors"`
	Skipped  int               `json:"skipped"`
}