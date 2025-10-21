package config

import (
	"github.com/spf13/viper"
)

// setDefaults sets all default configuration values
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 0) // 0 means auto-select
	v.SetDefault("server.data_dir", "/var/lib/caslink")
	v.SetDefault("server.base_url", "auto")
	v.SetDefault("server.behind_proxy", "auto")
	v.SetDefault("server.trusted_proxies", []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	v.SetDefault("server.real_ip_header", "auto")
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "120s")
	v.SetDefault("server.max_header_bytes", 1048576) // 1MB

	// Database defaults
	v.SetDefault("database.url", "sqlite:///var/lib/caslink/caslink.db")
	v.SetDefault("database.type", "sqlite")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 0) // 0 means auto-detect based on type
	v.SetDefault("database.name", "caslink")
	v.SetDefault("database.username", "caslink")
	v.SetDefault("database.password", "")
	v.SetDefault("database.ssl_mode", "auto")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", "300s")
	v.SetDefault("database.auto_migrate", true)
	v.SetDefault("database.migration_timeout", "300s")
	v.SetDefault("database.slow_query_threshold", "1s")
	v.SetDefault("database.log_queries", false)
	v.SetDefault("database.sqlite_wal", true)
	v.SetDefault("database.sqlite_cache_size", "64MB")
	v.SetDefault("database.sqlite_busy_timeout", "30s")

	// Application defaults
	v.SetDefault("application.brand_name", "Casjay URL Shortener")
	v.SetDefault("application.base_url", "auto")
	v.SetDefault("application.enable_registration", true)
	v.SetDefault("application.enable_anonymous_urls", true)
	v.SetDefault("application.require_login_for_urls", false)
	v.SetDefault("application.default_theme", "dark")
	v.SetDefault("application.custom_css_url", "")
	v.SetDefault("application.favicon_url", "")

	// URL defaults
	v.SetDefault("url.min_random_length", 6)
	v.SetDefault("url.max_random_length", 8)
	v.SetDefault("url.custom_code_min_length", 3)
	v.SetDefault("url.custom_code_max_length", 50)
	v.SetDefault("url.allowed_characters", "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	v.SetDefault("url.exclude_similar_chars", true)
	v.SetDefault("url.reserved_words", []string{"api", "admin", "www", "app", "help", "about", "setup", "login", "register", "dashboard"})
	v.SetDefault("url.max_url_length", 2048)
	v.SetDefault("url.default_expiration", "never")

	// Analytics defaults
	v.SetDefault("analytics.enabled", true)
	v.SetDefault("analytics.enable_geolocation", true)
	v.SetDefault("analytics.geoip_database_path", "/var/lib/caslink/GeoLite2-City.mmdb")
	v.SetDefault("analytics.retention_days", 365)
	v.SetDefault("analytics.anonymize_ips", true)
	v.SetDefault("analytics.track_bots", false)
	v.SetDefault("analytics.real_time_analytics", true)
	v.SetDefault("analytics.export_formats", []string{"csv", "json", "pdf"})

	// QR defaults
	v.SetDefault("qr.enabled", true)
	v.SetDefault("qr.default_size", 200)
	v.SetDefault("qr.max_size", 1000)
	v.SetDefault("qr.supported_formats", []string{"png", "svg", "pdf"})
	v.SetDefault("qr.default_style", "square")
	v.SetDefault("qr.allow_custom_colors", true)
	v.SetDefault("qr.allow_logos", true)
	v.SetDefault("qr.max_logo_size", 50)

	// Bulk defaults
	v.SetDefault("bulk.enabled", true)
	v.SetDefault("bulk.max_import_size", 10000)
	v.SetDefault("bulk.supported_formats", []string{"csv", "json"})
	v.SetDefault("bulk.timeout", "600s")

	// Auth defaults
	v.SetDefault("auth.session_secret", "auto")
	v.SetDefault("auth.session_timeout", "24h")
	v.SetDefault("auth.session_secure", "auto")
	v.SetDefault("auth.password_min_length", 8)
	v.SetDefault("auth.password_require_special", false)
	v.SetDefault("auth.api_token_length", 32)
	v.SetDefault("auth.api_token_expiration", "never")
	v.SetDefault("auth.enable_totp", true)
	v.SetDefault("auth.totp_issuer_name", "Caslink")
	v.SetDefault("auth.enable_webauthn", true)
	v.SetDefault("auth.webauthn_display_name", "Caslink URL Shortener")
	v.SetDefault("auth.webauthn_id", "auto")

	// OAuth defaults
	v.SetDefault("oauth.enabled", false)
	v.SetDefault("oauth.provider", "generic")
	v.SetDefault("oauth.client_id", "")
	v.SetDefault("oauth.client_secret", "")
	v.SetDefault("oauth.redirect_url", "auto")
	v.SetDefault("oauth.authorize_url", "")
	v.SetDefault("oauth.token_url", "")
	v.SetDefault("oauth.userinfo_url", "")
	v.SetDefault("oauth.scopes", []string{"openid", "profile", "email"})

	// Domains defaults
	v.SetDefault("domains.enabled", true)
	v.SetDefault("domains.verification_method", "dns")
	v.SetDefault("domains.ssl_auto_provision", false)
	v.SetDefault("domains.max_domains_per_user", "unlimited")

	// Federation defaults
	v.SetDefault("federation.enabled", true)
	v.SetDefault("federation.domain", "auto")
	v.SetDefault("federation.private_key_path", "/var/lib/caslink/federation.key")
	v.SetDefault("federation.public_key_path", "/var/lib/caslink/federation.pub")
	v.SetDefault("federation.sync_interval", "1h")
	v.SetDefault("federation.share_public_urls", true)
	v.SetDefault("federation.max_urls_per_sync", 100)
	v.SetDefault("federation.sync_timeout", "30s")
	v.SetDefault("federation.discovery_timeout", "10s")
	v.SetDefault("federation.retry_attempts", 3)
	v.SetDefault("federation.retry_backoff", "1s")

	// Webhooks defaults
	v.SetDefault("webhooks.enabled", true)
	v.SetDefault("webhooks.timeout", "10s")
	v.SetDefault("webhooks.retry_attempts", 3)
	v.SetDefault("webhooks.retry_backoff", "exponential")
	v.SetDefault("webhooks.max_payload_size", 1048576) // 1MB

	// Billing defaults
	v.SetDefault("billing.enabled", false)
	v.SetDefault("billing.provider", "none")
	v.SetDefault("billing.stripe_secret_key", "")
	v.SetDefault("billing.stripe_webhook_secret", "")
	v.SetDefault("billing.paypal_client_id", "")
	v.SetDefault("billing.paypal_client_secret", "")
	v.SetDefault("billing.currency", "USD")
	v.SetDefault("billing.grace_period_days", 7)
	v.SetDefault("billing.trial_period_days", 14)
	v.SetDefault("billing.require_payment", false)
	v.SetDefault("billing.enforce_limits", false)
	v.SetDefault("billing.usage_warning_threshold", 80)
	v.SetDefault("billing.auto_suspend", false)

	// Rate limit defaults
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.requests_per_minute", 60)
	v.SetDefault("rate_limit.burst", 10)
	v.SetDefault("rate_limit.cleanup_interval", "60s")

	// Security defaults
	v.SetDefault("security.enable_https_redirect", "auto")
	v.SetDefault("security.hsts_max_age", 31536000)
	v.SetDefault("security.csp_policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
	v.SetDefault("security.allowed_origins", []string{"*"})
	v.SetDefault("security.malware_detection_enabled", false)
	v.SetDefault("security.malware_api_key", "")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.file", "")
	v.SetDefault("logging.max_size", "100MB")
	v.SetDefault("logging.max_backups", 5)
	v.SetDefault("logging.max_age", 30)
	v.SetDefault("logging.compress", true)

	// Monitoring defaults
	v.SetDefault("monitoring.enable_metrics", true)
	v.SetDefault("monitoring.metrics_path", "/metrics")
	v.SetDefault("monitoring.enable_health_check", true)
	v.SetDefault("monitoring.health_check_path", "/health")

	// Email defaults
	v.SetDefault("email.enabled", false)
	v.SetDefault("email.provider", "smtp")
	v.SetDefault("email.from_address", "noreply@localhost")
	v.SetDefault("email.from_name", "Caslink")
	v.SetDefault("email.smtp_host", "")
	v.SetDefault("email.smtp_port", 587)
	v.SetDefault("email.smtp_username", "")
	v.SetDefault("email.smtp_password", "")
	v.SetDefault("email.smtp_tls", true)
	v.SetDefault("email.sendgrid_api_key", "")
	v.SetDefault("email.ses_access_key", "")
	v.SetDefault("email.ses_secret_key", "")
	v.SetDefault("email.ses_region", "us-east-1")

	// Notification defaults
	v.SetDefault("notifications.enabled", true)
	v.SetDefault("notifications.default_channel", "email")
	v.SetDefault("notifications.batch_size", 100)
	v.SetDefault("notifications.processing_interval", "5m")
	v.SetDefault("notifications.retry_attempts", 3)
	v.SetDefault("notifications.retry_delay", "30s")
	v.SetDefault("notifications.max_retry_delay", "1h")
	v.SetDefault("notifications.retention_days", 30)
	v.SetDefault("notifications.enable_sms", false)
	v.SetDefault("notifications.enable_push", false)
	v.SetDefault("notifications.enable_webhook", false)
	v.SetDefault("notifications.sms_provider", "twilio")
	v.SetDefault("notifications.push_provider", "fcm")
	v.SetDefault("notifications.twilio_account_sid", "")
	v.SetDefault("notifications.twilio_auth_token", "")
	v.SetDefault("notifications.twilio_from_number", "")
	v.SetDefault("notifications.fcm_server_key", "")
	v.SetDefault("notifications.apns_cert_path", "")
	v.SetDefault("notifications.apns_key_path", "")
	v.SetDefault("notifications.apns_bundle_id", "")
	v.SetDefault("notifications.apns_production", false)

	// Cache defaults
	v.SetDefault("cache.enabled", true)
	v.SetDefault("cache.ttl", "3600s")
	v.SetDefault("cache.cleanup_interval", "600s")
	v.SetDefault("cache.redis_enabled", false)
	v.SetDefault("cache.redis_url", "")
}