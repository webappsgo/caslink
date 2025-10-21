package url

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

// Service handles URL shortening operations
type Service struct {
	db     *db.DB
	config *config.URLConfig
	logger *logrus.Logger
}

// NewService creates a new URL service
func NewService(database *db.DB, cfg *config.URLConfig, logger *logrus.Logger) *Service {
	return &Service{
		db:     database,
		config: cfg,
		logger: logger,
	}
}

// CreateURL creates a new shortened URL
func (s *Service) CreateURL(ctx context.Context, req *CreateURLRequest) (*db.URL, error) {
	// Validate the original URL
	if err := s.validateURL(req.OriginalURL); err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Generate or validate short code
	shortCode, isCustom, err := s.generateShortCode(ctx, req.CustomCode)
	if err != nil {
		return nil, fmt.Errorf("failed to generate short code: %w", err)
	}

	// Hash password if provided
	var passwordHash *string
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		hashStr := string(hash)
		passwordHash = &hashStr
	}

	// Parse expiration
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		expiresAt = req.ExpiresAt
	} else if req.ExpireAfter != "" {
		exp, err := s.parseExpiration(req.ExpireAfter)
		if err != nil {
			return nil, fmt.Errorf("invalid expiration: %w", err)
		}
		expiresAt = exp
	}

	// Create URL record
	urlRecord := &db.URL{
		ID:          shortCode,
		OriginalURL: req.OriginalURL,
		IsCustom:    isCustom,
		Title:       req.Title,
		Description: req.Description,
		UserID:      req.UserID,
		DomainID:    req.DomainID,
		Password:    passwordHash,
		Tags:        req.Tags,
		UTMSource:   req.UTMSource,
		UTMMedium:   req.UTMMedium,
		UTMCampaign: req.UTMCampaign,
		UTMTerm:     req.UTMTerm,
		UTMContent:  req.UTMContent,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
		Active:      true,
	}

	// Save to database
	if err := s.saveURL(ctx, urlRecord); err != nil {
		return nil, fmt.Errorf("failed to save URL: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"short_code":   shortCode,
		"original_url": req.OriginalURL,
		"user_id":      req.UserID,
		"is_custom":    isCustom,
	}).Info("URL created successfully")

	return urlRecord, nil
}

// GetURL retrieves a URL by its short code
func (s *Service) GetURL(ctx context.Context, shortCode string) (*db.URL, error) {
	query := `
		SELECT id, original_url, is_custom, title, description, favicon_url,
		       created_at, updated_at, expires_at, clicks, unique_clicks,
		       user_id, domain_id, active, password, tags,
		       utm_source, utm_medium, utm_campaign, utm_term, utm_content
		FROM urls
		WHERE id = ?
	`

	if s.db.Type() == "postgres" {
		query = strings.Replace(query, "?", "$1", 1)
	}

	row := s.db.QueryRow(ctx, query, shortCode)

	var urlRecord db.URL
	var title, description, faviconURL, userID, domainID, password, tags sql.NullString
	var utmSource, utmMedium, utmCampaign, utmTerm, utmContent sql.NullString
	var expiresAt sql.NullTime

	err := row.Scan(
		&urlRecord.ID,
		&urlRecord.OriginalURL,
		&urlRecord.IsCustom,
		&title,
		&description,
		&faviconURL,
		&urlRecord.CreatedAt,
		&urlRecord.UpdatedAt,
		&expiresAt,
		&urlRecord.Clicks,
		&urlRecord.UniqueClicks,
		&userID,
		&domainID,
		&urlRecord.Active,
		&password,
		&tags,
		&utmSource,
		&utmMedium,
		&utmCampaign,
		&utmTerm,
		&utmContent,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrURLNotFound
		}
		return nil, fmt.Errorf("failed to query URL: %w", err)
	}

	// Set nullable fields
	if title.Valid {
		urlRecord.Title = &title.String
	}
	if description.Valid {
		urlRecord.Description = &description.String
	}
	if faviconURL.Valid {
		urlRecord.FaviconURL = &faviconURL.String
	}
	if userID.Valid {
		urlRecord.UserID = &userID.String
	}
	if domainID.Valid {
		urlRecord.DomainID = &domainID.String
	}
	if password.Valid {
		urlRecord.Password = &password.String
	}
	if tags.Valid {
		urlRecord.Tags = &tags.String
	}
	if utmSource.Valid {
		urlRecord.UTMSource = &utmSource.String
	}
	if utmMedium.Valid {
		urlRecord.UTMMedium = &utmMedium.String
	}
	if utmCampaign.Valid {
		urlRecord.UTMCampaign = &utmCampaign.String
	}
	if utmTerm.Valid {
		urlRecord.UTMTerm = &utmTerm.String
	}
	if utmContent.Valid {
		urlRecord.UTMContent = &utmContent.String
	}
	if expiresAt.Valid {
		urlRecord.ExpiresAt = &expiresAt.Time
	}

	return &urlRecord, nil
}

// UpdateURL updates an existing URL
func (s *Service) UpdateURL(ctx context.Context, shortCode string, req *UpdateURLRequest) (*db.URL, error) {
	// Get existing URL
	existing, err := s.GetURL(ctx, shortCode)
	if err != nil {
		return nil, err
	}

	// Update fields
	if req.OriginalURL != nil {
		if err := s.validateURL(*req.OriginalURL); err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}
		existing.OriginalURL = *req.OriginalURL
	}

	if req.Title != nil {
		existing.Title = req.Title
	}

	if req.Description != nil {
		existing.Description = req.Description
	}

	if req.Tags != nil {
		existing.Tags = req.Tags
	}

	if req.Password != nil {
		if *req.Password == "" {
			existing.Password = nil
		} else {
			hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
			if err != nil {
				return nil, fmt.Errorf("failed to hash password: %w", err)
			}
			hashStr := string(hash)
			existing.Password = &hashStr
		}
	}

	if req.ExpiresAt != nil {
		existing.ExpiresAt = req.ExpiresAt
	}

	if req.Active != nil {
		existing.Active = *req.Active
	}

	existing.UpdatedAt = time.Now()

	// Save changes
	if err := s.updateURL(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update URL: %w", err)
	}

	return existing, nil
}

// DeleteURL deletes a URL
func (s *Service) DeleteURL(ctx context.Context, shortCode string) error {
	query := "DELETE FROM urls WHERE id = ?"
	if s.db.Type() == "postgres" {
		query = "DELETE FROM urls WHERE id = $1"
	}

	result, err := s.db.Exec(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to delete URL: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrURLNotFound
	}

	s.logger.WithField("short_code", shortCode).Info("URL deleted")
	return nil
}

// ListURLs lists URLs with pagination and filtering
func (s *Service) ListURLs(ctx context.Context, req *ListURLsRequest) (*ListURLsResponse, error) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	// Build WHERE conditions
	if req.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIndex))
		args = append(args, *req.UserID)
		argIndex++
	}

	if req.DomainID != nil {
		conditions = append(conditions, fmt.Sprintf("domain_id = $%d", argIndex))
		args = append(args, *req.DomainID)
		argIndex++
	}

	if req.Active != nil {
		conditions = append(conditions, fmt.Sprintf("active = $%d", argIndex))
		args = append(args, *req.Active)
		argIndex++
	}

	if req.Search != nil && *req.Search != "" {
		searchCondition := fmt.Sprintf("(original_url ILIKE $%d OR title ILIKE $%d OR description ILIKE $%d)", argIndex, argIndex+1, argIndex+2)
		conditions = append(conditions, searchCondition)
		searchTerm := "%" + *req.Search + "%"
		args = append(args, searchTerm, searchTerm, searchTerm)
		argIndex += 3
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM urls %s", whereClause)
	var total int64
	if err := s.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count URLs: %w", err)
	}

	// Build main query
	orderBy := "created_at DESC"
	if req.SortBy != nil {
		switch *req.SortBy {
		case "created_at", "updated_at", "clicks", "unique_clicks":
			direction := "DESC"
			if req.SortDirection != nil && *req.SortDirection == "asc" {
				direction = "ASC"
			}
			orderBy = fmt.Sprintf("%s %s", *req.SortBy, direction)
		}
	}

	limit := int64(50)
	if req.Limit != nil && *req.Limit > 0 && *req.Limit <= 100 {
		limit = *req.Limit
	}

	offset := int64(0)
	if req.Offset != nil && *req.Offset > 0 {
		offset = *req.Offset
	}

	query := fmt.Sprintf(`
		SELECT id, original_url, is_custom, title, description, favicon_url,
		       created_at, updated_at, expires_at, clicks, unique_clicks,
		       user_id, domain_id, active, tags
		FROM urls
		%s
		ORDER BY %s
		LIMIT $%d OFFSET $%d
	`, whereClause, orderBy, argIndex, argIndex+1)

	args = append(args, limit, offset)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query URLs: %w", err)
	}
	defer rows.Close()

	var urls []*db.URL
	for rows.Next() {
		var urlRecord db.URL
		var title, description, faviconURL, userID, domainID, tags sql.NullString
		var expiresAt sql.NullTime

		err := rows.Scan(
			&urlRecord.ID,
			&urlRecord.OriginalURL,
			&urlRecord.IsCustom,
			&title,
			&description,
			&faviconURL,
			&urlRecord.CreatedAt,
			&urlRecord.UpdatedAt,
			&expiresAt,
			&urlRecord.Clicks,
			&urlRecord.UniqueClicks,
			&userID,
			&domainID,
			&urlRecord.Active,
			&tags,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan URL: %w", err)
		}

		// Set nullable fields
		if title.Valid {
			urlRecord.Title = &title.String
		}
		if description.Valid {
			urlRecord.Description = &description.String
		}
		if faviconURL.Valid {
			urlRecord.FaviconURL = &faviconURL.String
		}
		if userID.Valid {
			urlRecord.UserID = &userID.String
		}
		if domainID.Valid {
			urlRecord.DomainID = &domainID.String
		}
		if tags.Valid {
			urlRecord.Tags = &tags.String
		}
		if expiresAt.Valid {
			urlRecord.ExpiresAt = &expiresAt.Time
		}

		urls = append(urls, &urlRecord)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate URLs: %w", err)
	}

	return &ListURLsResponse{
		URLs:   urls,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// IncrementClick increments the click count for a URL
func (s *Service) IncrementClick(ctx context.Context, shortCode string, isUnique bool) error {
	var query string
	if isUnique {
		query = "UPDATE urls SET clicks = clicks + 1, unique_clicks = unique_clicks + 1, updated_at = ? WHERE id = ?"
	} else {
		query = "UPDATE urls SET clicks = clicks + 1, updated_at = ? WHERE id = ?"
	}

	if s.db.Type() == "postgres" {
		if isUnique {
			query = "UPDATE urls SET clicks = clicks + 1, unique_clicks = unique_clicks + 1, updated_at = $1 WHERE id = $2"
		} else {
			query = "UPDATE urls SET clicks = clicks + 1, updated_at = $1 WHERE id = $2"
		}
	}

	_, err := s.db.Exec(ctx, query, time.Now(), shortCode)
	if err != nil {
		return fmt.Errorf("failed to increment click count: %w", err)
	}

	return nil
}

// ValidatePassword validates a password for a protected URL
func (s *Service) ValidatePassword(ctx context.Context, shortCode, password string) error {
	urlRecord, err := s.GetURL(ctx, shortCode)
	if err != nil {
		return err
	}

	if urlRecord.Password == nil {
		return ErrPasswordNotRequired
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*urlRecord.Password), []byte(password)); err != nil {
		return ErrInvalidPassword
	}

	return nil
}

// IsExpired checks if a URL has expired
func (s *Service) IsExpired(urlRecord *db.URL) bool {
	if urlRecord.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*urlRecord.ExpiresAt)
}

// validateURL validates a URL format and length
func (s *Service) validateURL(rawURL string) error {
	if len(rawURL) > s.config.MaxURLLength {
		return fmt.Errorf("URL too long (max %d characters)", s.config.MaxURLLength)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("URL must include scheme and host")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("only HTTP and HTTPS URLs are supported")
	}

	return nil
}

// generateShortCode generates or validates a short code
func (s *Service) generateShortCode(ctx context.Context, customCode string) (string, bool, error) {
	if customCode != "" {
		// Validate custom code
		if err := s.validateCustomCode(customCode); err != nil {
			return "", false, err
		}

		// Check if custom code is available
		if exists, err := s.codeExists(ctx, customCode); err != nil {
			return "", false, err
		} else if exists {
			return "", false, ErrCodeAlreadyExists
		}

		return customCode, true, nil
	}

	// Generate random code
	for attempts := 0; attempts < 100; attempts++ {
		code := s.generateRandomCode()
		if exists, err := s.codeExists(ctx, code); err != nil {
			return "", false, err
		} else if !exists {
			return code, false, nil
		}
	}

	return "", false, fmt.Errorf("failed to generate unique code after 100 attempts")
}

// validateCustomCode validates a custom code
func (s *Service) validateCustomCode(code string) error {
	if len(code) < s.config.CustomCodeMinLength {
		return fmt.Errorf("custom code too short (min %d characters)", s.config.CustomCodeMinLength)
	}

	if len(code) > s.config.CustomCodeMaxLength {
		return fmt.Errorf("custom code too long (max %d characters)", s.config.CustomCodeMaxLength)
	}

	// Check allowed characters
	allowedChars := s.config.AllowedCharacters
	if s.config.ExcludeSimilarChars {
		allowedChars = strings.ReplaceAll(allowedChars, "0", "")
		allowedChars = strings.ReplaceAll(allowedChars, "O", "")
		allowedChars = strings.ReplaceAll(allowedChars, "1", "")
		allowedChars = strings.ReplaceAll(allowedChars, "l", "")
		allowedChars = strings.ReplaceAll(allowedChars, "I", "")
	}

	for _, char := range code {
		if !strings.ContainsRune(allowedChars, char) {
			return fmt.Errorf("invalid character in custom code: %c", char)
		}
	}

	// Check reserved words
	for _, reserved := range s.config.ReservedWords {
		if strings.EqualFold(code, reserved) {
			return fmt.Errorf("code is reserved: %s", code)
		}
	}

	return nil
}

// generateRandomCode generates a random short code
func (s *Service) generateRandomCode() string {
	chars := s.config.AllowedCharacters
	if s.config.ExcludeSimilarChars {
		chars = strings.ReplaceAll(chars, "0", "")
		chars = strings.ReplaceAll(chars, "O", "")
		chars = strings.ReplaceAll(chars, "1", "")
		chars = strings.ReplaceAll(chars, "l", "")
		chars = strings.ReplaceAll(chars, "I", "")
	}

	length := s.config.MinRandomLength + (s.generateSeed() % (s.config.MaxRandomLength - s.config.MinRandomLength + 1))

	code := make([]byte, length)
	for i := range code {
		code[i] = chars[s.generateSeed()%len(chars)]
	}

	return string(code)
}

// generateSeed generates a pseudo-random seed
func (s *Service) generateSeed() int {
	return int(time.Now().UnixNano() % 1000000)
}

// codeExists checks if a short code already exists
func (s *Service) codeExists(ctx context.Context, code string) (bool, error) {
	query := "SELECT COUNT(*) FROM urls WHERE id = ?"
	if s.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM urls WHERE id = $1"
	}

	var count int
	err := s.db.QueryRow(ctx, query, code).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check code existence: %w", err)
	}

	return count > 0, nil
}

// parseExpiration parses expiration string
func (s *Service) parseExpiration(expireAfter string) (*time.Time, error) {
	switch expireAfter {
	case "1h":
		exp := time.Now().Add(time.Hour)
		return &exp, nil
	case "24h":
		exp := time.Now().Add(24 * time.Hour)
		return &exp, nil
	case "7d":
		exp := time.Now().Add(7 * 24 * time.Hour)
		return &exp, nil
	case "30d":
		exp := time.Now().Add(30 * 24 * time.Hour)
		return &exp, nil
	case "never", "":
		return nil, nil
	default:
		// Try to parse as duration
		duration, err := time.ParseDuration(expireAfter)
		if err != nil {
			return nil, fmt.Errorf("invalid expiration format: %s", expireAfter)
		}
		exp := time.Now().Add(duration)
		return &exp, nil
	}
}

// saveURL saves a URL to the database
func (s *Service) saveURL(ctx context.Context, urlRecord *db.URL) error {
	query := `
		INSERT INTO urls (
			id, original_url, is_custom, title, description, user_id, domain_id,
			password, tags, utm_source, utm_medium, utm_campaign, utm_term, utm_content,
			created_at, updated_at, expires_at, active
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	if s.db.Type() == "postgres" {
		query = `
			INSERT INTO urls (
				id, original_url, is_custom, title, description, user_id, domain_id,
				password, tags, utm_source, utm_medium, utm_campaign, utm_term, utm_content,
				created_at, updated_at, expires_at, active
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		`
	}

	_, err := s.db.Exec(ctx, query,
		urlRecord.ID,
		urlRecord.OriginalURL,
		urlRecord.IsCustom,
		urlRecord.Title,
		urlRecord.Description,
		urlRecord.UserID,
		urlRecord.DomainID,
		urlRecord.Password,
		urlRecord.Tags,
		urlRecord.UTMSource,
		urlRecord.UTMMedium,
		urlRecord.UTMCampaign,
		urlRecord.UTMTerm,
		urlRecord.UTMContent,
		urlRecord.CreatedAt,
		urlRecord.UpdatedAt,
		urlRecord.ExpiresAt,
		urlRecord.Active,
	)

	return err
}

// updateURL updates a URL in the database
func (s *Service) updateURL(ctx context.Context, urlRecord *db.URL) error {
	query := `
		UPDATE urls SET
			original_url = ?, title = ?, description = ?, password = ?, tags = ?,
			utm_source = ?, utm_medium = ?, utm_campaign = ?, utm_term = ?, utm_content = ?,
			updated_at = ?, expires_at = ?, active = ?
		WHERE id = ?
	`

	if s.db.Type() == "postgres" {
		query = `
			UPDATE urls SET
				original_url = $1, title = $2, description = $3, password = $4, tags = $5,
				utm_source = $6, utm_medium = $7, utm_campaign = $8, utm_term = $9, utm_content = $10,
				updated_at = $11, expires_at = $12, active = $13
			WHERE id = $14
		`
	}

	_, err := s.db.Exec(ctx, query,
		urlRecord.OriginalURL,
		urlRecord.Title,
		urlRecord.Description,
		urlRecord.Password,
		urlRecord.Tags,
		urlRecord.UTMSource,
		urlRecord.UTMMedium,
		urlRecord.UTMCampaign,
		urlRecord.UTMTerm,
		urlRecord.UTMContent,
		urlRecord.UpdatedAt,
		urlRecord.ExpiresAt,
		urlRecord.Active,
		urlRecord.ID,
	)

	return err
}