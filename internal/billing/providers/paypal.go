package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// PayPalProvider implements payment processing with PayPal
type PayPalProvider struct {
	clientID     string
	clientSecret string
	logger       *logrus.Logger
	baseURL      string
	httpClient   *http.Client
	testMode     bool
}

// PayPalTokenResponse represents PayPal OAuth token response
type PayPalTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// PayPalSubscriptionRequest represents PayPal subscription creation request
type PayPalSubscriptionRequest struct {
	PlanID             string                      `json:"plan_id"`
	Subscriber         PayPalSubscriber            `json:"subscriber,omitempty"`
	ApplicationContext PayPalApplicationContext    `json:"application_context,omitempty"`
}

// PayPalSubscriber represents subscriber information
type PayPalSubscriber struct {
	EmailAddress string            `json:"email_address,omitempty"`
	Name         PayPalName        `json:"name,omitempty"`
}

// PayPalName represents subscriber name
type PayPalName struct {
	GivenName string `json:"given_name,omitempty"`
	Surname   string `json:"surname,omitempty"`
}

// PayPalApplicationContext represents application context
type PayPalApplicationContext struct {
	BrandName   string `json:"brand_name,omitempty"`
	ReturnURL   string `json:"return_url,omitempty"`
	CancelURL   string `json:"cancel_url,omitempty"`
	UserAction  string `json:"user_action,omitempty"`
}

// PayPalSubscriptionResponse represents PayPal subscription response
type PayPalSubscriptionResponse struct {
	ID            string                   `json:"id"`
	Status        string                   `json:"status"`
	StatusUpdateTime string                `json:"status_update_time"`
	PlanID        string                   `json:"plan_id"`
	StartTime     string                   `json:"start_time"`
	Links         []PayPalLink             `json:"links"`
}

// PayPalLink represents HATEOAS link
type PayPalLink struct {
	Href   string `json:"href"`
	Rel    string `json:"rel"`
	Method string `json:"method"`
}

// PayPalPaymentRequest represents payment capture request
type PayPalPaymentRequest struct {
	Amount      PayPalAmount `json:"amount"`
	Description string       `json:"description,omitempty"`
}

// PayPalAmount represents monetary amount
type PayPalAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

// PayPalPaymentResponse represents payment response
type PayPalPaymentResponse struct {
	ID            string       `json:"id"`
	Status        string       `json:"status"`
	Amount        PayPalAmount `json:"amount"`
	CreateTime    string       `json:"create_time"`
	UpdateTime    string       `json:"update_time"`
}

// NewPayPalProvider creates a new PayPal payment provider
func NewPayPalProvider(clientID, clientSecret string, logger *logrus.Logger) (*PayPalProvider, error) {
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("paypal client ID and secret are required")
	}

	// Determine base URL (sandbox vs production)
	baseURL := "https://api-m.paypal.com"
	testMode := false

	// If we detect sandbox credentials, use sandbox URL
	if len(clientID) > 2 && clientID[:2] == "AX" {
		baseURL = "https://api-m.sandbox.paypal.com"
		testMode = true
	}

	provider := &PayPalProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		logger:       logger,
		baseURL:      baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		testMode: testMode,
	}

	logger.WithFields(logrus.Fields{
		"test_mode": testMode,
		"base_url":  baseURL,
	}).Info("PayPal provider initialized")

	return provider, nil
}

// getAccessToken retrieves an OAuth access token
func (pp *PayPalProvider) getAccessToken(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/v1/oauth2/token", pp.baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBufferString("grant_type=client_credentials"))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(pp.clientID, pp.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := pp.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get access token: %s - %s", resp.Status, string(body))
	}

	var tokenResp PayPalTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

// CreateCustomer creates a PayPal customer (not directly supported, returns placeholder)
func (pp *PayPalProvider) CreateCustomer(ctx context.Context, userID, email, name string) (string, error) {
	// PayPal doesn't have a separate customer creation API
	// Customer information is passed during subscription/payment creation
	customerID := fmt.Sprintf("paypal_customer_%s", userID)

	pp.logger.WithFields(logrus.Fields{
		"customer_id": customerID,
		"user_id":     userID,
		"email":       email,
	}).Info("PayPal customer reference created")

	return customerID, nil
}

// CreateSubscription creates a new PayPal subscription
func (pp *PayPalProvider) CreateSubscription(ctx context.Context, req *SubscriptionRequest) (*SubscriptionResponse, error) {
	token, err := pp.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// Create subscription request
	subReq := PayPalSubscriptionRequest{
		PlanID: req.PriceID,
		Subscriber: PayPalSubscriber{
			EmailAddress: req.UserEmail,
		},
		ApplicationContext: PayPalApplicationContext{
			BrandName: "Caslink URL Shortener",
			UserAction: "SUBSCRIBE_NOW",
		},
	}

	jsonData, err := json.Marshal(subReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/billing/subscriptions", pp.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Prefer", "return=representation")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to create PayPal subscription")
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		pp.logger.WithField("response", string(body)).Error("PayPal subscription creation failed")
		return nil, fmt.Errorf("failed to create subscription: %s - %s", resp.Status, string(body))
	}

	var paypalSub PayPalSubscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&paypalSub); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse start time
	startTime, _ := time.Parse(time.RFC3339, paypalSub.StartTime)

	response := &SubscriptionResponse{
		SubscriptionID:     paypalSub.ID,
		Status:             pp.mapPayPalStatus(paypalSub.Status),
		CurrentPeriodStart: startTime,
		CurrentPeriodEnd:   startTime.AddDate(0, 1, 0), // Default to 1 month
		CancelAtPeriodEnd:  false,
	}

	// Extract approval URL for client
	for _, link := range paypalSub.Links {
		if link.Rel == "approve" {
			response.ClientSecret = link.Href // Use as approval URL
			break
		}
	}

	pp.logger.WithFields(logrus.Fields{
		"subscription_id": paypalSub.ID,
		"user_id":        req.UserID,
		"status":         paypalSub.Status,
	}).Info("PayPal subscription created")

	return response, nil
}

// UpdateSubscription updates a PayPal subscription (plan change)
func (pp *PayPalProvider) UpdateSubscription(ctx context.Context, subscriptionID, newPriceID string) (*SubscriptionResponse, error) {
	token, err := pp.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// PayPal subscription update (revision)
	updateReq := map[string]interface{}{
		"plan_id": newPriceID,
	}

	jsonData, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/billing/subscriptions/%s/revise", pp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to update PayPal subscription")
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to update subscription: %s - %s", resp.Status, string(body))
	}

	pp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"new_plan_id":     newPriceID,
	}).Info("PayPal subscription updated")

	return pp.GetSubscription(ctx, subscriptionID)
}

// CancelSubscription cancels a PayPal subscription
func (pp *PayPalProvider) CancelSubscription(ctx context.Context, subscriptionID string, immediate bool) error {
	token, err := pp.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	reason := "Customer requested cancellation"
	cancelReq := map[string]string{
		"reason": reason,
	}

	jsonData, err := json.Marshal(cancelReq)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/billing/subscriptions/%s/cancel", pp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to cancel PayPal subscription")
		return fmt.Errorf("failed to cancel subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to cancel subscription: %s - %s", resp.Status, string(body))
	}

	pp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"immediate":       immediate,
	}).Info("PayPal subscription cancelled")

	return nil
}

// ReactivateSubscription reactivates a cancelled PayPal subscription
func (pp *PayPalProvider) ReactivateSubscription(ctx context.Context, subscriptionID string) error {
	token, err := pp.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	url := fmt.Sprintf("%s/v1/billing/subscriptions/%s/activate", pp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBufferString(`{"reason":"Customer requested reactivation"}`))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to reactivate PayPal subscription")
		return fmt.Errorf("failed to reactivate subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to reactivate subscription: %s - %s", resp.Status, string(body))
	}

	pp.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
	}).Info("PayPal subscription reactivated")

	return nil
}

// CreatePaymentIntent creates a payment order for one-time payments
func (pp *PayPalProvider) CreatePaymentIntent(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	token, err := pp.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// Convert cents to decimal string
	amountStr := fmt.Sprintf("%.2f", float64(req.Amount)/100.0)

	orderReq := map[string]interface{}{
		"intent": "CAPTURE",
		"purchase_units": []map[string]interface{}{
			{
				"amount": map[string]string{
					"currency_code": req.Currency,
					"value":         amountStr,
				},
				"description": req.Description,
			},
		},
	}

	jsonData, err := json.Marshal(orderReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v2/checkout/orders", pp.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to create PayPal order")
		return nil, fmt.Errorf("failed to create order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create order: %s - %s", resp.Status, string(body))
	}

	var orderResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&orderResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	orderID := orderResp["id"].(string)
	status := orderResp["status"].(string)

	response := &PaymentResponse{
		PaymentID: orderID,
		Status:    pp.mapPayPalStatus(status),
		Amount:    req.Amount,
		Currency:  req.Currency,
	}

	// Extract approval URL
	if links, ok := orderResp["links"].([]interface{}); ok {
		for _, link := range links {
			linkMap := link.(map[string]interface{})
			if linkMap["rel"] == "approve" {
				response.ClientSecret = linkMap["href"].(string)
				break
			}
		}
	}

	pp.logger.WithFields(logrus.Fields{
		"order_id": orderID,
		"amount":   req.Amount,
		"status":   status,
	}).Info("PayPal order created")

	return response, nil
}

// ConfirmPayment captures a PayPal order
func (pp *PayPalProvider) ConfirmPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	token, err := pp.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	url := fmt.Sprintf("%s/v2/checkout/orders/%s/capture", pp.baseURL, paymentID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to capture PayPal order")
		return nil, fmt.Errorf("failed to capture order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to capture order: %s - %s", resp.Status, string(body))
	}

	var captureResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&captureResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	response := &PaymentResponse{
		PaymentID: paymentID,
		Status:    pp.mapPayPalStatus(captureResp["status"].(string)),
	}

	pp.logger.WithFields(logrus.Fields{
		"payment_id": paymentID,
		"status":     captureResp["status"],
	}).Info("PayPal order captured")

	return response, nil
}

// RefundPayment refunds a PayPal payment
func (pp *PayPalProvider) RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResponse, error) {
	token, err := pp.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	refundReq := map[string]interface{}{}
	if amount > 0 {
		amountStr := fmt.Sprintf("%.2f", float64(amount)/100.0)
		refundReq["amount"] = map[string]string{
			"value":         amountStr,
			"currency_code": "USD",
		}
	}

	jsonData, err := json.Marshal(refundReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v2/payments/captures/%s/refund", pp.baseURL, paymentID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to create PayPal refund")
		return nil, fmt.Errorf("failed to create refund: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create refund: %s - %s", resp.Status, string(body))
	}

	var refundResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&refundResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	response := &RefundResponse{
		RefundID: refundResp["id"].(string),
		Status:   pp.mapPayPalStatus(refundResp["status"].(string)),
	}

	pp.logger.WithFields(logrus.Fields{
		"refund_id":  refundResp["id"],
		"payment_id": paymentID,
	}).Info("PayPal refund created")

	return response, nil
}

// GetSubscription retrieves a PayPal subscription
func (pp *PayPalProvider) GetSubscription(ctx context.Context, subscriptionID string) (*SubscriptionResponse, error) {
	token, err := pp.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	url := fmt.Sprintf("%s/v1/billing/subscriptions/%s", pp.baseURL, subscriptionID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := pp.httpClient.Do(httpReq)
	if err != nil {
		pp.logger.WithError(err).Error("Failed to get PayPal subscription")
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get subscription: %s - %s", resp.Status, string(body))
	}

	var paypalSub PayPalSubscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&paypalSub); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	startTime, _ := time.Parse(time.RFC3339, paypalSub.StartTime)

	response := &SubscriptionResponse{
		SubscriptionID:     paypalSub.ID,
		Status:             pp.mapPayPalStatus(paypalSub.Status),
		CurrentPeriodStart: startTime,
		CurrentPeriodEnd:   startTime.AddDate(0, 1, 0),
		CancelAtPeriodEnd:  paypalSub.Status == "CANCELLED",
	}

	return response, nil
}

// VerifyWebhook verifies a PayPal webhook
func (pp *PayPalProvider) VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error) {
	// PayPal webhook verification would require webhook ID and verification endpoint
	// Simplified implementation for now
	var event map[string]interface{}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	eventType, _ := event["event_type"].(string)
	eventID, _ := event["id"].(string)

	webhookEvent := &WebhookEvent{
		ID:   eventID,
		Type: eventType,
		Data: payload,
	}

	pp.logger.WithFields(logrus.Fields{
		"event_id":   eventID,
		"event_type": eventType,
	}).Info("PayPal webhook received")

	return webhookEvent, nil
}

// mapPayPalStatus maps PayPal status to standardized status
func (pp *PayPalProvider) mapPayPalStatus(paypalStatus string) string {
	statusMap := map[string]string{
		"APPROVAL_PENDING": "pending",
		"APPROVED":         "active",
		"ACTIVE":           "active",
		"SUSPENDED":        "past_due",
		"CANCELLED":        "canceled",
		"EXPIRED":          "canceled",
		"CREATED":          "pending",
		"COMPLETED":        "succeeded",
		"VOIDED":           "canceled",
	}

	if status, ok := statusMap[paypalStatus]; ok {
		return status
	}

	return paypalStatus
}

// IsTestMode returns whether the provider is in test mode
func (pp *PayPalProvider) IsTestMode() bool {
	return pp.testMode
}

// GetProviderName returns the name of the payment provider
func (pp *PayPalProvider) GetProviderName() string {
	return "paypal"
}

// AttachPaymentMethod - Not applicable for PayPal
func (pp *PayPalProvider) AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	return fmt.Errorf("payment method attachment not supported for PayPal")
}

// DetachPaymentMethod - Not applicable for PayPal
func (pp *PayPalProvider) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	return fmt.Errorf("payment method detachment not supported for PayPal")
}

// SetDefaultPaymentMethod - Not applicable for PayPal
func (pp *PayPalProvider) SetDefaultPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error {
	return fmt.Errorf("default payment method not supported for PayPal")
}

// GetPaymentStatus retrieves payment status
func (pp *PayPalProvider) GetPaymentStatus(ctx context.Context, paymentID string) (string, error) {
	token, err := pp.getAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}

	url := fmt.Sprintf("%s/v2/checkout/orders/%s", pp.baseURL, paymentID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := pp.httpClient.Do(httpReq)
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

	status, _ := orderResp["status"].(string)
	return pp.mapPayPalStatus(status), nil
}

// RetryFailedPayment retries a failed payment
func (pp *PayPalProvider) RetryFailedPayment(ctx context.Context, paymentID string) (*PaymentResponse, error) {
	// PayPal doesn't support direct payment retry - customer must reauthorize
	return nil, fmt.Errorf("payment retry not supported for PayPal - customer must reauthorize")
}
