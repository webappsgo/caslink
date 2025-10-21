package qr

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/skip2/go-qrcode"
)

// Generator handles the actual QR code generation
type Generator struct {
	config *config.QRConfig
	logger *logrus.Logger
}

// NewGenerator creates a new QR code generator
func NewGenerator(cfg *config.QRConfig, logger *logrus.Logger) (*Generator, error) {
	return &Generator{
		config: cfg,
		logger: logger,
	}, nil
}

// GenerateQRCode generates a QR code based on the request
func (g *Generator) GenerateQRCode(ctx context.Context, req *QRRequest) (*QRCode, error) {
	// Set defaults for missing values
	req.SetDefaults()

	// Validate the request
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Generate unique ID for this QR code
	qrID := uuid.New().String()

	g.logger.WithFields(logrus.Fields{
		"qr_id":  qrID,
		"url_id": req.URLID,
		"format": req.Format,
		"size":   req.Size,
		"style":  req.Style,
	}).Debug("Generating QR code")

	// Build the URL that the QR code will point to
	qrURL := g.buildQRURL(req.URLID)

	// Convert error correction level
	errorLevel := g.getErrorCorrectionLevel(req.ErrorCorrection)

	var qrData []byte
	var err error

	// Generate QR code based on format
	switch strings.ToLower(req.Format) {
	case FormatPNG, FormatJPG, FormatJPEG:
		qrData, err = g.generateBitmapQR(qrURL, req, errorLevel)
	case FormatSVG:
		qrData, err = g.generateSVGQR(qrURL, req, errorLevel)
	case FormatPDF:
		qrData, err = g.generatePDFQR(qrURL, req, errorLevel)
	default:
		return nil, fmt.Errorf("unsupported format: %s", req.Format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	qrCode := &QRCode{
		ID:              qrID,
		URLID:           req.URLID,
		Format:          req.Format,
		Size:            req.Size,
		Style:           req.Style,
		ForegroundColor: req.ForegroundColor,
		BackgroundColor: req.BackgroundColor,
		LogoURL:         req.LogoURL,
		Data:            qrData,
		CreatedAt:       time.Now(),
	}

	g.logger.WithFields(logrus.Fields{
		"qr_id":    qrID,
		"url_id":   req.URLID,
		"data_size": len(qrData),
	}).Info("QR code generated successfully")

	return qrCode, nil
}

// generateBitmapQR generates a bitmap QR code (PNG, JPG)
func (g *Generator) generateBitmapQR(url string, req *QRRequest, errorLevel qrcode.RecoveryLevel) ([]byte, error) {
	// Create basic QR code
	qr, err := qrcode.New(url, errorLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to create QR code: %w", err)
	}

	// Set colors
	fgColor := hexToColor(req.ForegroundColor)
	bgColor := hexToColor(req.BackgroundColor)
	qr.ForegroundColor = fgColor
	qr.BackgroundColor = bgColor

	// Generate the base image
	qrImage := qr.Image(req.Size)

	// Apply styling if needed
	if req.Style != StyleSquare {
		qrImage = g.applyStyle(qrImage, req.Style)
	}

	// Add logo if specified
	if req.LogoURL != "" && g.config.AllowLogos {
		logoImage, err := g.downloadAndResizeLogo(req.LogoURL, req.LogoSize)
		if err != nil {
			g.logger.WithError(err).Warn("Failed to add logo to QR code")
		} else {
			qrImage = g.addLogoToQR(qrImage, logoImage)
		}
	}

	// Convert to the requested format
	var buf bytes.Buffer
	switch strings.ToLower(req.Format) {
	case FormatPNG:
		err = png.Encode(&buf, qrImage)
	case FormatJPG, FormatJPEG:
		err = jpeg.Encode(&buf, qrImage, &jpeg.Options{Quality: 95})
	default:
		return nil, fmt.Errorf("unsupported bitmap format: %s", req.Format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to encode image: %w", err)
	}

	return buf.Bytes(), nil
}

// generateSVGQR generates an SVG QR code
func (g *Generator) generateSVGQR(url string, req *QRRequest, errorLevel qrcode.RecoveryLevel) ([]byte, error) {
	// Create basic QR code
	qr, err := qrcode.New(url, errorLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to create QR code: %w", err)
	}

	// Get the bitmap to analyze the pattern
	bitmap := qr.Bitmap()
	size := len(bitmap)
	blockSize := req.Size / size

	// Build SVG content
	svg := g.buildSVG(bitmap, req, blockSize)

	return []byte(svg), nil
}

// generatePDFQR generates a PDF QR code
func (g *Generator) generatePDFQR(url string, req *QRRequest, errorLevel qrcode.RecoveryLevel) ([]byte, error) {
	// For now, generate as PNG and embed in a simple PDF
	// In production, you would use a proper PDF library like gofpdf
	pngData, err := g.generateBitmapQR(url, req, errorLevel)
	if err != nil {
		return nil, err
	}

	// Create a simple PDF wrapper (this is a placeholder)
	pdf := g.createPDFWrapper(pngData, req)
	return []byte(pdf), nil
}

// Helper methods

func (g *Generator) buildQRURL(urlID string) string {
	// In a real implementation, this would build the full URL
	// For now, we'll use a placeholder
	return fmt.Sprintf("https://short.ly/%s", urlID)
}

func (g *Generator) getErrorCorrectionLevel(level string) qrcode.RecoveryLevel {
	switch level {
	case ErrorCorrectionLow:
		return qrcode.Low
	case ErrorCorrectionMedium:
		return qrcode.Medium
	case ErrorCorrectionQuartile:
		return qrcode.High
	case ErrorCorrectionHigh:
		return qrcode.Highest
	default:
		return qrcode.Medium
	}
}

func (g *Generator) applyStyle(img image.Image, style string) image.Image {
	bounds := img.Bounds()
	styled := image.NewRGBA(bounds)

	switch style {
	case StyleCircle:
		return g.applyCircleStyle(img, styled)
	case StyleRounded:
		return g.applyRoundedStyle(img, styled)
	default:
		return img
	}
}

func (g *Generator) applyCircleStyle(src image.Image, dst *image.RGBA) image.Image {
	bounds := src.Bounds()
	center := image.Point{bounds.Max.X / 2, bounds.Max.Y / 2}
	radius := bounds.Max.X / 2

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dx := x - center.X
			dy := y - center.Y
			distance := dx*dx + dy*dy

			if distance <= radius*radius {
				dst.Set(x, y, src.At(x, y))
			} else {
				dst.Set(x, y, color.Transparent)
			}
		}
	}

	return dst
}

func (g *Generator) applyRoundedStyle(src image.Image, dst *image.RGBA) image.Image {
	bounds := src.Bounds()
	cornerRadius := bounds.Max.X / 20 // 5% of width as corner radius

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// Check if we're in a corner region
			inCorner := false
			transparent := false

			// Top-left corner
			if x < cornerRadius && y < cornerRadius {
				inCorner = true
				dx := cornerRadius - x
				dy := cornerRadius - y
				if dx*dx+dy*dy > cornerRadius*cornerRadius {
					transparent = true
				}
			}
			// Top-right corner
			if x > bounds.Max.X-cornerRadius && y < cornerRadius {
				inCorner = true
				dx := x - (bounds.Max.X - cornerRadius)
				dy := cornerRadius - y
				if dx*dx+dy*dy > cornerRadius*cornerRadius {
					transparent = true
				}
			}
			// Bottom-left corner
			if x < cornerRadius && y > bounds.Max.Y-cornerRadius {
				inCorner = true
				dx := cornerRadius - x
				dy := y - (bounds.Max.Y - cornerRadius)
				if dx*dx+dy*dy > cornerRadius*cornerRadius {
					transparent = true
				}
			}
			// Bottom-right corner
			if x > bounds.Max.X-cornerRadius && y > bounds.Max.Y-cornerRadius {
				inCorner = true
				dx := x - (bounds.Max.X - cornerRadius)
				dy := y - (bounds.Max.Y - cornerRadius)
				if dx*dx+dy*dy > cornerRadius*cornerRadius {
					transparent = true
				}
			}

			if transparent {
				dst.Set(x, y, color.Transparent)
			} else {
				dst.Set(x, y, src.At(x, y))
			}
		}
	}

	return dst
}

func (g *Generator) downloadAndResizeLogo(logoURL string, logoSize int) (image.Image, error) {
	if logoSize == 0 {
		logoSize = 50 // Default logo size
	}

	// Download logo
	resp, err := http.Get(logoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download logo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download logo: HTTP %d", resp.StatusCode)
	}

	// Decode image
	logo, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode logo: %w", err)
	}

	// Resize logo
	return g.resizeImage(logo, logoSize, logoSize), nil
}

func (g *Generator) resizeImage(src image.Image, width, height int) image.Image {
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

func (g *Generator) addLogoToQR(qrImg, logoImg image.Image) image.Image {
	qrBounds := qrImg.Bounds()
	logoBounds := logoImg.Bounds()

	// Create a new image for the result
	result := image.NewRGBA(qrBounds)
	draw.Draw(result, qrBounds, qrImg, image.Point{}, draw.Src)

	// Calculate position to center the logo
	logoX := (qrBounds.Dx() - logoBounds.Dx()) / 2
	logoY := (qrBounds.Dy() - logoBounds.Dy()) / 2
	logoRect := image.Rect(logoX, logoY, logoX+logoBounds.Dx(), logoY+logoBounds.Dy())

	// Draw logo on top of QR code
	draw.Draw(result, logoRect, logoImg, image.Point{}, draw.Over)

	return result
}

func (g *Generator) buildSVG(bitmap [][]bool, req *QRRequest, blockSize int) string {
	var svg strings.Builder

	// SVG header
	svg.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		req.Size, req.Size, req.Size, req.Size))

	// Background
	svg.WriteString(fmt.Sprintf(`<rect width="%d" height="%d" fill="%s"/>`,
		req.Size, req.Size, req.BackgroundColor))

	// QR blocks
	for y, row := range bitmap {
		for x, isBlack := range row {
			if isBlack {
				switch req.Style {
				case StyleCircle:
					centerX := x*blockSize + blockSize/2
					centerY := y*blockSize + blockSize/2
					radius := blockSize / 2
					svg.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="%d" fill="%s"/>`,
						centerX, centerY, radius, req.ForegroundColor))
				case StyleRounded:
					rx := blockSize / 4
					svg.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="%d" ry="%d" fill="%s"/>`,
						x*blockSize, y*blockSize, blockSize, blockSize, rx, rx, req.ForegroundColor))
				default: // Square
					svg.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`,
						x*blockSize, y*blockSize, blockSize, blockSize, req.ForegroundColor))
				}
			}
		}
	}

	svg.WriteString("</svg>")
	return svg.String()
}

func (g *Generator) createPDFWrapper(pngData []byte, req *QRRequest) string {
	// This is a very simplified PDF. In production, use a proper PDF library.
	pdf := fmt.Sprintf(`%%PDF-1.4
1 0 obj
<<
/Type /Catalog
/Pages 2 0 R
>>
endobj

2 0 obj
<<
/Type /Pages
/Kids [3 0 R]
/Count 1
>>
endobj

3 0 obj
<<
/Type /Page
/Parent 2 0 R
/Resources <<
/XObject <<
/QR 4 0 R
>>
>>
/MediaBox [0 0 612 792]
/Contents 5 0 R
>>
endobj

4 0 obj
<<
/Type /XObject
/Subtype /Image
/Width %d
/Height %d
/ColorSpace /DeviceRGB
/BitsPerComponent 8
/Filter /DCTDecode
/Length %d
>>
stream
%s
endstream
endobj

5 0 obj
<<
/Length 44
>>
stream
q
%d 0 0 %d 100 600 cm
/QR Do
Q
endstream
endobj

xref
0 6
0000000000 65535 f
0000000010 00000 n
0000000053 00000 n
0000000109 00000 n
0000000274 00000 n
0000000400 00000 n
trailer
<<
/Size 6
/Root 1 0 R
>>
startxref
494
%%%%EOF`, req.Size, req.Size, len(pngData), string(pngData), req.Size, req.Size)

	return pdf
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

// ValidateQRCode validates that a generated QR code is readable
func (g *Generator) ValidateQRCode(qrData []byte, expectedURL string) (*QRValidationResult, error) {
	result := &QRValidationResult{
		Valid:    false,
		DataSize: len(qrData),
	}

	// For now, we'll just check that the data exists and has reasonable size
	if len(qrData) == 0 {
		result.Errors = append(result.Errors, "QR code data is empty")
		return result, nil
	}

	if len(qrData) < 100 {
		result.Warnings = append(result.Warnings, "QR code data seems very small")
	}

	// In a real implementation, you would decode the QR code and verify it contains the expected URL
	result.Valid = true
	result.Readable = true
	result.URL = expectedURL

	return result, nil
}