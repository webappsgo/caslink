package providers

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

	"github.com/sirupsen/logrus"
)

// PaddleProvider implements payment processing with Paddle
type PaddleProvider struct {
	apiKey        string
	webhookSecret string
	logger        *logrus.Logger
	baseURL       string
	httpClient    *http.Client
	testMode      bool
}

// PaddleCustomerRequest represents customer creation request
type PaddleCustomerRequest struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// PaddleCustomerResponse represents customer response
type PaddleCustomerResponse struct {
	Data struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"data"`
}

// PaddleSubscriptionRequest represents subscription creation request
type PaddleSubscriptionRequest struct {
	CustomerID string                      `json:"customer_id"`
	Items      []PaddleSubscriptionItem    `json:"items"`
	Metadata   map[string]string           `json:"custom_data,omitempty"`
}

// PaddleSubscriptionItem represents subscription item
type PaddleSubscriptionItem struct {
	PriceID  string `json:"price_id"`
	Quantity int    `json:"quantity"`
}

// PaddleSubscriptionResponse represents subscription response
type PaddleSubscriptionResponse struct {
	Data struct {
		ID                 string    `json:"id"`
		Status             string    `json:"status"`
		CustomerID         string    `json:"customer_id"`
		CurrentBillingPeriod struct {
			StartsAt string `json:"starts_at"`
			EndsAt   string `json:"ends_at"`
		} `json:"current_billing_period"`
		ScheduledChange *struct {
			Action       string `json:"action"`
			EffectiveAt  string `json:"effective_at"`
		} `json:"scheduled_change,omitempty"`
		ManagementURLs struct {
			UpdatePaymentMethod string `json:"update_payment_method"`
			Cancel              string `json:"cancel"`
		} `json:"management_urls"`
	} `json:"data"`
}

// PaddleTransactionRequest represents transaction creation request
type PaddleTransactionRequest struct {
	Items []PaddleTransactionItem `json:"items"`
	CustomerID string              `json:"customer_id,omitempty"`
	CustomData map[string]string   `json:"custom_data,omitempty"`
}

// PaddleTransactionItem represents transaction item
type PaddleTransactionItem struct {
	PriceID  string `json:"price_id"`
	Quantity int    `json:"quantity"`
}

// PaddleTransactionResponse represents transaction response
type PaddleTransactionResponse struct {
	Data struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		CustomerID string `json:"customer_id"`
		Details    struct {
			Totals struct {
				Total    string `json:"total"`
				Currency string `json:"currency_code"`
			} `json:"totals"`
		} `json:"details"`
		CheckoutURL string `json:"checkout_url"`
	} `json:"data"`
}

// NewPaddleProvider creates a new Paddle payment provider
func NewPaddleProvider(apiKey, webhookSecret string, logger *logrus.Logger) (*PaddleProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("paddle API key is required")
	}

	// Determine environment
	baseURL := "https://api.paddle.com"
	testMode := false

	// Paddle uses different API keys for sandbox vs production
	if len(apiKey) > 5 && apiKey[:5] == "test_" {
		baseURL = "https://sandbox-api.paddle.com"
		testMode = true
	}

	provider := &PaddleProvider{
		apiKey:        apiKey,
		webhookSecret: webhookSecret,
		logger:        logger,
		baseURL:       baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		testMode: testMode,
	}

	logger.WithFields(logrus.Fields{
		"test_mode": testMode,
		"base_url":  baseURL,
	}).Info("Paddle provider initialized")

	return provider, nil
}

// CreateCustomer creates a new Paddle customer
func (pp *PaddleProvider) CreateCustomer(ctx context.Context, userID, email, name string) (string, error) {
	reqBody := PaddleCustomerRequest{
		Email: email,
		Name:  name,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/customers", pp.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", pp.apiKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(req)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to create Paddle customer")
		return "", fmt.Errorf("failed to create customer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create customer: %s - %s", resp.Status, string(body))
	}

	var customerResp PaddleCustomerResponse
	if err := json.NewDecoder(resp.Body).Decode(&customerResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	pp.logger.WithFields(logrus.Fields{
		"customer_id": customerResp.Data.ID,
		"user_id":     userID,
	}).Info("Paddle customer created")

	return customerResp.Data.ID, nil
}

// CreateSubscription creates a new Paddle subscription
func (pp *PaddleProvider) CreateSubscription(ctx context.Context, req *SubscriptionRequest) (*SubscriptionResponse, error) {
	reqBody := PaddleSubscriptionRequest{
		CustomerID: req.CustomerID,
		Items: []PaddleSubscriptionItem{
			{
				PriceID:  req.PriceID,
				Quantity: 1,
			},
		},
		Metadata: map[string]string{
			"user_id": req.UserID,
			"plan_id": req.PlanID,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/subscriptions", pp.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", pp.apiKey))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to create Paddle subscription")
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create subscription: %s - %s", resp.Status, string(body))
	}

	var subResp PaddleSubscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&subResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	startsAt, _ := time.Parse(time.RFC3339, subResp.Data.CurrentBillingPeriod.StartsAt)
	endsAt, _ := time.Parse(time.RFC3339, subResp.Data.CurrentBillingPeriod.EndsAt)

	response := &SubscriptionResponse{
		SubscriptionID:     subResp.Data.ID,
		Status:             pp.mapPaddleStatus(subResp.Data.Status),
		CurrentPeriodStart: startsAt,
		CurrentPeriodEnd:   endsAt,
		CancelAtPeriodEnd:  subResp.Data.ScheduledChange != nil && subResp.Data.ScheduledChange.Action == "cancel",
		ClientSecret:       subResp.Data.ManagementURLs.UpdatePaymentMethod,
	}

	pp.logger.WithFields(logrus.Fields{
		"subscription_id": subResp.Data.ID,
		"user_id":        req.UserID,
		"status":         subResp.Data.Status,
	}).Info("Paddle subscription created")

	return response, nil
}

// UpdateSubscription updates a Paddle subscription
func (pp *PaddleProvider) UpdateSubscription(ctx context.Context, subscriptionID, newPriceID string) (*SubscriptionResponse, error) {
	updateReq := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"price_id": newPriceID,
				"quantity": 1,
			},
		},
		"proration_billing_mode": "prorated_immediately",
	}

	jsonData, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/subscriptions/%s", pp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", pp.apiKey))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to update Paddle subscription")
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to update subscription: %s - %s", resp.Status, string(body))
	}

	pp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"new_price_id":    newPriceID,
	}).Info("Paddle subscription updated")

	return pp.GetSubscription(ctx, subscriptionID)
}

// CancelSubscription cancels a Paddle subscription
func (pp *PaddleProvider) CancelSubscription(ctx context.Context, subscriptionID string, immediate bool) error {
	cancelReq := map[string]interface{}{
		"effective_from": "next_billing_period",
	}

	if immediate {
		cancelReq["effective_from"] = "immediately"
	}

	jsonData, err := json.Marshal(cancelReq)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/subscriptions/%s/cancel", pp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", pp.apiKey))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to cancel Paddle subscription")
		return fmt.Errorf("failed to cancel subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to cancel subscription: %s - %s", resp.Status, string(body))
	}

	pp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"immediate":       immediate,
	}).Info("Paddle subscription cancelled")

	return nil
}

// ReactivateSubscription reactivates a cancelled Paddle subscription
func (pp *PaddleProvider) ReactivateSubscription(ctx context.Context, subscriptionID string) error {
	url := fmt.Sprintf("%s/subscriptions/%s/resume", pp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBufferString(`{"effective_from":"immediately"}`))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", pp.apiKey))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to reactivate Paddle subscription")
		return fmt.Errorf("failed to reactivate subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to reactivate subscription: %s - %s", resp.Status, string(body))
	}

	pp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
	}).Info("Paddle subscription reactivated")

	return nil
}

// CreatePaymentIntent creates a Paddle transaction (one-time payment)
func (pp *PaddleProvider) CreatePaymentIntent(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	// Paddle requires a price ID for transactions - we'll need to create one or use existing
	// For now, this is a simplified implementation
	transactionReq := PaddleTransactionRequest{
		CustomerID: req.CustomerID,
		Items: []PaddleTransactionItem{
			{
				PriceID:  req.PaymentMethodID, // Assuming this is the price ID
				Quantity: 1,
			},
		},
		CustomData: map[string]string{
			"user_id":     req.UserID,
			"description": req.Description,
		},
	}

	jsonData, err := json.Marshal(transactionReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/transactions", pp.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", pp.apiKey))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to create Paddle transaction")
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create transaction: %s - %s", resp.Status, string(body))
	}

	var txnResp PaddleTransactionResponse
	if err := json.NewDecoder(resp.Body).Decode(&txnResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	response := &PaymentResponse{
		PaymentID:    txnResp.Data.ID,
		Status:       pp.mapPaddleStatus(txnResp.Data.Status),
		ClientSecret: txnResp.Data.CheckoutURL,
		Currency:     txnResp.Data.Details.Totals.Currency,
	}

	pp.logger.WithFields(logrus.Fields{
		"transaction_id": txnResp.Data.ID,
		"status":         txnResp.Data.Status,
	}).Info("Paddle transaction created")

	return response, nil
}

// ConfirmPayment - Paddle handles payment confirmation automatically
func (pp *PaddleProvider) ConfirmPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	// Paddle automatically confirms payments through their checkout
	return &PaymentResponse{
		PaymentID: paymentID,
		Status:    "pending",
	}, nil
}

// RefundPayment creates a Paddle refund
func (pp *PaddleProvider) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResponse, error) {
	refundReq := map[string]interface{}{
		"reason": "customer_request",
	}

	// Paddle uses transaction ID for refunds
	jsonData, err := json.Marshal(refundReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/adjustments", pp.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", pp.apiKey))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to create Paddle refund")
		return nil, fmt.Errorf("failed to create refund: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create refund: %s - %s", resp.Status, string(body))
	}

	var refundResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&refundResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	data := refundResp["data"].(map[string]interface{})
	refundID := data["id"].(string)

	response := &RefundResponse{
		RefundID: refundID,
		Status:   "completed",
		Reason:   "customer_request",
	}

	pp.logger.WithFields(logrus.Fields{
		"refund_id":  refundID,
		"payment_id": paymentID,
	}).Info("Paddle refund created")

	return response, nil
}

// GetSubscription retrieves a Paddle subscription
func (pp *PaddleProvider) GetSubscription(ctx context.Context, subscriptionID string) (*SubscriptionResponse, error) {
	url := fmt.Sprintf("%s/subscriptions/%s", pp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", pp.apiKey))

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to get Paddle subscription")
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get subscription: %s - %s", resp.Status, string(body))
	}

	var subResp PaddleSubscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&subResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	startsAt, _ := time.Parse(time.RFC3339, subResp.Data.CurrentBillingPeriod.StartsAt)
	endsAt, _ := time.Parse(time.RFC3339, subResp.Data.CurrentBillingPeriod.EndsAt)

	response := &SubscriptionResponse{
		SubscriptionID:     subResp.Data.ID,
		Status:             pp.mapPaddleStatus(subResp.Data.Status),
		CurrentPeriodStart: startsAt,
		CurrentPeriodEnd:   endsAt,
		CancelAtPeriodEnd:  subResp.Data.ScheduledChange != nil && subResp.Data.ScheduledChange.Action == "cancel",
	}

	return response, nil
}

// VerifyWebhook verifies a Paddle webhook signature
func (pp *PaddleProvider) VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error) {
	if pp.webhookSecret == "" {
		return nil, fmt.Errorf("webhook secret is not configured")
	}

	// Verify HMAC signature
	h := hmac.New(sha256.New, []byte(pp.webhookSecret))
	h.Write(payload)
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return nil, fmt.Errorf("invalid webhook signature")
	}

	// Parse webhook event
	var event map[string]interface{}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	eventType, _ := event["event_type"].(string)
	eventID, _ := event["event_id"].(string)

	webhookEvent := &WebhookEvent{
		ID:   eventID,
		Type: eventType,
		Data: payload,
	}

	pp.logger.WithFields(logrus.Fields{
		"event_id":   eventID,
		"event_type": eventType,
	}).Info("Paddle webhook verified")

	return webhookEvent, nil
}

// mapPaddleStatus maps Paddle status to standardized status
func (pp *PaddleProvider) mapPaddleStatus(paddleStatus string) string {
	statusMap := map[string]string{
		"active":    "active",
		"trialing":  "trialing",
		"past_due":  "past_due",
		"paused":    "past_due",
		"canceled":  "canceled",
		"completed": "succeeded",
		"draft":     "pending",
		"ready":     "pending",
		"billed":    "succeeded",
		"paid":      "succeeded",
	}

	if status, ok := statusMap[paddleStatus]; ok {
		return status
	}

	return paddleStatus
}

// IsTestMode returns whether the provider is in test mode
func (pp *PaddleProvider) IsTestMode() bool {
	return pp.testMode
}

// GetProviderName returns the name of the payment provider
func (pp *PaddleProvider) GetProviderName() string {
	return "paddle"
}

// AttachPaymentMethod - Not directly applicable for Paddle
func (pp *PaddleProvider) AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	return fmt.Errorf("payment method attachment handled through Paddle checkout")
}

// DetachPaymentMethod - Not directly applicable for Paddle
func (pp *PaddleProvider) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	return fmt.Errorf("payment method detachment handled through Paddle portal")
}

// SetDefaultPaymentMethod - Not directly applicable for Paddle
func (pp *PaddleProvider) SetDefaultPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	return fmt.Errorf("default payment method handled through Paddle portal")
}

// GetPaymentStatus retrieves transaction status
func (pp *PaddleProvider) GetPaymentStatus(ctx context.Context, paymentID string) (string, error) {
	url := fmt.Sprintf("%s/transactions/%s", pp.baseURL, paymentID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", pp.apiKey))

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to get transaction: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get transaction: %s - %s", resp.Status, string(body))
	}

	var txnResp PaddleTransactionResponse
	if err := json.NewDecoder(resp.Body).Decode(&txnResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return pp.mapPaddleStatus(txnResp.Data.Status), nil
}

// RetryFailedPayment retries a failed payment
func (pp *PaddleProvider) RetryFailedPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	// Paddle handles payment retries automatically through their dunning system
	return nil, fmt.Errorf("payment retry handled automatically by Paddle dunning")
}
