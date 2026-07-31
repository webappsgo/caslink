package model

import (
	"errors"
	"time"
)

// URL represents a shortened URL
type URL struct {
	ID           int64      `json:"id"`
	ShortCode    string     `json:"short_code"`
	LongURL      string     `json:"long_url"`
	Title        *string    `json:"title,omitempty"`
	Description  *string    `json:"description,omitempty"`
	UserID       *int64     `json:"user_id,omitempty"`
	OrgID        *int64     `json:"org_id,omitempty"`
	CustomCode   bool       `json:"custom_code"`
	PasswordHash *string    `json:"-"` // Never expose in JSON
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	// Visibility gates GetURL/Stats to the owner (or an admin) when "private";
	// "public" (the default) keeps the prior open-by-code behavior. Never
	// affects the redirect itself — a short link always redirects for anyone
	// who has the code (IDEA.md line 25/37 link options).
	Visibility string `json:"visibility"`
	// Tags are freeform, owner-defined labels for organizing/filtering links.
	Tags []string `json:"tags,omitempty"`
	// UTM* are static query params appended to LongURL (or the device-target
	// override) at redirect time, when not already present on the URL.
	UTMSource   *string `json:"utm_source,omitempty"`
	UTMMedium   *string `json:"utm_medium,omitempty"`
	UTMCampaign *string `json:"utm_campaign,omitempty"`
	UTMTerm     *string `json:"utm_term,omitempty"`
	UTMContent  *string `json:"utm_content,omitempty"`
	// GeoMode/GeoCountries restrict redirect by visitor country (ISO 3166-1
	// alpha-2), mirroring AI.md PART 20's server-level country_mode/
	// allow_countries/deny_countries terminology at the per-link level.
	// "none" (default) applies no restriction.
	GeoMode      string   `json:"geo_mode"`
	GeoCountries []string `json:"geo_countries,omitempty"`
	// Device-target overrides: when set, RedirectURL sends mobile/tablet/
	// desktop visitors to a different destination than LongURL.
	MobileURL  *string   `json:"mobile_url,omitempty"`
	DesktopURL *string   `json:"desktop_url,omitempty"`
	TabletURL  *string   `json:"tablet_url,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateURLRequest represents a request to create a new URL
type CreateURLRequest struct {
	LongURL      string     `json:"url" validate:"required,url,max=2048"`
	CustomCode   string     `json:"custom_code,omitempty" validate:"omitempty,min=3,max=50"`
	Title        *string    `json:"title,omitempty" validate:"omitempty,max=255"`
	Description  *string    `json:"description,omitempty" validate:"omitempty,max=500"`
	Password     string     `json:"password,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	ExpireAfter  string     `json:"expire_after,omitempty" validate:"omitempty,oneof=1h 24h 7d 30d never"`
	Visibility   string     `json:"visibility,omitempty" validate:"omitempty,oneof=public private"`
	Tags         []string   `json:"tags,omitempty"`
	UTMSource    string     `json:"utm_source,omitempty" validate:"omitempty,max=255"`
	UTMMedium    string     `json:"utm_medium,omitempty" validate:"omitempty,max=255"`
	UTMCampaign  string     `json:"utm_campaign,omitempty" validate:"omitempty,max=255"`
	UTMTerm      string     `json:"utm_term,omitempty" validate:"omitempty,max=255"`
	UTMContent   string     `json:"utm_content,omitempty" validate:"omitempty,max=255"`
	GeoMode      string     `json:"geo_mode,omitempty" validate:"omitempty,oneof=none allow deny"`
	GeoCountries []string   `json:"geo_countries,omitempty"`
	MobileURL    string     `json:"mobile_url,omitempty" validate:"omitempty,url,max=2048"`
	DesktopURL   string     `json:"desktop_url,omitempty" validate:"omitempty,url,max=2048"`
	TabletURL    string     `json:"tablet_url,omitempty" validate:"omitempty,url,max=2048"`
}

// UpdateURLRequest represents a request to update an existing URL. Only
// non-nil fields are changed; Tags/GeoCountries use a pointer-to-slice so an
// explicit empty list (clear) is distinguishable from "leave unchanged".
type UpdateURLRequest struct {
	LongURL      *string    `json:"url,omitempty" validate:"omitempty,url,max=2048"`
	Title        *string    `json:"title,omitempty" validate:"omitempty,max=255"`
	Description  *string    `json:"description,omitempty" validate:"omitempty,max=500"`
	Password     *string    `json:"password,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Visibility   *string    `json:"visibility,omitempty" validate:"omitempty,oneof=public private"`
	Tags         *[]string  `json:"tags,omitempty"`
	UTMSource    *string    `json:"utm_source,omitempty" validate:"omitempty,max=255"`
	UTMMedium    *string    `json:"utm_medium,omitempty" validate:"omitempty,max=255"`
	UTMCampaign  *string    `json:"utm_campaign,omitempty" validate:"omitempty,max=255"`
	UTMTerm      *string    `json:"utm_term,omitempty" validate:"omitempty,max=255"`
	UTMContent   *string    `json:"utm_content,omitempty" validate:"omitempty,max=255"`
	GeoMode      *string    `json:"geo_mode,omitempty" validate:"omitempty,oneof=none allow deny"`
	GeoCountries *[]string  `json:"geo_countries,omitempty"`
	MobileURL    *string    `json:"mobile_url,omitempty" validate:"omitempty,url,max=2048"`
	DesktopURL   *string    `json:"desktop_url,omitempty" validate:"omitempty,url,max=2048"`
	TabletURL    *string    `json:"tablet_url,omitempty" validate:"omitempty,url,max=2048"`
}

// Click represents a click/visit to a shortened URL
type Click struct {
	ID        int64     `json:"id"`
	URLID     int64     `json:"url_id"`
	IPHash    string    `json:"-"` // Never expose
	Country   *string   `json:"country,omitempty"`
	City      *string   `json:"city,omitempty"`
	UserAgent *string   `json:"user_agent,omitempty"`
	Referrer  *string   `json:"referrer,omitempty"`
	Browser   *string   `json:"browser,omitempty"`
	OS        *string   `json:"os,omitempty"`
	Device    *string   `json:"device,omitempty"`
	IsBot     bool      `json:"is_bot"`
	ClickedAt time.Time `json:"clicked_at"`
}

// URLStats represents statistics for a URL
type URLStats struct {
	ShortCode    string         `json:"short_code"`
	TotalClicks  int64          `json:"total_clicks"`
	UniqueIPs    int64          `json:"unique_ips"`
	LastClick    *time.Time     `json:"last_click,omitempty"`
	TopCountries []CountryStat  `json:"top_countries,omitempty"`
	TopReferrers []ReferrerStat `json:"top_referrers,omitempty"`
}

// CountryStat represents click statistics by country
type CountryStat struct {
	Country string `json:"country"`
	Clicks  int64  `json:"clicks"`
}

// ReferrerStat represents click statistics by referrer
type ReferrerStat struct {
	Referrer string `json:"referrer"`
	Clicks   int64  `json:"clicks"`
}

// Error definitions
var (
	ErrURLNotFound       = errors.New("URL not found")
	ErrCodeAlreadyExists = errors.New("short code already exists")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrURLExpired        = errors.New("URL has expired")
	ErrInvalidCustomCode = errors.New("invalid custom code")
	ErrReservedWord      = errors.New("short code is a reserved word")
)
