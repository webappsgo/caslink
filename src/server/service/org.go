package service

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// OrgService handles organization operations
type OrgService struct {
	store *store.Store
}

// NewOrgService creates a new organization service
func NewOrgService(st *store.Store) *OrgService {
	return &OrgService{
		store: st,
	}
}

// Organization represents an organization
type Organization struct {
	ID        int64
	Name      string
	Slug      string
	OwnerID   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// OrgMember represents an organization member
type OrgMember struct {
	ID       int64
	OrgID    int64
	UserID   int64
	Role     string
	JoinedAt time.Time
}

// CreateOrganization creates a new organization
func (s *OrgService) CreateOrganization(ctx context.Context, userID int64, name, slug string) (*Organization, error) {
	// Generate slug if not provided
	if slug == "" {
		slug = generateSlug(name)
	}

	// Validate slug format
	if !isValidSlug(slug) {
		return nil, fmt.Errorf("invalid organization slug")
	}

	// Check if slug already exists
	var count int
	err := s.store.UsersDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM organizations WHERE slug = ?", slug).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check slug: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("organization slug already exists")
	}

	// Insert organization
	query := `INSERT INTO organizations (name, slug, owner_id, created_at, updated_at)
	          VALUES (?, ?, ?, ?, ?)`

	result, err := s.store.UsersDB.ExecContext(ctx, query, name, slug, userID, time.Now(), time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	orgID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get org ID: %w", err)
	}

	// Add owner as member with owner role
	memberQuery := `INSERT INTO org_members (org_id, user_id, role, joined_at)
	                VALUES (?, ?, 'owner', ?)`

	_, err = s.store.UsersDB.ExecContext(ctx, memberQuery, orgID, userID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to add owner as member: %w", err)
	}

	// Return created organization
	org := &Organization{
		ID:        orgID,
		Name:      name,
		Slug:      slug,
		OwnerID:   userID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return org, nil
}

// GetUserOrganizations gets all organizations for a user
func (s *OrgService) GetUserOrganizations(ctx context.Context, userID int64) ([]*Organization, error) {
	query := `SELECT o.id, o.name, o.slug, o.owner_id, o.created_at, o.updated_at
	          FROM organizations o
	          JOIN org_members m ON o.id = m.org_id
	          WHERE m.user_id = ?
	          ORDER BY o.created_at DESC`

	rows, err := s.store.UsersDB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query organizations: %w", err)
	}
	defer rows.Close()

	var orgs []*Organization
	for rows.Next() {
		var org Organization
		err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.OwnerID, &org.CreatedAt, &org.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan organization: %w", err)
		}
		orgs = append(orgs, &org)
	}

	return orgs, nil
}

// GetOrganizationBySlug gets an organization by slug
func (s *OrgService) GetOrganizationBySlug(ctx context.Context, slug string) (*Organization, error) {
	query := `SELECT id, name, slug, owner_id, created_at, updated_at
	          FROM organizations
	          WHERE slug = ?`

	var org Organization
	err := s.store.UsersDB.QueryRowContext(ctx, query, slug).Scan(
		&org.ID, &org.Name, &org.Slug, &org.OwnerID, &org.CreatedAt, &org.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("organization not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query organization: %w", err)
	}

	return &org, nil
}

// GetOrgMembers gets all members of an organization
func (s *OrgService) GetOrgMembers(ctx context.Context, orgID int64) ([]*OrgMember, error) {
	query := `SELECT id, org_id, user_id, role, joined_at
	          FROM org_members
	          WHERE org_id = ?
	          ORDER BY joined_at ASC`

	rows, err := s.store.UsersDB.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query members: %w", err)
	}
	defer rows.Close()

	var members []*OrgMember
	for rows.Next() {
		var member OrgMember
		err := rows.Scan(&member.ID, &member.OrgID, &member.UserID, &member.Role, &member.JoinedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, &member)
	}

	return members, nil
}

// IsMember checks if a user is a member of an organization
func (s *OrgService) IsMember(ctx context.Context, orgID, userID int64) (bool, string, error) {
	query := `SELECT role FROM org_members WHERE org_id = ? AND user_id = ?`

	var role string
	err := s.store.UsersDB.QueryRowContext(ctx, query, orgID, userID).Scan(&role)

	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("failed to check membership: %w", err)
	}

	return true, role, nil
}

// generateSlug generates a URL-friendly slug from a name
func generateSlug(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces and special chars with hyphens
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Limit length to 50 chars
	if len(slug) > 50 {
		slug = slug[:50]
	}

	return slug
}

// isValidSlug checks if a slug is valid (3-50 chars, lowercase alphanumeric + hyphens)
func isValidSlug(slug string) bool {
	if len(slug) < 3 || len(slug) > 50 {
		return false
	}

	validSlugRegex := regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)
	return validSlugRegex.MatchString(slug)
}
