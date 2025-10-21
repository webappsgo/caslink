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
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

// LemonSqueezyProvider implements payment processing with LemonSqueezy
type LemonSqueezyProvider struct {
	apiKey        string
	webhookSecret string
	logger        *logrus.Logger
	baseURL       string
	httpClient    *http.Client
	testMode      bool
}

// LemonSqueezyCustomerRequest represents customer creation request
type LemonSqueezyCustomerRequest struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"attributes"`
	} `json:"data"`
}

// LemonSqueezyCustomerResponse represents customer response
type LemonSqueezyCustomerResponse struct {
	Data struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			StoreID int    `json:"store_id"`
			Name    string `json:"name"`
			Email   string `json:"email"`
		} `json:"attributes"`
	} `json:"data"`
}

// LemonSqueezySubscriptionRequest represents subscription creation request
type LemonSqueezySubscriptionRequest struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			StoreID    int    `json:"store_id"`
			CustomerID int    `json:"customer_id"`
			VariantID  int    `json:"variant_id"`
		} `json:"attributes"`
		Relationships struct {
			Store struct {
				Data struct {
					Type string `json:"type"`
					ID   string `json:"id"`
				} `json:"data"`
			} `json:"store"`
		} `json:"relationships"`
	} `json:"data"`
}

// LemonSqueezySubscriptionResponse represents subscription response
type LemonSqueezySubscriptionResponse struct {
	Data struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			StoreID            int    `json:"store_id"`
			CustomerID         int    `json:"customer_id"`
			Status             string `json:"status"`
			StatusFormatted    string `json:"status_formatted"`
			RenewsAt           string `json:"renews_at"`
			EndsAt             string `json:"ends_at,omitempty"`
			TrialEndsAt        string `json:"trial_ends_at,omitempty"`
			CreatedAt          string `json:"created_at"`
			UpdatedAt          string `json:"updated_at"`
			TestMode           bool   `json:"test_mode"`
			URLs               struct {
				UpdatePaymentMethod string `json:"update_payment_method"`
				CustomerPortal      string `json:"customer_portal"`
			} `json:"urls"`
		} `json:"attributes"`
	} `json:"data"`
}

// LemonSqueezyCheckoutRequest represents checkout creation request
type LemonSqueezyCheckoutRequest struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			StoreID       int               `json:"store_id"`
			VariantID     int               `json:"variant_id"`
			CustomPrice   int               `json:"custom_price,omitempty"`
			CheckoutData  map[string]string `json:"checkout_data,omitempty"`
		} `json:"attributes"`
	} `json:"data"`
}

// LemonSqueezyCheckoutResponse represents checkout response
type LemonSqueezyCheckoutResponse struct {
	Data struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			StoreID   int    `json:"store_id"`
			VariantID int    `json:"variant_id"`
			URL       string `json:"url"`
		} `json:"attributes"`
	} `json:"data"`
}

// NewLemonSqueezyProvider creates a new LemonSqueezy payment provider
func NewLemonSqueezyProvider(apiKey, webhookSecret string, logger *logrus.Logger) (*LemonSqueezyProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("lemonsqueezy API key is required")
	}

	// LemonSqueezy uses a single API for both test and production
	baseURL := "https://api.lemonsqueezy.com/v1"

	// Test mode is determined by store configuration, not API key
	testMode := false

	provider := &LemonSqueezyProvider{
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
		"base_url": baseURL,
	}).Info("LemonSqueezy provider initialized")

	return provider, nil
}

// CreateCustomer creates a new LemonSqueezy customer
func (lsp *LemonSqueezyProvider) CreateCustomer(ctx context.Context, userID, email, name string) (string, error) {
	reqBody := LemonSqueezyCustomerRequest{}
	reqBody.Data.Type = "customers"
	reqBody.Data.Attributes.Name = name
	reqBody.Data.Attributes.Email = email

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/customers", lsp.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", lsp.apiKey))
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/vnd.api+json")

	resp, err := lsp.httpClient.Do(req)
	if err != nil {
		lsp.logger.WithError(err).Error("Failed to create LemonSqueezy customer")
		return "", fmt.Errorf("failed to create customer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create customer: %s - %s", resp.Status, string(body))
	}

	var customerResp LemonSqueezyCustomerResponse
	if err := json.NewDecoder(resp.Body).Decode(&customerResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	lsp.logger.WithFields(logrus.Fields{
		"customer_id": customerResp.Data.ID,
		"user_id":     userID,
	}).Info("LemonSqueezy customer created")

	return customerResp.Data.ID, nil
}

// CreateSubscription creates a new LemonSqueezy subscription
func (lsp *LemonSqueezyProvider) CreateSubscription(ctx context.Context, req *SubscriptionRequest) (*SubscriptionResponse, error) {
	// LemonSqueezy requires variant ID (equivalent to price ID)
	variantID, err := strconv.Atoi(req.PriceID)
	if err != nil {
		return nil, fmt.Errorf("invalid variant ID: %w", err)
	}

	customerID, err := strconv.Atoi(req.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("invalid customer ID: %w", err)
	}

	// Note: LemonSqueezy typically uses checkouts for subscription creation
	// This is a direct API approach which may require store ID
	reqBody := LemonSqueezySubscriptionRequest{}
	reqBody.Data.Type = "subscriptions"
	reqBody.Data.Attributes.CustomerID = customerID
	reqBody.Data.Attributes.VariantID = variantID

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/subscriptions", lsp.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", lsp.apiKey))
	httpReq.Header.Set("Content-Type", "application/vnd.api+json")
	httpReq.Header.Set("Accept", "application/vnd.api+json")

	resp, err := lsp.httpClient.Do(httpReq)
	if err != nil {
		lsp.logger.WithError(err).Error("Failed to create LemonSqueezy subscription")
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create subscription: %s - %s", resp.Status, string(body))
	}

	var subResp LemonSqueezySubscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&subResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse timestamps
	createdAt, _ := time.Parse(time.RFC3339, subResp.Data.Attributes.CreatedAt)
	renewsAt, _ := time.Parse(time.RFC3339, subResp.Data.Attributes.RenewsAt)

	response := &SubscriptionResponse{
		SubscriptionID:     subResp.Data.ID,
		Status:             lsp.mapLemonSqueezyStatus(subResp.Data.Attributes.Status),
		CurrentPeriodStart: createdAt,
		CurrentPeriodEnd:   renewsAt,
		CancelAtPeriodEnd:  subResp.Data.Attributes.EndsAt != "",
		ClientSecret:       subResp.Data.Attributes.URLs.UpdatePaymentMethod,
	}

	// Handle trial period
	if subResp.Data.Attributes.TrialEndsAt != "" {
		trialEnd, _ := time.Parse(time.RFC3339, subResp.Data.Attributes.TrialEndsAt)
		response.TrialEnd = &trialEnd
	}

	// Set test mode based on response
	lsp.testMode = subResp.Data.Attributes.TestMode

	lsp.logger.WithFields(logrus.Fields{
		"subscription_id": subResp.Data.ID,
		"user_id":        req.UserID,
		"status":         subResp.Data.Attributes.Status,
	}).Info("LemonSqueezy subscription created")

	return response, nil
}

// UpdateSubscription updates a LemonSqueezy subscription
func (lsp *LemonSqueezyProvider) UpdateSubscription(ctx context.Context, subscriptionID, newPriceID string) (*SubscriptionResponse, error) {
	variantID, err := strconv.Atoi(newPriceID)
	if err != nil {
		return nil, fmt.Errorf("invalid variant ID: %w", err)
	}

	updateReq := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "subscriptions",
			"id":   subscriptionID,
			"attributes": map[string]interface{}{
				"variant_id": variantID,
			},
		},
	}

	jsonData, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/subscriptions/%s", lsp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", lsp.apiKey))
	httpReq.Header.Set("Content-Type", "application/vnd.api+json")
	httpReq.Header.Set("Accept", "application/vnd.api+json")

	resp, err := lsp.httpClient.Do(httpReq)
	if err != nil {
		lsp.logger.WithError(err).Error("Failed to update LemonSqueezy subscription")
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to update subscription: %s - %s", resp.Status, string(body))
	}

	lsp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"new_variant_id":  newPriceID,
	}).Info("LemonSqueezy subscription updated")

	return lsp.GetSubscription(ctx, subscriptionID)
}

// CancelSubscription cancels a LemonSqueezy subscription
func (lsp *LemonSqueezyProvider) CancelSubscription(ctx context.Context, subscriptionID string, immediate bool) error {
	// LemonSqueezy uses DELETE for cancellation
	url := fmt.Sprintf("%s/subscriptions/%s", lsp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", lsp.apiKey))
	httpReq.Header.Set("Accept", "application/vnd.api+json")

	resp, err := lsp.httpClient.Do(httpReq)
	if err != nil {
		lsp.logger.WithError(err).Error("Failed to cancel LemonSqueezy subscription")
		return fmt.Errorf("failed to cancel subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to cancel subscription: %s - %s", resp.Status, string(body))
	}

	lsp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"immediate":       immediate,
	}).Info("LemonSqueezy subscription cancelled")

	return nil
}

// ReactivateSubscription reactivates a cancelled LemonSqueezy subscription
func (lsp *LemonSqueezyProvider) ReactivateSubscription(ctx context.Context, subscriptionID string) error {
	// LemonSqueezy doesn't directly support reactivation
	// Customer would need to create a new subscription
	return fmt.Errorf("subscription reactivation not supported - please create a new subscription")
}

// CreatePaymentIntent creates a LemonSqueezy checkout for one-time payment
func (lsp *LemonSqueezyProvider) CreatePaymentIntent(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	variantID, err := strconv.Atoi(req.PaymentMethodID)
	if err != nil {
		return nil, fmt.Errorf("invalid variant ID: %w", err)
	}

	checkoutReq := LemonSqueezyCheckoutRequest{}
	checkoutReq.Data.Type = "checkouts"
	checkoutReq.Data.Attributes.VariantID = variantID
	checkoutReq.Data.Attributes.CheckoutData = map[string]string{
		"email": req.UserID, // Simplified - would use actual email
	}

	jsonData, err := json.Marshal(checkoutReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/checkouts", lsp.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", lsp.apiKey))
	httpReq.Header.Set("Content-Type", "application/vnd.api+json")
	httpReq.Header.Set("Accept", "application/vnd.api+json")

	resp, err := lsp.httpClient.Do(httpReq)
	if err != nil {
		lsp.logger.WithError(err).Error("Failed to create LemonSqueezy checkout")
		return nil, fmt.Errorf("failed to create checkout: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create checkout: %s - %s", resp.Status, string(body))
	}

	var checkoutResp LemonSqueezyCheckoutResponse
	if err := json.NewDecoder(resp.Body).Decode(&checkoutResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	response := &PaymentResponse{
		PaymentID:    checkoutResp.Data.ID,
		Status:       "pending",
		ClientSecret: checkoutResp.Data.Attributes.URL,
		Amount:       req.Amount,
		Currency:     req.Currency,
	}

	lsp.logger.WithFields(logrus.Fields{
		"checkout_id": checkoutResp.Data.ID,
		"amount":      req.Amount,
	}).Info("LemonSqueezy checkout created")

	return response, nil
}

// ConfirmPayment - LemonSqueezy handles confirmation automatically
func (lsp *LemonSqueezyProvider) ConfirmPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	// LemonSqueezy automatically confirms payments through their checkout
	return &PaymentResponse{
		PaymentID: paymentID,
		Status:    "pending",
	}, nil
}

// RefundPayment - LemonSqueezy refunds are managed through dashboard
func (lsp *LemonSqueezyProvider) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResponse, error) {
	// LemonSqueezy doesn't provide a direct refund API
	// Refunds must be processed through the dashboard
	return nil, fmt.Errorf("refunds must be processed through LemonSqueezy dashboard")
}

// GetSubscription retrieves a LemonSqueezy subscription
func (lsp *LemonSqueezyProvider) GetSubscription(ctx context.Context, subscriptionID string) (*SubscriptionResponse, error) {
	url := fmt.Sprintf("%s/subscriptions/%s", lsp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", lsp.apiKey))
	httpReq.Header.Set("Accept", "application/vnd.api+json")

	resp, err := lsp.httpClient.Do(httpReq)
	if err != nil {
		lsp.logger.WithError(err).Error("Failed to get LemonSqueezy subscription")
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get subscription: %s - %s", resp.Status, string(body))
	}

	var subResp LemonSqueezySubscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&subResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	createdAt, _ := time.Parse(time.RFC3339, subResp.Data.Attributes.CreatedAt)
	renewsAt, _ := time.Parse(time.RFC3339, subResp.Data.Attributes.RenewsAt)

	response := &SubscriptionResponse{
		SubscriptionID:     subResp.Data.ID,
		Status:             lsp.mapLemonSqueezyStatus(subResp.Data.Attributes.Status),
		CurrentPeriodStart: createdAt,
		CurrentPeriodEnd:   renewsAt,
		CancelAtPeriodEnd:  subResp.Data.Attributes.EndsAt != "",
	}

	if subResp.Data.Attributes.TrialEndsAt != "" {
		trialEnd, _ := time.Parse(time.RFC3339, subResp.Data.Attributes.TrialEndsAt)
		response.TrialEnd = &trialEnd
	}

	return response, nil
}

// VerifyWebhook verifies a LemonSqueezy webhook signature
func (lsp *LemonSqueezyProvider) VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error) {
	if lsp.webhookSecret == "" {
		return nil, fmt.Errorf("webhook secret is not configured")
	}

	// Verify HMAC signature
	h := hmac.New(sha256.New, []byte(lsp.webhookSecret))
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

	meta, _ := event["meta"].(map[string]interface{})
	eventName, _ := meta["event_name"].(string)

	webhookEvent := &WebhookEvent{
		ID:   fmt.Sprintf("%v", meta["webhook_id"]),
		Type: eventName,
		Data: payload,
	}

	lsp.logger.WithFields(logrus.Fields{
		"event_name": eventName,
	}).Info("LemonSqueezy webhook verified")

	return webhookEvent, nil
}

// mapLemonSqueezyStatus maps LemonSqueezy status to standardized status
func (lsp *LemonSqueezyProvider) mapLemonSqueezyStatus(lsStatus string) string {
	statusMap := map[string]string{
		"on_trial": "trialing",
		"active":   "active",
		"paused":   "past_due",
		"past_due": "past_due",
		"unpaid":   "past_due",
		"cancelled": "canceled",
		"expired":  "canceled",
	}

	if status, ok := statusMap[lsStatus]; ok {
		return status
	}

	return lsStatus
}

// IsTestMode returns whether the provider is in test mode
func (lsp *LemonSqueezyProvider) IsTestMode() bool {
	return lsp.testMode
}

// GetProviderName returns the name of the payment provider
func (lsp *LemonSqueezyProvider) GetProviderName() string {
	return "lemonsqueezy"
}

// AttachPaymentMethod - Not applicable for LemonSqueezy
func (lsp *LemonSqueezyProvider) AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	return fmt.Errorf("payment method attachment handled through LemonSqueezy checkout")
}

// DetachPaymentMethod - Not applicable for LemonSqueezy
func (lsp *LemonSqueezyProvider) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	return fmt.Errorf("payment method detachment handled through LemonSqueezy portal")
}

// SetDefaultPaymentMethod - Not applicable for LemonSqueezy
func (lsp *LemonSqueezyProvider) SetDefaultPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	return fmt.Errorf("default payment method handled through LemonSqueezy portal")
}

// GetPaymentStatus retrieves payment status
func (lsp *LemonSqueezyProvider) GetPaymentStatus(ctx context.Context, paymentID string) (string, error) {
	// LemonSqueezy uses orders for payment tracking
	url := fmt.Sprintf("%s/orders/%s", lsp.baseURL, paymentID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", lsp.apiKey))
	httpReq.Header.Set("Accept", "application/vnd.api+json")

	resp, err := lsp.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to get order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get order: %s - %s", resp.Status, string(body))
	}

	var orderResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&orderResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	data := orderResp["data"].(map[string]interface{})
	attributes := data["attributes"].(map[string]interface{})
	status, _ := attributes["status"].(string)

	return lsp.mapLemonSqueezyStatus(status), nil
}

// RetryFailedPayment retries a failed payment
func (lsp *LemonSqueezyProvider) RetryFailedPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	// LemonSqueezy handles payment retries automatically
	return nil, fmt.Errorf("payment retry handled automatically by LemonSqueezy")
}
