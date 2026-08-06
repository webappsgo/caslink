// Package acmedns implements ACME DNS-01 certificate issuance for custom
// domains (PART 36). DNS-01 is the challenge type used for wildcard domains and
// for servers that cannot expose port 80/443 to the ACME server; the automatic
// HTTP-01 and TLS-ALPN-01 paths for non-wildcard domains are handled elsewhere
// by autocert.Manager.
//
// The package is split into a small, unit-testable surface and a live boundary:
//
//   - DNSChallengeProvider abstracts a DNS API (Cloudflare, Route53, ...). Real
//     providers require operator-supplied API credentials and reach an external
//     service, so they are the named infrastructure boundary; callers register
//     them via RegisterProvider and construct them by name via NewProvider.
//   - Issuer performs the ACME order. ACMEIssuer talks to a live ACME directory,
//     so it cannot run in unit tests; callers depend on the Issuer interface and
//     substitute a mock in tests, which keeps the DomainService integration
//     (persisting and encrypting the issued key, updating SSL status, purging the
//     resolve cache) fully testable without a real certificate authority.
package acmedns

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
)

// LetsEncryptProductionURL and LetsEncryptStagingURL are the ACME directory
// endpoints. Staging is used for testing to avoid Let's Encrypt rate limits.
const (
	LetsEncryptProductionURL = acme.LetsEncryptURL
	LetsEncryptStagingURL    = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// ErrProviderUnsupported is returned by NewProvider for a name with no
// registered factory. Callers map it to the SSL_PROVIDER_INVALID API error.
var ErrProviderUnsupported = errors.New("unsupported dns provider")

// DNSChallengeProvider publishes and removes the TXT record an ACME DNS-01
// challenge requires. fqdn is the fully-qualified record name (already prefixed
// with "_acme-challenge.") and value is the exact TXT payload to serve.
type DNSChallengeProvider interface {
	Present(ctx context.Context, fqdn, value string) error
	CleanUp(ctx context.Context, fqdn, value string) error
	Name() string
}

// ProviderFactory builds a DNSChallengeProvider from decrypted credentials.
type ProviderFactory func(creds map[string]string) (DNSChallengeProvider, error)

var (
	providerMu sync.RWMutex
	providers  = map[string]ProviderFactory{}
)

// RegisterProvider registers a DNS provider factory under name (compared
// case-insensitively). Registering the same name twice replaces the factory.
func RegisterProvider(name string, f ProviderFactory) {
	providerMu.Lock()
	defer providerMu.Unlock()
	providers[strings.ToLower(strings.TrimSpace(name))] = f
}

// NewProvider constructs a registered DNS provider by name, wrapping its
// decrypted credentials. It returns ErrProviderUnsupported when no factory is
// registered for name, so an operator naming an unbuilt provider gets a clean
// validation error rather than a panic.
func NewProvider(name string, creds map[string]string) (DNSChallengeProvider, error) {
	providerMu.RLock()
	f, ok := providers[strings.ToLower(strings.TrimSpace(name))]
	providerMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProviderUnsupported, name)
	}
	return f(creds)
}

// SupportedProviders returns the sorted-insertion-agnostic set of registered
// provider names. Used by the admin UI to populate the provider dropdown.
func SupportedProviders() []string {
	providerMu.RLock()
	defer providerMu.RUnlock()
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}

// CertResult is an issued certificate chain and its private key, both
// PEM-encoded, plus the leaf certificate's NotAfter expiry.
type CertResult struct {
	CertPEM  []byte
	KeyPEM   []byte
	NotAfter time.Time
}

// Issuer obtains a certificate for one or more domain names using DNS-01.
// ACMEIssuer is the live implementation; tests substitute a mock.
type Issuer interface {
	IssueDNS01(ctx context.Context, provider DNSChallengeProvider, domains ...string) (*CertResult, error)
}

// ACMEIssuer issues certificates from a live ACME directory (Let's Encrypt by
// default) using the DNS-01 challenge. It is not exercised by unit tests
// because it requires a reachable ACME server and DNS credentials.
type ACMEIssuer struct {
	email        string
	directoryURL string
	accountKey   crypto.Signer
	// propagation is how long to wait after publishing a TXT record before
	// asking the ACME server to validate it, giving DNS time to propagate.
	propagation time.Duration
}

// NewACMEIssuer creates an issuer with a fresh ECDSA P-256 account key. When
// staging is true the Let's Encrypt staging directory is used.
func NewACMEIssuer(email string, staging bool) (*ACMEIssuer, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ACME account key: %w", err)
	}
	dir := LetsEncryptProductionURL
	if staging {
		dir = LetsEncryptStagingURL
	}
	return &ACMEIssuer{
		email:        email,
		directoryURL: dir,
		accountKey:   key,
		propagation:  15 * time.Second,
	}, nil
}

// SetPropagationWait overrides the post-Present DNS propagation delay. Intended
// for callers that manage propagation externally.
func (i *ACMEIssuer) SetPropagationWait(d time.Duration) {
	if d >= 0 {
		i.propagation = d
	}
}

// IssueDNS01 runs the full DNS-01 order: register (idempotently), authorize
// every domain, publish and validate each TXT challenge via provider, finalize
// with a freshly generated key/CSR, and return the PEM chain, PEM key, and
// expiry. The provider's TXT records are always cleaned up, even on failure.
func (i *ACMEIssuer) IssueDNS01(ctx context.Context, provider DNSChallengeProvider, domains ...string) (*CertResult, error) {
	if provider == nil {
		return nil, errors.New("acmedns: nil DNS provider")
	}
	if len(domains) == 0 {
		return nil, errors.New("acmedns: no domains to issue")
	}

	client := &acme.Client{Key: i.accountKey, DirectoryURL: i.directoryURL}

	acct := &acme.Account{}
	if i.email != "" {
		acct.Contact = []string{"mailto:" + i.email}
	}
	if _, err := client.Register(ctx, acct, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, fmt.Errorf("acme register: %w", err)
	}

	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(domains...))
	if err != nil {
		return nil, fmt.Errorf("acme authorize order: %w", err)
	}

	// Track published records so every one is cleaned up regardless of outcome.
	type published struct{ name, value string }
	var pubs []published
	defer func() {
		for _, p := range pubs {
			_ = provider.CleanUp(ctx, p.name, p.value)
		}
	}()

	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return nil, fmt.Errorf("acme get authorization: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue
		}
		chal := findDNS01Challenge(authz)
		if chal == nil {
			return nil, fmt.Errorf("acme: no dns-01 challenge for %q", authz.Identifier.Value)
		}
		value, err := client.DNS01ChallengeRecord(chal.Token)
		if err != nil {
			return nil, fmt.Errorf("acme dns-01 record: %w", err)
		}
		name := ChallengeRecordName(authz.Identifier.Value)
		if err := provider.Present(ctx, name, value); err != nil {
			return nil, fmt.Errorf("dns provider present %q: %w", name, err)
		}
		pubs = append(pubs, published{name: name, value: value})

		if i.propagation > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(i.propagation):
			}
		}
		if _, err := client.Accept(ctx, chal); err != nil {
			return nil, fmt.Errorf("acme accept challenge: %w", err)
		}
		if _, err := client.WaitAuthorization(ctx, authzURL); err != nil {
			return nil, fmt.Errorf("acme wait authorization: %w", err)
		}
	}

	if _, err := client.WaitOrder(ctx, order.URI); err != nil {
		return nil, fmt.Errorf("acme wait order: %w", err)
	}

	certKey, csr, err := generateKeyAndCSR(domains)
	if err != nil {
		return nil, err
	}
	der, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, fmt.Errorf("acme finalize order: %w", err)
	}

	keyPEM, err := encodeECKeyPEM(certKey)
	if err != nil {
		return nil, err
	}
	certPEM := encodeCertChainPEM(der)
	notAfter, err := leafNotAfter(der)
	if err != nil {
		return nil, err
	}
	return &CertResult{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
}

// findDNS01Challenge returns the dns-01 challenge from an authorization, or nil.
func findDNS01Challenge(authz *acme.Authorization) *acme.Challenge {
	for _, c := range authz.Challenges {
		if c.Type == "dns-01" {
			return c
		}
	}
	return nil
}

// ChallengeRecordName returns the TXT record name for a domain's DNS-01
// challenge. The leading "*." of a wildcard is stripped (the record is placed
// on the base domain) and any trailing dot is removed before prefixing.
func ChallengeRecordName(identifier string) string {
	d := strings.TrimSuffix(strings.TrimPrefix(identifier, "*."), ".")
	return "_acme-challenge." + d
}

// generateKeyAndCSR creates a fresh ECDSA P-256 leaf key and a DER CSR covering
// all domains (first as CN, all as SAN DNSNames).
func generateKeyAndCSR(domains []string) (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate certificate key: %w", err)
	}
	tmpl := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domains[0]},
		DNSNames: domains,
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create CSR: %w", err)
	}
	return key, csr, nil
}

// encodeCertChainPEM concatenates a DER certificate chain into PEM blocks.
func encodeCertChainPEM(chain [][]byte) []byte {
	var out []byte
	for _, der := range chain {
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	return out
}

// encodeECKeyPEM marshals an ECDSA private key to a PKCS#8 PEM block.
func encodeECKeyPEM(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal certificate key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// leafNotAfter parses the first certificate in a DER chain and returns its
// NotAfter time — the chain's leaf, which governs renewal timing.
func leafNotAfter(chain [][]byte) (time.Time, error) {
	if len(chain) == 0 {
		return time.Time{}, errors.New("acmedns: empty certificate chain")
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse issued certificate: %w", err)
	}
	return leaf.NotAfter, nil
}
