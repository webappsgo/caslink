package service

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/webappsgo/caslink/src/server/store"
)

// joinTokenPrefix is the fixed prefix for cluster join tokens (AI.md PART 34
// "Token Rules": format node_{random_32_chars}).
const joinTokenPrefix = "node_"

// joinTokenTTL is how long a freshly generated join token remains valid
// (AI.md PART 34: valid for 15 minutes).
const joinTokenTTL = 15 * time.Minute

// joinTokenLockout is the reuse-lockout window applied once a token is consumed
// (AI.md PART 34: 90-day lockout prevents replay attacks).
const joinTokenLockout = 90 * 24 * time.Hour

// JoinTokenRecord is the display-safe view of a stored cluster join token.
// It deliberately never carries the plaintext or the SHA-256 hash.
type JoinTokenRecord struct {
	ID           string
	Label        string
	TokenPrefix  string
	CreatedBy    string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	UsedAt       *time.Time
	UsedBy       string
	LockoutUntil *time.Time
}

// Status returns a human-readable lifecycle state for display.
func (r JoinTokenRecord) Status() string {
	if r.UsedAt != nil {
		return "used"
	}
	if time.Now().After(r.ExpiresAt) {
		return "expired"
	}
	return "pending"
}

// ClusterService issues and validates cluster join tokens (AI.md PART 10 /
// PART 34). Tokens live in server.db (srv_cluster_join_tokens); only the
// SHA-256 hash is stored. This service covers the token-issuance and
// token-consumption halves of the join flow; the remote-database bootstrap
// handshake that actually migrates a node onto the shared cluster DB is a
// separate, infrastructure-gated concern.
type ClusterService struct {
	store *store.Store
}

// NewClusterService creates a new cluster service.
func NewClusterService(st *store.Store) *ClusterService {
	return &ClusterService{store: st}
}

// GenerateJoinToken creates a single-use cluster join token. It returns the
// plaintext exactly once; only the hash and an 8-char display prefix are
// persisted. label and createdBy are for operator bookkeeping only.
func (s *ClusterService) GenerateJoinToken(ctx context.Context, label, createdBy string) (string, error) {
	random, err := generateTokenRandom()
	if err != nil {
		return "", fmt.Errorf("failed to generate join token: %w", err)
	}
	plaintext := joinTokenPrefix + random
	tokenHash := hashAPIToken(plaintext)

	displayPrefix := plaintext
	if len(displayPrefix) > 8 {
		displayPrefix = displayPrefix[:8]
	}

	now := time.Now().UTC()
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = s.store.ServerDB.ExecContext(ctx2,
		`INSERT INTO srv_cluster_join_tokens
		   (id, token_hash, token_prefix, label, created_by, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), tokenHash, displayPrefix, label, createdBy, now, now.Add(joinTokenTTL),
	)
	if err != nil {
		return "", fmt.Errorf("failed to store join token: %w", err)
	}

	return plaintext, nil
}

// ValidateAndConsume verifies a presented join token and atomically marks it
// used. It enforces the full PART 34 token contract: the token must exist, not
// be expired, not already be used, and not be inside its reuse-lockout window.
// On success the token is single-use forever (used_at set, lockout_until set to
// used_at + 90 days). usedBy records the node id that redeemed it.
func (s *ClusterService) ValidateAndConsume(ctx context.Context, plaintext, usedBy string) error {
	wantHash := hashAPIToken(plaintext)

	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tx, err := s.store.ServerDB.BeginTx(ctx2, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		storedHash   string
		expiresAt    time.Time
		usedAt       sql.NullTime
		lockoutUntil sql.NullTime
	)
	err = tx.QueryRowContext(ctx2,
		`SELECT token_hash, expires_at, used_at, lockout_until
		   FROM srv_cluster_join_tokens WHERE token_hash = ?`, wantHash,
	).Scan(&storedHash, &expiresAt, &usedAt, &lockoutUntil)
	if err == sql.ErrNoRows {
		return fmt.Errorf("invalid join token")
	}
	if err != nil {
		return fmt.Errorf("failed to look up join token: %w", err)
	}

	// Constant-time comparison to avoid leaking hash bytes via timing.
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(wantHash)) != 1 {
		return fmt.Errorf("invalid join token")
	}

	now := time.Now().UTC()
	if lockoutUntil.Valid && now.Before(lockoutUntil.Time) {
		return fmt.Errorf("join token is locked out")
	}
	if usedAt.Valid {
		return fmt.Errorf("join token already used")
	}
	if now.After(expiresAt) {
		return fmt.Errorf("join token expired")
	}

	res, err := tx.ExecContext(ctx2,
		`UPDATE srv_cluster_join_tokens
		    SET used_at = ?, used_by = ?, lockout_until = ?
		  WHERE token_hash = ? AND used_at IS NULL`,
		now, usedBy, now.Add(joinTokenLockout), wantHash,
	)
	if err != nil {
		return fmt.Errorf("failed to consume join token: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm token consumption: %w", err)
	}
	if affected == 0 {
		// Lost a race with a concurrent redemption of the same token.
		return fmt.Errorf("join token already used")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit token consumption: %w", err)
	}
	return nil
}

// ListJoinTokens returns the most recent join tokens for admin display, newest
// first. The plaintext and hash are never included.
func (s *ClusterService) ListJoinTokens(ctx context.Context, limit int) ([]JoinTokenRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.store.ServerDB.QueryContext(ctx2,
		`SELECT id, COALESCE(label,''), token_prefix, COALESCE(created_by,''),
		        created_at, expires_at, used_at, COALESCE(used_by,''), lockout_until
		   FROM srv_cluster_join_tokens
		  ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list join tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []JoinTokenRecord
	for rows.Next() {
		var (
			rec          JoinTokenRecord
			usedAt       sql.NullTime
			lockoutUntil sql.NullTime
		)
		if err := rows.Scan(&rec.ID, &rec.Label, &rec.TokenPrefix, &rec.CreatedBy,
			&rec.CreatedAt, &rec.ExpiresAt, &usedAt, &rec.UsedBy, &lockoutUntil); err != nil {
			return nil, fmt.Errorf("failed to scan join token: %w", err)
		}
		if usedAt.Valid {
			t := usedAt.Time
			rec.UsedAt = &t
		}
		if lockoutUntil.Valid {
			t := lockoutUntil.Time
			rec.LockoutUntil = &t
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read join tokens: %w", err)
	}
	return out, nil
}

// RevokeJoinToken deletes a pending (unused) join token by id. Used tokens are
// retained so their reuse-lockout window keeps defeating replay; revoking one
// therefore only removes tokens that were never redeemed.
func (s *ClusterService) RevokeJoinToken(ctx context.Context, id string) error {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := s.store.ServerDB.ExecContext(ctx2,
		`DELETE FROM srv_cluster_join_tokens WHERE id = ? AND used_at IS NULL`, id,
	)
	if err != nil {
		return fmt.Errorf("failed to revoke join token: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm revocation: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("join token not found or already used")
	}
	return nil
}
