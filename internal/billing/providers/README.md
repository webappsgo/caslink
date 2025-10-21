# Payment Providers

This package contains implementations of various payment providers for the Caslink billing system.

## Overview

The billing providers implement a common `PaymentProvider` interface that abstracts payment processing, subscription management, and webhook handling across different payment platforms.

## Supported Providers

### 1. Stripe (`stripe.go`)
- **Description**: Global payment processing platform with comprehensive features
- **Website**: https://stripe.com
- **Capabilities**:
  - ✅ Subscriptions
  - ✅ One-time payments
  - ✅ Refunds
  - ✅ Payment methods management
  - ✅ Webhooks
  - ✅ Automatic retry
  - ✅ Customer portal
  - ✅ Invoice generation

**Configuration**:
```bash
CASLINK_BILLING_PROVIDER=stripe
CASLINK_BILLING_STRIPE_SECRET_KEY=sk_test_...
CASLINK_BILLING_STRIPE_WEBHOOK_SECRET=whsec_...
```

**Test Mode**: Detected automatically from API key prefix (`sk_test_`)

### 2. PayPal (`paypal.go`)
- **Description**: Popular payment platform with global reach
- **Website**: https://paypal.com
- **Capabilities**:
  - ✅ Subscriptions
  - ✅ One-time payments
  - ✅ Refunds
  - ✅ Webhooks
  - ✅ Customer portal
  - ❌ Payment methods management (handled through PayPal)
  - ❌ Automatic retry (customer must reauthorize)

**Configuration**:
```bash
CASLINK_BILLING_PROVIDER=paypal
CASLINK_BILLING_PAYPAL_CLIENT_ID=...
CASLINK_BILLING_PAYPAL_CLIENT_SECRET=...
```

**Test Mode**: Detected automatically from API endpoint (sandbox vs production)

### 3. Paddle (`paddle.go`)
- **Description**: Payment platform with built-in tax compliance
- **Website**: https://paddle.com
- **Capabilities**:
  - ✅ Subscriptions
  - ✅ One-time payments
  - ✅ Refunds
  - ✅ Webhooks
  - ✅ Automatic retry
  - ✅ Customer portal
  - ✅ Invoice generation
  - ❌ Payment methods management (handled through Paddle portal)

**Configuration**:
```bash
CASLINK_BILLING_PROVIDER=paddle
CASLINK_BILLING_PADDLE_API_KEY=...
CASLINK_BILLING_PADDLE_WEBHOOK_SECRET=...
```

**Test Mode**: Detected automatically from API key prefix (`test_`)

### 4. Lemon Squeezy (`lemonsqueezy.go`)
- **Description**: Modern payment platform for digital products
- **Website**: https://lemonsqueezy.com
- **Capabilities**:
  - ✅ Subscriptions
  - ✅ One-time payments
  - ✅ Webhooks
  - ✅ Automatic retry
  - ✅ Customer portal
  - ✅ Invoice generation
  - ❌ Refunds (must be done through dashboard)
  - ❌ Payment methods management (handled through portal)

**Configuration**:
```bash
CASLINK_BILLING_PROVIDER=lemonsqueezy
CASLINK_BILLING_LEMONSQUEEZY_API_KEY=...
CASLINK_BILLING_LEMONSQUEEZY_WEBHOOK_SECRET=...
```

**Test Mode**: Determined by store configuration

### 5. Manual/Enterprise (`manual.go`)
- **Description**: Manual billing for enterprise customers with custom payment arrangements
- **Use Cases**: Wire transfers, checks, enterprise contracts, custom invoicing
- **Capabilities**:
  - ✅ Subscriptions (manually managed)
  - ✅ One-time payments (manually recorded)
  - ✅ Refunds (manually processed)
  - ✅ Invoice generation
  - ❌ Automatic processing
  - ❌ Webhooks

**Configuration**:
```bash
CASLINK_BILLING_PROVIDER=manual
```

**Special Methods**:
- `MarkPaymentPaid()` - Manually mark payment as received
- `MarkPaymentFailed()` - Manually mark payment as failed
- `ExtendSubscription()` - Extend subscription period
- `SetSubscriptionStatus()` - Manually set subscription status
- `GenerateInvoice()` - Generate invoice reference
- `RecordPayment()` - Record payment receipt

## Interface

All providers implement the `PaymentProvider` interface:

```go
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
```

## Usage

### Creating a Provider

```go
import (
    "github.com/casjaysdevdocker/caslink/internal/billing/providers"
    "github.com/casjaysdevdocker/caslink/internal/config"
)

// Using the factory
provider, err := providers.NewProvider(cfg.Billing, logger)
if err != nil {
    log.Fatal(err)
}

// Direct instantiation
stripeProvider, err := providers.NewStripeProvider(secretKey, webhookSecret, logger)
```

### Creating a Subscription

```go
req := &providers.SubscriptionRequest{
    UserID:          "user_123",
    UserEmail:       "user@example.com",
    CustomerID:      "cus_xyz",
    PlanID:          "plan_pro",
    PriceID:         "price_abc",
    PaymentMethodID: "pm_card_123",
    TrialDays:       14,
}

subscription, err := provider.CreateSubscription(ctx, req)
if err != nil {
    return err
}

fmt.Printf("Subscription ID: %s\n", subscription.SubscriptionID)
fmt.Printf("Status: %s\n", subscription.Status)
```

### Processing a Payment

```go
req := &providers.PaymentRequest{
    UserID:          "user_123",
    CustomerID:      "cus_xyz",
    Amount:          9900, // $99.00 in cents
    Currency:        "USD",
    Description:     "Premium Plan - Annual",
    PaymentMethodID: "pm_card_123",
}

payment, err := provider.CreatePaymentIntent(ctx, req)
if err != nil {
    return err
}

// For providers that require confirmation
if payment.ClientSecret != "" {
    // Send client secret to frontend for confirmation
}
```

### Handling Webhooks

```go
// In your webhook handler
event, err := provider.VerifyWebhook(ctx, payload, signature)
if err != nil {
    http.Error(w, "Invalid signature", http.StatusBadRequest)
    return
}

// Process event based on type
switch event.Type {
case "customer.subscription.created":
    // Handle subscription created
case "customer.subscription.updated":
    // Handle subscription updated
case "invoice.payment_succeeded":
    // Handle successful payment
case "invoice.payment_failed":
    // Handle failed payment
}
```

## Status Mapping

All providers map their native statuses to standardized statuses:

- `pending` - Payment or subscription is pending
- `active` - Subscription is active and in good standing
- `trialing` - Subscription is in trial period
- `past_due` - Subscription has failed payment but not cancelled
- `canceled` - Subscription has been cancelled
- `succeeded` - Payment completed successfully
- `failed` - Payment failed

## Error Handling

All provider methods return errors following Go conventions. Providers log errors internally using structured logging (logrus).

```go
payment, err := provider.CreatePaymentIntent(ctx, req)
if err != nil {
    // Error already logged by provider
    return fmt.Errorf("failed to create payment: %w", err)
}
```

## Testing

Each provider can be tested independently:

```go
// Create test provider
provider, err := providers.NewStripeProvider(
    "sk_test_...", // Test API key
    "whsec_...",
    logger,
)

// Provider automatically detects test mode
if provider.IsTestMode() {
    fmt.Println("Running in test mode")
}
```

## Adding a New Provider

To add a new payment provider:

1. Create a new file `newprovider.go`
2. Implement the `PaymentProvider` interface
3. Add constructor function `NewNewProvider()`
4. Update `factory.go` to include the new provider
5. Add configuration fields to `config.BillingConfig`
6. Update documentation

Example skeleton:

```go
package providers

type NewProvider struct {
    apiKey string
    logger *logrus.Logger
}

func NewNewProvider(apiKey string, logger *logrus.Logger) (*NewProvider, error) {
    return &NewProvider{
        apiKey: apiKey,
        logger: logger,
    }, nil
}

// Implement all PaymentProvider interface methods
func (np *NewProvider) CreateCustomer(ctx context.Context, userID, email, name string) (string, error) {
    // Implementation
}

// ... implement remaining methods
```

## Security Considerations

1. **API Keys**: All API keys should be stored securely and never committed to version control
2. **Webhook Signatures**: Always verify webhook signatures before processing events
3. **Test Mode**: Use test mode credentials for development and testing
4. **Logging**: Sensitive data (API keys, payment details) are not logged
5. **HTTPS**: All provider communications use HTTPS

## Best Practices

1. **Error Handling**: Always check errors returned by provider methods
2. **Idempotency**: Use idempotency keys for payment operations when supported
3. **Retries**: Implement exponential backoff for failed API calls
4. **Webhooks**: Handle webhooks asynchronously and idempotently
5. **Testing**: Test with provider test modes before production deployment

## Provider-Specific Notes

### Stripe
- Most feature-complete provider
- Excellent documentation and developer tools
- Supports payment method updates, proration, and complex subscription scenarios
- Webhook endpoint format: `/api/v1/webhooks/stripe`

### PayPal
- Requires OAuth for API access
- Customer must approve subscriptions through PayPal interface
- Refunds have different policies based on transaction age
- Webhook endpoint format: `/api/v1/webhooks/paypal`

### Paddle
- Handles VAT/tax calculations automatically
- Uses checkout overlay for payments
- Provides customer portal for subscription management
- Webhook endpoint format: `/api/v1/webhooks/paddle`

### Lemon Squeezy
- Modern API with JSON:API specification
- Built-in store and product management
- Automatic tax calculation and compliance
- Webhook endpoint format: `/api/v1/webhooks/lemonsqueezy`

### Manual
- No external API calls
- All operations are database-only
- Suitable for enterprise contracts and custom payment arrangements
- Requires manual intervention for all payment operations

## License

This code is part of the Caslink project and follows the project's license terms.
