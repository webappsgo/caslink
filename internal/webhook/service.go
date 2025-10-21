package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Service manages webhook functionality
type Service struct {
	config     *config.WebhooksConfig
	db         *db.DB
	logger     *logrus.Logger
	dispatcher *Dispatcher
	queue      *Queue
	validator  *Validator
	client     *http.Client
}

// Event represents a webhook event
type Event struct {
	ID        string                 `json:"id" db:"id"`
	Type      string                 `json:"type" db:"type"`
	Timestamp time.Time              `json:"timestamp" db:"timestamp"`
	Data      map[string]interface{} `json:"data" db:"data"`
	UserID    string                 `json:"user_id,omitempty" db:"user_id"`
	Source    string                 `json:"source" db:"source"`
}

// Webhook represents a webhook endpoint
type Webhook struct {
	ID          string    `json:"id" db:"id"`
	UserID      string    `json:"user_id" db:"user_id"`
	URL         string    `json:"url" db:"url"`
	Secret      string    `json:"secret,omitempty" db:"secret"`
	Events      []string  `json:"events" db:"events"`
	Active      bool      `json:"active" db:"active"`
	LastPing    *time.Time `json:"last_ping,omitempty" db:"last_ping"`
	LastSuccess *time.Time `json:"last_success,omitempty" db:"last_success"`
	FailCount   int       `json:"fail_count" db:"fail_count"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	Headers     map[string]string `json:"headers,omitempty" db:"headers"`
}

// Delivery represents a webhook delivery attempt
type Delivery struct {
	ID           string    `json:"id" db:"id"`
	WebhookID    string    `json:"webhook_id" db:"webhook_id"`
	EventID      string    `json:"event_id" db:"event_id"`
	URL          string    `json:"url" db:"url"`
	Method       string    `json:"method" db:"method"`
	StatusCode   int       `json:"status_code" db:"status_code"`
	Response     string    `json:"response,omitempty" db:"response"`
	Error        string    `json:"error,omitempty" db:"error"`
	Attempt      int       `json:"attempt" db:"attempt"`
	NextAttempt  *time.Time `json:"next_attempt,omitempty" db:"next_attempt"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty" db:"delivered_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	Success      bool      `json:"success" db:"success"`
}

// EventType constants
const (
	EventTypeURLCreated    = "url.created"
	EventTypeURLUpdated    = "url.updated"
	EventTypeURLDeleted    = "url.deleted"
	EventTypeURLClicked    = "url.clicked"
	EventTypeUserCreated   = "user.created"
	EventTypeUserUpdated   = "user.updated"
	EventTypeUserDeleted   = "user.deleted"
	EventTypeBulkImported  = "bulk.imported"
	EventTypeBulkExported  = "bulk.exported"
	EventTypeQRGenerated   = "qr.generated"
)

// NewService creates a new webhook service
func NewService(cfg *config.WebhooksConfig, database *db.DB, logger *logrus.Logger) (*Service, error) {
	if !cfg.Enabled {
		logger.Info("Webhooks are disabled")
		return &Service{
			config: cfg,
			db:     database,
			logger: logger,
		}, nil
	}

	service := &Service{
		config: cfg,
		db:     database,
		logger: logger,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}

	// Initialize components
	var err error

	service.validator, err = NewValidator(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create validator: %w", err)
	}

	service.queue, err = NewQueue(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create queue: %w", err)
	}

	service.dispatcher, err = NewDispatcher(database, cfg, logger, service.client, service.validator)
	if err != nil {
		return nil, fmt.Errorf("failed to create dispatcher: %w", err)
	}

	logger.Info("Webhook service initialized successfully")
	return service, nil
}

// Start starts the webhook service
func (s *Service) Start(ctx context.Context) error {
	if !s.config.Enabled {
		return nil // Webhooks disabled
	}

	s.logger.Info("Starting webhook service")

	// Start the dispatcher worker
	go s.dispatcher.Start(ctx)

	// Start the queue processor
	go s.queue.Start(ctx)

	s.logger.Info("Webhook service started")
	return nil
}

// Stop stops the webhook service
func (s *Service) Stop() error {
	if !s.config.Enabled {
		return nil // Webhooks disabled
	}

	s.logger.Info("Stopping webhook service")
	// Cleanup will be handled by context cancellation
	return nil
}

// CreateWebhook creates a new webhook endpoint
func (s *Service) CreateWebhook(ctx context.Context, webhook *Webhook) error {
	if !s.config.Enabled {
		return fmt.Errorf("webhooks are disabled")
	}

	// Validate webhook
	if err := s.validator.ValidateWebhook(webhook); err != nil {
		return fmt.Errorf("webhook validation failed: %w", err)
	}

	// Generate ID if not provided
	if webhook.ID == "" {
		webhook.ID = s.generateWebhookID()
	}

	// Set timestamps
	now := time.Now()
	webhook.CreatedAt = now
	webhook.UpdatedAt = now

	// Convert events slice to JSON for storage
	eventsJSON, err := json.Marshal(webhook.Events)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	// Convert headers to JSON for storage
	headersJSON, err := json.Marshal(webhook.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	// Insert into database
	query := `
		INSERT INTO webhooks (id, user_id, url, secret, events, active, created_at, updated_at, headers)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		webhook.ID, webhook.UserID, webhook.URL, webhook.Secret,
		string(eventsJSON), webhook.Active, webhook.CreatedAt, webhook.UpdatedAt,
		string(headersJSON),
	)

	if err != nil {
		return fmt.Errorf("failed to create webhook: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"webhook_id": webhook.ID,
		"user_id":    webhook.UserID,
		"url":        webhook.URL,
	}).Info("Webhook created")

	return nil
}

// GetWebhook retrieves a webhook by ID
func (s *Service) GetWebhook(ctx context.Context, id string) (*Webhook, error) {
	query := `
		SELECT id, user_id, url, secret, events, active, last_ping, last_success,
		       fail_count, created_at, updated_at, headers
		FROM webhooks
		WHERE id = ?`

	row := s.db.QueryRowContext(ctx, query, id)

	webhook := &Webhook{}
	var eventsJSON, headersJSON string
	var lastPing, lastSuccess *time.Time

	err := row.Scan(
		&webhook.ID, &webhook.UserID, &webhook.URL, &webhook.Secret,
		&eventsJSON, &webhook.Active, &lastPing, &lastSuccess,
		&webhook.FailCount, &webhook.CreatedAt, &webhook.UpdatedAt, &headersJSON,
	)

	if err != nil {
		return nil, fmt.Errorf("webhook not found: %w", err)
	}

	// Parse events JSON
	if err := json.Unmarshal([]byte(eventsJSON), &webhook.Events); err != nil {
		return nil, fmt.Errorf("failed to parse events: %w", err)
	}

	// Parse headers JSON
	if headersJSON != "" {
		if err := json.Unmarshal([]byte(headersJSON), &webhook.Headers); err != nil {
			s.logger.WithError(err).Warn("Failed to parse webhook headers")
		}
	}

	webhook.LastPing = lastPing
	webhook.LastSuccess = lastSuccess

	return webhook, nil
}

// GetWebhooksByUser retrieves all webhooks for a user
func (s *Service) GetWebhooksByUser(ctx context.Context, userID string) ([]*Webhook, error) {
	query := `
		SELECT id, user_id, url, secret, events, active, last_ping, last_success,
		       fail_count, created_at, updated_at, headers
		FROM webhooks
		WHERE user_id = ?
		ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query webhooks: %w", err)
	}
	defer rows.Close()

	var webhooks []*Webhook

	for rows.Next() {
		webhook := &Webhook{}
		var eventsJSON, headersJSON string
		var lastPing, lastSuccess *time.Time

		err := rows.Scan(
			&webhook.ID, &webhook.UserID, &webhook.URL, &webhook.Secret,
			&eventsJSON, &webhook.Active, &lastPing, &lastSuccess,
			&webhook.FailCount, &webhook.CreatedAt, &webhook.UpdatedAt, &headersJSON,
		)

		if err != nil {
			s.logger.WithError(err).Warn("Failed to scan webhook")
			continue
		}

		// Parse events JSON
		if err := json.Unmarshal([]byte(eventsJSON), &webhook.Events); err != nil {
			s.logger.WithError(err).Warn("Failed to parse webhook events")
			continue
		}

		// Parse headers JSON
		if headersJSON != "" {
			if err := json.Unmarshal([]byte(headersJSON), &webhook.Headers); err != nil {
				s.logger.WithError(err).Warn("Failed to parse webhook headers")
			}
		}

		webhook.LastPing = lastPing
		webhook.LastSuccess = lastSuccess

		webhooks = append(webhooks, webhook)
	}

	return webhooks, nil
}

// UpdateWebhook updates an existing webhook
func (s *Service) UpdateWebhook(ctx context.Context, webhook *Webhook) error {
	if !s.config.Enabled {
		return fmt.Errorf("webhooks are disabled")
	}

	// Validate webhook
	if err := s.validator.ValidateWebhook(webhook); err != nil {
		return fmt.Errorf("webhook validation failed: %w", err)
	}

	// Update timestamp
	webhook.UpdatedAt = time.Now()

	// Convert events slice to JSON for storage
	eventsJSON, err := json.Marshal(webhook.Events)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	// Convert headers to JSON for storage
	headersJSON, err := json.Marshal(webhook.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	// Update in database
	query := `
		UPDATE webhooks
		SET url = ?, secret = ?, events = ?, active = ?, updated_at = ?, headers = ?
		WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query,
		webhook.URL, webhook.Secret, string(eventsJSON), webhook.Active,
		webhook.UpdatedAt, string(headersJSON), webhook.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update webhook: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("webhook not found")
	}

	s.logger.WithFields(logrus.Fields{
		"webhook_id": webhook.ID,
		"url":        webhook.URL,
	}).Info("Webhook updated")

	return nil
}

// DeleteWebhook deletes a webhook
func (s *Service) DeleteWebhook(ctx context.Context, id string) error {
	if !s.config.Enabled {
		return fmt.Errorf("webhooks are disabled")
	}

	// Delete from database
	query := "DELETE FROM webhooks WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("webhook not found")
	}

	s.logger.WithField("webhook_id", id).Info("Webhook deleted")
	return nil
}

// TriggerEvent triggers a webhook event
func (s *Service) TriggerEvent(ctx context.Context, event *Event) error {
	if !s.config.Enabled {
		return nil // Webhooks disabled
	}

	// Generate event ID if not provided
	if event.ID == "" {
		event.ID = s.generateEventID()
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Store event in database
	if err := s.storeEvent(ctx, event); err != nil {
		s.logger.WithError(err).Error("Failed to store event")
		return err
	}

	// Find webhooks that should receive this event
	webhooks, err := s.getWebhooksForEvent(ctx, event)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get webhooks for event")
		return err
	}

	// Queue deliveries for each webhook
	for _, webhook := range webhooks {
		delivery := &Delivery{
			ID:        s.generateDeliveryID(),
			WebhookID: webhook.ID,
			EventID:   event.ID,
			URL:       webhook.URL,
			Method:    "POST",
			Attempt:   1,
			CreatedAt: time.Now(),
		}

		if err := s.queue.Enqueue(ctx, delivery, event, webhook); err != nil {
			s.logger.WithError(err).WithFields(logrus.Fields{
				"webhook_id": webhook.ID,
				"event_id":   event.ID,
			}).Error("Failed to queue delivery")
		}
	}

	s.logger.WithFields(logrus.Fields{
		"event_id":     event.ID,
		"event_type":   event.Type,
		"webhook_count": len(webhooks),
	}).Info("Event triggered")

	return nil
}

// TestWebhook tests a webhook by sending a ping event
func (s *Service) TestWebhook(ctx context.Context, webhookID string) (*Delivery, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("webhooks are disabled")
	}

	// Get webhook
	webhook, err := s.GetWebhook(ctx, webhookID)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook: %w", err)
	}

	// Create test event
	event := &Event{
		ID:        s.generateEventID(),
		Type:      "ping",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"message": "This is a test webhook delivery",
			"ping":    true,
		},
		Source: "webhook_test",
	}

	// Create delivery
	delivery := &Delivery{
		ID:        s.generateDeliveryID(),
		WebhookID: webhook.ID,
		EventID:   event.ID,
		URL:       webhook.URL,
		Method:    "POST",
		Attempt:   1,
		CreatedAt: time.Now(),
	}

	// Dispatch immediately
	success, err := s.dispatcher.Dispatch(ctx, delivery, event, webhook)
	if err != nil {
		return delivery, fmt.Errorf("failed to dispatch test webhook: %w", err)
	}

	delivery.Success = success
	if success {
		now := time.Now()
		delivery.DeliveredAt = &now

		// Update webhook last ping time
		s.updateWebhookLastPing(ctx, webhook.ID)
	}

	s.logger.WithFields(logrus.Fields{
		"webhook_id": webhookID,
		"success":    success,
	}).Info("Webhook test completed")

	return delivery, nil
}

// GetDeliveries retrieves delivery history for a webhook
func (s *Service) GetDeliveries(ctx context.Context, webhookID string, limit int, offset int) ([]*Delivery, error) {
	query := `
		SELECT id, webhook_id, event_id, url, method, status_code, response, error,
		       attempt, next_attempt, delivered_at, created_at, success
		FROM webhook_deliveries
		WHERE webhook_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`

	rows, err := s.db.QueryContext(ctx, query, webhookID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []*Delivery

	for rows.Next() {
		delivery := &Delivery{}
		var nextAttempt, deliveredAt *time.Time

		err := rows.Scan(
			&delivery.ID, &delivery.WebhookID, &delivery.EventID, &delivery.URL,
			&delivery.Method, &delivery.StatusCode, &delivery.Response, &delivery.Error,
			&delivery.Attempt, &nextAttempt, &deliveredAt, &delivery.CreatedAt, &delivery.Success,
		)

		if err != nil {
			s.logger.WithError(err).Warn("Failed to scan delivery")
			continue
		}

		delivery.NextAttempt = nextAttempt
		delivery.DeliveredAt = deliveredAt

		deliveries = append(deliveries, delivery)
	}

	return deliveries, nil
}

// Helper methods

// storeEvent stores an event in the database
func (s *Service) storeEvent(ctx context.Context, event *Event) error {
	dataJSON, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	query := `
		INSERT INTO webhook_events (id, type, timestamp, data, user_id, source)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		event.ID, event.Type, event.Timestamp, string(dataJSON),
		event.UserID, event.Source,
	)

	return err
}

// getWebhooksForEvent finds webhooks that should receive the event
func (s *Service) getWebhooksForEvent(ctx context.Context, event *Event) ([]*Webhook, error) {
	query := `
		SELECT id, user_id, url, secret, events, active, headers
		FROM webhooks
		WHERE active = true`

	// Add user filter if event has user ID
	args := []interface{}{}
	if event.UserID != "" {
		query += " AND (user_id = ? OR user_id IS NULL)"
		args = append(args, event.UserID)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query webhooks: %w", err)
	}
	defer rows.Close()

	var webhooks []*Webhook

	for rows.Next() {
		webhook := &Webhook{}
		var eventsJSON, headersJSON string

		err := rows.Scan(
			&webhook.ID, &webhook.UserID, &webhook.URL, &webhook.Secret,
			&eventsJSON, &webhook.Active, &headersJSON,
		)

		if err != nil {
			s.logger.WithError(err).Warn("Failed to scan webhook")
			continue
		}

		// Parse events JSON
		if err := json.Unmarshal([]byte(eventsJSON), &webhook.Events); err != nil {
			s.logger.WithError(err).Warn("Failed to parse webhook events")
			continue
		}

		// Parse headers JSON
		if headersJSON != "" {
			if err := json.Unmarshal([]byte(headersJSON), &webhook.Headers); err != nil {
				s.logger.WithError(err).Warn("Failed to parse webhook headers")
			}
		}

		// Check if webhook listens for this event type
		if s.webhookListensForEvent(webhook, event.Type) {
			webhooks = append(webhooks, webhook)
		}
	}

	return webhooks, nil
}

// webhookListensForEvent checks if a webhook listens for a specific event type
func (s *Service) webhookListensForEvent(webhook *Webhook, eventType string) bool {
	for _, event := range webhook.Events {
		if event == eventType || event == "*" {
			return true
		}
	}
	return false
}

// updateWebhookLastPing updates the last ping time for a webhook
func (s *Service) updateWebhookLastPing(ctx context.Context, webhookID string) {
	query := "UPDATE webhooks SET last_ping = ? WHERE id = ?"
	_, err := s.db.ExecContext(ctx, query, time.Now(), webhookID)
	if err != nil {
		s.logger.WithError(err).WithField("webhook_id", webhookID).Warn("Failed to update webhook last ping")
	}
}

// generateWebhookID generates a unique ID for webhooks
func (s *Service) generateWebhookID() string {
	return fmt.Sprintf("webhook_%d", time.Now().UnixNano())
}

// generateEventID generates a unique ID for events
func (s *Service) generateEventID() string {
	return fmt.Sprintf("event_%d", time.Now().UnixNano())
}

// generateDeliveryID generates a unique ID for deliveries
func (s *Service) generateDeliveryID() string {
	return fmt.Sprintf("delivery_%d", time.Now().UnixNano())
}

// GetWebhookStats returns statistics about webhooks
func (s *Service) GetWebhookStats(ctx context.Context, userID string) (*WebhookStats, error) {
	stats := &WebhookStats{}

	// Count total webhooks
	query := "SELECT COUNT(*) FROM webhooks WHERE user_id = ?"
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&stats.TotalWebhooks)
	if err != nil {
		return nil, fmt.Errorf("failed to count webhooks: %w", err)
	}

	// Count active webhooks
	query = "SELECT COUNT(*) FROM webhooks WHERE user_id = ? AND active = true"
	err = s.db.QueryRowContext(ctx, query, userID).Scan(&stats.ActiveWebhooks)
	if err != nil {
		return nil, fmt.Errorf("failed to count active webhooks: %w", err)
	}

	// Count total deliveries
	query = `
		SELECT COUNT(*) FROM webhook_deliveries wd
		JOIN webhooks w ON wd.webhook_id = w.id
		WHERE w.user_id = ?`
	err = s.db.QueryRowContext(ctx, query, userID).Scan(&stats.TotalDeliveries)
	if err != nil {
		return nil, fmt.Errorf("failed to count deliveries: %w", err)
	}

	// Count successful deliveries
	query = `
		SELECT COUNT(*) FROM webhook_deliveries wd
		JOIN webhooks w ON wd.webhook_id = w.id
		WHERE w.user_id = ? AND wd.success = true`
	err = s.db.QueryRowContext(ctx, query, userID).Scan(&stats.SuccessfulDeliveries)
	if err != nil {
		return nil, fmt.Errorf("failed to count successful deliveries: %w", err)
	}

	// Calculate success rate
	if stats.TotalDeliveries > 0 {
		stats.SuccessRate = float64(stats.SuccessfulDeliveries) / float64(stats.TotalDeliveries) * 100
	}

	return stats, nil
}

// WebhookStats represents webhook statistics
type WebhookStats struct {
	TotalWebhooks        int64   `json:"total_webhooks"`
	ActiveWebhooks       int64   `json:"active_webhooks"`
	TotalDeliveries      int64   `json:"total_deliveries"`
	SuccessfulDeliveries int64   `json:"successful_deliveries"`
	SuccessRate          float64 `json:"success_rate"`
}