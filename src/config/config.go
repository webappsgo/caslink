package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Web     WebConfig     `yaml:"web"`
	Caslink CaslinkConfig `yaml:"caslink"`
}

// ServerConfig holds server-specific settings
type ServerConfig struct {
	Port      int    `yaml:"port"`
	Address   string `yaml:"address"`
	Mode      string `yaml:"mode"`
	FQDN      string `yaml:"fqdn"`
	Daemonize bool   `yaml:"daemonize"`
	PIDFile   bool   `yaml:"pidfile"`

	Branding       BrandingConfig       `yaml:"branding"`
	SEO            SEOConfig            `yaml:"seo"`
	Admin          AdminConfig          `yaml:"admin"`
	Contact        ContactConfig        `yaml:"contact"`
	SSL            SSLConfig            `yaml:"ssl"`
	Database       DatabaseConfig       `yaml:"database"`
	RateLimit      RateLimitConfig      `yaml:"rate_limit"`
	Limits         LimitsConfig         `yaml:"limits"`
	Compression    CompressionConfig    `yaml:"compression"`
	TrustedProxies TrustedProxiesConfig `yaml:"trusted_proxies"`
	Session        SessionConfig        `yaml:"session"`
	I18n           I18nConfig           `yaml:"i18n"`
	Tracking       TrackingConfig       `yaml:"tracking"`
	Scheduler      SchedulerConfig      `yaml:"scheduler"`
	Security       SecurityConfig       `yaml:"security"`
	Features       FeaturesConfig       `yaml:"features"`
	Notifications  NotificationsConfig  `yaml:"notifications"`
	Metrics        MetricsConfig        `yaml:"metrics"`
}

// SecurityConfig holds security policy configuration per AI.md PART 17.
type SecurityConfig struct {
	Password PasswordPolicyConfig `yaml:"password"`
}

// PasswordPolicyConfig holds password complexity requirements per AI.md PART 17.
// All complexity checks are off by default (spec line 16894); they auto-enable
// when compliance mode (HIPAA/SOC2/PCI-DSS) is active.
type PasswordPolicyConfig struct {
	MinLength        int  `yaml:"min_length"`        // minimum password length (default 8)
	RequireUppercase bool `yaml:"require_uppercase"` // at least one A-Z
	RequireLowercase bool `yaml:"require_lowercase"` // at least one a-z
	RequireNumber    bool `yaml:"require_number"`    // at least one 0-9
	RequireSpecial   bool `yaml:"require_special"`   // at least one !@#$%^&*…
}

// LimitsConfig holds HTTP request limits per AI.md PART 12.
type LimitsConfig struct {
	MaxBodySize   string `yaml:"max_body_size"`   // e.g., "10MB"
	ReadTimeout   int    `yaml:"read_timeout"`    // seconds
	WriteTimeout  int    `yaml:"write_timeout"`   // seconds
	IdleTimeout   int    `yaml:"idle_timeout"`    // seconds
}

// CompressionConfig holds response compression settings per AI.md PART 12.
type CompressionConfig struct {
	Enabled bool     `yaml:"enabled"`
	Level   int      `yaml:"level"` // 1–9
	Types   []string `yaml:"types"` // MIME types to compress
}

// TrustedProxiesConfig holds X-Forwarded-* trust gate config per AI.md PART 12.
type TrustedProxiesConfig struct {
	Additional []string `yaml:"additional"` // extra public IPs/CIDRs/hostnames
}

// SessionConfig holds admin and user session cookie settings per AI.md PART 12.
type SessionConfig struct {
	Admin            SessionCookieConfig `yaml:"admin"`
	User             SessionCookieConfig `yaml:"user"`
	ExtendOnActivity bool                `yaml:"extend_on_activity"`
	Secure           string              `yaml:"secure"`    // auto|true|false
	HTTPOnly         bool                `yaml:"http_only"`
	SameSite         string              `yaml:"same_site"` // strict|lax|none
}

// SessionCookieConfig holds per-role session cookie settings.
type SessionCookieConfig struct {
	CookieName  string `yaml:"cookie_name"`
	MaxAge      int    `yaml:"max_age"`      // seconds
	IdleTimeout int    `yaml:"idle_timeout"` // seconds
}

// I18nConfig holds internationalisation settings per AI.md PART 12.
type I18nConfig struct {
	DefaultLanguage string   `yaml:"default_language"`
	Supported       []string `yaml:"supported"`
}

// TrackingConfig holds analytics platform configuration per AI.md PART 12.
// Only active when the user explicitly sets Type — telemetry is opt-in.
type TrackingConfig struct {
	Type string `yaml:"type"` // google|matomo|plausible|umami|fathom|simple|cloudflare
	ID   string `yaml:"id"`   // tracking / measurement ID
	URL  string `yaml:"url"`  // self-hosted instance URL (required for some types)
}

// ContactConfig holds unified notification recipient config per AI.md PART 12.
type ContactConfig struct {
	Admin    ContactRecipient `yaml:"admin"`
	Security ContactRecipient `yaml:"security"`
	General  ContactRecipient `yaml:"general"`
}

// ContactRecipient holds a single notification role's target address and optional
// webhook transports. Webhook fields are keyed by provider name.
type ContactRecipient struct {
	Email    string            `yaml:"email"`
	Webhooks map[string]string `yaml:"webhooks,omitempty"` // provider → URL
}

// MetricsConfig holds Prometheus metrics configuration per AI.md PART 21.
type MetricsConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Endpoint       string `yaml:"endpoint"`
	IncludeSystem  bool   `yaml:"include_system"`
	IncludeRuntime bool   `yaml:"include_runtime"`
	Token          string `yaml:"token"`
}

// NotificationsConfig holds notification channel configuration per AI.md
// PART 18. The Email sub-struct mirrors the spec's
// cfg.Server.Notifications.Email.SMTP.{Host,Port,Username,...} access path.
type NotificationsConfig struct {
	Email EmailConfig `yaml:"email"`
}

// EmailConfig holds SMTP / sender configuration. When SMTP.Host is empty
// the EmailService treats email as unconfigured and silently skips sends
// per PART 26 "No SMTP = No emails".
type EmailConfig struct {
	Enabled  bool       `yaml:"enabled"`
	From     string     `yaml:"from"`
	FromName string     `yaml:"from_name"`
	ReplyTo  string     `yaml:"reply_to"`
	SMTP     SMTPConfig `yaml:"smtp"`
}

// SMTPConfig holds SMTP server connection details. Host is the only
// required field; everything else has a sane default (port 587, no auth,
// auto-TLS).
type SMTPConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	UseTLS      bool   `yaml:"use_tls"`
	UseStartTLS bool   `yaml:"use_starttls"`
}

// BrandingConfig holds branding settings
type BrandingConfig struct {
	Title       string `yaml:"title"`
	Tagline     string `yaml:"tagline"`
	Description string `yaml:"description"`
}

// SEOConfig holds SEO settings
type SEOConfig struct {
	Keywords []string `yaml:"keywords"`
}

// AdminConfig holds admin panel settings
type AdminConfig struct {
	Email string `yaml:"email"`
	Path  string `yaml:"path"` // URL segment for admin panel (default: "admin")
}

// SSLConfig holds SSL/TLS settings
type SSLConfig struct {
	Enabled    bool              `yaml:"enabled"`
	Cert       string            `yaml:"cert"`
	Key        string            `yaml:"key"`
	MinVersion string            `yaml:"min_version"`
	LetsEncrypt LetsEncryptConfig `yaml:"letsencrypt"`
}

// LetsEncryptConfig holds Let's Encrypt settings
type LetsEncryptConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Email     string `yaml:"email"`
	Challenge string `yaml:"challenge"`
	Staging   bool   `yaml:"staging"`
}

// DatabaseConfig holds database settings
type DatabaseConfig struct {
	Driver   string `yaml:"driver"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"sslmode"`
	Path     string `yaml:"path"`
}

// RateLimitConfig holds rate limiting settings
type RateLimitConfig struct {
	Enabled  bool `yaml:"enabled"`
	Requests int  `yaml:"requests"`
	Window   int  `yaml:"window"`
}

// SchedulerConfig holds scheduler settings
type SchedulerConfig struct {
	Enabled bool `yaml:"enabled"`
}

// WebConfig holds web frontend settings
type WebConfig struct {
	UI   UIConfig `yaml:"ui"`
	CORS string   `yaml:"cors"`
}

// UIConfig holds UI settings
type UIConfig struct {
	Theme string `yaml:"theme"`
}

// FeaturesConfig holds feature flags and settings
type FeaturesConfig struct {
	Users         UsersConfig          `yaml:"users"`
	Organizations OrganizationsConfig  `yaml:"organizations"`
	CustomDomains CustomDomainsConfig  `yaml:"custom_domains"`
	Billing       BillingConfig        `yaml:"billing"`
	Federation    FederationConfig     `yaml:"federation"`
}

// UsersConfig holds user management settings
type UsersConfig struct {
	Enabled      bool                  `yaml:"enabled"`
	Registration RegistrationConfig    `yaml:"registration"`
	Profile      ProfileConfig         `yaml:"profile"`
}

// RegistrationConfig holds registration settings
type RegistrationConfig struct {
	Enabled                 bool `yaml:"enabled"`
	RequireEmailVerification bool `yaml:"require_email_verification"`
	RequireApproval         bool `yaml:"require_approval"`
	AllowDisposableEmails   bool `yaml:"allow_disposable_emails"`
}

// ProfileConfig holds user profile settings
type ProfileConfig struct {
	AllowDisplayName bool `yaml:"allow_display_name"`
	AllowAvatar      bool `yaml:"allow_avatar"`
	AllowBio         bool `yaml:"allow_bio"`
}

// OrganizationsConfig holds organization settings
type OrganizationsConfig struct {
	Enabled       bool     `yaml:"enabled"`
	AllowCreation bool     `yaml:"allow_creation"`
	MaxPerUser    int      `yaml:"max_per_user"`
	Roles         []string `yaml:"roles"`
}

// CustomDomainsConfig holds custom domain settings
type CustomDomainsConfig struct {
	Enabled           bool     `yaml:"enabled"`
	MaxDomainsPerUser int      `yaml:"max_domains_per_user"`
	MaxDomainsPerOrg  int      `yaml:"max_domains_per_org"`
	RequireSSL        bool     `yaml:"require_ssl"`
	AllowApex         bool     `yaml:"allow_apex"`
	AllowSubdomain    bool     `yaml:"allow_subdomain"`
	AllowWildcard     bool     `yaml:"allow_wildcard"`
	VerificationTTL   int      `yaml:"verification_ttl"`
	SSLRenewalDays    int      `yaml:"ssl_renewal_days"`
	Reserved          []string `yaml:"reserved"`
	BlockedPatterns   []string `yaml:"blocked_patterns"`
}

// BillingConfig holds billing settings
type BillingConfig struct {
	Enabled   bool     `yaml:"enabled"`
	StripeKey string   `yaml:"stripe_key"`
	Plans     []string `yaml:"plans"`
}

// FederationConfig holds federation settings
type FederationConfig struct {
	Enabled   bool     `yaml:"enabled"`
	Instances []string `yaml:"instances"`
}

// CaslinkConfig holds caslink-specific settings
type CaslinkConfig struct {
	URL       URLConfig       `yaml:"url"`
	Analytics AnalyticsConfig `yaml:"analytics"`
	QR        QRConfig        `yaml:"qr"`
}

// URLConfig holds URL shortening settings
type URLConfig struct {
	MinRandomLength  int      `yaml:"min_random_length"`
	MaxCustomLength  int      `yaml:"max_custom_length"`
	DefaultExpiration string   `yaml:"default_expiration"`
	AllowCustomCodes bool     `yaml:"allow_custom_codes"`
	PerUserLimit     int      `yaml:"per_user_limit"`
	PerOrgLimit      int      `yaml:"per_org_limit"`
	ReservedWords    []string `yaml:"reserved_words"`
}

// AnalyticsConfig holds analytics settings
type AnalyticsConfig struct {
	Enabled             bool `yaml:"enabled"`
	EnableGeolocation   bool `yaml:"enable_geolocation"`
	AnonymizeIPs        bool `yaml:"anonymize_ips"`
	RetentionDays       int  `yaml:"retention_days"`
}

// QRConfig holds QR code settings
type QRConfig struct {
	DefaultSize      int    `yaml:"default_size"`
	MaxSize          int    `yaml:"max_size"`
	DefaultFormat    string `yaml:"default_format"`
	ErrorCorrection  string `yaml:"error_correction"`
}

// Load loads configuration from server.yml
func Load(configDir string) (*Config, error) {
	configPath := filepath.Join(configDir, "server.yml")

	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		cfg := DefaultConfig()
		if err := Save(configDir, cfg); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		applyEnvOverrides(cfg)
		return cfg, nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate config
	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Apply environment variable overrides (PART 26 precedence: env > config > default).
	applyEnvOverrides(&cfg)

	return &cfg, nil
}

// applyEnvOverrides overlays environment variables on top of the loaded config.
// Checks CASLINK_* prefix first (per AI.md PART 9 / {PROJECT_NAME}_* spec),
// then falls back to bare names for backward compatibility.
func applyEnvOverrides(cfg *Config) {
	if v := envStr("MODE"); v != "" {
		cfg.Server.Mode = v
	}
	if v := envStr("DOMAIN"); v != "" {
		cfg.Server.FQDN = v
	}
	if v := envStr("PORT"); v != "" {
		if n := parseInt(v); n > 0 {
			cfg.Server.Port = n
		}
	}
	if v := envStr("LISTEN"); v != "" {
		cfg.Server.Address = v
	}
	if v := envStr("DATABASE_DRIVER"); v != "" {
		cfg.Server.Database.Driver = v
	}
	if v := envStr("DATABASE_URL"); v != "" {
		cfg.Server.Database.Host = v // full DSN — factory.go resolves per driver
	}
}

// envStr returns the trimmed value of an environment variable, or "".
// It checks CASLINK_{key} first, then falls back to {key}.
func envStr(key string) string {
	if v := os.Getenv("CASLINK_" + key); v != "" {
		return strings.TrimSpace(v)
	}
	v := os.Getenv(key)
	if v == "" {
		return ""
	}
	return strings.TrimSpace(v)
}

// parseInt parses an integer from a string, returning 0 on error.
func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// Save saves configuration to server.yml
func Save(configDir string, cfg *Config) error {
	configPath := filepath.Join(configDir, "server.yml")

	// Ensure directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	return &Config{
		Server: ServerConfig{
			Port:      0, // 0 = auto-select from 64xxx range
			Address:   "[::]",
			Mode:      "production",
			FQDN:      hostname,
			Daemonize: false,
			PIDFile:   true,
			Branding: BrandingConfig{
				Title:       "caslink",
				Tagline:     "",
				Description: "",
			},
			SEO: SEOConfig{
				Keywords: []string{},
			},
			Admin: AdminConfig{
				Email: fmt.Sprintf("admin@%s", hostname),
				Path:  "admin",
			},
			Contact: ContactConfig{
				Admin: ContactRecipient{
					Email: fmt.Sprintf("admin@%s", hostname),
				},
				Security: ContactRecipient{
					Email: fmt.Sprintf("security@%s", hostname),
				},
				General: ContactRecipient{
					Email: fmt.Sprintf("admin@%s", hostname),
				},
			},
			Limits: LimitsConfig{
				MaxBodySize:  "10MB",
				ReadTimeout:  30,
				WriteTimeout: 30,
				IdleTimeout:  120,
			},
			Compression: CompressionConfig{
				Enabled: true,
				Level:   6,
				Types: []string{
					"text/html",
					"text/css",
					"text/javascript",
					"application/json",
					"application/javascript",
					"image/svg+xml",
				},
			},
			TrustedProxies: TrustedProxiesConfig{
				Additional: []string{},
			},
			Session: SessionConfig{
				Admin: SessionCookieConfig{
					CookieName:  "caslink_admin_session",
					MaxAge:      86400,    // 24 hours
					IdleTimeout: 3600,     // 1 hour
				},
				User: SessionCookieConfig{
					CookieName:  "caslink_session",
					MaxAge:      2592000,  // 30 days
					IdleTimeout: 86400,    // 24 hours
				},
				ExtendOnActivity: true,
				Secure:           "auto",
				HTTPOnly:         true,
				SameSite:         "lax",
			},
			I18n: I18nConfig{
				DefaultLanguage: "en",
				Supported:       []string{"en"},
			},
			Tracking: TrackingConfig{
				Type: "",
				ID:   "",
				URL:  "",
			},
			SSL: SSLConfig{
				Enabled:    false,
				Cert:       "",
				Key:        "",
				MinVersion: "TLS1.2",
				LetsEncrypt: LetsEncryptConfig{
					Enabled:   false,
					Email:     fmt.Sprintf("admin@%s", hostname),
					Challenge: "http-01",
					Staging:   false,
				},
			},
			Database: DatabaseConfig{
				Driver: "file",
				Path:   "{datadir}/db",
			},
			RateLimit: RateLimitConfig{
				Enabled:  true,
				Requests: 120,
				Window:   60,
			},
			Scheduler: SchedulerConfig{
				Enabled: true,
			},
			Features: FeaturesConfig{
				Users: UsersConfig{
					Enabled: true,
					Registration: RegistrationConfig{
						Enabled:                 true,
						RequireEmailVerification: false,
						RequireApproval:         false,
						AllowDisposableEmails:   false,
					},
					Profile: ProfileConfig{
						AllowDisplayName: true,
						AllowAvatar:      true,
						AllowBio:         true,
					},
				},
				Organizations: OrganizationsConfig{
					Enabled:       true,
					AllowCreation: true,
					MaxPerUser:    5,
					Roles:         []string{"owner", "admin", "member"},
				},
				CustomDomains: CustomDomainsConfig{
					Enabled:           true,
					MaxDomainsPerUser: 5,
					MaxDomainsPerOrg:  20,
					RequireSSL:        true,
					AllowApex:         true,
					AllowSubdomain:    true,
					AllowWildcard:     false,
					VerificationTTL:   86400,
					SSLRenewalDays:    7,
					Reserved: []string{
						"localhost",
						"*.local",
						"*.test",
						"*.example",
						"*.invalid",
					},
					BlockedPatterns: []string{
						".*\\.(gov|mil|edu)$",
					},
				},
				Billing: BillingConfig{
					Enabled:   false,
					StripeKey: "",
					Plans:     []string{},
				},
				Federation: FederationConfig{
					Enabled:   false,
					Instances: []string{},
				},
			},
			Security: SecurityConfig{
				Password: PasswordPolicyConfig{
					MinLength:        8,
					RequireUppercase: false,
					RequireLowercase: false,
					RequireNumber:    false,
					RequireSpecial:   false,
				},
			},
			Metrics: MetricsConfig{
				Enabled:        true,
				Endpoint:       "/metrics",
				IncludeSystem:  true,
				IncludeRuntime: true,
				Token:          "",
			},
			Notifications: NotificationsConfig{
				Email: EmailConfig{
					Enabled:  false,
					From:     fmt.Sprintf("no-reply@%s", hostname),
					FromName: "Caslink",
					ReplyTo:  "",
					SMTP: SMTPConfig{
						Host:        "",
						Port:        587,
						Username:    "",
						Password:    "",
						UseTLS:      false,
						UseStartTLS: true,
					},
				},
			},
		},
		Web: WebConfig{
			UI: UIConfig{
				Theme: "dark",
			},
			// Default to same-origin; using "*" alongside AllowCredentials:true
			// is rejected by browsers and is also a security misconfiguration.
			// Operators set this explicitly when exposing the API cross-origin.
			CORS: "",
		},
		Caslink: CaslinkConfig{
			URL: URLConfig{
				MinRandomLength:  6,
				MaxCustomLength:  50,
				DefaultExpiration: "never",
				AllowCustomCodes: true,
				PerUserLimit:     0,
				PerOrgLimit:      0,
				ReservedWords: []string{
					"admin", "api", "auth", "user", "org",
					"setup", "healthz", "swagger", "graphql", "graphiql",
				},
			},
			Analytics: AnalyticsConfig{
				Enabled:           true,
				EnableGeolocation: true,
				AnonymizeIPs:      true,
				RetentionDays:     365,
			},
			QR: QRConfig{
				DefaultSize:     256,
				MaxSize:         2048,
				DefaultFormat:   "png",
				ErrorCorrection: "medium",
			},
		},
	}
}

// Validate validates the configuration, warning on invalid values and replacing
// them with safe defaults per AI.md PART 12: "If config setting is invalid,
// warn and replace with default. Never fail startup."
func Validate(cfg *Config) error {
	// Validate mode — unknown values fall back to production.
	if cfg.Server.Mode != "production" && cfg.Server.Mode != "development" {
		fmt.Printf("config: invalid mode %q — defaulting to production\n", cfg.Server.Mode)
		cfg.Server.Mode = "production"
	}

	// Validate database driver — unknown values fall back to sqlite.
	validDrivers := map[string]bool{
		"file": true, "sqlite": true, "postgres": true,
		"mysql": true, "mariadb": true, "mssql": true,
	}
	if !validDrivers[cfg.Server.Database.Driver] {
		fmt.Printf("config: invalid database driver %q — defaulting to sqlite\n", cfg.Server.Database.Driver)
		cfg.Server.Database.Driver = "sqlite"
	}

	return nil
}
