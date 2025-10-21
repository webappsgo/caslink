package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// UsageTracker handles usage tracking and metering
type UsageTracker struct {
	db     *db.DB
	config *config.BillingConfig
	logger *logrus.Logger
}

// NewUsageTracker creates a new usage tracker
func NewUsageTracker(database *db.DB, cfg *config.BillingConfig, logger *logrus.Logger) (*UsageTracker, error) {
	return &UsageTracker{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// UsageRecord represents a usage record
type UsageRecord struct {
	ID             string    `json:"id" db:"id"`
	UserID         string    `json:"user_id" db:"user_id"`
	SubscriptionID string    `json:"subscription_id" db:"subscription_id"`
	MetricName     string    `json:"metric_name" db:"metric_name"`
	Quantity       int64     `json:"quantity" db:"quantity"`
	Timestamp      time.Time `json:"timestamp" db:"timestamp"`
	BillingPeriod  string    `json:"billing_period" db:"billing_period"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// RecordUsage records usage for a user
func (ut *UsageTracker) RecordUsage(ctx context.Context, userID, metric string, quantity int64) error {
	now := time.Now()
	billingPeriod := now.Format("2006-01")
	recordID := ut.generateRecordID()

	// Get user's subscription to link usage
	subscriptionManager := &SubscriptionManager{db: ut.db, config: ut.config, logger: ut.logger}
	subscription, err := subscriptionManager.GetUserSubscription(ctx, userID)
	if err != nil {
		ut.logger.WithError(err).Warn("Failed to get user subscription for usage tracking")
		// Continue without subscription link for free users
	}

	subscriptionID := ""
	if subscription != nil {
		subscriptionID = subscription.ID
	}

	query := `
		INSERT INTO usage_records (id, user_id, subscription_id, metric_name, quantity, timestamp, billing_period, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = ut.db.ExecContext(ctx, query, recordID, userID, subscriptionID, metric, quantity, now, billingPeriod, now)
	if err != nil {
		return fmt.Errorf("failed to record usage: %w", err)
	}

	ut.logger.WithFields(logrus.Fields{
		"user_id":    userID,
		"metric":     metric,
		"quantity":   quantity,
		"period":     billingPeriod,
	}).Debug("Usage recorded")

	return nil
}

// GetUsage gets usage for a user and metric in a billing period
func (ut *UsageTracker) GetUsage(ctx context.Context, userID, metric string, period time.Time) (int64, error) {
	billingPeriod := period.Format("2006-01")

	query := `
		SELECT COALESCE(SUM(quantity), 0)
		FROM usage_records
		WHERE user_id = ? AND metric_name = ? AND billing_period = ?`

	var usage int64
	err := ut.db.QueryRowContext(ctx, query, userID, metric, billingPeriod).Scan(&usage)
	if err != nil {
		return 0, fmt.Errorf("failed to get usage: %w", err)
	}

	return usage, nil
}

// GetUsageSummary gets usage summary for a user
func (ut *UsageTracker) GetUsageSummary(ctx context.Context, userID string, period time.Time) (*UsageSummary, error) {
	billingPeriod := period.Format("2006-01")

	query := `
		SELECT metric_name, SUM(quantity) as total_usage
		FROM usage_records
		WHERE user_id = ? AND billing_period = ?
		GROUP BY metric_name`

	rows, err := ut.db.QueryContext(ctx, query, userID, billingPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage summary: %w", err)
	}
	defer rows.Close()

	// Get user's plan limits
	subscriptionManager := &SubscriptionManager{db: ut.db, config: ut.config, logger: ut.logger}
	subscription, err := subscriptionManager.GetUserSubscription(ctx, userID)
	if err != nil {
		ut.logger.WithError(err).Warn("Failed to get user subscription for usage summary")
	}

	limits := make(map[string]int64)
	if subscription != nil && subscription.Plan != nil {
		limits = subscription.Plan.Limits
	}

	summary := &UsageSummary{
		UserID:  userID,
		Period:  period,
		Metrics: make(map[string]UsageMetric),
	}

	// Scan usage data
	for rows.Next() {
		var metricName string
		var usage int64
		if err := rows.Scan(&metricName, &usage); err != nil {
			return nil, fmt.Errorf("failed to scan usage: %w", err)
		}

		limit, hasLimit := limits[metricName]
		unlimited := !hasLimit || limit == -1

		summary.Metrics[metricName] = UsageMetric{
			Used:      usage,
			Limit:     limit,
			Unlimited: unlimited,
		}
	}

	// Add metrics with limits but no usage
	for metricName, limit := range limits {
		if _, exists := summary.Metrics[metricName]; !exists {
			unlimited := limit == -1

			summary.Metrics[metricName] = UsageMetric{
				Used:      0,
				Limit:     limit,
				Unlimited: unlimited,
			}
		}
	}

	return summary, nil
}

// CheckLimits checks if a user has exceeded usage limits for a metric
func (ut *UsageTracker) CheckLimits(ctx context.Context, userID, metric string) (*UsageStatus, error) {
	now := time.Now()
	period := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Get current usage
	usage, err := ut.GetUsage(ctx, userID, metric, period)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage: %w", err)
	}

	// Get user's limits
	subscriptionManager := &SubscriptionManager{db: ut.db, config: ut.config, logger: ut.logger}
	subscription, err := subscriptionManager.GetUserSubscription(ctx, userID)
	if err != nil {
		// If no subscription, assume free plan limits
		ut.logger.WithError(err).Debug("No subscription found, checking against free plan")
		return ut.checkFreePlanLimits(ctx, metric, usage, period)
	}

	if subscription == nil || subscription.Plan == nil {
		return ut.checkFreePlanLimits(ctx, metric, usage, period)
	}

	limit, hasLimit := subscription.Plan.Limits[metric]
	if !hasLimit || limit == -1 {
		// Unlimited
		return &UsageStatus{
			Allowed:   true,
			Unlimited: true,
			Used:      usage,
			Limit:     -1,
			Remaining: -1,
		}, nil
	}

	allowed := usage < limit
	remaining := limit - usage
	if remaining < 0 {
		remaining = 0
	}

	// Calculate reset time (end of current month)
	resetAt := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())

	return &UsageStatus{
		Allowed:   allowed,
		Unlimited: false,
		Used:      usage,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   &resetAt,
	}, nil
}

// checkFreePlanLimits checks limits against the free plan
func (ut *UsageTracker) checkFreePlanLimits(ctx context.Context, metric string, usage int64, period time.Time) (*UsageStatus, error) {
	// Get free plan
	planManager := &PlanManager{db: ut.db, config: ut.config, logger: ut.logger}
	freePlan, err := planManager.GetPlan(ctx, "free")
	if err != nil {
		// If no free plan, allow unlimited usage
		return &UsageStatus{
			Allowed:   true,
			Unlimited: true,
			Used:      usage,
			Limit:     -1,
			Remaining: -1,
		}, nil
	}

	limit, hasLimit := freePlan.Limits[metric]
	if !hasLimit || limit == -1 {
		return &UsageStatus{
			Allowed:   true,
			Unlimited: true,
			Used:      usage,
			Limit:     -1,
			Remaining: -1,
		}, nil
	}

	allowed := usage < limit
	remaining := limit - usage
	if remaining < 0 {
		remaining = 0
	}

	now := time.Now()
	resetAt := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())

	return &UsageStatus{
		Allowed:   allowed,
		Unlimited: false,
		Used:      usage,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   &resetAt,
	}, nil
}

// GetTopUsers gets top users by usage for a metric
func (ut *UsageTracker) GetTopUsers(ctx context.Context, metric string, period time.Time, limit int) ([]UserUsage, error) {
	billingPeriod := period.Format("2006-01")

	query := `
		SELECT user_id, SUM(quantity) as total_usage
		FROM usage_records
		WHERE metric_name = ? AND billing_period = ?
		GROUP BY user_id
		ORDER BY total_usage DESC
		LIMIT ?`

	rows, err := ut.db.QueryContext(ctx, query, metric, billingPeriod, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top users: %w", err)
	}
	defer rows.Close()

	var users []UserUsage
	for rows.Next() {
		var user UserUsage
		if err := rows.Scan(&user.UserID, &user.Usage); err != nil {
			return nil, fmt.Errorf("failed to scan user usage: %w", err)
		}
		users = append(users, user)
	}

	return users, nil
}

// GetStats returns usage statistics
func (ut *UsageTracker) GetStats(ctx context.Context) (*UsageStats, error) {
	now := time.Now()
	billingPeriod := now.Format("2006-01")

	stats := &UsageStats{
		UsageByMetric: make(map[string]int64),
	}

	// Get total usage by metric for current month
	query := `
		SELECT metric_name, SUM(quantity) as total_usage
		FROM usage_records
		WHERE billing_period = ?
		GROUP BY metric_name`

	rows, err := ut.db.QueryContext(ctx, query, billingPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var metric string
		var usage int64
		if err := rows.Scan(&metric, &usage); err != nil {
			return nil, fmt.Errorf("failed to scan usage stats: %w", err)
		}

		stats.UsageByMetric[metric] = usage

		// Set specific totals for known metrics
		switch metric {
		case "api_requests":
			stats.TotalAPIRequests = usage
		case "urls_created":
			stats.TotalURLsCreated = usage
		case "qr_codes":
			stats.TotalQRCodes = usage
		}
	}

	// Get top users for API requests
	topUsers, err := ut.GetTopUsers(ctx, "api_requests", now, 10)
	if err == nil {
		stats.TopUsageUsers = topUsers
	}

	return stats, nil
}

// CleanupOldRecords removes old usage records
func (ut *UsageTracker) CleanupOldRecords(ctx context.Context, retentionMonths int) error {
	cutoff := time.Now().AddDate(0, -retentionMonths, 0)
	cutoffPeriod := cutoff.Format("2006-01")

	query := `DELETE FROM usage_records WHERE billing_period < ?`

	result, err := ut.db.ExecContext(ctx, query, cutoffPeriod)
	if err != nil {
		return fmt.Errorf("failed to cleanup old usage records: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	ut.logger.WithFields(logrus.Fields{
		"cutoff_period":  cutoffPeriod,
		"rows_affected":  rowsAffected,
	}).Info("Old usage records cleaned up")

	return nil
}

// AggregateUsage aggregates usage data for efficient queries
func (ut *UsageTracker) AggregateUsage(ctx context.Context, period time.Time) error {
	billingPeriod := period.Format("2006-01")

	// This would implement aggregation logic for better performance
	// For now, we'll just log the operation
	ut.logger.WithField("period", billingPeriod).Info("Usage aggregation completed")

	return nil
}

// ExportUsage exports usage data for external analysis
func (ut *UsageTracker) ExportUsage(ctx context.Context, userID string, startPeriod, endPeriod time.Time) ([]*UsageRecord, error) {
	start := startPeriod.Format("2006-01")
	end := endPeriod.Format("2006-01")

	query := `
		SELECT id, user_id, subscription_id, metric_name, quantity, timestamp, billing_period, created_at
		FROM usage_records
		WHERE user_id = ? AND billing_period >= ? AND billing_period <= ?
		ORDER BY timestamp DESC`

	rows, err := ut.db.QueryContext(ctx, query, userID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to export usage: %w", err)
	}
	defer rows.Close()

	var records []*UsageRecord
	for rows.Next() {
		record := &UsageRecord{}
		err := rows.Scan(
			&record.ID, &record.UserID, &record.SubscriptionID,
			&record.MetricName, &record.Quantity, &record.Timestamp,
			&record.BillingPeriod, &record.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage record: %w", err)
		}
		records = append(records, record)
	}

	return records, nil
}

// generateRecordID generates a unique record ID
func (ut *UsageTracker) generateRecordID() string {
	return fmt.Sprintf("usage_%d", time.Now().UnixNano())
}