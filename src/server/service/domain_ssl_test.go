package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	appcrypto "github.com/webappsgo/caslink/src/common/crypto"
	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/service/acmedns"
	"github.com/webappsgo/caslink/src/server/store"
)

// genSelfSignedPEM returns a valid self-signed cert/key PEM pair for host, so
// CertificateFor's tls.X509KeyPair parse succeeds in tests.
func genSelfSignedPEM(t *testing.T, host string) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{host},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// insertActiveSSLDomain inserts an active, SSL-enabled domain row with the given
// cert/key encrypted under testSSLKey, so CertificateFor resolves it.
func insertActiveSSLDomain(t *testing.T, st *store.Store, host string, certPEM, keyPEM []byte) {
	t.Helper()
	encCert, err := appcrypto.EncryptGCM(testSSLKey(), certPEM)
	if err != nil {
		t.Fatalf("encrypt cert: %v", err)
	}
	encKey, err := appcrypto.EncryptGCM(testSSLKey(), keyPEM)
	if err != nil {
		t.Fatalf("encrypt key: %v", err)
	}
	_, err = st.UsersDB.Exec(
		`INSERT INTO custom_domains
		 (owner_type, owner_id, domain, verification_status, verified_at, status,
		  ssl_status, ssl_enabled, ssl_cert_pem, ssl_key_pem, ssl_expires_at)
		 VALUES ('user', 1, ?, 'verified', ?, 'active', 'active', 1, ?, ?, ?)`,
		host, time.Now(), encCert, encKey, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("insert active ssl domain %q: %v", host, err)
	}
}

// TestCertificateForReturnsDecryptedCert verifies CertificateFor loads,
// decrypts, parses, and memoises an active domain's certificate.
func TestCertificateForReturnsDecryptedCert(t *testing.T) {
	s, st := newSSLDomainService(t, &acmedns.MockIssuer{}, nil)
	certPEM, keyPEM := genSelfSignedPEM(t, "cert.example.com")
	insertActiveSSLDomain(t, st, "cert.example.com", certPEM, keyPEM)

	cert, err := s.CertificateFor(context.Background(), "cert.example.com")
	if err != nil {
		t.Fatalf("CertificateFor: %v", err)
	}
	if cert == nil || len(cert.Certificate) == 0 {
		t.Fatalf("expected a parsed certificate, got %v", cert)
	}

	// Second call must hit the in-memory cache and return the same pointer.
	cert2, err := s.CertificateFor(context.Background(), "cert.example.com")
	if err != nil {
		t.Fatalf("CertificateFor (cached): %v", err)
	}
	if cert2 != cert {
		t.Errorf("expected cached certificate pointer to be reused")
	}
}

// TestCertificateForCaseInsensitiveAndPort verifies SNI hosts with mixed case
// and a port still resolve (normalizeResolveHost strips them).
func TestCertificateForCaseInsensitiveAndPort(t *testing.T) {
	s, st := newSSLDomainService(t, &acmedns.MockIssuer{}, nil)
	certPEM, keyPEM := genSelfSignedPEM(t, "case.example.com")
	insertActiveSSLDomain(t, st, "case.example.com", certPEM, keyPEM)

	cert, err := s.CertificateFor(context.Background(), "CASE.Example.COM:443")
	if err != nil {
		t.Fatalf("CertificateFor: %v", err)
	}
	if cert == nil {
		t.Fatalf("expected certificate for case/port-normalised host")
	}
}

// TestCertificateForNoRowReturnsNil verifies an unknown host yields (nil, nil)
// so the caller falls through to the autocert manager.
func TestCertificateForNoRowReturnsNil(t *testing.T) {
	s, _ := newSSLDomainService(t, &acmedns.MockIssuer{}, nil)
	cert, err := s.CertificateFor(context.Background(), "unknown.example.com")
	if err != nil {
		t.Fatalf("CertificateFor: %v", err)
	}
	if cert != nil {
		t.Errorf("expected nil cert for unknown host, got %v", cert)
	}
}

// TestCertificateForPurgeEvictsCache verifies purgeCachedCert forces the next
// lookup to re-read from the database.
func TestCertificateForPurgeEvictsCache(t *testing.T) {
	s, st := newSSLDomainService(t, &acmedns.MockIssuer{}, nil)
	certPEM, keyPEM := genSelfSignedPEM(t, "purge.example.com")
	insertActiveSSLDomain(t, st, "purge.example.com", certPEM, keyPEM)

	first, err := s.CertificateFor(context.Background(), "purge.example.com")
	if err != nil || first == nil {
		t.Fatalf("first CertificateFor: cert=%v err=%v", first, err)
	}

	s.purgeCachedCert("purge.example.com")

	second, err := s.CertificateFor(context.Background(), "purge.example.com")
	if err != nil || second == nil {
		t.Fatalf("second CertificateFor: cert=%v err=%v", second, err)
	}
	if second == first {
		t.Errorf("expected a freshly reloaded certificate pointer after purge")
	}
}

// TestCertificateForInactiveDomainReturnsNil verifies a verified-but-not-active
// SSL domain does not resolve a certificate.
func TestCertificateForInactiveDomainReturnsNil(t *testing.T) {
	s, st := newSSLDomainService(t, &acmedns.MockIssuer{}, nil)
	insertVerifiedDomain(t, st, "user", 1, "inactive.example.com", false)

	cert, err := s.CertificateFor(context.Background(), "inactive.example.com")
	if err != nil {
		t.Fatalf("CertificateFor: %v", err)
	}
	if cert != nil {
		t.Errorf("expected nil cert for non-active domain, got %v", cert)
	}
}

// testSSLKey returns a deterministic 32-byte AES key for SSL credential tests.
func testSSLKey() []byte {
	k := make([]byte, appcrypto.KeySize)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

// insertVerifiedDomain inserts a verified, active domain row and returns its
// name, so the SSL issuance path (which requires verified+active) can run.
func insertVerifiedDomain(t *testing.T, st *store.Store, ownerType string, ownerID int64, domain string, wildcard bool) {
	t.Helper()
	_, err := st.UsersDB.Exec(
		`INSERT INTO custom_domains
		 (owner_type, owner_id, domain, is_apex, is_wildcard, verification_status, verified_at, status, ssl_status)
		 VALUES (?, ?, ?, 0, ?, 'verified', ?, 'active', 'none')`,
		ownerType, ownerID, domain, wildcard, time.Now())
	if err != nil {
		t.Fatalf("insert verified domain %q: %v", domain, err)
	}
}

// registerTestProvider registers a provider factory that requires an api_token.
func registerTestProvider(name string) {
	acmedns.RegisterProvider(name, func(creds map[string]string) (acmedns.DNSChallengeProvider, error) {
		if creds["api_token"] == "" {
			return nil, errors.New("missing api_token")
		}
		return acmedns.NewMockProvider(name), nil
	})
}

func newSSLDomainService(t *testing.T, issuer acmedns.Issuer, onChange func(string)) (*DomainService, *store.Store) {
	t.Helper()
	st := newTestDomainStore(t)
	s := NewDomainService(st, config.CustomDomainsConfig{})
	s.EnableDNS01SSL(issuer, testSSLKey(), 1, "admin@example.com", true, onChange)
	return s, st
}

func TestEnableDNS01SSLGuards(t *testing.T) {
	st := newTestDomainStore(t)
	s := NewDomainService(st, config.CustomDomainsConfig{})
	// nil issuer -> stays disabled
	s.EnableDNS01SSL(nil, testSSLKey(), 1, "a@b.com", true, nil)
	if s.sslConfigured() {
		t.Error("sslConfigured() true after nil issuer")
	}
	// wrong key length -> stays disabled
	s.EnableDNS01SSL(&acmedns.MockIssuer{}, []byte("short"), 1, "a@b.com", true, nil)
	if s.sslConfigured() {
		t.Error("sslConfigured() true after short key")
	}
	// valid -> enabled
	s.EnableDNS01SSL(&acmedns.MockIssuer{}, testSSLKey(), 1, "a@b.com", true, nil)
	if !s.sslConfigured() {
		t.Error("sslConfigured() false after valid enable")
	}
}

func TestSetDNSProviderStoresEncryptedCredentials(t *testing.T) {
	prov := "unit-set-provider"
	registerTestProvider(prov)
	s, st := newSSLDomainService(t, &acmedns.MockIssuer{}, nil)
	insertVerifiedDomain(t, st, "user", 1, "example.com", false)
	ctx := context.Background()

	creds := map[string]string{"api_token": "secret-value"}
	if err := s.SetDNSProvider(ctx, "user", 1, "example.com", prov, creds); err != nil {
		t.Fatalf("SetDNSProvider: %v", err)
	}

	var storedProvider, storedCreds, challenge string
	err := st.UsersDB.QueryRow(
		`SELECT ssl_provider, ssl_credentials, ssl_challenge FROM custom_domains WHERE domain = ?`,
		"example.com").Scan(&storedProvider, &storedCreds, &challenge)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if storedProvider != prov {
		t.Errorf("stored provider = %q, want %q", storedProvider, prov)
	}
	if challenge != "dns-01" {
		t.Errorf("challenge = %q, want dns-01", challenge)
	}
	// Credentials must NOT be stored in plaintext.
	if storedCreds == "" || storedCreds == "secret-value" {
		t.Fatalf("credentials not encrypted: %q", storedCreds)
	}
	dec, err := appcrypto.DecryptGCM(testSSLKey(), storedCreds)
	if err != nil {
		t.Fatalf("decrypt stored creds: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(dec, &got); err != nil {
		t.Fatalf("unmarshal decrypted creds: %v", err)
	}
	if got["api_token"] != "secret-value" {
		t.Errorf("decrypted api_token = %q, want secret-value", got["api_token"])
	}
}

func TestSetDNSProviderErrors(t *testing.T) {
	prov := "unit-err-provider"
	registerTestProvider(prov)
	s, st := newSSLDomainService(t, &acmedns.MockIssuer{}, nil)
	insertVerifiedDomain(t, st, "user", 1, "example.com", false)
	ctx := context.Background()

	// Unknown provider -> ErrSSLProviderInvalid
	if err := s.SetDNSProvider(ctx, "user", 1, "example.com", "no-such-provider", map[string]string{"api_token": "x"}); !errors.Is(err, model.ErrSSLProviderInvalid) {
		t.Errorf("unknown provider err = %v, want ErrSSLProviderInvalid", err)
	}
	// Bad credentials -> ErrSSLCredentialsInvalid
	if err := s.SetDNSProvider(ctx, "user", 1, "example.com", prov, map[string]string{}); !errors.Is(err, model.ErrSSLCredentialsInvalid) {
		t.Errorf("bad creds err = %v, want ErrSSLCredentialsInvalid", err)
	}
	// Unknown domain -> ErrDomainNotFound
	if err := s.SetDNSProvider(ctx, "user", 1, "nope.example.com", prov, map[string]string{"api_token": "x"}); !errors.Is(err, model.ErrDomainNotFound) {
		t.Errorf("unknown domain err = %v, want ErrDomainNotFound", err)
	}
	// Cross-owner isolation: org 9 cannot touch user 1's domain.
	if err := s.SetDNSProvider(ctx, "org", 9, "example.com", prov, map[string]string{"api_token": "x"}); !errors.Is(err, model.ErrDomainNotFound) {
		t.Errorf("cross-owner err = %v, want ErrDomainNotFound", err)
	}
}

func TestSetDNSProviderRequiresSSLConfigured(t *testing.T) {
	st := newTestDomainStore(t)
	s := NewDomainService(st, config.CustomDomainsConfig{})
	insertVerifiedDomain(t, st, "user", 1, "example.com", false)
	if err := s.SetDNSProvider(context.Background(), "user", 1, "example.com", "x", nil); !errors.Is(err, model.ErrSSLNotConfigured) {
		t.Errorf("err = %v, want ErrSSLNotConfigured", err)
	}
}

func TestIssueDNS01CertSuccess(t *testing.T) {
	prov := "unit-issue-provider"
	registerTestProvider(prov)
	notAfter := time.Now().Add(90 * 24 * time.Hour).Truncate(time.Second)
	mock := &acmedns.MockIssuer{Result: &acmedns.CertResult{
		CertPEM:  []byte("CERTIFICATE-DATA"),
		KeyPEM:   []byte("PRIVATE-KEY-DATA"),
		NotAfter: notAfter,
	}}
	var changed string
	s, st := newSSLDomainService(t, mock, func(host string) { changed = host })
	insertVerifiedDomain(t, st, "user", 1, "example.com", false)
	ctx := context.Background()

	if err := s.SetDNSProvider(ctx, "user", 1, "example.com", prov, map[string]string{"api_token": "x"}); err != nil {
		t.Fatalf("SetDNSProvider: %v", err)
	}

	cd, err := s.IssueDNS01Cert(ctx, "user", 1, "example.com")
	if err != nil {
		t.Fatalf("IssueDNS01Cert: %v", err)
	}
	if cd.SSLStatus != "active" || !cd.SSLEnabled {
		t.Errorf("status=%q enabled=%v, want active/true", cd.SSLStatus, cd.SSLEnabled)
	}
	if changed != "example.com" {
		t.Errorf("onCertChange host = %q, want example.com", changed)
	}
	if d := mock.Domains(); len(d) != 1 || d[0] != "example.com" {
		t.Errorf("issuer called with %v, want [example.com]", d)
	}

	// Cert and key must be stored encrypted, not plaintext.
	var certEnc, keyEnc, status, lastErr string
	var enabled int
	var issuedAt, expiresAt time.Time
	err = st.UsersDB.QueryRow(
		`SELECT ssl_cert_pem, ssl_key_pem, ssl_status, ssl_enabled, ssl_last_error, ssl_issued_at, ssl_expires_at
		 FROM custom_domains WHERE domain = ?`, "example.com").
		Scan(&certEnc, &keyEnc, &status, &enabled, &lastErr, &issuedAt, &expiresAt)
	if err != nil {
		t.Fatalf("read back cert: %v", err)
	}
	if certEnc == "CERTIFICATE-DATA" || keyEnc == "PRIVATE-KEY-DATA" {
		t.Fatal("cert/key stored in plaintext")
	}
	if status != "active" || enabled != 1 {
		t.Errorf("db status=%q enabled=%d", status, enabled)
	}
	if lastErr != "" {
		t.Errorf("ssl_last_error = %q, want empty", lastErr)
	}
	decCert, err := appcrypto.DecryptGCM(testSSLKey(), certEnc)
	if err != nil || string(decCert) != "CERTIFICATE-DATA" {
		t.Errorf("decrypt cert = %q err %v", decCert, err)
	}
	decKey, err := appcrypto.DecryptGCM(testSSLKey(), keyEnc)
	if err != nil || string(decKey) != "PRIVATE-KEY-DATA" {
		t.Errorf("decrypt key = %q err %v", decKey, err)
	}
	if !expiresAt.Equal(notAfter) {
		t.Errorf("ssl_expires_at = %v, want %v", expiresAt, notAfter)
	}
}

func TestIssueDNS01CertWildcardSAN(t *testing.T) {
	prov := "unit-wildcard-provider"
	registerTestProvider(prov)
	mock := &acmedns.MockIssuer{Result: &acmedns.CertResult{
		CertPEM: []byte("c"), KeyPEM: []byte("k"), NotAfter: time.Now().Add(time.Hour),
	}}
	s, st := newSSLDomainService(t, mock, nil)
	insertVerifiedDomain(t, st, "user", 1, "example.com", true)
	ctx := context.Background()
	if err := s.SetDNSProvider(ctx, "user", 1, "example.com", prov, map[string]string{"api_token": "x"}); err != nil {
		t.Fatalf("SetDNSProvider: %v", err)
	}
	if _, err := s.IssueDNS01Cert(ctx, "user", 1, "example.com"); err != nil {
		t.Fatalf("IssueDNS01Cert: %v", err)
	}
	d := mock.Domains()
	if len(d) != 2 || d[0] != "example.com" || d[1] != "*.example.com" {
		t.Errorf("wildcard SAN = %v, want [example.com *.example.com]", d)
	}
}

func TestIssueDNS01CertIssuerFailureRecorded(t *testing.T) {
	prov := "unit-fail-provider"
	registerTestProvider(prov)
	mock := &acmedns.MockIssuer{Err: errors.New("dns propagation timeout")}
	s, st := newSSLDomainService(t, mock, nil)
	insertVerifiedDomain(t, st, "user", 1, "example.com", false)
	ctx := context.Background()
	if err := s.SetDNSProvider(ctx, "user", 1, "example.com", prov, map[string]string{"api_token": "x"}); err != nil {
		t.Fatalf("SetDNSProvider: %v", err)
	}

	_, err := s.IssueDNS01Cert(ctx, "user", 1, "example.com")
	if !errors.Is(err, model.ErrSSLIssuanceFailed) {
		t.Errorf("err = %v, want ErrSSLIssuanceFailed", err)
	}

	var status, lastErr string
	if err := st.UsersDB.QueryRow(
		`SELECT ssl_status, ssl_last_error FROM custom_domains WHERE domain = ?`, "example.com").
		Scan(&status, &lastErr); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if status != "error" {
		t.Errorf("ssl_status = %q, want error", status)
	}
	if lastErr == "" {
		t.Error("ssl_last_error not recorded")
	}
}

func TestIssueDNS01CertRequiresVerified(t *testing.T) {
	prov := "unit-unverified-provider"
	registerTestProvider(prov)
	s, st := newSSLDomainService(t, &acmedns.MockIssuer{Result: &acmedns.CertResult{NotAfter: time.Now()}}, nil)
	// Insert a pending (unverified) domain directly.
	if _, err := st.UsersDB.Exec(
		`INSERT INTO custom_domains (owner_type, owner_id, domain, verification_status, status, ssl_status, ssl_provider, ssl_credentials, ssl_challenge)
		 VALUES ('user', 1, 'example.com', 'pending', 'pending', 'none', ?, ?, 'dns-01')`,
		prov, "enc"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := s.IssueDNS01Cert(context.Background(), "user", 1, "example.com"); !errors.Is(err, model.ErrDomainNotVerified) {
		t.Errorf("err = %v, want ErrDomainNotVerified", err)
	}
}

func TestIssueDNS01CertRequiresProvider(t *testing.T) {
	s, st := newSSLDomainService(t, &acmedns.MockIssuer{Result: &acmedns.CertResult{NotAfter: time.Now()}}, nil)
	insertVerifiedDomain(t, st, "user", 1, "example.com", false)
	// No SetDNSProvider call -> no provider configured.
	if _, err := s.IssueDNS01Cert(context.Background(), "user", 1, "example.com"); !errors.Is(err, model.ErrSSLNotConfigured) {
		t.Errorf("err = %v, want ErrSSLNotConfigured", err)
	}
}
