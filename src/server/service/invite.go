package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/webappsgo/caslink/src/server/store"
)

// Invite kinds. A single shared invite subsystem backs admin-registration
// invites (PART 17), user-registration invites (PART 34), and org-membership
// invites (PART 35). The kind discriminates which acceptance flow may consume
// a given invite.
const (
	InviteKindAdminRegistration = "admin_registration"
	InviteKindUserRegistration  = "user_registration"
	InviteKindOrgMembership     = "org_membership"
)

// InviteDefaultTTL is the default lifetime of a new invite (PART 17/34/35: 7 days).
const InviteDefaultTTL = 7 * 24 * time.Hour

// Invite-related sentinel errors.
var (
	ErrInviteInvalid      = errors.New("invite is invalid or does not exist")
	ErrInviteExpired      = errors.New("invite has expired")
	ErrInviteUsed         = errors.New("invite has already been used")
	ErrInviteRevoked      = errors.New("invite has been revoked")
	ErrInviteKindMismatch = errors.New("invite is not valid for this action")
	ErrInviteBadKind      = errors.New("unknown invite kind")
)

// InviteService issues, validates, and consumes single-use invite tokens.
type InviteService struct {
	store *store.Store
}

// NewInviteService constructs an InviteService.
func NewInviteService(st *store.Store) *InviteService {
	return &InviteService{store: st}
}

// Invite is a stored invite record. The plaintext token is never persisted and
// is returned only once, at creation time.
type Invite struct {
	ID        int64
	Kind      string
	Email     string
	OrgID     int64
	Role      string
	CreatedBy int64
	CreatedAt time.Time
	ExpiresAt time.Time
	MaxUses   int
	UseCount  int
	Revoked   bool
	UsedAt    *time.Time
	UsedBy    int64
}

// CreateInviteParams describes a new invite to issue.
type CreateInviteParams struct {
	// Kind is one of the InviteKind* constants (required).
	Kind string
	// Email optionally binds the invite to a specific recipient (may be empty).
	Email string
	// OrgID scopes org-membership invites; 0 stores NULL (no org).
	OrgID int64
	// Role is the role granted on acceptance (org-membership invites).
	Role string
	// CreatedBy is the actor id that issued the invite (0 = system).
	CreatedBy int64
	// TTL overrides the default 7-day lifetime when > 0.
	TTL time.Duration
	// MaxUses caps how many times the invite may be consumed. 0 applies the
	// single-use default (1); a negative value stores 0, meaning unlimited.
	MaxUses int
}

// CreateInvite issues a new invite and returns the one-time plaintext token
// alongside the stored record. The plaintext is never persisted; only its
// SHA-256 hash is stored.
func (s *InviteService) CreateInvite(ctx context.Context, p CreateInviteParams) (string, *Invite, error) {
	switch p.Kind {
	case InviteKindAdminRegistration, InviteKindUserRegistration, InviteKindOrgMembership:
	default:
		return "", nil, ErrInviteBadKind
	}

	ttl := p.TTL
	if ttl <= 0 {
		ttl = InviteDefaultTTL
	}

	maxUses := p.MaxUses
	switch {
	case maxUses == 0:
		maxUses = 1
	case maxUses < 0:
		maxUses = 0
	}

	plaintext, err := generateTokenRandom()
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate invite token: %w", err)
	}
	tokenHash := hashAPIToken(plaintext)

	now := time.Now()
	expiresAt := now.Add(ttl)
	email := strings.ToLower(strings.TrimSpace(p.Email))

	var orgID any
	if p.OrgID > 0 {
		orgID = p.OrgID
	}

	res, err := s.store.UsersDB.ExecContext(ctx,
		`INSERT INTO invites (token_hash, kind, email, org_id, role, created_by, expires_at, max_uses)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tokenHash, p.Kind, email, orgID, p.Role, p.CreatedBy, expiresAt.Unix(), maxUses,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create invite: %w", err)
	}
	id, _ := res.LastInsertId()

	inv := &Invite{
		ID:        id,
		Kind:      p.Kind,
		Email:     email,
		OrgID:     p.OrgID,
		Role:      p.Role,
		CreatedBy: p.CreatedBy,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		MaxUses:   maxUses,
		UseCount:  0,
		Revoked:   false,
	}
	return plaintext, inv, nil
}

// getByHash loads an invite by its token hash.
func (s *InviteService) getByHash(ctx context.Context, tokenHash string) (*Invite, error) {
	var (
		inv       Invite
		orgID     sql.NullInt64
		createdAt int64
		expiresAt int64
		revoked   int64
		usedAt    sql.NullInt64
		usedBy    sql.NullInt64
	)
	err := s.store.UsersDB.QueryRowContext(ctx,
		`SELECT id, kind, email, org_id, role, created_by, created_at, expires_at,
		        max_uses, use_count, revoked, used_at, used_by
		 FROM invites WHERE token_hash = ?`, tokenHash).
		Scan(&inv.ID, &inv.Kind, &inv.Email, &orgID, &inv.Role, &inv.CreatedBy,
			&createdAt, &expiresAt, &inv.MaxUses, &inv.UseCount, &revoked, &usedAt, &usedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load invite: %w", err)
	}
	inv.OrgID = orgID.Int64
	inv.CreatedAt = time.Unix(createdAt, 0)
	inv.ExpiresAt = time.Unix(expiresAt, 0)
	inv.Revoked = revoked != 0
	if usedAt.Valid {
		t := time.Unix(usedAt.Int64, 0)
		inv.UsedAt = &t
	}
	inv.UsedBy = usedBy.Int64
	return &inv, nil
}

// checkUsable returns the appropriate sentinel error if the invite cannot be
// consumed, or nil if it is currently valid.
func (inv *Invite) checkUsable(now time.Time) error {
	if inv.Revoked {
		return ErrInviteRevoked
	}
	if !now.Before(inv.ExpiresAt) {
		return ErrInviteExpired
	}
	if inv.MaxUses != 0 && inv.UseCount >= inv.MaxUses {
		return ErrInviteUsed
	}
	return nil
}

// ValidateInvite loads an invite by plaintext token and checks that it is
// currently consumable. When kind is non-empty it must match the invite's kind.
// It does NOT consume the invite.
func (s *InviteService) ValidateInvite(ctx context.Context, plaintext, kind string) (*Invite, error) {
	inv, err := s.getByHash(ctx, hashAPIToken(plaintext))
	if err != nil {
		return nil, err
	}
	if kind != "" && inv.Kind != kind {
		return nil, ErrInviteKindMismatch
	}
	if err := inv.checkUsable(time.Now()); err != nil {
		return nil, err
	}
	return inv, nil
}

// ConsumeInvite atomically records one use of an invite, returning the updated
// record. It is race-safe: concurrent consumers of a single-use invite will see
// exactly one success; the rest receive ErrInviteUsed. When kind is non-empty it
// must match the invite's kind. usedBy is the id of the account created/joined.
func (s *InviteService) ConsumeInvite(ctx context.Context, plaintext, kind string, usedBy int64) (*Invite, error) {
	tokenHash := hashAPIToken(plaintext)
	inv, err := s.getByHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if kind != "" && inv.Kind != kind {
		return nil, ErrInviteKindMismatch
	}
	now := time.Now()
	if err := inv.checkUsable(now); err != nil {
		return nil, err
	}

	res, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE invites
		 SET use_count = use_count + 1,
		     used_at = COALESCE(used_at, ?),
		     used_by = ?
		 WHERE token_hash = ?
		   AND revoked = 0
		   AND expires_at > ?
		   AND (max_uses = 0 OR use_count < max_uses)`,
		now.Unix(), usedBy, tokenHash, now.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to consume invite: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Lost a race, or state changed between load and update. Re-read to
		// report the precise reason.
		if fresh, ferr := s.getByHash(ctx, tokenHash); ferr == nil {
			if cerr := fresh.checkUsable(time.Now()); cerr != nil {
				return nil, cerr
			}
		}
		return nil, ErrInviteUsed
	}

	inv.UseCount++
	if inv.UsedAt == nil {
		inv.UsedAt = &now
	}
	inv.UsedBy = usedBy
	return inv, nil
}

// RevokeInvite marks a specific invite revoked so it can no longer be consumed.
func (s *InviteService) RevokeInvite(ctx context.Context, id int64) error {
	_, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE invites SET revoked = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to revoke invite: %w", err)
	}
	return nil
}

// RevokeUnusedByKind revokes every not-yet-consumed invite of a given kind.
// Used when a registration/org-creation mode changes to "disabled", which must
// invalidate outstanding invite links (PART 34/35). Returns the count revoked.
func (s *InviteService) RevokeUnusedByKind(ctx context.Context, kind string) (int64, error) {
	res, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE invites SET revoked = 1 WHERE kind = ? AND revoked = 0 AND use_count = 0`, kind)
	if err != nil {
		return 0, fmt.Errorf("failed to revoke invites: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// CleanupExpired deletes invites whose expiry has passed. Invoked by the
// scheduler's token-cleanup task. Returns the number of rows removed.
func (s *InviteService) CleanupExpired(ctx context.Context) (int64, error) {
	res, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM invites WHERE expires_at < ?`, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("failed to clean up expired invites: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ListInvitesByKind returns non-revoked, unexpired invites of a kind, newest
// first, for admin display. org scoping is applied when orgID > 0.
func (s *InviteService) ListInvitesByKind(ctx context.Context, kind string, orgID int64) ([]*Invite, error) {
	query := `SELECT id, kind, email, org_id, role, created_by, created_at, expires_at,
	                 max_uses, use_count, revoked, used_at, used_by
	          FROM invites WHERE kind = ? AND revoked = 0 AND expires_at > ?`
	args := []any{kind, time.Now().Unix()}
	if orgID > 0 {
		query += ` AND org_id = ?`
		args = append(args, orgID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.store.UsersDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list invites: %w", err)
	}
	defer rows.Close()

	var invites []*Invite
	for rows.Next() {
		var (
			inv       Invite
			orgIDCol  sql.NullInt64
			createdAt int64
			expiresAt int64
			revoked   int64
			usedAt    sql.NullInt64
			usedBy    sql.NullInt64
		)
		if err := rows.Scan(&inv.ID, &inv.Kind, &inv.Email, &orgIDCol, &inv.Role,
			&inv.CreatedBy, &createdAt, &expiresAt, &inv.MaxUses, &inv.UseCount,
			&revoked, &usedAt, &usedBy); err != nil {
			return nil, fmt.Errorf("failed to scan invite: %w", err)
		}
		inv.OrgID = orgIDCol.Int64
		inv.CreatedAt = time.Unix(createdAt, 0)
		inv.ExpiresAt = time.Unix(expiresAt, 0)
		inv.Revoked = revoked != 0
		if usedAt.Valid {
			t := time.Unix(usedAt.Int64, 0)
			inv.UsedAt = &t
		}
		inv.UsedBy = usedBy.Int64
		invites = append(invites, &inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate invites: %w", err)
	}
	return invites, nil
}
