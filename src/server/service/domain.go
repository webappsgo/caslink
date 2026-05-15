package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// DomainService handles custom domain operations
type DomainService struct {
	store *store.Store
}

// NewDomainService creates a new domain service
func NewDomainService(st *store.Store) *DomainService {
	return &DomainService{
		store: st,
	}
}

// CustomDomain represents a custom domain
type CustomDomain struct {
	ID                 int64
	OwnerType          string
	OwnerID            int64
	Domain             string
	IsApex             bool
	IsWildcard         bool
	VerificationStatus string
	VerifiedAt         *time.Time
	VerifiedIP         *string
	LastCheckAt        *time.Time
	CheckCount         int
	SSLEnabled         bool
	SSLStatus          string
	SSLExpiresAt       *time.Time
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// AddDomain adds a new custom domain for a user or organization
func (s *DomainService) AddDomain(ctx context.Context, ownerType string, ownerID int64, domain string) (*CustomDomain, error) {
	// Check if domain already exists
	var count int
	err := s.store.UsersDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM custom_domains WHERE domain = ?", domain).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check domain: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("domain already exists")
	}

	// Determine if apex or subdomain
	isApex := !strings.Contains(domain, ".")

	// Insert domain
	query := `INSERT INTO custom_domains (
		owner_type, owner_id, domain, is_apex, is_wildcard,
		verification_status, ssl_status, status, created_at, updated_at
	) VALUES (?, ?, ?, ?, 0, 'pending', 'none', 'pending', ?, ?)`

	result, err := s.store.UsersDB.ExecContext(ctx, query,
		ownerType, ownerID, domain, isApex, time.Now(), time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to add domain: %w", err)
	}

	domainID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get domain ID: %w", err)
	}

	// Return created domain
	cd := &CustomDomain{
		ID:                 domainID,
		OwnerType:          ownerType,
		OwnerID:            ownerID,
		Domain:             domain,
		IsApex:             isApex,
		IsWildcard:         false,
		VerificationStatus: "pending",
		SSLEnabled:         false,
		SSLStatus:          "none",
		Status:             "pending",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	return cd, nil
}

// GetUserDomains gets all domains for a user
func (s *DomainService) GetUserDomains(ctx context.Context, userID int64) ([]*CustomDomain, error) {
	query := `SELECT id, owner_type, owner_id, domain, is_apex, is_wildcard,
	          verification_status, verified_at, ssl_enabled, ssl_status, ssl_expires_at,
	          status, created_at, updated_at
	          FROM custom_domains
	          WHERE owner_type = 'user' AND owner_id = ?
	          ORDER BY created_at DESC`

	rows, err := s.store.UsersDB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query domains: %w", err)
	}
	defer rows.Close()

	var domains []*CustomDomain
	for rows.Next() {
		var d CustomDomain
		err := rows.Scan(
			&d.ID, &d.OwnerType, &d.OwnerID, &d.Domain, &d.IsApex, &d.IsWildcard,
			&d.VerificationStatus, &d.VerifiedAt, &d.SSLEnabled, &d.SSLStatus, &d.SSLExpiresAt,
			&d.Status, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan domain: %w", err)
		}
		domains = append(domains, &d)
	}

	return domains, nil
}

// GetOrgDomains gets all domains for an organization
func (s *DomainService) GetOrgDomains(ctx context.Context, orgID int64) ([]*CustomDomain, error) {
	query := `SELECT id, owner_type, owner_id, domain, is_apex, is_wildcard,
	          verification_status, verified_at, ssl_enabled, ssl_status, ssl_expires_at,
	          status, created_at, updated_at
	          FROM custom_domains
	          WHERE owner_type = 'org' AND owner_id = ?
	          ORDER BY created_at DESC`

	rows, err := s.store.UsersDB.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query domains: %w", err)
	}
	defer rows.Close()

	var domains []*CustomDomain
	for rows.Next() {
		var d CustomDomain
		err := rows.Scan(
			&d.ID, &d.OwnerType, &d.OwnerID, &d.Domain, &d.IsApex, &d.IsWildcard,
			&d.VerificationStatus, &d.VerifiedAt, &d.SSLEnabled, &d.SSLStatus, &d.SSLExpiresAt,
			&d.Status, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan domain: %w", err)
		}
		domains = append(domains, &d)
	}

	return domains, nil
}

// VerifyDomain performs domain verification
func (s *DomainService) VerifyDomain(ctx context.Context, domainID int64) error {
	// Pending (tracked in TODO.AI.md): Implement DNS verification logic per PART 35
	// For now, just mark as verified (placeholder)
	query := `UPDATE custom_domains
	          SET verification_status = 'verified',
	              verified_at = ?,
	              status = 'active',
	              updated_at = ?
	          WHERE id = ?`

	_, err := s.store.UsersDB.ExecContext(ctx, query, time.Now(), time.Now(), domainID)
	if err != nil {
		return fmt.Errorf("failed to update domain: %w", err)
	}

	return nil
}
