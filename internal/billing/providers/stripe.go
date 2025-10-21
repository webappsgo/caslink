package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/customer"
	"github.com/stripe/stripe-go/v76/paymentintent"
	"github.com/stripe/stripe-go/v76/paymentmethod"
	"github.com/stripe/stripe-go/v76/refund"
	"github.com/stripe/stripe-go/v76/subscription"
	"github.com/stripe/stripe-go/v76/webhook"
)

// StripeProvider implements payment processing with Stripe
type StripeProvider struct {
	secretKey     string
	webhookSecret string
	logger        *logrus.Logger
	testMode      bool
}

// NewStripeProvider creates a new Stripe payment provider
func NewStripeProvider(secretKey, webhookSecret string, logger *logrus.Logger) (*StripeProvider, error) {
	if secretKey == "" {
		return nil, fmt.Errorf("stripe secret key is required")
	}

	// Configure Stripe API key
	stripe.Key = secretKey

	// Detect test mode
	testMode := len(secretKey) >= 3 && secretKey[:3] == "sk_test"

	provider := &StripeProvider{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		logger:        logger,
		testMode:      testMode,
	}

	logger.WithFields(logrus.Fields{
		"test_mode": testMode,
	}).Info("Stripe provider initialized")

	return provider, nil
}

// CreateCustomer creates a new Stripe customer
func (sp *StripeProvider) CreateCustomer(ctx context.Context, userID, email, name string) (string, error) {
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
		Name:  stripe.String(name),
		Metadata: map[string]string{
			"user_id": userID,
		},
	}

	c, err := customer.New(params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to create Stripe customer")
		return "", fmt.Errorf("failed to create customer: %w", err)
	}

	sp.logger.WithFields(logrus.Fields{
		"customer_id": c.ID,
		"user_id":     userID,
	}).Info("Stripe customer created")

	return c.ID, nil
}

// CreateSubscription creates a new subscription
func (sp *StripeProvider) CreateSubscription(ctx context.Context, req *SubscriptionRequest) (*SubscriptionResponse, error) {
	// Ensure customer has a default payment method
	if req.PaymentMethodID != "" {
		// Attach payment method to customer
		pmParams := &stripe.PaymentMethodAttachParams{
			Customer: stripe.String(req.CustomerID),
		}
		_, err := paymentmethod.Attach(req.PaymentMethodID, pmParams)
		if err != nil {
			sp.logger.WithError(err).Error("Failed to attach payment method")
			return nil, fmt.Errorf("failed to attach payment method: %w", err)
		}

		// Set as default payment method
		customerParams := &stripe.CustomerParams{
			InvoiceSettings: &stripe.CustomerInvoiceSettingsParams{
				DefaultPaymentMethod: stripe.String(req.PaymentMethodID),
			},
		}
		_, err = customer.Update(req.CustomerID, customerParams)
		if err != nil {
			sp.logger.WithError(err).Error("Failed to set default payment method")
			return nil, fmt.Errorf("failed to set default payment method: %w", err)
		}
	}

	// Create subscription
	params := &stripe.SubscriptionParams{
		Customer: stripe.String(req.CustomerID),
		Items: []*stripe.SubscriptionItemsParams{
			{
				Price: stripe.String(req.PriceID),
			},
		},
		Metadata: map[string]string{
			"user_id": req.UserID,
			"plan_id": req.PlanID,
		},
	}

	// Add trial period if specified
	if req.TrialDays > 0 {
		params.TrialPeriodDays = stripe.Int64(int64(req.TrialDays))
	}

	// Configure payment behavior
	params.PaymentBehavior = stripe.String("default_incomplete")
	params.PaymentSettings = &stripe.SubscriptionPaymentSettingsParams{
		SaveDefaultPaymentMethod: stripe.String("on_subscription"),
	}

	sub, err := subscription.New(params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to create Stripe subscription")
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	response := &SubscriptionResponse{
		SubscriptionID:       sub.ID,
		Status:               string(sub.Status),
		CurrentPeriodStart:   time.Unix(sub.CurrentPeriodStart, 0),
		CurrentPeriodEnd:     time.Unix(sub.CurrentPeriodEnd, 0),
		CancelAtPeriodEnd:    sub.CancelAtPeriodEnd,
		LatestInvoiceID:      "",
		ClientSecret:         "",
	}

	if sub.TrialStart > 0 {
		trialStart := time.Unix(sub.TrialStart, 0)
		response.TrialStart = &trialStart
	}

	if sub.TrialEnd > 0 {
		trialEnd := time.Unix(sub.TrialEnd, 0)
		response.TrialEnd = &trialEnd
	}

	if sub.LatestInvoice != nil {
		response.LatestInvoiceID = sub.LatestInvoice.ID
		if sub.LatestInvoice.PaymentIntent != nil {
			response.ClientSecret = sub.LatestInvoice.PaymentIntent.ClientSecret
		}
	}

	sp.logger.WithFields(logrus.Fields{
		"subscription_id": sub.ID,
		"user_id":        req.UserID,
		"status":         sub.Status,
	}).Info("Stripe subscription created")

	return response, nil
}

// UpdateSubscription updates an existing subscription
func (sp *StripeProvider) UpdateSubscription(ctx context.Context, subscriptionID, newPriceID string) (*SubscriptionResponse, error) {
	// Get current subscription
	sub, err := subscription.Get(subscriptionID, nil)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to get Stripe subscription")
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	// Update subscription with new price
	params := &stripe.SubscriptionParams{
		Items: []*stripe.SubscriptionItemsParams{
			{
				ID:    stripe.String(sub.Items.Data[0].ID),
				Price: stripe.String(newPriceID),
			},
		},
		ProrationBehavior: stripe.String("create_prorations"),
	}

	updatedSub, err := subscription.Update(subscriptionID, params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to update Stripe subscription")
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	response := &SubscriptionResponse{
		SubscriptionID:     updatedSub.ID,
		Status:             string(updatedSub.Status),
		CurrentPeriodStart: time.Unix(updatedSub.CurrentPeriodStart, 0),
		CurrentPeriodEnd:   time.Unix(updatedSub.CurrentPeriodEnd, 0),
		CancelAtPeriodEnd:  updatedSub.CancelAtPeriodEnd,
	}

	sp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"new_price_id":    newPriceID,
	}).Info("Stripe subscription updated")

	return response, nil
}

// CancelSubscription cancels a subscription
func (sp *StripeProvider) CancelSubscription(ctx context.Context, subscriptionID string, immediate bool) error {
	if immediate {
		// Cancel immediately
		params := &stripe.SubscriptionCancelParams{}
		_, err := subscription.Cancel(subscriptionID, params)
		if err != nil {
			sp.logger.WithError(err).Error("Failed to cancel Stripe subscription")
			return fmt.Errorf("failed to cancel subscription: %w", err)
		}
	} else {
		// Cancel at period end
		params := &stripe.SubscriptionParams{}
		params.CancelAtPeriodEnd = stripe.Bool(true)
		_, err := subscription.Update(subscriptionID, params)
		if err != nil {
			sp.logger.WithError(err).Error("Failed to update Stripe subscription")
			return fmt.Errorf("failed to update subscription: %w", err)
		}
	}

	sp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"immediate":       immediate,
	}).Info("Stripe subscription cancelled")

	return nil
}

// ReactivateSubscription reactivates a cancelled subscription
func (sp *StripeProvider) ReactivateSubscription(ctx context.Context, subscriptionID string) error {
	params := &stripe.SubscriptionParams{
		CancelAtPeriodEnd: stripe.Bool(false),
	}

	_, err := subscription.Update(subscriptionID, params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to reactivate Stripe subscription")
		return fmt.Errorf("failed to reactivate subscription: %w", err)
	}

	sp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
	}).Info("Stripe subscription reactivated")

	return nil
}

// CreatePaymentIntent creates a payment intent for one-time payments
func (sp *StripeProvider) CreatePaymentIntent(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(req.Amount),
		Currency: stripe.String(req.Currency),
		Customer: stripe.String(req.CustomerID),
		Metadata: map[string]string{
			"user_id": req.UserID,
		},
	}

	if req.PaymentMethodID != "" {
		params.PaymentMethod = stripe.String(req.PaymentMethodID)
		params.Confirm = stripe.Bool(true)
	}

	if req.Description != "" {
		params.Description = stripe.String(req.Description)
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to create Stripe payment intent")
		return nil, fmt.Errorf("failed to create payment intent: %w", err)
	}

	response := &PaymentResponse{
		PaymentID:    pi.ID,
		Status:       string(pi.Status),
		ClientSecret: pi.ClientSecret,
		Amount:       pi.Amount,
		Currency:     string(pi.Currency),
	}

	sp.logger.WithFields(logrus.Fields{
		"payment_id": pi.ID,
		"amount":     pi.Amount,
		"status":     pi.Status,
	}).Info("Stripe payment intent created")

	return response, nil
}

// ConfirmPayment confirms a payment intent
func (sp *StripeProvider) ConfirmPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	params := &stripe.PaymentIntentConfirmParams{}
	pi, err := paymentintent.Confirm(paymentID, params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to confirm Stripe payment")
		return nil, fmt.Errorf("failed to confirm payment: %w", err)
	}

	response := &PaymentResponse{
		PaymentID:    pi.ID,
		Status:       string(pi.Status),
		ClientSecret: pi.ClientSecret,
		Amount:       pi.Amount,
		Currency:     string(pi.Currency),
	}

	sp.logger.WithFields(logrus.Fields{
		"payment_id": pi.ID,
		"status":     pi.Status,
	}).Info("Stripe payment confirmed")

	return response, nil
}

// RefundPayment refunds a payment
func (sp *StripeProvider) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResponse, error) {
	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(paymentID),
	}

	if amount > 0 {
		params.Amount = stripe.Int64(amount)
	}

	r, err := refund.New(params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to create Stripe refund")
		return nil, fmt.Errorf("failed to create refund: %w", err)
	}

	response := &RefundResponse{
		RefundID: r.ID,
		Status:   string(r.Status),
		Amount:   r.Amount,
		Currency: string(r.Currency),
	}

	sp.logger.WithFields(logrus.Fields{
		"refund_id":  r.ID,
		"payment_id": paymentID,
		"amount":     r.Amount,
	}).Info("Stripe refund created")

	return response, nil
}

// AttachPaymentMethod attaches a payment method to a customer
func (sp *StripeProvider) AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	params := &stripe.PaymentMethodAttachParams{
		Customer: stripe.String(customerID),
	}

	_, err := paymentmethod.Attach(paymentMethodID, params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to attach payment method")
		return fmt.Errorf("failed to attach payment method: %w", err)
	}

	sp.logger.WithFields(logrus.Fields{
		"customer_id":       customerID,
		"payment_method_id": paymentMethodID,
	}).Info("Payment method attached")

	return nil
}

// DetachPaymentMethod detaches a payment method from a customer
func (sp *StripeProvider) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	params := &stripe.PaymentMethodDetachParams{}

	_, err := paymentmethod.Detach(paymentMethodID, params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to detach payment method")
		return fmt.Errorf("failed to detach payment method: %w", err)
	}

	sp.logger.WithFields(logrus.Fields{
		"payment_method_id": paymentMethodID,
	}).Info("Payment method detached")

	return nil
}

// SetDefaultPaymentMethod sets the default payment method for a customer
func (sp *StripeProvider) SetDefaultPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	params := &stripe.CustomerParams{
		InvoiceSettings: &stripe.CustomerInvoiceSettingsParams{
			DefaultPaymentMethod: stripe.String(paymentMethodID),
		},
	}

	_, err := customer.Update(customerID, params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to set default payment method")
		return fmt.Errorf("failed to set default payment method: %w", err)
	}

	sp.logger.WithFields(logrus.Fields{
		"customer_id":       customerID,
		"payment_method_id": paymentMethodID,
	}).Info("Default payment method set")

	return nil
}

// VerifyWebhook verifies a Stripe webhook signature
func (sp *StripeProvider) VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error) {
	if sp.webhookSecret == "" {
		return nil, fmt.Errorf("webhook secret is not configured")
	}

	event, err := webhook.ConstructEvent(payload, signature, sp.webhookSecret)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to verify Stripe webhook")
		return nil, fmt.Errorf("failed to verify webhook: %w", err)
	}

	webhookEvent := &WebhookEvent{
		ID:   event.ID,
		Type: string(event.Type),
		Data: event.Data.Raw,
	}

	sp.logger.WithFields(logrus.Fields{
		"event_id":   event.ID,
		"event_type": event.Type,
	}).Info("Stripe webhook verified")

	return webhookEvent, nil
}

// GetSubscription retrieves a subscription by ID
func (sp *StripeProvider) GetSubscription(ctx context.Context, subscriptionID string) (*SubscriptionResponse, error) {
	sub, err := subscription.Get(subscriptionID, nil)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to get Stripe subscription")
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	response := &SubscriptionResponse{
		SubscriptionID:     sub.ID,
		Status:             string(sub.Status),
		CurrentPeriodStart: time.Unix(sub.CurrentPeriodStart, 0),
		CurrentPeriodEnd:   time.Unix(sub.CurrentPeriodEnd, 0),
		CancelAtPeriodEnd:  sub.CancelAtPeriodEnd,
	}

	if sub.TrialStart > 0 {
		trialStart := time.Unix(sub.TrialStart, 0)
		response.TrialStart = &trialStart
	}

	if sub.TrialEnd > 0 {
		trialEnd := time.Unix(sub.TrialEnd, 0)
		response.TrialEnd = &trialEnd
	}

	return response, nil
}

// GetPaymentStatus retrieves the status of a payment
func (sp *StripeProvider) GetPaymentStatus(ctx context.Context, paymentID string) (string, error) {
	pi, err := paymentintent.Get(paymentID, nil)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to get Stripe payment intent")
		return "", fmt.Errorf("failed to get payment intent: %w", err)
	}

	return string(pi.Status), nil
}

// IsTestMode returns whether the provider is in test mode
func (sp *StripeProvider) IsTestMode() bool {
	return sp.testMode
}

// GetProviderName returns the name of the payment provider
func (sp *StripeProvider) GetProviderName() string {
	return "stripe"
}

// RetryFailedPayment attempts to retry a failed payment
func (sp *StripeProvider) RetryFailedPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	// Get the payment intent
	pi, err := paymentintent.Get(paymentID, nil)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to get Stripe payment intent")
		return nil, fmt.Errorf("failed to get payment intent: %w", err)
	}

	// Only retry if payment failed
	if pi.Status != stripe.PaymentIntentStatusRequiresPaymentMethod {
		return nil, fmt.Errorf("payment is not in a retryable state: %s", pi.Status)
	}

	// Confirm the payment again
	params := &stripe.PaymentIntentConfirmParams{}
	confirmedPI, err := paymentintent.Confirm(paymentID, params)
	if err != nil {
		sp.logger.WithError(err).Error("Failed to retry Stripe payment")
		return nil, fmt.Errorf("failed to retry payment: %w", err)
	}

	response := &PaymentResponse{
		PaymentID:    confirmedPI.ID,
		Status:       string(confirmedPI.Status),
		ClientSecret: confirmedPI.ClientSecret,
		Amount:       confirmedPI.Amount,
		Currency:     string(confirmedPI.Currency),
	}

	sp.logger.WithFields(logrus.Fields{
		"payment_id": confirmedPI.ID,
		"status":     confirmedPI.Status,
	}).Info("Stripe payment retried")

	return response, nil
}
