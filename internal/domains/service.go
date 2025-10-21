package domains

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Service handles custom domain management
type Service struct {
	db            *db.DB
	config        *config.Config
	logger        *logrus.Logger
	verification  *VerificationService
	ssl           *SSLService
	routing       *RoutingService
}

// Domain represents a custom domain configuration
type Domain struct {
	ID                 string    `json:"id" db:"id"`
	UserID             string    `json:"user_id" db:"user_id"`
	Domain             string    `json:"domain" db:"domain"`
	IsDefault          bool      `json:"is_default" db:"is_default"`
	SSLEnabled         bool      `json:"ssl_enabled" db:"ssl_enabled"`
	SSLCertPath        string    `json:"ssl_cert_path,omitempty" db:"ssl_cert_path"`
	SSLKeyPath         string    `json:"ssl_key_path,omitempty" db:"ssl_key_path"`
	Verified           bool      `json:"verified" db:"verified"`
	VerificationToken  string    `json:"verification_token,omitempty" db:"verification_token"`
	VerificationMethod string    `json:"verification_method" db:"verification_method"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	VerifiedAt         *time.Time `json:"verified_at,omitempty" db:"verified_at"`
}

// DomainCreateRequest represents a domain creation request
type DomainCreateRequest struct {
	Domain             string `json:"domain" validate:"required,fqdn"`
	VerificationMethod string `json:"verification_method" validate:"required,oneof=dns file email"`
	IsDefault          bool   `json:"is_default"`
}

// DomainUpdateRequest represents a domain update request
type DomainUpdateRequest struct {
	IsDefault          *bool   `json:"is_default,omitempty"`
	SSLEnabled         *bool   `json:"ssl_enabled,omitempty"`
	SSLCertPath        *string `json:"ssl_cert_path,omitempty"`
	SSLKeyPath         *string `json:"ssl_key_path,omitempty"`
	VerificationMethod *string `json:"verification_method,omitempty" validate:"omitempty,oneof=dns file email"`
}

// DomainListResponse represents paginated domain list
type DomainListResponse struct {
	Domains    []Domain `json:"domains"`
	Total      int      `json:"total"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
	TotalPages int      `json:"total_pages"`
}

// DomainStats represents domain usage statistics
type DomainStats struct {
	DomainID      string `json:"domain_id"`
	Domain        string `json:"domain"`
	URLCount      int    `json:"url_count"`
	TotalClicks   int64  `json:"total_clicks"`
	UniqueClicks  int64  `json:"unique_clicks"`
	LastUsed      *time.Time `json:"last_used,omitempty"`
	CreatedURLs24h int    `json:"created_urls_24h"`
	Clicks24h     int64  `json:"clicks_24h"`
}

// VerificationStatus represents domain verification status
type VerificationStatus struct {
	Domain     string    `json:"domain"`
	Verified   bool      `json:"verified"`
	Method     string    `json:"method"`
	Token      string    `json:"token,omitempty"`
	LastCheck  time.Time `json:"last_check"`
	NextCheck  time.Time `json:"next_check"`
	Error      string    `json:"error,omitempty"`
	Instructions map[string]interface{} `json:"instructions,omitempty"`
}

// NewService creates a new domains service
func NewService(database *db.DB, cfg *config.Config, logger *logrus.Logger) (*Service, error) {
	verification, err := NewVerificationService(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create verification service: %w", err)
	}

	ssl, err := NewSSLService(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSL service: %w", err)
	}

	routing, err := NewRoutingService(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create routing service: %w", err)
	}

	return &Service{
		db:           database,
		config:       cfg,
		logger:       logger,
		verification: verification,
		ssl:          ssl,
		routing:      routing,
	}, nil
}

// CreateDomain creates a new custom domain
func (s *Service) CreateDomain(ctx context.Context, userID string, req *DomainCreateRequest) (*Domain, error) {
	// Validate domain format
	if err := s.validateDomainName(req.Domain); err != nil {
		return nil, fmt.Errorf("invalid domain: %w", err)
	}

	// Check if domain already exists
	exists, err := s.domainExists(ctx, req.Domain)
	if err != nil {
		return nil, fmt.Errorf("failed to check domain existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("domain already registered")
	}

	// Generate verification token
	token, err := s.generateVerificationToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate verification token: %w", err)
	}

	// Create domain record
	domain := &Domain{
		ID:                 s.generateDomainID(),
		UserID:             userID,
		Domain:             strings.ToLower(req.Domain),
		IsDefault:          req.IsDefault,
		SSLEnabled:         false,
		Verified:           false,
		VerificationToken:  token,
		VerificationMethod: req.VerificationMethod,
		CreatedAt:          time.Now(),
	}

	// If this is the user's first domain or explicitly set as default,
	// make it the default domain
	if req.IsDefault {
		if err := s.clearDefaultDomain(ctx, userID); err != nil {
			return nil, fmt.Errorf("failed to clear existing default domain: %w", err)
		}
	}

	// Insert domain into database
	query := `
		INSERT INTO domains (id, user_id, domain, is_default, ssl_enabled, ssl_cert_path, ssl_key_path,
		                    verified, verification_token, verification_method, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		domain.ID, domain.UserID, domain.Domain, domain.IsDefault, domain.SSLEnabled,
		domain.SSLCertPath, domain.SSLKeyPath, domain.Verified, domain.VerificationToken,
		domain.VerificationMethod, domain.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create domain: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"domain_id": domain.ID,
		"domain":    domain.Domain,
		"user_id":   userID,
		"method":    req.VerificationMethod,
	}).Info("Domain created, verification required")

	return domain, nil
}

// GetDomain retrieves a domain by ID
func (s *Service) GetDomain(ctx context.Context, userID, domainID string) (*Domain, error) {
	query := `
		SELECT id, user_id, domain, is_default, ssl_enabled, ssl_cert_path, ssl_key_path,
		       verified, verification_token, verification_method, created_at, verified_at
		FROM domains
		WHERE id = ? AND user_id = ?`

	row := s.db.QueryRowContext(ctx, query, domainID, userID)

	domain := &Domain{}
	err := row.Scan(
		&domain.ID, &domain.UserID, &domain.Domain, &domain.IsDefault, &domain.SSLEnabled,
		&domain.SSLCertPath, &domain.SSLKeyPath, &domain.Verified, &domain.VerificationToken,
		&domain.VerificationMethod, &domain.CreatedAt, &domain.VerifiedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("domain not found: %w", err)
	}

	return domain, nil
}

// GetDomainByName retrieves a domain by domain name
func (s *Service) GetDomainByName(ctx context.Context, domainName string) (*Domain, error) {
	query := `
		SELECT id, user_id, domain, is_default, ssl_enabled, ssl_cert_path, ssl_key_path,
		       verified, verification_token, verification_method, created_at, verified_at
		FROM domains
		WHERE domain = ?`

	row := s.db.QueryRowContext(ctx, query, strings.ToLower(domainName))

	domain := &Domain{}
	err := row.Scan(
		&domain.ID, &domain.UserID, &domain.Domain, &domain.IsDefault, &domain.SSLEnabled,
		&domain.SSLCertPath, &domain.SSLKeyPath, &domain.Verified, &domain.VerificationToken,
		&domain.VerificationMethod, &domain.CreatedAt, &domain.VerifiedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("domain not found: %w", err)
	}

	return domain, nil
}

// ListDomains lists domains for a user with pagination
func (s *Service) ListDomains(ctx context.Context, userID string, page, pageSize int) (*DomainListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// Get total count
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM domains WHERE user_id = ?", userID).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("failed to count domains: %w", err)
	}

	// Get domains
	query := `
		SELECT id, user_id, domain, is_default, ssl_enabled, ssl_cert_path, ssl_key_path,
		       verified, verification_token, verification_method, created_at, verified_at
		FROM domains
		WHERE user_id = ?
		ORDER BY is_default DESC, created_at DESC
		LIMIT ? OFFSET ?`

	rows, err := s.db.QueryContext(ctx, query, userID, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query domains: %w", err)
	}
	defer rows.Close()

	var domains []Domain
	for rows.Next() {
		domain := Domain{}
		err := rows.Scan(
			&domain.ID, &domain.UserID, &domain.Domain, &domain.IsDefault, &domain.SSLEnabled,
			&domain.SSLCertPath, &domain.SSLKeyPath, &domain.Verified, &domain.VerificationToken,
			&domain.VerificationMethod, &domain.CreatedAt, &domain.VerifiedAt,
		)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to scan domain")
			continue
		}
		domains = append(domains, domain)
	}

	totalPages := (total + pageSize - 1) / pageSize

	return &DomainListResponse{
		Domains:    domains,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// UpdateDomain updates domain settings
func (s *Service) UpdateDomain(ctx context.Context, userID, domainID string, req *DomainUpdateRequest) (*Domain, error) {
	// Get existing domain
	domain, err := s.GetDomain(ctx, userID, domainID)
	if err != nil {
		return nil, err
	}

	// Build update query dynamically
	updates := []string{}
	args := []interface{}{}

	if req.IsDefault != nil {
		if *req.IsDefault {
			// Clear other default domains for this user
			if err := s.clearDefaultDomain(ctx, userID); err != nil {
				return nil, fmt.Errorf("failed to clear existing default domain: %w", err)
			}
		}
		updates = append(updates, "is_default = ?")
		args = append(args, *req.IsDefault)
		domain.IsDefault = *req.IsDefault
	}

	if req.SSLEnabled != nil {
		updates = append(updates, "ssl_enabled = ?")
		args = append(args, *req.SSLEnabled)
		domain.SSLEnabled = *req.SSLEnabled
	}

	if req.SSLCertPath != nil {
		updates = append(updates, "ssl_cert_path = ?")
		args = append(args, *req.SSLCertPath)
		domain.SSLCertPath = *req.SSLCertPath
	}

	if req.SSLKeyPath != nil {
		updates = append(updates, "ssl_key_path = ?")
		args = append(args, *req.SSLKeyPath)
		domain.SSLKeyPath = *req.SSLKeyPath
	}

	if req.VerificationMethod != nil {
		updates = append(updates, "verification_method = ?")
		args = append(args, *req.VerificationMethod)
		domain.VerificationMethod = *req.VerificationMethod

		// Generate new verification token
		token, err := s.generateVerificationToken()
		if err != nil {
			return nil, fmt.Errorf("failed to generate verification token: %w", err)
		}
		updates = append(updates, "verification_token = ?, verified = ?")
		args = append(args, token, false)
		domain.VerificationToken = token
		domain.Verified = false
		domain.VerifiedAt = nil
	}

	if len(updates) == 0 {
		return domain, nil // No updates needed
	}

	// Execute update
	query := fmt.Sprintf("UPDATE domains SET %s WHERE id = ? AND user_id = ?", strings.Join(updates, ", "))
	args = append(args, domainID, userID)

	_, err = s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update domain: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"domain_id": domainID,
		"domain":    domain.Domain,
		"user_id":   userID,
	}).Info("Domain updated")

	return domain, nil
}

// DeleteDomain deletes a custom domain
func (s *Service) DeleteDomain(ctx context.Context, userID, domainID string) error {
	// Check if domain exists and belongs to user
	domain, err := s.GetDomain(ctx, userID, domainID)
	if err != nil {
		return err
	}

	// Check if domain has URLs associated with it
	var urlCount int
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM urls WHERE domain_id = ?", domainID).Scan(&urlCount)
	if err != nil {
		return fmt.Errorf("failed to check domain usage: %w", err)
	}

	if urlCount > 0 {
		return fmt.Errorf("cannot delete domain with %d associated URLs", urlCount)
	}

	// Delete domain
	_, err = s.db.ExecContext(ctx, "DELETE FROM domains WHERE id = ? AND user_id = ?", domainID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete domain: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"domain_id": domainID,
		"domain":    domain.Domain,
		"user_id":   userID,
	}).Info("Domain deleted")

	return nil
}

// VerifyDomain initiates domain verification
func (s *Service) VerifyDomain(ctx context.Context, userID, domainID string) (*VerificationStatus, error) {
	domain, err := s.GetDomain(ctx, userID, domainID)
	if err != nil {
		return nil, err
	}

	if domain.Verified {
		return &VerificationStatus{
			Domain:    domain.Domain,
			Verified:  true,
			Method:    domain.VerificationMethod,
			LastCheck: time.Now(),
			NextCheck: time.Now().Add(24 * time.Hour),
		}, nil
	}

	return s.verification.VerifyDomain(ctx, domain)
}

// GetDomainStats retrieves domain usage statistics
func (s *Service) GetDomainStats(ctx context.Context, userID, domainID string, days int) (*DomainStats, error) {
	domain, err := s.GetDomain(ctx, userID, domainID)
	if err != nil {
		return nil, err
	}

	stats := &DomainStats{
		DomainID: domainID,
		Domain:   domain.Domain,
	}

	// Calculate date ranges
	now := time.Now()
	since := now.AddDate(0, 0, -days)
	last24h := now.Add(-24 * time.Hour)

	// Get URL count
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM urls WHERE domain_id = ?", domainID).Scan(&stats.URLCount)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get URL count for domain")
	}

	// Get total clicks
	query := `
		SELECT COALESCE(SUM(u.clicks), 0), COALESCE(SUM(u.unique_clicks), 0)
		FROM urls u
		WHERE u.domain_id = ? AND u.created_at >= ?`

	err = s.db.QueryRowContext(ctx, query, domainID, since).Scan(&stats.TotalClicks, &stats.UniqueClicks)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get click statistics for domain")
	}

	// Get last used time
	var lastUsed *time.Time
	query = `
		SELECT MAX(c.clicked_at)
		FROM clicks c
		JOIN urls u ON c.url_id = u.id
		WHERE u.domain_id = ?`

	err = s.db.QueryRowContext(ctx, query, domainID).Scan(&lastUsed)
	if err == nil && lastUsed != nil {
		stats.LastUsed = lastUsed
	}

	// Get 24h statistics
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM urls WHERE domain_id = ? AND created_at >= ?", domainID, last24h).Scan(&stats.CreatedURLs24h)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get 24h URL creation count")
	}

	query = `
		SELECT COALESCE(COUNT(*), 0)
		FROM clicks c
		JOIN urls u ON c.url_id = u.id
		WHERE u.domain_id = ? AND c.clicked_at >= ?`

	err = s.db.QueryRowContext(ctx, query, domainID, last24h).Scan(&stats.Clicks24h)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get 24h click count")
	}

	return stats, nil
}

// GetDefaultDomain gets the default domain for a user
func (s *Service) GetDefaultDomain(ctx context.Context, userID string) (*Domain, error) {
	query := `
		SELECT id, user_id, domain, is_default, ssl_enabled, ssl_cert_path, ssl_key_path,
		       verified, verification_token, verification_method, created_at, verified_at
		FROM domains
		WHERE user_id = ? AND is_default = true
		LIMIT 1`

	row := s.db.QueryRowContext(ctx, query, userID)

	domain := &Domain{}
	err := row.Scan(
		&domain.ID, &domain.UserID, &domain.Domain, &domain.IsDefault, &domain.SSLEnabled,
		&domain.SSLCertPath, &domain.SSLKeyPath, &domain.Verified, &domain.VerificationToken,
		&domain.VerificationMethod, &domain.CreatedAt, &domain.VerifiedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("no default domain found: %w", err)
	}

	return domain, nil
}

// validateDomainName validates domain name format
func (s *Service) validateDomainName(domain string) error {
	if len(domain) < 3 || len(domain) > 253 {
		return fmt.Errorf("domain length must be between 3 and 253 characters")
	}

	// Basic domain format validation
	if strings.Contains(domain, "..") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return fmt.Errorf("invalid domain format")
	}

	// Check for valid characters
	for _, char := range domain {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			 (char >= '0' && char <= '9') || char == '.' || char == '-') {
			return fmt.Errorf("domain contains invalid characters")
		}
	}

	// Check for reserved domains
	reservedDomains := []string{
		"localhost", "127.0.0.1", "::1",
		"api", "www", "admin", "app", "dashboard",
		"mail", "email", "ftp", "ssh", "ssl", "tls",
	}

	lowerDomain := strings.ToLower(domain)
	for _, reserved := range reservedDomains {
		if lowerDomain == reserved || strings.HasPrefix(lowerDomain, reserved+".") {
			return fmt.Errorf("domain is reserved and cannot be used")
		}
	}

	return nil
}

// domainExists checks if a domain already exists
func (s *Service) domainExists(ctx context.Context, domain string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM domains WHERE domain = ?", strings.ToLower(domain)).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// clearDefaultDomain clears the default flag from all domains for a user
func (s *Service) clearDefaultDomain(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE domains SET is_default = false WHERE user_id = ?", userID)
	return err
}

// generateDomainID generates a unique domain ID
func (s *Service) generateDomainID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// generateVerificationToken generates a verification token
func (s *Service) generateVerificationToken() (string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}