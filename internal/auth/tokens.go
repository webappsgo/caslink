package auth

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

// TokenManager manages API tokens
type TokenManager struct {
	db     *db.DB
	config *config.APITokenConfig
	logger *logrus.Logger
}

// NewTokenManager creates a new token manager
func NewTokenManager(database *db.DB, cfg *config.APITokenConfig, logger *logrus.Logger) (*TokenManager, error) {
	return &TokenManager{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// CreateToken creates a new API token for a user
func (tm *TokenManager) CreateToken(ctx context.Context, req *CreateTokenRequest) (*APIToken, error) {
	// Validate request
	if err := tm.validateCreateTokenRequest(req); err != nil {
		return nil, err
	}

	// Generate token ID and value
	tokenID, err := generateTokenID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token ID: %w", err)
	}

	tokenValue, err := generateTokenValue(tm.config.Length)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token value: %w", err)
	}

	now := time.Now()
	var expiresAt *time.Time

	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		expiry := now.Add(*req.ExpiresIn)
		expiresAt = &expiry
	} else if tm.config.DefaultExpiration > 0 {
		expiry := now.Add(tm.config.DefaultExpiration)
		expiresAt = &expiry
	}

	token := &APIToken{
		ID:          tokenID,
		UserID:      req.UserID,
		Name:        req.Name,
		Token:       tokenValue,
		Permissions: req.Permissions,
		RateLimit:   req.RateLimit,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
		Active:      true,
	}

	if err := tm.createTokenInDB(ctx, token); err != nil {
		return nil, fmt.Errorf("failed to create token in database: %w", err)
	}

	tm.logger.WithFields(logrus.Fields{
		"token_id":    token.ID,
		"user_id":     req.UserID,
		"name":        req.Name,
		"permissions": req.Permissions,
		"expires_at":  expiresAt,
	}).Info("New API token created")

	return token, nil
}

// ValidateToken validates an API token and returns token info
func (tm *TokenManager) ValidateToken(ctx context.Context, tokenValue string) (*APIToken, error) {
	token, err := tm.getTokenByValue(ctx, tokenValue)
	if err != nil {
		return nil, err
	}

	// Check if token is active
	if !token.Active {
		return nil, ErrTokenInactive
	}

	// Check if token is expired
	if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
		// Automatically deactivate expired token
		if err := tm.deactivateToken(ctx, token.ID); err != nil {
			tm.logger.WithError(err).WithField("token_id", token.ID).Error("Failed to deactivate expired token")
		}
		return nil, ErrTokenExpired
	}

	// Update last used timestamp
	if err := tm.updateLastUsed(ctx, token.ID); err != nil {
		tm.logger.WithError(err).WithField("token_id", token.ID).Error("Failed to update token last used time")
		// Don't fail validation for this
	}

	return token, nil
}

// GetToken retrieves a token by ID
func (tm *TokenManager) GetToken(ctx context.Context, tokenID string) (*APIToken, error) {
	return tm.getTokenByID(ctx, tokenID)
}

// ListTokens lists API tokens for a user
func (tm *TokenManager) ListTokens(ctx context.Context, userID string) ([]*APIToken, error) {
	query := `
		SELECT id, user_id, name, permissions, rate_limit, created_at,
		       expires_at, last_used, last_used_ip, active
		FROM api_tokens
		WHERE user_id = ? AND active = true
		ORDER BY created_at DESC
	`

	if tm.db.Type() == "postgres" {
		query = `
			SELECT id, user_id, name, permissions, rate_limit, created_at,
			       expires_at, last_used, last_used_ip, active
			FROM api_tokens
			WHERE user_id = $1 AND active = true
			ORDER BY created_at DESC
		`
	}

	rows, err := tm.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		var token APIToken
		var permissions, lastUsedIP *string
		var expiresAt, lastUsed *time.Time

		err := rows.Scan(
			&token.ID,
			&token.UserID,
			&token.Name,
			&permissions,
			&token.RateLimit,
			&token.CreatedAt,
			&expiresAt,
			&lastUsed,
			&lastUsedIP,
			&token.Active,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan token: %w", err)
		}

		token.Permissions = parsePermissions(permissions)
		token.ExpiresAt = expiresAt
		token.LastUsed = lastUsed
		token.LastUsedIP = lastUsedIP

		// Don't include the actual token value in listings
		token.Token = ""

		tokens = append(tokens, &token)
	}

	return tokens, rows.Err()
}

// UpdateToken updates a token's properties
func (tm *TokenManager) UpdateToken(ctx context.Context, tokenID string, req *UpdateTokenRequest) (*APIToken, error) {
	// Get existing token
	token, err := tm.getTokenByID(ctx, tokenID)
	if err != nil {
		return nil, err
	}

	// Update fields
	updated := false

	if req.Name != nil && *req.Name != token.Name {
		token.Name = *req.Name
		updated = true
	}

	if req.Permissions != nil {
		token.Permissions = *req.Permissions
		updated = true
	}

	if req.RateLimit != nil && *req.RateLimit != token.RateLimit {
		token.RateLimit = *req.RateLimit
		updated = true
	}

	if req.ExpiresAt != nil {
		token.ExpiresAt = req.ExpiresAt
		updated = true
	}

	if req.Active != nil && *req.Active != token.Active {
		token.Active = *req.Active
		updated = true
	}

	if updated {
		if err := tm.updateTokenInDB(ctx, token); err != nil {
			return nil, fmt.Errorf("failed to update token in database: %w", err)
		}

		tm.logger.WithFields(logrus.Fields{
			"token_id": token.ID,
			"user_id":  token.UserID,
			"name":     token.Name,
		}).Info("API token updated")
	}

	// Don't return the actual token value
	token.Token = ""
	return token, nil
}

// RevokeToken revokes (deactivates) a token
func (tm *TokenManager) RevokeToken(ctx context.Context, tokenID string) error {
	if err := tm.deactivateToken(ctx, tokenID); err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	tm.logger.WithField("token_id", tokenID).Info("API token revoked")
	return nil
}

// RevokeUserTokens revokes all tokens for a user
func (tm *TokenManager) RevokeUserTokens(ctx context.Context, userID string) error {
	query := "UPDATE api_tokens SET active = false WHERE user_id = ?"
	if tm.db.Type() == "postgres" {
		query = "UPDATE api_tokens SET active = false WHERE user_id = $1"
	}

	result, err := tm.db.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke user tokens: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	tm.logger.WithFields(logrus.Fields{
		"user_id":       userID,
		"rows_affected": rowsAffected,
	}).Info("User API tokens revoked")

	return nil
}

// CleanupExpiredTokens removes expired tokens from the database
func (tm *TokenManager) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	query := "DELETE FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at <= ?"
	if tm.db.Type() == "postgres" {
		query = "DELETE FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at <= $1"
	}

	result, err := tm.db.Exec(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired tokens: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	tm.logger.WithField("rows_affected", rowsAffected).Info("Expired API tokens cleaned up")

	return rowsAffected, nil
}

// GetTokenStats returns token statistics
func (tm *TokenManager) GetTokenStats(ctx context.Context) (*TokenStats, error) {
	stats := &TokenStats{}

	// Count active tokens
	query := "SELECT COUNT(*) FROM api_tokens WHERE active = true"
	err := tm.db.QueryRow(ctx, query).Scan(&stats.ActiveTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to count active tokens: %w", err)
	}

	// Count expired tokens
	query = "SELECT COUNT(*) FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at <= ?"
	if tm.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at <= $1"
	}

	err = tm.db.QueryRow(ctx, query, time.Now()).Scan(&stats.ExpiredTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to count expired tokens: %w", err)
	}

	// Count tokens created today
	today := time.Now().Truncate(24 * time.Hour)
	query = "SELECT COUNT(*) FROM api_tokens WHERE created_at >= ?"
	if tm.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM api_tokens WHERE created_at >= $1"
	}

	err = tm.db.QueryRow(ctx, query, today).Scan(&stats.TokensToday)
	if err != nil {
		return nil, fmt.Errorf("failed to count tokens today: %w", err)
	}

	// Count tokens used in last 24 hours
	yesterday := time.Now().Add(-24 * time.Hour)
	query = "SELECT COUNT(*) FROM api_tokens WHERE last_used >= ?"
	if tm.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM api_tokens WHERE last_used >= $1"
	}

	err = tm.db.QueryRow(ctx, query, yesterday).Scan(&stats.TokensUsedRecently)
	if err != nil {
		return nil, fmt.Errorf("failed to count recently used tokens: %w", err)
	}

	return stats, nil
}

// Helper functions

func (tm *TokenManager) createTokenInDB(ctx context.Context, token *APIToken) error {
	query := `
		INSERT INTO api_tokens (id, user_id, name, token, permissions, rate_limit,
		                       created_at, expires_at, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	if tm.db.Type() == "postgres" {
		query = `
			INSERT INTO api_tokens (id, user_id, name, token, permissions, rate_limit,
			                       created_at, expires_at, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`
	}

	permissionsStr := formatPermissions(token.Permissions)

	_, err := tm.db.Exec(ctx, query,
		token.ID,
		token.UserID,
		token.Name,
		token.Token,
		permissionsStr,
		token.RateLimit,
		token.CreatedAt,
		token.ExpiresAt,
		token.Active,
	)

	return err
}

func (tm *TokenManager) getTokenByValue(ctx context.Context, tokenValue string) (*APIToken, error) {
	query := `
		SELECT id, user_id, name, token, permissions, rate_limit, created_at,
		       expires_at, last_used, last_used_ip, active
		FROM api_tokens
		WHERE token = ? AND active = true
	`

	if tm.db.Type() == "postgres" {
		query = `
			SELECT id, user_id, name, token, permissions, rate_limit, created_at,
			       expires_at, last_used, last_used_ip, active
			FROM api_tokens
			WHERE token = $1 AND active = true
		`
	}

	var token APIToken
	var permissions, lastUsedIP *string
	var expiresAt, lastUsed *time.Time

	err := tm.db.QueryRow(ctx, query, tokenValue).Scan(
		&token.ID,
		&token.UserID,
		&token.Name,
		&token.Token,
		&permissions,
		&token.RateLimit,
		&token.CreatedAt,
		&expiresAt,
		&lastUsed,
		&lastUsedIP,
		&token.Active,
	)

	if err != nil {
		if err == db.ErrNoRows {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}

	token.Permissions = parsePermissions(permissions)
	token.ExpiresAt = expiresAt
	token.LastUsed = lastUsed
	token.LastUsedIP = lastUsedIP

	return &token, nil
}

func (tm *TokenManager) getTokenByID(ctx context.Context, tokenID string) (*APIToken, error) {
	query := `
		SELECT id, user_id, name, permissions, rate_limit, created_at,
		       expires_at, last_used, last_used_ip, active
		FROM api_tokens
		WHERE id = ?
	`

	if tm.db.Type() == "postgres" {
		query = `
			SELECT id, user_id, name, permissions, rate_limit, created_at,
			       expires_at, last_used, last_used_ip, active
			FROM api_tokens
			WHERE id = $1
		`
	}

	var token APIToken
	var permissions, lastUsedIP *string
	var expiresAt, lastUsed *time.Time

	err := tm.db.QueryRow(ctx, query, tokenID).Scan(
		&token.ID,
		&token.UserID,
		&token.Name,
		&permissions,
		&token.RateLimit,
		&token.CreatedAt,
		&expiresAt,
		&lastUsed,
		&lastUsedIP,
		&token.Active,
	)

	if err != nil {
		if err == db.ErrNoRows {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}

	token.Permissions = parsePermissions(permissions)
	token.ExpiresAt = expiresAt
	token.LastUsed = lastUsed
	token.LastUsedIP = lastUsedIP

	return &token, nil
}

func (tm *TokenManager) updateTokenInDB(ctx context.Context, token *APIToken) error {
	query := `
		UPDATE api_tokens
		SET name = ?, permissions = ?, rate_limit = ?, expires_at = ?, active = ?
		WHERE id = ?
	`

	if tm.db.Type() == "postgres" {
		query = `
			UPDATE api_tokens
			SET name = $1, permissions = $2, rate_limit = $3, expires_at = $4, active = $5
			WHERE id = $6
		`
	}

	permissionsStr := formatPermissions(token.Permissions)

	_, err := tm.db.Exec(ctx, query,
		token.Name,
		permissionsStr,
		token.RateLimit,
		token.ExpiresAt,
		token.Active,
		token.ID,
	)

	return err
}

func (tm *TokenManager) deactivateToken(ctx context.Context, tokenID string) error {
	query := "UPDATE api_tokens SET active = false WHERE id = ?"
	if tm.db.Type() == "postgres" {
		query = "UPDATE api_tokens SET active = false WHERE id = $1"
	}

	_, err := tm.db.Exec(ctx, query, tokenID)
	return err
}

func (tm *TokenManager) updateLastUsed(ctx context.Context, tokenID string) error {
	query := "UPDATE api_tokens SET last_used = ? WHERE id = ?"
	if tm.db.Type() == "postgres" {
		query = "UPDATE api_tokens SET last_used = $1 WHERE id = $2"
	}

	_, err := tm.db.Exec(ctx, query, time.Now(), tokenID)
	return err
}

func (tm *TokenManager) validateCreateTokenRequest(req *CreateTokenRequest) error {
	if req.UserID == "" {
		return ErrInvalidUserID
	}

	if req.Name == "" {
		return ErrInvalidTokenName
	}

	if len(req.Name) > 255 {
		return ErrTokenNameTooLong
	}

	if req.RateLimit < 0 {
		return ErrInvalidRateLimit
	}

	return nil
}

// generateTokenID generates a unique token ID
func generateTokenID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// generateTokenValue generates a cryptographically secure token value
func generateTokenValue(length int) (string, error) {
	if length < 16 {
		length = 32 // Default to 256 bits
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// formatPermissions converts permissions slice to JSON string
func formatPermissions(permissions []string) *string {
	if len(permissions) == 0 {
		return nil
	}

	// Simple JSON array formatting
	result := "[\"" + strings.Join(permissions, "\",\"") + "\"]"
	return &result
}

// parsePermissions converts JSON string to permissions slice
func parsePermissions(permissionsStr *string) []string {
	if permissionsStr == nil || *permissionsStr == "" {
		return []string{}
	}

	// Simple JSON array parsing
	str := *permissionsStr
	if !strings.HasPrefix(str, "[") || !strings.HasSuffix(str, "]") {
		return []string{}
	}

	str = str[1 : len(str)-1] // Remove brackets
	if str == "" {
		return []string{}
	}

	// Split by comma and clean quotes
	parts := strings.Split(str, ",")
	var permissions []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"")
		if part != "" {
			permissions = append(permissions, part)
		}
	}

	return permissions
}

// ValidateTokenValue checks if a token value is valid format
func ValidateTokenValue(tokenValue string) bool {
	// Token should be at least 32 hex characters
	if len(tokenValue) < 32 || len(tokenValue)%2 != 0 {
		return false
	}

	// Check if all characters are valid hex
	for _, char := range tokenValue {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}

	return true
}