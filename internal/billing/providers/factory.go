package providers

import (
	"fmt"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
)

// NewProvider creates a payment provider based on configuration
// This factory method instantiates the appropriate provider implementation
// based on the billing configuration settings.
func NewProvider(cfg *config.BillingConfig, logger *logrus.Logger) (PaymentProvider, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("billing is not enabled")
	}

	switch cfg.Provider {
	case "stripe":
		if cfg.StripeSecretKey == "" {
			return nil, fmt.Errorf("stripe secret key is required")
		}
		return NewStripeProvider(cfg.StripeSecretKey, cfg.StripeWebhookSecret, logger)

	case "paypal":
		if cfg.PayPalClientID == "" || cfg.PayPalClientSecret == "" {
			return nil, fmt.Errorf("paypal client ID and secret are required")
		}
		return NewPayPalProvider(cfg.PayPalClientID, cfg.PayPalClientSecret, logger)

	case "paddle":
		// Paddle configuration would need to be added to BillingConfig
		// For now, using a placeholder
		apiKey := "" // Would come from cfg.PaddleAPIKey
		webhookSecret := "" // Would come from cfg.PaddleWebhookSecret

		if apiKey == "" {
			return nil, fmt.Errorf("paddle API key is required")
		}
		return NewPaddleProvider(apiKey, webhookSecret, logger)

	case "lemonsqueezy":
		// LemonSqueezy configuration would need to be added to BillingConfig
		apiKey := "" // Would come from cfg.LemonSqueezyAPIKey
		webhookSecret := "" // Would come from cfg.LemonSqueezyWebhookSecret

		if apiKey == "" {
			return nil, fmt.Errorf("lemonsqueezy API key is required")
		}
		return NewLemonSqueezyProvider(apiKey, webhookSecret, logger)

	case "manual", "enterprise":
		return NewManualProvider(logger)

	case "none", "":
		return nil, fmt.Errorf("no payment provider configured")

	default:
		return nil, fmt.Errorf("unsupported payment provider: %s", cfg.Provider)
	}
}

// ValidateProviderConfig validates provider configuration before initialization
func ValidateProviderConfig(cfg *config.BillingConfig) error {
	if !cfg.Enabled {
		return nil // No validation needed if billing is disabled
	}

	switch cfg.Provider {
	case "stripe":
		if cfg.StripeSecretKey == "" {
			return fmt.Errorf("stripe secret key is required")
		}
		// Validate key format
		if len(cfg.StripeSecretKey) < 10 {
			return fmt.Errorf("invalid stripe secret key format")
		}
		// Recommend webhook secret
		if cfg.StripeWebhookSecret == "" {
			// Warning, not error - webhooks are optional but recommended
		}

	case "paypal":
		if cfg.PayPalClientID == "" {
			return fmt.Errorf("paypal client ID is required")
		}
		if cfg.PayPalClientSecret == "" {
			return fmt.Errorf("paypal client secret is required")
		}

	case "paddle":
		// Paddle validation would go here
		// For now, return nil since config fields aren't defined yet

	case "lemonsqueezy":
		// LemonSqueezy validation would go here
		// For now, return nil since config fields aren't defined yet

	case "manual", "enterprise":
		// Manual billing doesn't require external credentials
		return nil

	case "none", "":
		return fmt.Errorf("billing provider must be specified when billing is enabled")

	default:
		return fmt.Errorf("unsupported payment provider: %s", cfg.Provider)
	}

	return nil
}

// GetSupportedProviders returns a list of supported payment providers
func GetSupportedProviders() []string {
	return []string{
		"stripe",
		"paypal",
		"paddle",
		"lemonsqueezy",
		"manual",
	}
}

// GetProviderCapabilities returns the capabilities of a specific provider
func GetProviderCapabilities(provider string) map[string]bool {
	capabilities := map[string]bool{
		"subscriptions":        false,
		"one_time_payments":    false,
		"refunds":              false,
		"payment_methods":      false,
		"webhooks":             false,
		"automatic_retry":      false,
		"customer_portal":      false,
		"invoice_generation":   false,
	}

	switch provider {
	case "stripe":
		capabilities["subscriptions"] = true
		capabilities["one_time_payments"] = true
		capabilities["refunds"] = true
		capabilities["payment_methods"] = true
		capabilities["webhooks"] = true
		capabilities["automatic_retry"] = true
		capabilities["customer_portal"] = true
		capabilities["invoice_generation"] = true

	case "paypal":
		capabilities["subscriptions"] = true
		capabilities["one_time_payments"] = true
		capabilities["refunds"] = true
		capabilities["webhooks"] = true
		capabilities["automatic_retry"] = false
		capabilities["customer_portal"] = true
		capabilities["invoice_generation"] = false

	case "paddle":
		capabilities["subscriptions"] = true
		capabilities["one_time_payments"] = true
		capabilities["refunds"] = true
		capabilities["webhooks"] = true
		capabilities["automatic_retry"] = true
		capabilities["customer_portal"] = true
		capabilities["invoice_generation"] = true

	case "lemonsqueezy":
		capabilities["subscriptions"] = true
		capabilities["one_time_payments"] = true
		capabilities["refunds"] = false // Must be done through dashboard
		capabilities["webhooks"] = true
		capabilities["automatic_retry"] = true
		capabilities["customer_portal"] = true
		capabilities["invoice_generation"] = true

	case "manual":
		capabilities["subscriptions"] = true
		capabilities["one_time_payments"] = true
		capabilities["refunds"] = true
		capabilities["invoice_generation"] = true
		// All other capabilities are manually managed
	}

	return capabilities
}

// ProviderInfo contains information about a payment provider
type ProviderInfo struct {
	Name         string
	DisplayName  string
	Description  string
	Website      string
	Capabilities map[string]bool
	RequiredConfig []string
}

// GetProviderInfo returns detailed information about a provider
func GetProviderInfo(provider string) *ProviderInfo {
	switch provider {
	case "stripe":
		return &ProviderInfo{
			Name:         "stripe",
			DisplayName:  "Stripe",
			Description:  "Global payment processing platform with comprehensive features",
			Website:      "https://stripe.com",
			Capabilities: GetProviderCapabilities("stripe"),
			RequiredConfig: []string{
				"CASLINK_BILLING_STRIPE_SECRET_KEY",
				"CASLINK_BILLING_STRIPE_WEBHOOK_SECRET (recommended)",
			},
		}

	case "paypal":
		return &ProviderInfo{
			Name:         "paypal",
			DisplayName:  "PayPal",
			Description:  "Popular payment platform with global reach",
			Website:      "https://paypal.com",
			Capabilities: GetProviderCapabilities("paypal"),
			RequiredConfig: []string{
				"CASLINK_BILLING_PAYPAL_CLIENT_ID",
				"CASLINK_BILLING_PAYPAL_CLIENT_SECRET",
			},
		}

	case "paddle":
		return &ProviderInfo{
			Name:         "paddle",
			DisplayName:  "Paddle",
			Description:  "Payment platform with built-in tax compliance",
			Website:      "https://paddle.com",
			Capabilities: GetProviderCapabilities("paddle"),
			RequiredConfig: []string{
				"CASLINK_BILLING_PADDLE_API_KEY",
				"CASLINK_BILLING_PADDLE_WEBHOOK_SECRET (recommended)",
			},
		}

	case "lemonsqueezy":
		return &ProviderInfo{
			Name:         "lemonsqueezy",
			DisplayName:  "Lemon Squeezy",
			Description:  "Modern payment platform for digital products",
			Website:      "https://lemonsqueezy.com",
			Capabilities: GetProviderCapabilities("lemonsqueezy"),
			RequiredConfig: []string{
				"CASLINK_BILLING_LEMONSQUEEZY_API_KEY",
				"CASLINK_BILLING_LEMONSQUEEZY_WEBHOOK_SECRET (recommended)",
			},
		}

	case "manual":
		return &ProviderInfo{
			Name:         "manual",
			DisplayName:  "Manual/Enterprise Billing",
			Description:  "Manual billing for enterprise customers with custom payment arrangements",
			Website:      "",
			Capabilities: GetProviderCapabilities("manual"),
			RequiredConfig: []string{},
		}

	default:
		return nil
	}
}
