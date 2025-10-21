package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// SubscriptionManager handles subscription lifecycle management
type SubscriptionManager struct {
	db     *db.DB
	config *config.BillingConfig
	logger *logrus.Logger
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager(database *db.DB, cfg *config.BillingConfig, logger *logrus.Logger) (*SubscriptionManager, error) {
	return &SubscriptionManager{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// SubscriptionStatus represents subscription status
type SubscriptionStatus string

const (
	StatusTrialing  SubscriptionStatus = "trialing"
	StatusActive    SubscriptionStatus = "active"
	StatusPastDue   SubscriptionStatus = "past_due"
	StatusCanceled  SubscriptionStatus = "canceled"
	StatusUnpaid    SubscriptionStatus = "unpaid"
)

// BillingCycle represents billing cycle
type BillingCycle string

const (
	CycleMonthly BillingCycle = "monthly"
	CycleYearly  BillingCycle = "yearly"
)

// Subscription represents a user subscription
type Subscription struct {
	ID                     string             `json:"id" db:"id"`
	UserID                 string             `json:"user_id" db:"user_id"`
	PlanID                 string             `json:"plan_id" db:"plan_id"`
	ProviderSubscriptionID string             `json:"provider_subscription_id" db:"provider_subscription_id"`
	Status                 SubscriptionStatus `json:"status" db:"status"`
	BillingCycle           BillingCycle       `json:"billing_cycle" db:"billing_cycle"`
	CurrentPeriodStart     time.Time          `json:"current_period_start" db:"current_period_start"`
	CurrentPeriodEnd       time.Time          `json:"current_period_end" db:"current_period_end"`
	TrialStart             *time.Time         `json:"trial_start" db:"trial_start"`
	TrialEnd               *time.Time         `json:"trial_end" db:"trial_end"`
	CancelAtPeriodEnd      bool               `json:"cancel_at_period_end" db:"cancel_at_period_end"`
	CanceledAt             *time.Time         `json:"canceled_at" db:"canceled_at"`
	CreatedAt              time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time          `json:"updated_at" db:"updated_at"`

	// Joined fields
	Plan *Plan `json:"plan,omitempty"`
}

// CreateSubscription creates a new subscription
func (sm *SubscriptionManager) CreateSubscription(ctx context.Context, userID string, plan *Plan, paymentMethodID string) (*Subscription, error) {
	now := time.Now()
	subscriptionID := sm.generateSubscriptionID()

	subscription := &Subscription{
		ID:                 subscriptionID,
		UserID:             userID,
		PlanID:             plan.ID,
		Status:             StatusTrialing,
		BillingCycle:       CycleMonthly, // Default to monthly
		CurrentPeriodStart: now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// Set trial period if applicable
	if plan.TrialDays > 0 {
		trialStart := now
		trialEnd := now.AddDate(0, 0, plan.TrialDays)
		subscription.TrialStart = &trialStart
		subscription.TrialEnd = &trialEnd
		subscription.CurrentPeriodEnd = trialEnd
	} else {
		subscription.Status = StatusActive
		subscription.CurrentPeriodEnd = now.AddDate(0, 1, 0) // 1 month
	}

	// Create provider subscription if payment required
	if plan.PriceMonthly > 0 {
		providerSubID, err := sm.createProviderSubscription(ctx, subscription, plan, paymentMethodID)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider subscription: %w", err)
		}
		subscription.ProviderSubscriptionID = providerSubID
	}

	// Save to database
	query := `
		INSERT INTO subscriptions (
			id, user_id, plan_id, provider_subscription_id, status, billing_cycle,
			current_period_start, current_period_end, trial_start, trial_end,
			cancel_at_period_end, canceled_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := sm.db.ExecContext(ctx, query,
		subscription.ID, subscription.UserID, subscription.PlanID,
		subscription.ProviderSubscriptionID, subscription.Status, subscription.BillingCycle,
		subscription.CurrentPeriodStart, subscription.CurrentPeriodEnd,
		subscription.TrialStart, subscription.TrialEnd,
		subscription.CancelAtPeriodEnd, subscription.CanceledAt,
		subscription.CreatedAt, subscription.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to save subscription: %w", err)
	}

	sm.logger.WithFields(logrus.Fields{
		"subscription_id": subscription.ID,
		"user_id":         userID,
		"plan_id":         plan.ID,
		"status":          subscription.Status,
	}).Info("Subscription created")

	return subscription, nil
}

// GetUserSubscription gets the active subscription for a user
func (sm *SubscriptionManager) GetUserSubscription(ctx context.Context, userID string) (*Subscription, error) {
	query := `
		SELECT s.id, s.user_id, s.plan_id, s.provider_subscription_id, s.status,
		       s.billing_cycle, s.current_period_start, s.current_period_end,
		       s.trial_start, s.trial_end, s.cancel_at_period_end, s.canceled_at,
		       s.created_at, s.updated_at,
		       p.name, p.display_name, p.description, p.price_monthly, p.price_yearly,
		       p.currency, p.features, p.limits, p.trial_days, p.active
		FROM subscriptions s
		JOIN billing_plans p ON s.plan_id = p.id
		WHERE s.user_id = ? AND s.status IN ('trialing', 'active', 'past_due')
		ORDER BY s.created_at DESC
		LIMIT 1`

	row := sm.db.QueryRowContext(ctx, query, userID)

	subscription, err := sm.scanSubscriptionWithPlan(row)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil // No active subscription
		}
		return nil, fmt.Errorf("failed to get user subscription: %w", err)
	}

	return subscription, nil
}

// GetSubscription gets a subscription by ID
func (sm *SubscriptionManager) GetSubscription(ctx context.Context, subscriptionID string) (*Subscription, error) {
	query := `
		SELECT s.id, s.user_id, s.plan_id, s.provider_subscription_id, s.status,
		       s.billing_cycle, s.current_period_start, s.current_period_end,
		       s.trial_start, s.trial_end, s.cancel_at_period_end, s.canceled_at,
		       s.created_at, s.updated_at,
		       p.name, p.display_name, p.description, p.price_monthly, p.price_yearly,
		       p.currency, p.features, p.limits, p.trial_days, p.active
		FROM subscriptions s
		JOIN billing_plans p ON s.plan_id = p.id
		WHERE s.id = ?`

	row := sm.db.QueryRowContext(ctx, query, subscriptionID)

	subscription, err := sm.scanSubscriptionWithPlan(row)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return subscription, nil
}

// UpdateSubscription updates a subscription
func (sm *SubscriptionManager) UpdateSubscription(ctx context.Context, subscription *Subscription) error {
	subscription.UpdatedAt = time.Now()

	query := `
		UPDATE subscriptions SET
			status = ?, billing_cycle = ?, current_period_start = ?, current_period_end = ?,
			trial_start = ?, trial_end = ?, cancel_at_period_end = ?, canceled_at = ?,
			updated_at = ?
		WHERE id = ?`

	_, err := sm.db.ExecContext(ctx, query,
		subscription.Status, subscription.BillingCycle,
		subscription.CurrentPeriodStart, subscription.CurrentPeriodEnd,
		subscription.TrialStart, subscription.TrialEnd,
		subscription.CancelAtPeriodEnd, subscription.CanceledAt,
		subscription.UpdatedAt, subscription.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	sm.logger.WithFields(logrus.Fields{
		"subscription_id": subscription.ID,
		"status":          subscription.Status,
	}).Info("Subscription updated")

	return nil
}

// CancelSubscription cancels a subscription
func (sm *SubscriptionManager) CancelSubscription(ctx context.Context, subscriptionID, userID string) error {
	subscription, err := sm.GetSubscription(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	if subscription.UserID != userID {
		return fmt.Errorf("subscription does not belong to user")
	}

	// Cancel at provider if needed
	if subscription.ProviderSubscriptionID != "" {
		if err := sm.cancelProviderSubscription(ctx, subscription.ProviderSubscriptionID); err != nil {
			sm.logger.WithError(err).Warn("Failed to cancel provider subscription")
		}
	}

	// Update subscription
	now := time.Now()
	subscription.CancelAtPeriodEnd = true
	subscription.CanceledAt = &now
	subscription.Status = StatusCanceled

	return sm.UpdateSubscription(ctx, subscription)
}

// ReactivateSubscription reactivates a canceled subscription
func (sm *SubscriptionManager) ReactivateSubscription(ctx context.Context, subscriptionID, userID string) error {
	subscription, err := sm.GetSubscription(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	if subscription.UserID != userID {
		return fmt.Errorf("subscription does not belong to user")
	}

	if subscription.Status != StatusCanceled {
		return fmt.Errorf("subscription is not canceled")
	}

	// Reactivate at provider if needed
	if subscription.ProviderSubscriptionID != "" {
		if err := sm.reactivateProviderSubscription(ctx, subscription.ProviderSubscriptionID); err != nil {
			return fmt.Errorf("failed to reactivate provider subscription: %w", err)
		}
	}

	// Update subscription
	subscription.CancelAtPeriodEnd = false
	subscription.CanceledAt = nil
	subscription.Status = StatusActive

	return sm.UpdateSubscription(ctx, subscription)
}

// ChangePlan changes the plan for a subscription
func (sm *SubscriptionManager) ChangePlan(ctx context.Context, subscriptionID, userID, newPlanID string) error {
	subscription, err := sm.GetSubscription(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	if subscription.UserID != userID {
		return fmt.Errorf("subscription does not belong to user")
	}

	// Get new plan
	planManager := &PlanManager{db: sm.db, config: sm.config, logger: sm.logger}
	newPlan, err := planManager.GetPlan(ctx, newPlanID)
	if err != nil {
		return fmt.Errorf("failed to get new plan: %w", err)
	}

	// Update at provider if needed
	if subscription.ProviderSubscriptionID != "" {
		if err := sm.changeProviderPlan(ctx, subscription.ProviderSubscriptionID, newPlan); err != nil {
			return fmt.Errorf("failed to change provider plan: %w", err)
		}
	}

	// Update subscription
	subscription.PlanID = newPlanID

	if err := sm.UpdateSubscription(ctx, subscription); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	sm.logger.WithFields(logrus.Fields{
		"subscription_id": subscriptionID,
		"old_plan_id":     subscription.PlanID,
		"new_plan_id":     newPlanID,
	}).Info("Subscription plan changed")

	return nil
}

// GetUserSubscriptions gets all subscriptions for a user
func (sm *SubscriptionManager) GetUserSubscriptions(ctx context.Context, userID string, limit, offset int) ([]*Subscription, error) {
	query := `
		SELECT s.id, s.user_id, s.plan_id, s.provider_subscription_id, s.status,
		       s.billing_cycle, s.current_period_start, s.current_period_end,
		       s.trial_start, s.trial_end, s.cancel_at_period_end, s.canceled_at,
		       s.created_at, s.updated_at,
		       p.name, p.display_name, p.description, p.price_monthly, p.price_yearly,
		       p.currency, p.features, p.limits, p.trial_days, p.active
		FROM subscriptions s
		JOIN billing_plans p ON s.plan_id = p.id
		WHERE s.user_id = ?
		ORDER BY s.created_at DESC
		LIMIT ? OFFSET ?`

	rows, err := sm.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query user subscriptions: %w", err)
	}
	defer rows.Close()

	var subscriptions []*Subscription
	for rows.Next() {
		subscription, err := sm.scanSubscriptionWithPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subscription: %w", err)
		}
		subscriptions = append(subscriptions, subscription)
	}

	return subscriptions, nil
}

// GetStats returns subscription statistics
func (sm *SubscriptionManager) GetStats(ctx context.Context) (*SubscriptionStats, error) {
	stats := &SubscriptionStats{}

	query := `
		SELECT status, COUNT(*) as count
		FROM subscriptions
		GROUP BY status`

	rows, err := sm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscription stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan stats: %w", err)
		}

		switch SubscriptionStatus(status) {
		case StatusActive:
			stats.Active = count
		case StatusTrialing:
			stats.Trialing = count
		case StatusPastDue:
			stats.PastDue = count
		case StatusCanceled:
			stats.Canceled = count
		}

		stats.Total += count
	}

	return stats, nil
}

// Helper methods for provider integration

func (sm *SubscriptionManager) createProviderSubscription(ctx context.Context, subscription *Subscription, plan *Plan, paymentMethodID string) (string, error) {
	switch sm.config.Provider {
	case "stripe":
		return sm.createStripeSubscription(ctx, subscription, plan, paymentMethodID)
	case "manual":
		return "", nil // No provider subscription needed for manual billing
	default:
		return "", fmt.Errorf("unsupported billing provider: %s", sm.config.Provider)
	}
}

func (sm *SubscriptionManager) cancelProviderSubscription(ctx context.Context, providerSubID string) error {
	switch sm.config.Provider {
	case "stripe":
		return sm.cancelStripeSubscription(ctx, providerSubID)
	case "manual":
		return nil // No action needed for manual billing
	default:
		return fmt.Errorf("unsupported billing provider: %s", sm.config.Provider)
	}
}

func (sm *SubscriptionManager) reactivateProviderSubscription(ctx context.Context, providerSubID string) error {
	switch sm.config.Provider {
	case "stripe":
		return sm.reactivateStripeSubscription(ctx, providerSubID)
	case "manual":
		return nil // No action needed for manual billing
	default:
		return fmt.Errorf("unsupported billing provider: %s", sm.config.Provider)
	}
}

func (sm *SubscriptionManager) changeProviderPlan(ctx context.Context, providerSubID string, newPlan *Plan) error {
	switch sm.config.Provider {
	case "stripe":
		return sm.changeStripePlan(ctx, providerSubID, newPlan)
	case "manual":
		return nil // No action needed for manual billing
	default:
		return fmt.Errorf("unsupported billing provider: %s", sm.config.Provider)
	}
}

// Provider-specific implementations (would be in separate files)

func (sm *SubscriptionManager) createStripeSubscription(ctx context.Context, subscription *Subscription, plan *Plan, paymentMethodID string) (string, error) {
	// Implementation would use Stripe API
	// For now, return a mock ID
	return fmt.Sprintf("stripe_sub_%d", time.Now().UnixNano()), nil
}

func (sm *SubscriptionManager) cancelStripeSubscription(ctx context.Context, providerSubID string) error {
	// Implementation would use Stripe API
	return nil
}

func (sm *SubscriptionManager) reactivateStripeSubscription(ctx context.Context, providerSubID string) error {
	// Implementation would use Stripe API
	return nil
}

func (sm *SubscriptionManager) changeStripePlan(ctx context.Context, providerSubID string, newPlan *Plan) error {
	// Implementation would use Stripe API
	return nil
}

// Helper methods

func (sm *SubscriptionManager) generateSubscriptionID() string {
	return fmt.Sprintf("sub_%d", time.Now().UnixNano())
}

func (sm *SubscriptionManager) scanSubscriptionWithPlan(scanner interface{ Scan(...interface{}) error }) (*Subscription, error) {
	subscription := &Subscription{}
	plan := &Plan{}

	var featuresJSON, limitsJSON string

	err := scanner.Scan(
		&subscription.ID, &subscription.UserID, &subscription.PlanID,
		&subscription.ProviderSubscriptionID, &subscription.Status, &subscription.BillingCycle,
		&subscription.CurrentPeriodStart, &subscription.CurrentPeriodEnd,
		&subscription.TrialStart, &subscription.TrialEnd,
		&subscription.CancelAtPeriodEnd, &subscription.CanceledAt,
		&subscription.CreatedAt, &subscription.UpdatedAt,
		&plan.Name, &plan.DisplayName, &plan.Description,
		&plan.PriceMonthly, &plan.PriceYearly, &plan.Currency,
		&featuresJSON, &limitsJSON, &plan.TrialDays, &plan.Active,
	)
	if err != nil {
		return nil, err
	}

	// Parse plan JSON fields
	if err := json.Unmarshal([]byte(featuresJSON), &plan.Features); err != nil {
		return nil, fmt.Errorf("failed to parse plan features: %w", err)
	}

	if err := json.Unmarshal([]byte(limitsJSON), &plan.Limits); err != nil {
		return nil, fmt.Errorf("failed to parse plan limits: %w", err)
	}

	plan.ID = subscription.PlanID
	subscription.Plan = plan

	return subscription, nil
}