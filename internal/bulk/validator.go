package bulk

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
)

// Validator handles bulk data validation
type Validator struct {
	config *config.BulkConfig
	logger *logrus.Logger
}

// NewValidator creates a new validator instance
func NewValidator(cfg *config.BulkConfig, logger *logrus.Logger) (*Validator, error) {
	return &Validator{
		config: cfg,
		logger: logger,
	}, nil
}

// ValidateURLRecord validates a single URL record
func (v *Validator) ValidateURLRecord(record URLRecord, row int) []ImportError {
	var errors []ImportError

	// Validate original URL
	if urlErrors := v.validateOriginalURL(record.OriginalURL, row); len(urlErrors) > 0 {
		errors = append(errors, urlErrors...)
	}

	// Validate short code
	if record.ShortCode != "" {
		if codeErrors := v.validateShortCode(record.ShortCode, row); len(codeErrors) > 0 {
			errors = append(errors, codeErrors...)
		}
	}

	// Validate title
	if titleErrors := v.validateTitle(record.Title, row); len(titleErrors) > 0 {
		errors = append(errors, titleErrors...)
	}

	// Validate description
	if descErrors := v.validateDescription(record.Description, row); len(descErrors) > 0 {
		errors = append(errors, descErrors...)
	}

	// Validate tags
	if tagErrors := v.validateTags(record.Tags, row); len(tagErrors) > 0 {
		errors = append(errors, tagErrors...)
	}

	// Validate expiration date
	if record.ExpiresAt != nil {
		if expErrors := v.validateExpiresAt(*record.ExpiresAt, row); len(expErrors) > 0 {
			errors = append(errors, expErrors...)
		}
	}

	// Validate password
	if record.Password != "" {
		if pwdErrors := v.validatePassword(record.Password, row); len(pwdErrors) > 0 {
			errors = append(errors, pwdErrors...)
		}
	}

	return errors
}

// validateOriginalURL validates the original URL field
func (v *Validator) validateOriginalURL(originalURL string, row int) []ImportError {
	var errors []ImportError

	// Required field check
	if originalURL == "" {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "original_url",
			Value:       originalURL,
			Error:       "required",
			Description: "Original URL is required",
		})
		return errors
	}

	// Length check
	if len(originalURL) > 2048 {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "original_url",
			Value:       originalURL,
			Error:       "too_long",
			Description: "URL cannot exceed 2048 characters",
		})
	}

	// URL format validation
	if !v.isValidURL(originalURL) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "original_url",
			Value:       originalURL,
			Error:       "invalid_format",
			Description: "URL format is invalid",
		})
		return errors
	}

	// Protocol validation
	if !v.hasValidProtocol(originalURL) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "original_url",
			Value:       originalURL,
			Error:       "invalid_protocol",
			Description: "URL must use HTTP or HTTPS protocol",
		})
	}

	// Malicious URL check
	if v.isSuspiciousURL(originalURL) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "original_url",
			Value:       originalURL,
			Error:       "suspicious_url",
			Description: "URL appears to be potentially malicious",
		})
	}

	return errors
}

// validateShortCode validates the short code field
func (v *Validator) validateShortCode(shortCode string, row int) []ImportError {
	var errors []ImportError

	// Length validation
	if len(shortCode) < 3 {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "short_code",
			Value:       shortCode,
			Error:       "too_short",
			Description: "Short code must be at least 3 characters",
		})
	}

	if len(shortCode) > 50 {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "short_code",
			Value:       shortCode,
			Error:       "too_long",
			Description: "Short code cannot exceed 50 characters",
		})
	}

	// Character validation
	if !v.isValidShortCode(shortCode) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "short_code",
			Value:       shortCode,
			Error:       "invalid_characters",
			Description: "Short code can only contain letters, numbers, hyphens, and underscores",
		})
	}

	// Reserved word check
	if v.isReservedWord(shortCode) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "short_code",
			Value:       shortCode,
			Error:       "reserved_word",
			Description: "Short code is a reserved word",
		})
	}

	return errors
}

// validateTitle validates the title field
func (v *Validator) validateTitle(title string, row int) []ImportError {
	var errors []ImportError

	if len(title) > 255 {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "title",
			Value:       title,
			Error:       "too_long",
			Description: "Title cannot exceed 255 characters",
		})
	}

	// Check for potentially harmful content
	if v.containsHarmfulContent(title) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "title",
			Value:       title,
			Error:       "harmful_content",
			Description: "Title contains potentially harmful content",
		})
	}

	return errors
}

// validateDescription validates the description field
func (v *Validator) validateDescription(description string, row int) []ImportError {
	var errors []ImportError

	if len(description) > 500 {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "description",
			Value:       description,
			Error:       "too_long",
			Description: "Description cannot exceed 500 characters",
		})
	}

	// Check for potentially harmful content
	if v.containsHarmfulContent(description) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "description",
			Value:       description,
			Error:       "harmful_content",
			Description: "Description contains potentially harmful content",
		})
	}

	return errors
}

// validateTags validates the tags field
func (v *Validator) validateTags(tags []string, row int) []ImportError {
	var errors []ImportError

	if len(tags) > 20 {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "tags",
			Value:       strings.Join(tags, ","),
			Error:       "too_many",
			Description: "Cannot have more than 20 tags",
		})
	}

	for i, tag := range tags {
		if len(tag) > 50 {
			errors = append(errors, ImportError{
				Row:         row,
				Field:       "tags",
				Value:       tag,
				Error:       "tag_too_long",
				Description: fmt.Sprintf("Tag %d exceeds 50 characters", i+1),
			})
		}

		if !v.isValidTag(tag) {
			errors = append(errors, ImportError{
				Row:         row,
				Field:       "tags",
				Value:       tag,
				Error:       "invalid_tag",
				Description: fmt.Sprintf("Tag %d contains invalid characters", i+1),
			})
		}
	}

	return errors
}

// validateExpiresAt validates the expiration date
func (v *Validator) validateExpiresAt(expiresAt time.Time, row int) []ImportError {
	var errors []ImportError

	// Check if date is in the past
	if expiresAt.Before(time.Now()) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "expires_at",
			Value:       expiresAt.Format(time.RFC3339),
			Error:       "past_date",
			Description: "Expiration date cannot be in the past",
		})
	}

	// Check if date is too far in the future
	maxFuture := time.Now().AddDate(10, 0, 0) // 10 years
	if expiresAt.After(maxFuture) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "expires_at",
			Value:       expiresAt.Format(time.RFC3339),
			Error:       "too_far_future",
			Description: "Expiration date cannot be more than 10 years in the future",
		})
	}

	return errors
}

// validatePassword validates the password field
func (v *Validator) validatePassword(password string, row int) []ImportError {
	var errors []ImportError

	if len(password) > 100 {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "password",
			Value:       password,
			Error:       "too_long",
			Description: "Password cannot exceed 100 characters",
		})
	}

	// Check password strength (optional)
	if v.config.ValidatePasswords && !v.isStrongPassword(password) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "password",
			Value:       password,
			Error:       "weak_password",
			Description: "Password does not meet strength requirements",
		})
	}

	return errors
}

// Helper validation functions

// isValidURL checks if a string is a valid URL
func (v *Validator) isValidURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return u.Scheme != "" && u.Host != ""
}

// hasValidProtocol checks if URL has HTTP/HTTPS protocol
func (v *Validator) hasValidProtocol(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return u.Scheme == "http" || u.Scheme == "https"
}

// isSuspiciousURL checks for potentially malicious URLs
func (v *Validator) isSuspiciousURL(rawURL string) bool {
	suspiciousPatterns := []string{
		// Common malicious patterns
		"bit\\.ly/[a-zA-Z0-9]{6}", // Suspicious bit.ly patterns
		"tinyurl\\.com/[a-zA-Z0-9]{6}",
		"goo\\.gl/[a-zA-Z0-9]{6}",
		// IP addresses instead of domains
		"https?://\\d+\\.\\d+\\.\\d+\\.\\d+",
		// Suspicious file extensions
		"\\.(exe|bat|scr|pif|com|cmd|jar)$",
		// Suspicious keywords
		"(phishing|malware|virus|trojan|hack)",
	}

	lowerURL := strings.ToLower(rawURL)
	for _, pattern := range suspiciousPatterns {
		matched, _ := regexp.MatchString(pattern, lowerURL)
		if matched {
			return true
		}
	}

	return false
}

// isValidShortCode checks if short code contains only allowed characters
func (v *Validator) isValidShortCode(code string) bool {
	// Allow alphanumeric characters, hyphens, and underscores
	validPattern := "^[a-zA-Z0-9_-]+$"
	matched, _ := regexp.MatchString(validPattern, code)
	return matched
}

// isReservedWord checks if short code is a reserved word
func (v *Validator) isReservedWord(code string) bool {
	reservedWords := []string{
		"api", "admin", "www", "app", "help", "about", "setup",
		"login", "register", "dashboard", "analytics", "qr",
		"bulk", "export", "import", "health", "metrics",
		"docs", "swagger", "static", "assets", "js", "css",
		"img", "images", "fonts", "favicon", "robots",
		"sitemap", "manifest", "service-worker",
	}

	lowerCode := strings.ToLower(code)
	for _, word := range reservedWords {
		if lowerCode == word {
			return true
		}
	}

	return false
}

// containsHarmfulContent checks for potentially harmful content
func (v *Validator) containsHarmfulContent(content string) bool {
	harmfulPatterns := []string{
		// Script tags
		"<script[^>]*>",
		"</script>",
		"javascript:",
		"vbscript:",
		// SQL injection patterns
		"(union|select|insert|update|delete|drop|create|alter)\\s+",
		"'\\s*(or|and)\\s*'",
		// XSS patterns
		"(onload|onerror|onclick|onmouseover)\\s*=",
		"alert\\s*\\(",
		"document\\.",
		"window\\.",
	}

	lowerContent := strings.ToLower(content)
	for _, pattern := range harmfulPatterns {
		matched, _ := regexp.MatchString(pattern, lowerContent)
		if matched {
			return true
		}
	}

	return false
}

// isValidTag checks if a tag contains only allowed characters
func (v *Validator) isValidTag(tag string) bool {
	// Allow alphanumeric characters, spaces, hyphens, and underscores
	validPattern := "^[a-zA-Z0-9\\s_-]+$"
	matched, _ := regexp.MatchString(validPattern, tag)
	return matched
}

// isStrongPassword checks password strength
func (v *Validator) isStrongPassword(password string) bool {
	if len(password) < 8 {
		return false
	}

	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
	hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\?]`).MatchString(password)

	// Require at least 3 of the 4 character types
	score := 0
	if hasUpper {
		score++
	}
	if hasLower {
		score++
	}
	if hasNumber {
		score++
	}
	if hasSpecial {
		score++
	}

	return score >= 3
}

// ValidateBatchSize validates the batch size for processing
func (v *Validator) ValidateBatchSize(size int) error {
	if size <= 0 {
		return fmt.Errorf("batch size must be positive")
	}

	if size > MaxBatchSize {
		return fmt.Errorf("batch size %d exceeds maximum %d", size, MaxBatchSize)
	}

	return nil
}

// ValidateFileSize validates the uploaded file size
func (v *Validator) ValidateFileSize(size int64) error {
	if size <= 0 {
		return fmt.Errorf("file size must be positive")
	}

	if size > MaxFileSize {
		return fmt.Errorf("file size %d exceeds maximum %d", size, MaxFileSize)
	}

	return nil
}

// ValidateFormat validates the file format
func (v *Validator) ValidateFormat(format string) error {
	validFormats := map[string]bool{
		FormatCSV:  true,
		FormatJSON: true,
		FormatXLSX: true,
		FormatTXT:  true,
	}

	if !validFormats[strings.ToLower(format)] {
		return fmt.Errorf("unsupported format: %s", format)
	}

	return nil
}

// ValidateUserPermissions validates user permissions for bulk operations
func (v *Validator) ValidateUserPermissions(userID string, operationType string) error {
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}

	// In a real implementation, you would check user permissions here
	// For now, we'll just validate the operation type

	validOperations := map[string]bool{
		"import": true,
		"export": true,
	}

	if !validOperations[operationType] {
		return fmt.Errorf("invalid operation type: %s", operationType)
	}

	return nil
}

// GetValidationStats returns validation statistics
func (v *Validator) GetValidationStats(errors []ImportError) map[string]int {
	stats := make(map[string]int)

	for _, err := range errors {
		stats[err.Error]++
	}

	return stats
}

// SummarizeValidationErrors creates a summary of validation errors
func (v *Validator) SummarizeValidationErrors(errors []ImportError) string {
	if len(errors) == 0 {
		return "No validation errors found"
	}

	stats := v.GetValidationStats(errors)
	var summary strings.Builder

	summary.WriteString(fmt.Sprintf("Found %d validation errors:\n", len(errors)))

	for errorType, count := range stats {
		summary.WriteString(fmt.Sprintf("- %s: %d\n", errorType, count))
	}

	return summary.String()
}