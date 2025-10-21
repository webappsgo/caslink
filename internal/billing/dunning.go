package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// DunningManager handles failed payment recovery
type DunningManager struct {
	db     *db.DB
	config *config.BillingConfig
	logger *logrus.Logger
}

// NewDunningManager creates a new dunning manager
func NewDunningManager(database *db.DB, cfg *config.BillingConfig, logger *logrus.Logger) (*DunningManager, error) {
	return &DunningManager{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// DunningAttempt represents a dunning attempt
type DunningAttempt struct {
	ID             string    `json:"id" db:"id"`
	SubscriptionID string    `json:"subscription_id" db:"subscription_id"`
	InvoiceID      string    `json:"invoice_id" db:"invoice_id"`
	AttemptNumber  int       `json:"attempt_number" db:"attempt_number"`
	Status         string    `json:"status" db:"status"` // pending, succeeded, failed
	FailureReason  string    `json:"failure_reason" db:"failure_reason"`
	NextRetryAt    time.Time `json:"next_retry_at" db:"next_retry_at"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// ProcessFailedPayments processes failed payments and attempts recovery
func (dm *DunningManager) ProcessFailedPayments(ctx context.Context) error {
	dm.logger.Info("Starting dunning process for failed payments")

	// Get failed payments that need retry
	failedPayments, err := dm.getFailedPaymentsForRetry(ctx)
	if err != nil {
		return fmt.Errorf("failed to get failed payments: %w", err)
	}

	dm.logger.WithField("count", len(failedPayments)).Info("Found failed payments for dunning")

	processedCount := 0
	successCount := 0

	for _, payment := range failedPayments {
		if err := dm.processFailedPayment(ctx, payment); err != nil {
			dm.logger.WithError(err).WithField("payment_id", payment.ID).Error("Failed to process dunning attempt")
			continue
		}

		processedCount++

		// Check if payment succeeded
		updatedPayment, err := dm.getPayment(ctx, payment.ID)
		if err == nil && updatedPayment.Status == PaymentStatusSucceeded {
			successCount++
		}
	}

	dm.logger.WithFields(logrus.Fields{
		"processed": processedCount,
		"succeeded": successCount,
	}).Info("Dunning process completed")

	return nil
}

// processFailedPayment processes a single failed payment
func (dm *DunningManager) processFailedPayment(ctx context.Context, payment *Payment) error {
	// Get subscription for this payment
	subscription, err := dm.getSubscription(ctx, payment.SubscriptionID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Check if subscription should be suspended
	daysSinceFailure := time.Since(payment.CreatedAt).Hours() / 24
	gracePeriod := float64(dm.config.GracePeriodDays)

	if daysSinceFailure > gracePeriod {
		if err := dm.suspendSubscription(ctx, subscription); err != nil {
			dm.logger.WithError(err).Warn("Failed to suspend subscription")
		}
		return nil
	}

	// Get previous dunning attempts
	attempts, err := dm.getDunningAttempts(ctx, payment.ID)
	if err != nil {
		return fmt.Errorf("failed to get dunning attempts: %w", err)
	}

	// Calculate next attempt details
	attemptNumber := len(attempts) + 1
	maxAttempts := dm.getMaxRetryAttempts()

	if attemptNumber > maxAttempts {
		dm.logger.WithFields(logrus.Fields{
			"payment_id":     payment.ID,
			"attempt_number": attemptNumber,
			"max_attempts":   maxAttempts,
		}).Info("Maximum retry attempts reached")
		return nil
	}

	// Calculate retry delay
	retryDelay := dm.calculateRetryDelay(attemptNumber)
	nextRetryAt := time.Now().Add(retryDelay)

	// Create dunning attempt record
	attempt := &DunningAttempt{
		ID:             dm.generateAttemptID(),
		SubscriptionID: payment.SubscriptionID,
		InvoiceID:      payment.InvoiceID,
		AttemptNumber:  attemptNumber,
		Status:         "pending",
		NextRetryAt:    nextRetryAt,
		CreatedAt:      time.Now(),
	}

	if err := dm.saveDunningAttempt(ctx, attempt); err != nil {
		return fmt.Errorf("failed to save dunning attempt: %w", err)
	}

	// Attempt to retry payment
	if err := dm.retryPayment(ctx, payment, attempt); err != nil {
		attempt.Status = "failed"
		attempt.FailureReason = err.Error()
		dm.updateDunningAttempt(ctx, attempt)

		dm.logger.WithError(err).WithFields(logrus.Fields{
			"payment_id":     payment.ID,
			"attempt_number": attemptNumber,
		}).Warn("Payment retry failed")

		// Send failure notification
		dm.sendDunningNotification(ctx, subscription, payment, attempt)
		return nil
	}

	attempt.Status = "succeeded"
	dm.updateDunningAttempt(ctx, attempt)

	dm.logger.WithFields(logrus.Fields{
		"payment_id":     payment.ID,
		"attempt_number": attemptNumber,
	}).Info("Payment retry succeeded")

	// Send success notification
	dm.sendSuccessNotification(ctx, subscription, payment)

	return nil
}

// getFailedPaymentsForRetry gets failed payments that are eligible for retry
func (dm *DunningManager) getFailedPaymentsForRetry(ctx context.Context) ([]*Payment, error) {
	// Get payments that failed and are within grace period
	gracePeriod := time.Duration(dm.config.GracePeriodDays) * 24 * time.Hour
	cutoff := time.Now().Add(-gracePeriod)

	query := `
		SELECT p.id, p.user_id, p.subscription_id, p.invoice_id, p.provider_payment_id,
		       p.status, p.method, p.amount, p.currency, p.failure_reason,
		       p.processed_at, p.created_at, p.updated_at
		FROM payments p
		LEFT JOIN (
			SELECT subscription_id, MAX(created_at) as last_attempt
			FROM dunning_attempts
			GROUP BY subscription_id
		) da ON p.subscription_id = da.subscription_id
		WHERE p.status = 'failed'
		  AND p.created_at >= ?
		  AND (da.last_attempt IS NULL OR da.last_attempt <= DATE_SUB(NOW(), INTERVAL 24 HOUR))
		ORDER BY p.created_at ASC`

	rows, err := dm.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to query failed payments: %w", err)
	}
	defer rows.Close()

	var payments []*Payment
	for rows.Next() {
		payment := &Payment{}
		err := rows.Scan(
			&payment.ID, &payment.UserID, &payment.SubscriptionID,
			&payment.InvoiceID, &payment.ProviderPaymentID,
			&payment.Status, &payment.Method, &payment.Amount, &payment.Currency,
			&payment.FailureReason, &payment.ProcessedAt, &payment.CreatedAt, &payment.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payment: %w", err)
		}
		payments = append(payments, payment)
	}

	return payments, nil
}

// retryPayment attempts to retry a failed payment
func (dm *DunningManager) retryPayment(ctx context.Context, payment *Payment, attempt *DunningAttempt) error {
	// Implementation would retry payment with provider
	// For now, simulate retry with random success/failure
	dm.logger.WithFields(logrus.Fields{
		"payment_id":     payment.ID,
		"attempt_number": attempt.AttemptNumber,
	}).Info("Retrying payment")

	// Simulate retry logic - in real implementation this would call payment provider
	// For demo purposes, let's say retries have a 30% success rate
	if time.Now().UnixNano()%10 < 3 {
		// Update payment status to succeeded
		now := time.Now()
		payment.Status = PaymentStatusSucceeded
		payment.ProcessedAt = &now
		payment.FailureReason = ""

		return dm.updatePaymentStatus(ctx, payment)
	}

	return fmt.Errorf("payment retry failed with provider")
}

// suspendSubscription suspends a subscription due to failed payments
func (dm *DunningManager) suspendSubscription(ctx context.Context, subscription *Subscription) error {
	subscription.Status = StatusPastDue
	subscription.UpdatedAt = time.Now()

	query := `
		UPDATE subscriptions SET
			status = ?, updated_at = ?
		WHERE id = ?`

	_, err := dm.db.ExecContext(ctx, query, subscription.Status, subscription.UpdatedAt, subscription.ID)
	if err != nil {
		return fmt.Errorf("failed to suspend subscription: %w", err)
	}

	dm.logger.WithField("subscription_id", subscription.ID).Info("Subscription suspended due to failed payments")

	// Send suspension notification
	dm.sendSuspensionNotification(ctx, subscription)

	return nil
}

// getDunningAttempts gets dunning attempts for a payment
func (dm *DunningManager) getDunningAttempts(ctx context.Context, paymentID string) ([]*DunningAttempt, error) {
	query := `
		SELECT id, subscription_id, invoice_id, attempt_number, status,
		       failure_reason, next_retry_at, created_at
		FROM dunning_attempts
		WHERE subscription_id = (SELECT subscription_id FROM payments WHERE id = ?)
		ORDER BY attempt_number ASC`

	rows, err := dm.db.QueryContext(ctx, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query dunning attempts: %w", err)
	}
	defer rows.Close()

	var attempts []*DunningAttempt
	for rows.Next() {
		attempt := &DunningAttempt{}
		err := rows.Scan(
			&attempt.ID, &attempt.SubscriptionID, &attempt.InvoiceID,
			&attempt.AttemptNumber, &attempt.Status, &attempt.FailureReason,
			&attempt.NextRetryAt, &attempt.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan dunning attempt: %w", err)
		}
		attempts = append(attempts, attempt)
	}

	return attempts, nil
}

// Helper methods

func (dm *DunningManager) getMaxRetryAttempts() int {
	if dm.config.RetryAttempts > 0 {
		return dm.config.RetryAttempts
	}
	return 3 // Default
}

func (dm *DunningManager) calculateRetryDelay(attemptNumber int) time.Duration {
	// Exponential backoff: 1 hour, 4 hours, 16 hours, etc.
	hours := 1
	for i := 1; i < attemptNumber; i++ {
		hours *= 4
	}

	// Cap at 7 days
	if hours > 168 {
		hours = 168
	}

	return time.Duration(hours) * time.Hour
}

func (dm *DunningManager) saveDunningAttempt(ctx context.Context, attempt *DunningAttempt) error {
	query := `
		INSERT INTO dunning_attempts (
			id, subscription_id, invoice_id, attempt_number, status,
			failure_reason, next_retry_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := dm.db.ExecContext(ctx, query,
		attempt.ID, attempt.SubscriptionID, attempt.InvoiceID,
		attempt.AttemptNumber, attempt.Status, attempt.FailureReason,
		attempt.NextRetryAt, attempt.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save dunning attempt: %w", err)
	}

	return nil
}

func (dm *DunningManager) updateDunningAttempt(ctx context.Context, attempt *DunningAttempt) error {
	query := `
		UPDATE dunning_attempts SET
			status = ?, failure_reason = ?
		WHERE id = ?`

	_, err := dm.db.ExecContext(ctx, query, attempt.Status, attempt.FailureReason, attempt.ID)
	if err != nil {
		return fmt.Errorf("failed to update dunning attempt: %w", err)
	}

	return nil
}

func (dm *DunningManager) updatePaymentStatus(ctx context.Context, payment *Payment) error {
	query := `
		UPDATE payments SET
			status = ?, failure_reason = ?, processed_at = ?, updated_at = ?
		WHERE id = ?`

	_, err := dm.db.ExecContext(ctx, query,
		payment.Status, payment.FailureReason, payment.ProcessedAt, time.Now(), payment.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	return nil
}

func (dm *DunningManager) getPayment(ctx context.Context, paymentID string) (*Payment, error) {
	query := `
		SELECT id, user_id, subscription_id, invoice_id, provider_payment_id,
		       status, method, amount, currency, failure_reason,
		       processed_at, created_at, updated_at
		FROM payments
		WHERE id = ?`

	row := dm.db.QueryRowContext(ctx, query, paymentID)

	payment := &Payment{}
	err := row.Scan(
		&payment.ID, &payment.UserID, &payment.SubscriptionID,
		&payment.InvoiceID, &payment.ProviderPaymentID,
		&payment.Status, &payment.Method, &payment.Amount, &payment.Currency,
		&payment.FailureReason, &payment.ProcessedAt, &payment.CreatedAt, &payment.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	return payment, nil
}

func (dm *DunningManager) getSubscription(ctx context.Context, subscriptionID string) (*Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, provider_subscription_id, status, billing_cycle,
		       current_period_start, current_period_end, trial_start, trial_end,
		       cancel_at_period_end, canceled_at, created_at, updated_at
		FROM subscriptions
		WHERE id = ?`

	row := dm.db.QueryRowContext(ctx, query, subscriptionID)

	subscription := &Subscription{}
	err := row.Scan(
		&subscription.ID, &subscription.UserID, &subscription.PlanID,
		&subscription.ProviderSubscriptionID, &subscription.Status, &subscription.BillingCycle,
		&subscription.CurrentPeriodStart, &subscription.CurrentPeriodEnd,
		&subscription.TrialStart, &subscription.TrialEnd,
		&subscription.CancelAtPeriodEnd, &subscription.CanceledAt,
		&subscription.CreatedAt, &subscription.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return subscription, nil
}

// Notification methods (would integrate with notification system)

func (dm *DunningManager) sendDunningNotification(ctx context.Context, subscription *Subscription, payment *Payment, attempt *DunningAttempt) {
	dm.logger.WithFields(logrus.Fields{
		"subscription_id": subscription.ID,
		"payment_id":      payment.ID,
		"attempt_number":  attempt.AttemptNumber,
	}).Info("Sending dunning notification")

	// Implementation would send email/notification about failed payment retry
}

func (dm *DunningManager) sendSuccessNotification(ctx context.Context, subscription *Subscription, payment *Payment) {
	dm.logger.WithFields(logrus.Fields{
		"subscription_id": subscription.ID,
		"payment_id":      payment.ID,
	}).Info("Sending payment success notification")

	// Implementation would send email/notification about successful payment retry
}

func (dm *DunningManager) sendSuspensionNotification(ctx context.Context, subscription *Subscription) {
	dm.logger.WithField("subscription_id", subscription.ID).Info("Sending suspension notification")

	// Implementation would send email/notification about subscription suspension
}

func (dm *DunningManager) generateAttemptID() string {
	return fmt.Sprintf("dun_%d", time.Now().UnixNano())
}

// GetDunningStats returns dunning statistics
func (dm *DunningManager) GetDunningStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get total dunning attempts
	var totalAttempts int64
	err := dm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM dunning_attempts").Scan(&totalAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to get total attempts: %w", err)
	}
	stats["total_attempts"] = totalAttempts

	// Get successful attempts
	var successfulAttempts int64
	err = dm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM dunning_attempts WHERE status = 'succeeded'").Scan(&successfulAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to get successful attempts: %w", err)
	}
	stats["successful_attempts"] = successfulAttempts

	// Calculate success rate
	if totalAttempts > 0 {
		stats["success_rate"] = float64(successfulAttempts) / float64(totalAttempts) * 100
	} else {
		stats["success_rate"] = 0.0
	}

	// Get suspended subscriptions
	var suspendedSubs int64
	err = dm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscriptions WHERE status = 'past_due'").Scan(&suspendedSubs)
	if err != nil {
		return nil, fmt.Errorf("failed to get suspended subscriptions: %w", err)
	}
	stats["suspended_subscriptions"] = suspendedSubs

	return stats, nil
}