package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
)

// PushMessage represents a push notification message
type PushMessage struct {
	Token   string            `json:"token"`
	Title   string            `json:"title"`
	Body    string            `json:"body"`
	Data    map[string]string `json:"data,omitempty"`
	ImageURL string           `json:"image_url,omitempty"`
}

// FCMProvider implements push notifications via Firebase Cloud Messaging
type FCMProvider struct {
	config *config.NotificationConfig
	logger *logrus.Logger
}

// NewFCMProvider creates a new Firebase Cloud Messaging provider
func NewFCMProvider(cfg *config.NotificationConfig, logger *logrus.Logger) (*FCMProvider, error) {
	return &FCMProvider{
		config: cfg,
		logger: logger,
	}, nil
}

// SendPush sends a push notification via FCM
func (p *FCMProvider) SendPush(ctx context.Context, message *PushMessage) error {
	// FCM API endpoint
	apiURL := "https://fcm.googleapis.com/fcm/send"

	// Prepare FCM payload
	payload := map[string]interface{}{
		"to": message.Token,
		"notification": map[string]interface{}{
			"title": message.Title,
			"body":  message.Body,
		},
	}

	// Add optional fields
	if message.ImageURL != "" {
		payload["notification"].(map[string]interface{})["image"] = message.ImageURL
	}

	if message.Data != nil && len(message.Data) > 0 {
		payload["data"] = message.Data
	}

	// Convert to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal FCM payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+p.config.FCMServerKey)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send push notification via FCM: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var fcmResponse struct {
		Success      int `json:"success"`
		Failure      int `json:"failure"`
		CanonicalIDs int `json:"canonical_ids"`
		Results      []struct {
			MessageID      string `json:"message_id"`
			RegistrationID string `json:"registration_id"`
			Error          string `json:"error"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&fcmResponse); err != nil {
		return fmt.Errorf("failed to decode FCM response: %w", err)
	}

	// Check for errors
	if resp.StatusCode != 200 {
		return fmt.Errorf("FCM API error: HTTP %d", resp.StatusCode)
	}

	if fcmResponse.Failure > 0 && len(fcmResponse.Results) > 0 {
		if fcmResponse.Results[0].Error != "" {
			return fmt.Errorf("FCM delivery error: %s", fcmResponse.Results[0].Error)
		}
	}

	p.logger.WithFields(logrus.Fields{
		"token":    message.Token,
		"title":    message.Title,
		"provider": "fcm",
		"success":  fcmResponse.Success,
		"failure":  fcmResponse.Failure,
	}).Info("Push notification sent successfully via FCM")

	return nil
}

// ValidateConfig validates the FCM configuration
func (p *FCMProvider) ValidateConfig() error {
	if p.config.FCMServerKey == "" {
		return fmt.Errorf("FCM server key is required")
	}

	// Basic server key format validation
	if len(p.config.FCMServerKey) < 100 {
		return fmt.Errorf("FCM server key appears to be invalid (too short)")
	}

	return nil
}

// TestConnection tests the FCM API connection
func (p *FCMProvider) TestConnection(ctx context.Context) error {
	// Test with a dummy token to verify API key validity
	testMessage := &PushMessage{
		Token: "test_token_for_validation",
		Title: "Test",
		Body:  "Connection test",
	}

	// Create test payload
	payload := map[string]interface{}{
		"to": testMessage.Token,
		"notification": map[string]interface{}{
			"title": testMessage.Title,
			"body":  testMessage.Body,
		},
		"dry_run": true, // This prevents actual delivery
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal test payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://fcm.googleapis.com/fcm/send", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+p.config.FCMServerKey)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to test FCM connection: %w", err)
	}
	defer resp.Body.Close()

	// Check authentication - 401 means invalid key, 400 means invalid token (expected for test)
	if resp.StatusCode == 401 {
		return fmt.Errorf("FCM connection test failed: invalid server key")
	}

	// For test tokens, we expect 400 (invalid registration token) which is fine
	if resp.StatusCode != 200 && resp.StatusCode != 400 {
		return fmt.Errorf("FCM connection test failed: HTTP %d", resp.StatusCode)
	}

	return nil
}

// APNSProvider implements push notifications via Apple Push Notification Service
type APNSProvider struct {
	config *config.NotificationConfig
	logger *logrus.Logger
}

// NewAPNSProvider creates a new Apple Push Notification Service provider
func NewAPNSProvider(cfg *config.NotificationConfig, logger *logrus.Logger) (*APNSProvider, error) {
	return &APNSProvider{
		config: cfg,
		logger: logger,
	}, nil
}

// SendPush sends a push notification via APNS
func (p *APNSProvider) SendPush(ctx context.Context, message *PushMessage) error {
	// Note: This is a simplified implementation
	// In production, you would use a proper APNS library like github.com/sideshow/apns2
	// that handles certificate/token authentication and HTTP/2 requirements

	// APNS requires HTTP/2 and proper certificate authentication
	// This implementation would need enhancement for production use
	return fmt.Errorf("APNS implementation requires proper HTTP/2 client and certificate handling")
}

// ValidateConfig validates the APNS configuration
func (p *APNSProvider) ValidateConfig() error {
	if p.config.APNSBundleID == "" {
		return fmt.Errorf("APNS bundle ID is required")
	}

	// Check if we have either certificate files or auth key
	hasCert := p.config.APNSCertPath != "" && p.config.APNSKeyPath != ""

	if !hasCert {
		return fmt.Errorf("APNS requires certificate and key files")
	}

	return nil
}

// TestConnection tests the APNS connection
func (p *APNSProvider) TestConnection(ctx context.Context) error {
	// This would require implementing proper APNS HTTP/2 client
	return fmt.Errorf("APNS test connection not implemented - requires proper HTTP/2 client")
}