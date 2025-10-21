package url

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
)

// ShortCodeGenerator generates short codes for URLs
type ShortCodeGenerator struct {
	config *config.URLConfig
	db     *db.DB
}

// NewShortCodeGenerator creates a new short code generator
func NewShortCodeGenerator(cfg *config.URLConfig, database *db.DB) *ShortCodeGenerator {
	return &ShortCodeGenerator{
		config: cfg,
		db:     database,
	}
}

// Generate generates a new short code
func (g *ShortCodeGenerator) Generate(ctx context.Context, originalURL string, customCode string) (string, bool, error) {
	if customCode != "" {
		// Validate and use custom code
		if err := g.validateCustomCode(customCode); err != nil {
			return "", false, err
		}

		// Check availability
		exists, err := g.codeExists(ctx, customCode)
		if err != nil {
			return "", false, fmt.Errorf("failed to check code availability: %w", err)
		}
		if exists {
			return "", false, ErrCodeAlreadyExists
		}

		return customCode, true, nil
	}

	// Generate random code
	return g.generateRandomCode(ctx, originalURL)
}

// GenerateSuggestions generates suggested short codes based on the URL
func (g *ShortCodeGenerator) GenerateSuggestions(ctx context.Context, originalURL string, count int) ([]string, error) {
	if count <= 0 || count > 10 {
		count = 5
	}

	var suggestions []string

	// Try word-based suggestions first
	wordSuggestions := g.generateWordBasedSuggestions(originalURL, count/2)
	for _, suggestion := range wordSuggestions {
		if exists, err := g.codeExists(ctx, suggestion); err == nil && !exists {
			suggestions = append(suggestions, suggestion)
			if len(suggestions) >= count {
				break
			}
		}
	}

	// Fill remaining with random codes
	for len(suggestions) < count {
		code, _, err := g.generateRandomCode(ctx, originalURL)
		if err != nil {
			break
		}
		suggestions = append(suggestions, code)
	}

	return suggestions, nil
}

// validateCustomCode validates a custom short code
func (g *ShortCodeGenerator) validateCustomCode(code string) error {
	// Length validation
	if len(code) < g.config.CustomCodeMinLength {
		return fmt.Errorf("code too short (minimum %d characters)", g.config.CustomCodeMinLength)
	}
	if len(code) > g.config.CustomCodeMaxLength {
		return fmt.Errorf("code too long (maximum %d characters)", g.config.CustomCodeMaxLength)
	}

	// Character validation
	allowedChars := g.config.AllowedCharacters
	if g.config.ExcludeSimilarChars {
		allowedChars = g.removeSimilarChars(allowedChars)
	}

	for _, char := range code {
		if !strings.ContainsRune(allowedChars, char) {
			return fmt.Errorf("invalid character '%c'", char)
		}
	}

	// Reserved words validation
	for _, reserved := range g.config.ReservedWords {
		if strings.EqualFold(code, reserved) {
			return fmt.Errorf("'%s' is reserved", code)
		}
	}

	// Additional validation
	return g.validateCodeSafety(code)
}

// validateCodeSafety performs additional safety checks
func (g *ShortCodeGenerator) validateCodeSafety(code string) error {
	// Check for potentially confusing patterns
	lowerCode := strings.ToLower(code)

	// All numbers might be confusing
	if matched, _ := regexp.MatchString(`^[0-9]+$`, code); matched {
		return fmt.Errorf("all-numeric codes are not allowed")
	}

	// Single characters might be too generic
	if len(code) == 1 {
		return fmt.Errorf("single character codes are not allowed")
	}

	// Check for common file extensions that might cause confusion
	extensions := []string{".html", ".php", ".asp", ".jsp", ".js", ".css", ".xml", ".json"}
	for _, ext := range extensions {
		if strings.HasSuffix(lowerCode, ext) {
			return fmt.Errorf("codes ending with file extensions are not allowed")
		}
	}

	// Check for URL-like patterns
	if strings.Contains(code, "://") || strings.Contains(code, "www.") {
		return fmt.Errorf("URL-like patterns are not allowed")
	}

	return nil
}

// generateRandomCode generates a random short code
func (g *ShortCodeGenerator) generateRandomCode(ctx context.Context, originalURL string) (string, bool, error) {
	chars := g.config.AllowedCharacters
	if g.config.ExcludeSimilarChars {
		chars = g.removeSimilarChars(chars)
	}

	maxAttempts := 100
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Determine length based on attempt (start small, grow if needed)
		length := g.config.MinRandomLength
		if attempt > 20 {
			length = g.config.MinRandomLength + 1
		}
		if attempt > 50 {
			length = g.config.MaxRandomLength
		}

		code := g.generateRandomString(chars, length)
		if code == "" {
			continue
		}

		// Check if code already exists
		exists, err := g.codeExists(ctx, code)
		if err != nil {
			return "", false, fmt.Errorf("failed to check code existence: %w", err)
		}

		if !exists {
			return code, false, nil
		}
	}

	return "", false, fmt.Errorf("failed to generate unique code after %d attempts", maxAttempts)
}

// generateRandomString generates a random string of specified length
func (g *ShortCodeGenerator) generateRandomString(charset string, length int) string {
	if len(charset) == 0 || length <= 0 {
		return ""
	}

	result := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := range result {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return ""
		}
		result[i] = charset[num.Int64()]
	}

	return string(result)
}

// generateWordBasedSuggestions generates suggestions based on URL content
func (g *ShortCodeGenerator) generateWordBasedSuggestions(originalURL string, count int) []string {
	var suggestions []string

	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		return suggestions
	}

	// Extract words from domain
	domain := parsedURL.Host
	if strings.HasPrefix(domain, "www.") {
		domain = domain[4:]
	}

	// Split domain by dots and dashes
	domainParts := regexp.MustCompile(`[.\-]`).Split(domain, -1)

	// Extract words from path
	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")

	// Combine all parts
	allParts := append(domainParts, pathParts...)

	// Generate suggestions from parts
	for _, part := range allParts {
		if part == "" || len(part) < 3 {
			continue
		}

		// Clean the part
		cleanPart := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(part, "")
		if len(cleanPart) < 3 {
			continue
		}

		// Take first few characters
		suggestions = append(suggestions, g.truncateToValidLength(cleanPart))

		// Take abbreviated form
		if len(cleanPart) > 6 {
			abbreviated := g.abbreviate(cleanPart)
			if abbreviated != "" {
				suggestions = append(suggestions, abbreviated)
			}
		}

		if len(suggestions) >= count*2 {
			break
		}
	}

	// Remove duplicates and filter
	return g.filterAndDeduplicateSuggestions(suggestions, count)
}

// truncateToValidLength truncates a string to valid code length
func (g *ShortCodeGenerator) truncateToValidLength(s string) string {
	if len(s) <= g.config.MaxRandomLength {
		return s
	}
	return s[:g.config.MaxRandomLength]
}

// abbreviate creates an abbreviation from a word
func (g *ShortCodeGenerator) abbreviate(word string) string {
	if len(word) <= 4 {
		return word
	}

	// Take consonants preferentially
	var result strings.Builder
	vowels := "aeiouAEIOU"

	for i, char := range word {
		if result.Len() >= g.config.MaxRandomLength {
			break
		}

		// Always include first character
		if i == 0 {
			result.WriteRune(char)
			continue
		}

		// Include consonants and occasional vowels
		if !strings.ContainsRune(vowels, char) {
			result.WriteRune(char)
		} else if result.Len() < 3 && i%2 == 0 {
			result.WriteRune(char)
		}
	}

	abbreviation := result.String()
	if len(abbreviation) < g.config.MinRandomLength {
		return word[:g.config.MinRandomLength]
	}

	return abbreviation
}

// filterAndDeduplicateSuggestions filters and removes duplicates
func (g *ShortCodeGenerator) filterAndDeduplicateSuggestions(suggestions []string, count int) []string {
	seen := make(map[string]bool)
	var filtered []string

	for _, suggestion := range suggestions {
		if seen[suggestion] {
			continue
		}

		// Validate the suggestion
		if err := g.validateCustomCode(suggestion); err == nil {
			seen[suggestion] = true
			filtered = append(filtered, suggestion)
			if len(filtered) >= count {
				break
			}
		}
	}

	return filtered
}

// removeSimilarChars removes visually similar characters
func (g *ShortCodeGenerator) removeSimilarChars(chars string) string {
	// Characters to remove if ExcludeSimilarChars is true
	toRemove := []rune{'0', 'O', 'o', '1', 'l', 'I', '2', 'Z', '5', 'S', '6', 'G', '8', 'B'}

	var result strings.Builder
	for _, char := range chars {
		shouldRemove := false
		for _, remove := range toRemove {
			if char == remove {
				shouldRemove = true
				break
			}
		}
		if !shouldRemove {
			result.WriteRune(char)
		}
	}

	return result.String()
}

// codeExists checks if a code already exists in the database
func (g *ShortCodeGenerator) codeExists(ctx context.Context, code string) (bool, error) {
	query := "SELECT COUNT(*) FROM urls WHERE id = ?"
	if g.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM urls WHERE id = $1"
	}

	var count int
	err := g.db.QueryRow(ctx, query, code).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// ReserveCode reserves a code for a limited time to prevent race conditions
func (g *ShortCodeGenerator) ReserveCode(ctx context.Context, code string, duration time.Duration) error {
	// This would typically use a cache/Redis to temporarily reserve codes
	// For now, we'll just check if it exists
	exists, err := g.codeExists(ctx, code)
	if err != nil {
		return err
	}
	if exists {
		return ErrCodeAlreadyExists
	}
	return nil
}

// ValidateAvailability checks if a code is available for use
func (g *ShortCodeGenerator) ValidateAvailability(ctx context.Context, code string) error {
	exists, err := g.codeExists(ctx, code)
	if err != nil {
		return fmt.Errorf("failed to check availability: %w", err)
	}
	if exists {
		return ErrCodeAlreadyExists
	}
	return nil
}