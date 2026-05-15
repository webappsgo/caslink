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

// TokenRecord represents a stored API token (plaintext never persisted).
type TokenRecord struct {
	ID        int64
	UserID    int64
	UserType  string
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
	CreatedAt time.Time
	LastUsed  *time.Time
}

// TokenService manages API tokens.
type TokenService struct {
	store *store.Store
}

// NewTokenService creates a new token service.
func NewTokenService(st *store.Store) *TokenService {
	return &TokenService{store: st}
}

// CreateToken generates a 32-byte random token, stores only its SHA-256 hash,
// and returns the plaintext for the caller to present once.
func (s *TokenService) CreateToken(ctx context.Context, userID int64, userType, name string, scopes []string, expiresAt *time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	plaintext := hex.EncodeToString(raw)
	tokenHash := hashAPIToken(plaintext)

	scopesStr := strings.Join(scopes, ",")

	var expiresVal interface{}
	if expiresAt != nil {
		expiresVal = expiresAt.UTC()
	}

	query := `INSERT INTO api_tokens (user_id, user_type, token_hash, name, permissions, expires_at, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := s.store.UsersDB.ExecContext(ctx2, query,
		userID, userType, tokenHash, name, scopesStr, expiresVal, time.Now().UTC(),
	); err != nil {
		return "", fmt.Errorf("failed to store token: %w", err)
	}

	return plaintext, nil
}

// ValidateToken looks up a token by its SHA-256 hash using constant-time
// comparison and returns the record if the token is valid and not expired.
func (s *TokenService) ValidateToken(ctx context.Context, plaintext string) (*TokenRecord, error) {
	wantHash := hashAPIToken(plaintext)

	query := `SELECT id, user_id, user_type, name, permissions, expires_at, created_at, last_used
	          FROM api_tokens
	          WHERE token_hash = ?`

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rec TokenRecord
	var storedHash string
	var scopesStr string
	var expiresAt sql.NullTime
	var lastUsed sql.NullTime

	// Fetch by hash (index lookup), then double-check via constant-time compare.
	err := s.store.UsersDB.QueryRowContext(ctx2,
		`SELECT id, user_id, user_type, name, permissions, expires_at, created_at, last_used, token_hash
		 FROM api_tokens WHERE token_hash = ?`, wantHash,
	).Scan(
		&rec.ID, &rec.UserID, &rec.UserType, &rec.Name, &scopesStr,
		&expiresAt, &rec.CreatedAt, &lastUsed, &storedHash,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid token")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	_ = query // suppress unused warning

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
	query := `SELECT id, user_id, user_type, name, permissions, expires_at, created_at, last_used
	          FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC`

	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.store.UsersDB.QueryContext(ctx2, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*TokenRecord
	for rows.Next() {
		var rec TokenRecord
		var scopesStr string
		var expiresAt sql.NullTime
		var lastUsed sql.NullTime

		if err := rows.Scan(
			&rec.ID, &rec.UserID, &rec.UserType, &rec.Name, &scopesStr,
			&expiresAt, &rec.CreatedAt, &lastUsed,
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
