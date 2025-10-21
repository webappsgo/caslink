package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// validate performs validation on the configuration
func validate(config *Config) error {
	// Validate server configuration
	if err := validateServer(&config.Server); err != nil {
		return fmt.Errorf("server config: %w", err)
	}

	// Validate database configuration
	if err := validateDatabase(&config.Database); err != nil {
		return fmt.Errorf("database config: %w", err)
	}

	// Validate URL configuration
	if err := validateURL(&config.URL); err != nil {
		return fmt.Errorf("url config: %w", err)
	}

	// Validate authentication configuration
	if err := validateAuth(&config.Auth); err != nil {
		return fmt.Errorf("auth config: %w", err)
	}

	// Validate OAuth configuration
	if err := validateOAuth(&config.OAuth); err != nil {
		return fmt.Errorf("oauth config: %w", err)
	}

	// Validate billing configuration
	if err := validateBilling(&config.Billing); err != nil {
		return fmt.Errorf("billing config: %w", err)
	}

	return nil
}

func validateServer(server *ServerConfig) error {
	// Validate host
	if server.Host != "" && server.Host != "0.0.0.0" {
		if ip := net.ParseIP(server.Host); ip == nil {
			return fmt.Errorf("invalid host IP address: %s", server.Host)
		}
	}

	// Validate port
	if server.Port < 0 || server.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535, got: %d", server.Port)
	}

	// Validate trusted proxies
	for _, proxy := range server.TrustedProxies {
		if _, _, err := net.ParseCIDR(proxy); err != nil {
			if ip := net.ParseIP(proxy); ip == nil {
				return fmt.Errorf("invalid trusted proxy format: %s", proxy)
			}
		}
	}

	// Validate timeouts
	if server.ReadTimeout < 0 {
		return fmt.Errorf("read_timeout cannot be negative")
	}
	if server.WriteTimeout < 0 {
		return fmt.Errorf("write_timeout cannot be negative")
	}
	if server.IdleTimeout < 0 {
		return fmt.Errorf("idle_timeout cannot be negative")
	}

	// Validate max header bytes
	if server.MaxHeaderBytes <= 0 {
		return fmt.Errorf("max_header_bytes must be positive")
	}

	return nil
}

func validateDatabase(db *DatabaseConfig) error {
	// Validate database type
	validTypes := []string{"sqlite", "postgresql", "mysql", "mariadb", "sqlserver", "cockroachdb"}
	if !contains(validTypes, db.Type) {
		return fmt.Errorf("unsupported database type: %s", db.Type)
	}

	// Validate database URL if provided
	if db.URL != "" {
		if !strings.Contains(db.URL, "://") {
			return fmt.Errorf("invalid database URL format")
		}
	}

	// Validate port
	if db.Port < 0 || db.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535, got: %d", db.Port)
	}

	// Validate connection pool settings
	if db.MaxOpenConns < 0 {
		return fmt.Errorf("max_open_conns cannot be negative")
	}
	if db.MaxIdleConns < 0 {
		return fmt.Errorf("max_idle_conns cannot be negative")
	}
	if db.MaxIdleConns > db.MaxOpenConns && db.MaxOpenConns > 0 {
		return fmt.Errorf("max_idle_conns cannot be greater than max_open_conns")
	}

	// Validate timeouts
	if db.ConnMaxLifetime < 0 {
		return fmt.Errorf("conn_max_lifetime cannot be negative")
	}
	if db.MigrationTimeout < 0 {
		return fmt.Errorf("migration_timeout cannot be negative")
	}
	if db.SlowQueryThreshold < 0 {
		return fmt.Errorf("slow_query_threshold cannot be negative")
	}
	if db.SQLiteBusyTimeout < 0 {
		return fmt.Errorf("sqlite_busy_timeout cannot be negative")
	}

	return nil
}

func validateURL(url *URLConfig) error {
	// Validate length constraints
	if url.MinRandomLength <= 0 {
		return fmt.Errorf("min_random_length must be positive")
	}
	if url.MaxRandomLength <= 0 {
		return fmt.Errorf("max_random_length must be positive")
	}
	if url.MinRandomLength > url.MaxRandomLength {
		return fmt.Errorf("min_random_length cannot be greater than max_random_length")
	}

	if url.CustomCodeMinLength <= 0 {
		return fmt.Errorf("custom_code_min_length must be positive")
	}
	if url.CustomCodeMaxLength <= 0 {
		return fmt.Errorf("custom_code_max_length must be positive")
	}
	if url.CustomCodeMinLength > url.CustomCodeMaxLength {
		return fmt.Errorf("custom_code_min_length cannot be greater than custom_code_max_length")
	}

	// Validate allowed characters
	if url.AllowedCharacters == "" {
		return fmt.Errorf("allowed_characters cannot be empty")
	}

	// Validate max URL length
	if url.MaxURLLength <= 0 {
		return fmt.Errorf("max_url_length must be positive")
	}

	// Validate default expiration
	validExpirations := []string{"never", "1h", "24h", "7d", "30d"}
	if !contains(validExpirations, url.DefaultExpiration) {
		return fmt.Errorf("invalid default_expiration: %s", url.DefaultExpiration)
	}

	return nil
}

func validateAuth(auth *AuthConfig) error {
	// Validate password requirements
	if auth.PasswordMinLength < 1 {
		return fmt.Errorf("password_min_length must be at least 1")
	}

	// Validate API token settings
	if auth.APITokenLength < 16 {
		return fmt.Errorf("api_token_length must be at least 16")
	}

	// Validate session timeout
	if auth.SessionTimeout < time.Minute {
		return fmt.Errorf("session_timeout must be at least 1 minute")
	}

	// Validate TOTP issuer name
	if auth.EnableTOTP && auth.TOTPIssuerName == "" {
		return fmt.Errorf("totp_issuer_name is required when TOTP is enabled")
	}

	// Validate WebAuthn display name
	if auth.EnableWebAuthn && auth.WebAuthnDisplayName == "" {
		return fmt.Errorf("webauthn_display_name is required when WebAuthn is enabled")
	}

	return nil
}

func validateOAuth(oauth *OAuthConfig) error {
	if !oauth.Enabled {
		return nil
	}

	// Validate required OAuth fields
	if oauth.ClientID == "" {
		return fmt.Errorf("oauth client_id is required when OAuth is enabled")
	}
	if oauth.ClientSecret == "" {
		return fmt.Errorf("oauth client_secret is required when OAuth is enabled")
	}

	// Validate URLs
	if oauth.AuthorizeURL != "" {
		if _, err := url.Parse(oauth.AuthorizeURL); err != nil {
			return fmt.Errorf("invalid oauth authorize_url: %w", err)
		}
	}
	if oauth.TokenURL != "" {
		if _, err := url.Parse(oauth.TokenURL); err != nil {
			return fmt.Errorf("invalid oauth token_url: %w", err)
		}
	}
	if oauth.UserInfoURL != "" {
		if _, err := url.Parse(oauth.UserInfoURL); err != nil {
			return fmt.Errorf("invalid oauth userinfo_url: %w", err)
		}
	}

	return nil
}

func validateBilling(billing *BillingConfig) error {
	if !billing.Enabled {
		return nil
	}

	// Validate billing provider
	validProviders := []string{"stripe", "paypal", "paddle", "lemonsqueezy", "manual"}
	if !contains(validProviders, billing.Provider) {
		return fmt.Errorf("unsupported billing provider: %s", billing.Provider)
	}

	// Validate provider-specific settings
	switch billing.Provider {
	case "stripe":
		if billing.StripeSecretKey == "" {
			return fmt.Errorf("stripe_secret_key is required when using Stripe")
		}
	case "paypal":
		if billing.PayPalClientID == "" {
			return fmt.Errorf("paypal_client_id is required when using PayPal")
		}
		if billing.PayPalClientSecret == "" {
			return fmt.Errorf("paypal_client_secret is required when using PayPal")
		}
	}

	// Validate numeric values
	if billing.GracePeriodDays < 0 {
		return fmt.Errorf("grace_period_days cannot be negative")
	}
	if billing.TrialPeriodDays < 0 {
		return fmt.Errorf("trial_period_days cannot be negative")
	}
	if billing.UsageWarningThreshold < 0 || billing.UsageWarningThreshold > 100 {
		return fmt.Errorf("usage_warning_threshold must be between 0 and 100")
	}

	return nil
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}