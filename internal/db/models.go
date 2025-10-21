package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// URL represents a shortened URL
type URL struct {
	ID              string    `json:"id" db:"id"`
	OriginalURL     string    `json:"original_url" db:"original_url"`
	IsCustom        bool      `json:"is_custom" db:"is_custom"`
	Title           *string   `json:"title" db:"title"`
	Description     *string   `json:"description" db:"description"`
	FaviconURL      *string   `json:"favicon_url" db:"favicon_url"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
	ExpiresAt       *time.Time `json:"expires_at" db:"expires_at"`
	Clicks          int64     `json:"clicks" db:"clicks"`
	UniqueClicks    int64     `json:"unique_clicks" db:"unique_clicks"`
	UserID          *string   `json:"user_id" db:"user_id"`
	DomainID        *string   `json:"domain_id" db:"domain_id"`
	Active          bool      `json:"active" db:"active"`
	Password        *string   `json:"-" db:"password"`
	Tags            *string   `json:"tags" db:"tags"`
	UTMSource       *string   `json:"utm_source" db:"utm_source"`
	UTMMedium       *string   `json:"utm_medium" db:"utm_medium"`
	UTMCampaign     *string   `json:"utm_campaign" db:"utm_campaign"`
	UTMTerm         *string   `json:"utm_term" db:"utm_term"`
	UTMContent      *string   `json:"utm_content" db:"utm_content"`
}

// Click represents a click event
type Click struct {
	ID               string     `json:"id" db:"id"`
	URLID            string     `json:"url_id" db:"url_id"`
	ClickedAt        time.Time  `json:"clicked_at" db:"clicked_at"`
	IPAddress        *string    `json:"ip_address" db:"ip_address"`
	IPHash           string     `json:"ip_hash" db:"ip_hash"`
	UserAgent        *string    `json:"user_agent" db:"user_agent"`
	ParsedBrowser    *string    `json:"parsed_browser" db:"parsed_browser"`
	ParsedOS         *string    `json:"parsed_os" db:"parsed_os"`
	ParsedDevice     *string    `json:"parsed_device" db:"parsed_device"`
	Referrer         *string    `json:"referrer" db:"referrer"`
	ReferrerDomain   *string    `json:"referrer_domain" db:"referrer_domain"`
	CountryCode      *string    `json:"country_code" db:"country_code"`
	CountryName      *string    `json:"country_name" db:"country_name"`
	Region           *string    `json:"region" db:"region"`
	City             *string    `json:"city" db:"city"`
	Latitude         *float64   `json:"latitude" db:"latitude"`
	Longitude        *float64   `json:"longitude" db:"longitude"`
	Timezone         *string    `json:"timezone" db:"timezone"`
	IsBot            bool       `json:"is_bot" db:"is_bot"`
	IsUnique         bool       `json:"is_unique" db:"is_unique"`
}

// ClickDailyStats represents aggregated daily click statistics
type ClickDailyStats struct {
	ID            string          `json:"id" db:"id"`
	URLID         string          `json:"url_id" db:"url_id"`
	Date          time.Time       `json:"date" db:"date"`
	Clicks        int64           `json:"clicks" db:"clicks"`
	UniqueClicks  int64           `json:"unique_clicks" db:"unique_clicks"`
	TopCountries  JSONStringSlice `json:"top_countries" db:"top_countries"`
	TopReferrers  JSONStringSlice `json:"top_referrers" db:"top_referrers"`
	TopBrowsers   JSONStringSlice `json:"top_browsers" db:"top_browsers"`
	TopDevices    JSONStringSlice `json:"top_devices" db:"top_devices"`
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
}

// Upload represents a file upload associated with a URL
type Upload struct {
	ID        string    `json:"id" db:"id"`
	URLID     string    `json:"url_id" db:"url_id"`
	Filename  string    `json:"filename" db:"filename"`
	MimeType  string    `json:"mime_type" db:"mime_type"`
	Size      int64     `json:"size" db:"size"`
	Data      []byte    `json:"-" db:"data"`
	UploadedAt time.Time `json:"uploaded_at" db:"uploaded_at"`
}

// QRCode represents a cached QR code
type QRCode struct {
	ID               string    `json:"id" db:"id"`
	URLID            string    `json:"url_id" db:"url_id"`
	Format           string    `json:"format" db:"format"`
	Size             int       `json:"size" db:"size"`
	Style            string    `json:"style" db:"style"`
	ForegroundColor  *string   `json:"foreground_color" db:"foreground_color"`
	BackgroundColor  *string   `json:"background_color" db:"background_color"`
	LogoURL          *string   `json:"logo_url" db:"logo_url"`
	Data             []byte    `json:"-" db:"data"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

// User represents a user account
type User struct {
	ID                    string     `json:"id" db:"id"`
	Username              string     `json:"username" db:"username"`
	Email                 *string    `json:"email" db:"email"`
	PasswordHash          string     `json:"-" db:"password_hash"`
	IsAdmin               bool       `json:"is_admin" db:"is_admin"`
	IsPremium             bool       `json:"is_premium" db:"is_premium"`
	PremiumExpiresAt      *time.Time `json:"premium_expires_at" db:"premium_expires_at"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
	LastLogin             *time.Time `json:"last_login" db:"last_login"`
	LastActive            *time.Time `json:"last_active" db:"last_active"`
	TwoFASecret           string     `json:"-" db:"two_fa_secret"`
	TwoFAEnabled          bool       `json:"two_fa_enabled" db:"two_fa_enabled"`
	WebAuthnCredentials   string     `json:"-" db:"webauthn_credentials"`
	APIRateLimit          int        `json:"api_rate_limit" db:"api_rate_limit"`
	URLLimit              int        `json:"url_limit" db:"url_limit"`
	Timezone              string     `json:"timezone" db:"timezone"`
	Language              string     `json:"language" db:"language"`
	Theme                 string     `json:"theme" db:"theme"`
}

// Domain represents a custom domain
type Domain struct {
	ID                  string     `json:"id" db:"id"`
	Domain              string     `json:"domain" db:"domain"`
	UserID              string     `json:"user_id" db:"user_id"`
	IsDefault           bool       `json:"is_default" db:"is_default"`
	SSLEnabled          bool       `json:"ssl_enabled" db:"ssl_enabled"`
	SSLCertPath         *string    `json:"ssl_cert_path" db:"ssl_cert_path"`
	SSLKeyPath          *string    `json:"ssl_key_path" db:"ssl_key_path"`
	Verified            bool       `json:"verified" db:"verified"`
	VerificationToken   string     `json:"verification_token" db:"verification_token"`
	VerificationMethod  string     `json:"verification_method" db:"verification_method"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	VerifiedAt          *time.Time `json:"verified_at" db:"verified_at"`
}

// APIToken represents an API token
type APIToken struct {
	ID          string     `json:"id" db:"id"`
	UserID      string     `json:"user_id" db:"user_id"`
	Name        string     `json:"name" db:"name"`
	Token       string     `json:"token" db:"token"`
	Permissions JSONStringSlice `json:"permissions" db:"permissions"`
	RateLimit   int        `json:"rate_limit" db:"rate_limit"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at" db:"expires_at"`
	LastUsed    *time.Time `json:"last_used" db:"last_used"`
	LastUsedIP  *string    `json:"last_used_ip" db:"last_used_ip"`
	Active      bool       `json:"active" db:"active"`
}

// Session represents a user session
type Session struct {
	ID           string    `json:"id" db:"id"`
	UserID       string    `json:"user_id" db:"user_id"`
	Data         string    `json:"-" db:"data"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	LastAccessed time.Time `json:"last_accessed" db:"last_accessed"`
	IPAddress    *string   `json:"ip_address" db:"ip_address"`
	UserAgent    *string   `json:"user_agent" db:"user_agent"`
}

// ServerConfig represents server configuration
type ServerConfig struct {
	Key         string    `json:"key" db:"key"`
	Value       string    `json:"value" db:"value"`
	Type        string    `json:"type" db:"type"`
	Description *string   `json:"description" db:"description"`
	UpdatedBy   *string   `json:"updated_by" db:"updated_by"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID           string     `json:"id" db:"id"`
	UserID       *string    `json:"user_id" db:"user_id"`
	Action       string     `json:"action" db:"action"`
	ResourceType string     `json:"resource_type" db:"resource_type"`
	ResourceID   *string    `json:"resource_id" db:"resource_id"`
	OldValues    *string    `json:"old_values" db:"old_values"`
	NewValues    *string    `json:"new_values" db:"new_values"`
	IPAddress    *string    `json:"ip_address" db:"ip_address"`
	UserAgent    *string    `json:"user_agent" db:"user_agent"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	Success      bool       `json:"success" db:"success"`
	ErrorMessage *string    `json:"error_message" db:"error_message"`
}

// BillingPlan represents a billing plan
type BillingPlan struct {
	ID           string          `json:"id" db:"id"`
	Name         string          `json:"name" db:"name"`
	DisplayName  string          `json:"display_name" db:"display_name"`
	Description  *string         `json:"description" db:"description"`
	PriceMonthly int64           `json:"price_monthly" db:"price_monthly"`
	PriceYearly  int64           `json:"price_yearly" db:"price_yearly"`
	Currency     string          `json:"currency" db:"currency"`
	Features     JSONStringSlice `json:"features" db:"features"`
	Limits       string          `json:"limits" db:"limits"`
	TrialDays    int             `json:"trial_days" db:"trial_days"`
	Active       bool            `json:"active" db:"active"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

// Subscription represents a user subscription
type Subscription struct {
	ID                     string     `json:"id" db:"id"`
	UserID                 string     `json:"user_id" db:"user_id"`
	PlanID                 string     `json:"plan_id" db:"plan_id"`
	ProviderSubscriptionID *string    `json:"provider_subscription_id" db:"provider_subscription_id"`
	Status                 string     `json:"status" db:"status"`
	BillingCycle           string     `json:"billing_cycle" db:"billing_cycle"`
	CurrentPeriodStart     time.Time  `json:"current_period_start" db:"current_period_start"`
	CurrentPeriodEnd       time.Time  `json:"current_period_end" db:"current_period_end"`
	TrialStart             *time.Time `json:"trial_start" db:"trial_start"`
	TrialEnd               *time.Time `json:"trial_end" db:"trial_end"`
	CancelAtPeriodEnd      bool       `json:"cancel_at_period_end" db:"cancel_at_period_end"`
	CanceledAt             *time.Time `json:"canceled_at" db:"canceled_at"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at" db:"updated_at"`
}

// UsageRecord represents usage tracking
type UsageRecord struct {
	ID             string    `json:"id" db:"id"`
	UserID         string    `json:"user_id" db:"user_id"`
	SubscriptionID string    `json:"subscription_id" db:"subscription_id"`
	MetricName     string    `json:"metric_name" db:"metric_name"`
	Quantity       int64     `json:"quantity" db:"quantity"`
	Timestamp      time.Time `json:"timestamp" db:"timestamp"`
	BillingPeriod  string    `json:"billing_period" db:"billing_period"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// FederationInstance represents a federated instance
type FederationInstance struct {
	ID           string     `json:"id" db:"id"`
	Domain       string     `json:"domain" db:"domain"`
	PublicKey    string     `json:"public_key" db:"public_key"`
	DiscoveredAt time.Time  `json:"discovered_at" db:"discovered_at"`
	LastSync     *time.Time `json:"last_sync" db:"last_sync"`
	Active       bool       `json:"active" db:"active"`
	Blocked      bool       `json:"blocked" db:"blocked"`
	SyncEnabled  bool       `json:"sync_enabled" db:"sync_enabled"`
}

// FederatedURL represents a URL from a federated instance
type FederatedURL struct {
	ID             string    `json:"id" db:"id"`
	OriginalID     string    `json:"original_id" db:"original_id"`
	SourceInstance string    `json:"source_instance" db:"source_instance"`
	OriginalURL    string    `json:"original_url" db:"original_url"`
	ShortCode      string    `json:"short_code" db:"short_code"`
	Title          *string   `json:"title" db:"title"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	SyncedAt       time.Time `json:"synced_at" db:"synced_at"`
}

// JSONStringSlice is a custom type for handling JSON arrays in database
type JSONStringSlice []string

// Value implements the driver.Valuer interface
func (j JSONStringSlice) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
func (j *JSONStringSlice) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONStringSlice", value)
	}

	return json.Unmarshal(bytes, j)
}

// Notification represents a notification message
type Notification struct {
	ID        string                 `json:"id" db:"id"`
	UserID    *string                `json:"user_id" db:"user_id"`
	Type      string                 `json:"type" db:"type"`
	Channel   string                 `json:"channel" db:"channel"`
	Subject   string                 `json:"subject" db:"subject"`
	Content   string                 `json:"content" db:"content"`
	Data      map[string]interface{} `json:"data" db:"data"`
	Status    string                 `json:"status" db:"status"`
	CreatedAt time.Time              `json:"created_at" db:"created_at"`
	SentAt    *time.Time             `json:"sent_at" db:"sent_at"`
	ReadAt    *time.Time             `json:"read_at" db:"read_at"`
	ExpiresAt *time.Time             `json:"expires_at" db:"expires_at"`
	Priority  int                    `json:"priority" db:"priority"`
	Retries   int                    `json:"retries" db:"retries"`
	MaxRetries int                   `json:"max_retries" db:"max_retries"`
	LastError *string                `json:"last_error" db:"last_error"`
}

// NotificationPreferences represents user notification preferences
type NotificationPreferences struct {
	ID                    string     `json:"id" db:"id"`
	UserID                string     `json:"user_id" db:"user_id"`
	EmailAddress          string     `json:"email_address" db:"email_address"`
	PhoneNumber           *string    `json:"phone_number" db:"phone_number"`
	PushToken             *string    `json:"push_token" db:"push_token"`
	WebhookURL            *string    `json:"webhook_url" db:"webhook_url"`
	EnableEmail           bool       `json:"enable_email" db:"enable_email"`
	EnableSMS             bool       `json:"enable_sms" db:"enable_sms"`
	EnablePush            bool       `json:"enable_push" db:"enable_push"`
	EnableWebhook         bool       `json:"enable_webhook" db:"enable_webhook"`
	NotificationTypes     JSONStringSlice `json:"notification_types" db:"notification_types"`
	QuietHoursStart       *string    `json:"quiet_hours_start" db:"quiet_hours_start"`
	QuietHoursEnd         *string    `json:"quiet_hours_end" db:"quiet_hours_end"`
	Timezone              string     `json:"timezone" db:"timezone"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at" db:"updated_at"`
}

// NotificationTemplate represents a notification template
type NotificationTemplate struct {
	ID          string                 `json:"id" db:"id"`
	Name        string                 `json:"name" db:"name"`
	Type        string                 `json:"type" db:"type"`
	Channel     string                 `json:"channel" db:"channel"`
	Subject     string                 `json:"subject" db:"subject"`
	BodyText    string                 `json:"body_text" db:"body_text"`
	BodyHTML    *string                `json:"body_html" db:"body_html"`
	Variables   JSONStringSlice        `json:"variables" db:"variables"`
	Metadata    map[string]interface{} `json:"metadata" db:"metadata"`
	Active      bool                   `json:"active" db:"active"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
}

// NotificationLog represents notification delivery logs
type NotificationLog struct {
	ID             string     `json:"id" db:"id"`
	NotificationID string     `json:"notification_id" db:"notification_id"`
	Channel        string     `json:"channel" db:"channel"`
	Provider       string     `json:"provider" db:"provider"`
	Status         string     `json:"status" db:"status"`
	DeliveryID     *string    `json:"delivery_id" db:"delivery_id"`
	Response       *string    `json:"response" db:"response"`
	ErrorMessage   *string    `json:"error_message" db:"error_message"`
	AttemptNumber  int        `json:"attempt_number" db:"attempt_number"`
	DeliveredAt    *time.Time `json:"delivered_at" db:"delivered_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}