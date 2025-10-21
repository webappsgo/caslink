package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// PlanManager handles billing plan management
type PlanManager struct {
	db     *db.DB
	config *config.BillingConfig
	logger *logrus.Logger
}

// NewPlanManager creates a new plan manager
func NewPlanManager(database *db.DB, cfg *config.BillingConfig, logger *logrus.Logger) (*PlanManager, error) {
	pm := &PlanManager{
		db:     database,
		config: cfg,
		logger: logger,
	}

	// Initialize default plans if none exist
	if err := pm.initializeDefaultPlans(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize default plans: %w", err)
	}

	return pm, nil
}

// Plan represents a billing plan
type Plan struct {
	ID           string            `json:"id" db:"id"`
	Name         string            `json:"name" db:"name"`
	DisplayName  string            `json:"display_name" db:"display_name"`
	Description  string            `json:"description" db:"description"`
	PriceMonthly int64             `json:"price_monthly" db:"price_monthly"` // in cents
	PriceYearly  int64             `json:"price_yearly" db:"price_yearly"`   // in cents
	Currency     string            `json:"currency" db:"currency"`
	Features     []string          `json:"features" db:"features"`
	Limits       map[string]int64  `json:"limits" db:"limits"`
	TrialDays    int               `json:"trial_days" db:"trial_days"`
	Active       bool              `json:"active" db:"active"`
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`
}

// GetPlans returns all active plans
func (pm *PlanManager) GetPlans(ctx context.Context) ([]*Plan, error) {
	query := `
		SELECT id, name, display_name, description, price_monthly, price_yearly,
		       currency, features, limits, trial_days, active, created_at
		FROM billing_plans
		WHERE active = true
		ORDER BY price_monthly ASC`

	rows, err := pm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query plans: %w", err)
	}
	defer rows.Close()

	var plans []*Plan
	for rows.Next() {
		plan := &Plan{}
		var featuresJSON, limitsJSON string

		err := rows.Scan(
			&plan.ID, &plan.Name, &plan.DisplayName, &plan.Description,
			&plan.PriceMonthly, &plan.PriceYearly, &plan.Currency,
			&featuresJSON, &limitsJSON, &plan.TrialDays,
			&plan.Active, &plan.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan plan: %w", err)
		}

		// Parse JSON fields
		if err := json.Unmarshal([]byte(featuresJSON), &plan.Features); err != nil {
			return nil, fmt.Errorf("failed to parse features: %w", err)
		}

		if err := json.Unmarshal([]byte(limitsJSON), &plan.Limits); err != nil {
			return nil, fmt.Errorf("failed to parse limits: %w", err)
		}

		plans = append(plans, plan)
	}

	return plans, nil
}

// GetPlan returns a specific plan by ID
func (pm *PlanManager) GetPlan(ctx context.Context, planID string) (*Plan, error) {
	query := `
		SELECT id, name, display_name, description, price_monthly, price_yearly,
		       currency, features, limits, trial_days, active, created_at
		FROM billing_plans
		WHERE id = ? AND active = true`

	row := pm.db.QueryRowContext(ctx, query, planID)

	plan := &Plan{}
	var featuresJSON, limitsJSON string

	err := row.Scan(
		&plan.ID, &plan.Name, &plan.DisplayName, &plan.Description,
		&plan.PriceMonthly, &plan.PriceYearly, &plan.Currency,
		&featuresJSON, &limitsJSON, &plan.TrialDays,
		&plan.Active, &plan.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	// Parse JSON fields
	if err := json.Unmarshal([]byte(featuresJSON), &plan.Features); err != nil {
		return nil, fmt.Errorf("failed to parse features: %w", err)
	}

	if err := json.Unmarshal([]byte(limitsJSON), &plan.Limits); err != nil {
		return nil, fmt.Errorf("failed to parse limits: %w", err)
	}

	return plan, nil
}

// CreatePlan creates a new billing plan
func (pm *PlanManager) CreatePlan(ctx context.Context, plan *Plan) error {
	plan.ID = pm.generatePlanID()
	plan.CreatedAt = time.Now()
	plan.Active = true

	// Serialize JSON fields
	featuresJSON, err := json.Marshal(plan.Features)
	if err != nil {
		return fmt.Errorf("failed to marshal features: %w", err)
	}

	limitsJSON, err := json.Marshal(plan.Limits)
	if err != nil {
		return fmt.Errorf("failed to marshal limits: %w", err)
	}

	query := `
		INSERT INTO billing_plans (
			id, name, display_name, description, price_monthly, price_yearly,
			currency, features, limits, trial_days, active, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = pm.db.ExecContext(ctx, query,
		plan.ID, plan.Name, plan.DisplayName, plan.Description,
		plan.PriceMonthly, plan.PriceYearly, plan.Currency,
		string(featuresJSON), string(limitsJSON), plan.TrialDays,
		plan.Active, plan.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create plan: %w", err)
	}

	pm.logger.WithFields(logrus.Fields{
		"plan_id":   plan.ID,
		"plan_name": plan.Name,
	}).Info("Plan created")

	return nil
}

// UpdatePlan updates an existing plan
func (pm *PlanManager) UpdatePlan(ctx context.Context, plan *Plan) error {
	// Serialize JSON fields
	featuresJSON, err := json.Marshal(plan.Features)
	if err != nil {
		return fmt.Errorf("failed to marshal features: %w", err)
	}

	limitsJSON, err := json.Marshal(plan.Limits)
	if err != nil {
		return fmt.Errorf("failed to marshal limits: %w", err)
	}

	query := `
		UPDATE billing_plans SET
			name = ?, display_name = ?, description = ?,
			price_monthly = ?, price_yearly = ?, currency = ?,
			features = ?, limits = ?, trial_days = ?, active = ?
		WHERE id = ?`

	_, err = pm.db.ExecContext(ctx, query,
		plan.Name, plan.DisplayName, plan.Description,
		plan.PriceMonthly, plan.PriceYearly, plan.Currency,
		string(featuresJSON), string(limitsJSON), plan.TrialDays,
		plan.Active, plan.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update plan: %w", err)
	}

	pm.logger.WithFields(logrus.Fields{
		"plan_id":   plan.ID,
		"plan_name": plan.Name,
	}).Info("Plan updated")

	return nil
}

// DeletePlan soft deletes a plan (sets active = false)
func (pm *PlanManager) DeletePlan(ctx context.Context, planID string) error {
	query := `UPDATE billing_plans SET active = false WHERE id = ?`

	_, err := pm.db.ExecContext(ctx, query, planID)
	if err != nil {
		return fmt.Errorf("failed to delete plan: %w", err)
	}

	pm.logger.WithField("plan_id", planID).Info("Plan deleted")

	return nil
}

// GetPlanLimits returns the limits for a specific plan
func (pm *PlanManager) GetPlanLimits(ctx context.Context, planID string) (map[string]int64, error) {
	plan, err := pm.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}

	return plan.Limits, nil
}

// HasFeature checks if a plan has a specific feature
func (pm *PlanManager) HasFeature(ctx context.Context, planID, feature string) (bool, error) {
	plan, err := pm.GetPlan(ctx, planID)
	if err != nil {
		return false, err
	}

	for _, f := range plan.Features {
		if f == feature {
			return true, nil
		}
	}

	return false, nil
}

// initializeDefaultPlans creates default plans if none exist
func (pm *PlanManager) initializeDefaultPlans(ctx context.Context) error {
	// Check if plans already exist
	count := 0
	err := pm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM billing_plans").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check existing plans: %w", err)
	}

	if count > 0 {
		return nil // Plans already exist
	}

	// Create default plans
	defaultPlans := []*Plan{
		{
			Name:         "free",
			DisplayName:  "Free",
			Description:  "Perfect for personal use and getting started",
			PriceMonthly: 0,
			PriceYearly:  0,
			Currency:     "USD",
			Features: []string{
				"url_shortening",
				"basic_analytics",
				"qr_codes",
			},
			Limits: map[string]int64{
				"urls_per_month":     100,
				"api_requests_hour":  1000,
				"qr_codes_month":    50,
				"custom_domains":    0,
			},
			TrialDays: 0,
		},
		{
			Name:         "starter",
			DisplayName:  "Starter",
			Description:  "Great for small businesses and growing projects",
			PriceMonthly: 999,  // $9.99
			PriceYearly:  9999, // $99.99 (2 months free)
			Currency:     "USD",
			Features: []string{
				"url_shortening",
				"advanced_analytics",
				"qr_codes",
				"custom_domains",
				"bulk_operations",
			},
			Limits: map[string]int64{
				"urls_per_month":     10000,
				"api_requests_hour":  10000,
				"qr_codes_month":    1000,
				"custom_domains":    3,
			},
			TrialDays: 14,
		},
		{
			Name:         "pro",
			DisplayName:  "Pro",
			Description:  "Perfect for professionals and growing teams",
			PriceMonthly: 2999,  // $29.99
			PriceYearly:  29999, // $299.99 (2 months free)
			Currency:     "USD",
			Features: []string{
				"url_shortening",
				"advanced_analytics",
				"qr_codes",
				"custom_domains",
				"bulk_operations",
				"team_management",
				"api_access",
				"webhook_integration",
			},
			Limits: map[string]int64{
				"urls_per_month":     100000,
				"api_requests_hour":  100000,
				"qr_codes_month":    10000,
				"custom_domains":    10,
				"team_members":      10,
			},
			TrialDays: 14,
		},
		{
			Name:         "business",
			DisplayName:  "Business",
			Description:  "Advanced features for larger teams and organizations",
			PriceMonthly: 9999,  // $99.99
			PriceYearly:  99999, // $999.99 (2 months free)
			Currency:     "USD",
			Features: []string{
				"url_shortening",
				"advanced_analytics",
				"qr_codes",
				"custom_domains",
				"bulk_operations",
				"team_management",
				"api_access",
				"webhook_integration",
				"priority_support",
				"white_labeling",
			},
			Limits: map[string]int64{
				"urls_per_month":     1000000,
				"api_requests_hour":  1000000,
				"qr_codes_month":    100000,
				"custom_domains":    50,
				"team_members":      50,
			},
			TrialDays: 14,
		},
		{
			Name:         "enterprise",
			DisplayName:  "Enterprise",
			Description:  "Unlimited features for large enterprises",
			PriceMonthly: 29999, // $299.99
			PriceYearly:  299999, // $2999.99 (2 months free)
			Currency:     "USD",
			Features: []string{
				"url_shortening",
				"advanced_analytics",
				"qr_codes",
				"custom_domains",
				"bulk_operations",
				"team_management",
				"api_access",
				"webhook_integration",
				"priority_support",
				"white_labeling",
				"sso_integration",
				"dedicated_support",
				"custom_integrations",
			},
			Limits: map[string]int64{
				"urls_per_month":     -1, // unlimited
				"api_requests_hour":  -1, // unlimited
				"qr_codes_month":    -1, // unlimited
				"custom_domains":    -1, // unlimited
				"team_members":      -1, // unlimited
			},
			TrialDays: 30,
		},
	}

	for _, plan := range defaultPlans {
		if err := pm.CreatePlan(ctx, plan); err != nil {
			return fmt.Errorf("failed to create default plan %s: %w", plan.Name, err)
		}
	}

	pm.logger.Info("Default billing plans created")
	return nil
}

// generatePlanID generates a unique plan ID
func (pm *PlanManager) generatePlanID() string {
	return fmt.Sprintf("plan_%d", time.Now().UnixNano())
}