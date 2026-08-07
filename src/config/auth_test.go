package config

import (
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/common/crypto"
)

// testKey returns a valid 32-byte AES key for encrypt/decrypt round-trips.
func testKey(t *testing.T) []byte {
	t.Helper()
	encoded, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key, err := crypto.DecodeKey(encoded)
	if err != nil {
		t.Fatalf("DecodeKey: %v", err)
	}
	return key
}

// validOIDC returns a minimal valid OIDC provider (secure defaults on).
func validOIDC() OIDCProvider {
	p := OIDCProvider{
		Name:     "keycloak",
		Issuer:   "https://idp.example.com/realms/main",
		ClientID: "caslink",
		PKCE:     true,
		UseState: true,
		UseNonce: true,
	}
	p.Normalize()
	return p
}

// validLDAP returns a minimal valid LDAP provider.
func validLDAP() LDAPProvider {
	p := LDAPProvider{
		Name:       "corp",
		Server:     "ldaps://ldap.example.com",
		BaseDN:     "dc=example,dc=com",
		UserFilter: "(uid=%s)",
		TLSMode:    "ldaps",
	}
	p.Normalize()
	return p
}

// validSAML returns a minimal valid SAML provider.
func validSAML() SAMLProvider {
	p := SAMLProvider{
		Name:             "okta",
		IDPMetadataURL:   "https://idp.example.com/metadata",
		AutoGenerateCert: true,
	}
	p.Normalize()
	return p
}

func TestOIDCProviderValidate(t *testing.T) {
	if err := validOIDC().Validate(); err != nil {
		t.Fatalf("valid OIDC provider rejected: %v", err)
	}

	cases := map[string]func(*OIDCProvider){
		"bad name":     func(p *OIDCProvider) { p.Name = "Bad Name" },
		"empty issuer": func(p *OIDCProvider) { p.Issuer = "" },
		"empty client": func(p *OIDCProvider) { p.ClientID = "" },
		"pkce off":     func(p *OIDCProvider) { p.PKCE = false },
		"state off":    func(p *OIDCProvider) { p.UseState = false },
		"nonce off":    func(p *OIDCProvider) { p.UseNonce = false },
		"bad mode":     func(p *OIDCProvider) { p.UsernameResolution.Mode = "nope" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := validOIDC()
			mutate(&p)
			if err := p.Validate(); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestOIDCNormalizeDefaults(t *testing.T) {
	p := OIDCProvider{Name: " keycloak "}
	p.Normalize()
	if p.Name != "keycloak" {
		t.Errorf("name not trimmed: %q", p.Name)
	}
	if p.UsernameResolution.Mode != "prompt_on_first_login" {
		t.Errorf("resolution default missing: %q", p.UsernameResolution.Mode)
	}
	if p.DiscoveryCacheTTL != "1h" || p.ClockSkew != "2m" || p.TokenRefreshInterval != "15m" {
		t.Errorf("timing defaults missing: %+v", p)
	}
}

func TestLDAPProviderValidate(t *testing.T) {
	if err := validLDAP().Validate(); err != nil {
		t.Fatalf("valid LDAP provider rejected: %v", err)
	}

	cases := map[string]func(*LDAPProvider){
		"bad name":      func(p *LDAPProvider) { p.Name = "BAD" },
		"empty server":  func(p *LDAPProvider) { p.Server = "" },
		"empty base_dn": func(p *LDAPProvider) { p.BaseDN = "" },
		"empty filter":  func(p *LDAPProvider) { p.UserFilter = "" },
		"bad tls_mode":  func(p *LDAPProvider) { p.TLSMode = "wat" },
		"plaintext remote": func(p *LDAPProvider) {
			p.Server = "ldap://ldap.example.com"
			p.TLSMode = "none"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := validLDAP()
			mutate(&p)
			if err := p.Validate(); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestLDAPPlaintextLocalAllowed(t *testing.T) {
	p := validLDAP()
	p.Server = "ldap://127.0.0.1:389"
	p.TLSMode = "none"
	if err := p.Validate(); err != nil {
		t.Fatalf("local plaintext ldap should be allowed: %v", err)
	}
}

// TestLDAPTLSVerifyDefaultsSecure ensures an omitted tls_verify (zero-value
// nil pointer) normalizes to verification enabled — AI.md PART 34 requires
// "tls_verify: true is the default" so certificate verification is never
// silently disabled by a missing config field.
func TestLDAPTLSVerifyDefaultsSecure(t *testing.T) {
	p := LDAPProvider{Name: "corp"}
	p.Normalize()
	if p.TLSVerify == nil || !*p.TLSVerify {
		t.Fatalf("expected TLSVerify to default to true, got %+v", p.TLSVerify)
	}
	if !p.EffectiveTLSVerify() {
		t.Fatalf("expected EffectiveTLSVerify() to be true by default")
	}
}

// TestLDAPTLSVerifyExplicitFalsePreserved ensures an operator's deliberate
// opt-out survives Normalize() unchanged rather than being coerced back to
// the secure default.
func TestLDAPTLSVerifyExplicitFalsePreserved(t *testing.T) {
	disabled := false
	p := LDAPProvider{Name: "corp", TLSVerify: &disabled}
	p.Normalize()
	if p.TLSVerify == nil || *p.TLSVerify {
		t.Fatalf("expected explicit false TLSVerify to be preserved, got %+v", p.TLSVerify)
	}
	if p.EffectiveTLSVerify() {
		t.Fatalf("expected EffectiveTLSVerify() to be false when explicitly disabled")
	}
}

func TestSAMLProviderValidate(t *testing.T) {
	if err := validSAML().Validate(); err != nil {
		t.Fatalf("valid SAML provider rejected: %v", err)
	}

	cases := map[string]func(*SAMLProvider){
		"bad name":    func(p *SAMLProvider) { p.Name = "BAD NAME" },
		"no metadata": func(p *SAMLProvider) { p.IDPMetadataURL = "" },
		"both metadata": func(p *SAMLProvider) {
			p.IDPMetadataXML = "<xml/>"
		},
		"missing cert paths": func(p *SAMLProvider) {
			p.AutoGenerateCert = false
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := validSAML()
			mutate(&p)
			if err := p.Validate(); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestSAMLManualCertValid(t *testing.T) {
	p := validSAML()
	p.AutoGenerateCert = false
	p.SPCertPath = "/etc/caslink/sp.crt"
	p.SPKeyPath = "/etc/caslink/sp.key"
	if err := p.Validate(); err != nil {
		t.Fatalf("manual-cert SAML provider rejected: %v", err)
	}
}

func TestEncryptSecretsRoundTrip(t *testing.T) {
	key := testKey(t)
	c := AuthConfig{
		OIDC: OIDCAuthConfig{Providers: []OIDCProvider{{Name: "a", ClientSecret: "oidc-secret"}}},
		LDAP: LDAPAuthConfig{Providers: []LDAPProvider{{Name: "b", BindPassword: "ldap-pass"}}},
		SAML: SAMLAuthConfig{Providers: []SAMLProvider{{Name: "c", SPPrivateKey: "saml-key"}}},
	}
	if err := c.EncryptSecrets(key); err != nil {
		t.Fatalf("EncryptSecrets: %v", err)
	}

	for _, val := range []string{
		c.OIDC.Providers[0].ClientSecret,
		c.LDAP.Providers[0].BindPassword,
		c.SAML.Providers[0].SPPrivateKey,
	} {
		if !strings.HasPrefix(val, encPrefix) {
			t.Fatalf("secret not encrypted: %q", val)
		}
	}

	got, err := DecryptSecret(key, c.OIDC.Providers[0].ClientSecret)
	if err != nil {
		t.Fatalf("DecryptSecret oidc: %v", err)
	}
	if got != "oidc-secret" {
		t.Errorf("oidc round-trip mismatch: %q", got)
	}
	got, err = DecryptSecret(key, c.LDAP.Providers[0].BindPassword)
	if err != nil {
		t.Fatalf("DecryptSecret ldap: %v", err)
	}
	if got != "ldap-pass" {
		t.Errorf("ldap round-trip mismatch: %q", got)
	}
	got, err = DecryptSecret(key, c.SAML.Providers[0].SPPrivateKey)
	if err != nil {
		t.Fatalf("DecryptSecret saml: %v", err)
	}
	if got != "saml-key" {
		t.Errorf("saml round-trip mismatch: %q", got)
	}
}

func TestEncryptSecretsIdempotent(t *testing.T) {
	key := testKey(t)
	c := AuthConfig{OIDC: OIDCAuthConfig{Providers: []OIDCProvider{{Name: "a", ClientSecret: "s"}}}}
	if err := c.EncryptSecrets(key); err != nil {
		t.Fatalf("first encrypt: %v", err)
	}
	first := c.OIDC.Providers[0].ClientSecret
	if err := c.EncryptSecrets(key); err != nil {
		t.Fatalf("second encrypt: %v", err)
	}
	if c.OIDC.Providers[0].ClientSecret != first {
		t.Fatalf("double-encrypt changed value: %q -> %q", first, c.OIDC.Providers[0].ClientSecret)
	}
	got, err := DecryptSecret(key, c.OIDC.Providers[0].ClientSecret)
	if err != nil {
		t.Fatalf("decrypt after double: %v", err)
	}
	if got != "s" {
		t.Errorf("value corrupted by double-encrypt: %q", got)
	}
}

func TestDecryptSecretPlaintextPassthrough(t *testing.T) {
	key := testKey(t)
	got, err := DecryptSecret(key, "freshly-typed")
	if err != nil {
		t.Fatalf("passthrough: %v", err)
	}
	if got != "freshly-typed" {
		t.Errorf("plaintext passthrough mismatch: %q", got)
	}
}

func TestMaskedCopy(t *testing.T) {
	c := AuthConfig{
		OIDC: OIDCAuthConfig{Providers: []OIDCProvider{
			{Name: "a", ClientSecret: "enc:xyz"},
			{Name: "b", ClientSecret: ""},
		}},
		LDAP: LDAPAuthConfig{Providers: []LDAPProvider{{Name: "c", BindPassword: "enc:pw"}}},
		SAML: SAMLAuthConfig{Providers: []SAMLProvider{{Name: "d", SPPrivateKey: "enc:k"}}},
	}
	masked := c.MaskedCopy()

	if masked.OIDC.Providers[0].ClientSecret != secretMask {
		t.Errorf("present secret not masked: %q", masked.OIDC.Providers[0].ClientSecret)
	}
	if masked.OIDC.Providers[1].ClientSecret != "" {
		t.Errorf("absent secret should stay empty: %q", masked.OIDC.Providers[1].ClientSecret)
	}
	if masked.LDAP.Providers[0].BindPassword != secretMask {
		t.Errorf("ldap secret not masked")
	}
	if masked.SAML.Providers[0].SPPrivateKey != secretMask {
		t.Errorf("saml secret not masked")
	}

	// Original must be untouched (deep copy, not aliased).
	if c.OIDC.Providers[0].ClientSecret != "enc:xyz" {
		t.Errorf("MaskedCopy mutated original: %q", c.OIDC.Providers[0].ClientSecret)
	}
}

func TestAuthIndex(t *testing.T) {
	oidc := OIDCAuthConfig{Providers: []OIDCProvider{{Name: "a"}, {Name: "b"}}}
	if oidc.Index("b") != 1 {
		t.Errorf("oidc index b: got %d", oidc.Index("b"))
	}
	if oidc.Index("missing") != -1 {
		t.Errorf("oidc index missing should be -1")
	}
	ldap := LDAPAuthConfig{Providers: []LDAPProvider{{Name: "x"}}}
	if ldap.Index("x") != 0 || ldap.Index("y") != -1 {
		t.Errorf("ldap index wrong")
	}
	saml := SAMLAuthConfig{Providers: []SAMLProvider{{Name: "z"}}}
	if saml.Index("z") != 0 || saml.Index("q") != -1 {
		t.Errorf("saml index wrong")
	}
}

func TestDefaultConfigAuthDisabled(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.Auth.OIDC.Enabled || cfg.Server.Auth.LDAP.Enabled || cfg.Server.Auth.SAML.Enabled {
		t.Errorf("auth sections must default to disabled")
	}
	if cfg.Server.Auth.OIDC.Providers != nil || cfg.Server.Auth.LDAP.Providers != nil || cfg.Server.Auth.SAML.Providers != nil {
		t.Errorf("auth providers must default to nil")
	}
}
