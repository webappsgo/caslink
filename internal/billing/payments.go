package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// PaymentManager handles payment processing
type PaymentManager struct {
	db     *db.DB
	config *config.BillingConfig
	logger *logrus.Logger
}

// NewPaymentManager creates a new payment manager
func NewPaymentManager(database *db.DB, cfg *config.BillingConfig, logger *logrus.Logger) (*PaymentManager, error) {
	return &PaymentManager{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// PaymentStatus represents payment status
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusSucceeded PaymentStatus = "succeeded"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusCanceled  PaymentStatus = "canceled"
	PaymentStatusRefunded  PaymentStatus = "refunded"
)

// PaymentMethod represents payment method
type PaymentMethod string

const (
	PaymentMethodCard   PaymentMethod = "card"
	PaymentMethodACH    PaymentMethod = "ach"
	PaymentMethodWire   PaymentMethod = "wire"
	PaymentMethodPayPal PaymentMethod = "paypal"
)

// Payment represents a payment transaction
type Payment struct {
	ID                 string        `json:"id" db:"id"`
	UserID             string        `json:"user_id" db:"user_id"`
	SubscriptionID     string        `json:"subscription_id" db:"subscription_id"`
	InvoiceID          string        `json:"invoice_id" db:"invoice_id"`
	ProviderPaymentID  string        `json:"provider_payment_id" db:"provider_payment_id"`
	Status             PaymentStatus `json:"status" db:"status"`
	Method             PaymentMethod `json:"method" db:"method"`
	Amount             int64         `json:"amount" db:"amount"` // in cents
	Currency           string        `json:"currency" db:"currency"`
	FailureReason      string        `json:"failure_reason" db:"failure_reason"`
	ProcessedAt        *time.Time    `json:"processed_at" db:"processed_at"`
	CreatedAt          time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at" db:"updated_at"`
}

// ProcessPayment processes a payment
func (pm *PaymentManager) ProcessPayment(ctx context.Context, userID string, amount int64, currency string) (*Payment, error) {
	now := time.Now()
	payment := &Payment{
		ID:        pm.generatePaymentID(),
		UserID:    userID,
		Status:    PaymentStatusPending,
		Method:    PaymentMethodCard, // Default method
		Amount:    amount,
		Currency:  currency,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Process payment with provider
	providerPaymentID, err := pm.processWithProvider(ctx, payment)
	if err != nil {
		payment.Status = PaymentStatusFailed
		payment.FailureReason = err.Error()
	} else {
		payment.ProviderPaymentID = providerPaymentID
		payment.Status = PaymentStatusSucceeded
		payment.ProcessedAt = &now
	}

	// Save payment to database
	if err := pm.savePayment(ctx, payment); err != nil {
		return nil, fmt.Errorf("failed to save payment: %w", err)
	}

	pm.logger.WithFields(logrus.Fields{
		"payment_id": payment.ID,
		"user_id":    userID,
		"amount":     amount,
		"status":     payment.Status,
	}).Info("Payment processed")

	return payment, nil
}

// GetPayment gets a payment by ID
func (pm *PaymentManager) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	query := `
		SELECT id, user_id, subscription_id, invoice_id, provider_payment_id,
		       status, method, amount, currency, failure_reason,
		       processed_at, created_at, updated_at
		FROM payments
		WHERE id = ?`

	row := pm.db.QueryRowContext(ctx, query, paymentID)

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

// GetUserPayments gets payments for a user
func (pm *PaymentManager) GetUserPayments(ctx context.Context, userID string, limit, offset int) ([]*Payment, error) {
	query := `
		SELECT id, user_id, subscription_id, invoice_id, provider_payment_id,
		       status, method, amount, currency, failure_reason,
		       processed_at, created_at, updated_at
		FROM payments
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`

	rows, err := pm.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query user payments: %w", err)
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

// UpdatePaymentStatus updates a payment status
func (pm *PaymentManager) UpdatePaymentStatus(ctx context.Context, paymentID string, status PaymentStatus, reason string) error {
	now := time.Now()
	var processedAt *time.Time

	if status == PaymentStatusSucceeded {
		processedAt = &now
	}

	query := `
		UPDATE payments SET
			status = ?, failure_reason = ?, processed_at = ?, updated_at = ?
		WHERE id = ?`

	_, err := pm.db.ExecContext(ctx, query, status, reason, processedAt, now, paymentID)
	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	pm.logger.WithFields(logrus.Fields{
		"payment_id": paymentID,
		"status":     status,
		"reason":     reason,
	}).Info("Payment status updated")

	return nil
}

// RefundPayment refunds a payment
func (pm *PaymentManager) RefundPayment(ctx context.Context, paymentID string, amount int64) error {
	payment, err := pm.GetPayment(ctx, paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment.Status != PaymentStatusSucceeded {
		return fmt.Errorf("cannot refund payment with status: %s", payment.Status)
	}

	// Process refund with provider
	if err := pm.refundWithProvider(ctx, payment.ProviderPaymentID, amount); err != nil {
		return fmt.Errorf("failed to process refund with provider: %w", err)
	}

	// Update payment status
	if err := pm.UpdatePaymentStatus(ctx, paymentID, PaymentStatusRefunded, "refunded"); err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	pm.logger.WithFields(logrus.Fields{
		"payment_id": paymentID,
		"amount":     amount,
	}).Info("Payment refunded")

	return nil
}

// GetFailedPayments gets failed payments for retry
func (pm *PaymentManager) GetFailedPayments(ctx context.Context, since time.Time) ([]*Payment, error) {
	query := `
		SELECT id, user_id, subscription_id, invoice_id, provider_payment_id,
		       status, method, amount, currency, failure_reason,
		       processed_at, created_at, updated_at
		FROM payments
		WHERE status = 'failed' AND created_at >= ?
		ORDER BY created_at DESC`

	rows, err := pm.db.QueryContext(ctx, query, since)
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
			return nil, fmt.Errorf("failed to scan failed payment: %w", err)
		}
		payments = append(payments, payment)
	}

	return payments, nil
}

// GetStats returns payment statistics
func (pm *PaymentManager) GetStats(ctx context.Context) (*RevenueStats, error) {
	stats := &RevenueStats{
		RevenueByPlan:  make(map[string]int64),
		RevenueByMonth: make(map[string]int64),
	}

	// Get total revenue
	query := `SELECT COALESCE(SUM(amount), 0) FROM payments WHERE status = 'succeeded'`
	err := pm.db.QueryRowContext(ctx, query).Scan(&stats.TotalRevenue)
	if err != nil {
		return nil, fmt.Errorf("failed to get total revenue: %w", err)
	}

	// Get MRR (Monthly Recurring Revenue) - simplified calculation
	now := time.Now()
	currentMonth := now.Format("2006-01")
	query = `SELECT COALESCE(SUM(amount), 0) FROM payments WHERE status = 'succeeded' AND DATE(created_at) >= ?`
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	err = pm.db.QueryRowContext(ctx, query, firstOfMonth).Scan(&stats.MonthlyRecurringRevenue)
	if err != nil {
		return nil, fmt.Errorf("failed to get MRR: %w", err)
	}

	// Calculate ARR (simplified)
	stats.AnnualRecurringRevenue = stats.MonthlyRecurringRevenue * 12

	// Get revenue by month (last 12 months)
	query = `
		SELECT DATE_FORMAT(created_at, '%Y-%m') as month, SUM(amount) as revenue
		FROM payments
		WHERE status = 'succeeded' AND created_at >= DATE_SUB(NOW(), INTERVAL 12 MONTH)
		GROUP BY month
		ORDER BY month`

	rows, err := pm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query revenue by month: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var month string
		var revenue int64
		if err := rows.Scan(&month, &revenue); err != nil {
			return nil, fmt.Errorf("failed to scan revenue by month: %w", err)
		}
		stats.RevenueByMonth[month] = revenue
	}

	return stats, nil
}

// Helper methods

func (pm *PaymentManager) savePayment(ctx context.Context, payment *Payment) error {
	query := `
		INSERT INTO payments (
			id, user_id, subscription_id, invoice_id, provider_payment_id,
			status, method, amount, currency, failure_reason,
			processed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := pm.db.ExecContext(ctx, query,
		payment.ID, payment.UserID, payment.SubscriptionID,
		payment.InvoiceID, payment.ProviderPaymentID,
		payment.Status, payment.Method, payment.Amount, payment.Currency,
		payment.FailureReason, payment.ProcessedAt, payment.CreatedAt, payment.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save payment: %w", err)
	}

	return nil
}

func (pm *PaymentManager) processWithProvider(ctx context.Context, payment *Payment) (string, error) {
	switch pm.config.Provider {
	case "stripe":
		return pm.processWithStripe(ctx, payment)
	case "paypal":
		return pm.processWithPayPal(ctx, payment)
	case "manual":
		return "", nil // Manual payments don't need provider processing
	default:
		return "", fmt.Errorf("unsupported payment provider: %s", pm.config.Provider)
	}
}

func (pm *PaymentManager) refundWithProvider(ctx context.Context, providerPaymentID string, amount int64) error {
	switch pm.config.Provider {
	case "stripe":
		return pm.refundWithStripe(ctx, providerPaymentID, amount)
	case "paypal":
		return pm.refundWithPayPal(ctx, providerPaymentID, amount)
	case "manual":
		return nil // Manual refunds don't need provider processing
	default:
		return fmt.Errorf("unsupported payment provider: %s", pm.config.Provider)
	}
}

// Provider-specific implementations (would be in separate files)

func (pm *PaymentManager) processWithStripe(ctx context.Context, payment *Payment) (string, error) {
	// Implementation would use Stripe API
	// For now, return a mock ID
	return fmt.Sprintf("stripe_pi_%d", time.Now().UnixNano()), nil
}

func (pm *PaymentManager) processWithPayPal(ctx context.Context, payment *Payment) (string, error) {
	// Implementation would use PayPal API
	// For now, return a mock ID
	return fmt.Sprintf("paypal_payment_%d", time.Now().UnixNano()), nil
}

func (pm *PaymentManager) refundWithStripe(ctx context.Context, providerPaymentID string, amount int64) error {
	// Implementation would use Stripe API
	return nil
}

func (pm *PaymentManager) refundWithPayPal(ctx context.Context, providerPaymentID string, amount int64) error {
	// Implementation would use PayPal API
	return nil
}

func (pm *PaymentManager) generatePaymentID() string {
	return fmt.Sprintf("pay_%d", time.Now().UnixNano())
}