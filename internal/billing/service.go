package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Service coordinates billing operations
type Service struct {
	db       *db.DB
	config   *config.BillingConfig
	logger   *logrus.Logger
	plans    *PlanManager
	subs     *SubscriptionManager
	usage    *UsageTracker
	invoices *InvoiceManager
	payments *PaymentManager
	webhooks *WebhookHandler
	dunning  *DunningManager
}

// NewService creates a new billing service
func NewService(database *db.DB, cfg *config.BillingConfig, logger *logrus.Logger) (*Service, error) {
	if !cfg.Enabled {
		return &Service{
			db:     database,
			config: cfg,
			logger: logger,
		}, nil
	}

	service := &Service{
		db:     database,
		config: cfg,
		logger: logger,
	}

	// Initialize components
	var err error

	service.plans, err = NewPlanManager(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create plan manager: %w", err)
	}

	service.subs, err = NewSubscriptionManager(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription manager: %w", err)
	}

	service.usage, err = NewUsageTracker(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create usage tracker: %w", err)
	}

	service.invoices, err = NewInvoiceManager(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create invoice manager: %w", err)
	}

	service.payments, err = NewPaymentManager(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment manager: %w", err)
	}

	service.webhooks, err = NewWebhookHandler(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook handler: %w", err)
	}

	service.dunning, err = NewDunningManager(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create dunning manager: %w", err)
	}

	return service, nil
}

// IsEnabled returns whether billing is enabled
func (s *Service) IsEnabled() bool {
	return s.config.Enabled
}

// GetPlans returns the plan manager
func (s *Service) GetPlans() *PlanManager {
	return s.plans
}

// GetSubscriptions returns the subscription manager
func (s *Service) GetSubscriptions() *SubscriptionManager {
	return s.subs
}

// GetUsage returns the usage tracker
func (s *Service) GetUsage() *UsageTracker {
	return s.usage
}

// GetInvoices returns the invoice manager
func (s *Service) GetInvoices() *InvoiceManager {
	return s.invoices
}

// GetPayments returns the payment manager
func (s *Service) GetPayments() *PaymentManager {
	return s.payments
}

// GetWebhooks returns the webhook handler
func (s *Service) GetWebhooks() *WebhookHandler {
	return s.webhooks
}

// GetDunning returns the dunning manager
func (s *Service) GetDunning() *DunningManager {
	return s.dunning
}

// CreateSubscription creates a new subscription for a user
func (s *Service) CreateSubscription(ctx context.Context, userID, planID string, paymentMethodID string) (*Subscription, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("billing is not enabled")
	}

	// Get plan
	plan, err := s.plans.GetPlan(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	// Create subscription
	subscription, err := s.subs.CreateSubscription(ctx, userID, plan, paymentMethodID)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":         userID,
		"plan_id":         planID,
		"subscription_id": subscription.ID,
	}).Info("Subscription created")

	return subscription, nil
}

// CancelSubscription cancels a user's subscription
func (s *Service) CancelSubscription(ctx context.Context, userID, subscriptionID string) error {
	if !s.IsEnabled() {
		return fmt.Errorf("billing is not enabled")
	}

	err := s.subs.CancelSubscription(ctx, subscriptionID, userID)
	if err != nil {
		return fmt.Errorf("failed to cancel subscription: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":         userID,
		"subscription_id": subscriptionID,
	}).Info("Subscription cancelled")

	return nil
}

// GetUserSubscription gets the active subscription for a user
func (s *Service) GetUserSubscription(ctx context.Context, userID string) (*Subscription, error) {
	if !s.IsEnabled() {
		return nil, nil // No subscription when billing disabled
	}

	return s.subs.GetUserSubscription(ctx, userID)
}

// TrackUsage records usage for billing
func (s *Service) TrackUsage(ctx context.Context, userID, metric string, quantity int64) error {
	if !s.IsEnabled() {
		return nil // No tracking when billing disabled
	}

	return s.usage.RecordUsage(ctx, userID, metric, quantity)
}

// CheckUsageLimits checks if user has exceeded usage limits
func (s *Service) CheckUsageLimits(ctx context.Context, userID, metric string) (*UsageStatus, error) {
	if !s.IsEnabled() {
		return &UsageStatus{
			Allowed:   true,
			Unlimited: true,
		}, nil
	}

	return s.usage.CheckLimits(ctx, userID, metric)
}

// GetUserUsage gets usage statistics for a user
func (s *Service) GetUserUsage(ctx context.Context, userID string, period time.Time) (*UsageSummary, error) {
	if !s.IsEnabled() {
		return &UsageSummary{}, nil
	}

	return s.usage.GetUsageSummary(ctx, userID, period)
}

// ProcessPayment processes a payment
func (s *Service) ProcessPayment(ctx context.Context, userID string, amount int64, currency string) (*Payment, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("billing is not enabled")
	}

	return s.payments.ProcessPayment(ctx, userID, amount, currency)
}

// HandleWebhook handles payment provider webhooks
func (s *Service) HandleWebhook(ctx context.Context, provider string, payload []byte, signature string) error {
	if !s.IsEnabled() {
		return fmt.Errorf("billing is not enabled")
	}

	return s.webhooks.HandleWebhook(ctx, provider, payload, signature)
}

// GetUserInvoices gets invoices for a user
func (s *Service) GetUserInvoices(ctx context.Context, userID string, limit, offset int) ([]*Invoice, error) {
	if !s.IsEnabled() {
		return []*Invoice{}, nil
	}

	return s.invoices.GetUserInvoices(ctx, userID, limit, offset)
}

// GenerateInvoice generates an invoice for a subscription
func (s *Service) GenerateInvoice(ctx context.Context, subscriptionID string) (*Invoice, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("billing is not enabled")
	}

	return s.invoices.GenerateInvoice(ctx, subscriptionID)
}

// GetBillingStats gets billing statistics
func (s *Service) GetBillingStats(ctx context.Context) (*BillingStats, error) {
	if !s.IsEnabled() {
		return &BillingStats{}, nil
	}

	stats := &BillingStats{}

	// Get subscription stats
	subStats, err := s.subs.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription stats: %w", err)
	}
	stats.Subscriptions = subStats

	// Get revenue stats
	revenueStats, err := s.payments.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get revenue stats: %w", err)
	}
	stats.Revenue = revenueStats

	// Get usage stats
	usageStats, err := s.usage.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage stats: %w", err)
	}
	stats.Usage = usageStats

	return stats, nil
}

// RunDunning runs the dunning process for failed payments
func (s *Service) RunDunning(ctx context.Context) error {
	if !s.IsEnabled() {
		return nil
	}

	return s.dunning.ProcessFailedPayments(ctx)
}

// BillingStats represents billing statistics
type BillingStats struct {
	Subscriptions *SubscriptionStats `json:"subscriptions"`
	Revenue       *RevenueStats      `json:"revenue"`
	Usage         *UsageStats        `json:"usage"`
}

// SubscriptionStats represents subscription statistics
type SubscriptionStats struct {
	Active    int64 `json:"active"`
	Trialing  int64 `json:"trialing"`
	PastDue   int64 `json:"past_due"`
	Canceled  int64 `json:"canceled"`
	Total     int64 `json:"total"`
}

// RevenueStats represents revenue statistics
type RevenueStats struct {
	MonthlyRecurringRevenue int64                    `json:"monthly_recurring_revenue"`
	AnnualRecurringRevenue  int64                    `json:"annual_recurring_revenue"`
	TotalRevenue           int64                    `json:"total_revenue"`
	RevenueByPlan          map[string]int64         `json:"revenue_by_plan"`
	RevenueByMonth         map[string]int64         `json:"revenue_by_month"`
}

// UsageStats represents usage statistics
type UsageStats struct {
	TotalAPIRequests     int64            `json:"total_api_requests"`
	TotalURLsCreated     int64            `json:"total_urls_created"`
	TotalQRCodes         int64            `json:"total_qr_codes"`
	UsageByMetric        map[string]int64 `json:"usage_by_metric"`
	TopUsageUsers        []UserUsage      `json:"top_usage_users"`
}

// UserUsage represents usage by a specific user
type UserUsage struct {
	UserID string `json:"user_id"`
	Usage  int64  `json:"usage"`
}

// UsageStatus represents current usage status
type UsageStatus struct {
	Allowed     bool  `json:"allowed"`
	Unlimited   bool  `json:"unlimited"`
	Used        int64 `json:"used"`
	Limit       int64 `json:"limit"`
	Remaining   int64 `json:"remaining"`
	ResetAt     *time.Time `json:"reset_at,omitempty"`
}

// UsageSummary represents usage summary for a user
type UsageSummary struct {
	UserID  string                 `json:"user_id"`
	Period  time.Time              `json:"period"`
	Metrics map[string]UsageMetric `json:"metrics"`
}

// UsageMetric represents usage for a specific metric
type UsageMetric struct {
	Used      int64 `json:"used"`
	Limit     int64 `json:"limit"`
	Unlimited bool  `json:"unlimited"`
}