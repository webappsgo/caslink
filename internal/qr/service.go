package qr

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/skip2/go-qrcode"
)

// Service handles QR code generation and management
type Service struct {
	db        *db.DB
	config    *config.QRConfig
	logger    *logrus.Logger
	generator *Generator
	cache     map[string]*QRCode // Simple in-memory cache
}

// NewService creates a new QR code service
func NewService(database *db.DB, cfg *config.QRConfig, logger *logrus.Logger) (*Service, error) {
	generator, err := NewGenerator(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create QR generator: %w", err)
	}

	return &Service{
		db:        database,
		config:    cfg,
		logger:    logger,
		generator: generator,
		cache:     make(map[string]*QRCode),
	}, nil
}

// GenerateQRCode generates a QR code for a URL
func (s *Service) GenerateQRCode(ctx context.Context, req *QRRequest) (*QRCode, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("QR code generation is disabled")
	}

	// Validate request
	if err := s.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid QR request: %w", err)
	}

	// Check cache first
	cacheKey := s.generateCacheKey(req)
	if cached, exists := s.cache[cacheKey]; exists {
		s.logger.WithField("cache_key", cacheKey).Debug("QR code found in cache")
		return cached, nil
	}

	// Check database cache
	if dbCached, err := s.getFromDatabase(ctx, req); err == nil && dbCached != nil {
		s.logger.WithField("url_id", req.URLID).Debug("QR code found in database cache")
		s.cache[cacheKey] = dbCached
		return dbCached, nil
	}

	// Generate new QR code
	s.logger.WithFields(logrus.Fields{
		"url_id": req.URLID,
		"format": req.Format,
		"size":   req.Size,
		"style":  req.Style,
	}).Info("Generating new QR code")

	qrCode, err := s.generator.GenerateQRCode(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	// Store in database cache
	if err := s.storeInDatabase(ctx, qrCode); err != nil {
		s.logger.WithError(err).Warn("Failed to store QR code in database cache")
	}

	// Store in memory cache
	s.cache[cacheKey] = qrCode

	return qrCode, nil
}

// GenerateBatchQRCodes generates QR codes for multiple URLs
func (s *Service) GenerateBatchQRCodes(ctx context.Context, requests []*QRRequest) ([]*QRCode, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("QR code generation is disabled")
	}

	if len(requests) == 0 {
		return []*QRCode{}, nil
	}

	if len(requests) > s.config.MaxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum allowed %d", len(requests), s.config.MaxBatchSize)
	}

	s.logger.WithField("batch_size", len(requests)).Info("Generating batch QR codes")

	var qrCodes []*QRCode
	var errors []error

	for i, req := range requests {
		qrCode, err := s.GenerateQRCode(ctx, req)
		if err != nil {
			s.logger.WithError(err).WithField("request_index", i).Error("Failed to generate QR code in batch")
			errors = append(errors, fmt.Errorf("request %d: %w", i, err))
			continue
		}
		qrCodes = append(qrCodes, qrCode)
	}

	if len(errors) > 0 {
		return qrCodes, fmt.Errorf("batch generation completed with %d errors", len(errors))
	}

	return qrCodes, nil
}

// GetQRCode retrieves an existing QR code
func (s *Service) GetQRCode(ctx context.Context, qrID string) (*QRCode, error) {
	query := `
		SELECT id, url_id, format, size, style, foreground_color, background_color,
		       logo_url, data, created_at
		FROM qr_codes
		WHERE id = ?`

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$1")
	}

	var qr QRCode
	err := s.db.QueryRow(ctx, query, qrID).Scan(
		&qr.ID, &qr.URLID, &qr.Format, &qr.Size, &qr.Style,
		&qr.ForegroundColor, &qr.BackgroundColor, &qr.LogoURL,
		&qr.Data, &qr.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get QR code: %w", err)
	}

	return &qr, nil
}

// ListQRCodes lists QR codes for a URL
func (s *Service) ListQRCodes(ctx context.Context, urlID string) ([]*QRCode, error) {
	query := `
		SELECT id, url_id, format, size, style, foreground_color, background_color,
		       logo_url, data, created_at
		FROM qr_codes
		WHERE url_id = ?
		ORDER BY created_at DESC`

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$1")
	}

	rows, err := s.db.Query(ctx, query, urlID)
	if err != nil {
		return nil, fmt.Errorf("failed to list QR codes: %w", err)
	}
	defer rows.Close()

	var qrCodes []*QRCode
	for rows.Next() {
		var qr QRCode
		err := rows.Scan(
			&qr.ID, &qr.URLID, &qr.Format, &qr.Size, &qr.Style,
			&qr.ForegroundColor, &qr.BackgroundColor, &qr.LogoURL,
			&qr.Data, &qr.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan QR code: %w", err)
		}
		qrCodes = append(qrCodes, &qr)
	}

	return qrCodes, rows.Err()
}

// DeleteQRCode deletes a QR code
func (s *Service) DeleteQRCode(ctx context.Context, qrID string) error {
	query := `DELETE FROM qr_codes WHERE id = ?`

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$1")
	}

	result, err := s.db.Exec(ctx, query, qrID)
	if err != nil {
		return fmt.Errorf("failed to delete QR code: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("QR code not found")
	}

	// Remove from cache
	for key, cached := range s.cache {
		if cached.ID == qrID {
			delete(s.cache, key)
			break
		}
	}

	return nil
}

// CleanupOldQRCodes removes old QR codes based on retention policy
func (s *Service) CleanupOldQRCodes(ctx context.Context) (int64, error) {
	if s.config.RetentionDays <= 0 {
		return 0, nil // Unlimited retention
	}

	cutoffDate := time.Now().AddDate(0, 0, -s.config.RetentionDays)

	query := `DELETE FROM qr_codes WHERE created_at < ?`

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$1")
	}

	result, err := s.db.Exec(ctx, query, cutoffDate)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old QR codes: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"cutoff_date":     cutoffDate,
		"deleted_count":   rowsAffected,
		"retention_days":  s.config.RetentionDays,
	}).Info("QR codes cleanup completed")

	// Clear cache to ensure consistency
	s.cache = make(map[string]*QRCode)

	return rowsAffected, nil
}

// CustomizeQRCode applies custom styling to an existing QR code
func (s *Service) CustomizeQRCode(ctx context.Context, qrID string, customization *QRCustomization) (*QRCode, error) {
	// Get existing QR code
	existing, err := s.GetQRCode(ctx, qrID)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing QR code: %w", err)
	}

	// Create new request with customization
	req := &QRRequest{
		URLID:           existing.URLID,
		Format:          existing.Format,
		Size:            existing.Size,
		Style:           customization.Style,
		ForegroundColor: customization.ForegroundColor,
		BackgroundColor: customization.BackgroundColor,
		LogoURL:         customization.LogoURL,
		LogoSize:        customization.LogoSize,
	}

	// Generate new customized QR code
	return s.GenerateQRCode(ctx, req)
}

// GetQRCodeStats returns statistics about QR code usage
func (s *Service) GetQRCodeStats(ctx context.Context) (*QRStats, error) {
	query := `
		SELECT
			COUNT(*) as total_codes,
			COUNT(DISTINCT url_id) as unique_urls,
			COUNT(DISTINCT format) as formats_used,
			AVG(size) as avg_size
		FROM qr_codes`

	var stats QRStats
	err := s.db.QueryRow(ctx, query).Scan(
		&stats.TotalCodes, &stats.UniqueURLs, &stats.FormatsUsed, &stats.AverageSize,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get QR code stats: %w", err)
	}

	// Get format breakdown
	formatQuery := `
		SELECT format, COUNT(*) as count
		FROM qr_codes
		GROUP BY format
		ORDER BY count DESC`

	rows, err := s.db.Query(ctx, formatQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get format breakdown: %w", err)
	}
	defer rows.Close()

	stats.FormatBreakdown = make(map[string]int64)
	for rows.Next() {
		var format string
		var count int64
		if err := rows.Scan(&format, &count); err != nil {
			return nil, fmt.Errorf("failed to scan format breakdown: %w", err)
		}
		stats.FormatBreakdown[format] = count
	}

	return &stats, nil
}

// Helper methods

func (s *Service) validateRequest(req *QRRequest) error {
	if req.URLID == "" {
		return fmt.Errorf("URL ID is required")
	}

	if req.Size < s.config.MinSize || req.Size > s.config.MaxSize {
		return fmt.Errorf("size %d is outside allowed range %d-%d", req.Size, s.config.MinSize, s.config.MaxSize)
	}

	if !s.isValidFormat(req.Format) {
		return fmt.Errorf("unsupported format: %s", req.Format)
	}

	if !s.isValidStyle(req.Style) {
		return fmt.Errorf("unsupported style: %s", req.Style)
	}

	if req.ForegroundColor != "" && !s.isValidColor(req.ForegroundColor) {
		return fmt.Errorf("invalid foreground color: %s", req.ForegroundColor)
	}

	if req.BackgroundColor != "" && !s.isValidColor(req.BackgroundColor) {
		return fmt.Errorf("invalid background color: %s", req.BackgroundColor)
	}

	if req.LogoURL != "" && !s.isValidURL(req.LogoURL) {
		return fmt.Errorf("invalid logo URL: %s", req.LogoURL)
	}

	if req.LogoSize > s.config.MaxLogoSize {
		return fmt.Errorf("logo size %d exceeds maximum %d", req.LogoSize, s.config.MaxLogoSize)
	}

	return nil
}

func (s *Service) isValidFormat(format string) bool {
	validFormats := map[string]bool{
		"png": true,
		"svg": true,
		"pdf": true,
		"jpg": true,
		"jpeg": true,
	}
	return validFormats[strings.ToLower(format)]
}

func (s *Service) isValidStyle(style string) bool {
	validStyles := map[string]bool{
		"square":  true,
		"circle":  true,
		"rounded": true,
	}
	return validStyles[strings.ToLower(style)]
}

func (s *Service) isValidColor(colorStr string) bool {
	// Check if it's a valid hex color
	if strings.HasPrefix(colorStr, "#") && len(colorStr) == 7 {
		for _, r := range colorStr[1:] {
			if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F') || (r >= 'a' && r <= 'f')) {
				return false
			}
		}
		return true
	}
	return false
}

func (s *Service) isValidURL(urlStr string) bool {
	if urlStr == "" {
		return true
	}
	_, err := url.Parse(urlStr)
	return err == nil
}

func (s *Service) generateCacheKey(req *QRRequest) string {
	return fmt.Sprintf("%s_%s_%d_%s_%s_%s_%s_%d",
		req.URLID, req.Format, req.Size, req.Style,
		req.ForegroundColor, req.BackgroundColor, req.LogoURL, req.LogoSize)
}

func (s *Service) getFromDatabase(ctx context.Context, req *QRRequest) (*QRCode, error) {
	query := `
		SELECT id, url_id, format, size, style, foreground_color, background_color,
		       logo_url, data, created_at
		FROM qr_codes
		WHERE url_id = ? AND format = ? AND size = ? AND style = ?
		  AND COALESCE(foreground_color, '') = ? AND COALESCE(background_color, '') = ?
		  AND COALESCE(logo_url, '') = ?
		ORDER BY created_at DESC
		LIMIT 1`

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		for i := 1; i <= 7; i++ {
			query = strings.Replace(query, "$", fmt.Sprintf("$%d", i), 1)
		}
	}

	var qr QRCode
	err := s.db.QueryRow(ctx, query,
		req.URLID, req.Format, req.Size, req.Style,
		req.ForegroundColor, req.BackgroundColor, req.LogoURL,
	).Scan(
		&qr.ID, &qr.URLID, &qr.Format, &qr.Size, &qr.Style,
		&qr.ForegroundColor, &qr.BackgroundColor, &qr.LogoURL,
		&qr.Data, &qr.CreatedAt,
	)

	if err != nil {
		return nil, err // Not found or other error
	}

	return &qr, nil
}

func (s *Service) storeInDatabase(ctx context.Context, qr *QRCode) error {
	query := `
		INSERT INTO qr_codes
		(id, url_id, format, size, style, foreground_color, background_color, logo_url, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		for i := 1; i <= 10; i++ {
			query = strings.Replace(query, "$", fmt.Sprintf("$%d", i), 1)
		}
	}

	_, err := s.db.Exec(ctx, query,
		qr.ID, qr.URLID, qr.Format, qr.Size, qr.Style,
		qr.ForegroundColor, qr.BackgroundColor, qr.LogoURL,
		qr.Data, qr.CreatedAt,
	)

	return err
}

// downloadLogo downloads a logo image from URL
func (s *Service) downloadLogo(logoURL string) (image.Image, error) {
	if logoURL == "" {
		return nil, nil
	}

	resp, err := http.Get(logoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download logo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download logo: HTTP %d", resp.StatusCode)
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode logo image: %w", err)
	}

	return img, nil
}

// resizeImage resizes an image to the specified dimensions
func (s *Service) resizeImage(src image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	// Simple nearest-neighbor scaling
	srcBounds := src.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := (x * srcWidth) / width
			srcY := (y * srcHeight) / height
			dst.Set(x, y, src.At(srcBounds.Min.X+srcX, srcBounds.Min.Y+srcY))
		}
	}

	return dst
}

// hexToColor converts a hex color string to color.RGBA
func hexToColor(hex string) color.RGBA {
	if hex == "" || !strings.HasPrefix(hex, "#") || len(hex) != 7 {
		return color.RGBA{0, 0, 0, 255} // Default to black
	}

	var r, g, b uint8
	fmt.Sscanf(hex[1:3], "%02x", &r)
	fmt.Sscanf(hex[3:5], "%02x", &g)
	fmt.Sscanf(hex[5:7], "%02x", &b)

	return color.RGBA{r, g, b, 255}
}