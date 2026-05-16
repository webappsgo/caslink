package config

import (
	"fmt"
	"os"
	"path/filepath"

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

	Branding      BrandingConfig      `yaml:"branding"`
	SEO           SEOConfig           `yaml:"seo"`
	Admin         AdminConfig         `yaml:"admin"`
	SSL           SSLConfig           `yaml:"ssl"`
	Database      DatabaseConfig      `yaml:"database"`
	RateLimit     RateLimitConfig     `yaml:"rate_limit"`
	Scheduler     SchedulerConfig     `yaml:"scheduler"`
	Features      FeaturesConfig      `yaml:"features"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Metrics       MetricsConfig       `yaml:"metrics"`
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

	return &cfg, nil
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
