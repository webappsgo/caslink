package ssl

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds SSL/TLS configuration
type Config struct {
	Enabled    bool
	CertPath   string
	KeyPath    string
	MinVersion uint16
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

// AutoDetectCertificate attempts to auto-detect SSL certificate location
// Per SPEC: /etc/letsencrypt/live/{fqdn}/ → {config_dir}/ssl/letsencrypt/{fqdn}/ → {config_dir}/ssl/local/{fqdn}/
func AutoDetectCertificate(fqdn, configDir string) (certPath, keyPath string, found bool) {
	// Paths to check in order
	searchPaths := []struct {
		cert string
		key  string
	}{
		{
			cert: filepath.Join("/etc/letsencrypt/live", fqdn, "fullchain.pem"),
			key:  filepath.Join("/etc/letsencrypt/live", fqdn, "privkey.pem"),
		},
		{
			cert: filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "fullchain.pem"),
			key:  filepath.Join(configDir, "ssl", "letsencrypt", fqdn, "privkey.pem"),
		},
		{
			cert: filepath.Join(configDir, "ssl", "local", fqdn, "cert.pem"),
			key:  filepath.Join(configDir, "ssl", "local", fqdn, "key.pem"),
		},
	}

	for _, paths := range searchPaths {
		if _, err := os.Stat(paths.cert); err == nil {
			if _, err := os.Stat(paths.key); err == nil {
				return paths.cert, paths.key, true
			}
		}
	}

	return "", "", false
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
