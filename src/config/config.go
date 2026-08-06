package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/webappsgo/caslink/src/common/crypto"
)

// Config represents the complete application configuration
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Web     WebConfig     `yaml:"web"`
	Caslink CaslinkConfig `yaml:"caslink"`
}

// ServerConfig holds server-specific settings
type ServerConfig struct {
	Port       int    `yaml:"port"`
	Address    string `yaml:"address"`
	Mode       string `yaml:"mode"`
	FQDN       string `yaml:"fqdn"`
	APIVersion string `yaml:"api_version"`
	Daemonize  bool   `yaml:"daemonize"`
	PIDFile    bool   `yaml:"pidfile"`

	Healthz        HealthzConfig        `yaml:"healthz"`
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
	GeoIP          GeoIPConfig          `yaml:"geoip"`
	Tor            TorConfig            `yaml:"tor"`
	Backup         BackupConfig         `yaml:"backup"`
	Compliance     ComplianceConfig     `yaml:"compliance"`
	Privacy        PrivacyConfig        `yaml:"privacy"`
}

// ResolvedAPIVersion returns the configured API version URL segment (PART 14),
// falling back to "v1" when unset or invalid. A valid segment is lowercase
// alphanumeric (e.g. "v1", "v2beta"). Per the config rule, an invalid value is
// substituted with the default rather than causing a crash.
func (s ServerConfig) ResolvedAPIVersion() string {
	v := strings.ToLower(strings.TrimSpace(s.APIVersion))
	if v == "" || !isValidAPIVersion(v) {
		return "v1"
	}
	return v
}

// APIBasePath returns the mount prefix for all versioned API routes, e.g.
// "/api/v1" (PART 13/14). Never hardcode "/api/v1" — always use this.
func (s ServerConfig) APIBasePath() string {
	return "/api/" + s.ResolvedAPIVersion()
}

// isValidAPIVersion reports whether v is a valid API version URL segment:
// non-empty and composed only of lowercase letters and digits.
func isValidAPIVersion(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// HealthzConfig controls the health endpoints per AI.md PART 13. The
// versioned /api/v1/server/healthz and the /server/healthz page are always
// registered; the bare-root /healthz alias is opt-in.
type HealthzConfig struct {
	Root HealthzRootConfig `yaml:"root"`
}

// HealthzRootConfig gates the optional root-level /healthz alias. Default
// false: /healthz is only served when server.healthz.root.enabled is true.
type HealthzRootConfig struct {
	Enabled bool `yaml:"enabled"`
}

// BackupConfig holds backup encryption/retention configuration per AI.md
// PART 22. The encryption password itself is NEVER stored here — only
// whether one has been set (Encryption.Enabled).
type BackupConfig struct {
	Encryption BackupEncryptionConfig `yaml:"encryption"`
	Retention  BackupRetentionConfig  `yaml:"retention"`
}

// BackupEncryptionConfig tracks whether a backup password has been
// configured. Enabled is set true when an admin sets a password via the
// WebUI/API/setup wizard; the password itself is prompted on-demand and
// never persisted to server.yml.
type BackupEncryptionConfig struct {
	Enabled bool `yaml:"enabled"`
}

// BackupRetentionConfig controls how many backups are kept per AI.md
// PART 22 "Backup Retention". MaxTotalSize accepts a percentage ("10%"),
// an absolute size ("50G"), or a falsey value ("0", "false", "off", ...)
// to disable the cap.
type BackupRetentionConfig struct {
	MaxBackups   int    `yaml:"max_backups"`
	KeepWeekly   int    `yaml:"keep_weekly"`
	KeepMonthly  int    `yaml:"keep_monthly"`
	KeepYearly   int    `yaml:"keep_yearly"`
	MaxTotalSize string `yaml:"max_total_size"`
}

// ComplianceConfig enables compliance mode (HIPAA, SOC2, etc.) per AI.md
// PART 22. When Enabled, Server.Backup.Encryption.Enabled MUST be true or
// backups are blocked until an encryption password is set.
type ComplianceConfig struct {
	Enabled bool `yaml:"enabled"`
}

// GeoIPConfig holds GeoIP database configuration per AI.md PART 20.
// Databases are sourced from sapics/ip-location-db (no API key required).
type GeoIPConfig struct {
	Enabled        bool                 `yaml:"enabled"`
	Dir            string               `yaml:"dir"`             // {data_dir}/security/geoip when blank
	DenyCountries  []string             `yaml:"deny_countries"`  // ISO 3166-1 alpha-2
	AllowCountries []string             `yaml:"allow_countries"` // ISO 3166-1 alpha-2; wins if both set
	Databases      GeoIPDatabasesConfig `yaml:"databases"`
}

// GeoIPDatabasesConfig selects which MMDB databases to download/use.
type GeoIPDatabasesConfig struct {
	ASN     bool `yaml:"asn"`
	Country bool `yaml:"country"`
	City    bool `yaml:"city"`
}

// TorConfig holds Tor hidden service + outbound network configuration per
// AI.md PART 32. Hidden service is auto-enabled when the tor binary is found
// on PATH; this struct only controls outbound network behaviour, performance,
// and bandwidth caps. Server.tor.binary is auto-detected when blank.
type TorConfig struct {
	Binary                    string `yaml:"binary"`
	UseNetwork                bool   `yaml:"use_network"`
	AllowUserPreference       bool   `yaml:"allow_user_preference"`
	MaxCircuits               int    `yaml:"max_circuits"`
	CircuitTimeout            string `yaml:"circuit_timeout"`
	BootstrapTimeout          string `yaml:"bootstrap_timeout"`
	SafeLogging               bool   `yaml:"safe_logging"`
	MaxStreamsPerCircuit      int    `yaml:"max_streams_per_circuit"`
	CloseCircuitOnStreamLimit bool   `yaml:"close_circuit_on_stream_limit"`
	BandwidthRate             string `yaml:"bandwidth_rate"`
	BandwidthBurst            string `yaml:"bandwidth_burst"`
	MaxMonthlyBandwidth       string `yaml:"max_monthly_bandwidth"`
	NumIntroPoints            int    `yaml:"num_intro_points"`
	VirtualPort               int    `yaml:"virtual_port"`
}

// SecurityConfig holds security policy configuration per AI.md PART 17.
type SecurityConfig struct {
	Password  PasswordPolicyConfig `yaml:"password"`
	Blocklist BlocklistConfig      `yaml:"blocklist"`
	CVE       CVEConfig            `yaml:"cve"`

	// EncryptionKey is the canonical at-rest AES-256-GCM key per AI.md
	// PART 11 ("Cryptographic Keys"): base64-encoded 32 random bytes,
	// auto-generated on first run and persisted to server.yml. Used for
	// 2FA secrets and any other data the spec calls out as "encrypted at
	// rest" (security report bodies fall back to this key when no PGP
	// keypair exists).
	EncryptionKey string `yaml:"encryption_key"`
	// EncryptionKeyVersion is incremented on every "Rotate Encryption Key"
	// admin action; starts at 1.
	EncryptionKeyVersion int `yaml:"encryption_key_version"`
}

// BlocklistConfig holds IP/domain blocklist source configuration per AI.md PART 19.
// When Sources is empty the blocklist_update scheduler task silently skips.
type BlocklistConfig struct {
	// Sources lists remote blocklist files to download and cache locally.
	Sources []BlocklistSource `yaml:"sources"`
	// Dir overrides the default storage path ({data_dir}/security/blocklists).
	Dir string `yaml:"dir"`
}

// BlocklistSource describes a single remote blocklist feed.
type BlocklistSource struct {
	// Name is a human-readable label used in logs and the admin panel.
	Name string `yaml:"name"`
	// URL is the HTTP(S) address of the blocklist file (one entry per line).
	URL string `yaml:"url"`
	// Type is "ip", "domain", or "mixed".
	Type string `yaml:"type"`
	// Enabled controls whether this source is downloaded. Defaults to true.
	Enabled bool `yaml:"enabled"`
}

// CVEConfig holds CVE/security database source configuration per AI.md PART 19.
// When Sources is empty the cve_update scheduler task silently skips.
type CVEConfig struct {
	// Sources lists remote CVE database feeds to download and cache locally.
	Sources []CVESource `yaml:"sources"`
	// Dir overrides the default storage path ({data_dir}/security/cve).
	Dir string `yaml:"dir"`
}

// CVESource describes a single remote CVE/security database feed.
type CVESource struct {
	// Name is a human-readable label used in logs and the admin panel.
	Name string `yaml:"name"`
	// URL is the HTTP(S) address of the CVE feed (JSON or plain-text).
	URL string `yaml:"url"`
	// Enabled controls whether this source is downloaded. Defaults to true.
	Enabled bool `yaml:"enabled"`
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
	MaxBodySize  string `yaml:"max_body_size"` // e.g., "10MB"
	ReadTimeout  int    `yaml:"read_timeout"`  // seconds
	WriteTimeout int    `yaml:"write_timeout"` // seconds
	IdleTimeout  int    `yaml:"idle_timeout"`  // seconds
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
	Admin             SessionCookieConfig `yaml:"admin"`
	User              SessionCookieConfig `yaml:"user"`
	ExtendOnActivity  bool                `yaml:"extend_on_activity"`
	Secure            string              `yaml:"secure"` // auto|true|false
	HTTPOnly          bool                `yaml:"http_only"`
	SameSite          string              `yaml:"same_site"`           // strict|lax|none
	Timeout           string              `yaml:"timeout"`             // e.g. "24h"
	RememberMeTimeout string              `yaml:"remember_me_timeout"` // e.g. "720h"
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

// PrivacyConfig holds server-wide privacy settings, including the always-on
// cookie-consent banner (GDPR/CCPA) per AI.md PART 12. The banner is never
// disabled — sessions and preferences use cookies — but its text adapts to
// whether user data is sold (Data.Sold).
type PrivacyConfig struct {
	Data    PrivacyDataConfig `yaml:"data"`
	Consent ConsentConfig     `yaml:"consent"`
}

// PrivacyDataConfig drives dynamic CCPA "Do Not Sell" messaging.
type PrivacyDataConfig struct {
	Sold bool `yaml:"sold"`
}

// ConsentConfig configures the cookie-consent banner text, links, and buttons.
type ConsentConfig struct {
	Message         string         `yaml:"message"`
	MessageIfSold   string         `yaml:"message_if_sold"`
	Policy          ConsentPolicy  `yaml:"policy"`
	Buttons         ConsentButtons `yaml:"buttons"`
	Position        string         `yaml:"position"`
	ShowPreferences bool           `yaml:"show_preferences"`
	PreferencesText string         `yaml:"preferences_text"`
}

// ConsentPolicy is the inline privacy-policy link shown in the banner.
type ConsentPolicy struct {
	Text string `yaml:"text"`
	URL  string `yaml:"url"`
}

// ConsentButtons holds the accept/decline button labels.
type ConsentButtons struct {
	Accept  string `yaml:"accept"`
	Decline string `yaml:"decline"`
}

// GetConsentMessage returns the banner message appropriate to the data-sold
// setting: MessageIfSold when data is sold (and set), otherwise Message.
func (p PrivacyConfig) GetConsentMessage() string {
	if p.Data.Sold && p.Consent.MessageIfSold != "" {
		return p.Consent.MessageIfSold
	}
	return p.Consent.Message
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
	Provider string     `yaml:"provider"` // smtp|sendgrid|ses
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
	Title        string `yaml:"title"`
	Tagline      string `yaml:"tagline"`
	Description  string `yaml:"description"`
	LogoURL      string `yaml:"logo_url"`
	FaviconURL   string `yaml:"favicon_url"`
	DefaultTheme string `yaml:"default_theme"`
	PrimaryColor string `yaml:"primary_color"`
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
	Enabled     bool              `yaml:"enabled"`
	Cert        string            `yaml:"cert"`
	Key         string            `yaml:"key"`
	MinVersion  string            `yaml:"min_version"`
	LetsEncrypt LetsEncryptConfig `yaml:"letsencrypt"`
}

// LetsEncryptConfig holds Let's Encrypt settings
type LetsEncryptConfig struct {
	Enabled   bool     `yaml:"enabled"`
	Email     string   `yaml:"email"`
	Challenge string   `yaml:"challenge"`
	Staging   bool     `yaml:"staging"`
	Domains   []string `yaml:"domains"`
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
	Enabled                  bool `yaml:"enabled"`
	Requests                 int  `yaml:"requests"`
	Window                   int  `yaml:"window"`
	Burst                    int  `yaml:"burst"`
	LoginMaxAttempts         int  `yaml:"login_max_attempts"`
	PasswordResetMaxAttempts int  `yaml:"password_reset_max_attempts"`
}

// SchedulerConfig holds scheduler settings per AI.md PART 19. Per-task cron
// expressions are the live, authoritative schedules — the scheduler package
// reads these directly (no hardcoded fallbacks) so the admin panel and
// server.yml are the single source of truth for "Task Configuration".
type SchedulerConfig struct {
	Enabled bool `yaml:"enabled"`

	// Timezone used to evaluate cron expressions (PART 19 "Core Requirements").
	// Empty or invalid falls back to UTC.
	Timezone string `yaml:"timezone"`

	// CatchUpWindow: run missed tasks on startup if the missed next_run falls
	// within this duration of now (PART 19 "Startup Behavior"). Duration
	// string, e.g. "1h".
	CatchUpWindow string `yaml:"catch_up_window"`

	// MaxRetries/RetryDelay are the default retry policy (PART 19 "Retry
	// Policy"); backoff is always exponential (retry_delay, *2, *4, ...).
	MaxRetries int    `yaml:"max_retries"`
	RetryDelay string `yaml:"retry_delay"`

	SessionCleanupCron    string `yaml:"session_cleanup_cron"`
	SessionCleanupEnabled bool   `yaml:"session_cleanup_enabled"`

	TokenCleanupCron    string `yaml:"token_cleanup_cron"`
	TokenCleanupEnabled bool   `yaml:"token_cleanup_enabled"`

	ExpireURLsCron    string `yaml:"expire_urls_cron"`
	ExpireURLsEnabled bool   `yaml:"expire_urls_enabled"`

	LogRotationCron    string `yaml:"log_rotation_cron"`
	LogRotationEnabled bool   `yaml:"log_rotation_enabled"`

	BackupCron    string `yaml:"backup_cron"`
	BackupEnabled bool   `yaml:"backup_enabled"`

	BackupHourlyCron    string `yaml:"backup_hourly_cron"`
	BackupHourlyEnabled bool   `yaml:"backup_hourly_enabled"`

	SSLRenewalCron    string `yaml:"ssl_renewal_cron"`
	SSLRenewalEnabled bool   `yaml:"ssl_renewal_enabled"`

	GeoIPUpdateCron    string `yaml:"geoip_update_cron"`
	GeoIPUpdateEnabled bool   `yaml:"geoip_update_enabled"`

	BlocklistUpdateCron    string `yaml:"blocklist_update_cron"`
	BlocklistUpdateEnabled bool   `yaml:"blocklist_update_enabled"`

	CVEUpdateCron    string `yaml:"cve_update_cron"`
	CVEUpdateEnabled bool   `yaml:"cve_update_enabled"`

	// UpdateCheck* implement the `update_check` task (PART 19): notify-only
	// unless UpdateAutoInstall is true; UpdateDeferDays delays install after
	// a release is first observed.
	UpdateCheckCron    string `yaml:"update_check_cron"`
	UpdateCheckEnabled bool   `yaml:"update_check_enabled"`
	UpdateBranch       string `yaml:"update_branch"`
	UpdateAutoInstall  bool   `yaml:"update_auto_install"`
	UpdateDeferDays    int    `yaml:"update_defer_days"`

	HealthcheckCron    string `yaml:"healthcheck_cron"`
	HealthcheckEnabled bool   `yaml:"healthcheck_enabled"`

	TorHealthCron    string `yaml:"tor_health_cron"`
	TorHealthEnabled bool   `yaml:"tor_health_enabled"`

	// ClusterHeartbeatCron/Enabled implement `cluster_heartbeat` (PART 19);
	// the task is a no-op when the node is not running in cluster mode.
	ClusterHeartbeatCron    string `yaml:"cluster_heartbeat_cron"`
	ClusterHeartbeatEnabled bool   `yaml:"cluster_heartbeat_enabled"`

	// DomainVerificationCron/Enabled implement the custom-domain maintenance
	// task (PART 36): retry pending/failed DNS-TXT verifications still inside
	// the verification_ttl window and clean up domains left unverified past it.
	// The task is a no-op when the custom_domains feature is disabled.
	DomainVerificationCron    string `yaml:"domain_verification_cron"`
	DomainVerificationEnabled bool   `yaml:"domain_verification_enabled"`
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
	Users           UsersConfig         `yaml:"users"`
	Organizations   OrganizationsConfig `yaml:"organizations"`
	CustomDomains   CustomDomainsConfig `yaml:"custom_domains"`
	Billing         BillingConfig       `yaml:"billing"`
	Federation      FederationConfig    `yaml:"federation"`
	TOTPIssuer      string              `yaml:"totp_issuer"`      // issuer name shown in authenticator apps
	WebAuthnDisplay string              `yaml:"webauthn_display"` // display name for WebAuthn/FIDO2
}

// UsersConfig holds user management settings
type UsersConfig struct {
	Enabled      bool               `yaml:"enabled"`
	Registration RegistrationConfig `yaml:"registration"`
	Profile      ProfileConfig      `yaml:"profile"`
}

// RegistrationConfig holds registration settings
type RegistrationConfig struct {
	Enabled                  bool   `yaml:"enabled"`
	Mode                     string `yaml:"mode"`
	RequireEmailVerification bool   `yaml:"require_email_verification"`
	RequireApproval          bool   `yaml:"require_approval"`
	AllowDisposableEmails    bool   `yaml:"allow_disposable_emails"`
}

// NormalizedMode returns the registration mode lowercased and trimmed,
// substituting the "open" default for an empty or unrecognized value (PART 34).
func (r RegistrationConfig) NormalizedMode() string {
	switch m := strings.ToLower(strings.TrimSpace(r.Mode)); m {
	case "open", "invite", "admin_only", "disabled":
		return m
	default:
		return "open"
	}
}

// PublicSelfRegistrationAllowed reports whether an unauthenticated visitor may
// create their own account. Only the "open" mode permits public self-service
// registration (PART 34); invite, admin_only, and disabled all forbid it, as
// does disabling registration entirely.
func (r RegistrationConfig) PublicSelfRegistrationAllowed() bool {
	return r.Enabled && r.NormalizedMode() == "open"
}

// InviteAcceptanceAllowed reports whether a valid user-registration invite or
// activation link may still be consumed under the current mode. Invites are
// honored in open, invite, and admin_only modes, but the disabled mode rejects
// every existing unused link (PART 34); disabling the feature honors none.
func (r RegistrationConfig) InviteAcceptanceAllowed() bool {
	if !r.Enabled {
		return false
	}
	switch r.NormalizedMode() {
	case "open", "invite", "admin_only":
		return true
	default:
		return false
	}
}

// ProfileConfig holds user profile settings
type ProfileConfig struct {
	AllowDisplayName bool `yaml:"allow_display_name"`
	AllowAvatar      bool `yaml:"allow_avatar"`
	AllowBio         bool `yaml:"allow_bio"`
}

// OrganizationsConfig holds organization settings
type OrganizationsConfig struct {
	Enabled       bool              `yaml:"enabled"`
	AllowCreation bool              `yaml:"allow_creation"`
	Creation      OrgCreationConfig `yaml:"creation"`
	MaxPerUser    int               `yaml:"max_per_user"`
	Roles         []string          `yaml:"roles"`
}

// OrgCreationConfig holds the server-level organization creation policy.
type OrgCreationConfig struct {
	Mode string `yaml:"mode"`
}

// NormalizedCreationMode returns the org creation mode lowercased and trimmed,
// substituting the "open" default for an empty or unrecognized value (PART 35).
func (o OrganizationsConfig) NormalizedCreationMode() string {
	switch m := strings.ToLower(strings.TrimSpace(o.Creation.Mode)); m {
	case "open", "invite", "admin_only", "disabled":
		return m
	default:
		return "open"
	}
}

// AuthenticatedCreationAllowed reports whether an ordinary authenticated user
// may create a new organization through the self-service routes. Only the
// "open" mode permits it (PART 35); invite, admin_only, and disabled route
// creation elsewhere or forbid it, as does turning off AllowCreation entirely.
func (o OrganizationsConfig) AuthenticatedCreationAllowed() bool {
	return o.Enabled && o.AllowCreation && o.NormalizedCreationMode() == "open"
}

// OrgCreationInviteAllowed reports whether a valid organization-creation invite
// may be consumed to create an organization under the current mode. Creation
// invites apply to the invite mode (the open mode already allows unconditional
// creation); admin_only routes creation through an administrator and disabled
// forbids it entirely (PART 35).
func (o OrganizationsConfig) OrgCreationInviteAllowed() bool {
	if !o.Enabled {
		return false
	}
	switch o.NormalizedCreationMode() {
	case "open", "invite":
		return true
	default:
		return false
	}
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
	MinRandomLength   int      `yaml:"min_random_length"`
	MaxCustomLength   int      `yaml:"max_custom_length"`
	DefaultExpiration string   `yaml:"default_expiration"`
	AllowCustomCodes  bool     `yaml:"allow_custom_codes"`
	PerUserLimit      int      `yaml:"per_user_limit"`
	PerOrgLimit       int      `yaml:"per_org_limit"`
	ReservedWords     []string `yaml:"reserved_words"`
}

// AnalyticsConfig holds analytics settings
type AnalyticsConfig struct {
	Enabled           bool `yaml:"enabled"`
	EnableGeolocation bool `yaml:"enable_geolocation"`
	AnonymizeIPs      bool `yaml:"anonymize_ips"`
	RetentionDays     int  `yaml:"retention_days"`
}

// QRConfig holds QR code settings
type QRConfig struct {
	DefaultSize     int    `yaml:"default_size"`
	MaxSize         int    `yaml:"max_size"`
	DefaultFormat   string `yaml:"default_format"`
	ErrorCorrection string `yaml:"error_correction"`
}

// Load loads configuration from server.yml
func Load(configDir string) (*Config, error) {
	configPath := filepath.Join(configDir, "server.yml")

	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		cfg := DefaultConfig()
		if err := ensureEncryptionKey(cfg); err != nil {
			return nil, err
		}
		if err := Save(configDir, cfg); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		applyEnvOverrides(cfg)
		// Re-validate after env overrides so out-of-range env values
		// (e.g. an invalid CASLINK_PORT) are clamped/rejected here too.
		if err := Validate(cfg); err != nil {
			return nil, fmt.Errorf("invalid config after env overrides: %w", err)
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

	// First-run / upgrade path: generate server.security.encryption_key if
	// this config predates it (AI.md PART 11 "Cryptographic Keys"), then
	// persist it so it survives a restart.
	if cfg.Server.Security.EncryptionKey == "" {
		if err := ensureEncryptionKey(&cfg); err != nil {
			return nil, err
		}
		if err := Save(configDir, &cfg); err != nil {
			return nil, fmt.Errorf("failed to persist generated encryption key: %w", err)
		}
	}

	// Apply environment variable overrides (PART 26 precedence: env > config > default).
	applyEnvOverrides(&cfg)

	// Re-validate after env overrides so out-of-range env values are caught.
	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config after env overrides: %w", err)
	}

	return &cfg, nil
}

// ensureEncryptionKey generates server.security.encryption_key when absent,
// per AI.md PART 11: "auto-generated on first run", starting
// encryption_key_version at 1.
func ensureEncryptionKey(cfg *Config) error {
	if cfg.Server.Security.EncryptionKey != "" {
		return nil
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}
	cfg.Server.Security.EncryptionKey = key
	cfg.Server.Security.EncryptionKeyVersion = 1
	return nil
}

// applyEnvOverrides overlays environment variables on top of the loaded config.
// Checks CASLINK_* prefix first (per AI.md PART 9 / {PROJECT_NAME}_* spec),
// then falls back to bare names for backward compatibility.
// applyEnvOverrides overlays CASLINK_* environment variables on top of the
// loaded config per AI.md PART 12 hierarchy. ALL boolean values use
// config.ParseBool() — never strconv.ParseBool().
func applyEnvOverrides(cfg *Config) {
	// Server basics
	if v := envStr("MODE"); v != "" {
		cfg.Server.Mode = v
	}
	if v := envStr("DOMAIN"); v != "" {
		cfg.Server.FQDN = v
	}
	if v := envStr("FQDN"); v != "" {
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
	if v := envStr("ADDRESS"); v != "" {
		cfg.Server.Address = v
	}

	// Admin
	if v := envStr("ADMIN_EMAIL"); v != "" {
		cfg.Server.Admin.Email = v
	}
	if v := envStr("ADMIN_PATH"); v != "" {
		cfg.Server.Admin.Path = v
	}

	// Database
	if v := envStr("DATABASE_DRIVER"); v != "" {
		cfg.Server.Database.Driver = v
	}
	if v := envStr("DATABASE_URL"); v != "" {
		cfg.Server.Database.Host = v // full DSN — factory.go resolves per driver
	}
	if v := envStr("DATABASE_HOST"); v != "" {
		cfg.Server.Database.Host = v
	}
	if v := envStr("DATABASE_NAME"); v != "" {
		cfg.Server.Database.Name = v
	}
	if v := envStr("DATABASE_USERNAME"); v != "" {
		cfg.Server.Database.Username = v
	}
	if v := envStr("DATABASE_PASSWORD"); v != "" {
		cfg.Server.Database.Password = v
	}

	// SSL / Let's Encrypt (booleans use ParseBool per AI.md PART 12)
	if v := envStr("SSL_ENABLED"); v != "" {
		if b, err := ParseBool(v, cfg.Server.SSL.Enabled); err == nil {
			cfg.Server.SSL.Enabled = b
		} else {
			fmt.Fprintf(os.Stderr, "config: warning: invalid SSL_ENABLED value %q, keeping default\n", v)
		}
	}
	if v := envStr("LE_ENABLED"); v != "" {
		if b, err := ParseBool(v, cfg.Server.SSL.LetsEncrypt.Enabled); err == nil {
			cfg.Server.SSL.LetsEncrypt.Enabled = b
		} else {
			fmt.Fprintf(os.Stderr, "config: warning: invalid LE_ENABLED value %q, keeping default\n", v)
		}
	}
	if v := envStr("LE_EMAIL"); v != "" {
		cfg.Server.SSL.LetsEncrypt.Email = v
	}
	if v := envStr("LE_CHALLENGE"); v != "" {
		cfg.Server.SSL.LetsEncrypt.Challenge = v
	}
	if v := envStr("LE_STAGING"); v != "" {
		if b, err := ParseBool(v, cfg.Server.SSL.LetsEncrypt.Staging); err == nil {
			cfg.Server.SSL.LetsEncrypt.Staging = b
		} else {
			fmt.Fprintf(os.Stderr, "config: warning: invalid LE_STAGING value %q, keeping default\n", v)
		}
	}

	// Rate limiting
	if v := envStr("RATE_LIMIT_ENABLED"); v != "" {
		if b, err := ParseBool(v, cfg.Server.RateLimit.Enabled); err == nil {
			cfg.Server.RateLimit.Enabled = b
		} else {
			fmt.Fprintf(os.Stderr, "config: warning: invalid RATE_LIMIT_ENABLED value %q, keeping default\n", v)
		}
	}
	if v := envStr("RATE_LIMIT_REQUESTS"); v != "" {
		if n := parseInt(v); n > 0 {
			cfg.Server.RateLimit.Requests = n
		}
	}

	// Metrics
	if v := envStr("METRICS_ENABLED"); v != "" {
		if b, err := ParseBool(v, cfg.Server.Metrics.Enabled); err == nil {
			cfg.Server.Metrics.Enabled = b
		} else {
			fmt.Fprintf(os.Stderr, "config: warning: invalid METRICS_ENABLED value %q, keeping default\n", v)
		}
	}
	if v := envStr("METRICS_TOKEN"); v != "" {
		cfg.Server.Metrics.Token = v
	}

	// GeoIP
	if v := envStr("GEOIP_ENABLED"); v != "" {
		if b, err := ParseBool(v, cfg.Server.GeoIP.Enabled); err == nil {
			cfg.Server.GeoIP.Enabled = b
		} else {
			fmt.Fprintf(os.Stderr, "config: warning: invalid GEOIP_ENABLED value %q, keeping default\n", v)
		}
	}

	// Email / SMTP
	if v := envStr("EMAIL_ENABLED"); v != "" {
		if b, err := ParseBool(v, cfg.Server.Notifications.Email.Enabled); err == nil {
			cfg.Server.Notifications.Email.Enabled = b
		} else {
			fmt.Fprintf(os.Stderr, "config: warning: invalid EMAIL_ENABLED value %q, keeping default\n", v)
		}
	}
	if v := envStr("SMTP_HOST"); v != "" {
		cfg.Server.Notifications.Email.SMTP.Host = v
	}
	if v := envStr("SMTP_PORT"); v != "" {
		if n := parseInt(v); n > 0 {
			cfg.Server.Notifications.Email.SMTP.Port = n
		}
	}
	if v := envStr("SMTP_USERNAME"); v != "" {
		cfg.Server.Notifications.Email.SMTP.Username = v
	}
	if v := envStr("SMTP_PASSWORD"); v != "" {
		cfg.Server.Notifications.Email.SMTP.Password = v
	}
	if v := envStr("EMAIL_FROM"); v != "" {
		cfg.Server.Notifications.Email.From = v
	}

	// Branding
	if v := envStr("APP_NAME"); v != "" {
		cfg.Server.Branding.Title = v
	}
	if v := envStr("APP_DESCRIPTION"); v != "" {
		cfg.Server.Branding.Description = v
	}

	// Caslink features
	if v := envStr("MIN_RANDOM_LENGTH"); v != "" {
		if n := parseInt(v); n > 0 {
			cfg.Caslink.URL.MinRandomLength = n
		}
	}
	if v := envStr("ANALYTICS_ENABLED"); v != "" {
		if b, err := ParseBool(v, cfg.Caslink.Analytics.Enabled); err == nil {
			cfg.Caslink.Analytics.Enabled = b
		} else {
			fmt.Fprintf(os.Stderr, "config: warning: invalid ANALYTICS_ENABLED value %q, keeping default\n", v)
		}
	}
	if v := envStr("ANONYMIZE_IPS"); v != "" {
		if b, err := ParseBool(v, cfg.Caslink.Analytics.AnonymizeIPs); err == nil {
			cfg.Caslink.Analytics.AnonymizeIPs = b
		} else {
			fmt.Fprintf(os.Stderr, "config: warning: invalid ANONYMIZE_IPS value %q, keeping default\n", v)
		}
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

// parseInt parses an unsigned decimal string for env-var overrides. It
// returns 0 on any non-digit character or on empty input — callers must
// treat 0 as "unset/invalid" and fall back to the existing config value
// (every call site already guards with `n > 0` before applying it). There is
// no overflow guard; this is only ever used for small config sizes (worker
// counts, pool sizes, timeouts), never for user-controlled large values.
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
	if err := os.WriteFile(configPath, data, 0600); err != nil {
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
			Port:       0, // 0 = auto-select from 64xxx range
			Address:    "[::]",
			Mode:       "production",
			FQDN:       hostname,
			APIVersion: "v1",
			Daemonize:  false,
			PIDFile:    true,
			Healthz: HealthzConfig{
				Root: HealthzRootConfig{Enabled: false},
			},
			Branding: BrandingConfig{
				Title:        "caslink",
				Tagline:      "",
				Description:  "",
				LogoURL:      "",
				FaviconURL:   "",
				DefaultTheme: "dark",
				PrimaryColor: "",
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
					MaxAge:      86400, // 24 hours
					IdleTimeout: 3600,  // 1 hour
				},
				User: SessionCookieConfig{
					CookieName:  "caslink_session",
					MaxAge:      2592000, // 30 days
					IdleTimeout: 86400,   // 24 hours
				},
				ExtendOnActivity:  true,
				Secure:            "auto",
				HTTPOnly:          true,
				SameSite:          "lax",
				Timeout:           "24h",
				RememberMeTimeout: "720h",
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
			Privacy: PrivacyConfig{
				Data: PrivacyDataConfig{Sold: false},
				Consent: ConsentConfig{
					Message:       "In accordance with the EU GDPR law this message is being displayed. By using this site you consent to our use of cookies.",
					MessageIfSold: "This site uses cookies and may share limited data with third parties. See our privacy policy for details and your opt-out rights.",
					Policy: ConsentPolicy{
						Text: "Privacy Policy",
						URL:  "/server/privacy",
					},
					Buttons: ConsentButtons{
						Accept:  "I Agree",
						Decline: "Decline",
					},
					Position:        "bottom",
					ShowPreferences: true,
					PreferencesText: "Manage Preferences",
				},
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
				Enabled:                  true,
				Requests:                 120,
				Window:                   60,
				Burst:                    10,
				LoginMaxAttempts:         5,
				PasswordResetMaxAttempts: 3,
			},
			Scheduler: SchedulerConfig{
				Enabled:                   true,
				Timezone:                  "America/New_York",
				CatchUpWindow:             "1h",
				MaxRetries:                3,
				RetryDelay:                "5m",
				SessionCleanupCron:        "@every 15m",
				SessionCleanupEnabled:     true,
				TokenCleanupCron:          "@every 15m",
				TokenCleanupEnabled:       true,
				ExpireURLsCron:            "30 2 * * *",
				ExpireURLsEnabled:         true,
				LogRotationCron:           "0 0 * * *",
				LogRotationEnabled:        true,
				BackupCron:                "0 2 * * *",
				BackupEnabled:             true,
				BackupHourlyCron:          "@hourly",
				BackupHourlyEnabled:       false,
				SSLRenewalCron:            "0 3 * * *",
				SSLRenewalEnabled:         true,
				GeoIPUpdateCron:           "0 3 * * 0",
				GeoIPUpdateEnabled:        true,
				BlocklistUpdateCron:       "0 4 * * *",
				BlocklistUpdateEnabled:    true,
				CVEUpdateCron:             "0 5 * * *",
				CVEUpdateEnabled:          true,
				UpdateCheckCron:           "0 6 * * *",
				UpdateCheckEnabled:        true,
				UpdateBranch:              "stable",
				UpdateAutoInstall:         false,
				UpdateDeferDays:           0,
				HealthcheckCron:           "@every 5m",
				HealthcheckEnabled:        true,
				TorHealthCron:             "@every 10m",
				TorHealthEnabled:          true,
				ClusterHeartbeatCron:      "@every 30s",
				ClusterHeartbeatEnabled:   true,
				DomainVerificationCron:    "@every 30m",
				DomainVerificationEnabled: true,
			},
			Features: FeaturesConfig{
				Users: UsersConfig{
					Enabled: true,
					Registration: RegistrationConfig{
						Enabled:                  true,
						Mode:                     "open",
						RequireEmailVerification: false,
						RequireApproval:          false,
						AllowDisposableEmails:    false,
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
					Creation:      OrgCreationConfig{Mode: "open"},
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
				TOTPIssuer:      "Caslink",
				WebAuthnDisplay: "Caslink",
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
			GeoIP: GeoIPConfig{
				Enabled:        true,
				Dir:            "", // resolved to {data_dir}/security/geoip at runtime
				DenyCountries:  []string{},
				AllowCountries: []string{},
				Databases: GeoIPDatabasesConfig{
					ASN:     true,
					Country: true,
					City:    true,
				},
			},
			Tor: TorConfig{
				Binary:                    "",
				UseNetwork:                false,
				AllowUserPreference:       true,
				MaxCircuits:               32,
				CircuitTimeout:            "60s",
				BootstrapTimeout:          "3m",
				SafeLogging:               true,
				MaxStreamsPerCircuit:      100,
				CloseCircuitOnStreamLimit: true,
				BandwidthRate:             "1 MB",
				BandwidthBurst:            "2 MB",
				MaxMonthlyBandwidth:       "100 GB",
				NumIntroPoints:            3,
				VirtualPort:               80,
			},
			Backup: BackupConfig{
				Encryption: BackupEncryptionConfig{
					Enabled: false,
				},
				Retention: BackupRetentionConfig{
					MaxBackups:   1,
					KeepWeekly:   0,
					KeepMonthly:  0,
					KeepYearly:   0,
					MaxTotalSize: "10%",
				},
			},
			Compliance: ComplianceConfig{
				Enabled: false,
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
				MinRandomLength:   6,
				MaxCustomLength:   50,
				DefaultExpiration: "never",
				AllowCustomCodes:  true,
				PerUserLimit:      0,
				PerOrgLimit:       0,
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
	// Mode — unknown values fall back to production (AI.md PART 12).
	if cfg.Server.Mode != "production" && cfg.Server.Mode != "development" {
		fmt.Printf("config: invalid mode %q — defaulting to production\n", cfg.Server.Mode)
		cfg.Server.Mode = "production"
	}

	// Database driver — unknown values fall back to sqlite.
	validDrivers := map[string]bool{
		"file": true, "sqlite": true, "postgres": true,
		"mysql": true, "mariadb": true, "mssql": true,
	}
	if !validDrivers[cfg.Server.Database.Driver] {
		fmt.Printf("config: invalid database driver %q — defaulting to sqlite\n", cfg.Server.Database.Driver)
		cfg.Server.Database.Driver = "sqlite"
	}

	// Port — 0 means auto-select; negative or >65535 is invalid.
	if cfg.Server.Port < 0 || cfg.Server.Port > 65535 {
		fmt.Printf("config: invalid port %d — will auto-select from 64xxx range\n", cfg.Server.Port)
		cfg.Server.Port = 0
	}

	// Rate limit — requests must be positive when enabled.
	if cfg.Server.RateLimit.Enabled && cfg.Server.RateLimit.Requests <= 0 {
		fmt.Printf("config: rate_limit.requests must be > 0 when enabled — defaulting to 120\n")
		cfg.Server.RateLimit.Requests = 120
	}
	if cfg.Server.RateLimit.Enabled && cfg.Server.RateLimit.Window <= 0 {
		fmt.Printf("config: rate_limit.window must be > 0 when enabled — defaulting to 60\n")
		cfg.Server.RateLimit.Window = 60
	}
	if cfg.Server.RateLimit.Burst < 0 {
		fmt.Printf("config: rate_limit.burst must be >= 0 — defaulting to 10\n")
		cfg.Server.RateLimit.Burst = 10
	}
	if cfg.Server.RateLimit.LoginMaxAttempts < 0 {
		fmt.Printf("config: rate_limit.login_max_attempts must be >= 0 — defaulting to 5\n")
		cfg.Server.RateLimit.LoginMaxAttempts = 5
	}
	if cfg.Server.RateLimit.PasswordResetMaxAttempts < 0 {
		fmt.Printf("config: rate_limit.password_reset_max_attempts must be >= 0 — defaulting to 3\n")
		cfg.Server.RateLimit.PasswordResetMaxAttempts = 3
	}

	// Session timeouts — fall back to defaults on empty/zero.
	if cfg.Server.Session.Timeout == "" {
		cfg.Server.Session.Timeout = "24h"
	}
	if cfg.Server.Session.RememberMeTimeout == "" {
		cfg.Server.Session.RememberMeTimeout = "720h"
	}

	// Admin path — must be a non-empty lowercase alphanumeric slug.
	if cfg.Server.Admin.Path == "" {
		fmt.Printf("config: admin.path is empty — defaulting to \"admin\"\n")
		cfg.Server.Admin.Path = "admin"
	}

	// SameSite session cookie — unknown values fall back to lax.
	switch cfg.Server.Session.SameSite {
	case "strict", "lax", "none", "":
		// OK
	default:
		fmt.Printf("config: session.same_site %q is not valid — defaulting to lax\n",
			cfg.Server.Session.SameSite)
		cfg.Server.Session.SameSite = "lax"
	}

	// Secure session cookie — unknown values fall back to auto.
	switch cfg.Server.Session.Secure {
	case "true", "false", "auto", "":
		// OK
	default:
		fmt.Printf("config: session.secure %q is not valid — defaulting to auto\n",
			cfg.Server.Session.Secure)
		cfg.Server.Session.Secure = "auto"
	}

	// SSL TLS min version — unknown values fall back to TLS1.2.
	switch cfg.Server.SSL.MinVersion {
	case "TLS1.0", "TLS1.1", "TLS1.2", "TLS1.3", "":
		// OK
	default:
		fmt.Printf("config: ssl.min_version %q is not valid — defaulting to TLS1.2\n",
			cfg.Server.SSL.MinVersion)
		cfg.Server.SSL.MinVersion = "TLS1.2"
	}

	// SMTP port — must be 1-65535 when host is configured.
	if cfg.Server.Notifications.Email.SMTP.Host != "" {
		if cfg.Server.Notifications.Email.SMTP.Port <= 0 || cfg.Server.Notifications.Email.SMTP.Port > 65535 {
			fmt.Printf("config: notifications.email.smtp.port %d is invalid — defaulting to 587\n",
				cfg.Server.Notifications.Email.SMTP.Port)
			cfg.Server.Notifications.Email.SMTP.Port = 587
		}
	}

	// Caslink URL lengths — sanity check.
	if cfg.Caslink.URL.MinRandomLength < 3 {
		fmt.Printf("config: caslink.url.min_random_length %d is too short (min 3) — defaulting to 6\n",
			cfg.Caslink.URL.MinRandomLength)
		cfg.Caslink.URL.MinRandomLength = 6
	}
	if cfg.Caslink.URL.MaxCustomLength < cfg.Caslink.URL.MinRandomLength {
		fmt.Printf("config: caslink.url.max_custom_length %d < min_random_length — defaulting to 50\n",
			cfg.Caslink.URL.MaxCustomLength)
		cfg.Caslink.URL.MaxCustomLength = 50
	}

	// Analytics retention — -1 means unlimited; 0 is invalid.
	if cfg.Caslink.Analytics.RetentionDays == 0 {
		fmt.Printf("config: caslink.analytics.retention_days is 0 — defaulting to 365 (use -1 for unlimited)\n")
		cfg.Caslink.Analytics.RetentionDays = 365
	}

	return nil
}
