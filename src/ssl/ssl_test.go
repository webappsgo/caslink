package ssl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSelfSignedCert generates a throwaway ECDSA self-signed certificate for
// host and writes the PEM-encoded cert/key pair to certPath/keyPath, creating
// any missing parent directories. notBefore/notAfter control the
// certificate's validity window so tests can exercise expiry logic without
// real ACME/network calls. host is set as both the CN and a DNS SAN so
// DiscoverCertificate's VerifyHostname check matches.
func writeSelfSignedCert(t *testing.T, certPath, keyPath, host string, notBefore, notAfter time.Time) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshaling key: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		t.Fatalf("mkdir cert dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		t.Fatalf("mkdir key dir: %v", err)
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("encoding cert PEM: %v", err)
	}

	keyOut, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	defer keyOut.Close()
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatalf("encoding key PEM: %v", err)
	}
}

// TestLoadCertificateSuccess verifies a valid cert/key pair loads cleanly.
func TestLoadCertificateSuccess(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writeSelfSignedCert(t, certPath, keyPath, "test.example.invalid", time.Now(), time.Now().Add(90*24*time.Hour))

	cert, err := LoadCertificate(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadCertificate() error = %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("loaded certificate has no DER-encoded chain")
	}
}

// TestLoadCertificateMissingCert verifies a clear, non-panicking error when
// the certificate file does not exist.
func TestLoadCertificateMissingCert(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.pem")
	writeSelfSignedCert(t, filepath.Join(dir, "throwaway.pem"), keyPath, "test.example.invalid", time.Now(), time.Now().Add(time.Hour))

	_, err := LoadCertificate(filepath.Join(dir, "does-not-exist.pem"), keyPath)
	if err == nil {
		t.Fatal("LoadCertificate() error = nil, want error for missing cert file")
	}
}

// TestLoadCertificateMissingKey verifies a clear, non-panicking error when
// the key file does not exist.
func TestLoadCertificateMissingKey(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	writeSelfSignedCert(t, certPath, filepath.Join(dir, "throwaway-key.pem"), "test.example.invalid", time.Now(), time.Now().Add(time.Hour))

	_, err := LoadCertificate(certPath, filepath.Join(dir, "does-not-exist-key.pem"))
	if err == nil {
		t.Fatal("LoadCertificate() error = nil, want error for missing key file")
	}
}

// TestLoadCertificateMismatchedKeyPair verifies a cert and a key that do
// not belong together produce an error rather than a silently-broken
// tls.Certificate.
func TestLoadCertificateMismatchedKeyPair(t *testing.T) {
	dir := t.TempDir()
	certA := filepath.Join(dir, "a-cert.pem")
	keyA := filepath.Join(dir, "a-key.pem")
	writeSelfSignedCert(t, certA, keyA, "test.example.invalid", time.Now(), time.Now().Add(time.Hour))

	certB := filepath.Join(dir, "b-cert.pem")
	keyB := filepath.Join(dir, "b-key.pem")
	writeSelfSignedCert(t, certB, keyB, "test.example.invalid", time.Now(), time.Now().Add(time.Hour))

	// Pair cert A with key B — mismatched.
	_, err := LoadCertificate(certA, keyB)
	if err == nil {
		t.Fatal("LoadCertificate() error = nil, want error for mismatched cert/key pair")
	}
}

// TestAutoDetectCertificateNoneFound verifies AutoDetectCertificate reports
// found=false with empty paths when nothing exists in any searched
// location (excluding the unwritable system /etc/letsencrypt path, which
// this sandbox cannot safely populate).
func TestAutoDetectCertificateNoneFound(t *testing.T) {
	configDir := t.TempDir()

	certPath, keyPath, found := AutoDetectCertificate("nothing-here.example.invalid", configDir)
	if found {
		t.Errorf("AutoDetectCertificate() found = true, want false (cert=%q key=%q)", certPath, keyPath)
	}
	if certPath != "" || keyPath != "" {
		t.Errorf("AutoDetectCertificate() = (%q, %q), want empty paths when not found", certPath, keyPath)
	}
}

// TestAutoDetectCertificatePrefersLetsEncryptOverLocal verifies that when
// both the app-managed Let's Encrypt directory and the local (manual)
// directory contain a cert for the same FQDN, the Let's Encrypt one wins —
// matching the documented lookup order (system LE -> app-managed LE ->
// local -> request new).
func TestAutoDetectCertificatePrefersLetsEncryptOverLocal(t *testing.T) {
	configDir := t.TempDir()
	fqdn := "example.test"

	leCert := filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "fullchain.pem")
	leKey := filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "privkey.pem")
	writeSelfSignedCert(t, leCert, leKey, fqdn, time.Now(), time.Now().Add(time.Hour))

	localCert := filepath.Join(configDir, "ssl", "local", fqdn, "cert.pem")
	localKey := filepath.Join(configDir, "ssl", "local", fqdn, "key.pem")
	writeSelfSignedCert(t, localCert, localKey, fqdn, time.Now(), time.Now().Add(time.Hour))

	certPath, keyPath, found := AutoDetectCertificate(fqdn, configDir)
	if !found {
		t.Fatal("AutoDetectCertificate() found = false, want true")
	}
	if certPath != leCert || keyPath != leKey {
		t.Errorf("AutoDetectCertificate() = (%q, %q), want the app-managed Let's Encrypt pair (%q, %q)", certPath, keyPath, leCert, leKey)
	}
}

// TestAutoDetectCertificateFallsBackToLocal verifies that when only the
// local (manually managed) certificate directory has files, it is used.
func TestAutoDetectCertificateFallsBackToLocal(t *testing.T) {
	configDir := t.TempDir()
	fqdn := "local-only.test"

	localCert := filepath.Join(configDir, "ssl", "local", fqdn, "cert.pem")
	localKey := filepath.Join(configDir, "ssl", "local", fqdn, "key.pem")
	writeSelfSignedCert(t, localCert, localKey, fqdn, time.Now(), time.Now().Add(time.Hour))

	certPath, keyPath, found := AutoDetectCertificate(fqdn, configDir)
	if !found {
		t.Fatal("AutoDetectCertificate() found = false, want true")
	}
	if certPath != localCert || keyPath != localKey {
		t.Errorf("AutoDetectCertificate() = (%q, %q), want local pair (%q, %q)", certPath, keyPath, localCert, localKey)
	}
}

// TestAutoDetectCertificateRequiresBothFiles verifies that a directory with
// only the certificate (no matching key) is treated as not-found rather
// than a partial match, for both searched app-managed locations.
func TestAutoDetectCertificateRequiresBothFiles(t *testing.T) {
	configDir := t.TempDir()
	fqdn := "cert-only.test"

	leCertDir := filepath.Join(configDir, "ssl", "letsencrypt", fqdn)
	if err := os.MkdirAll(leCertDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(leCertDir, "fullchain.pem"), []byte("not a real cert"), 0644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	// Deliberately no privkey.pem written.

	_, _, found := AutoDetectCertificate(fqdn, configDir)
	if found {
		t.Error("AutoDetectCertificate() found = true, want false when only the cert file (no key) exists")
	}
}

// TestAutoDetectCertificateIsFQDNScoped verifies certificates stored under
// a different FQDN are not matched.
func TestAutoDetectCertificateIsFQDNScoped(t *testing.T) {
	configDir := t.TempDir()

	otherCert := filepath.Join(configDir, "ssl", "letsencrypt", "other.test", "fullchain.pem")
	otherKey := filepath.Join(configDir, "ssl", "letsencrypt", "other.test", "privkey.pem")
	writeSelfSignedCert(t, otherCert, otherKey, "other.test", time.Now(), time.Now().Add(time.Hour))

	_, _, found := AutoDetectCertificate("mine.test", configDir)
	if found {
		t.Error("AutoDetectCertificate() found = true, want false for an unrelated FQDN's certificate")
	}
}

// TestCreateTLSConfigDefaultsToTLS12 verifies MinVersion 0 is normalized to
// TLS 1.2, per the documented "TLS 1.2+ required" security policy.
func TestCreateTLSConfigDefaultsToTLS12(t *testing.T) {
	cert := tls.Certificate{}
	cfg := CreateTLSConfig(cert, 0)

	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want TLS 1.2 (%#x)", cfg.MinVersion, tls.VersionTLS12)
	}
}

// TestCreateTLSConfigHonorsExplicitMinVersion verifies an explicit,
// stricter minimum version (e.g. TLS 1.3) is passed through unchanged.
func TestCreateTLSConfigHonorsExplicitMinVersion(t *testing.T) {
	cert := tls.Certificate{}
	cfg := CreateTLSConfig(cert, tls.VersionTLS13)

	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %#x, want TLS 1.3 (%#x)", cfg.MinVersion, tls.VersionTLS13)
	}
}

// TestCreateTLSConfigCarriesCertificateAndCipherSuites verifies the
// returned config embeds the supplied certificate and a non-empty,
// server-preferred cipher suite list.
func TestCreateTLSConfigCarriesCertificateAndCipherSuites(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writeSelfSignedCert(t, certPath, keyPath, "test.example.invalid", time.Now(), time.Now().Add(time.Hour))

	cert, err := LoadCertificate(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadCertificate() error = %v", err)
	}

	cfg := CreateTLSConfig(cert, 0)

	if len(cfg.Certificates) != 1 {
		t.Fatalf("len(cfg.Certificates) = %d, want 1", len(cfg.Certificates))
	}
	if len(cfg.Certificates[0].Certificate) != len(cert.Certificate) {
		t.Error("returned config's certificate does not match the loaded certificate")
	}
	if len(cfg.CipherSuites) == 0 {
		t.Error("expected a non-empty cipher suite allowlist")
	}
}

// TestDiscoverCertificateAppSource verifies a cert under
// {config_dir}/ssl/letsencrypt/{fqdn}/ is classified SourceApp (PART 15
// ownership: app-managed, auto-renew).
func TestDiscoverCertificateAppSource(t *testing.T) {
	configDir := t.TempDir()
	fqdn := "app.example.test"
	cp := filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "fullchain.pem")
	kp := filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "privkey.pem")
	writeSelfSignedCert(t, cp, kp, fqdn, time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))

	dc, ok := DiscoverCertificate(fqdn, configDir)
	if !ok {
		t.Fatal("DiscoverCertificate() ok = false, want true")
	}
	if dc.Source != SourceApp {
		t.Errorf("Source = %v, want SourceApp", dc.Source)
	}
	if !dc.Source.CanAutoRenew() {
		t.Error("app-managed certificate must be auto-renewable")
	}
	if dc.CertPath != cp || dc.KeyPath != kp {
		t.Errorf("paths = (%q, %q), want (%q, %q)", dc.CertPath, dc.KeyPath, cp, kp)
	}
	if dc.Certificate == nil || dc.Certificate.Leaf == nil {
		t.Error("expected a parsed certificate with a populated leaf")
	}
}

// TestDiscoverCertificateLocalSource verifies a cert under
// {config_dir}/ssl/local/{fqdn}/ is classified SourceLocal and is NOT
// auto-renewable (PART 15 ownership: user-managed, manual only).
func TestDiscoverCertificateLocalSource(t *testing.T) {
	configDir := t.TempDir()
	fqdn := "local.example.test"
	cp := filepath.Join(configDir, "ssl", "local", fqdn, "cert.pem")
	kp := filepath.Join(configDir, "ssl", "local", fqdn, "key.pem")
	writeSelfSignedCert(t, cp, kp, fqdn, time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))

	dc, ok := DiscoverCertificate(fqdn, configDir)
	if !ok {
		t.Fatal("DiscoverCertificate() ok = false, want true")
	}
	if dc.Source != SourceLocal {
		t.Errorf("Source = %v, want SourceLocal", dc.Source)
	}
	if dc.Source.CanAutoRenew() {
		t.Error("local certificate must NOT be auto-renewable")
	}
}

// TestDiscoverCertificateSkipsExpired verifies an expired higher-priority cert
// is skipped in favour of a valid lower-priority one, rather than returned.
func TestDiscoverCertificateSkipsExpired(t *testing.T) {
	configDir := t.TempDir()
	fqdn := "expired.example.test"

	appCert := filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "fullchain.pem")
	appKey := filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "privkey.pem")
	writeSelfSignedCert(t, appCert, appKey, fqdn, time.Now().Add(-90*24*time.Hour), time.Now().Add(-time.Hour))

	localCert := filepath.Join(configDir, "ssl", "local", fqdn, "cert.pem")
	localKey := filepath.Join(configDir, "ssl", "local", fqdn, "key.pem")
	writeSelfSignedCert(t, localCert, localKey, fqdn, time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))

	dc, ok := DiscoverCertificate(fqdn, configDir)
	if !ok {
		t.Fatal("DiscoverCertificate() ok = false, want true (should fall through to valid local cert)")
	}
	if dc.Source != SourceLocal {
		t.Errorf("Source = %v, want SourceLocal (expired app cert must be skipped)", dc.Source)
	}
}

// TestDiscoverCertificateSkipsHostnameMismatch verifies a cert whose CN/SAN
// does not match the requested FQDN is rejected (PART 15 validation).
func TestDiscoverCertificateSkipsHostnameMismatch(t *testing.T) {
	configDir := t.TempDir()
	fqdn := "wanted.example.test"
	cp := filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "fullchain.pem")
	kp := filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "privkey.pem")
	// Certificate issued for a different host than the discovery target.
	writeSelfSignedCert(t, cp, kp, "other.example.test", time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))

	if _, ok := DiscoverCertificate(fqdn, configDir); ok {
		t.Error("DiscoverCertificate() ok = true, want false on hostname mismatch")
	}
}

// TestDiscoverCertificateNormalizesHost verifies a host carrying a port,
// mixed case, and trailing dot still matches a cert issued for the bare
// lowercase hostname.
func TestDiscoverCertificateNormalizesHost(t *testing.T) {
	configDir := t.TempDir()
	fqdn := "norm.example.test"
	cp := filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "fullchain.pem")
	kp := filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "privkey.pem")
	writeSelfSignedCert(t, cp, kp, fqdn, time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))

	if _, ok := DiscoverCertificate("Norm.Example.Test.:8443", configDir); !ok {
		t.Error("DiscoverCertificate() ok = false, want true after host normalization (case/port/trailing dot)")
	}
}

// TestSourceRenewalOwnership pins the PART 15 ownership rules: only
// app-managed certs auto-renew; system (certbot) and user-local never do.
func TestSourceRenewalOwnership(t *testing.T) {
	cases := []struct {
		src  Source
		auto bool
		name string
	}{
		{SourceSystem, false, "system"},
		{SourceApp, true, "app"},
		{SourceLocal, false, "local"},
		{SourceNone, false, "none"},
	}
	for _, c := range cases {
		if got := c.src.CanAutoRenew(); got != c.auto {
			t.Errorf("%s.CanAutoRenew() = %v, want %v", c.name, got, c.auto)
		}
		if got := c.src.String(); got != c.name {
			t.Errorf("Source.String() = %q, want %q", got, c.name)
		}
	}
}
