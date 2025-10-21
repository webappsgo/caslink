package providers

import (
	"context"
	"time"
)

// PaymentProvider defines the interface that all payment providers must implement
type PaymentProvider interface {
	// Customer Management
	CreateCustomer(ctx context.Context, userID, email, name string) (string, error)

	// Subscription Management
	CreateSubscription(ctx context.Context, req *SubscriptionRequest) (*SubscriptionResponse, error)
	UpdateSubscription(ctx context.Context, subscriptionID, newPriceID string) (*SubscriptionResponse, error)
	CancelSubscription(ctx context.Context, subscriptionID string, immediate bool) error
	ReactivateSubscription(ctx context.Context, subscriptionID string) error
	GetSubscription(ctx context.Context, subscriptionID string) (*SubscriptionResponse, error)

	// Payment Processing
	CreatePaymentIntent(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error)
	ConfirmPayment(ctx context.Context, paymentID string) (*PaymentResponse, error)
	RefundPayment(ctx context.Context, paymentID string, amount int64) (*RefundResponse, error)
	GetPaymentStatus(ctx context.Context, paymentID string) (string, error)
	RetryFailedPayment(ctx context.Context, paymentID string) (*PaymentResponse, error)

	// Payment Method Management
	AttachPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error
	DetachPaymentMethod(ctx context.Context, paymentMethodID string) error
	SetDefaultPaymentMethod(ctx context.Context, customerID, paymentMethodID string) error

	// Webhook Handling
	VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error)

	// Provider Information
	GetProviderName() string
	IsTestMode() bool
}

// SubscriptionRequest represents a subscription creation request
type SubscriptionRequest struct {
	UserID          string
	UserEmail       string
	CustomerID      string
	PlanID          string
	PriceID         string
	PaymentMethodID string
	TrialDays       int
	Metadata        map[string]string
}

// SubscriptionResponse represents a subscription response
type SubscriptionResponse struct {
	SubscriptionID     string
	Status             string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	TrialStart         *time.Time
	TrialEnd           *time.Time
	CancelAtPeriodEnd  bool
	LatestInvoiceID    string
	ClientSecret       string
}

// PaymentRequest represents a payment creation request
type PaymentRequest struct {
	UserID          string
	CustomerID      string
	Amount          int64
	Currency        string
	Description     string
	PaymentMethodID string
	Metadata        map[string]string
}

// PaymentResponse represents a payment response
type PaymentResponse struct {
	PaymentID    string
	Status       string
	ClientSecret string
	Amount       int64
	Currency     string
	FailureCode  string
	FailureMessage string
}

// RefundResponse represents a refund response
type RefundResponse struct {
	RefundID string
	Status   string
	Amount   int64
	Currency string
	Reason   string
}

// WebhookEvent represents a webhook event
type WebhookEvent struct {
	ID   string
	Type string
	Data []byte
}

// ProviderConfig represents provider configuration
type ProviderConfig struct {
	Provider      string
	APIKey        string
	APISecret     string
	WebhookSecret string
	TestMode      bool
	Metadata      map[string]string
}
