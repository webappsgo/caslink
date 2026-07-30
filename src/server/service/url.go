package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/geoip"
	"github.com/casjaysdevdocker/caslink/src/server/model"
	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// URLService handles URL shortening operations
type URLService struct {
	store *store.Store
	geo   *geoip.Service // optional; nil means click records leave country/city empty
}

// NewURLService creates a new URL service
func NewURLService(st *store.Store) *URLService {
	return &URLService{
		store: st,
	}
}

// SetGeoIP attaches a GeoIP service so click analytics can be enriched with
// country/city. Pass nil to disable enrichment.
func (s *URLService) SetGeoIP(g *geoip.Service) {
	s.geo = g
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
		hash, err := hashPasswordArgon2id(req.Password)
		if err != nil {
			return nil, fmt.Errorf("failed to hash URL password: %w", err)
		}
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

// UpdateURL applies a partial update to an existing short URL per AI.md
// PART 16 (`PUT /api/{api_version}/links/{code}`). Only fields present in
// req are changed; nil fields are left as-is. Returns model.ErrURLNotFound
// if the code does not exist.
func (s *URLService) UpdateURL(ctx context.Context, shortCode string, req *model.UpdateURLRequest) (*model.URL, error) {
	// Use the raw (non-expiry-checking) fetch so an expired link can still
	// be edited — e.g. to push out expires_at and revive it.
	existing, err := s.getURLByCodeRaw(ctx, shortCode)
	if err != nil {
		return nil, err
	}

	longURL := existing.LongURL
	if req.LongURL != nil {
		if _, err := url.ParseRequestURI(*req.LongURL); err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}
		longURL = *req.LongURL
	}

	title := existing.Title
	if req.Title != nil {
		title = req.Title
	}

	description := existing.Description
	if req.Description != nil {
		description = req.Description
	}

	passwordHash := existing.PasswordHash
	if req.Password != nil {
		if *req.Password == "" {
			passwordHash = nil
		} else {
			hash, err := hashPasswordArgon2id(*req.Password)
			if err != nil {
				return nil, fmt.Errorf("failed to hash URL password: %w", err)
			}
			passwordHash = &hash
		}
	}

	// A non-nil pointer to the zero time.Time is the sentinel for "clear the
	// expiration" (a link can never legitimately expire in year 1), since
	// nil already means "leave expires_at unchanged".
	expiresAt := existing.ExpiresAt
	if req.ExpiresAt != nil {
		if req.ExpiresAt.IsZero() {
			expiresAt = nil
		} else {
			expiresAt = req.ExpiresAt
		}
	}

	query := `UPDATE urls SET long_url = ?, title = ?, description = ?, password_hash = ?, expires_at = ?, updated_at = CURRENT_TIMESTAMP
	          WHERE short_code = ?`
	if _, err := s.store.ServerDB.ExecContext(ctx, query,
		longURL, title, description, passwordHash, expiresAt, shortCode); err != nil {
		return nil, fmt.Errorf("failed to update URL: %w", err)
	}

	return s.GetURLByCode(ctx, shortCode)
}

// DeleteURL permanently removes a short URL per AI.md PART 16
// (`DELETE /api/{api_version}/links/{code}`). Returns model.ErrURLNotFound
// if the code does not exist.
func (s *URLService) DeleteURL(ctx context.Context, shortCode string) error {
	result, err := s.store.ServerDB.ExecContext(ctx, `DELETE FROM urls WHERE short_code = ?`, shortCode)
	if err != nil {
		return fmt.Errorf("failed to delete URL: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm delete: %w", err)
	}
	if rows == 0 {
		return model.ErrURLNotFound
	}
	return nil
}

// GetURLByCode retrieves a URL by its short code
func (s *URLService) GetURLByCode(ctx context.Context, shortCode string) (*model.URL, error) {
	u, err := s.getURLByCodeRaw(ctx, shortCode)
	if err != nil {
		return nil, err
	}

	// Check if expired
	if u.ExpiresAt != nil && time.Now().After(*u.ExpiresAt) {
		return nil, model.ErrURLExpired
	}

	return u, nil
}

// GetURLByCodeAny retrieves a URL by its short code without the expiry
// check, so callers that need to operate on (or revive) an expired link —
// UpdateURL, DeleteURL, and their ownership checks — are not blocked by
// model.ErrURLExpired.
func (s *URLService) GetURLByCodeAny(ctx context.Context, shortCode string) (*model.URL, error) {
	return s.getURLByCodeRaw(ctx, shortCode)
}

func (s *URLService) getURLByCodeRaw(ctx context.Context, shortCode string) (*model.URL, error) {
	query := `SELECT id, short_code, long_url, title, description, user_id, org_id, custom_code, password_hash, expires_at, created_at, updated_at
	          FROM urls WHERE short_code = ?`

	var u model.URL
	err := s.store.ServerDB.QueryRowContext(ctx, query, shortCode).Scan(
		&u.ID, &u.ShortCode, &u.LongURL, &u.Title, &u.Description,
		&u.UserID, &u.OrgID, &u.CustomCode, &u.PasswordHash, &u.ExpiresAt,
		&u.CreatedAt, &u.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, model.ErrURLNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query URL: %w", err)
	}

	return &u, nil
}

// RecordClick records a click/visit to a URL. When the GeoIP service is
// available and the IP is public, country/city are looked up and stored
// alongside the hashed IP, user agent, and referrer (AI.md PART 20 + PART 9
// clicks schema).
func (s *URLService) RecordClick(ctx context.Context, urlID int64, ipAddress, userAgent, referrer string) error {
	// Hash IP for privacy (per SPEC PART 36: anonymize_ips)
	ipHash := hashIP(ipAddress)

	var country, city string
	if s.geo != nil && s.geo.Enabled() {
		if ip := net.ParseIP(ipAddress); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() {
			res := s.geo.LookupCity(ip)
			country = res.CountryCode
			city = res.City
		}
	}

	query := `INSERT INTO clicks (url_id, ip_hash, country, city, user_agent, referrer)
	          VALUES (?, ?, ?, ?, ?, ?)`

	_, err := s.store.ServerDB.ExecContext(ctx, query, urlID, ipHash, country, city, userAgent, referrer)
	if err != nil {
		return fmt.Errorf("failed to record click: %w", err)
	}

	return nil
}

// CreateURLForUser creates a shortened URL owned by the given userID.
func (s *URLService) CreateURLForUser(ctx context.Context, userID int64, req *model.CreateURLRequest) (*model.URL, error) {
	// Validate URL
	if _, err := url.ParseRequestURI(req.LongURL); err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	var shortCode string
	var isCustom bool

	if req.CustomCode != "" {
		if err := s.validateCustomCode(req.CustomCode); err != nil {
			return nil, err
		}
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
		code, err := s.generateRandomCode(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to generate code: %w", err)
		}
		shortCode = code
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		expiresAt = req.ExpiresAt
	} else if req.ExpireAfter != "" {
		exp := parseExpiration(req.ExpireAfter)
		expiresAt = &exp
	}

	query := `INSERT INTO urls (short_code, long_url, title, description, user_id, custom_code, expires_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	result, err := s.store.ServerDB.ExecContext(ctx, query,
		shortCode, req.LongURL, req.Title, req.Description, userID, isCustom, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert URL: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get insert ID: %w", err)
	}

	return &model.URL{
		ID:        id,
		ShortCode: shortCode,
		LongURL:   req.LongURL,
		Title:     req.Title,
		UserID:    &userID,
		CustomCode: isCustom,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// CreateURLForOrg creates a shortened URL owned by the given org (used by
// org-scoped Bearer tokens, OwnerType "org" — see service/token.go). Mirrors
// CreateURLForUser but sets org_id instead of user_id, per AI.md PART 35 /
// TODO.AI.md PART 16 org-owned-link modeling.
func (s *URLService) CreateURLForOrg(ctx context.Context, orgID int64, req *model.CreateURLRequest) (*model.URL, error) {
	if _, err := url.ParseRequestURI(req.LongURL); err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	var shortCode string
	var isCustom bool

	if req.CustomCode != "" {
		if err := s.validateCustomCode(req.CustomCode); err != nil {
			return nil, err
		}
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
		code, err := s.generateRandomCode(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to generate code: %w", err)
		}
		shortCode = code
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		expiresAt = req.ExpiresAt
	} else if req.ExpireAfter != "" {
		exp := parseExpiration(req.ExpireAfter)
		expiresAt = &exp
	}

	query := `INSERT INTO urls (short_code, long_url, title, description, org_id, custom_code, expires_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	result, err := s.store.ServerDB.ExecContext(ctx, query,
		shortCode, req.LongURL, req.Title, req.Description, orgID, isCustom, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert URL: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get insert ID: %w", err)
	}

	return &model.URL{
		ID:         id,
		ShortCode:  shortCode,
		LongURL:    req.LongURL,
		Title:      req.Title,
		OrgID:      &orgID,
		CustomCode: isCustom,
		ExpiresAt:  expiresAt,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

// ListByUser returns the most recent URLs created by a user (up to limit).
func (s *URLService) ListByUser(ctx context.Context, userID int64, limit int) ([]*model.URL, error) {
	return s.ListByUserPage(ctx, userID, limit, 0)
}

// ListByUserPage returns a page of URLs created by a user, newest first,
// per AI.md PART 14 query-param pagination convention (?page&limit).
func (s *URLService) ListByUserPage(ctx context.Context, userID int64, limit, offset int) ([]*model.URL, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := `SELECT id, short_code, long_url, title, description, user_id, org_id, custom_code, password_hash, expires_at, created_at, updated_at
	          FROM urls WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := s.store.ServerDB.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list URLs: %w", err)
	}
	defer rows.Close()

	var urls []*model.URL
	for rows.Next() {
		var u model.URL
		if err := rows.Scan(
			&u.ID, &u.ShortCode, &u.LongURL, &u.Title, &u.Description,
			&u.UserID, &u.OrgID, &u.CustomCode, &u.PasswordHash, &u.ExpiresAt,
			&u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan URL row: %w", err)
		}
		urls = append(urls, &u)
	}
	return urls, rows.Err()
}

// CountByUser returns the total number of URLs owned by a user, for
// pagination totals on GET /api/v1/urls.
func (s *URLService) CountByUser(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.store.ServerDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM urls WHERE user_id = ?`, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to count URLs: %w", err)
	}
	return n, nil
}

// ListByOrgPage returns a page of URLs owned by an org, newest first,
// mirroring ListByUserPage for org-scoped Bearer tokens.
func (s *URLService) ListByOrgPage(ctx context.Context, orgID int64, limit, offset int) ([]*model.URL, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := `SELECT id, short_code, long_url, title, description, user_id, org_id, custom_code, password_hash, expires_at, created_at, updated_at
	          FROM urls WHERE org_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := s.store.ServerDB.QueryContext(ctx, query, orgID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list URLs: %w", err)
	}
	defer rows.Close()

	var urls []*model.URL
	for rows.Next() {
		var u model.URL
		if err := rows.Scan(
			&u.ID, &u.ShortCode, &u.LongURL, &u.Title, &u.Description,
			&u.UserID, &u.OrgID, &u.CustomCode, &u.PasswordHash, &u.ExpiresAt,
			&u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan URL row: %w", err)
		}
		urls = append(urls, &u)
	}
	return urls, rows.Err()
}

// CountByOrg returns the total number of URLs owned by an org, for
// pagination totals on org-scoped list endpoints.
func (s *URLService) CountByOrg(ctx context.Context, orgID int64) (int, error) {
	var n int
	err := s.store.ServerDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM urls WHERE org_id = ?`, orgID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to count URLs: %w", err)
	}
	return n, nil
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
