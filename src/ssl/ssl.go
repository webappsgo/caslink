package ssl

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config holds SSL/TLS configuration
type Config struct {
	Enabled    bool
	CertPath   string
	KeyPath    string
	MinVersion uint16
}

// Source classifies where a discovered certificate lives, which determines who
// is responsible for renewing it (AI.md PART 15 "Certificate Management
// Ownership").
type Source int

const (
	// SourceNone means no certificate was discovered.
	SourceNone Source = iota
	// SourceSystem is a certbot-managed cert under /etc/letsencrypt/live/**.
	// The app uses it but NEVER renews it — the system (certbot) owns renewal.
	SourceSystem
	// SourceApp is an app-managed Let's Encrypt cert under
	// {config_dir}/ssl/letsencrypt/{fqdn}/. The app auto-renews it 7 days
	// before expiry.
	SourceApp
	// SourceLocal is a user-provided or self-signed cert under
	// {config_dir}/ssl/local/{fqdn}/. The app uses it but NEVER auto-renews it.
	SourceLocal
)

// String renders the Source for logs.
func (s Source) String() string {
	switch s {
	case SourceSystem:
		return "system"
	case SourceApp:
		return "app"
	case SourceLocal:
		return "local"
	default:
		return "none"
	}
}

// CanAutoRenew reports whether the app is permitted to auto-renew a certificate
// from this source. Only app-managed Let's Encrypt certs under
// {config_dir}/ssl/letsencrypt/{fqdn}/ are auto-renewed (PART 15 "Renewal
// Rules"); system (certbot) and user-managed local certs are never touched.
func (s Source) CanAutoRenew() bool {
	return s == SourceApp
}

// DiscoveredCert is a validated certificate found on disk along with the paths
// it was loaded from and its ownership classification.
type DiscoveredCert struct {
	Certificate *tls.Certificate
	CertPath    string
	KeyPath     string
	Source      Source
}

// candidate describes one path pair to probe, in priority order.
type candidate struct {
	certPath string
	keyPath  string
	source   Source
}

// candidatePaths returns the ordered certificate search paths for a host,
// exactly per the AI.md PART 15 "Certificate Lookup Order" table:
//
//  1. /etc/letsencrypt/live/domain/        (literal "domain" dir — shared setup)
//  2. /etc/letsencrypt/live/{fqdn}/        (fqdn-named certbot dir)
//  3. {config_dir}/ssl/letsencrypt/{fqdn}/ (app-managed, auto-renew)
//  4. {config_dir}/ssl/local/{fqdn}/       (user/self-signed, manual)
func candidatePaths(fqdn, configDir string) []candidate {
	return []candidate{
		{
			certPath: filepath.Join("/etc/letsencrypt/live", "domain", "fullchain.pem"),
			keyPath:  filepath.Join("/etc/letsencrypt/live", "domain", "privkey.pem"),
			source:   SourceSystem,
		},
		{
			certPath: filepath.Join("/etc/letsencrypt/live", fqdn, "fullchain.pem"),
			keyPath:  filepath.Join("/etc/letsencrypt/live", fqdn, "privkey.pem"),
			source:   SourceSystem,
		},
		{
			certPath: filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "fullchain.pem"),
			keyPath:  filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "privkey.pem"),
			source:   SourceApp,
		},
		{
			certPath: filepath.Join(configDir, "ssl", "local", fqdn, "cert.pem"),
			keyPath:  filepath.Join(configDir, "ssl", "local", fqdn, "key.pem"),
			source:   SourceLocal,
		},
	}
}

// DiscoverCertificate probes the PART 15 certificate lookup order for the given
// host and returns the first certificate that (a) has both cert and key files
// readable, (b) is not expired, and (c) matches the host via CN or SAN. It
// returns ok=false when no valid certificate is found in any location, in which
// case the caller should request a new certificate (e.g. via Let's Encrypt) and
// save it under {config_dir}/ssl/letsencrypt/{fqdn}/.
//
// A candidate that exists but fails validation (unreadable, expired, or
// hostname mismatch) is skipped in favour of the next lower-priority path,
// never returned.
func DiscoverCertificate(fqdn, configDir string) (DiscoveredCert, bool) {
	host := normalizeHost(fqdn)
	if host == "" {
		return DiscoveredCert{}, false
	}
	for _, c := range candidatePaths(host, configDir) {
		cert, err := loadValidated(c.certPath, c.keyPath, host)
		if err != nil {
			continue
		}
		return DiscoveredCert{
			Certificate: cert,
			CertPath:    c.certPath,
			KeyPath:     c.keyPath,
			Source:      c.source,
		}, true
	}
	return DiscoveredCert{}, false
}

// loadValidated loads a keypair and validates it against the host. It returns
// an error (rather than a zero cert) so DiscoverCertificate can skip to the
// next candidate.
func loadValidated(certPath, keyPath, host string) (*tls.Certificate, error) {
	if _, err := os.Stat(certPath); err != nil {
		return nil, fmt.Errorf("certificate not readable: %s: %w", certPath, err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		return nil, fmt.Errorf("key not readable: %s: %w", keyPath, err)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load keypair (%s / %s): %w", certPath, keyPath, err)
	}

	leaf := cert.Leaf
	if leaf == nil {
		parsed, perr := x509.ParseCertificate(cert.Certificate[0])
		if perr != nil {
			return nil, fmt.Errorf("failed to parse leaf certificate %s: %w", certPath, perr)
		}
		leaf = parsed
		cert.Leaf = parsed
	}

	now := time.Now()
	if now.Before(leaf.NotBefore) {
		return nil, fmt.Errorf("certificate %s not yet valid (NotBefore %s)", certPath, leaf.NotBefore)
	}
	if now.After(leaf.NotAfter) {
		return nil, fmt.Errorf("certificate %s expired (NotAfter %s)", certPath, leaf.NotAfter)
	}

	if err := leaf.VerifyHostname(host); err != nil {
		return nil, fmt.Errorf("certificate %s does not match host %q: %w", certPath, host, err)
	}

	return &cert, nil
}

// normalizeHost lowercases the host and strips any port and trailing dot so the
// lookup and VerifyHostname behave consistently.
func normalizeHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return ""
	}
	if i := strings.LastIndex(h, ":"); i != -1 && !strings.Contains(h, "]") {
		h = h[:i]
	}
	return strings.TrimSuffix(h, ".")
}

// LoadCertificate loads SSL/TLS certificate and key
func LoadCertificate(certPath, keyPath string) (tls.Certificate, error) {
	// Check if files exist
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return tls.Certificate{}, fmt.Errorf("certificate not found: %s", certPath)
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return tls.Certificate{}, fmt.Errorf("key not found: %s", keyPath)
	}

	// Load certificate
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to load certificate: %w", err)
	}

	return cert, nil
}

// AutoDetectCertificate attempts to auto-detect an SSL certificate location for
// the host, returning the paths of the first valid certificate found in the
// PART 15 lookup order. It is a thin wrapper over DiscoverCertificate for
// callers that only need the paths.
func AutoDetectCertificate(fqdn, configDir string) (certPath, keyPath string, found bool) {
	dc, ok := DiscoverCertificate(fqdn, configDir)
	if !ok {
		return "", "", false
	}
	return dc.CertPath, dc.KeyPath, true
}

// CreateTLSConfig creates a TLS configuration
func CreateTLSConfig(cert tls.Certificate, minVersion uint16) *tls.Config {
	if minVersion == 0 {
		minVersion = tls.VersionTLS12 // Default to TLS 1.2
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   minVersion,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
		PreferServerCipherSuites: true,
	}
}
