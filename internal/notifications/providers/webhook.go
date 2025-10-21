package providers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
)

// WebhookMessage represents a webhook notification message
type WebhookMessage struct {
	URL     string                 `json:"url"`
	Secret  string                 `json:"secret,omitempty"`
	Headers map[string]string      `json:"headers,omitempty"`
	Payload map[string]interface{} `json:"payload"`
	Timeout time.Duration          `json:"timeout,omitempty"`
}

// WebhookProvider implements webhook notifications
type WebhookProvider struct {
	config *config.NotificationConfig
	logger *logrus.Logger
}

// NewWebhookProvider creates a new webhook notification provider
func NewWebhookProvider(cfg *config.NotificationConfig, logger *logrus.Logger) (*WebhookProvider, error) {
	return &WebhookProvider{
		config: cfg,
		logger: logger,
	}, nil
}

// SendWebhook sends a webhook notification
func (p *WebhookProvider) SendWebhook(ctx context.Context, message *WebhookMessage) error {
	// Convert payload to JSON
	jsonPayload, err := json.Marshal(message.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", message.URL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	// Set default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Caslink-Webhook/1.0")
	req.Header.Set("X-Caslink-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	// Add custom headers
	for key, value := range message.Headers {
		req.Header.Set(key, value)
	}

	// Add HMAC signature if secret is provided
	if message.Secret != "" {
		signature := p.generateSignature(jsonPayload, message.Secret)
		req.Header.Set("X-Caslink-Signature", signature)
		req.Header.Set("X-Caslink-Signature-256", "sha256="+signature)
	}

	// Set timeout
	timeout := message.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{
		Timeout: timeout,
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook failed with status %d", resp.StatusCode)
	}

	p.logger.WithFields(logrus.Fields{
		"url":        message.URL,
		"status":     resp.StatusCode,
		"provider":   "webhook",
		"payload_size": len(jsonPayload),
	}).Info("Webhook notification sent successfully")

	return nil
}

// generateSignature generates HMAC-SHA256 signature for webhook payload
func (p *WebhookProvider) generateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateConfig validates the webhook configuration
func (p *WebhookProvider) ValidateConfig() error {
	// Webhook provider doesn't require global configuration
	// Validation is done per webhook message
	return nil
}

// ValidateWebhookMessage validates a webhook message
func (p *WebhookProvider) ValidateWebhookMessage(message *WebhookMessage) error {
	if message.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	// Basic URL validation
	if !isValidURL(message.URL) {
		return fmt.Errorf("invalid webhook URL format")
	}

	// Security check - don't allow private networks in production
	if !isAllowedWebhookURL(message.URL) {
		return fmt.Errorf("webhook URL not allowed for security reasons")
	}

	if message.Payload == nil {
		return fmt.Errorf("webhook payload is required")
	}

	return nil
}

// TestConnection tests a webhook URL
func (p *WebhookProvider) TestConnection(ctx context.Context, url string) error {
	// Send a test ping payload
	testPayload := map[string]interface{}{
		"event": "ping",
		"test":  true,
		"timestamp": time.Now().Unix(),
		"message": "Test webhook from Caslink",
	}

	testMessage := &WebhookMessage{
		URL:     url,
		Payload: testPayload,
		Timeout: 10 * time.Second,
	}

	if err := p.ValidateWebhookMessage(testMessage); err != nil {
		return fmt.Errorf("webhook validation failed: %w", err)
	}

	return p.SendWebhook(ctx, testMessage)
}

// isValidURL performs basic URL validation
func isValidURL(urlStr string) bool {
	// Basic checks for HTTP/HTTPS URLs
	return len(urlStr) > 0 &&
		   (len(urlStr) >= 7 && urlStr[:7] == "http://") ||
		   (len(urlStr) >= 8 && urlStr[:8] == "https://")
}

// isAllowedWebhookURL checks if the webhook URL is allowed
func isAllowedWebhookURL(urlStr string) bool {
	// In production, you might want to:
	// 1. Block private IP ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
	// 2. Block localhost/127.0.0.1
	// 3. Block metadata services (169.254.169.254)
	// 4. Use an allowlist of domains

	// For now, we allow all HTTP/HTTPS URLs
	// This should be enhanced based on security requirements
	return isValidURL(urlStr)
}

// WebhookNotificationPayload represents the standard payload structure for webhook notifications
type WebhookNotificationPayload struct {
	Event       string                 `json:"event"`
	Timestamp   int64                  `json:"timestamp"`
	Source      string                 `json:"source"`
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Subject     string                 `json:"subject"`
	Content     string                 `json:"content"`
	Data        map[string]interface{} `json:"data,omitempty"`
	UserID      *string                `json:"user_id,omitempty"`
	Retry       int                    `json:"retry,omitempty"`
}

// CreateStandardPayload creates a standard webhook payload for notifications
func (p *WebhookProvider) CreateStandardPayload(notificationID, notificationType, subject, content string, userID *string, data map[string]interface{}) *WebhookNotificationPayload {
	return &WebhookNotificationPayload{
		Event:     "notification",
		Timestamp: time.Now().Unix(),
		Source:    "caslink",
		ID:        notificationID,
		Type:      notificationType,
		Subject:   subject,
		Content:   content,
		Data:      data,
		UserID:    userID,
		Retry:     0,
	}
}