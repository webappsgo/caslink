package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// tokenAlphabet is the character set for the random part of API tokens.
// Lowercase alphanumeric only — safe in URLs, headers, and config files.
const tokenAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// tokenRandomLen is the number of alphanumeric characters after the prefix+underscore.
const tokenRandomLen = 32

// tokenPrefixForType returns the token prefix for a given owner type.
// AI.md PART 11: adm_ (admin), usr_ (user), org_ (org).
func tokenPrefixForType(ownerType string) string {
	switch strings.ToLower(ownerType) {
	case "admin":
		return "adm_"
	case "org":
		return "org_"
	default:
		return "usr_"
	}
}

// generateTokenRandom returns tokenRandomLen cryptographically-random lowercase
// alphanumeric characters.
func generateTokenRandom() (string, error) {
	buf := make([]byte, tokenRandomLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, tokenRandomLen)
	n := byte(len(tokenAlphabet))
	for i, b := range buf {
		out[i] = tokenAlphabet[b%n]
	}
	return string(out), nil
}

// TokenRecord represents a stored API token (plaintext never persisted).
// Uses the unified tokens table schema from AI.md PART 11.
type TokenRecord struct {
	ID          int64
	OwnerType   string
	OwnerID     int64
	Name        string
	TokenPrefix string
	Scope       string
	ExpiresAt   *time.Time
	CreatedAt   time.Time
	LastUsedAt  *time.Time
}

// TokenService manages API tokens using the unified tokens table (AI.md PART 11).
type TokenService struct {
	store *store.Store
}

// NewTokenService creates a new token service.
func NewTokenService(st *store.Store) *TokenService {
	return &TokenService{store: st}
}

// CreateToken generates a prefixed API token per AI.md PART 11:
//
//	{prefix}_{32_alphanumeric}   e.g.  adm_a1b2c3...
//
// Only the SHA-256 hash and the display prefix (first 8 chars) are persisted.
// The full plaintext is returned once and never stored.
// Writes to the unified tokens table; keeps old api_tokens untouched.
func (s *TokenService) CreateToken(ctx context.Context, ownerID int64, ownerType, name string, scopes []string, expiresAt *time.Time) (string, error) {
	prefix := tokenPrefixForType(ownerType)
	random, err := generateTokenRandom()
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	plaintext := prefix + random
	tokenHash := hashAPIToken(plaintext)

	// Store first 8 chars for display: e.g. "adm_a1b2"
	displayPrefix := plaintext
	if len(displayPrefix) > 8 {
		displayPrefix = displayPrefix[:8]
	}

	// Map scopes list to a single scope string per the spec:
	// 'global', 'read-write', or 'read'. Default to 'global'.
	scope := "global"
	if len(scopes) > 0 {
		scope = scopes[0]
	}

	if name == "" {
		name = "default"
	}

	var expiresVal interface{}
	if expiresAt != nil {
		expiresVal = expiresAt.UTC()
	}

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = s.store.UsersDB.ExecContext(ctx2,
		`INSERT INTO tokens (owner_type, owner_id, name, token_hash, token_prefix, scope, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ownerType, ownerID, name, tokenHash, displayPrefix, scope, expiresVal, time.Now().UTC(),
	)
	if err != nil {
		return "", fmt.Errorf("failed to store token: %w", err)
	}

	return plaintext, nil
}

// ValidateToken looks up a token by its SHA-256 hash using constant-time
// comparison and returns the record if the token is valid and not expired.
// Checks the unified tokens table per AI.md PART 11.
func (s *TokenService) ValidateToken(ctx context.Context, plaintext string) (*TokenRecord, error) {
	wantHash := hashAPIToken(plaintext)

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rec TokenRecord
	var storedHash string
	var tokenPrefix sql.NullString
	var expiresAt sql.NullTime
	var lastUsedAt sql.NullTime

	err := s.store.UsersDB.QueryRowContext(ctx2,
		`SELECT id, owner_type, owner_id, name, scope, expires_at, created_at, last_used_at,
		        token_hash, COALESCE(token_prefix, '') as token_prefix
		 FROM tokens WHERE token_hash = ?`, wantHash,
	).Scan(
		&rec.ID, &rec.OwnerType, &rec.OwnerID, &rec.Name, &rec.Scope,
		&expiresAt, &rec.CreatedAt, &lastUsedAt, &storedHash, &tokenPrefix,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid token")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	// Constant-time hash comparison to prevent timing attacks.
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(wantHash)) != 1 {
		return nil, fmt.Errorf("invalid token")
	}

	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		return nil, fmt.Errorf("token expired")
	}

	if expiresAt.Valid {
		t := expiresAt.Time
		rec.ExpiresAt = &t
	}
	if lastUsedAt.Valid {
		t := lastUsedAt.Time
		rec.LastUsedAt = &t
	}
	if tokenPrefix.Valid {
		rec.TokenPrefix = tokenPrefix.String
	}

	// Update last_used_at asynchronously — ignore errors (non-critical).
	go func() {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer bgCancel()
		_, _ = s.store.UsersDB.ExecContext(bgCtx,
			`UPDATE tokens SET last_used_at = ? WHERE id = ?`,
			time.Now().UTC(), rec.ID,
		)
	}()

	return &rec, nil
}

// ListTokens returns all tokens belonging to a given owner.
func (s *TokenService) ListTokens(ctx context.Context, ownerID int64) ([]*TokenRecord, error) {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.store.UsersDB.QueryContext(ctx2,
		`SELECT id, owner_type, owner_id, name, scope, expires_at, created_at, last_used_at,
		        COALESCE(token_prefix, '') as token_prefix
		 FROM tokens WHERE owner_id = ? ORDER BY created_at DESC`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*TokenRecord
	for rows.Next() {
		var rec TokenRecord
		var tokenPrefix sql.NullString
		var expiresAt sql.NullTime
		var lastUsedAt sql.NullTime

		if err := rows.Scan(
			&rec.ID, &rec.OwnerType, &rec.OwnerID, &rec.Name, &rec.Scope,
			&expiresAt, &rec.CreatedAt, &lastUsedAt, &tokenPrefix,
		); err != nil {
			continue
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			rec.ExpiresAt = &t
		}
		if lastUsedAt.Valid {
			t := lastUsedAt.Time
			rec.LastUsedAt = &t
		}
		if tokenPrefix.Valid {
			rec.TokenPrefix = tokenPrefix.String
		}
		tokens = append(tokens, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("token row iteration failed: %w", err)
	}

	return tokens, nil
}

// RevokeToken deletes a token, verifying it belongs to the given owner.
func (s *TokenService) RevokeToken(ctx context.Context, tokenID, ownerID int64) error {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := s.store.UsersDB.ExecContext(ctx2,
		`DELETE FROM tokens WHERE id = ? AND owner_id = ?`, tokenID, ownerID,
	)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("token not found or not owned by user")
	}
	return nil
}

// hashAPIToken returns the hex-encoded SHA-256 hash of the plaintext token.
func hashAPIToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// generateOrgToken generates a prefixed org_ token and returns
// (plaintext, tokenHash, displayPrefix, error).
// Used by OrgService which is in the same package.
func generateOrgToken() (string, string, string, error) {
	random, err := generateTokenRandom()
	if err != nil {
		return "", "", "", err
	}
	plaintext := "org_" + random
	tokenHash := hashAPIToken(plaintext)
	displayPrefix := plaintext
	if len(displayPrefix) > 8 {
		displayPrefix = displayPrefix[:8]
	}
	return plaintext, tokenHash, displayPrefix, nil
}
