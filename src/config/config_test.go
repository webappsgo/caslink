package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestLoadCreatesDefaultConfigWhenMissing covers the happy path of Load()
// against a directory with no server.yml: it must write server.yml (never
// server.yaml), populate an encryption key, and return a config that passes
// validation.
func TestLoadCreatesDefaultConfigWhenMissing(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil config with nil error")
	}

	// Config file must be named server.yml, never server.yaml.
	if _, err := os.Stat(filepath.Join(dir, "server.yml")); err != nil {
		t.Errorf("expected server.yml to be created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "server.yaml")); err == nil {
		t.Error("server.yaml must never be created")
	}

	if cfg.Server.Security.EncryptionKey == "" {
		t.Error("expected encryption key to be generated on first run")
	}
	if cfg.Server.Security.EncryptionKeyVersion != 1 {
		t.Errorf("expected encryption_key_version 1, got %d", cfg.Server.Security.EncryptionKeyVersion)
	}
	if cfg.Server.Mode != "production" {
		t.Errorf("expected default mode production, got %q", cfg.Server.Mode)
	}
}

// TestLoadTwiceIsIdempotent verifies loading the same directory twice
// produces configs with the same persisted encryption key (i.e. the second
// Load reads the file written by the first, rather than regenerating it).
func TestLoadTwiceIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	first, err := Load(dir)
	if err != nil {
		t.Fatalf("first Load: %v", err)
	}
	second, err := Load(dir)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}

	if first.Server.Security.EncryptionKey != second.Server.Security.EncryptionKey {
		t.Error("expected encryption key to persist across repeated Load calls")
	}
	if first.Server.Branding.Title != second.Server.Branding.Title {
		t.Error("expected identical branding across repeated Load calls")
	}
}

// TestSaveLoadRoundTrip covers Save() writing a customized config and Load()
// reading it back with the same values (round-trip idempotency).
func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()

	cfg := DefaultConfig()
	cfg.Server.FQDN = "example.test"
	cfg.Server.Port = 8443
	cfg.Server.Branding.Title = "My Caslink"
	if err := ensureEncryptionKey(cfg); err != nil {
		t.Fatalf("ensureEncryptionKey: %v", err)
	}

	if err := Save(dir, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Server.FQDN != "example.test" {
		t.Errorf("FQDN = %q, want %q", loaded.Server.FQDN, "example.test")
	}
	if loaded.Server.Port != 8443 {
		t.Errorf("Port = %d, want %d", loaded.Server.Port, 8443)
	}
	if loaded.Server.Branding.Title != "My Caslink" {
		t.Errorf("Branding.Title = %q, want %q", loaded.Server.Branding.Title, "My Caslink")
	}
	if loaded.Server.Security.EncryptionKey != cfg.Server.Security.EncryptionKey {
		t.Error("expected encryption key to round-trip unchanged")
	}
}

// TestSaveCreatesParentDirectory covers Save() being called against a
// directory tree that does not yet exist.
func TestSaveCreatesParentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "config", "dir")

	cfg := DefaultConfig()
	if err := Save(dir, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "server.yml")); err != nil {
		t.Errorf("expected server.yml under created directory: %v", err)
	}
}

// TestSaveFilePerms ensures server.yml is written with restrictive
// permissions (0600) since it can contain SMTP passwords and DB credentials.
func TestSaveFilePerms(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	if err := Save(dir, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "server.yml"))
	if err != nil {
		t.Fatalf("stat server.yml: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("server.yml perms = %o, want %o", perm, 0600)
	}
}

// TestLoadMalformedYAMLReturnsError covers a corrupt server.yml producing a
// clear error rather than a panic or a silently substituted default config.
func TestLoadMalformedYAMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(path, []byte("server:\n  port: [this is not valid: yaml"), 0600); err != nil {
		t.Fatalf("write malformed yaml: %v", err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatal("expected error loading malformed YAML, got nil")
	}
}

// TestLoadMissingFieldsUseZeroValues covers a server.yml with only a subset
// of fields present: unset fields must unmarshal to their Go zero value and
// then be corrected by Validate() where applicable (e.g. mode, driver), and
// left as zero otherwise (fields Validate doesn't police).
func TestLoadMissingFieldsUseZeroValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	minimal := "server:\n  fqdn: partial.example\n"
	if err := os.WriteFile(path, []byte(minimal), 0600); err != nil {
		t.Fatalf("write minimal yaml: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.FQDN != "partial.example" {
		t.Errorf("FQDN = %q, want %q", cfg.Server.FQDN, "partial.example")
	}
	// Mode was omitted (zero value "") -- Validate must substitute production.
	if cfg.Server.Mode != "production" {
		t.Errorf("Mode = %q, want %q (defaulted by Validate)", cfg.Server.Mode, "production")
	}
	// Database driver omitted -- Validate must substitute sqlite.
	if cfg.Server.Database.Driver != "sqlite" {
		t.Errorf("Database.Driver = %q, want %q (defaulted by Validate)", cfg.Server.Database.Driver, "sqlite")
	}
	// An encryption key must still be generated and persisted for an
	// upgrade-path config that predates the field.
	if cfg.Server.Security.EncryptionKey == "" {
		t.Error("expected encryption key to be generated for a config missing it")
	}
}

// TestLoadPreservesExistingEncryptionKey ensures Load() does not regenerate
// (and thus invalidate any data encrypted with) an already-present
// encryption key.
func TestLoadPreservesExistingEncryptionKey(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	if err := ensureEncryptionKey(cfg); err != nil {
		t.Fatalf("ensureEncryptionKey: %v", err)
	}
	original := cfg.Server.Security.EncryptionKey
	cfg.Server.Security.EncryptionKeyVersion = 3
	if err := Save(dir, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Server.Security.EncryptionKey != original {
		t.Error("expected existing encryption key to be preserved, not regenerated")
	}
	if loaded.Server.Security.EncryptionKeyVersion != 3 {
		t.Errorf("EncryptionKeyVersion = %d, want 3 (must not be reset)", loaded.Server.Security.EncryptionKeyVersion)
	}
}

// TestDefaultConfigPassesValidate ensures DefaultConfig() never itself
// requires any correction from Validate() -- if this ever fails, either
// DefaultConfig or Validate drifted out of sync.
func TestDefaultConfigPassesValidate(t *testing.T) {
	cfg := DefaultConfig()
	before, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	after, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal validated config: %v", err)
	}
	if string(before) != string(after) {
		t.Error("Validate() mutated a fresh DefaultConfig() -- defaults and validation are out of sync")
	}
}

// TestValidateSubstitutesDefaultsOnInvalidValues is the core table-driven
// coverage of Validate()'s "warn and replace with default, never fail"
// contract for every field it polices.
func TestValidateSubstitutesDefaultsOnInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		check   func(*Config) bool
		explain string
	}{
		{
			name:    "unknown mode falls back to production",
			mutate:  func(c *Config) { c.Server.Mode = "banana" },
			check:   func(c *Config) bool { return c.Server.Mode == "production" },
			explain: "Mode should default to production",
		},
		{
			name:    "unknown database driver falls back to sqlite",
			mutate:  func(c *Config) { c.Server.Database.Driver = "banana" },
			check:   func(c *Config) bool { return c.Server.Database.Driver == "sqlite" },
			explain: "Database.Driver should default to sqlite",
		},
		{
			name:    "negative port resets to 0 (auto-select)",
			mutate:  func(c *Config) { c.Server.Port = -1 },
			check:   func(c *Config) bool { return c.Server.Port == 0 },
			explain: "Port should reset to 0",
		},
		{
			name:    "port above 65535 resets to 0",
			mutate:  func(c *Config) { c.Server.Port = 70000 },
			check:   func(c *Config) bool { return c.Server.Port == 0 },
			explain: "Port should reset to 0",
		},
		{
			name: "rate limit requests <= 0 while enabled resets to 120",
			mutate: func(c *Config) {
				c.Server.RateLimit.Enabled = true
				c.Server.RateLimit.Requests = 0
			},
			check:   func(c *Config) bool { return c.Server.RateLimit.Requests == 120 },
			explain: "RateLimit.Requests should reset to 120",
		},
		{
			name: "rate limit window <= 0 while enabled resets to 60",
			mutate: func(c *Config) {
				c.Server.RateLimit.Enabled = true
				c.Server.RateLimit.Window = -5
			},
			check:   func(c *Config) bool { return c.Server.RateLimit.Window == 60 },
			explain: "RateLimit.Window should reset to 60",
		},
		{
			name:    "negative burst resets to 10",
			mutate:  func(c *Config) { c.Server.RateLimit.Burst = -1 },
			check:   func(c *Config) bool { return c.Server.RateLimit.Burst == 10 },
			explain: "RateLimit.Burst should reset to 10",
		},
		{
			name:    "negative login max attempts resets to 5",
			mutate:  func(c *Config) { c.Server.RateLimit.LoginMaxAttempts = -1 },
			check:   func(c *Config) bool { return c.Server.RateLimit.LoginMaxAttempts == 5 },
			explain: "RateLimit.LoginMaxAttempts should reset to 5",
		},
		{
			name:    "negative password reset max attempts resets to 3",
			mutate:  func(c *Config) { c.Server.RateLimit.PasswordResetMaxAttempts = -1 },
			check:   func(c *Config) bool { return c.Server.RateLimit.PasswordResetMaxAttempts == 3 },
			explain: "RateLimit.PasswordResetMaxAttempts should reset to 3",
		},
		{
			name:    "empty session timeout resets to 24h",
			mutate:  func(c *Config) { c.Server.Session.Timeout = "" },
			check:   func(c *Config) bool { return c.Server.Session.Timeout == "24h" },
			explain: "Session.Timeout should reset to 24h",
		},
		{
			name:    "empty remember-me timeout resets to 720h",
			mutate:  func(c *Config) { c.Server.Session.RememberMeTimeout = "" },
			check:   func(c *Config) bool { return c.Server.Session.RememberMeTimeout == "720h" },
			explain: "Session.RememberMeTimeout should reset to 720h",
		},
		{
			name:    "empty admin path resets to admin",
			mutate:  func(c *Config) { c.Server.Admin.Path = "" },
			check:   func(c *Config) bool { return c.Server.Admin.Path == "admin" },
			explain: "Admin.Path should reset to admin",
		},
		{
			name:    "unknown same_site resets to lax",
			mutate:  func(c *Config) { c.Server.Session.SameSite = "banana" },
			check:   func(c *Config) bool { return c.Server.Session.SameSite == "lax" },
			explain: "Session.SameSite should reset to lax",
		},
		{
			name:    "unknown secure resets to auto",
			mutate:  func(c *Config) { c.Server.Session.Secure = "banana" },
			check:   func(c *Config) bool { return c.Server.Session.Secure == "auto" },
			explain: "Session.Secure should reset to auto",
		},
		{
			name:    "unknown ssl min_version resets to TLS1.2",
			mutate:  func(c *Config) { c.Server.SSL.MinVersion = "SSLv3" },
			check:   func(c *Config) bool { return c.Server.SSL.MinVersion == "TLS1.2" },
			explain: "SSL.MinVersion should reset to TLS1.2",
		},
		{
			name: "invalid smtp port with host set resets to 587",
			mutate: func(c *Config) {
				c.Server.Notifications.Email.SMTP.Host = "smtp.example.test"
				c.Server.Notifications.Email.SMTP.Port = 0
			},
			check:   func(c *Config) bool { return c.Server.Notifications.Email.SMTP.Port == 587 },
			explain: "SMTP.Port should reset to 587",
		},
		{
			name: "smtp port out of range resets to 587",
			mutate: func(c *Config) {
				c.Server.Notifications.Email.SMTP.Host = "smtp.example.test"
				c.Server.Notifications.Email.SMTP.Port = 70000
			},
			check:   func(c *Config) bool { return c.Server.Notifications.Email.SMTP.Port == 587 },
			explain: "SMTP.Port should reset to 587",
		},
		{
			name:    "min_random_length below 3 resets to 6",
			mutate:  func(c *Config) { c.Caslink.URL.MinRandomLength = 1 },
			check:   func(c *Config) bool { return c.Caslink.URL.MinRandomLength == 6 },
			explain: "URL.MinRandomLength should reset to 6",
		},
		{
			name: "max_custom_length below min_random_length resets to 50",
			mutate: func(c *Config) {
				c.Caslink.URL.MinRandomLength = 10
				c.Caslink.URL.MaxCustomLength = 5
			},
			check:   func(c *Config) bool { return c.Caslink.URL.MaxCustomLength == 50 },
			explain: "URL.MaxCustomLength should reset to 50",
		},
		{
			name:    "zero retention days resets to 365",
			mutate:  func(c *Config) { c.Caslink.Analytics.RetentionDays = 0 },
			check:   func(c *Config) bool { return c.Caslink.Analytics.RetentionDays == 365 },
			explain: "Analytics.RetentionDays should reset to 365",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(cfg)
			if err := Validate(cfg); err != nil {
				t.Fatalf("Validate returned error (must never fail startup): %v", err)
			}
			if !tt.check(cfg) {
				t.Errorf("%s", tt.explain)
			}
		})
	}
}

// TestValidateNeverErrors documents that Validate always substitutes
// defaults rather than returning an error, even when nearly every field is
// simultaneously invalid.
func TestValidateNeverErrors(t *testing.T) {
	cfg := &Config{}
	cfg.Server.Port = -100
	cfg.Server.Mode = "nonsense"
	cfg.Server.RateLimit.Enabled = true
	cfg.Caslink.Analytics.RetentionDays = -1 // negative is treated as "unlimited", not invalid
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate must never return an error, got: %v", err)
	}
}

// TestValidateNegativeRetentionDaysMeansUnlimited covers the documented
// -1-is-unlimited boundary distinct from the 0-is-invalid case.
func TestValidateNegativeRetentionDaysMeansUnlimited(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Caslink.Analytics.RetentionDays = -1
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.Caslink.Analytics.RetentionDays != -1 {
		t.Errorf("RetentionDays = %d, want -1 preserved as unlimited", cfg.Caslink.Analytics.RetentionDays)
	}
}

// TestValidateValidSameSiteValuesAccepted ensures the three legal
// same_site values (plus empty) pass through unchanged rather than being
// clobbered by the fallback branch.
func TestValidateValidSameSiteValuesAccepted(t *testing.T) {
	for _, v := range []string{"strict", "lax", "none"} {
		cfg := DefaultConfig()
		cfg.Server.Session.SameSite = v
		if err := Validate(cfg); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		if cfg.Server.Session.SameSite != v {
			t.Errorf("SameSite = %q, want unchanged %q", cfg.Server.Session.SameSite, v)
		}
	}
}

// TestValidateSMTPPortIgnoredWhenHostEmpty ensures an invalid SMTP port is
// left untouched when no host is configured (email unconfigured entirely).
func TestValidateSMTPPortIgnoredWhenHostEmpty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Notifications.Email.SMTP.Host = ""
	cfg.Server.Notifications.Email.SMTP.Port = -1
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.Server.Notifications.Email.SMTP.Port != -1 {
		t.Errorf("SMTP.Port = %d, want untouched -1 when host is empty", cfg.Server.Notifications.Email.SMTP.Port)
	}
}

// TestApplyEnvOverridesPrefixedWinsOverBare verifies the documented
// CASLINK_-prefix-first, bare-fallback precedence in envStr, exercised via
// applyEnvOverrides against the FQDN field (checked through both DOMAIN and
// the bare/prefixed forms).
func TestApplyEnvOverridesPrefixedWinsOverBare(t *testing.T) {
	t.Setenv("CASLINK_DOMAIN", "prefixed.example")
	t.Setenv("DOMAIN", "bare.example")

	cfg := DefaultConfig()
	applyEnvOverrides(cfg)

	if cfg.Server.FQDN != "prefixed.example" {
		t.Errorf("FQDN = %q, want %q (CASLINK_ prefix must win)", cfg.Server.FQDN, "prefixed.example")
	}
}

// TestApplyEnvOverridesBareFallback verifies the bare env var name is used
// when no CASLINK_-prefixed variant is set.
func TestApplyEnvOverridesBareFallback(t *testing.T) {
	t.Setenv("DOMAIN", "bare-only.example")

	cfg := DefaultConfig()
	applyEnvOverrides(cfg)

	if cfg.Server.FQDN != "bare-only.example" {
		t.Errorf("FQDN = %q, want %q (bare fallback)", cfg.Server.FQDN, "bare-only.example")
	}
}

// TestApplyEnvOverridesBooleans exercises every boolean env override,
// confirming they all go through ParseBool (accepting "yes"/"no" style
// values, not just "true"/"false").
func TestApplyEnvOverridesBooleans(t *testing.T) {
	t.Setenv("CASLINK_SSL_ENABLED", "yes")
	t.Setenv("CASLINK_LE_ENABLED", "on")
	t.Setenv("CASLINK_LE_STAGING", "enable")
	t.Setenv("CASLINK_RATE_LIMIT_ENABLED", "no")
	t.Setenv("CASLINK_METRICS_ENABLED", "off")
	t.Setenv("CASLINK_GEOIP_ENABLED", "disable")
	t.Setenv("CASLINK_EMAIL_ENABLED", "1")
	t.Setenv("CASLINK_ANALYTICS_ENABLED", "0")
	t.Setenv("CASLINK_ANONYMIZE_IPS", "true")

	cfg := DefaultConfig()
	applyEnvOverrides(cfg)

	if !cfg.Server.SSL.Enabled {
		t.Error("SSL.Enabled should be true from CASLINK_SSL_ENABLED=yes")
	}
	if !cfg.Server.SSL.LetsEncrypt.Enabled {
		t.Error("LetsEncrypt.Enabled should be true from CASLINK_LE_ENABLED=on")
	}
	if !cfg.Server.SSL.LetsEncrypt.Staging {
		t.Error("LetsEncrypt.Staging should be true from CASLINK_LE_STAGING=enable")
	}
	if cfg.Server.RateLimit.Enabled {
		t.Error("RateLimit.Enabled should be false from CASLINK_RATE_LIMIT_ENABLED=no")
	}
	if cfg.Server.Metrics.Enabled {
		t.Error("Metrics.Enabled should be false from CASLINK_METRICS_ENABLED=off")
	}
	if cfg.Server.GeoIP.Enabled {
		t.Error("GeoIP.Enabled should be false from CASLINK_GEOIP_ENABLED=disable")
	}
	if !cfg.Server.Notifications.Email.Enabled {
		t.Error("Email.Enabled should be true from CASLINK_EMAIL_ENABLED=1")
	}
	if cfg.Caslink.Analytics.Enabled {
		t.Error("Analytics.Enabled should be false from CASLINK_ANALYTICS_ENABLED=0")
	}
	if !cfg.Caslink.Analytics.AnonymizeIPs {
		t.Error("Analytics.AnonymizeIPs should be true from CASLINK_ANONYMIZE_IPS=true")
	}
}

// TestApplyEnvOverridesIntegers exercises numeric env overrides, including
// the "0 or invalid means unset, keep existing value" guard every call site
// applies around parseInt.
func TestApplyEnvOverridesIntegers(t *testing.T) {
	t.Setenv("CASLINK_PORT", "9443")
	t.Setenv("CASLINK_SMTP_PORT", "2525")
	t.Setenv("CASLINK_MIN_RANDOM_LENGTH", "8")

	cfg := DefaultConfig()
	applyEnvOverrides(cfg)

	if cfg.Server.Port != 9443 {
		t.Errorf("Port = %d, want 9443", cfg.Server.Port)
	}
	if cfg.Server.Notifications.Email.SMTP.Port != 2525 {
		t.Errorf("SMTP.Port = %d, want 2525", cfg.Server.Notifications.Email.SMTP.Port)
	}
	if cfg.Caslink.URL.MinRandomLength != 8 {
		t.Errorf("MinRandomLength = %d, want 8", cfg.Caslink.URL.MinRandomLength)
	}
}

// TestApplyEnvOverridesInvalidIntegerIgnored covers a non-numeric env var
// value: parseInt returns 0, which every call site treats as "leave the
// existing config value alone" rather than corrupting it to zero.
func TestApplyEnvOverridesInvalidIntegerIgnored(t *testing.T) {
	t.Setenv("CASLINK_PORT", "not-a-number")

	cfg := DefaultConfig()
	cfg.Server.Port = 12345
	applyEnvOverrides(cfg)

	if cfg.Server.Port != 12345 {
		t.Errorf("Port = %d, want unchanged 12345 (invalid env value must be ignored)", cfg.Server.Port)
	}
}

// TestApplyEnvOverridesEmptyLeavesConfigUnchanged covers the no-env-vars-set
// case: applyEnvOverrides must be a no-op.
func TestApplyEnvOverridesEmptyLeavesConfigUnchanged(t *testing.T) {
	cfg := DefaultConfig()
	before, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	applyEnvOverrides(cfg)

	after, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(before) != string(after) {
		t.Error("applyEnvOverrides mutated config with no relevant env vars set")
	}
}

// TestLoadAppliesEnvOverridesAndRevalidates verifies Load() applies env
// overrides after reading the file and re-validates so an out-of-range env
// value is clamped rather than accepted verbatim.
func TestLoadAppliesEnvOverridesAndRevalidates(t *testing.T) {
	dir := t.TempDir()
	// First Load creates the on-disk default config.
	if _, err := Load(dir); err != nil {
		t.Fatalf("initial Load: %v", err)
	}

	// An out-of-range port from the environment must be clamped by the
	// post-override Validate() call, not passed through as-is.
	t.Setenv("CASLINK_PORT", "999999")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load with env override: %v", err)
	}
	if cfg.Server.Port != 0 {
		t.Errorf("Port = %d, want 0 (out-of-range env value must be clamped by Validate)", cfg.Server.Port)
	}
}

// TestLoadEnvOverrideAppliesValidValue is the positive counterpart: a
// legitimate env override must actually take effect on Load, proving env
// truly wins over the on-disk file per the documented precedence.
func TestLoadEnvOverrideAppliesValidValue(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err != nil {
		t.Fatalf("initial Load: %v", err)
	}

	t.Setenv("CASLINK_DOMAIN", "env-override.example")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load with env override: %v", err)
	}
	if cfg.Server.FQDN != "env-override.example" {
		t.Errorf("FQDN = %q, want %q", cfg.Server.FQDN, "env-override.example")
	}

	// The override must not be persisted back to disk -- reloading with the
	// env var cleared should return the original on-disk value.
	os.Unsetenv("CASLINK_DOMAIN")
	reloaded, err := Load(dir)
	if err != nil {
		t.Fatalf("reload without env override: %v", err)
	}
	if reloaded.Server.FQDN == "env-override.example" {
		t.Error("env override must not be persisted to server.yml")
	}
}

// TestEnvStrTrimsWhitespace covers envStr's whitespace-trimming behavior
// directly.
func TestEnvStrTrimsWhitespace(t *testing.T) {
	t.Setenv("CASLINK_FQDN", "  padded.example  ")
	if got := envStr("FQDN"); got != "padded.example" {
		t.Errorf("envStr(FQDN) = %q, want %q", got, "padded.example")
	}
}

// TestEnvStrUnsetReturnsEmpty covers envStr when neither the prefixed nor
// bare variant is set.
func TestEnvStrUnsetReturnsEmpty(t *testing.T) {
	os.Unsetenv("CASLINK_TOTALLY_UNSET_KEY")
	os.Unsetenv("TOTALLY_UNSET_KEY")
	if got := envStr("TOTALLY_UNSET_KEY"); got != "" {
		t.Errorf("envStr(TOTALLY_UNSET_KEY) = %q, want empty", got)
	}
}

// TestParseIntValid covers parseInt on well-formed non-negative integers of
// various magnitudes.
func TestParseIntValid(t *testing.T) {
	tests := map[string]int{
		"0":     0,
		"5":     5,
		"120":   120,
		"65535": 65535,
	}
	for in, want := range tests {
		if got := parseInt(in); got != want {
			t.Errorf("parseInt(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestParseIntInvalid covers parseInt's documented behavior of returning 0
// for empty input or any non-digit character (including negative signs,
// decimals, and letters).
func TestParseIntInvalid(t *testing.T) {
	tests := []string{"", "abc", "-5", "1.5", "12a", "a12", " 5", "5 "}
	for _, in := range tests {
		if got := parseInt(in); got != 0 {
			t.Errorf("parseInt(%q) = %d, want 0", in, got)
		}
	}
}

// TestEnsureEncryptionKeyIsNoOpWhenAlreadySet ensures ensureEncryptionKey
// never regenerates an already-present key.
func TestEnsureEncryptionKeyIsNoOpWhenAlreadySet(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Security.EncryptionKey = "already-set-value"
	cfg.Server.Security.EncryptionKeyVersion = 7

	if err := ensureEncryptionKey(cfg); err != nil {
		t.Fatalf("ensureEncryptionKey: %v", err)
	}
	if cfg.Server.Security.EncryptionKey != "already-set-value" {
		t.Error("ensureEncryptionKey must not overwrite an existing key")
	}
	if cfg.Server.Security.EncryptionKeyVersion != 7 {
		t.Error("ensureEncryptionKey must not touch an existing key's version")
	}
}

// TestEnsureEncryptionKeyGeneratesFreshKey covers the empty-key path,
// including that the version is initialized to 1 and two independently
// generated keys are not equal (basic randomness sanity check).
func TestEnsureEncryptionKeyGeneratesFreshKey(t *testing.T) {
	cfgA := &Config{}
	if err := ensureEncryptionKey(cfgA); err != nil {
		t.Fatalf("ensureEncryptionKey: %v", err)
	}
	if cfgA.Server.Security.EncryptionKey == "" {
		t.Fatal("expected a non-empty generated key")
	}
	if cfgA.Server.Security.EncryptionKeyVersion != 1 {
		t.Errorf("EncryptionKeyVersion = %d, want 1", cfgA.Server.Security.EncryptionKeyVersion)
	}

	cfgB := &Config{}
	if err := ensureEncryptionKey(cfgB); err != nil {
		t.Fatalf("ensureEncryptionKey: %v", err)
	}
	if cfgA.Server.Security.EncryptionKey == cfgB.Server.Security.EncryptionKey {
		t.Error("two independently generated encryption keys must not collide")
	}
}

// TestDefaultConfigHostnameFallback ensures DefaultConfig never leaves FQDN
// empty even in an environment where os.Hostname() fails or returns "" --
// simulated here by simply asserting the invariant holds for the real
// environment, since os.Hostname cannot be faked without refactoring
// production code.
func TestDefaultConfigHostnameFallback(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.FQDN == "" {
		t.Error("DefaultConfig must never leave FQDN empty")
	}
}

// TestLoadRejectsConfigDirThatIsAFile covers Load() being pointed at a
// config directory path that is itself a file (not a directory), which
// must surface as an error rather than panicking.
func TestLoadRejectsConfigDirThatIsAFile(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	// configDir itself is a regular file, so server.yml can never be created
	// under it.
	if _, err := Load(blocker); err == nil {
		t.Fatal("expected an error when configDir is a regular file, got nil")
	}
}

func TestRegistrationModeGating(t *testing.T) {
	cases := []struct {
		name        string
		enabled     bool
		mode        string
		wantNorm    string
		wantAllowed bool
	}{
		{"open default", true, "open", "open", true},
		{"empty defaults to open", true, "", "open", true},
		{"unknown defaults to open", true, "bogus", "open", true},
		{"mixed case open", true, "OpEn", "open", true},
		{"invite blocks public", true, "invite", "invite", false},
		{"admin_only blocks public", true, "admin_only", "admin_only", false},
		{"disabled blocks public", true, "disabled", "disabled", false},
		{"registration disabled blocks even open", false, "open", "open", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := RegistrationConfig{Enabled: tc.enabled, Mode: tc.mode}
			if got := r.NormalizedMode(); got != tc.wantNorm {
				t.Errorf("NormalizedMode() = %q, want %q", got, tc.wantNorm)
			}
			if got := r.PublicSelfRegistrationAllowed(); got != tc.wantAllowed {
				t.Errorf("PublicSelfRegistrationAllowed() = %v, want %v", got, tc.wantAllowed)
			}
		})
	}
}

func TestOrgCreationModeGating(t *testing.T) {
	cases := []struct {
		name        string
		enabled     bool
		allow       bool
		mode        string
		wantNorm    string
		wantAllowed bool
	}{
		{"open default", true, true, "open", "open", true},
		{"empty defaults to open", true, true, "", "open", true},
		{"unknown defaults to open", true, true, "bogus", "open", true},
		{"invite blocks self-service", true, true, "invite", "invite", false},
		{"admin_only blocks self-service", true, true, "admin_only", "admin_only", false},
		{"disabled blocks self-service", true, true, "disabled", "disabled", false},
		{"allow_creation off blocks open", true, false, "open", "open", false},
		{"feature disabled blocks open", false, true, "open", "open", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := OrganizationsConfig{
				Enabled:       tc.enabled,
				AllowCreation: tc.allow,
				Creation:      OrgCreationConfig{Mode: tc.mode},
			}
			if got := o.NormalizedCreationMode(); got != tc.wantNorm {
				t.Errorf("NormalizedCreationMode() = %q, want %q", got, tc.wantNorm)
			}
			if got := o.AuthenticatedCreationAllowed(); got != tc.wantAllowed {
				t.Errorf("AuthenticatedCreationAllowed() = %v, want %v", got, tc.wantAllowed)
			}
		})
	}
}
