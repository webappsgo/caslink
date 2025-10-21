package providers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// ManualProvider implements manual/enterprise billing without external payment processor
// This provider is designed for enterprise customers who handle payments through
// traditional invoicing, wire transfers, checks, or custom payment arrangements.
type ManualProvider struct {
	logger *logrus.Logger
}

// NewManualProvider creates a new manual payment provider
func NewManualProvider(logger *logrus.Logger) (*ManualProvider, error) {
	provider := &ManualProvider{
		logger: logger,
	}

	logger.Info("Manual billing provider initialized")

	return provider, nil
}

// CreateCustomer creates a customer reference for manual billing
// Since there's no external provider, we just generate a local customer ID
func (mp *ManualProvider) CreateCustomer(ctx context.Context, userID, email, name string) (string, error) {
	// Generate a unique customer ID
	customerID := fmt.Sprintf("manual_customer_%s_%s", userID, mp.generateID(8))

	mp.logger.WithFields(logrus.Fields{
		"customer_id": customerID,
		"user_id":     userID,
		"email":       email,
		"name":        name,
	}).Info("Manual billing customer created")

	return customerID, nil
}

// CreateSubscription creates a manual subscription record
// For manual billing, subscriptions are managed internally without external provider
func (mp *ManualProvider) CreateSubscription(ctx context.Context, req *SubscriptionRequest) (*SubscriptionResponse, error) {
	now := time.Now()

	// Calculate period dates
	var periodEnd time.Time
	if req.PlanID == "yearly" || req.PlanID == "annual" {
		periodEnd = now.AddDate(1, 0, 0)
	} else {
		periodEnd = now.AddDate(0, 1, 0) // Default to monthly
	}

	// Generate subscription ID
	subscriptionID := fmt.Sprintf("manual_sub_%s", mp.generateID(16))

	response := &SubscriptionResponse{
		SubscriptionID:     subscriptionID,
		Status:             "active", // Manual subscriptions start active
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		CancelAtPeriodEnd:  false,
	}

	// Handle trial period if specified
	if req.TrialDays > 0 {
		trialEnd := now.AddDate(0, 0, req.TrialDays)
		response.TrialEnd = &trialEnd
		response.Status = "trialing"
	}

	mp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"user_id":        req.UserID,
		"plan_id":        req.PlanID,
		"status":         response.Status,
		"period_start":   response.CurrentPeriodStart,
		"period_end":     response.CurrentPeriodEnd,
	}).Info("Manual subscription created")

	return response, nil
}

// UpdateSubscription updates a manual subscription
func (mp *ManualProvider) UpdateSubscription(ctx context.Context, subscriptionID, newPriceID string) (*SubscriptionResponse, error) {
	now := time.Now()

	// For manual subscriptions, we just return updated dates
	var periodEnd time.Time
	if newPriceID == "yearly" || newPriceID == "annual" {
		periodEnd = now.AddDate(1, 0, 0)
	} else {
		periodEnd = now.AddDate(0, 1, 0)
	}

	response := &SubscriptionResponse{
		SubscriptionID:     subscriptionID,
		Status:             "active",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		CancelAtPeriodEnd:  false,
	}

	mp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"new_price_id":    newPriceID,
	}).Info("Manual subscription updated")

	return response, nil
}

// CancelSubscription cancels a manual subscription
func (mp *ManualProvider) CancelSubscription(ctx context.Context, subscriptionID string, immediate bool) error {
	mp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"immediate":       immediate,
	}).Info("Manual subscription cancelled")

	return nil
}

// ReactivateSubscription reactivates a cancelled manual subscription
func (mp *ManualProvider) ReactivateSubscription(ctx context.Context, subscriptionID string) error {
	mp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
	}).Info("Manual subscription reactivated")

	return nil
}

// CreatePaymentIntent creates a manual payment record
// For manual billing, this generates an invoice/payment reference
func (mp *ManualProvider) CreatePaymentIntent(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	// Generate payment/invoice ID
	paymentID := fmt.Sprintf("manual_payment_%s", mp.generateID(16))
	invoiceNumber := fmt.Sprintf("INV-%s", mp.generateID(12))

	response := &PaymentResponse{
		PaymentID:    paymentID,
		Status:       "pending", // Manual payments start as pending
		ClientSecret: invoiceNumber,
		Amount:       req.Amount,
		Currency:     req.Currency,
	}

	mp.logger.WithFields(logrus.Fields{
		"payment_id":     paymentID,
		"invoice_number": invoiceNumber,
		"amount":         req.Amount,
		"currency":       req.Currency,
		"user_id":        req.UserID,
		"description":    req.Description,
	}).Info("Manual payment intent created")

	return response, nil
}

// ConfirmPayment confirms a manual payment
// This would be called when payment is received (e.g., wire transfer confirmed)
func (mp *ManualProvider) ConfirmPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	response := &PaymentResponse{
		PaymentID: paymentID,
		Status:    "succeeded",
	}

	mp.logger.WithFields(logrus.Fields{
		"payment_id": paymentID,
		"status":     "succeeded",
	}).Info("Manual payment confirmed")

	return response, nil
}

// RefundPayment processes a manual refund
func (mp *ManualProvider) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResponse, error) {
	refundID := fmt.Sprintf("manual_refund_%s", mp.generateID(16))

	response := &RefundResponse{
		RefundID: refundID,
		Status:   "succeeded",
		Amount:   amount,
		Reason:   "manual_refund",
	}

	mp.logger.WithFields(logrus.Fields{
		"refund_id":  refundID,
		"payment_id": paymentID,
		"amount":     amount,
	}).Info("Manual refund processed")

	return response, nil
}

// GetSubscription retrieves a manual subscription
// For manual provider, this would typically query the database
func (mp *ManualProvider) GetSubscription(ctx context.Context, subscriptionID string) (*SubscriptionResponse, error) {
	// In a real implementation, this would query the database
	// For now, we return a basic response
	now := time.Now()

	response := &SubscriptionResponse{
		SubscriptionID:     subscriptionID,
		Status:             "active",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
		CancelAtPeriodEnd:  false,
	}

	mp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
	}).Debug("Manual subscription retrieved")

	return response, nil
}

// VerifyWebhook - Not applicable for manual billing
func (mp *ManualProvider) VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error) {
	return nil, fmt.Errorf("webhooks not supported for manual billing")
}

// AttachPaymentMethod - Not applicable for manual billing
func (mp *ManualProvider) AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	mp.logger.WithFields(logrus.Fields{
		"customer_id":       customerID,
		"payment_method_id": paymentMethodID,
	}).Info("Manual payment method reference attached")

	return nil
}

// DetachPaymentMethod - Not applicable for manual billing
func (mp *ManualProvider) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	mp.logger.WithFields(logrus.Fields{
		"payment_method_id": paymentMethodID,
	}).Info("Manual payment method reference detached")

	return nil
}

// SetDefaultPaymentMethod - Not applicable for manual billing
func (mp *ManualProvider) SetDefaultPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	mp.logger.WithFields(logrus.Fields{
		"customer_id":       customerID,
		"payment_method_id": paymentMethodID,
	}).Info("Manual default payment method set")

	return nil
}

// GetPaymentStatus retrieves manual payment status
func (mp *ManualProvider) GetPaymentStatus(ctx context.Context, paymentID string) (string, error) {
	// In a real implementation, this would query the database
	// For manual billing, payments would be marked manually in the system
	mp.logger.WithFields(logrus.Fields{
		"payment_id": paymentID,
	}).Debug("Manual payment status retrieved")

	return "pending", nil
}

// RetryFailedPayment - Not applicable for manual billing
func (mp *ManualProvider) RetryFailedPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	return nil, fmt.Errorf("automatic payment retry not supported for manual billing")
}

// IsTestMode returns whether the provider is in test mode
// Manual billing doesn't have test mode
func (mp *ManualProvider) IsTestMode() bool {
	return false
}

// GetProviderName returns the name of the payment provider
func (mp *ManualProvider) GetProviderName() string {
	return "manual"
}

// MarkPaymentPaid manually marks a payment as paid
// This is a special method for manual billing when payment is confirmed
func (mp *ManualProvider) MarkPaymentPaid(ctx context.Context, paymentID, referenceNumber string) error {
	mp.logger.WithFields(logrus.Fields{
		"payment_id":       paymentID,
		"reference_number": referenceNumber,
	}).Info("Manual payment marked as paid")

	return nil
}

// MarkPaymentFailed manually marks a payment as failed
func (mp *ManualProvider) MarkPaymentFailed(ctx context.Context, paymentID, reason string) error {
	mp.logger.WithFields(logrus.Fields{
		"payment_id": paymentID,
		"reason":     reason,
	}).Info("Manual payment marked as failed")

	return nil
}

// ExtendSubscription extends a manual subscription period
// This is useful for enterprise customers who pay annually or have custom terms
func (mp *ManualProvider) ExtendSubscription(ctx context.Context, subscriptionID string, months int) (*SubscriptionResponse, error) {
	now := time.Now()
	periodEnd := now.AddDate(0, months, 0)

	response := &SubscriptionResponse{
		SubscriptionID:     subscriptionID,
		Status:             "active",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		CancelAtPeriodEnd:  false,
	}

	mp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"months":          months,
		"new_period_end":  periodEnd,
	}).Info("Manual subscription extended")

	return response, nil
}

// SetSubscriptionStatus sets a manual subscription status
// This allows administrators to manually control subscription states
func (mp *ManualProvider) SetSubscriptionStatus(ctx context.Context, subscriptionID, status string) error {
	mp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"status":          status,
	}).Info("Manual subscription status updated")

	return nil
}

// GenerateInvoice generates an invoice for manual billing
func (mp *ManualProvider) GenerateInvoice(ctx context.Context, subscriptionID string, amount int64, currency string) (string, error) {
	invoiceID := fmt.Sprintf("INV-%s-%s", time.Now().Format("20060102"), mp.generateID(8))

	mp.logger.WithFields(logrus.Fields{
		"invoice_id":      invoiceID,
		"subscription_id": subscriptionID,
		"amount":          amount,
		"currency":        currency,
	}).Info("Manual invoice generated")

	return invoiceID, nil
}

// RecordPayment records a manual payment receipt
func (mp *ManualProvider) RecordPayment(ctx context.Context, invoiceID string, amount int64, currency, paymentMethod, referenceNumber string) (string, error) {
	paymentID := fmt.Sprintf("manual_payment_%s", mp.generateID(16))

	mp.logger.WithFields(logrus.Fields{
		"payment_id":       paymentID,
		"invoice_id":       invoiceID,
		"amount":           amount,
		"currency":         currency,
		"payment_method":   paymentMethod,
		"reference_number": referenceNumber,
	}).Info("Manual payment recorded")

	return paymentID, nil
}

// Helper method to generate random IDs
func (mp *ManualProvider) generateID(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
