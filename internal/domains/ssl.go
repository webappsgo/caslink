package domains

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// SSLService handles SSL certificate management
type SSLService struct {
	db         *db.DB
	config     *config.Config
	logger     *logrus.Logger
	certDir    string
	keyDir     string
}

// SSLCertificate represents an SSL certificate
type SSLCertificate struct {
	ID          string     `json:"id" db:"id"`
	DomainID    string     `json:"domain_id" db:"domain_id"`
	Domain      string     `json:"domain" db:"domain"`
	CertPath    string     `json:"cert_path" db:"cert_path"`
	KeyPath     string     `json:"key_path" db:"key_path"`
	Provider    string     `json:"provider" db:"provider"` // self-signed, manual, letsencrypt
	Status      string     `json:"status" db:"status"`     // pending, active, expired, error
	IssuedBy    string     `json:"issued_by" db:"issued_by"`
	IssuedAt    time.Time  `json:"issued_at" db:"issued_at"`
	ExpiresAt   time.Time  `json:"expires_at" db:"expires_at"`
	RenewedAt   *time.Time `json:"renewed_at,omitempty" db:"renewed_at"`
	Error       string     `json:"error,omitempty" db:"error"`
	AutoRenew   bool       `json:"auto_renew" db:"auto_renew"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// CertificateInfo represents certificate details
type CertificateInfo struct {
	Subject        string    `json:"subject"`
	Issuer         string    `json:"issuer"`
	SerialNumber   string    `json:"serial_number"`
	NotBefore      time.Time `json:"not_before"`
	NotAfter       time.Time `json:"not_after"`
	DNSNames       []string  `json:"dns_names"`
	SignatureAlgo  string    `json:"signature_algorithm"`
	PublicKeyAlgo  string    `json:"public_key_algorithm"`
	KeyUsage       []string  `json:"key_usage"`
	ExtKeyUsage    []string  `json:"ext_key_usage"`
	IsCA           bool      `json:"is_ca"`
	IsSelfSigned   bool      `json:"is_self_signed"`
	ValidDays      int       `json:"valid_days"`
	ExpiresInDays  int       `json:"expires_in_days"`
}

// CertificateRequest represents a certificate request
type CertificateRequest struct {
	Domain        string   `json:"domain" validate:"required,fqdn"`
	Provider      string   `json:"provider" validate:"required,oneof=self-signed manual letsencrypt"`
	AutoRenew     bool     `json:"auto_renew"`
	KeySize       int      `json:"key_size,omitempty"`
	Organization  string   `json:"organization,omitempty"`
	Country       string   `json:"country,omitempty"`
	State         string   `json:"state,omitempty"`
	City          string   `json:"city,omitempty"`
	ValidityDays  int      `json:"validity_days,omitempty"`
}

// NewSSLService creates a new SSL service
func NewSSLService(database *db.DB, cfg *config.Config, logger *logrus.Logger) (*SSLService, error) {
	certDir := filepath.Join(cfg.Server.DataDir, "ssl", "certs")
	keyDir := filepath.Join(cfg.Server.DataDir, "ssl", "private")

	// Ensure SSL directories exist with proper permissions
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create certificate directory: %w", err)
	}

	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create private key directory: %w", err)
	}

	return &SSLService{
		db:      database,
		config:  cfg,
		logger:  logger,
		certDir: certDir,
		keyDir:  keyDir,
	}, nil
}

// GenerateSelfSignedCertificate generates a self-signed certificate for a domain
func (s *SSLService) GenerateSelfSignedCertificate(ctx context.Context, domainID string, req *CertificateRequest) (*SSLCertificate, error) {
	// Set defaults
	if req.KeySize == 0 {
		req.KeySize = 2048
	}
	if req.ValidityDays == 0 {
		req.ValidityDays = 365
	}

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, req.KeySize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{req.Organization},
			Country:       []string{req.Country},
			Province:      []string{req.State},
			Locality:      []string{req.City},
			CommonName:    req.Domain,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Duration(req.ValidityDays) * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{req.Domain},
		IPAddresses:           []net.IP{},
	}

	// Add wildcard support if domain starts with *.
	if strings.HasPrefix(req.Domain, "*.") {
		template.DNSNames = append(template.DNSNames, req.Domain[2:]) // Add apex domain
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Generate file paths
	certID := s.generateCertificateID()
	certFile := filepath.Join(s.certDir, fmt.Sprintf("%s_%s.crt", domainID, certID))
	keyFile := filepath.Join(s.keyDir, fmt.Sprintf("%s_%s.key", domainID, certID))

	// Save certificate
	certOut, err := os.Create(certFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return nil, fmt.Errorf("failed to write certificate: %w", err)
	}

	// Save private key with restricted permissions
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create private key file: %w", err)
	}
	defer keyOut.Close()

	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER}); err != nil {
		return nil, fmt.Errorf("failed to write private key: %w", err)
	}

	// Create certificate record
	cert := &SSLCertificate{
		ID:        certID,
		DomainID:  domainID,
		Domain:    req.Domain,
		CertPath:  certFile,
		KeyPath:   keyFile,
		Provider:  "self-signed",
		Status:    "active",
		IssuedBy:  "Caslink Self-Signed",
		IssuedAt:  template.NotBefore,
		ExpiresAt: template.NotAfter,
		AutoRenew: req.AutoRenew,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save certificate record to database
	if err := s.saveCertificateRecord(ctx, cert); err != nil {
		// Clean up files on database error
		os.Remove(certFile)
		os.Remove(keyFile)
		return nil, fmt.Errorf("failed to save certificate record: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"domain_id":     domainID,
		"domain":        req.Domain,
		"certificate_id": certID,
		"expires_at":    cert.ExpiresAt,
	}).Info("Self-signed certificate generated")

	return cert, nil
}

// InstallManualCertificate installs a manually provided certificate
func (s *SSLService) InstallManualCertificate(ctx context.Context, domainID string, certPEM, keyPEM []byte) (*SSLCertificate, error) {
	// Parse certificate to extract information
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid certificate PEM format")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Parse private key to validate
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("invalid private key PEM format")
	}

	var privateKey interface{}
	switch keyBlock.Type {
	case "PRIVATE KEY":
		privateKey, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	case "EC PRIVATE KEY":
		privateKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	default:
		return nil, fmt.Errorf("unsupported private key type: %s", keyBlock.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Verify certificate and key match
	if err := s.verifyCertificateKeyPair(cert, privateKey); err != nil {
		return nil, fmt.Errorf("certificate and private key do not match: %w", err)
	}

	// Generate file paths
	certID := s.generateCertificateID()
	certFile := filepath.Join(s.certDir, fmt.Sprintf("%s_%s.crt", domainID, certID))
	keyFile := filepath.Join(s.keyDir, fmt.Sprintf("%s_%s.key", domainID, certID))

	// Save certificate file
	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("failed to save certificate file: %w", err)
	}

	// Save private key file with restricted permissions
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		os.Remove(certFile)
		return nil, fmt.Errorf("failed to save private key file: %w", err)
	}

	// Determine domain name from certificate
	domain := cert.Subject.CommonName
	if domain == "" && len(cert.DNSNames) > 0 {
		domain = cert.DNSNames[0]
	}

	// Create certificate record
	sslCert := &SSLCertificate{
		ID:        certID,
		DomainID:  domainID,
		Domain:    domain,
		CertPath:  certFile,
		KeyPath:   keyFile,
		Provider:  "manual",
		Status:    "active",
		IssuedBy:  cert.Issuer.CommonName,
		IssuedAt:  cert.NotBefore,
		ExpiresAt: cert.NotAfter,
		AutoRenew: false, // Manual certificates don't auto-renew
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save certificate record to database
	if err := s.saveCertificateRecord(ctx, sslCert); err != nil {
		// Clean up files on database error
		os.Remove(certFile)
		os.Remove(keyFile)
		return nil, fmt.Errorf("failed to save certificate record: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"domain_id":      domainID,
		"domain":         domain,
		"certificate_id": certID,
		"issuer":         cert.Issuer.CommonName,
		"expires_at":     cert.NotAfter,
	}).Info("Manual certificate installed")

	return sslCert, nil
}

// GetCertificate retrieves a certificate by ID
func (s *SSLService) GetCertificate(ctx context.Context, certID string) (*SSLCertificate, error) {
	query := `
		SELECT id, domain_id, domain, cert_path, key_path, provider, status,
		       issued_by, issued_at, expires_at, renewed_at, error, auto_renew,
		       created_at, updated_at
		FROM ssl_certificates
		WHERE id = ?`

	row := s.db.QueryRowContext(ctx, query, certID)

	cert := &SSLCertificate{}
	err := row.Scan(
		&cert.ID, &cert.DomainID, &cert.Domain, &cert.CertPath, &cert.KeyPath,
		&cert.Provider, &cert.Status, &cert.IssuedBy, &cert.IssuedAt,
		&cert.ExpiresAt, &cert.RenewedAt, &cert.Error, &cert.AutoRenew,
		&cert.CreatedAt, &cert.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("certificate not found: %w", err)
	}

	return cert, nil
}

// GetCertificateByDomain retrieves the active certificate for a domain
func (s *SSLService) GetCertificateByDomain(ctx context.Context, domainID string) (*SSLCertificate, error) {
	query := `
		SELECT id, domain_id, domain, cert_path, key_path, provider, status,
		       issued_by, issued_at, expires_at, renewed_at, error, auto_renew,
		       created_at, updated_at
		FROM ssl_certificates
		WHERE domain_id = ? AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1`

	row := s.db.QueryRowContext(ctx, query, domainID)

	cert := &SSLCertificate{}
	err := row.Scan(
		&cert.ID, &cert.DomainID, &cert.Domain, &cert.CertPath, &cert.KeyPath,
		&cert.Provider, &cert.Status, &cert.IssuedBy, &cert.IssuedAt,
		&cert.ExpiresAt, &cert.RenewedAt, &cert.Error, &cert.AutoRenew,
		&cert.CreatedAt, &cert.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("no active certificate found for domain: %w", err)
	}

	return cert, nil
}

// GetCertificateInfo retrieves detailed certificate information
func (s *SSLService) GetCertificateInfo(ctx context.Context, certID string) (*CertificateInfo, error) {
	cert, err := s.GetCertificate(ctx, certID)
	if err != nil {
		return nil, err
	}

	return s.parseCertificateFile(cert.CertPath)
}

// parseCertificateFile parses a certificate file and extracts information
func (s *SSLService) parseCertificateFile(certPath string) (*CertificateInfo, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid certificate file format")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Calculate days until expiration
	now := time.Now()
	validDays := int(cert.NotAfter.Sub(cert.NotBefore).Hours() / 24)
	expiresInDays := int(cert.NotAfter.Sub(now).Hours() / 24)

	// Extract key usage
	keyUsage := []string{}
	if cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		keyUsage = append(keyUsage, "Digital Signature")
	}
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
		keyUsage = append(keyUsage, "Key Encipherment")
	}
	if cert.KeyUsage&x509.KeyUsageDataEncipherment != 0 {
		keyUsage = append(keyUsage, "Data Encipherment")
	}

	// Extract extended key usage
	extKeyUsage := []string{}
	for _, usage := range cert.ExtKeyUsage {
		switch usage {
		case x509.ExtKeyUsageServerAuth:
			extKeyUsage = append(extKeyUsage, "Server Authentication")
		case x509.ExtKeyUsageClientAuth:
			extKeyUsage = append(extKeyUsage, "Client Authentication")
		case x509.ExtKeyUsageCodeSigning:
			extKeyUsage = append(extKeyUsage, "Code Signing")
		case x509.ExtKeyUsageEmailProtection:
			extKeyUsage = append(extKeyUsage, "Email Protection")
		}
	}

	// Check if self-signed
	isSelfSigned := cert.Subject.String() == cert.Issuer.String()

	return &CertificateInfo{
		Subject:       cert.Subject.String(),
		Issuer:        cert.Issuer.String(),
		SerialNumber:  cert.SerialNumber.String(),
		NotBefore:     cert.NotBefore,
		NotAfter:      cert.NotAfter,
		DNSNames:      cert.DNSNames,
		SignatureAlgo: cert.SignatureAlgorithm.String(),
		PublicKeyAlgo: cert.PublicKeyAlgorithm.String(),
		KeyUsage:      keyUsage,
		ExtKeyUsage:   extKeyUsage,
		IsCA:          cert.IsCA,
		IsSelfSigned:  isSelfSigned,
		ValidDays:     validDays,
		ExpiresInDays: expiresInDays,
	}, nil
}

// LoadTLSConfig loads TLS configuration for a domain
func (s *SSLService) LoadTLSConfig(ctx context.Context, domainID string) (*tls.Config, error) {
	cert, err := s.GetCertificateByDomain(ctx, domainID)
	if err != nil {
		return nil, err
	}

	// Load certificate and key
	tlsCert, err := tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate pair: %w", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	return config, nil
}

// DeleteCertificate deletes a certificate and its files
func (s *SSLService) DeleteCertificate(ctx context.Context, certID string) error {
	cert, err := s.GetCertificate(ctx, certID)
	if err != nil {
		return err
	}

	// Delete certificate files
	if err := os.Remove(cert.CertPath); err != nil && !os.IsNotExist(err) {
		s.logger.WithError(err).Warn("Failed to delete certificate file")
	}

	if err := os.Remove(cert.KeyPath); err != nil && !os.IsNotExist(err) {
		s.logger.WithError(err).Warn("Failed to delete private key file")
	}

	// Delete database record
	_, err = s.db.ExecContext(ctx, "DELETE FROM ssl_certificates WHERE id = ?", certID)
	if err != nil {
		return fmt.Errorf("failed to delete certificate record: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"certificate_id": certID,
		"domain":         cert.Domain,
	}).Info("Certificate deleted")

	return nil
}

// CheckExpiringCertificates returns certificates expiring within the specified days
func (s *SSLService) CheckExpiringCertificates(ctx context.Context, days int) ([]*SSLCertificate, error) {
	threshold := time.Now().Add(time.Duration(days) * 24 * time.Hour)

	query := `
		SELECT id, domain_id, domain, cert_path, key_path, provider, status,
		       issued_by, issued_at, expires_at, renewed_at, error, auto_renew,
		       created_at, updated_at
		FROM ssl_certificates
		WHERE status = 'active' AND expires_at <= ?
		ORDER BY expires_at ASC`

	rows, err := s.db.QueryContext(ctx, query, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to query expiring certificates: %w", err)
	}
	defer rows.Close()

	var certificates []*SSLCertificate
	for rows.Next() {
		cert := &SSLCertificate{}
		err := rows.Scan(
			&cert.ID, &cert.DomainID, &cert.Domain, &cert.CertPath, &cert.KeyPath,
			&cert.Provider, &cert.Status, &cert.IssuedBy, &cert.IssuedAt,
			&cert.ExpiresAt, &cert.RenewedAt, &cert.Error, &cert.AutoRenew,
			&cert.CreatedAt, &cert.UpdatedAt,
		)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to scan certificate")
			continue
		}
		certificates = append(certificates, cert)
	}

	return certificates, nil
}

// verifyCertificateKeyPair verifies that a certificate and private key match
func (s *SSLService) verifyCertificateKeyPair(cert *x509.Certificate, privateKey interface{}) error {
	// Create a temporary TLS certificate to test the pairing
	certDER := cert.Raw
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	_, err = tls.X509KeyPair(certPEM, keyPEM)
	return err
}

// saveCertificateRecord saves a certificate record to the database
func (s *SSLService) saveCertificateRecord(ctx context.Context, cert *SSLCertificate) error {
	query := `
		INSERT INTO ssl_certificates
		(id, domain_id, domain, cert_path, key_path, provider, status,
		 issued_by, issued_at, expires_at, auto_renew, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		cert.ID, cert.DomainID, cert.Domain, cert.CertPath, cert.KeyPath,
		cert.Provider, cert.Status, cert.IssuedBy, cert.IssuedAt,
		cert.ExpiresAt, cert.AutoRenew, cert.CreatedAt, cert.UpdatedAt,
	)

	return err
}

// generateCertificateID generates a unique certificate ID
func (s *SSLService) generateCertificateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("%x", bytes)
}