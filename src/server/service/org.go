package service

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/webappsgo/caslink/src/server/store"
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

	// Insert the organization and its owner membership in a single
	// transaction so a failure mid-way cannot leave an orphaned org with
	// no owner (and a permanently-taken slug).
	tx, err := s.store.UsersDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := `INSERT INTO organizations (name, slug, owner_id, created_at, updated_at)
	          VALUES (?, ?, ?, ?, ?)`

	result, err := tx.ExecContext(ctx, query, name, slug, userID, time.Now(), time.Now())
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

	if _, err = tx.ExecContext(ctx, memberQuery, orgID, userID, time.Now()); err != nil {
		return nil, fmt.Errorf("failed to add owner as member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit organization: %w", err)
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

// OrgSummary is an organization plus the requesting user's role and the
// org's total member count, fetched in one query to avoid N+1 lookups.
type OrgSummary struct {
	Organization
	Role        string
	MemberCount int
}

// GetUserOrganizationsWithSummary returns every organization the user
// belongs to, along with their role and the org's member count, in a
// single query (a per-org member-count/role subquery would be N+1).
func (s *OrgService) GetUserOrganizationsWithSummary(ctx context.Context, userID int64) ([]*OrgSummary, error) {
	query := `SELECT o.id, o.name, o.slug, o.owner_id, o.created_at, o.updated_at, m.role,
	                 (SELECT COUNT(*) FROM org_members WHERE org_id = o.id) AS member_count
	          FROM organizations o
	          JOIN org_members m ON o.id = m.org_id
	          WHERE m.user_id = ?
	          ORDER BY o.created_at DESC`

	rows, err := s.store.UsersDB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query organizations: %w", err)
	}
	defer rows.Close()

	var summaries []*OrgSummary
	for rows.Next() {
		var sum OrgSummary
		if err := rows.Scan(&sum.ID, &sum.Name, &sum.Slug, &sum.OwnerID, &sum.CreatedAt, &sum.UpdatedAt,
			&sum.Role, &sum.MemberCount); err != nil {
			return nil, fmt.Errorf("failed to scan organization summary: %w", err)
		}
		summaries = append(summaries, &sum)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("organization summary row iteration failed: %w", err)
	}

	return summaries, nil
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

// OrgMemberDetail joins org_members with users to include the username.
type OrgMemberDetail struct {
	ID       int64
	OrgID    int64
	UserID   int64
	Username string
	Role     string
	JoinedAt time.Time
}

// GetMembersWithUsernames returns org members including their usernames.
func (s *OrgService) GetMembersWithUsernames(ctx context.Context, orgID int64) ([]*OrgMemberDetail, error) {
	query := `SELECT m.id, m.org_id, m.user_id, u.username, m.role, m.joined_at
	          FROM org_members m
	          JOIN users u ON u.id = m.user_id
	          WHERE m.org_id = ?
	          ORDER BY m.joined_at ASC`

	rows, err := s.store.UsersDB.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query members: %w", err)
	}
	defer rows.Close()

	var members []*OrgMemberDetail
	for rows.Next() {
		var m OrgMemberDetail
		if err := rows.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Username, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("member row iteration failed: %w", err)
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

// OrgToken represents an API token scoped to an organisation.
type OrgToken struct {
	ID          int64      `json:"id"`
	OrgID       int64      `json:"org_id"`
	CreatedBy   int64      `json:"created_by"`
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	Active      bool       `json:"active"`
}

// CreateOrgToken creates a new org-scoped API token (org_ prefix).
// Uses the unified tokens table per AI.md PART 11.
// Only org owners and admins may create tokens (caller must enforce this).
// Returns the saved OrgToken and the single-use plain-text token.
// The plain token is NOT stored — only its SHA-256 hex digest is persisted.
func (s *OrgService) CreateOrgToken(ctx context.Context, orgID, createdBy int64, name string, permissions []string) (*OrgToken, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	plainToken, tokenHash, displayPrefix, err := generateOrgToken()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate token: %w", err)
	}

	scope := "global"
	if len(permissions) > 0 {
		scope = permissions[0]
	}
	if name == "" {
		name = "default"
	}

	now := time.Now()
	res, err := s.store.UsersDB.ExecContext(ctx,
		`INSERT INTO tokens (owner_type, owner_id, name, token_hash, token_prefix, scope, created_at)
		 VALUES ('org', ?, ?, ?, ?, ?, ?)`,
		orgID, name, tokenHash, displayPrefix, scope, now,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to insert org token: %w", err)
	}

	id, _ := res.LastInsertId()
	tok := &OrgToken{
		ID:          id,
		OrgID:       orgID,
		CreatedBy:   createdBy,
		Name:        name,
		Permissions: permissions,
		CreatedAt:   now,
		Active:      true,
	}
	return tok, plainToken, nil
}

// ListOrgTokens returns all active tokens for the given org.
// Reads from the unified tokens table per AI.md PART 11.
func (s *OrgService) ListOrgTokens(ctx context.Context, orgID int64) ([]*OrgToken, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.store.UsersDB.QueryContext(ctx,
		`SELECT id, owner_id, name, scope, created_at, expires_at, last_used_at
		 FROM tokens WHERE owner_type = 'org' AND owner_id = ? ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query org tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*OrgToken
	for rows.Next() {
		var t OrgToken
		var expiresAt, lastUsedAt sql.NullTime
		var scope string
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &scope,
			&t.CreatedAt, &expiresAt, &lastUsedAt); err != nil {
			return nil, fmt.Errorf("failed to scan org token: %w", err)
		}
		if scope != "" {
			t.Permissions = []string{scope}
		}
		if expiresAt.Valid {
			t.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			t.LastUsedAt = &lastUsedAt.Time
		}
		t.Active = true
		tokens = append(tokens, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("org token row error: %w", err)
	}
	return tokens, nil
}

// RevokeOrgToken deletes an org token from the unified tokens table.
// tokenID must belong to orgID — callers must ensure this to prevent IDOR.
func (s *OrgService) RevokeOrgToken(ctx context.Context, tokenID, orgID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM tokens WHERE id = ? AND owner_type = 'org' AND owner_id = ?`,
		tokenID, orgID,
	)
	if err != nil {
		return fmt.Errorf("failed to revoke org token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("token not found")
	}
	return nil
}

// TransferOwnership transfers organisation ownership from currentOwnerID to
// newOwnerID. Both users must already be members. The old owner's role is
// downgraded to 'admin' to preserve their membership. All role changes are
// performed atomically inside a transaction.
func (s *OrgService) TransferOwnership(ctx context.Context, orgID, currentOwnerID, newOwnerID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.store.UsersDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Confirm the org exists and currentOwnerID is the owner.
	var ownerID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT owner_id FROM organizations WHERE id = ?`, orgID,
	).Scan(&ownerID); err == sql.ErrNoRows {
		return fmt.Errorf("organization not found")
	} else if err != nil {
		return fmt.Errorf("failed to load organization: %w", err)
	}
	if ownerID != currentOwnerID {
		return fmt.Errorf("caller is not the organization owner")
	}

	// New owner must already be a member.
	var newRole string
	if err := tx.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id = ? AND user_id = ?`, orgID, newOwnerID,
	).Scan(&newRole); err == sql.ErrNoRows {
		return fmt.Errorf("new owner must already be an organization member")
	} else if err != nil {
		return fmt.Errorf("failed to check new owner membership: %w", err)
	}

	// Demote the current owner to admin.
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_members SET role = 'admin' WHERE org_id = ? AND user_id = ?`,
		orgID, currentOwnerID,
	); err != nil {
		return fmt.Errorf("failed to demote current owner: %w", err)
	}

	// Promote the new owner.
	if _, err := tx.ExecContext(ctx,
		`UPDATE org_members SET role = 'owner' WHERE org_id = ? AND user_id = ?`,
		orgID, newOwnerID,
	); err != nil {
		return fmt.Errorf("failed to promote new owner: %w", err)
	}

	// Update the organizations.owner_id column.
	if _, err := tx.ExecContext(ctx,
		`UPDATE organizations SET owner_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newOwnerID, orgID,
	); err != nil {
		return fmt.Errorf("failed to update organization owner: %w", err)
	}

	return tx.Commit()
}

// RemoveMember removes a user from an organization. The organization's
// owner cannot be removed this way — callers must use TransferOwnership
// first per AI.md PART 35.
func (s *OrgService) RemoveMember(ctx context.Context, orgID, userID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var role string
	if err := s.store.UsersDB.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id = ? AND user_id = ?`, orgID, userID,
	).Scan(&role); err == sql.ErrNoRows {
		return fmt.Errorf("member not found")
	} else if err != nil {
		return fmt.Errorf("failed to load member: %w", err)
	}
	if role == "owner" {
		return fmt.Errorf("cannot remove the organization owner — transfer ownership first")
	}

	if _, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM org_members WHERE org_id = ? AND user_id = ?`, orgID, userID,
	); err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}

	return nil
}

// ChangeMemberRole updates a member's role. Only "member" and "admin" are
// valid targets here — promoting to "owner" must go through
// TransferOwnership so organizations.owner_id stays consistent.
func (s *OrgService) ChangeMemberRole(ctx context.Context, orgID, userID int64, newRole string) error {
	if newRole != "member" && newRole != "admin" {
		return fmt.Errorf("invalid role %q — must be \"member\" or \"admin\"", newRole)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var currentRole string
	if err := s.store.UsersDB.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id = ? AND user_id = ?`, orgID, userID,
	).Scan(&currentRole); err == sql.ErrNoRows {
		return fmt.Errorf("member not found")
	} else if err != nil {
		return fmt.Errorf("failed to load member: %w", err)
	}
	if currentRole == "owner" {
		return fmt.Errorf("cannot change the owner's role — transfer ownership first")
	}

	if _, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE org_members SET role = ? WHERE org_id = ? AND user_id = ?`,
		newRole, orgID, userID,
	); err != nil {
		return fmt.Errorf("failed to change member role: %w", err)
	}

	return nil
}

// AddMemberByEmail looks up an existing user account by email and adds them
// directly to the organization. Returns an error if no account exists with
// that email (token-based email invitations for accounts that don't yet
// exist are tracked separately — see TODO.AI.md PART 35) or if the user is
// already a member.
func (s *OrgService) AddMemberByEmail(ctx context.Context, orgID int64, email, role string) error {
	if role != "member" && role != "admin" {
		role = "member"
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var userID int64
	if err := s.store.UsersDB.QueryRowContext(ctx,
		`SELECT id FROM users WHERE email = ?`, email,
	).Scan(&userID); err == sql.ErrNoRows {
		return fmt.Errorf("no account found with email %q", email)
	} else if err != nil {
		return fmt.Errorf("failed to look up user: %w", err)
	}

	var existingRole string
	err := s.store.UsersDB.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id = ? AND user_id = ?`, orgID, userID,
	).Scan(&existingRole)
	if err == nil {
		return fmt.Errorf("user is already a member of this organization")
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing membership: %w", err)
	}

	if _, err := s.store.UsersDB.ExecContext(ctx,
		`INSERT INTO org_members (org_id, user_id, role, joined_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		orgID, userID, role,
	); err != nil {
		return fmt.Errorf("failed to add member: %w", err)
	}

	return nil
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
