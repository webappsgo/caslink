package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
)

// SMSMessage represents an SMS message
type SMSMessage struct {
	To      string `json:"to"`
	Body    string `json:"body"`
	From    string `json:"from,omitempty"`
}

// TwilioProvider implements SMS sending via Twilio
type TwilioProvider struct {
	config *config.NotificationConfig
	logger *logrus.Logger
}

// NewTwilioProvider creates a new Twilio SMS provider
func NewTwilioProvider(cfg *config.NotificationConfig, logger *logrus.Logger) (*TwilioProvider, error) {
	return &TwilioProvider{
		config: cfg,
		logger: logger,
	}, nil
}

// SendSMS sends an SMS message via Twilio
func (p *TwilioProvider) SendSMS(ctx context.Context, message *SMSMessage) error {
	// Twilio REST API endpoint
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", p.config.TwilioAccountSID)

	// Prepare form data
	data := url.Values{}
	data.Set("To", message.To)
	data.Set("Body", message.Body)
	if message.From != "" {
		data.Set("From", message.From)
	} else {
		data.Set("From", p.config.TwilioFromNumber)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.config.TwilioAccountSID, p.config.TwilioAuthToken)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send SMS via Twilio: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errorResponse struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		}
		if json.NewDecoder(resp.Body).Decode(&errorResponse) == nil {
			return fmt.Errorf("Twilio API error %d: %s", errorResponse.Code, errorResponse.Message)
		}
		return fmt.Errorf("Twilio API error: HTTP %d", resp.StatusCode)
	}

	p.logger.WithFields(logrus.Fields{
		"to":       message.To,
		"provider": "twilio",
	}).Info("SMS sent successfully via Twilio")

	return nil
}

// ValidateConfig validates the Twilio configuration
func (p *TwilioProvider) ValidateConfig() error {
	if p.config.TwilioAccountSID == "" {
		return fmt.Errorf("Twilio Account SID is required")
	}

	if p.config.TwilioAuthToken == "" {
		return fmt.Errorf("Twilio Auth Token is required")
	}

	if p.config.TwilioFromNumber == "" {
		return fmt.Errorf("Twilio from number is required")
	}

	// Basic validation of phone number format
	if !strings.HasPrefix(p.config.TwilioFromNumber, "+") {
		return fmt.Errorf("Twilio from number must include country code (e.g., +1234567890)")
	}

	// Basic SID format validation
	if len(p.config.TwilioAccountSID) != 34 || !strings.HasPrefix(p.config.TwilioAccountSID, "AC") {
		return fmt.Errorf("Twilio Account SID appears to be invalid")
	}

	return nil
}

// TestConnection tests the Twilio API connection
func (p *TwilioProvider) TestConnection(ctx context.Context) error {
	// Test by fetching account information
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s.json", p.config.TwilioAccountSID)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	req.SetBasicAuth(p.config.TwilioAccountSID, p.config.TwilioAuthToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to test Twilio connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Twilio connection test failed: HTTP %d", resp.StatusCode)
	}

	return nil
}