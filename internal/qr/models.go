package qr

import (
	"fmt"
	"time"
)

// QRCode represents a generated QR code
type QRCode struct {
	ID              string    `json:"id"`
	URLID           string    `json:"url_id"`
	Format          string    `json:"format"`
	Size            int       `json:"size"`
	Style           string    `json:"style"`
	ForegroundColor string    `json:"foreground_color,omitempty"`
	BackgroundColor string    `json:"background_color,omitempty"`
	LogoURL         string    `json:"logo_url,omitempty"`
	Data            []byte    `json:"data"`
	CreatedAt       time.Time `json:"created_at"`
}

// QRRequest represents a request to generate a QR code
type QRRequest struct {
	URLID           string `json:"url_id"`
	Format          string `json:"format"`           // png, svg, pdf, jpg
	Size            int    `json:"size"`             // Size in pixels
	Style           string `json:"style"`            // square, circle, rounded
	ForegroundColor string `json:"foreground_color,omitempty"` // Hex color (e.g., #000000)
	BackgroundColor string `json:"background_color,omitempty"` // Hex color (e.g., #FFFFFF)
	LogoURL         string `json:"logo_url,omitempty"`         // URL to logo image
	LogoSize        int    `json:"logo_size,omitempty"`        // Logo size in pixels
	ErrorCorrection string `json:"error_correction,omitempty"` // L, M, Q, H
	Margin          int    `json:"margin,omitempty"`           // Margin in pixels
}

// QRResponse represents a response containing a generated QR code
type QRResponse struct {
	QRCode      *QRCode `json:"qr_code"`
	DownloadURL string  `json:"download_url"`
	EmbedURL    string  `json:"embed_url"`
}

// QRCustomization represents customization options for QR codes
type QRCustomization struct {
	Style           string `json:"style"`
	ForegroundColor string `json:"foreground_color,omitempty"`
	BackgroundColor string `json:"background_color,omitempty"`
	LogoURL         string `json:"logo_url,omitempty"`
	LogoSize        int    `json:"logo_size,omitempty"`
	ErrorCorrection string `json:"error_correction,omitempty"`
	Margin          int    `json:"margin,omitempty"`
}

// QRStats represents statistics about QR code usage
type QRStats struct {
	TotalCodes      int64              `json:"total_codes"`
	UniqueURLs      int64              `json:"unique_urls"`
	FormatsUsed     int64              `json:"formats_used"`
	AverageSize     float64            `json:"average_size"`
	FormatBreakdown map[string]int64   `json:"format_breakdown"`
	CreatedToday    int64              `json:"created_today"`
	CreatedThisWeek int64              `json:"created_this_week"`
	PopularSizes    []SizeUsage        `json:"popular_sizes"`
	PopularStyles   []StyleUsage       `json:"popular_styles"`
}

// SizeUsage represents usage statistics for QR code sizes
type SizeUsage struct {
	Size  int   `json:"size"`
	Count int64 `json:"count"`
}

// StyleUsage represents usage statistics for QR code styles
type StyleUsage struct {
	Style string `json:"style"`
	Count int64  `json:"count"`
}

// QRBatchRequest represents a request to generate multiple QR codes
type QRBatchRequest struct {
	Requests []*QRRequest `json:"requests"`
	Format   string       `json:"format,omitempty"`   // Default format for all
	Size     int          `json:"size,omitempty"`     // Default size for all
	Style    string       `json:"style,omitempty"`    // Default style for all
}

// QRBatchResponse represents a response for batch QR code generation
type QRBatchResponse struct {
	QRCodes     []*QRCode `json:"qr_codes"`
	Successful  int       `json:"successful"`
	Failed      int       `json:"failed"`
	Errors      []string  `json:"errors,omitempty"`
	DownloadURL string    `json:"download_url,omitempty"` // URL to download all as ZIP
}

// QRTemplate represents a template for QR code generation
type QRTemplate struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	Format          string    `json:"format"`
	Size            int       `json:"size"`
	Style           string    `json:"style"`
	ForegroundColor string    `json:"foreground_color,omitempty"`
	BackgroundColor string    `json:"background_color,omitempty"`
	LogoURL         string    `json:"logo_url,omitempty"`
	LogoSize        int       `json:"logo_size,omitempty"`
	ErrorCorrection string    `json:"error_correction,omitempty"`
	Margin          int       `json:"margin,omitempty"`
	UserID          string    `json:"user_id"`
	IsPublic        bool      `json:"is_public"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	UsageCount      int64     `json:"usage_count"`
}

// QRTemplateRequest represents a request to create or update a QR template
type QRTemplateRequest struct {
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Format          string `json:"format"`
	Size            int    `json:"size"`
	Style           string `json:"style"`
	ForegroundColor string `json:"foreground_color,omitempty"`
	BackgroundColor string `json:"background_color,omitempty"`
	LogoURL         string `json:"logo_url,omitempty"`
	LogoSize        int    `json:"logo_size,omitempty"`
	ErrorCorrection string `json:"error_correction,omitempty"`
	Margin          int    `json:"margin,omitempty"`
	IsPublic        bool   `json:"is_public"`
}

// QRAnalytics represents analytics data for QR code usage
type QRAnalytics struct {
	QRCodeID    string    `json:"qr_code_id"`
	Downloads   int64     `json:"downloads"`
	Views       int64     `json:"views"`
	LastUsed    time.Time `json:"last_used,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	PopularFormats []FormatUsage `json:"popular_formats"`
}

// FormatUsage represents usage statistics for QR code formats
type FormatUsage struct {
	Format string `json:"format"`
	Count  int64  `json:"count"`
}

// QRExportRequest represents a request to export QR codes
type QRExportRequest struct {
	URLIDs      []string  `json:"url_ids,omitempty"`      // Specific URLs to export
	Format      string    `json:"format"`                 // Export format (zip, pdf)
	QRFormat    string    `json:"qr_format,omitempty"`    // QR code format (png, svg)
	Size        int       `json:"size,omitempty"`         // QR code size
	Style       string    `json:"style,omitempty"`        // QR code style
	IncludeData bool      `json:"include_data"`           // Include URL data in export
	StartDate   time.Time `json:"start_date,omitempty"`   // Filter by creation date
	EndDate     time.Time `json:"end_date,omitempty"`     // Filter by creation date
}

// QRExportResponse represents a response for QR code export
type QRExportResponse struct {
	Content     []byte `json:"content"`
	ContentType string `json:"content_type"`
	Filename    string `json:"filename"`
}

// QRValidationResult represents the result of QR code validation
type QRValidationResult struct {
	Valid       bool     `json:"valid"`
	Errors      []string `json:"errors,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	Format      string   `json:"format"`
	Size        int      `json:"size"`
	DataSize    int      `json:"data_size"`
	Readable    bool     `json:"readable"`
	URL         string   `json:"url,omitempty"`
}

// QRSettings represents user preferences for QR code generation
type QRSettings struct {
	UserID          string `json:"user_id"`
	DefaultFormat   string `json:"default_format"`
	DefaultSize     int    `json:"default_size"`
	DefaultStyle    string `json:"default_style"`
	DefaultFgColor  string `json:"default_fg_color,omitempty"`
	DefaultBgColor  string `json:"default_bg_color,omitempty"`
	DefaultLogo     string `json:"default_logo,omitempty"`
	AutoGenerate    bool   `json:"auto_generate"`    // Auto-generate QR on URL creation
	CacheEnabled    bool   `json:"cache_enabled"`    // Enable QR code caching
	IncludeBranding bool   `json:"include_branding"` // Include service branding
	UpdatedAt       time.Time `json:"updated_at"`
}

// QRMetrics represents metrics for QR code performance
type QRMetrics struct {
	QRCodeID         string            `json:"qr_code_id"`
	ScanCount        int64             `json:"scan_count"`
	UniqueScans      int64             `json:"unique_scans"`
	ScansByCountry   map[string]int64  `json:"scans_by_country"`
	ScansByDevice    map[string]int64  `json:"scans_by_device"`
	ScansByApp       map[string]int64  `json:"scans_by_app"`
	AverageScanTime  float64           `json:"average_scan_time"`
	SuccessRate      float64           `json:"success_rate"`
	LastScanAt       time.Time         `json:"last_scan_at,omitempty"`
	PeakScanHour     int               `json:"peak_scan_hour"`
	WeeklyScanTrend  []int64           `json:"weekly_scan_trend"`
}

// QRErrorCorrection levels
const (
	ErrorCorrectionLow     = "L" // ~7% recovery
	ErrorCorrectionMedium  = "M" // ~15% recovery
	ErrorCorrectionQuartile = "Q" // ~25% recovery
	ErrorCorrectionHigh    = "H" // ~30% recovery
)

// QR code formats
const (
	FormatPNG  = "png"
	FormatSVG  = "svg"
	FormatPDF  = "pdf"
	FormatJPG  = "jpg"
	FormatJPEG = "jpeg"
)

// QR code styles
const (
	StyleSquare  = "square"
	StyleCircle  = "circle"
	StyleRounded = "rounded"
)

// Default values
const (
	DefaultSize            = 200
	DefaultFormat          = FormatPNG
	DefaultStyle           = StyleSquare
	DefaultErrorCorrection = ErrorCorrectionMedium
	DefaultMargin          = 4
	DefaultForegroundColor = "#000000"
	DefaultBackgroundColor = "#FFFFFF"
	MinSize                = 50
	MaxSize                = 2000
	MaxLogoSize            = 100
	MaxBatchSize           = 100
)

// ValidateQRRequest validates a QR code generation request
func (req *QRRequest) Validate() error {
	if req.URLID == "" {
		return fmt.Errorf("URL ID is required")
	}

	if req.Size < MinSize || req.Size > MaxSize {
		return fmt.Errorf("size must be between %d and %d pixels", MinSize, MaxSize)
	}

	validFormats := map[string]bool{
		FormatPNG:  true,
		FormatSVG:  true,
		FormatPDF:  true,
		FormatJPG:  true,
		FormatJPEG: true,
	}

	if !validFormats[req.Format] {
		return fmt.Errorf("invalid format: %s", req.Format)
	}

	validStyles := map[string]bool{
		StyleSquare:  true,
		StyleCircle:  true,
		StyleRounded: true,
	}

	if !validStyles[req.Style] {
		return fmt.Errorf("invalid style: %s", req.Style)
	}

	validErrorCorrection := map[string]bool{
		ErrorCorrectionLow:      true,
		ErrorCorrectionMedium:   true,
		ErrorCorrectionQuartile: true,
		ErrorCorrectionHigh:     true,
	}

	if req.ErrorCorrection != "" && !validErrorCorrection[req.ErrorCorrection] {
		return fmt.Errorf("invalid error correction level: %s", req.ErrorCorrection)
	}

	if req.LogoSize > MaxLogoSize {
		return fmt.Errorf("logo size cannot exceed %d pixels", MaxLogoSize)
	}

	return nil
}

// SetDefaults sets default values for unspecified fields
func (req *QRRequest) SetDefaults() {
	if req.Format == "" {
		req.Format = DefaultFormat
	}
	if req.Size == 0 {
		req.Size = DefaultSize
	}
	if req.Style == "" {
		req.Style = DefaultStyle
	}
	if req.ErrorCorrection == "" {
		req.ErrorCorrection = DefaultErrorCorrection
	}
	if req.Margin == 0 {
		req.Margin = DefaultMargin
	}
	if req.ForegroundColor == "" {
		req.ForegroundColor = DefaultForegroundColor
	}
	if req.BackgroundColor == "" {
		req.BackgroundColor = DefaultBackgroundColor
	}
}

// GetContentType returns the appropriate content type for the QR code format
func (qr *QRCode) GetContentType() string {
	switch qr.Format {
	case FormatPNG:
		return "image/png"
	case FormatJPG, FormatJPEG:
		return "image/jpeg"
	case FormatSVG:
		return "image/svg+xml"
	case FormatPDF:
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// GetFileExtension returns the appropriate file extension for the QR code format
func (qr *QRCode) GetFileExtension() string {
	switch qr.Format {
	case FormatPNG:
		return "png"
	case FormatJPG, FormatJPEG:
		return "jpg"
	case FormatSVG:
		return "svg"
	case FormatPDF:
		return "pdf"
	default:
		return "bin"
	}
}

// GetFilename generates a filename for the QR code
func (qr *QRCode) GetFilename() string {
	return fmt.Sprintf("qr_%s_%s_%dx%d.%s",
		qr.URLID, qr.Style, qr.Size, qr.Size, qr.GetFileExtension())
}

// IsValidSize checks if the QR code size is within acceptable limits
func IsValidSize(size int) bool {
	return size >= MinSize && size <= MaxSize
}

// IsValidFormat checks if the format is supported
func IsValidFormat(format string) bool {
	validFormats := map[string]bool{
		FormatPNG:  true,
		FormatSVG:  true,
		FormatPDF:  true,
		FormatJPG:  true,
		FormatJPEG: true,
	}
	return validFormats[format]
}

// IsValidStyle checks if the style is supported
func IsValidStyle(style string) bool {
	validStyles := map[string]bool{
		StyleSquare:  true,
		StyleCircle:  true,
		StyleRounded: true,
	}
	return validStyles[style]
}