package url

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
)

// Validator handles URL validation and sanitization
type Validator struct {
	config     *config.URLConfig
	httpClient *http.Client
}

// NewValidator creates a new URL validator
func NewValidator(cfg *config.URLConfig) *Validator {
	return &Validator{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Follow up to 10 redirects
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
}

// ValidateURL performs comprehensive URL validation
func (v *Validator) ValidateURL(rawURL string) error {
	if err := v.validateFormat(rawURL); err != nil {
		return err
	}

	if err := v.validateLength(rawURL); err != nil {
		return err
	}

	if err := v.validateScheme(rawURL); err != nil {
		return err
	}

	if err := v.validateHost(rawURL); err != nil {
		return err
	}

	return nil
}

// ValidateAndSanitizeURL validates and sanitizes a URL
func (v *Validator) ValidateAndSanitizeURL(rawURL string) (string, error) {
	// First validate
	if err := v.ValidateURL(rawURL); err != nil {
		return "", err
	}

	// Then sanitize
	return v.sanitizeURL(rawURL)
}

// CheckURLReachability checks if a URL is reachable
func (v *Validator) CheckURLReachability(ctx context.Context, rawURL string) (*URLHealth, error) {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "HEAD", rawURL, nil)
	if err != nil {
		return &URLHealth{
			URL:          rawURL,
			Status:       "unreachable",
			LastChecked:  time.Now(),
			Error:        err.Error(),
		}, nil
	}

	// Set a reasonable user agent
	req.Header.Set("User-Agent", "Caslink-Bot/1.0 (+https://github.com/casjaysdevdocker/caslink)")

	resp, err := v.httpClient.Do(req)
	responseTime := time.Since(start)

	health := &URLHealth{
		URL:          rawURL,
		ResponseTime: responseTime.Milliseconds(),
		LastChecked:  time.Now(),
	}

	if err != nil {
		health.Status = "unreachable"
		health.Error = err.Error()
		return health, nil
	}
	defer resp.Body.Close()

	health.StatusCode = resp.StatusCode

	// Determine status based on HTTP status code
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		health.Status = "active"
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		health.Status = "active" // Redirects are OK
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		health.Status = "unreachable"
		health.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	case resp.StatusCode >= 500:
		health.Status = "unreachable"
		health.Error = fmt.Sprintf("HTTP %d - Server Error", resp.StatusCode)
	default:
		health.Status = "unknown"
		health.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return health, nil
}

// validateFormat validates basic URL format
func (v *Validator) validateFormat(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme == "" {
		return fmt.Errorf("URL must include a scheme (http:// or https://)")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must include a host")
	}

	return nil
}

// validateLength validates URL length
func (v *Validator) validateLength(rawURL string) error {
	if len(rawURL) > v.config.MaxURLLength {
		return fmt.Errorf("URL too long (max %d characters, got %d)", v.config.MaxURLLength, len(rawURL))
	}

	if len(rawURL) == 0 {
		return fmt.Errorf("URL cannot be empty")
	}

	return nil
}

// validateScheme validates URL scheme
func (v *Validator) validateScheme(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("only HTTP and HTTPS URLs are supported, got: %s", scheme)
	}

	return nil
}

// validateHost validates URL host
func (v *Validator) validateHost(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	host := strings.ToLower(parsedURL.Host)

	// Check for localhost/private networks (optional security measure)
	if v.isPrivateHost(host) {
		return fmt.Errorf("private/local URLs are not allowed: %s", host)
	}

	// Basic host format validation
	if strings.Contains(host, "..") {
		return fmt.Errorf("invalid host format: %s", host)
	}

	return nil
}

// isPrivateHost checks if a host is private/local
func (v *Validator) isPrivateHost(host string) bool {
	// Remove port if present
	if colonIndex := strings.LastIndex(host, ":"); colonIndex != -1 {
		host = host[:colonIndex]
	}

	privateHosts := []string{
		"localhost",
		"127.0.0.1",
		"::1",
		"0.0.0.0",
	}

	for _, private := range privateHosts {
		if host == private {
			return true
		}
	}

	// Check for private IP ranges
	privatePatterns := []string{
		`^10\.`,
		`^172\.(1[6-9]|2[0-9]|3[01])\.`,
		`^192\.168\.`,
		`^169\.254\.`,
		`^fc[0-9a-f][0-9a-f]:`,
		`^fe[89ab][0-9a-f]:`,
	}

	for _, pattern := range privatePatterns {
		if matched, _ := regexp.MatchString(pattern, host); matched {
			return true
		}
	}

	return false
}

// sanitizeURL sanitizes a URL by normalizing it
func (v *Validator) sanitizeURL(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Normalize scheme to lowercase
	parsedURL.Scheme = strings.ToLower(parsedURL.Scheme)

	// Normalize host to lowercase
	parsedURL.Host = strings.ToLower(parsedURL.Host)

	// Remove default ports
	if parsedURL.Scheme == "http" && strings.HasSuffix(parsedURL.Host, ":80") {
		parsedURL.Host = strings.TrimSuffix(parsedURL.Host, ":80")
	}
	if parsedURL.Scheme == "https" && strings.HasSuffix(parsedURL.Host, ":443") {
		parsedURL.Host = strings.TrimSuffix(parsedURL.Host, ":443")
	}

	// Clean path
	if parsedURL.Path == "" {
		parsedURL.Path = "/"
	}

	// Remove fragment if present (everything after #)
	parsedURL.Fragment = ""

	return parsedURL.String(), nil
}

// ValidateCustomCode validates a custom short code
func (v *Validator) ValidateCustomCode(code string) error {
	if len(code) < v.config.CustomCodeMinLength {
		return fmt.Errorf("custom code too short (minimum %d characters)", v.config.CustomCodeMinLength)
	}

	if len(code) > v.config.CustomCodeMaxLength {
		return fmt.Errorf("custom code too long (maximum %d characters)", v.config.CustomCodeMaxLength)
	}

	// Check allowed characters
	allowedChars := v.config.AllowedCharacters
	if v.config.ExcludeSimilarChars {
		allowedChars = v.removeSimilarChars(allowedChars)
	}

	for _, char := range code {
		if !strings.ContainsRune(allowedChars, char) {
			return fmt.Errorf("invalid character '%c' in custom code", char)
		}
	}

	// Check reserved words
	for _, reserved := range v.config.ReservedWords {
		if strings.EqualFold(code, reserved) {
			return fmt.Errorf("'%s' is a reserved word", code)
		}
	}

	// Additional validation rules
	if err := v.validateCodeContent(code); err != nil {
		return err
	}

	return nil
}

// validateCodeContent performs additional content validation
func (v *Validator) validateCodeContent(code string) error {
	// Check for profanity or inappropriate content
	inappropriateWords := []string{
		"admin", "root", "test", "demo", "api", "www", "mail", "ftp", "ssh",
		"sql", "dev", "stage", "prod", "config", "system", "null", "void",
	}

	lowerCode := strings.ToLower(code)
	for _, word := range inappropriateWords {
		if strings.Contains(lowerCode, word) {
			return fmt.Errorf("code contains inappropriate content: %s", word)
		}
	}

	// Check for patterns that might be confusing
	if matched, _ := regexp.MatchString(`^[0-9]+$`, code); matched {
		return fmt.Errorf("code cannot be all numbers")
	}

	if matched, _ := regexp.MatchString(`^[a-zA-Z]$`, code); matched && len(code) == 1 {
		return fmt.Errorf("single character codes are not allowed")
	}

	return nil
}

// removeSimilarChars removes visually similar characters
func (v *Validator) removeSimilarChars(chars string) string {
	similarChars := map[rune]bool{
		'0': true, 'O': true, 'o': true,
		'1': true, 'l': true, 'I': true,
		'2': true, 'Z': true,
		'5': true, 'S': true,
		'6': true, 'G': true,
		'8': true, 'B': true,
	}

	var result strings.Builder
	for _, char := range chars {
		if !similarChars[char] {
			result.WriteRune(char)
		}
	}

	return result.String()
}

// SanitizeTitle sanitizes a URL title
func (v *Validator) SanitizeTitle(title string) string {
	if title == "" {
		return ""
	}

	// Remove leading/trailing whitespace
	title = strings.TrimSpace(title)

	// Limit length
	maxLength := 255
	if len(title) > maxLength {
		title = title[:maxLength]
	}

	// Remove control characters
	title = regexp.MustCompile(`[\x00-\x1f\x7f-\x9f]`).ReplaceAllString(title, "")

	// Normalize whitespace
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")

	return title
}

// SanitizeDescription sanitizes a URL description
func (v *Validator) SanitizeDescription(description string) string {
	if description == "" {
		return ""
	}

	// Remove leading/trailing whitespace
	description = strings.TrimSpace(description)

	// Limit length
	maxLength := 500
	if len(description) > maxLength {
		description = description[:maxLength]
	}

	// Remove control characters
	description = regexp.MustCompile(`[\x00-\x1f\x7f-\x9f]`).ReplaceAllString(description, "")

	// Normalize whitespace but preserve line breaks
	description = regexp.MustCompile(`[ \t]+`).ReplaceAllString(description, " ")
	description = regexp.MustCompile(`\n{3,}`).ReplaceAllString(description, "\n\n")

	return description
}

// SanitizeTags sanitizes URL tags
func (v *Validator) SanitizeTags(tags string) string {
	if tags == "" {
		return ""
	}

	// Split by comma, trim each tag, and rejoin
	tagList := strings.Split(tags, ",")
	var cleanTags []string

	for _, tag := range tagList {
		tag = strings.TrimSpace(tag)
		if tag != "" && len(tag) <= 50 {
			// Remove special characters except hyphens and underscores
			tag = regexp.MustCompile(`[^\w\-]`).ReplaceAllString(tag, "")
			if tag != "" {
				cleanTags = append(cleanTags, tag)
			}
		}
	}

	// Limit to 10 tags
	if len(cleanTags) > 10 {
		cleanTags = cleanTags[:10]
	}

	return strings.Join(cleanTags, ",")
}