package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// WebhookHandler handles payment provider webhooks
type WebhookHandler struct {
	db     *db.DB
	config *config.BillingConfig
	logger *logrus.Logger
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(database *db.DB, cfg *config.BillingConfig, logger *logrus.Logger) (*WebhookHandler, error) {
	return &WebhookHandler{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// WebhookEvent represents a webhook event
type WebhookEvent struct {
	ID        string    `json:"id" db:"id"`
	Provider  string    `json:"provider" db:"provider"`
	EventType string    `json:"event_type" db:"event_type"`
	Data      string    `json:"data" db:"data"`
	Processed bool      `json:"processed" db:"processed"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// HandleWebhook handles a webhook from a payment provider
func (wh *WebhookHandler) HandleWebhook(ctx context.Context, provider string, payload []byte, signature string) error {
	// Verify webhook signature
	if err := wh.verifySignature(provider, payload, signature); err != nil {
		return fmt.Errorf("webhook signature verification failed: %w", err)
	}

	// Parse webhook payload
	event, err := wh.parseWebhookPayload(provider, payload)
	if err != nil {
		return fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Store webhook event
	webhookEvent := &WebhookEvent{
		ID:        wh.generateEventID(),
		Provider:  provider,
		EventType: event.Type,
		Data:      string(payload),
		Processed: false,
		CreatedAt: time.Now(),
	}

	if err := wh.storeWebhookEvent(ctx, webhookEvent); err != nil {
		wh.logger.WithError(err).Error("Failed to store webhook event")
		// Continue processing even if storage fails
	}

	// Process webhook event
	if err := wh.processWebhookEvent(ctx, provider, event); err != nil {
		wh.logger.WithError(err).Error("Failed to process webhook event")
		return fmt.Errorf("failed to process webhook event: %w", err)
	}

	// Mark event as processed
	if err := wh.markEventProcessed(ctx, webhookEvent.ID); err != nil {
		wh.logger.WithError(err).Warn("Failed to mark webhook event as processed")
	}

	wh.logger.WithFields(logrus.Fields{
		"provider":   provider,
		"event_type": event.Type,
		"event_id":   webhookEvent.ID,
	}).Info("Webhook processed successfully")

	return nil
}

// WebhookEventData represents parsed webhook event data
type WebhookEventData struct {
	Type   string                 `json:"type"`
	Data   map[string]interface{} `json:"data"`
	Object map[string]interface{} `json:"object"`
}

func (wh *WebhookHandler) parseWebhookPayload(provider string, payload []byte) (*WebhookEventData, error) {
	switch provider {
	case "stripe":
		return wh.parseStripeWebhook(payload)
	case "paypal":
		return wh.parsePayPalWebhook(payload)
	default:
		return nil, fmt.Errorf("unsupported webhook provider: %s", provider)
	}
}

func (wh *WebhookHandler) processWebhookEvent(ctx context.Context, provider string, event *WebhookEventData) error {
	switch provider {
	case "stripe":
		return wh.processStripeEvent(ctx, event)
	case "paypal":
		return wh.processPayPalEvent(ctx, event)
	default:
		return fmt.Errorf("unsupported webhook provider: %s", provider)
	}
}

// Stripe webhook processing
func (wh *WebhookHandler) parseStripeWebhook(payload []byte) (*WebhookEventData, error) {
	var event WebhookEventData
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to parse Stripe webhook: %w", err)
	}
	return &event, nil
}

func (wh *WebhookHandler) processStripeEvent(ctx context.Context, event *WebhookEventData) error {
	switch event.Type {
	case "invoice.payment_succeeded":
		return wh.handleInvoicePaymentSucceeded(ctx, event)
	case "invoice.payment_failed":
		return wh.handleInvoicePaymentFailed(ctx, event)
	case "customer.subscription.created":
		return wh.handleSubscriptionCreated(ctx, event)
	case "customer.subscription.updated":
		return wh.handleSubscriptionUpdated(ctx, event)
	case "customer.subscription.deleted":
		return wh.handleSubscriptionDeleted(ctx, event)
	case "payment_method.attached":
		return wh.handlePaymentMethodAttached(ctx, event)
	default:
		wh.logger.WithField("event_type", event.Type).Debug("Unhandled Stripe webhook event")
		return nil
	}
}

// PayPal webhook processing
func (wh *WebhookHandler) parsePayPalWebhook(payload []byte) (*WebhookEventData, error) {
	var event WebhookEventData
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to parse PayPal webhook: %w", err)
	}
	return &event, nil
}

func (wh *WebhookHandler) processPayPalEvent(ctx context.Context, event *WebhookEventData) error {
	switch event.Type {
	case "PAYMENT.SALE.COMPLETED":
		return wh.handlePayPalPaymentCompleted(ctx, event)
	case "BILLING.SUBSCRIPTION.CREATED":
		return wh.handlePayPalSubscriptionCreated(ctx, event)
	case "BILLING.SUBSCRIPTION.CANCELLED":
		return wh.handlePayPalSubscriptionCancelled(ctx, event)
	default:
		wh.logger.WithField("event_type", event.Type).Debug("Unhandled PayPal webhook event")
		return nil
	}
}

// Event handlers
func (wh *WebhookHandler) handleInvoicePaymentSucceeded(ctx context.Context, event *WebhookEventData) error {
	// Extract invoice ID from event data
	invoiceID, ok := event.Data["id"].(string)
	if !ok {
		return fmt.Errorf("missing invoice ID in webhook data")
	}

	// Find invoice by provider ID and mark as paid
	query := `
		UPDATE invoices SET status = 'paid', paid_at = ?, updated_at = ?
		WHERE provider_invoice_id = ?`

	now := time.Now()
	_, err := wh.db.ExecContext(ctx, query, now, now, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to mark invoice as paid: %w", err)
	}

	wh.logger.WithField("invoice_id", invoiceID).Info("Invoice payment succeeded")
	return nil
}

func (wh *WebhookHandler) handleInvoicePaymentFailed(ctx context.Context, event *WebhookEventData) error {
	// Extract invoice ID from event data
	invoiceID, ok := event.Data["id"].(string)
	if !ok {
		return fmt.Errorf("missing invoice ID in webhook data")
	}

	// Mark invoice as overdue
	query := `
		UPDATE invoices SET status = 'overdue', updated_at = ?
		WHERE provider_invoice_id = ?`

	_, err := wh.db.ExecContext(ctx, query, time.Now(), invoiceID)
	if err != nil {
		return fmt.Errorf("failed to mark invoice as overdue: %w", err)
	}

	wh.logger.WithField("invoice_id", invoiceID).Info("Invoice payment failed")
	return nil
}

func (wh *WebhookHandler) handleSubscriptionCreated(ctx context.Context, event *WebhookEventData) error {
	// Handle subscription creation event
	subscriptionID, ok := event.Data["id"].(string)
	if !ok {
		return fmt.Errorf("missing subscription ID in webhook data")
	}

	wh.logger.WithField("subscription_id", subscriptionID).Info("Subscription created webhook received")
	return nil
}

func (wh *WebhookHandler) handleSubscriptionUpdated(ctx context.Context, event *WebhookEventData) error {
	// Handle subscription update event
	subscriptionID, ok := event.Data["id"].(string)
	if !ok {
		return fmt.Errorf("missing subscription ID in webhook data")
	}

	// Update subscription status if needed
	status, ok := event.Data["status"].(string)
	if ok {
		query := `
			UPDATE subscriptions SET status = ?, updated_at = ?
			WHERE provider_subscription_id = ?`

		_, err := wh.db.ExecContext(ctx, query, status, time.Now(), subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to update subscription status: %w", err)
		}
	}

	wh.logger.WithField("subscription_id", subscriptionID).Info("Subscription updated")
	return nil
}

func (wh *WebhookHandler) handleSubscriptionDeleted(ctx context.Context, event *WebhookEventData) error {
	// Handle subscription deletion event
	subscriptionID, ok := event.Data["id"].(string)
	if !ok {
		return fmt.Errorf("missing subscription ID in webhook data")
	}

	// Mark subscription as canceled
	query := `
		UPDATE subscriptions SET status = 'canceled', canceled_at = ?, updated_at = ?
		WHERE provider_subscription_id = ?`

	now := time.Now()
	_, err := wh.db.ExecContext(ctx, query, now, now, subscriptionID)
	if err != nil {
		return fmt.Errorf("failed to cancel subscription: %w", err)
	}

	wh.logger.WithField("subscription_id", subscriptionID).Info("Subscription deleted")
	return nil
}

func (wh *WebhookHandler) handlePaymentMethodAttached(ctx context.Context, event *WebhookEventData) error {
	// Handle payment method attachment
	wh.logger.Info("Payment method attached")
	return nil
}

func (wh *WebhookHandler) handlePayPalPaymentCompleted(ctx context.Context, event *WebhookEventData) error {
	// Handle PayPal payment completion
	wh.logger.Info("PayPal payment completed")
	return nil
}

func (wh *WebhookHandler) handlePayPalSubscriptionCreated(ctx context.Context, event *WebhookEventData) error {
	// Handle PayPal subscription creation
	wh.logger.Info("PayPal subscription created")
	return nil
}

func (wh *WebhookHandler) handlePayPalSubscriptionCancelled(ctx context.Context, event *WebhookEventData) error {
	// Handle PayPal subscription cancellation
	wh.logger.Info("PayPal subscription cancelled")
	return nil
}

// Helper methods
func (wh *WebhookHandler) verifySignature(provider string, payload []byte, signature string) error {
	switch provider {
	case "stripe":
		return wh.verifyStripeSignature(payload, signature)
	case "paypal":
		return wh.verifyPayPalSignature(payload, signature)
	default:
		return fmt.Errorf("unsupported provider for signature verification: %s", provider)
	}
}

func (wh *WebhookHandler) verifyStripeSignature(payload []byte, signature string) error {
	// Implementation would verify Stripe webhook signature
	// For now, just log and return success
	wh.logger.Debug("Verifying Stripe webhook signature")
	return nil
}

func (wh *WebhookHandler) verifyPayPalSignature(payload []byte, signature string) error {
	// Implementation would verify PayPal webhook signature
	// For now, just log and return success
	wh.logger.Debug("Verifying PayPal webhook signature")
	return nil
}

func (wh *WebhookHandler) storeWebhookEvent(ctx context.Context, event *WebhookEvent) error {
	query := `
		INSERT INTO webhook_events (id, provider, event_type, data, processed, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err := wh.db.ExecContext(ctx, query,
		event.ID, event.Provider, event.EventType,
		event.Data, event.Processed, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to store webhook event: %w", err)
	}

	return nil
}

func (wh *WebhookHandler) markEventProcessed(ctx context.Context, eventID string) error {
	query := `UPDATE webhook_events SET processed = true WHERE id = ?`

	_, err := wh.db.ExecContext(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark event as processed: %w", err)
	}

	return nil
}

func (wh *WebhookHandler) generateEventID() string {
	return fmt.Sprintf("evt_%d", time.Now().UnixNano())
}

// GetUnprocessedEvents gets unprocessed webhook events for retry
func (wh *WebhookHandler) GetUnprocessedEvents(ctx context.Context, limit int) ([]*WebhookEvent, error) {
	query := `
		SELECT id, provider, event_type, data, processed, created_at
		FROM webhook_events
		WHERE processed = false
		ORDER BY created_at ASC
		LIMIT ?`

	rows, err := wh.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query unprocessed events: %w", err)
	}
	defer rows.Close()

	var events []*WebhookEvent
	for rows.Next() {
		event := &WebhookEvent{}
		err := rows.Scan(
			&event.ID, &event.Provider, &event.EventType,
			&event.Data, &event.Processed, &event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan webhook event: %w", err)
		}
		events = append(events, event)
	}

	return events, nil
}

// RetryUnprocessedEvents retries processing of unprocessed webhook events
func (wh *WebhookHandler) RetryUnprocessedEvents(ctx context.Context) error {
	events, err := wh.GetUnprocessedEvents(ctx, 100)
	if err != nil {
		return fmt.Errorf("failed to get unprocessed events: %w", err)
	}

	for _, event := range events {
		// Parse event data
		eventData, err := wh.parseWebhookPayload(event.Provider, []byte(event.Data))
		if err != nil {
			wh.logger.WithError(err).WithField("event_id", event.ID).Error("Failed to parse webhook event for retry")
			continue
		}

		// Process event
		if err := wh.processWebhookEvent(ctx, event.Provider, eventData); err != nil {
			wh.logger.WithError(err).WithField("event_id", event.ID).Error("Failed to retry webhook event")
			continue
		}

		// Mark as processed
		if err := wh.markEventProcessed(ctx, event.ID); err != nil {
			wh.logger.WithError(err).WithField("event_id", event.ID).Warn("Failed to mark retried event as processed")
		}

		wh.logger.WithField("event_id", event.ID).Info("Webhook event retried successfully")
	}

	return nil
}