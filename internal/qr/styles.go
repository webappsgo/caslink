package qr

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
)

// StyleEngine handles different QR code styling options
type StyleEngine struct {
	config *StyleConfig
}

// StyleConfig contains configuration for QR code styling
type StyleConfig struct {
	AllowCustomColors bool
	AllowLogos        bool
	MaxCornerRadius   int
	MaxBorderWidth    int
}

// NewStyleEngine creates a new style engine
func NewStyleEngine(config *StyleConfig) *StyleEngine {
	return &StyleEngine{
		config: config,
	}
}

// ApplyStyle applies the specified style to a QR code image
func (se *StyleEngine) ApplyStyle(img image.Image, style string, options *StyleOptions) (image.Image, error) {
	switch style {
	case StyleSquare:
		return se.applySquareStyle(img, options)
	case StyleCircle:
		return se.applyCircleStyle(img, options)
	case StyleRounded:
		return se.applyRoundedStyle(img, options)
	default:
		return nil, fmt.Errorf("unsupported style: %s", style)
	}
}

// StyleOptions contains options for styling QR codes
type StyleOptions struct {
	CornerRadius  int
	BorderWidth   int
	BorderColor   color.Color
	ShadowOffset  int
	ShadowColor   color.Color
	GradientStart color.Color
	GradientEnd   color.Color
	Pattern       string
}

// applySquareStyle applies square styling (default, no modifications)
func (se *StyleEngine) applySquareStyle(img image.Image, options *StyleOptions) (image.Image, error) {
	if options == nil {
		return img, nil
	}

	bounds := img.Bounds()
	result := image.NewRGBA(bounds)

	// Copy original image
	draw.Draw(result, bounds, img, image.Point{}, draw.Src)

	// Apply border if specified
	if options.BorderWidth > 0 && options.BorderColor != nil {
		se.addBorder(result, options.BorderWidth, options.BorderColor)
	}

	// Apply shadow if specified
	if options.ShadowOffset > 0 && options.ShadowColor != nil {
		result = se.addShadow(result, options.ShadowOffset, options.ShadowColor)
	}

	return result, nil
}

// applyCircleStyle applies circular styling to the QR code
func (se *StyleEngine) applyCircleStyle(img image.Image, options *StyleOptions) (image.Image, error) {
	bounds := img.Bounds()
	result := image.NewRGBA(bounds)

	center := image.Point{bounds.Dx() / 2, bounds.Dy() / 2}
	radius := float64(bounds.Dx()) / 2

	// Apply circular mask
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dx := float64(x - center.X)
			dy := float64(y - center.Y)
			distance := math.Sqrt(dx*dx + dy*dy)

			if distance <= radius {
				result.Set(x, y, img.At(x, y))
			} else {
				result.Set(x, y, color.Transparent)
			}
		}
	}

	// Apply border if specified
	if options != nil && options.BorderWidth > 0 && options.BorderColor != nil {
		se.addCircularBorder(result, center, radius, options.BorderWidth, options.BorderColor)
	}

	return result, nil
}

// applyRoundedStyle applies rounded corner styling to the QR code
func (se *StyleEngine) applyRoundedStyle(img image.Image, options *StyleOptions) (image.Image, error) {
	bounds := img.Bounds()
	result := image.NewRGBA(bounds)

	cornerRadius := bounds.Dx() / 20 // Default 5% of width
	if options != nil && options.CornerRadius > 0 {
		cornerRadius = options.CornerRadius
	}

	// Ensure corner radius doesn't exceed maximum
	if se.config.MaxCornerRadius > 0 && cornerRadius > se.config.MaxCornerRadius {
		cornerRadius = se.config.MaxCornerRadius
	}

	// Apply rounded corners
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if se.isInRoundedArea(x, y, bounds, cornerRadius) {
				result.Set(x, y, img.At(x, y))
			} else {
				result.Set(x, y, color.Transparent)
			}
		}
	}

	// Apply border if specified
	if options != nil && options.BorderWidth > 0 && options.BorderColor != nil {
		se.addRoundedBorder(result, bounds, cornerRadius, options.BorderWidth, options.BorderColor)
	}

	return result, nil
}

// isInRoundedArea checks if a point is within the rounded rectangle area
func (se *StyleEngine) isInRoundedArea(x, y int, bounds image.Rectangle, cornerRadius int) bool {
	// Check if we're in a corner region
	inTopLeft := x < bounds.Min.X+cornerRadius && y < bounds.Min.Y+cornerRadius
	inTopRight := x > bounds.Max.X-cornerRadius && y < bounds.Min.Y+cornerRadius
	inBottomLeft := x < bounds.Min.X+cornerRadius && y > bounds.Max.Y-cornerRadius
	inBottomRight := x > bounds.Max.X-cornerRadius && y > bounds.Max.Y-cornerRadius

	if inTopLeft {
		dx := float64(bounds.Min.X + cornerRadius - x)
		dy := float64(bounds.Min.Y + cornerRadius - y)
		return dx*dx+dy*dy <= float64(cornerRadius*cornerRadius)
	}

	if inTopRight {
		dx := float64(x - (bounds.Max.X - cornerRadius))
		dy := float64(bounds.Min.Y + cornerRadius - y)
		return dx*dx+dy*dy <= float64(cornerRadius*cornerRadius)
	}

	if inBottomLeft {
		dx := float64(bounds.Min.X + cornerRadius - x)
		dy := float64(y - (bounds.Max.Y - cornerRadius))
		return dx*dx+dy*dy <= float64(cornerRadius*cornerRadius)
	}

	if inBottomRight {
		dx := float64(x - (bounds.Max.X - cornerRadius))
		dy := float64(y - (bounds.Max.Y - cornerRadius))
		return dx*dx+dy*dy <= float64(cornerRadius*cornerRadius)
	}

	// Not in a corner region, so it's included
	return true
}

// addBorder adds a border around the QR code
func (se *StyleEngine) addBorder(img *image.RGBA, width int, borderColor color.Color) {
	bounds := img.Bounds()

	// Top and bottom borders
	for y := 0; y < width; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, bounds.Min.Y+y, borderColor)         // Top
			img.Set(x, bounds.Max.Y-1-y, borderColor)       // Bottom
		}
	}

	// Left and right borders
	for x := 0; x < width; x++ {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			img.Set(bounds.Min.X+x, y, borderColor)         // Left
			img.Set(bounds.Max.X-1-x, y, borderColor)       // Right
		}
	}
}

// addCircularBorder adds a circular border around the QR code
func (se *StyleEngine) addCircularBorder(img *image.RGBA, center image.Point, radius float64, width int, borderColor color.Color) {
	bounds := img.Bounds()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dx := float64(x - center.X)
			dy := float64(y - center.Y)
			distance := math.Sqrt(dx*dx + dy*dy)

			if distance >= radius-float64(width) && distance <= radius {
				img.Set(x, y, borderColor)
			}
		}
	}
}

// addRoundedBorder adds a rounded border around the QR code
func (se *StyleEngine) addRoundedBorder(img *image.RGBA, bounds image.Rectangle, cornerRadius, borderWidth int, borderColor color.Color) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if se.isOnRoundedBorder(x, y, bounds, cornerRadius, borderWidth) {
				img.Set(x, y, borderColor)
			}
		}
	}
}

// isOnRoundedBorder checks if a point is on the rounded border
func (se *StyleEngine) isOnRoundedBorder(x, y int, bounds image.Rectangle, cornerRadius, borderWidth int) bool {
	// Check if point is within border width of the edge
	nearLeft := x < bounds.Min.X+borderWidth
	nearRight := x >= bounds.Max.X-borderWidth
	nearTop := y < bounds.Min.Y+borderWidth
	nearBottom := y >= bounds.Max.Y-borderWidth

	// If not near any edge, it's not on the border
	if !nearLeft && !nearRight && !nearTop && !nearBottom {
		return false
	}

	// Check corners
	if (nearLeft || nearRight) && (nearTop || nearBottom) {
		// We're in a corner region, check if we're within the rounded corner
		var centerX, centerY int
		if nearLeft && nearTop {
			centerX = bounds.Min.X + cornerRadius
			centerY = bounds.Min.Y + cornerRadius
		} else if nearRight && nearTop {
			centerX = bounds.Max.X - cornerRadius
			centerY = bounds.Min.Y + cornerRadius
		} else if nearLeft && nearBottom {
			centerX = bounds.Min.X + cornerRadius
			centerY = bounds.Max.Y - cornerRadius
		} else { // nearRight && nearBottom
			centerX = bounds.Max.X - cornerRadius
			centerY = bounds.Max.Y - cornerRadius
		}

		dx := float64(x - centerX)
		dy := float64(y - centerY)
		distance := math.Sqrt(dx*dx + dy*dy)

		return distance >= float64(cornerRadius-borderWidth) && distance <= float64(cornerRadius)
	}

	// On straight edge
	return true
}

// addShadow adds a drop shadow to the QR code
func (se *StyleEngine) addShadow(img *image.RGBA, offset int, shadowColor color.Color) *image.RGBA {
	bounds := img.Bounds()
	shadowBounds := image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X+offset, bounds.Max.Y+offset)
	result := image.NewRGBA(shadowBounds)

	// Draw shadow (offset image)
	shadowOffset := image.Point{offset, offset}
	draw.Draw(result, bounds.Add(shadowOffset), &image.Uniform{shadowColor}, image.Point{}, draw.Src)

	// Draw original image on top
	draw.Draw(result, bounds, img, image.Point{}, draw.Over)

	return result
}

// ApplyGradient applies a gradient effect to the QR code
func (se *StyleEngine) ApplyGradient(img image.Image, startColor, endColor color.Color) image.Image {
	bounds := img.Bounds()
	result := image.NewRGBA(bounds)

	r1, g1, b1, a1 := startColor.RGBA()
	r2, g2, b2, a2 := endColor.RGBA()

	height := float64(bounds.Dy())

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		// Calculate gradient factor (0.0 to 1.0)
		factor := float64(y-bounds.Min.Y) / height

		// Interpolate colors
		r := uint8((float64(r1)*(1-factor) + float64(r2)*factor) / 256)
		g := uint8((float64(g1)*(1-factor) + float64(g2)*factor) / 256)
		b := uint8((float64(b1)*(1-factor) + float64(b2)*factor) / 256)
		a := uint8((float64(a1)*(1-factor) + float64(a2)*factor) / 256)

		gradientColor := color.RGBA{r, g, b, a}

		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			originalColor := img.At(x, y)
			if originalColor != color.Transparent {
				result.Set(x, y, gradientColor)
			} else {
				result.Set(x, y, originalColor)
			}
		}
	}

	return result
}

// ApplyPattern applies a pattern overlay to the QR code
func (se *StyleEngine) ApplyPattern(img image.Image, pattern string) (image.Image, error) {
	switch pattern {
	case "dots":
		return se.applyDotPattern(img), nil
	case "stripes":
		return se.applyStripePattern(img), nil
	case "checkers":
		return se.applyCheckerPattern(img), nil
	default:
		return img, fmt.Errorf("unsupported pattern: %s", pattern)
	}
}

// applyDotPattern applies a dot pattern overlay
func (se *StyleEngine) applyDotPattern(img image.Image) image.Image {
	bounds := img.Bounds()
	result := image.NewRGBA(bounds)

	dotSize := 3
	spacing := 6

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// Check if we're at a dot position
			isDot := (x%spacing < dotSize) && (y%spacing < dotSize)

			if isDot {
				result.Set(x, y, img.At(x, y))
			} else {
				// Make it semi-transparent
				original := img.At(x, y)
				r, g, b, _ := original.RGBA()
				result.Set(x, y, color.RGBA{uint8(r / 256), uint8(g / 256), uint8(b / 256), 128})
			}
		}
	}

	return result
}

// applyStripePattern applies a stripe pattern overlay
func (se *StyleEngine) applyStripePattern(img image.Image) image.Image {
	bounds := img.Bounds()
	result := image.NewRGBA(bounds)

	stripeWidth := 4

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			isStripe := (x/stripeWidth)%2 == 0

			if isStripe {
				result.Set(x, y, img.At(x, y))
			} else {
				// Make it semi-transparent
				original := img.At(x, y)
				r, g, b, _ := original.RGBA()
				result.Set(x, y, color.RGBA{uint8(r / 256), uint8(g / 256), uint8(b / 256), 128})
			}
		}
	}

	return result
}

// applyCheckerPattern applies a checker pattern overlay
func (se *StyleEngine) applyCheckerPattern(img image.Image) image.Image {
	bounds := img.Bounds()
	result := image.NewRGBA(bounds)

	checkerSize := 8

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			isChecker := ((x/checkerSize)+(y/checkerSize))%2 == 0

			if isChecker {
				result.Set(x, y, img.At(x, y))
			} else {
				// Make it semi-transparent
				original := img.At(x, y)
				r, g, b, _ := original.RGBA()
				result.Set(x, y, color.RGBA{uint8(r / 256), uint8(g / 256), uint8(b / 256), 128})
			}
		}
	}

	return result
}

// CreateLogo creates a logo overlay for QR codes
func (se *StyleEngine) CreateLogo(logoImg image.Image, size int, style string) image.Image {
	// Resize logo to specified size
	resized := se.resizeImage(logoImg, size, size)

	switch style {
	case "circle":
		return se.applyCircleMask(resized)
	case "rounded":
		return se.applyRoundedMask(resized, size/8)
	default:
		return resized
	}
}

// applyCircleMask applies a circular mask to an image
func (se *StyleEngine) applyCircleMask(img image.Image) image.Image {
	bounds := img.Bounds()
	result := image.NewRGBA(bounds)

	center := image.Point{bounds.Dx() / 2, bounds.Dy() / 2}
	radius := float64(bounds.Dx()) / 2

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dx := float64(x - center.X)
			dy := float64(y - center.Y)
			distance := math.Sqrt(dx*dx + dy*dy)

			if distance <= radius {
				result.Set(x, y, img.At(x, y))
			} else {
				result.Set(x, y, color.Transparent)
			}
		}
	}

	return result
}

// applyRoundedMask applies a rounded rectangle mask to an image
func (se *StyleEngine) applyRoundedMask(img image.Image, cornerRadius int) image.Image {
	bounds := img.Bounds()
	result := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if se.isInRoundedArea(x, y, bounds, cornerRadius) {
				result.Set(x, y, img.At(x, y))
			} else {
				result.Set(x, y, color.Transparent)
			}
		}
	}

	return result
}

// resizeImage resizes an image using nearest-neighbor interpolation
func (se *StyleEngine) resizeImage(src image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))

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

// ValidateStyleOptions validates style options
func (se *StyleEngine) ValidateStyleOptions(options *StyleOptions) error {
	if options == nil {
		return nil
	}

	if se.config.MaxCornerRadius > 0 && options.CornerRadius > se.config.MaxCornerRadius {
		return fmt.Errorf("corner radius %d exceeds maximum %d", options.CornerRadius, se.config.MaxCornerRadius)
	}

	if se.config.MaxBorderWidth > 0 && options.BorderWidth > se.config.MaxBorderWidth {
		return fmt.Errorf("border width %d exceeds maximum %d", options.BorderWidth, se.config.MaxBorderWidth)
	}

	return nil
}