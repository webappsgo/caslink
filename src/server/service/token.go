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
func tokenPrefixForType(userType string) string {
	switch strings.ToLower(userType) {
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
type TokenRecord struct {
	ID          int64
	UserID      int64
	UserType    string
	Name        string
	TokenPrefix string
	Scopes      []string
	ExpiresAt   *time.Time
	CreatedAt   time.Time
	LastUsed    *time.Time
}

// TokenService manages API tokens.
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
func (s *TokenService) CreateToken(ctx context.Context, userID int64, userType, name string, scopes []string, expiresAt *time.Time) (string, error) {
	prefix := tokenPrefixForType(userType)
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

	scopesStr := strings.Join(scopes, ",")

	var expiresVal interface{}
	if expiresAt != nil {
		expiresVal = expiresAt.UTC()
	}

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = s.store.UsersDB.ExecContext(ctx2,
		`INSERT INTO api_tokens (user_id, user_type, token_hash, token_prefix, name, permissions, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, userType, tokenHash, displayPrefix, name, scopesStr, expiresVal, time.Now().UTC(),
	)
	if err != nil {
		// Fall back to the old schema (without token_prefix column) for existing DBs.
		_, err = s.store.UsersDB.ExecContext(ctx2,
			`INSERT INTO api_tokens (user_id, user_type, token_hash, name, permissions, expires_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			userID, userType, tokenHash, name, scopesStr, expiresVal, time.Now().UTC(),
		)
		if err != nil {
			return "", fmt.Errorf("failed to store token: %w", err)
		}
	}

	return plaintext, nil
}

// ValidateToken looks up a token by its SHA-256 hash using constant-time
// comparison and returns the record if the token is valid and not expired.
func (s *TokenService) ValidateToken(ctx context.Context, plaintext string) (*TokenRecord, error) {
	wantHash := hashAPIToken(plaintext)

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rec TokenRecord
	var storedHash string
	var scopesStr string
	var tokenPrefix sql.NullString
	var expiresAt sql.NullTime
	var lastUsed sql.NullTime

	// Fetch by hash (index lookup), then double-check via constant-time compare.
	err := s.store.UsersDB.QueryRowContext(ctx2,
		`SELECT id, user_id, user_type, name, permissions, expires_at, created_at, last_used, token_hash, COALESCE(token_prefix, '') as token_prefix
		 FROM api_tokens WHERE token_hash = ?`, wantHash,
	).Scan(
		&rec.ID, &rec.UserID, &rec.UserType, &rec.Name, &scopesStr,
		&expiresAt, &rec.CreatedAt, &lastUsed, &storedHash, &tokenPrefix,
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
	if lastUsed.Valid {
		t := lastUsed.Time
		rec.LastUsed = &t
	}
	if scopesStr != "" {
		rec.Scopes = strings.Split(scopesStr, ",")
	}
	if tokenPrefix.Valid {
		rec.TokenPrefix = tokenPrefix.String
	}

	// Update last_used asynchronously — ignore errors (non-critical).
	go func() {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer bgCancel()
		_, _ = s.store.UsersDB.ExecContext(bgCtx,
			`UPDATE api_tokens SET last_used = ? WHERE id = ?`,
			time.Now().UTC(), rec.ID,
		)
	}()

	return &rec, nil
}

// ListTokens returns all tokens belonging to a user (no hashes exposed).
func (s *TokenService) ListTokens(ctx context.Context, userID int64) ([]*TokenRecord, error) {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.store.UsersDB.QueryContext(ctx2,
		`SELECT id, user_id, user_type, name, permissions, expires_at, created_at, last_used, COALESCE(token_prefix, '') as token_prefix
		 FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*TokenRecord
	for rows.Next() {
		var rec TokenRecord
		var scopesStr string
		var tokenPrefix sql.NullString
		var expiresAt sql.NullTime
		var lastUsed sql.NullTime

		if err := rows.Scan(
			&rec.ID, &rec.UserID, &rec.UserType, &rec.Name, &scopesStr,
			&expiresAt, &rec.CreatedAt, &lastUsed, &tokenPrefix,
		); err != nil {
			continue
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			rec.ExpiresAt = &t
		}
		if lastUsed.Valid {
			t := lastUsed.Time
			rec.LastUsed = &t
		}
		if scopesStr != "" {
			rec.Scopes = strings.Split(scopesStr, ",")
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

// RevokeToken deletes a token, verifying it belongs to the given user.
func (s *TokenService) RevokeToken(ctx context.Context, tokenID, userID int64) error {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := s.store.UsersDB.ExecContext(ctx2,
		`DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, tokenID, userID,
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
