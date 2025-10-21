package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config represents the complete application configuration
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Application   ApplicationConfig   `mapstructure:"application"`
	URL           URLConfig           `mapstructure:"url"`
	Analytics     AnalyticsConfig     `mapstructure:"analytics"`
	QR            QRConfig            `mapstructure:"qr"`
	Bulk          BulkConfig          `mapstructure:"bulk"`
	Auth          AuthConfig          `mapstructure:"auth"`
	OAuth         OAuthConfig         `mapstructure:"oauth"`
	Domains       DomainsConfig       `mapstructure:"domains"`
	Federation    FederationConfig    `mapstructure:"federation"`
	Webhooks      WebhooksConfig      `mapstructure:"webhooks"`
	Billing       BillingConfig       `mapstructure:"billing"`
	RateLimit     RateLimitConfig     `mapstructure:"rate_limit"`
	Security      SecurityConfig      `mapstructure:"security"`
	Logging       LoggingConfig       `mapstructure:"logging"`
	Monitoring    MonitoringConfig    `mapstructure:"monitoring"`
	Email         EmailConfig         `mapstructure:"email"`
	Notifications NotificationConfig  `mapstructure:"notifications"`
	Cache         CacheConfig         `mapstructure:"cache"`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Host                string        `mapstructure:"host"`
	Port                int           `mapstructure:"port"`
	DataDir             string        `mapstructure:"data_dir"`
	BaseURL             string        `mapstructure:"base_url"`
	BehindProxy         string        `mapstructure:"behind_proxy"`
	TrustedProxies      []string      `mapstructure:"trusted_proxies"`
	RealIPHeader        string        `mapstructure:"real_ip_header"`
	ReadTimeout         time.Duration `mapstructure:"read_timeout"`
	WriteTimeout        time.Duration `mapstructure:"write_timeout"`
	IdleTimeout         time.Duration `mapstructure:"idle_timeout"`
	MaxHeaderBytes      int           `mapstructure:"max_header_bytes"`
}

// DatabaseConfig contains database configuration
type DatabaseConfig struct {
	URL                    string        `mapstructure:"url"`
	Type                   string        `mapstructure:"type"`
	Host                   string        `mapstructure:"host"`
	Port                   int           `mapstructure:"port"`
	Name                   string        `mapstructure:"name"`
	Username               string        `mapstructure:"username"`
	Password               string        `mapstructure:"password"`
	SSLMode                string        `mapstructure:"ssl_mode"`
	MaxOpenConns           int           `mapstructure:"max_open_conns"`
	MaxIdleConns           int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime        time.Duration `mapstructure:"conn_max_lifetime"`
	AutoMigrate            bool          `mapstructure:"auto_migrate"`
	MigrationTimeout       time.Duration `mapstructure:"migration_timeout"`
	SlowQueryThreshold     time.Duration `mapstructure:"slow_query_threshold"`
	LogQueries             bool          `mapstructure:"log_queries"`
	SQLiteWAL              bool          `mapstructure:"sqlite_wal"`
	SQLiteCacheSize        string        `mapstructure:"sqlite_cache_size"`
	SQLiteBusyTimeout      time.Duration `mapstructure:"sqlite_busy_timeout"`
}

// ApplicationConfig contains general application settings
type ApplicationConfig struct {
	BrandName              string `mapstructure:"brand_name"`
	BaseURL                string `mapstructure:"base_url"`
	EnableRegistration     bool   `mapstructure:"enable_registration"`
	EnableAnonymousURLs    bool   `mapstructure:"enable_anonymous_urls"`
	RequireLoginForURLs    bool   `mapstructure:"require_login_for_urls"`
	DefaultTheme           string `mapstructure:"default_theme"`
	CustomCSSURL           string `mapstructure:"custom_css_url"`
	FaviconURL             string `mapstructure:"favicon_url"`
}

// URLConfig contains URL shortening configuration
type URLConfig struct {
	MinRandomLength      int      `mapstructure:"min_random_length"`
	MaxRandomLength      int      `mapstructure:"max_random_length"`
	CustomCodeMinLength  int      `mapstructure:"custom_code_min_length"`
	CustomCodeMaxLength  int      `mapstructure:"custom_code_max_length"`
	AllowedCharacters    string   `mapstructure:"allowed_characters"`
	ExcludeSimilarChars  bool     `mapstructure:"exclude_similar_chars"`
	ReservedWords        []string `mapstructure:"reserved_words"`
	MaxURLLength         int      `mapstructure:"max_url_length"`
	DefaultExpiration    string   `mapstructure:"default_expiration"`
}

// AnalyticsConfig contains analytics configuration
type AnalyticsConfig struct {
	Enabled                bool     `mapstructure:"enabled"`
	EnableGeolocation      bool     `mapstructure:"enable_geolocation"`
	GeoIPDatabasePath      string   `mapstructure:"geoip_database_path"`
	RetentionDays          int      `mapstructure:"retention_days"`
	AnonymizeIPs           bool     `mapstructure:"anonymize_ips"`
	TrackBots              bool     `mapstructure:"track_bots"`
	RealTimeAnalytics      bool     `mapstructure:"real_time_analytics"`
	ExportFormats          []string `mapstructure:"export_formats"`
}

// QRConfig contains QR code configuration
type QRConfig struct {
	Enabled              bool     `mapstructure:"enabled"`
	DefaultSize          int      `mapstructure:"default_size"`
	MaxSize              int      `mapstructure:"max_size"`
	SupportedFormats     []string `mapstructure:"supported_formats"`
	DefaultStyle         string   `mapstructure:"default_style"`
	AllowCustomColors    bool     `mapstructure:"allow_custom_colors"`
	AllowLogos           bool     `mapstructure:"allow_logos"`
	MaxLogoSize          int      `mapstructure:"max_logo_size"`
}

// BulkConfig contains bulk operations configuration
type BulkConfig struct {
	Enabled            bool          `mapstructure:"enabled"`
	MaxImportSize      int           `mapstructure:"max_import_size"`
	SupportedFormats   []string      `mapstructure:"supported_formats"`
	Timeout            time.Duration `mapstructure:"timeout"`
}

// AuthConfig contains authentication configuration
type AuthConfig struct {
	Session               SessionConfig      `mapstructure:"session"`
	Password              PasswordConfig     `mapstructure:"password"`
	APIToken              APITokenConfig     `mapstructure:"api_token"`
	SessionSecret         string        `mapstructure:"session_secret"`
	SessionTimeout        time.Duration `mapstructure:"session_timeout"`
	SessionSecure         string        `mapstructure:"session_secure"`
	PasswordMinLength     int           `mapstructure:"password_min_length"`
	PasswordRequireSpecial bool         `mapstructure:"password_require_special"`
	APITokenLength        int           `mapstructure:"api_token_length"`
	APITokenExpiration    string        `mapstructure:"api_token_expiration"`
	EnableTOTP            bool          `mapstructure:"enable_totp"`
	TOTPIssuerName        string        `mapstructure:"totp_issuer_name"`
	EnableWebAuthn        bool          `mapstructure:"enable_webauthn"`
	WebAuthnDisplayName   string        `mapstructure:"webauthn_display_name"`
	WebAuthnID            string        `mapstructure:"webauthn_id"`
}

// SessionConfig contains session management configuration
type SessionConfig struct {
	Secret             string        `mapstructure:"secret"`
	Timeout            time.Duration `mapstructure:"timeout"`
	RememberMeDuration time.Duration `mapstructure:"remember_me_duration"`
	Secure             bool          `mapstructure:"secure"`
	HTTPOnly           bool          `mapstructure:"http_only"`
	SameSite           string        `mapstructure:"same_site"`
	CookieName         string        `mapstructure:"cookie_name"`
}

// PasswordConfig contains password policy configuration
type PasswordConfig struct {
	MinLength       int  `mapstructure:"min_length"`
	RequireSpecial  bool `mapstructure:"require_special"`
	RequireDigit    bool `mapstructure:"require_digit"`
	RequireUppercase bool `mapstructure:"require_uppercase"`
	RequireLowercase bool `mapstructure:"require_lowercase"`
	Cost            int  `mapstructure:"cost"`
}

// APITokenConfig contains API token configuration
type APITokenConfig struct {
	Length            int           `mapstructure:"length"`
	DefaultExpiration time.Duration `mapstructure:"default_expiration"`
	DefaultRateLimit  int           `mapstructure:"default_rate_limit"`
}

// OAuthConfig contains OAuth configuration
type OAuthConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	Provider      string `mapstructure:"provider"`
	ClientID      string `mapstructure:"client_id"`
	ClientSecret  string `mapstructure:"client_secret"`
	RedirectURL   string `mapstructure:"redirect_url"`
	AuthorizeURL  string `mapstructure:"authorize_url"`
	TokenURL      string `mapstructure:"token_url"`
	UserInfoURL   string `mapstructure:"userinfo_url"`
	Scopes        []string `mapstructure:"scopes"`
}

// DomainsConfig contains custom domains configuration
type DomainsConfig struct {
	Enabled                bool   `mapstructure:"enabled"`
	VerificationMethod     string `mapstructure:"verification_method"`
	SSLAutoProvision       bool   `mapstructure:"ssl_auto_provision"`
	MaxDomainsPerUser      string `mapstructure:"max_domains_per_user"`
}

// FederationConfig contains federation configuration
type FederationConfig struct {
	Enabled            bool          `mapstructure:"enabled"`
	Domain             string        `mapstructure:"domain"`
	PrivateKeyPath     string        `mapstructure:"private_key_path"`
	PublicKeyPath      string        `mapstructure:"public_key_path"`
	SyncInterval       time.Duration `mapstructure:"sync_interval"`
	SharePublicURLs    bool          `mapstructure:"share_public_urls"`
	MaxURLsPerSync     int           `mapstructure:"max_urls_per_sync"`
	SyncTimeout        time.Duration `mapstructure:"sync_timeout"`
	DiscoveryTimeout   time.Duration `mapstructure:"discovery_timeout"`
	RetryAttempts      int           `mapstructure:"retry_attempts"`
	RetryBackoff       time.Duration `mapstructure:"retry_backoff"`
}

// WebhooksConfig contains webhook configuration
type WebhooksConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Timeout          time.Duration `mapstructure:"timeout"`
	RetryAttempts    int           `mapstructure:"retry_attempts"`
	RetryBackoff     string        `mapstructure:"retry_backoff"`
	MaxPayloadSize   int           `mapstructure:"max_payload_size"`
}

// BillingConfig contains billing configuration
type BillingConfig struct {
	Enabled                    bool   `mapstructure:"enabled"`
	Provider                   string `mapstructure:"provider"`
	StripeSecretKey            string `mapstructure:"stripe_secret_key"`
	StripeWebhookSecret        string `mapstructure:"stripe_webhook_secret"`
	PayPalClientID             string `mapstructure:"paypal_client_id"`
	PayPalClientSecret         string `mapstructure:"paypal_client_secret"`
	Currency                   string `mapstructure:"currency"`
	GracePeriodDays            int    `mapstructure:"grace_period_days"`
	TrialPeriodDays            int    `mapstructure:"trial_period_days"`
	RequirePayment             bool   `mapstructure:"require_payment"`
	EnforceLimits              bool   `mapstructure:"enforce_limits"`
	UsageWarningThreshold      int    `mapstructure:"usage_warning_threshold"`
	AutoSuspend                bool   `mapstructure:"auto_suspend"`
}

// RateLimitConfig contains rate limiting configuration
type RateLimitConfig struct {
	Enabled             bool          `mapstructure:"enabled"`
	RequestsPerMinute   int           `mapstructure:"requests_per_minute"`
	Burst               int           `mapstructure:"burst"`
	CleanupInterval     time.Duration `mapstructure:"cleanup_interval"`
}

// SecurityConfig contains security configuration
type SecurityConfig struct {
	EnableHTTPSRedirect     string   `mapstructure:"enable_https_redirect"`
	HSTSMaxAge              int      `mapstructure:"hsts_max_age"`
	CSPPolicy               string   `mapstructure:"csp_policy"`
	AllowedOrigins          []string `mapstructure:"allowed_origins"`
	MalwareDetectionEnabled bool     `mapstructure:"malware_detection_enabled"`
	MalwareAPIKey           string   `mapstructure:"malware_api_key"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level       string `mapstructure:"level"`
	Format      string `mapstructure:"format"`
	File        string `mapstructure:"file"`
	MaxSize     string `mapstructure:"max_size"`
	MaxBackups  int    `mapstructure:"max_backups"`
	MaxAge      int    `mapstructure:"max_age"`
	Compress    bool   `mapstructure:"compress"`
}

// MonitoringConfig contains monitoring configuration
type MonitoringConfig struct {
	EnableMetrics      bool   `mapstructure:"enable_metrics"`
	MetricsPath        string `mapstructure:"metrics_path"`
	EnableHealthCheck  bool   `mapstructure:"enable_health_check"`
	HealthCheckPath    string `mapstructure:"health_check_path"`
}

// EmailConfig contains email configuration
type EmailConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	Provider        string `mapstructure:"provider"`
	FromAddress     string `mapstructure:"from_address"`
	FromName        string `mapstructure:"from_name"`
	SMTPHost        string `mapstructure:"smtp_host"`
	SMTPPort        int    `mapstructure:"smtp_port"`
	SMTPUsername    string `mapstructure:"smtp_username"`
	SMTPPassword    string `mapstructure:"smtp_password"`
	SMTPTLS         bool   `mapstructure:"smtp_tls"`
	SendGridAPIKey  string `mapstructure:"sendgrid_api_key"`
	SESAccessKey    string `mapstructure:"ses_access_key"`
	SESSecretKey    string `mapstructure:"ses_secret_key"`
	SESRegion       string `mapstructure:"ses_region"`
}

// NotificationConfig contains notification system configuration
type NotificationConfig struct {
	Enabled                 bool          `mapstructure:"enabled"`
	DefaultChannel          string        `mapstructure:"default_channel"`
	BatchSize               int           `mapstructure:"batch_size"`
	ProcessingInterval      time.Duration `mapstructure:"processing_interval"`
	RetryAttempts           int           `mapstructure:"retry_attempts"`
	RetryDelay              time.Duration `mapstructure:"retry_delay"`
	MaxRetryDelay           time.Duration `mapstructure:"max_retry_delay"`
	RetentionDays           int           `mapstructure:"retention_days"`
	EnableSMS               bool          `mapstructure:"enable_sms"`
	EnablePush              bool          `mapstructure:"enable_push"`
	EnableWebhook           bool          `mapstructure:"enable_webhook"`
	SMSProvider             string        `mapstructure:"sms_provider"`
	PushProvider            string        `mapstructure:"push_provider"`
	TwilioAccountSID        string        `mapstructure:"twilio_account_sid"`
	TwilioAuthToken         string        `mapstructure:"twilio_auth_token"`
	TwilioFromNumber        string        `mapstructure:"twilio_from_number"`
	FCMServerKey            string        `mapstructure:"fcm_server_key"`
	APNSCertPath            string        `mapstructure:"apns_cert_path"`
	APNSKeyPath             string        `mapstructure:"apns_key_path"`
	APNSBundleID            string        `mapstructure:"apns_bundle_id"`
	APNSProduction          bool          `mapstructure:"apns_production"`
}

// CacheConfig contains caching configuration
type CacheConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	TTL              time.Duration `mapstructure:"ttl"`
	CleanupInterval  time.Duration `mapstructure:"cleanup_interval"`
	RedisEnabled     bool          `mapstructure:"redis_enabled"`
	RedisURL         string        `mapstructure:"redis_url"`
}

// Load loads configuration from environment variables and config files
func Load() (*Config, error) {
	// Set up viper
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Environment variables
	v.SetEnvPrefix("CASLINK")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Config file
	v.SetConfigName("config")
	v.SetConfigType("toml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/caslink/")
	v.AddConfigPath("/var/lib/caslink/")

	// Read config file if it exists
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal config
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validate(&config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Post-process configuration
	if err := postProcess(&config); err != nil {
		return nil, fmt.Errorf("configuration post-processing failed: %w", err)
	}

	return &config, nil
}

// postProcess performs post-processing on the configuration
func postProcess(config *Config) error {
	// Ensure data directory exists
	dataDir := "/var/lib/caslink"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		// Try using local directory if system directory fails
		dataDir = "./data"
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return fmt.Errorf("failed to create data directory: %w", err)
		}
	}

	// Update database path if using SQLite
	if config.Database.Type == "sqlite" && strings.Contains(config.Database.URL, "/var/lib/caslink/") {
		config.Database.URL = strings.Replace(config.Database.URL, "/var/lib/caslink/", dataDir+"/", 1)
	}

	// Update federation key paths
	if config.Federation.PrivateKeyPath == "/var/lib/caslink/federation.key" {
		config.Federation.PrivateKeyPath = filepath.Join(dataDir, "federation.key")
	}
	if config.Federation.PublicKeyPath == "/var/lib/caslink/federation.pub" {
		config.Federation.PublicKeyPath = filepath.Join(dataDir, "federation.pub")
	}

	// Update GeoIP database path
	if config.Analytics.GeoIPDatabasePath == "/var/lib/caslink/GeoLite2-City.mmdb" {
		config.Analytics.GeoIPDatabasePath = filepath.Join(dataDir, "GeoLite2-City.mmdb")
	}

	return nil
}