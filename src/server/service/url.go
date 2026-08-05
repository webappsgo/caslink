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

	"github.com/webappsgo/caslink/src/geoip"
	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/store"
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

// URLRequiresPassword reports whether the URL is password-protected and must
// be unlocked before a redirect is served. Password protection is an
// advertised link option (IDEA.md "Link options"); the redirect path MUST
// consult this before honoring the destination.
func URLRequiresPassword(u *model.URL) bool {
	return u.PasswordHash != nil && *u.PasswordHash != ""
}

// VerifyURLPassword reports whether plaintext matches the URL's stored
// Argon2id password hash, using the same constant-time verifier as account
// passwords (AI.md PART 11). Returns false for an unprotected URL.
func VerifyURLPassword(u *model.URL, plaintext string) bool {
	if u.PasswordHash == nil || *u.PasswordHash == "" {
		return false
	}
	return verifyPasswordArgon2id(plaintext, *u.PasswordHash)
}

// CreateURL creates a new shortened URL
func (s *URLService) CreateURL(ctx context.Context, req *model.CreateURLRequest) (*model.URL, error) {
	// Validate URL
	if _, err := url.ParseRequestURI(req.LongURL); err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrInvalidURL, err)
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
		expiresAt = parseExpiration(req.ExpireAfter)
	}

	opts, err := buildLinkOptions(req)
	if err != nil {
		return nil, err
	}

	// Insert into database
	query := `INSERT INTO urls (short_code, long_url, title, description, user_id, custom_code, password_hash, expires_at,
	          visibility, tags, utm_source, utm_medium, utm_campaign, utm_term, utm_content, geo_mode, geo_countries,
	          mobile_url, desktop_url, tablet_url)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := s.store.ServerDB.ExecContext(ctx, query,
		shortCode, req.LongURL, req.Title, req.Description, nil, isCustom, passwordHash, expiresAt,
		opts.visibility, opts.tags, opts.utmSource, opts.utmMedium, opts.utmCampaign, opts.utmTerm, opts.utmContent,
		opts.geoMode, opts.geoCountries, opts.mobileURL, opts.desktopURL, opts.tabletURL)
	if err != nil {
		return nil, fmt.Errorf("failed to insert URL: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get insert ID: %w", err)
	}

	// Return created URL
	urlRecord := &model.URL{
		ID:           id,
		ShortCode:    shortCode,
		LongURL:      req.LongURL,
		Title:        req.Title,
		Description:  req.Description,
		CustomCode:   isCustom,
		PasswordHash: passwordHash,
		ExpiresAt:    expiresAt,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	opts.applyTo(urlRecord)

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

	visibility := existing.Visibility
	if req.Visibility != nil {
		visibility = *req.Visibility
	}

	tagsCSV := joinCSV(existing.Tags)
	if req.Tags != nil {
		tagsCSV = joinCSV(cleanStrings(*req.Tags))
	}

	geoMode := existing.GeoMode
	if req.GeoMode != nil {
		geoMode = *req.GeoMode
	}

	geoCountriesCSV := joinCSV(existing.GeoCountries)
	if req.GeoCountries != nil {
		codes, err := normalizeCountryCodes(*req.GeoCountries)
		if err != nil {
			return nil, err
		}
		geoCountriesCSV = joinCSV(codes)
	}
	if geoMode != "none" && geoCountriesCSV == "" {
		return nil, fmt.Errorf("geo_countries is required when geo_mode is not none")
	}

	utmSource := existing.UTMSource
	if req.UTMSource != nil {
		utmSource = req.UTMSource
	}
	utmMedium := existing.UTMMedium
	if req.UTMMedium != nil {
		utmMedium = req.UTMMedium
	}
	utmCampaign := existing.UTMCampaign
	if req.UTMCampaign != nil {
		utmCampaign = req.UTMCampaign
	}
	utmTerm := existing.UTMTerm
	if req.UTMTerm != nil {
		utmTerm = req.UTMTerm
	}
	utmContent := existing.UTMContent
	if req.UTMContent != nil {
		utmContent = req.UTMContent
	}

	mobileURL := existing.MobileURL
	if req.MobileURL != nil {
		mobileURL = req.MobileURL
	}
	desktopURL := existing.DesktopURL
	if req.DesktopURL != nil {
		desktopURL = req.DesktopURL
	}
	tabletURL := existing.TabletURL
	if req.TabletURL != nil {
		tabletURL = req.TabletURL
	}

	query := `UPDATE urls SET long_url = ?, title = ?, description = ?, password_hash = ?, expires_at = ?,
	          visibility = ?, tags = ?, utm_source = ?, utm_medium = ?, utm_campaign = ?, utm_term = ?, utm_content = ?,
	          geo_mode = ?, geo_countries = ?, mobile_url = ?, desktop_url = ?, tablet_url = ?, updated_at = CURRENT_TIMESTAMP
	          WHERE short_code = ?`
	if _, err := s.store.ServerDB.ExecContext(ctx, query,
		longURL, title, description, passwordHash, expiresAt,
		visibility, tagsCSV, utmSource, utmMedium, utmCampaign, utmTerm, utmContent,
		geoMode, geoCountriesCSV, mobileURL, desktopURL, tabletURL, shortCode); err != nil {
		return nil, fmt.Errorf("failed to update URL: %w", err)
	}

	// Re-fetch via the raw (non-expiry-checking) path — the write above
	// just succeeded, so it must never be reported as model.ErrURLExpired
	// even if the caller set expires_at to a past time (e.g. an admin
	// "revive an expired link" edit).
	return s.getURLByCodeRaw(ctx, shortCode)
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

// urlSelectColumns is the shared column list for reading a full urls row,
// used by getURLByCodeRaw, ListByUserPage, and ListByOrgPage.
const urlSelectColumns = `id, short_code, long_url, title, description, user_id, org_id, custom_code, password_hash, expires_at,
	          visibility, tags, utm_source, utm_medium, utm_campaign, utm_term, utm_content, geo_mode, geo_countries,
	          mobile_url, desktop_url, tablet_url, created_at, updated_at`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanURLRow scans one urls row (using urlSelectColumns' column order) into a
// model.URL, converting the nullable link-option columns.
func scanURLRow(row rowScanner) (*model.URL, error) {
	var u model.URL
	var tags, utmSource, utmMedium, utmCampaign, utmTerm, utmContent, geoCountries, mobileURL, desktopURL, tabletURL sql.NullString

	err := row.Scan(
		&u.ID, &u.ShortCode, &u.LongURL, &u.Title, &u.Description,
		&u.UserID, &u.OrgID, &u.CustomCode, &u.PasswordHash, &u.ExpiresAt,
		&u.Visibility, &tags, &utmSource, &utmMedium, &utmCampaign, &utmTerm, &utmContent,
		&u.GeoMode, &geoCountries, &mobileURL, &desktopURL, &tabletURL,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	u.Tags = splitCSV(tags.String)
	if utmSource.Valid && utmSource.String != "" {
		u.UTMSource = strPtr(utmSource.String)
	}
	if utmMedium.Valid && utmMedium.String != "" {
		u.UTMMedium = strPtr(utmMedium.String)
	}
	if utmCampaign.Valid && utmCampaign.String != "" {
		u.UTMCampaign = strPtr(utmCampaign.String)
	}
	if utmTerm.Valid && utmTerm.String != "" {
		u.UTMTerm = strPtr(utmTerm.String)
	}
	if utmContent.Valid && utmContent.String != "" {
		u.UTMContent = strPtr(utmContent.String)
	}
	u.GeoCountries = splitCSV(geoCountries.String)
	if mobileURL.Valid && mobileURL.String != "" {
		u.MobileURL = strPtr(mobileURL.String)
	}
	if desktopURL.Valid && desktopURL.String != "" {
		u.DesktopURL = strPtr(desktopURL.String)
	}
	if tabletURL.Valid && tabletURL.String != "" {
		u.TabletURL = strPtr(tabletURL.String)
	}

	return &u, nil
}

func (s *URLService) getURLByCodeRaw(ctx context.Context, shortCode string) (*model.URL, error) {
	query := `SELECT ` + urlSelectColumns + ` FROM urls WHERE short_code = ?`

	u, err := scanURLRow(s.store.ServerDB.QueryRowContext(ctx, query, shortCode))
	if err == sql.ErrNoRows {
		return nil, model.ErrURLNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query URL: %w", err)
	}

	return u, nil
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

	device := DetectDeviceType(userAgent)

	query := `INSERT INTO clicks (url_id, ip_hash, country, city, user_agent, referrer, device)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := s.store.ServerDB.ExecContext(ctx, query, urlID, ipHash, country, city, userAgent, referrer, device)
	if err != nil {
		return fmt.Errorf("failed to record click: %w", err)
	}

	return nil
}

// LookupCountry resolves ipAddress to an ISO 3166-1 alpha-2 country code
// using the same GeoIP service RecordClick uses for analytics, for
// per-link geo-restriction enforcement at redirect time. Returns "" when
// GeoIP is disabled, the address is unparseable, or it is a private/
// loopback/link-local address (never blocked, per AI.md PART 20).
func (s *URLService) LookupCountry(ipAddress string) string {
	if s.geo == nil || !s.geo.Enabled() {
		return ""
	}
	ip := net.ParseIP(ipAddress)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return ""
	}
	return s.geo.LookupCountry(ip)
}

// CreateURLForUser creates a shortened URL owned by the given userID.
func (s *URLService) CreateURLForUser(ctx context.Context, userID int64, req *model.CreateURLRequest) (*model.URL, error) {
	// Validate URL
	if _, err := url.ParseRequestURI(req.LongURL); err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrInvalidURL, err)
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
		expiresAt = parseExpiration(req.ExpireAfter)
	}

	opts, err := buildLinkOptions(req)
	if err != nil {
		return nil, err
	}

	query := `INSERT INTO urls (short_code, long_url, title, description, user_id, custom_code, expires_at,
	          visibility, tags, utm_source, utm_medium, utm_campaign, utm_term, utm_content, geo_mode, geo_countries,
	          mobile_url, desktop_url, tablet_url)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := s.store.ServerDB.ExecContext(ctx, query,
		shortCode, req.LongURL, req.Title, req.Description, userID, isCustom, expiresAt,
		opts.visibility, opts.tags, opts.utmSource, opts.utmMedium, opts.utmCampaign, opts.utmTerm, opts.utmContent,
		opts.geoMode, opts.geoCountries, opts.mobileURL, opts.desktopURL, opts.tabletURL)
	if err != nil {
		return nil, fmt.Errorf("failed to insert URL: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get insert ID: %w", err)
	}

	urlRecord := &model.URL{
		ID:         id,
		ShortCode:  shortCode,
		LongURL:    req.LongURL,
		Title:      req.Title,
		UserID:     &userID,
		CustomCode: isCustom,
		ExpiresAt:  expiresAt,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	opts.applyTo(urlRecord)
	return urlRecord, nil
}

// CreateURLForOrg creates a shortened URL owned by the given org (used by
// org-scoped Bearer tokens, OwnerType "org" — see service/token.go). Mirrors
// CreateURLForUser but sets org_id instead of user_id, per AI.md PART 35 and
// PART 16 org-owned-link modeling.
func (s *URLService) CreateURLForOrg(ctx context.Context, orgID int64, req *model.CreateURLRequest) (*model.URL, error) {
	if _, err := url.ParseRequestURI(req.LongURL); err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrInvalidURL, err)
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
		expiresAt = parseExpiration(req.ExpireAfter)
	}

	opts, err := buildLinkOptions(req)
	if err != nil {
		return nil, err
	}

	query := `INSERT INTO urls (short_code, long_url, title, description, org_id, custom_code, expires_at,
	          visibility, tags, utm_source, utm_medium, utm_campaign, utm_term, utm_content, geo_mode, geo_countries,
	          mobile_url, desktop_url, tablet_url)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := s.store.ServerDB.ExecContext(ctx, query,
		shortCode, req.LongURL, req.Title, req.Description, orgID, isCustom, expiresAt,
		opts.visibility, opts.tags, opts.utmSource, opts.utmMedium, opts.utmCampaign, opts.utmTerm, opts.utmContent,
		opts.geoMode, opts.geoCountries, opts.mobileURL, opts.desktopURL, opts.tabletURL)
	if err != nil {
		return nil, fmt.Errorf("failed to insert URL: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get insert ID: %w", err)
	}

	urlRecord := &model.URL{
		ID:         id,
		ShortCode:  shortCode,
		LongURL:    req.LongURL,
		Title:      req.Title,
		OrgID:      &orgID,
		CustomCode: isCustom,
		ExpiresAt:  expiresAt,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	opts.applyTo(urlRecord)
	return urlRecord, nil
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
	query := `SELECT ` + urlSelectColumns + ` FROM urls WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := s.store.ServerDB.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list URLs: %w", err)
	}
	defer rows.Close()

	var urls []*model.URL
	for rows.Next() {
		u, err := scanURLRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan URL row: %w", err)
		}
		urls = append(urls, u)
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
	query := `SELECT ` + urlSelectColumns + ` FROM urls WHERE org_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := s.store.ServerDB.QueryContext(ctx, query, orgID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list URLs: %w", err)
	}
	defer rows.Close()

	var urls []*model.URL
	for rows.Next() {
		u, err := scanURLRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan URL row: %w", err)
		}
		urls = append(urls, u)
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

// linkOptions holds the parsed/validated per-link option fields shared by
// CreateURL, CreateURLForUser, and CreateURLForOrg (IDEA.md line 25/37 link
// options: tags, visibility, UTM passthrough, geo-restriction, device
// targeting). Optional fields are *string/nil so they bind as SQL NULL.
type linkOptions struct {
	visibility   string
	tags         *string
	utmSource    *string
	utmMedium    *string
	utmCampaign  *string
	utmTerm      *string
	utmContent   *string
	geoMode      string
	geoCountries *string
	mobileURL    *string
	desktopURL   *string
	tabletURL    *string
}

// applyTo copies the parsed options onto a URL record, e.g. after an insert
// whose returned struct is built by hand rather than re-queried.
func (o linkOptions) applyTo(u *model.URL) {
	u.Visibility = o.visibility
	u.Tags = splitCSV(derefStr(o.tags))
	u.UTMSource = o.utmSource
	u.UTMMedium = o.utmMedium
	u.UTMCampaign = o.utmCampaign
	u.UTMTerm = o.utmTerm
	u.UTMContent = o.utmContent
	u.GeoMode = o.geoMode
	u.GeoCountries = splitCSV(derefStr(o.geoCountries))
	u.MobileURL = o.mobileURL
	u.DesktopURL = o.desktopURL
	u.TabletURL = o.tabletURL
}

// buildLinkOptions validates and normalizes the link-option fields of a
// create request. GeoMode defaults to "none" and visibility to "public" per
// AI.md PART 20's allow/deny terminology mirrored at the link level.
func buildLinkOptions(req *model.CreateURLRequest) (linkOptions, error) {
	opts := linkOptions{visibility: "public", geoMode: "none"}
	if req.Visibility != "" {
		opts.visibility = req.Visibility
	}
	if req.GeoMode != "" {
		opts.geoMode = req.GeoMode
	}
	if tags := cleanStrings(req.Tags); len(tags) > 0 {
		opts.tags = strPtr(joinCSV(tags))
	}
	if len(req.GeoCountries) > 0 {
		codes, err := normalizeCountryCodes(req.GeoCountries)
		if err != nil {
			return opts, err
		}
		opts.geoCountries = strPtr(joinCSV(codes))
	}
	if opts.geoMode != "none" && opts.geoCountries == nil {
		return opts, fmt.Errorf("geo_countries is required when geo_mode is not none")
	}
	if req.UTMSource != "" {
		opts.utmSource = strPtr(req.UTMSource)
	}
	if req.UTMMedium != "" {
		opts.utmMedium = strPtr(req.UTMMedium)
	}
	if req.UTMCampaign != "" {
		opts.utmCampaign = strPtr(req.UTMCampaign)
	}
	if req.UTMTerm != "" {
		opts.utmTerm = strPtr(req.UTMTerm)
	}
	if req.UTMContent != "" {
		opts.utmContent = strPtr(req.UTMContent)
	}
	if req.MobileURL != "" {
		if _, err := url.ParseRequestURI(req.MobileURL); err != nil {
			return opts, fmt.Errorf("invalid mobile_url: %w", err)
		}
		opts.mobileURL = strPtr(req.MobileURL)
	}
	if req.DesktopURL != "" {
		if _, err := url.ParseRequestURI(req.DesktopURL); err != nil {
			return opts, fmt.Errorf("invalid desktop_url: %w", err)
		}
		opts.desktopURL = strPtr(req.DesktopURL)
	}
	if req.TabletURL != "" {
		if _, err := url.ParseRequestURI(req.TabletURL); err != nil {
			return opts, fmt.Errorf("invalid tablet_url: %w", err)
		}
		opts.tabletURL = strPtr(req.TabletURL)
	}
	return opts, nil
}

// normalizeCountryCodes upper-cases, trims, validates (ISO 3166-1 alpha-2:
// exactly 2 letters), and de-duplicates a list of country codes.
func normalizeCountryCodes(codes []string) ([]string, error) {
	out := make([]string, 0, len(codes))
	seen := make(map[string]bool, len(codes))
	for _, c := range codes {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" {
			continue
		}
		if len(c) != 2 || !isAlphaUpper(c) {
			return nil, fmt.Errorf("invalid country code %q (expected ISO 3166-1 alpha-2)", c)
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, nil
}

func isAlphaUpper(s string) bool {
	for _, ch := range s {
		if ch < 'A' || ch > 'Z' {
			return false
		}
	}
	return true
}

// cleanStrings trims and drops empty entries from a string slice.
func cleanStrings(vals []string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func joinCSV(vals []string) string {
	return strings.Join(vals, ",")
}

// splitCSV splits a stored comma-separated column back into a slice,
// dropping empties. Returns nil (not an empty slice) for "" so
// `omitempty` on the JSON tag hides the field when unset.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func strPtr(s string) *string { return &s }

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// DetectDeviceType classifies a User-Agent string as "mobile", "tablet", or
// "desktop" using simple substring heuristics (no external UA-parsing
// dependency is vendored — see TODO.AI.md). Used both for per-link device
// targeting (RedirectURL) and to populate clicks.device for analytics.
func DetectDeviceType(userAgent string) string {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "ipad") ||
		strings.Contains(ua, "tablet") ||
		(strings.Contains(ua, "android") && !strings.Contains(ua, "mobile")):
		return "tablet"
	case strings.Contains(ua, "mobi") ||
		strings.Contains(ua, "iphone") ||
		strings.Contains(ua, "ipod") ||
		strings.Contains(ua, "android"):
		return "mobile"
	default:
		return "desktop"
	}
}

// SelectDestination returns the link's device-targeted override URL for
// deviceType ("mobile"/"tablet"/"desktop"), falling back to u.LongURL when no
// override is set for that device (IDEA.md line 25/37 device targeting).
func SelectDestination(u *model.URL, deviceType string) string {
	switch deviceType {
	case "mobile":
		if u.MobileURL != nil && *u.MobileURL != "" {
			return *u.MobileURL
		}
	case "tablet":
		if u.TabletURL != nil && *u.TabletURL != "" {
			return *u.TabletURL
		}
	case "desktop":
		if u.DesktopURL != nil && *u.DesktopURL != "" {
			return *u.DesktopURL
		}
	}
	return u.LongURL
}

// ApplyUTM appends the link's static UTM query parameters to dest, without
// overwriting any UTM param the destination URL already carries (IDEA.md
// line 25/37 UTM passthrough). Returns dest unchanged (including on parse
// failure) when the link has no UTM fields set.
func ApplyUTM(dest string, u *model.URL) string {
	if u.UTMSource == nil && u.UTMMedium == nil && u.UTMCampaign == nil && u.UTMTerm == nil && u.UTMContent == nil {
		return dest
	}
	parsed, err := url.Parse(dest)
	if err != nil {
		return dest
	}
	q := parsed.Query()
	setIfAbsent := func(key string, val *string) {
		if val != nil && *val != "" && q.Get(key) == "" {
			q.Set(key, *val)
		}
	}
	setIfAbsent("utm_source", u.UTMSource)
	setIfAbsent("utm_medium", u.UTMMedium)
	setIfAbsent("utm_campaign", u.UTMCampaign)
	setIfAbsent("utm_term", u.UTMTerm)
	setIfAbsent("utm_content", u.UTMContent)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// GeoAllowed reports whether a visitor from countryCode (ISO 3166-1 alpha-2,
// empty when unknown) may follow this link, per its geo_mode/geo_countries
// (IDEA.md line 25/37 geo-restriction, mirroring AI.md PART 20's server-level
// country_mode/allow_countries/deny_countries terminology at link scope).
// An empty countryCode (GeoIP unavailable/private IP) always passes — GeoIP
// is a redirect gate here only when it positively identifies the country,
// consistent with IDEA.md's "GeoIP is a risk signal, not identity" caution.
func GeoAllowed(u *model.URL, countryCode string) bool {
	if u.GeoMode == "" || u.GeoMode == "none" || countryCode == "" {
		return true
	}
	countryCode = strings.ToUpper(countryCode)
	inList := false
	for _, c := range u.GeoCountries {
		if strings.EqualFold(c, countryCode) {
			inList = true
			break
		}
	}
	switch u.GeoMode {
	case "allow":
		return inList
	case "deny":
		return !inList
	default:
		return true
	}
}

// CanView reports whether the given Bearer/session caller may read a
// "private" link's details/stats. Admins and the owning user/org always can;
// "public" links (the default) are readable by anyone, matching the prior
// open-by-code behavior (IDEA.md line 25/37 visibility).
func CanView(u *model.URL, isAdmin bool, ownerType string, ownerID int64) bool {
	if u.Visibility != "private" {
		return true
	}
	if isAdmin {
		return true
	}
	if strings.EqualFold(ownerType, "org") {
		return u.OrgID != nil && *u.OrgID == ownerID
	}
	return u.UserID != nil && *u.UserID == ownerID
}

// hashIP hashes an IP address for privacy (per SPEC PART 36)
func hashIP(ip string) string {
	// Use daily salt for additional privacy
	salt := time.Now().Format("2006-01-02")
	data := fmt.Sprintf("%s:%s", ip, salt)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// parseExpiration converts an "expire after" duration string into an
// absolute expiry time. It returns nil for "never" and for any
// unrecognized value (the default is never-expires), so callers persist
// SQL NULL instead of a year-0001 timestamp that time.Now().After()
// would read as already expired — which previously killed the link the
// moment it was created.
func parseExpiration(duration string) *time.Time {
	now := time.Now()

	var exp time.Time
	switch duration {
	case "1h":
		exp = now.Add(1 * time.Hour)
	case "24h":
		exp = now.Add(24 * time.Hour)
	case "7d":
		exp = now.Add(7 * 24 * time.Hour)
	case "30d":
		exp = now.Add(30 * 24 * time.Hour)
	case "never":
		return nil
	default:
		return nil
	}
	return &exp
}
