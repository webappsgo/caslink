package service

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"image/color"
	"image/png"

	"github.com/skip2/go-qrcode"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// QRService handles QR code generation
type QRService struct {
	store *store.Store
}

// NewQRService creates a new QR service
func NewQRService(st *store.Store) *QRService {
	return &QRService{
		store: st,
	}
}

// QRCodeOptions represents options for QR code generation
type QRCodeOptions struct {
	Format string // png, svg
	Size   int    // pixels (default: 200, max: 1000)
	Style  string // square, circle, rounded (default: square)
}

// GenerateQRCode generates a QR code for a URL
func (s *QRService) GenerateQRCode(ctx context.Context, urlID int64, url string, opts *QRCodeOptions) ([]byte, string, error) {
	// Set defaults
	if opts == nil {
		opts = &QRCodeOptions{
			Format: "png",
			Size:   200,
			Style:  "square",
		}
	}

	// Validate size (per SPEC PART 36: default 200, max 1000)
	if opts.Size <= 0 {
		opts.Size = 200
	}
	if opts.Size > 1000 {
		opts.Size = 1000
	}

	// Check cache first
	cached, contentType, err := s.getFromCache(ctx, urlID, opts)
	if err == nil && cached != nil {
		return cached, contentType, nil
	}

	// Generate QR code
	var data []byte
	var ct string

	switch opts.Format {
	case "svg":
		svgData, err := generateSVG(url, opts.Size)
		if err != nil {
			return nil, "", fmt.Errorf("failed to generate SVG QR code: %w", err)
		}
		data = svgData
		ct = "image/svg+xml"

	default: // png
		qr, err := qrcode.New(url, qrcode.Medium)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create QR code: %w", err)
		}

		img := qr.Image(opts.Size)

		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, "", fmt.Errorf("failed to encode PNG: %w", err)
		}

		data = buf.Bytes()
		ct = "image/png"
	}

	// Cache the generated QR code — best-effort; caller still gets the data on failure.
	_ = s.saveToCache(ctx, urlID, opts.Format, opts.Size, opts.Style, data)

	return data, ct, nil
}

// getFromCache retrieves a cached QR code
func (s *QRService) getFromCache(ctx context.Context, urlID int64, opts *QRCodeOptions) ([]byte, string, error) {
	query := `SELECT data, format FROM qr_codes
	          WHERE url_id = ? AND format = ? AND size = ? AND style = ?
	          ORDER BY created_at DESC LIMIT 1`

	var data []byte
	var format string

	err := s.store.ServerDB.QueryRowContext(ctx, query, urlID, opts.Format, opts.Size, opts.Style).Scan(&data, &format)
	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("not cached")
	}
	if err != nil {
		return nil, "", err
	}

	// Determine content type
	var contentType string
	switch format {
	case "svg":
		contentType = "image/svg+xml"
	default:
		contentType = "image/png"
	}

	return data, contentType, nil
}

// saveToCache saves a generated QR code to cache
func (s *QRService) saveToCache(ctx context.Context, urlID int64, format string, size int, style string, data []byte) error {
	query := `INSERT INTO qr_codes (url_id, format, size, style, data) VALUES (?, ?, ?, ?, ?)`
	_, err := s.store.ServerDB.ExecContext(ctx, query, urlID, format, size, style, data)
	return err
}

// ClearCache clears cached QR codes for a URL
func (s *QRService) ClearCache(ctx context.Context, urlID int64) error {
	query := `DELETE FROM qr_codes WHERE url_id = ?`
	_, err := s.store.ServerDB.ExecContext(ctx, query, urlID)
	return err
}

// generateSVG renders a QR code as SVG by iterating the bitmap from go-qrcode.
// Each dark module becomes a <rect> element so the output scales cleanly.
func generateSVG(data string, size int) ([]byte, error) {
	qr, err := qrcode.New(data, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("failed to create QR code: %w", err)
	}

	img := qr.Image(size)
	bounds := img.Bounds()
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y

	var buf bytes.Buffer
	fmt.Fprintf(&buf, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, w, h, size, size)
	fmt.Fprintf(&buf, `<rect width="%d" height="%d" fill="white"/>`, w, h)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// Dark module: luminance below 50% (each channel is 16-bit from RGBA())
			if isDarkPixel(color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: 255,
			}) {
				fmt.Fprintf(&buf, `<rect x="%d" y="%d" width="1" height="1" fill="black"/>`, x-bounds.Min.X, y-bounds.Min.Y)
			}
		}
	}

	buf.WriteString(`</svg>`)
	return buf.Bytes(), nil
}

// isDarkPixel returns true when the pixel colour is closer to black than white.
func isDarkPixel(c color.RGBA) bool {
	// Simple luminance threshold: average of R, G, B < 128
	lum := (int(c.R) + int(c.G) + int(c.B)) / 3
	return lum < 128
}

// GenerateQRCodeForText generates a QR code for any text/URL (used for TOTP).
func (s *QRService) GenerateQRCodeForText(text string, size int) ([]byte, error) {
	if size <= 0 {
		size = 200
	}
	if size > 1000 {
		size = 1000
	}

	qr, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	var buf bytes.Buffer
	img := qr.Image(size)
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("failed to encode QR code: %w", err)
	}

	return buf.Bytes(), nil
}
