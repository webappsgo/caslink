package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/webappsgo/caslink/src/common/crypto"
)

// AuthConfig holds external identity-provider configuration (OIDC, LDAP, SAML)
// per AI.md PART 34 ("External Identity Provider Requirements"). Each protocol
// supports multiple named providers, managed from the admin panel under
// /server/{admin_path}/config/security/auth/*. This is the CONFIG + ADMIN
// MANAGEMENT layer only; browser login flows live in a separate feature.
type AuthConfig struct {
	OIDC OIDCAuthConfig `yaml:"oidc"`
	LDAP LDAPAuthConfig `yaml:"ldap"`
	SAML SAMLAuthConfig `yaml:"saml"`
}

// encPrefix marks an already-encrypted secret value so EncryptSecrets never
// double-encrypts and DecryptSecret knows the payload is ciphertext.
const encPrefix = "enc:"

// secretMask is returned in place of a stored secret for API/UI output.
const secretMask = "********"

// providerNameRe validates a provider name: a lowercase-safe slug so it is safe
// to embed in the stored source value ("oidc:{provider}") and in routes.
var providerNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// validResolutionModes is the closed set of username_resolution.mode values.
var validResolutionModes = map[string]bool{
	"prompt_on_first_login": true,
	"prompt_if_conflict":    true,
	"reject_if_conflict":    true,
}

// UsernameResolution controls how an initial username is chosen for a new
// external account on first login.
type UsernameResolution struct {
	Mode                    string `yaml:"mode" json:"mode"`
	AllowCustomOnFirstLogin bool   `yaml:"allow_custom_on_first_login" json:"allow_custom_on_first_login"`
}

// OIDCAuthConfig is the OIDC section: an enable flag plus named providers.
type OIDCAuthConfig struct {
	Enabled   bool           `yaml:"enabled" json:"enabled"`
	Providers []OIDCProvider `yaml:"providers" json:"providers"`
}

// OIDCClaimsMapping maps id_token/userinfo claims to user fields.
type OIDCClaimsMapping struct {
	Username string `yaml:"username" json:"username"`
	Email    string `yaml:"email" json:"email"`
	Name     string `yaml:"name" json:"name"`
	Groups   string `yaml:"groups" json:"groups"`
}

// OIDCProvider is a single OIDC identity provider definition.
type OIDCProvider struct {
	Name               string              `yaml:"name" json:"name"`
	DisplayName        string              `yaml:"display_name" json:"display_name"`
	Issuer             string              `yaml:"issuer" json:"issuer"`
	ClientID           string              `yaml:"client_id" json:"client_id"`
	ClientSecret       string              `yaml:"client_secret" json:"client_secret"`
	Scopes             []string            `yaml:"scopes" json:"scopes"`
	AutoRegister       bool                `yaml:"auto_register" json:"auto_register"`
	ClaimsMapping      OIDCClaimsMapping   `yaml:"claims_mapping" json:"claims_mapping"`
	UsernameResolution UsernameResolution  `yaml:"username_resolution" json:"username_resolution"`
	AdminGroups        []string            `yaml:"admin_groups" json:"admin_groups"`
	RoleMapping        map[string][]string `yaml:"role_mapping" json:"role_mapping"`

	PKCE     bool `yaml:"pkce" json:"pkce"`
	UseState bool `yaml:"use_state" json:"use_state"`
	UseNonce bool `yaml:"use_nonce" json:"use_nonce"`

	DiscoveryCacheTTL      string `yaml:"discovery_cache_ttl" json:"discovery_cache_ttl"`
	ClockSkew              string `yaml:"clock_skew" json:"clock_skew"`
	TokenRefreshInterval   string `yaml:"token_refresh_interval" json:"token_refresh_interval"`
	RPInitiatedLogout      bool   `yaml:"rp_initiated_logout" json:"rp_initiated_logout"`
	BackchannelLogout      bool   `yaml:"backchannel_logout" json:"backchannel_logout"`
	RevokeSessionsOnDelete bool   `yaml:"revoke_sessions_on_delete" json:"revoke_sessions_on_delete"`
}

// LDAPAuthConfig is the LDAP section: an enable flag plus named providers.
type LDAPAuthConfig struct {
	Enabled   bool           `yaml:"enabled" json:"enabled"`
	Providers []LDAPProvider `yaml:"providers" json:"providers"`
}

// LDAPAttributes maps directory attributes to user fields.
type LDAPAttributes struct {
	Username string `yaml:"username" json:"username"`
	Email    string `yaml:"email" json:"email"`
	Name     string `yaml:"name" json:"name"`
	Groups   string `yaml:"groups" json:"groups"`
}

// LDAPPool sizes the connection pool for the service (bind) account.
type LDAPPool struct {
	MaxConns  int  `yaml:"max_conns" json:"max_conns"`
	MaxIdle   int  `yaml:"max_idle" json:"max_idle"`
	Reconnect bool `yaml:"reconnect" json:"reconnect"`
}

// LDAPProvider is a single LDAP/AD directory definition.
type LDAPProvider struct {
	Name               string              `yaml:"name" json:"name"`
	DisplayName        string              `yaml:"display_name" json:"display_name"`
	Server             string              `yaml:"server" json:"server"`
	BindDN             string              `yaml:"bind_dn" json:"bind_dn"`
	BindPassword       string              `yaml:"bind_password" json:"bind_password"`
	BaseDN             string              `yaml:"base_dn" json:"base_dn"`
	UserFilter         string              `yaml:"user_filter" json:"user_filter"`
	AutoRegister       bool                `yaml:"auto_register" json:"auto_register"`
	Attributes         LDAPAttributes      `yaml:"attributes" json:"attributes"`
	UsernameResolution UsernameResolution  `yaml:"username_resolution" json:"username_resolution"`
	AdminGroups        []string            `yaml:"admin_groups" json:"admin_groups"`
	RoleMapping        map[string][]string `yaml:"role_mapping" json:"role_mapping"`

	TLSMode string `yaml:"tls_mode" json:"tls_mode"`
	// TLSVerify is a tri-state pointer so an omitted config value (nil) can be
	// distinguished from an explicit false: Normalize defaults nil to true so
	// certificate verification is secure by default per AI.md PART 34
	// ("tls_verify: true is the default"), never silently disabled by a
	// missing field. Use EffectiveTLSVerify to read it safely.
	TLSVerify *bool `yaml:"tls_verify" json:"tls_verify"`

	ConnectTimeout string `yaml:"connect_timeout" json:"connect_timeout"`
	RequestTimeout string `yaml:"request_timeout" json:"request_timeout"`

	Pool               LDAPPool `yaml:"pool" json:"pool"`
	FailoverServers    []string `yaml:"failover_servers" json:"failover_servers"`
	FollowReferrals    bool     `yaml:"follow_referrals" json:"follow_referrals"`
	PageSize           int      `yaml:"page_size" json:"page_size"`
	NestedGroups       bool     `yaml:"nested_groups" json:"nested_groups"`
	BindFailureBackoff string   `yaml:"bind_failure_backoff" json:"bind_failure_backoff"`
}

// SAMLAuthConfig is the SAML section: an enable flag plus named providers.
type SAMLAuthConfig struct {
	Enabled   bool           `yaml:"enabled" json:"enabled"`
	Providers []SAMLProvider `yaml:"providers" json:"providers"`
}

// SAMLAttributeMapping maps assertion attributes to user fields.
type SAMLAttributeMapping struct {
	Username string `yaml:"username" json:"username"`
	Email    string `yaml:"email" json:"email"`
	Name     string `yaml:"name" json:"name"`
	Groups   string `yaml:"groups" json:"groups"`
}

// SAMLProvider is a single SAML 2.0 IdP definition (the app acts as SP only).
type SAMLProvider struct {
	Name               string               `yaml:"name" json:"name"`
	DisplayName        string               `yaml:"display_name" json:"display_name"`
	IDPMetadataURL     string               `yaml:"idp_metadata_url" json:"idp_metadata_url"`
	IDPMetadataXML     string               `yaml:"idp_metadata_xml" json:"idp_metadata_xml"`
	SPEntityID         string               `yaml:"sp_entity_id" json:"sp_entity_id"`
	ACSURL             string               `yaml:"acs_url" json:"acs_url"`
	AutoRegister       bool                 `yaml:"auto_register" json:"auto_register"`
	AttributeMapping   SAMLAttributeMapping `yaml:"attribute_mapping" json:"attribute_mapping"`
	UsernameResolution UsernameResolution   `yaml:"username_resolution" json:"username_resolution"`
	AdminGroups        []string             `yaml:"admin_groups" json:"admin_groups"`
	RoleMapping        map[string][]string  `yaml:"role_mapping" json:"role_mapping"`

	NameIDFormat     string `yaml:"nameid_format" json:"nameid_format"`
	AutoGenerateCert bool   `yaml:"auto_generate_cert" json:"auto_generate_cert"`
	SPCertPath       string `yaml:"sp_cert_path" json:"sp_cert_path"`
	SPKeyPath        string `yaml:"sp_key_path" json:"sp_key_path"`
	SPPrivateKey     string `yaml:"sp_private_key" json:"sp_private_key"`

	SLOEnabled        bool   `yaml:"slo_enabled" json:"slo_enabled"`
	IDPSLOURL         string `yaml:"idp_slo_url" json:"idp_slo_url"`
	AllowIDPInitiated bool   `yaml:"allow_idp_initiated" json:"allow_idp_initiated"`
}

// isValidProviderName reports whether name is a lowercase-safe slug.
func isValidProviderName(name string) bool {
	return providerNameRe.MatchString(name)
}

// validateResolution rejects an unknown username_resolution.mode. An empty mode
// is treated as valid (Normalize substitutes the default).
func validateResolution(res UsernameResolution) error {
	if res.Mode != "" && !validResolutionModes[res.Mode] {
		return fmt.Errorf("username_resolution.mode %q is invalid", res.Mode)
	}
	return nil
}

// Normalize fills OIDC defaults that the spec marks REQUIRED-by-default so a
// minimally-specified provider is valid and secure (PKCE S256, state, nonce).
func (p *OIDCProvider) Normalize() {
	p.Name = strings.TrimSpace(p.Name)
	if p.UsernameResolution.Mode == "" {
		p.UsernameResolution.Mode = "prompt_on_first_login"
	}
	if p.DiscoveryCacheTTL == "" {
		p.DiscoveryCacheTTL = "1h"
	}
	if p.ClockSkew == "" {
		p.ClockSkew = "2m"
	}
	if p.TokenRefreshInterval == "" {
		p.TokenRefreshInterval = "15m"
	}
}

// Validate enforces the OIDC provider invariants from AI.md PART 34.
func (p OIDCProvider) Validate() error {
	if !isValidProviderName(p.Name) {
		return fmt.Errorf("oidc provider name %q must be a lowercase slug", p.Name)
	}
	if strings.TrimSpace(p.Issuer) == "" {
		return fmt.Errorf("oidc provider %q: issuer is required", p.Name)
	}
	if strings.TrimSpace(p.ClientID) == "" {
		return fmt.Errorf("oidc provider %q: client_id is required", p.Name)
	}
	if !p.PKCE {
		return fmt.Errorf("oidc provider %q: PKCE (S256) is required and cannot be disabled", p.Name)
	}
	if !p.UseState || !p.UseNonce {
		return fmt.Errorf("oidc provider %q: use_state and use_nonce are required per request", p.Name)
	}
	return validateResolution(p.UsernameResolution)
}

// Normalize fills LDAP defaults (tls_mode, tls_verify, resolution mode).
func (p *LDAPProvider) Normalize() {
	p.Name = strings.TrimSpace(p.Name)
	if p.TLSMode == "" {
		p.TLSMode = "starttls"
	}
	if p.TLSVerify == nil {
		verify := true
		p.TLSVerify = &verify
	}
	if p.UsernameResolution.Mode == "" {
		p.UsernameResolution.Mode = "prompt_on_first_login"
	}
	if p.ConnectTimeout == "" {
		p.ConnectTimeout = "5s"
	}
	if p.RequestTimeout == "" {
		p.RequestTimeout = "10s"
	}
}

// EffectiveTLSVerify reports whether certificate verification is enabled,
// treating an unset (nil) TLSVerify as verified — the secure default per
// AI.md PART 34. Prefer calling this over reading the field directly.
func (p LDAPProvider) EffectiveTLSVerify() bool {
	return p.TLSVerify == nil || *p.TLSVerify
}

// isLocalLDAPHost reports whether the LDAP server host is loopback, where
// plaintext ldap:// with tls_mode none is permitted for local testing.
func isLocalLDAPHost(server string) bool {
	host := server
	if u, err := url.Parse(server); err == nil && u.Host != "" {
		host = u.Hostname()
	} else if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host[i+1:], "]") {
		host = host[:i]
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// Validate enforces the LDAP provider invariants from AI.md PART 34.
func (p LDAPProvider) Validate() error {
	if !isValidProviderName(p.Name) {
		return fmt.Errorf("ldap provider name %q must be a lowercase slug", p.Name)
	}
	if strings.TrimSpace(p.Server) == "" {
		return fmt.Errorf("ldap provider %q: server is required", p.Name)
	}
	if strings.TrimSpace(p.BaseDN) == "" {
		return fmt.Errorf("ldap provider %q: base_dn is required", p.Name)
	}
	if strings.TrimSpace(p.UserFilter) == "" {
		return fmt.Errorf("ldap provider %q: user_filter is required", p.Name)
	}
	switch p.TLSMode {
	case "ldaps", "starttls", "none":
	default:
		return fmt.Errorf("ldap provider %q: tls_mode must be ldaps, starttls, or none", p.Name)
	}
	if p.TLSMode == "none" && strings.HasPrefix(strings.ToLower(p.Server), "ldap://") && !isLocalLDAPHost(p.Server) {
		return fmt.Errorf("ldap provider %q: plaintext ldap:// with tls_mode none is blocked for non-local hosts", p.Name)
	}
	return validateResolution(p.UsernameResolution)
}

// Normalize fills SAML defaults (resolution mode, auto_generate_cert intent).
func (p *SAMLProvider) Normalize() {
	p.Name = strings.TrimSpace(p.Name)
	if p.UsernameResolution.Mode == "" {
		p.UsernameResolution.Mode = "prompt_on_first_login"
	}
	if p.NameIDFormat == "" {
		p.NameIDFormat = "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent"
	}
}

// Validate enforces the SAML provider invariants from AI.md PART 34. acs_url and
// sp_entity_id are derivable from the FQDN and so are not required here.
func (p SAMLProvider) Validate() error {
	if !isValidProviderName(p.Name) {
		return fmt.Errorf("saml provider name %q must be a lowercase slug", p.Name)
	}
	hasURL := strings.TrimSpace(p.IDPMetadataURL) != ""
	hasXML := strings.TrimSpace(p.IDPMetadataXML) != ""
	if hasURL == hasXML {
		return fmt.Errorf("saml provider %q: exactly one of idp_metadata_url or idp_metadata_xml is required", p.Name)
	}
	if !p.AutoGenerateCert {
		if strings.TrimSpace(p.SPCertPath) == "" || strings.TrimSpace(p.SPKeyPath) == "" {
			return fmt.Errorf("saml provider %q: sp_cert_path and sp_key_path are required when auto_generate_cert is false", p.Name)
		}
	}
	return validateResolution(p.UsernameResolution)
}

// encryptOne encrypts a single secret value in place unless it is empty or
// already carries the enc: marker (idempotent double-encrypt guard).
func encryptOne(key []byte, val string) (string, error) {
	if val == "" || strings.HasPrefix(val, encPrefix) {
		return val, nil
	}
	enc, err := crypto.EncryptGCM(key, []byte(val))
	if err != nil {
		return "", err
	}
	return encPrefix + enc, nil
}

// EncryptSecrets encrypts every reversible provider secret at rest with the
// AES-256-GCM key. Already-encrypted (enc:-prefixed) values are left untouched.
func (c *AuthConfig) EncryptSecrets(key []byte) error {
	for i := range c.OIDC.Providers {
		v, err := encryptOne(key, c.OIDC.Providers[i].ClientSecret)
		if err != nil {
			return err
		}
		c.OIDC.Providers[i].ClientSecret = v
	}
	for i := range c.LDAP.Providers {
		v, err := encryptOne(key, c.LDAP.Providers[i].BindPassword)
		if err != nil {
			return err
		}
		c.LDAP.Providers[i].BindPassword = v
	}
	for i := range c.SAML.Providers {
		v, err := encryptOne(key, c.SAML.Providers[i].SPPrivateKey)
		if err != nil {
			return err
		}
		c.SAML.Providers[i].SPPrivateKey = v
	}
	return nil
}

// DecryptSecret reverses encryptOne for a single value. A value without the
// enc: marker is returned unchanged (freshly-entered plaintext).
func DecryptSecret(key []byte, val string) (string, error) {
	if !strings.HasPrefix(val, encPrefix) {
		return val, nil
	}
	plain, err := crypto.DecryptGCM(key, strings.TrimPrefix(val, encPrefix))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// HasSecret reports whether a stored secret value is present (encrypted or not).
func HasSecret(val string) bool {
	return strings.TrimSpace(val) != ""
}

// MaskedCopy returns a deep copy of the AuthConfig with every reversible secret
// replaced by a mask (present) or empty string (absent). Never used for
// persistence — only for API/UI output so decrypted secrets never leave the
// process.
func (c AuthConfig) MaskedCopy() AuthConfig {
	out := c
	out.OIDC.Providers = make([]OIDCProvider, len(c.OIDC.Providers))
	copy(out.OIDC.Providers, c.OIDC.Providers)
	for i := range out.OIDC.Providers {
		out.OIDC.Providers[i].ClientSecret = maskSecret(c.OIDC.Providers[i].ClientSecret)
	}
	out.LDAP.Providers = make([]LDAPProvider, len(c.LDAP.Providers))
	copy(out.LDAP.Providers, c.LDAP.Providers)
	for i := range out.LDAP.Providers {
		out.LDAP.Providers[i].BindPassword = maskSecret(c.LDAP.Providers[i].BindPassword)
	}
	out.SAML.Providers = make([]SAMLProvider, len(c.SAML.Providers))
	copy(out.SAML.Providers, c.SAML.Providers)
	for i := range out.SAML.Providers {
		out.SAML.Providers[i].SPPrivateKey = maskSecret(c.SAML.Providers[i].SPPrivateKey)
	}
	return out
}

// maskSecret returns the mask when a secret is present, else empty string.
func maskSecret(val string) string {
	if HasSecret(val) {
		return secretMask
	}
	return ""
}

// OIDCIndex returns the slice index of the named OIDC provider, or -1.
func (c *OIDCAuthConfig) Index(name string) int {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			return i
		}
	}
	return -1
}

// LDAPIndex returns the slice index of the named LDAP provider, or -1.
func (c *LDAPAuthConfig) Index(name string) int {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			return i
		}
	}
	return -1
}

// SAMLIndex returns the slice index of the named SAML provider, or -1.
func (c *SAMLAuthConfig) Index(name string) int {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			return i
		}
	}
	return -1
}
