package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Dispatcher handles webhook delivery
type Dispatcher struct {
	db        *db.DB
	config    *config.WebhooksConfig
	logger    *logrus.Logger
	client    *http.Client
	validator *Validator
}

// WebhookPayload represents the payload sent to webhook endpoints
type WebhookPayload struct {
	Event     *Event                 `json:"event"`
	Timestamp int64                  `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Signature string                 `json:"signature,omitempty"`
}

// NewDispatcher creates a new webhook dispatcher
func NewDispatcher(database *db.DB, cfg *config.WebhooksConfig, logger *logrus.Logger, client *http.Client, validator *Validator) (*Dispatcher, error) {
	return &Dispatcher{
		db:        database,
		config:    cfg,
		logger:    logger,
		client:    client,
		validator: validator,
	}, nil
}

// Start starts the dispatcher worker
func (d *Dispatcher) Start(ctx context.Context) {
	d.logger.Info("Starting webhook dispatcher")

	ticker := time.NewTicker(10 * time.Second) // Check for pending deliveries every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("Webhook dispatcher stopped")
			return
		case <-ticker.C:
			if err := d.processPendingDeliveries(ctx); err != nil {
				d.logger.WithError(err).Error("Failed to process pending deliveries")
			}
		}
	}
}

// Dispatch immediately dispatches a webhook
func (d *Dispatcher) Dispatch(ctx context.Context, delivery *Delivery, event *Event, webhook *Webhook) (bool, error) {
	d.logger.WithFields(logrus.Fields{
		"delivery_id": delivery.ID,
		"webhook_id":  webhook.ID,
		"event_type":  event.Type,
	}).Debug("Dispatching webhook")

	// Create payload
	payload := &WebhookPayload{
		Event:     event,
		Timestamp: time.Now().Unix(),
		Data:      event.Data,
	}

	// Serialize payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Add signature if webhook has secret
	if webhook.Secret != "" {
		signature := d.generateSignature(payloadBytes, webhook.Secret)
		payload.Signature = signature

		// Re-marshal with signature
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			return false, fmt.Errorf("failed to marshal payload with signature: %w", err)
		}
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Caslink-Webhooks/1.0")
	req.Header.Set("X-Webhook-Event", event.Type)
	req.Header.Set("X-Webhook-Delivery", delivery.ID)
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", payload.Timestamp))

	// Add signature header if present
	if webhook.Secret != "" {
		req.Header.Set("X-Webhook-Signature", "sha256="+payload.Signature)
	}

	// Add custom headers
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := d.client.Do(req)
	if err != nil {
		delivery.Error = err.Error()
		delivery.StatusCode = 0
		d.saveDeliveryResult(ctx, delivery)
		return false, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		d.logger.WithError(err).Warn("Failed to read response body")
		responseBody = []byte("Failed to read response")
	}

	// Update delivery with response
	delivery.StatusCode = resp.StatusCode
	delivery.Response = string(responseBody)

	// Check if delivery was successful
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	delivery.Success = success

	if success {
		now := time.Now()
		delivery.DeliveredAt = &now
		d.updateWebhookSuccess(ctx, webhook.ID)
	} else {
		delivery.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)
		d.updateWebhookFailure(ctx, webhook.ID)
	}

	// Save delivery result
	d.saveDeliveryResult(ctx, delivery)

	d.logger.WithFields(logrus.Fields{
		"delivery_id":  delivery.ID,
		"webhook_id":   webhook.ID,
		"status_code":  resp.StatusCode,
		"success":      success,
	}).Info("Webhook delivered")

	return success, nil
}

// processPendingDeliveries processes deliveries that need retry
func (d *Dispatcher) processPendingDeliveries(ctx context.Context) error {
	// Get pending deliveries that are ready for retry
	deliveries, err := d.getPendingDeliveries(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending deliveries: %w", err)
	}

	for _, delivery := range deliveries {
		// Get event and webhook for this delivery
		event, err := d.getEvent(ctx, delivery.EventID)
		if err != nil {
			d.logger.WithError(err).WithField("event_id", delivery.EventID).Error("Failed to get event for delivery")
			continue
		}

		webhook, err := d.getWebhook(ctx, delivery.WebhookID)
		if err != nil {
			d.logger.WithError(err).WithField("webhook_id", delivery.WebhookID).Error("Failed to get webhook for delivery")
			continue
		}

		// Skip if webhook is inactive
		if !webhook.Active {
			d.logger.WithField("webhook_id", webhook.ID).Debug("Skipping inactive webhook")
			continue
		}

		// Check if we should retry
		if delivery.Attempt > d.config.RetryAttempts {
			d.logger.WithFields(logrus.Fields{
				"delivery_id": delivery.ID,
				"attempts":    delivery.Attempt,
			}).Info("Delivery exceeded max retry attempts")
			d.markDeliveryFailed(ctx, delivery.ID)
			continue
		}

		// Increment attempt counter
		delivery.Attempt++

		// Attempt delivery
		success, err := d.Dispatch(ctx, delivery, event, webhook)
		if err != nil {
			d.logger.WithError(err).WithField("delivery_id", delivery.ID).Error("Delivery attempt failed")
		}

		// Schedule next retry if needed
		if !success && delivery.Attempt <= d.config.RetryAttempts {
			nextAttempt := d.calculateNextAttempt(delivery.Attempt)
			delivery.NextAttempt = &nextAttempt
			d.updateDeliveryNextAttempt(ctx, delivery.ID, nextAttempt)
		}
	}

	return nil
}

// getPendingDeliveries retrieves deliveries that need retry
func (d *Dispatcher) getPendingDeliveries(ctx context.Context) ([]*Delivery, error) {
	query := `
		SELECT id, webhook_id, event_id, url, method, attempt, created_at
		FROM webhook_deliveries
		WHERE success = false AND attempt <= ? AND (next_attempt IS NULL OR next_attempt <= ?)
		ORDER BY created_at ASC
		LIMIT 100`

	now := time.Now()
	rows, err := d.db.QueryContext(ctx, query, d.config.RetryAttempts, now)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []*Delivery

	for rows.Next() {
		delivery := &Delivery{}
		err := rows.Scan(
			&delivery.ID, &delivery.WebhookID, &delivery.EventID,
			&delivery.URL, &delivery.Method, &delivery.Attempt, &delivery.CreatedAt,
		)
		if err != nil {
			d.logger.WithError(err).Warn("Failed to scan pending delivery")
			continue
		}

		deliveries = append(deliveries, delivery)
	}

	return deliveries, nil
}

// getEvent retrieves an event by ID
func (d *Dispatcher) getEvent(ctx context.Context, eventID string) (*Event, error) {
	query := `
		SELECT id, type, timestamp, data, user_id, source
		FROM webhook_events
		WHERE id = ?`

	row := d.db.QueryRowContext(ctx, query, eventID)

	event := &Event{}
	var dataJSON string

	err := row.Scan(
		&event.ID, &event.Type, &event.Timestamp,
		&dataJSON, &event.UserID, &event.Source,
	)

	if err != nil {
		return nil, fmt.Errorf("event not found: %w", err)
	}

	// Parse data JSON
	if err := json.Unmarshal([]byte(dataJSON), &event.Data); err != nil {
		return nil, fmt.Errorf("failed to parse event data: %w", err)
	}

	return event, nil
}

// getWebhook retrieves a webhook by ID
func (d *Dispatcher) getWebhook(ctx context.Context, webhookID string) (*Webhook, error) {
	query := `
		SELECT id, user_id, url, secret, events, active, headers
		FROM webhooks
		WHERE id = ?`

	row := d.db.QueryRowContext(ctx, query, webhookID)

	webhook := &Webhook{}
	var eventsJSON, headersJSON string

	err := row.Scan(
		&webhook.ID, &webhook.UserID, &webhook.URL, &webhook.Secret,
		&eventsJSON, &webhook.Active, &headersJSON,
	)

	if err != nil {
		return nil, fmt.Errorf("webhook not found: %w", err)
	}

	// Parse events JSON
	if err := json.Unmarshal([]byte(eventsJSON), &webhook.Events); err != nil {
		return nil, fmt.Errorf("failed to parse webhook events: %w", err)
	}

	// Parse headers JSON
	if headersJSON != "" {
		if err := json.Unmarshal([]byte(headersJSON), &webhook.Headers); err != nil {
			d.logger.WithError(err).Warn("Failed to parse webhook headers")
		}
	}

	return webhook, nil
}

// saveDeliveryResult saves the delivery result to database
func (d *Dispatcher) saveDeliveryResult(ctx context.Context, delivery *Delivery) {
	query := `
		INSERT OR REPLACE INTO webhook_deliveries
		(id, webhook_id, event_id, url, method, status_code, response, error, attempt, delivered_at, created_at, success)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := d.db.ExecContext(ctx, query,
		delivery.ID, delivery.WebhookID, delivery.EventID, delivery.URL,
		delivery.Method, delivery.StatusCode, delivery.Response, delivery.Error,
		delivery.Attempt, delivery.DeliveredAt, delivery.CreatedAt, delivery.Success,
	)

	if err != nil {
		d.logger.WithError(err).WithField("delivery_id", delivery.ID).Error("Failed to save delivery result")
	}
}

// updateWebhookSuccess updates webhook success statistics
func (d *Dispatcher) updateWebhookSuccess(ctx context.Context, webhookID string) {
	query := `
		UPDATE webhooks
		SET last_success = ?, fail_count = 0
		WHERE id = ?`

	_, err := d.db.ExecContext(ctx, query, time.Now(), webhookID)
	if err != nil {
		d.logger.WithError(err).WithField("webhook_id", webhookID).Error("Failed to update webhook success")
	}
}

// updateWebhookFailure updates webhook failure statistics
func (d *Dispatcher) updateWebhookFailure(ctx context.Context, webhookID string) {
	query := `
		UPDATE webhooks
		SET fail_count = fail_count + 1
		WHERE id = ?`

	_, err := d.db.ExecContext(ctx, query, webhookID)
	if err != nil {
		d.logger.WithError(err).WithField("webhook_id", webhookID).Error("Failed to update webhook failure")
	}
}

// markDeliveryFailed marks a delivery as permanently failed
func (d *Dispatcher) markDeliveryFailed(ctx context.Context, deliveryID string) {
	query := `
		UPDATE webhook_deliveries
		SET error = 'Exceeded maximum retry attempts', next_attempt = NULL
		WHERE id = ?`

	_, err := d.db.ExecContext(ctx, query, deliveryID)
	if err != nil {
		d.logger.WithError(err).WithField("delivery_id", deliveryID).Error("Failed to mark delivery as failed")
	}
}

// updateDeliveryNextAttempt updates the next attempt time for a delivery
func (d *Dispatcher) updateDeliveryNextAttempt(ctx context.Context, deliveryID string, nextAttempt time.Time) {
	query := `
		UPDATE webhook_deliveries
		SET next_attempt = ?
		WHERE id = ?`

	_, err := d.db.ExecContext(ctx, query, nextAttempt, deliveryID)
	if err != nil {
		d.logger.WithError(err).WithField("delivery_id", deliveryID).Error("Failed to update delivery next attempt")
	}
}

// generateSignature generates HMAC-SHA256 signature for webhook payload
func (d *Dispatcher) generateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// calculateNextAttempt calculates the next retry attempt time using exponential backoff
func (d *Dispatcher) calculateNextAttempt(attempt int) time.Time {
	// Exponential backoff: 1s, 2s, 4s, 8s, 16s, etc.
	backoffSeconds := 1 << (attempt - 1)
	if backoffSeconds > 3600 { // Cap at 1 hour
		backoffSeconds = 3600
	}

	return time.Now().Add(time.Duration(backoffSeconds) * time.Second)
}

// GetDeliveryStats returns delivery statistics
func (d *Dispatcher) GetDeliveryStats(ctx context.Context, webhookID string, days int) (*DeliveryStats, error) {
	stats := &DeliveryStats{}

	// Calculate date range
	since := time.Now().AddDate(0, 0, -days)

	// Total deliveries
	query := `
		SELECT COUNT(*) FROM webhook_deliveries
		WHERE webhook_id = ? AND created_at >= ?`
	err := d.db.QueryRowContext(ctx, query, webhookID, since).Scan(&stats.TotalDeliveries)
	if err != nil {
		return nil, fmt.Errorf("failed to count total deliveries: %w", err)
	}

	// Successful deliveries
	query = `
		SELECT COUNT(*) FROM webhook_deliveries
		WHERE webhook_id = ? AND created_at >= ? AND success = true`
	err = d.db.QueryRowContext(ctx, query, webhookID, since).Scan(&stats.SuccessfulDeliveries)
	if err != nil {
		return nil, fmt.Errorf("failed to count successful deliveries: %w", err)
	}

	// Failed deliveries
	stats.FailedDeliveries = stats.TotalDeliveries - stats.SuccessfulDeliveries

	// Success rate
	if stats.TotalDeliveries > 0 {
		stats.SuccessRate = float64(stats.SuccessfulDeliveries) / float64(stats.TotalDeliveries) * 100
	}

	// Average response time (for successful deliveries)
	query = `
		SELECT AVG(CAST((JulianDay(delivered_at) - JulianDay(created_at)) * 86400000 AS INTEGER))
		FROM webhook_deliveries
		WHERE webhook_id = ? AND created_at >= ? AND success = true AND delivered_at IS NOT NULL`
	var avgResponseTime *float64
	err = d.db.QueryRowContext(ctx, query, webhookID, since).Scan(&avgResponseTime)
	if err == nil && avgResponseTime != nil {
		stats.AverageResponseTime = *avgResponseTime
	}

	return stats, nil
}

// DeliveryStats represents delivery statistics
type DeliveryStats struct {
	TotalDeliveries      int64   `json:"total_deliveries"`
	SuccessfulDeliveries int64   `json:"successful_deliveries"`
	FailedDeliveries     int64   `json:"failed_deliveries"`
	SuccessRate          float64 `json:"success_rate"`
	AverageResponseTime  float64 `json:"average_response_time_ms"`
}