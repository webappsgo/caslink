package acmedns

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"
)

// bigOne is a fixed serial number for the self-signed test certificates.
func bigOne() *big.Int { return big.NewInt(1) }

func TestChallengeRecordName(t *testing.T) {
	cases := map[string]string{
		"example.com":    "_acme-challenge.example.com",
		"*.example.com":  "_acme-challenge.example.com",
		"example.com.":   "_acme-challenge.example.com",
		"*.example.com.": "_acme-challenge.example.com",
		"sub.example.io": "_acme-challenge.sub.example.io",
	}
	for in, want := range cases {
		if got := ChallengeRecordName(in); got != want {
			t.Errorf("ChallengeRecordName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGenerateKeyAndCSR(t *testing.T) {
	domains := []string{"a.example.com", "b.example.com"}
	key, csrDER, err := generateKeyAndCSR(domains)
	if err != nil {
		t.Fatalf("generateKeyAndCSR: %v", err)
	}
	if key == nil {
		t.Fatal("nil key")
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		t.Fatalf("parse CSR: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("CSR signature: %v", err)
	}
	if csr.Subject.CommonName != "a.example.com" {
		t.Errorf("CN = %q, want a.example.com", csr.Subject.CommonName)
	}
	if len(csr.DNSNames) != 2 || csr.DNSNames[0] != "a.example.com" || csr.DNSNames[1] != "b.example.com" {
		t.Errorf("DNSNames = %v, want the two input domains", csr.DNSNames)
	}
}

func TestEncodeECKeyPEMRoundTrip(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemBytes, err := encodeECKeyPEM(key)
	if err != nil {
		t.Fatalf("encodeECKeyPEM: %v", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatalf("bad PEM block: %+v", block)
	}
	if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
		t.Fatalf("parse PKCS8: %v", err)
	}
}

// makeSelfSignedDER builds a self-signed leaf so the chain helpers can be
// exercised without a live CA.
func makeSelfSignedDER(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: bigOne(),
		NotBefore:    notAfter.Add(-24 * time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{"leaf.example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return der
}

func TestEncodeCertChainPEMAndLeafNotAfter(t *testing.T) {
	want := time.Now().Add(90 * 24 * time.Hour).Truncate(time.Second)
	leaf := makeSelfSignedDER(t, want)
	intermediate := makeSelfSignedDER(t, want.Add(365*24*time.Hour))
	chain := [][]byte{leaf, intermediate}

	pemBytes := encodeCertChainPEM(chain)
	var blocks int
	rest := pemBytes
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if b.Type != "CERTIFICATE" {
			t.Fatalf("unexpected PEM type %q", b.Type)
		}
		blocks++
	}
	if blocks != 2 {
		t.Fatalf("got %d CERTIFICATE blocks, want 2", blocks)
	}

	got, err := leafNotAfter(chain)
	if err != nil {
		t.Fatalf("leafNotAfter: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("leafNotAfter = %v, want %v (must read the leaf, not the intermediate)", got, want)
	}
}

func TestLeafNotAfterEmptyChain(t *testing.T) {
	if _, err := leafNotAfter(nil); err == nil {
		t.Error("expected error on empty chain")
	}
}

func TestNewACMEIssuerStagingVsProd(t *testing.T) {
	prod, err := NewACMEIssuer("admin@example.com", false)
	if err != nil {
		t.Fatalf("prod issuer: %v", err)
	}
	if prod.directoryURL != LetsEncryptProductionURL {
		t.Errorf("prod directory = %q, want %q", prod.directoryURL, LetsEncryptProductionURL)
	}
	if prod.accountKey == nil {
		t.Error("nil account key")
	}
	staging, err := NewACMEIssuer("", true)
	if err != nil {
		t.Fatalf("staging issuer: %v", err)
	}
	if staging.directoryURL != LetsEncryptStagingURL {
		t.Errorf("staging directory = %q, want %q", staging.directoryURL, LetsEncryptStagingURL)
	}
}

func TestIssueDNS01GuardsBadInput(t *testing.T) {
	iss, err := NewACMEIssuer("a@b.com", true)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	if _, err := iss.IssueDNS01(context.Background(), nil, "example.com"); err == nil {
		t.Error("expected error for nil provider")
	}
	if _, err := iss.IssueDNS01(context.Background(), NewMockProvider("mock")); err == nil {
		t.Error("expected error for no domains")
	}
}

func TestProviderRegistry(t *testing.T) {
	name := "unit-test-provider"
	RegisterProvider(name, func(creds map[string]string) (DNSChallengeProvider, error) {
		if creds["api_token"] == "" {
			return nil, errors.New("missing api_token")
		}
		return NewMockProvider(name), nil
	})

	if _, err := NewProvider("no-such-provider", nil); !errors.Is(err, ErrProviderUnsupported) {
		t.Errorf("NewProvider(unknown) err = %v, want ErrProviderUnsupported", err)
	}
	if _, err := NewProvider(name, map[string]string{}); err == nil {
		t.Error("expected factory validation error for empty creds")
	}
	p, err := NewProvider("  UNIT-TEST-PROVIDER  ", map[string]string{"api_token": "x"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != name {
		t.Errorf("provider name = %q, want %q", p.Name(), name)
	}

	var found bool
	for _, n := range SupportedProviders() {
		if n == name {
			found = true
		}
	}
	if !found {
		t.Errorf("SupportedProviders() missing %q", name)
	}
}

func TestMockProviderRecords(t *testing.T) {
	m := NewMockProvider("mock")
	ctx := context.Background()
	if err := m.Present(ctx, "_acme-challenge.example.com", "tok"); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if got := m.Presented()["_acme-challenge.example.com"]; got != "tok" {
		t.Errorf("Presented value = %q, want tok", got)
	}
	if err := m.CleanUp(ctx, "_acme-challenge.example.com", "tok"); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if got := m.CleanedUp(); len(got) != 1 || got[0] != "_acme-challenge.example.com" {
		t.Errorf("CleanedUp = %v", got)
	}

	failing := &MockProvider{ProviderName: "boom", PresentErr: errors.New("nope")}
	if err := failing.Present(ctx, "x", "y"); err == nil {
		t.Error("expected PresentErr")
	}
}

func TestMockIssuer(t *testing.T) {
	want := &CertResult{CertPEM: []byte("cert"), KeyPEM: []byte("key"), NotAfter: time.Now()}
	iss := &MockIssuer{Result: want}
	prov := NewMockProvider("mock")
	got, err := iss.IssueDNS01(context.Background(), prov, "a.example.com", "b.example.com")
	if err != nil {
		t.Fatalf("IssueDNS01: %v", err)
	}
	if got != want {
		t.Error("returned result is not the configured one")
	}
	if d := iss.Domains(); len(d) != 2 || d[0] != "a.example.com" {
		t.Errorf("recorded domains = %v", d)
	}
	if iss.CalledWith != prov {
		t.Error("provider not recorded")
	}

	failing := &MockIssuer{Err: errors.New("fail")}
	if _, err := failing.IssueDNS01(context.Background(), prov, "x"); err == nil {
		t.Error("expected error from MockIssuer")
	}
}
