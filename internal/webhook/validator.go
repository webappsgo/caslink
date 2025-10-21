package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
)

// Validator handles webhook validation
type Validator struct {
	config *config.WebhooksConfig
	logger *logrus.Logger
}

// ValidationError represents a webhook validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult contains validation results
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// NewValidator creates a new webhook validator
func NewValidator(cfg *config.WebhooksConfig, logger *logrus.Logger) (*Validator, error) {
	return &Validator{
		config: cfg,
		logger: logger,
	}, nil
}

// ValidateWebhook validates a webhook configuration
func (v *Validator) ValidateWebhook(webhook *Webhook) error {
	var errors []ValidationError

	// Validate URL
	if err := v.validateURL(webhook.URL); err != nil {
		errors = append(errors, ValidationError{
			Field:   "url",
			Message: err.Error(),
		})
	}

	// Validate events
	if err := v.validateEvents(webhook.Events); err != nil {
		errors = append(errors, ValidationError{
			Field:   "events",
			Message: err.Error(),
		})
	}

	// Validate secret
	if err := v.validateSecret(webhook.Secret); err != nil {
		errors = append(errors, ValidationError{
			Field:   "secret",
			Message: err.Error(),
		})
	}

	// Validate headers
	if err := v.validateHeaders(webhook.Headers); err != nil {
		errors = append(errors, ValidationError{
			Field:   "headers",
			Message: err.Error(),
		})
	}

	// Return first error if any
	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

// ValidateWebhookResult validates a webhook and returns detailed results
func (v *Validator) ValidateWebhookResult(webhook *Webhook) *ValidationResult {
	var errors []ValidationError

	// Validate URL
	if err := v.validateURL(webhook.URL); err != nil {
		errors = append(errors, ValidationError{
			Field:   "url",
			Message: err.Error(),
		})
	}

	// Validate events
	if err := v.validateEvents(webhook.Events); err != nil {
		errors = append(errors, ValidationError{
			Field:   "events",
			Message: err.Error(),
		})
	}

	// Validate secret
	if err := v.validateSecret(webhook.Secret); err != nil {
		errors = append(errors, ValidationError{
			Field:   "secret",
			Message: err.Error(),
		})
	}

	// Validate headers
	if err := v.validateHeaders(webhook.Headers); err != nil {
		errors = append(errors, ValidationError{
			Field:   "headers",
			Message: err.Error(),
		})
	}

	return &ValidationResult{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

// ValidateEvent validates an event before processing
func (v *Validator) ValidateEvent(event *Event) error {
	if event.Type == "" {
		return fmt.Errorf("event type is required")
	}

	if !v.isValidEventType(event.Type) {
		return fmt.Errorf("invalid event type: %s", event.Type)
	}

	if event.Data == nil {
		return fmt.Errorf("event data is required")
	}

	return nil
}

// ValidatePayload validates webhook payload and signature
func (v *Validator) ValidatePayload(payload []byte, signature, secret string) error {
	if secret == "" {
		return nil // No secret configured, skip validation
	}

	if signature == "" {
		return fmt.Errorf("signature is required when secret is configured")
	}

	// Parse signature (format: "sha256=...")
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("invalid signature format, expected sha256=...")
	}

	providedSig := strings.TrimPrefix(signature, "sha256=")

	// Calculate expected signature
	expectedSig := v.calculateSignature(payload, secret)

	// Compare signatures using constant-time comparison
	if !hmac.Equal([]byte(providedSig), []byte(expectedSig)) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// validateURL validates webhook URL
func (v *Validator) validateURL(webhookURL string) error {
	if webhookURL == "" {
		return fmt.Errorf("URL is required")
	}

	// Parse URL
	parsedURL, err := url.Parse(webhookURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Must be HTTPS in production
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return fmt.Errorf("URL must use HTTP or HTTPS scheme")
	}

	// Check for localhost/private IPs in production
	if v.config.Enabled && !v.isAllowedHost(parsedURL.Host) {
		return fmt.Errorf("URL points to private/local address")
	}

	// Validate URL length
	if len(webhookURL) > 2048 {
		return fmt.Errorf("URL is too long (max 2048 characters)")
	}

	return nil
}

// validateEvents validates webhook event types
func (v *Validator) validateEvents(events []string) error {
	if len(events) == 0 {
		return fmt.Errorf("at least one event type is required")
	}

	if len(events) > 20 {
		return fmt.Errorf("too many event types (max 20)")
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for _, event := range events {
		if event == "" {
			return fmt.Errorf("empty event type not allowed")
		}

		if seen[event] {
			return fmt.Errorf("duplicate event type: %s", event)
		}
		seen[event] = true

		// Validate event type format
		if !v.isValidEventType(event) {
			return fmt.Errorf("invalid event type: %s", event)
		}
	}

	return nil
}

// validateSecret validates webhook secret
func (v *Validator) validateSecret(secret string) error {
	if secret == "" {
		return nil // Secret is optional
	}

	// Minimum length for security
	if len(secret) < 16 {
		return fmt.Errorf("secret must be at least 16 characters long")
	}

	// Maximum length
	if len(secret) > 255 {
		return fmt.Errorf("secret is too long (max 255 characters)")
	}

	// Check for non-ASCII characters
	for _, r := range secret {
		if r > 127 {
			return fmt.Errorf("secret must contain only ASCII characters")
		}
	}

	return nil
}

// validateHeaders validates custom headers
func (v *Validator) validateHeaders(headers map[string]string) error {
	if len(headers) > 10 {
		return fmt.Errorf("too many custom headers (max 10)")
	}

	forbiddenHeaders := map[string]bool{
		"content-type":        true,
		"user-agent":          true,
		"x-webhook-event":     true,
		"x-webhook-delivery":  true,
		"x-webhook-signature": true,
		"x-webhook-timestamp": true,
		"authorization":       true,
		"host":               true,
		"content-length":     true,
	}

	for name, value := range headers {
		// Validate header name
		if name == "" {
			return fmt.Errorf("header name cannot be empty")
		}

		normalizedName := strings.ToLower(name)
		if forbiddenHeaders[normalizedName] {
			return fmt.Errorf("header '%s' is not allowed", name)
		}

		// Validate header name format
		if !v.isValidHeaderName(name) {
			return fmt.Errorf("invalid header name: %s", name)
		}

		// Validate header value
		if len(value) > 1024 {
			return fmt.Errorf("header value too long (max 1024 characters): %s", name)
		}

		// Check for control characters
		for _, r := range value {
			if r < 32 && r != '\t' {
				return fmt.Errorf("header value contains invalid characters: %s", name)
			}
		}
	}

	return nil
}

// isValidEventType checks if an event type is valid
func (v *Validator) isValidEventType(eventType string) bool {
	// Allow wildcard
	if eventType == "*" {
		return true
	}

	// Define valid event types
	validEvents := map[string]bool{
		EventTypeURLCreated:   true,
		EventTypeURLUpdated:   true,
		EventTypeURLDeleted:   true,
		EventTypeURLClicked:   true,
		EventTypeUserCreated:  true,
		EventTypeUserUpdated:  true,
		EventTypeUserDeleted:  true,
		EventTypeBulkImported: true,
		EventTypeBulkExported: true,
		EventTypeQRGenerated:  true,
		"ping":               true, // Special ping event
	}

	return validEvents[eventType]
}

// isAllowedHost checks if a host is allowed for webhooks
func (v *Validator) isAllowedHost(host string) bool {
	// Extract hostname from host:port
	hostname := host
	if strings.Contains(host, ":") {
		var err error
		hostname, _, err = net.SplitHostPort(host)
		if err != nil {
			return false
		}
	}

	// Parse IP if it's an IP address
	ip := net.ParseIP(hostname)
	if ip != nil {
		// Reject private/local IPs in production
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return false
		}
	}

	// Check for localhost variations
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		return false
	}

	// Check for internal domains
	internalDomains := []string{
		".local",
		".internal",
		".corp",
		".lan",
	}

	for _, domain := range internalDomains {
		if strings.HasSuffix(hostname, domain) {
			return false
		}
	}

	return true
}

// isValidHeaderName validates HTTP header name format
func (v *Validator) isValidHeaderName(name string) bool {
	// RFC 7230 compliant header name
	if name == "" {
		return false
	}

	for _, r := range name {
		if !((r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_') {
			return false
		}
	}

	return true
}

// calculateSignature calculates HMAC-SHA256 signature
func (v *Validator) calculateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateDelivery validates a delivery attempt
func (v *Validator) ValidateDelivery(delivery *Delivery) error {
	if delivery.WebhookID == "" {
		return fmt.Errorf("webhook ID is required")
	}

	if delivery.EventID == "" {
		return fmt.Errorf("event ID is required")
	}

	if delivery.URL == "" {
		return fmt.Errorf("URL is required")
	}

	if delivery.Method == "" {
		delivery.Method = "POST" // Default to POST
	}

	if delivery.Method != "POST" {
		return fmt.Errorf("only POST method is supported")
	}

	return nil
}

// SanitizeWebhook sanitizes webhook data for safe storage
func (v *Validator) SanitizeWebhook(webhook *Webhook) {
	// Trim whitespace from URL
	webhook.URL = strings.TrimSpace(webhook.URL)

	// Normalize events (remove duplicates, trim whitespace)
	seen := make(map[string]bool)
	var cleanEvents []string
	for _, event := range webhook.Events {
		event = strings.TrimSpace(event)
		if event != "" && !seen[event] {
			cleanEvents = append(cleanEvents, event)
			seen[event] = true
		}
	}
	webhook.Events = cleanEvents

	// Sanitize headers
	if webhook.Headers == nil {
		webhook.Headers = make(map[string]string)
	}

	cleanHeaders := make(map[string]string)
	for name, value := range webhook.Headers {
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name != "" && value != "" {
			cleanHeaders[name] = value
		}
	}
	webhook.Headers = cleanHeaders
}

// GetSupportedEvents returns list of supported event types
func (v *Validator) GetSupportedEvents() []string {
	return []string{
		EventTypeURLCreated,
		EventTypeURLUpdated,
		EventTypeURLDeleted,
		EventTypeURLClicked,
		EventTypeUserCreated,
		EventTypeUserUpdated,
		EventTypeUserDeleted,
		EventTypeBulkImported,
		EventTypeBulkExported,
		EventTypeQRGenerated,
	}
}

// ValidateEventData validates event-specific data
func (v *Validator) ValidateEventData(eventType string, data map[string]interface{}) error {
	switch eventType {
	case EventTypeURLCreated, EventTypeURLUpdated:
		return v.validateURLEventData(data)
	case EventTypeURLClicked:
		return v.validateClickEventData(data)
	case EventTypeUserCreated, EventTypeUserUpdated:
		return v.validateUserEventData(data)
	case EventTypeBulkImported, EventTypeBulkExported:
		return v.validateBulkEventData(data)
	case EventTypeQRGenerated:
		return v.validateQREventData(data)
	default:
		// Generic validation for unknown event types
		return nil
	}
}

// validateURLEventData validates URL event data
func (v *Validator) validateURLEventData(data map[string]interface{}) error {
	if _, ok := data["id"]; !ok {
		return fmt.Errorf("URL event data must include 'id' field")
	}

	if _, ok := data["original_url"]; !ok {
		return fmt.Errorf("URL event data must include 'original_url' field")
	}

	if _, ok := data["short_code"]; !ok {
		return fmt.Errorf("URL event data must include 'short_code' field")
	}

	return nil
}

// validateClickEventData validates click event data
func (v *Validator) validateClickEventData(data map[string]interface{}) error {
	if _, ok := data["url_id"]; !ok {
		return fmt.Errorf("Click event data must include 'url_id' field")
	}

	if _, ok := data["timestamp"]; !ok {
		return fmt.Errorf("Click event data must include 'timestamp' field")
	}

	return nil
}

// validateUserEventData validates user event data
func (v *Validator) validateUserEventData(data map[string]interface{}) error {
	if _, ok := data["user_id"]; !ok {
		return fmt.Errorf("User event data must include 'user_id' field")
	}

	return nil
}

// validateBulkEventData validates bulk operation event data
func (v *Validator) validateBulkEventData(data map[string]interface{}) error {
	if _, ok := data["operation_id"]; !ok {
		return fmt.Errorf("Bulk event data must include 'operation_id' field")
	}

	if _, ok := data["count"]; !ok {
		return fmt.Errorf("Bulk event data must include 'count' field")
	}

	return nil
}

// validateQREventData validates QR code event data
func (v *Validator) validateQREventData(data map[string]interface{}) error {
	if _, ok := data["url_id"]; !ok {
		return fmt.Errorf("QR event data must include 'url_id' field")
	}

	if _, ok := data["format"]; !ok {
		return fmt.Errorf("QR event data must include 'format' field")
	}

	return nil
}