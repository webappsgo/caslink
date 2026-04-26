package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/model"
	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// URLService handles URL shortening operations
type URLService struct {
	store *store.Store
}

// NewURLService creates a new URL service
func NewURLService(st *store.Store) *URLService {
	return &URLService{
		store: st,
	}
}

// CreateURL creates a new shortened URL
func (s *URLService) CreateURL(ctx context.Context, req *model.CreateURLRequest) (*model.URL, error) {
	// Validate URL
	if _, err := url.ParseRequestURI(req.LongURL); err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Generate or validate short code
	var shortCode string
	var isCustom bool

	if req.CustomCode != "" {
		// Validate custom code
		if err := s.validateCustomCode(req.CustomCode); err != nil {
			return nil, err
		}

		// Check if code already exists
		exists, err := s.codeExists(ctx, req.CustomCode)
		if err != nil {
			return nil, fmt.Errorf("failed to check code: %w", err)
		}
		if exists {
			return nil, model.ErrCodeAlreadyExists
		}

		shortCode = req.CustomCode
		isCustom = true
	} else {
		// Generate random code
		code, err := s.generateRandomCode(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to generate code: %w", err)
		}
		shortCode = code
		isCustom = false
	}

	// Hash password if provided (using Argon2id per SPEC line 129)
	var passwordHash *string
	if req.Password != "" {
		hash := hashPasswordArgon2id(req.Password)
		passwordHash = &hash
	}

	// Parse expiration
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		expiresAt = req.ExpiresAt
	} else if req.ExpireAfter != "" {
		exp := parseExpiration(req.ExpireAfter)
		expiresAt = &exp
	}

	// Insert into database
	query := `INSERT INTO urls (short_code, long_url, title, description, user_id, custom_code, password_hash, expires_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := s.store.ServerDB.ExecContext(ctx, query,
		shortCode, req.LongURL, req.Title, req.Description, nil, isCustom, passwordHash, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert URL: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get insert ID: %w", err)
	}

	// Return created URL
	urlRecord := &model.URL{
		ID:          id,
		ShortCode:   shortCode,
		LongURL:     req.LongURL,
		Title:       req.Title,
		Description: req.Description,
		CustomCode:  isCustom,
		PasswordHash: passwordHash,
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return urlRecord, nil
}

// GetURLByCode retrieves a URL by its short code
func (s *URLService) GetURLByCode(ctx context.Context, shortCode string) (*model.URL, error) {
	query := `SELECT id, short_code, long_url, title, description, user_id, custom_code, password_hash, expires_at, created_at, updated_at
	          FROM urls WHERE short_code = ?`

	var u model.URL
	err := s.store.ServerDB.QueryRowContext(ctx, query, shortCode).Scan(
		&u.ID, &u.ShortCode, &u.LongURL, &u.Title, &u.Description,
		&u.UserID, &u.CustomCode, &u.PasswordHash, &u.ExpiresAt,
		&u.CreatedAt, &u.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, model.ErrURLNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query URL: %w", err)
	}

	// Check if expired
	if u.ExpiresAt != nil && time.Now().After(*u.ExpiresAt) {
		return nil, model.ErrURLExpired
	}

	return &u, nil
}

// RecordClick records a click/visit to a URL
func (s *URLService) RecordClick(ctx context.Context, urlID int64, ipAddress, userAgent, referrer string) error {
	// Hash IP for privacy (per SPEC PART 36: anonymize_ips)
	ipHash := hashIP(ipAddress)

	query := `INSERT INTO clicks (url_id, ip_hash, user_agent, referrer)
	          VALUES (?, ?, ?, ?)`

	_, err := s.store.ServerDB.ExecContext(ctx, query, urlID, ipHash, userAgent, referrer)
	if err != nil {
		return fmt.Errorf("failed to record click: %w", err)
	}

	return nil
}

// validateCustomCode validates a custom short code
func (s *URLService) validateCustomCode(code string) error {
	// Check length (per SPEC PART 36: min 3, max 50)
	if len(code) < 3 || len(code) > 50 {
		return model.ErrInvalidCustomCode
	}

	// Check for reserved words (per SPEC PART 36)
	reservedWords := []string{
		"api", "admin", "www", "app", "help", "about", "setup",
		"login", "register", "dashboard", "health", "version",
	}

	codeLower := strings.ToLower(code)
	for _, reserved := range reservedWords {
		if codeLower == reserved {
			return model.ErrReservedWord
		}
	}

	// Check allowed characters (alphanumeric only)
	for _, ch := range code {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')) {
			return model.ErrInvalidCustomCode
		}
	}

	return nil
}

// codeExists checks if a short code already exists
func (s *URLService) codeExists(ctx context.Context, code string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM urls WHERE short_code = ?)`
	err := s.store.ServerDB.QueryRowContext(ctx, query, code).Scan(&exists)
	return exists, err
}

// generateRandomCode generates a random short code
func (s *URLService) generateRandomCode(ctx context.Context) (string, error) {
	// Allowed characters (per SPEC PART 36: exclude similar chars)
	// Excludes: 0/O, 1/l/I for clarity
	const charset = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
	const minLength = 6
	const maxLength = 8
	const maxRetries = 10

	for retry := 0; retry < maxRetries; retry++ {
		// Start with min length, increase on collision
		length := minLength + retry
		if length > maxLength {
			length = maxLength
		}

		// Generate random code
		code := make([]byte, length)
		for i := range code {
			num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return "", fmt.Errorf("random generation failed: %w", err)
			}
			code[i] = charset[num.Int64()]
		}

		shortCode := string(code)

		// Check if code exists
		exists, err := s.codeExists(ctx, shortCode)
		if err != nil {
			return "", fmt.Errorf("failed to check code: %w", err)
		}

		if !exists {
			return shortCode, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique code after %d retries", maxRetries)
}


// hashIP hashes an IP address for privacy (per SPEC PART 36)
func hashIP(ip string) string {
	// Use daily salt for additional privacy
	salt := time.Now().Format("2006-01-02")
	data := fmt.Sprintf("%s:%s", ip, salt)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// parseExpiration parses expiration duration string
func parseExpiration(duration string) time.Time {
	now := time.Now()

	switch duration {
	case "1h":
		return now.Add(1 * time.Hour)
	case "24h":
		return now.Add(24 * time.Hour)
	case "7d":
		return now.Add(7 * 24 * time.Hour)
	case "30d":
		return now.Add(30 * 24 * time.Hour)
	case "never":
		// Return zero time for never expires
		return time.Time{}
	default:
		// Default to never
		return time.Time{}
	}
}
