package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// InvoiceManager handles invoice generation and management
type InvoiceManager struct {
	db     *db.DB
	config *config.BillingConfig
	logger *logrus.Logger
}

// NewInvoiceManager creates a new invoice manager
func NewInvoiceManager(database *db.DB, cfg *config.BillingConfig, logger *logrus.Logger) (*InvoiceManager, error) {
	return &InvoiceManager{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// InvoiceStatus represents invoice status
type InvoiceStatus string

const (
	InvoiceStatusDraft     InvoiceStatus = "draft"
	InvoiceStatusSent      InvoiceStatus = "sent"
	InvoiceStatusPaid      InvoiceStatus = "paid"
	InvoiceStatusOverdue   InvoiceStatus = "overdue"
	InvoiceStatusCanceled  InvoiceStatus = "canceled"
)

// Invoice represents an invoice
type Invoice struct {
	ID             string        `json:"id" db:"id"`
	UserID         string        `json:"user_id" db:"user_id"`
	SubscriptionID string        `json:"subscription_id" db:"subscription_id"`
	Number         string        `json:"number" db:"number"`
	Status         InvoiceStatus `json:"status" db:"status"`
	Currency       string        `json:"currency" db:"currency"`
	Subtotal       int64         `json:"subtotal" db:"subtotal"`         // in cents
	TaxAmount      int64         `json:"tax_amount" db:"tax_amount"`     // in cents
	Total          int64         `json:"total" db:"total"`               // in cents
	DueDate        time.Time     `json:"due_date" db:"due_date"`
	PaidAt         *time.Time    `json:"paid_at" db:"paid_at"`
	CreatedAt      time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at" db:"updated_at"`

	// Line items
	Items []*InvoiceItem `json:"items,omitempty"`
}

// InvoiceItem represents an invoice line item
type InvoiceItem struct {
	ID          string `json:"id" db:"id"`
	InvoiceID   string `json:"invoice_id" db:"invoice_id"`
	Description string `json:"description" db:"description"`
	Quantity    int64  `json:"quantity" db:"quantity"`
	UnitPrice   int64  `json:"unit_price" db:"unit_price"` // in cents
	Amount      int64  `json:"amount" db:"amount"`         // in cents
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// GenerateInvoice generates an invoice for a subscription
func (im *InvoiceManager) GenerateInvoice(ctx context.Context, subscriptionID string) (*Invoice, error) {
	// Get subscription details
	subscriptionManager := &SubscriptionManager{db: im.db, config: im.config, logger: im.logger}
	subscription, err := subscriptionManager.GetSubscription(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	now := time.Now()
	invoice := &Invoice{
		ID:             im.generateInvoiceID(),
		UserID:         subscription.UserID,
		SubscriptionID: subscriptionID,
		Number:         im.generateInvoiceNumber(),
		Status:         InvoiceStatusDraft,
		Currency:       subscription.Plan.Currency,
		DueDate:        now.AddDate(0, 0, 30), // 30 days to pay
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Calculate invoice amount based on plan and billing cycle
	var amount int64
	var description string

	switch subscription.BillingCycle {
	case CycleMonthly:
		amount = subscription.Plan.PriceMonthly
		description = fmt.Sprintf("%s - Monthly Subscription", subscription.Plan.DisplayName)
	case CycleYearly:
		amount = subscription.Plan.PriceYearly
		description = fmt.Sprintf("%s - Yearly Subscription", subscription.Plan.DisplayName)
	default:
		amount = subscription.Plan.PriceMonthly
		description = fmt.Sprintf("%s - Monthly Subscription", subscription.Plan.DisplayName)
	}

	// Add usage-based charges if applicable
	usageCharges, err := im.calculateUsageCharges(ctx, subscription)
	if err != nil {
		im.logger.WithError(err).Warn("Failed to calculate usage charges")
	}

	// Create invoice items
	items := []*InvoiceItem{
		{
			ID:          im.generateItemID(),
			InvoiceID:   invoice.ID,
			Description: description,
			Quantity:    1,
			UnitPrice:   amount,
			Amount:      amount,
			CreatedAt:   now,
		},
	}

	// Add usage-based items
	for _, charge := range usageCharges {
		items = append(items, &InvoiceItem{
			ID:          im.generateItemID(),
			InvoiceID:   invoice.ID,
			Description: charge.Description,
			Quantity:    charge.Quantity,
			UnitPrice:   charge.UnitPrice,
			Amount:      charge.Amount,
			CreatedAt:   now,
		})
		amount += charge.Amount
	}

	invoice.Subtotal = amount
	invoice.TaxAmount = im.calculateTax(amount, subscription.UserID)
	invoice.Total = invoice.Subtotal + invoice.TaxAmount
	invoice.Items = items

	// Save invoice to database
	if err := im.saveInvoice(ctx, invoice); err != nil {
		return nil, fmt.Errorf("failed to save invoice: %w", err)
	}

	im.logger.WithFields(logrus.Fields{
		"invoice_id":      invoice.ID,
		"subscription_id": subscriptionID,
		"amount":          invoice.Total,
	}).Info("Invoice generated")

	return invoice, nil
}

// GetInvoice gets an invoice by ID
func (im *InvoiceManager) GetInvoice(ctx context.Context, invoiceID string) (*Invoice, error) {
	query := `
		SELECT id, user_id, subscription_id, number, status, currency,
		       subtotal, tax_amount, total, due_date, paid_at, created_at, updated_at
		FROM invoices
		WHERE id = ?`

	row := im.db.QueryRowContext(ctx, query, invoiceID)

	invoice := &Invoice{}
	err := row.Scan(
		&invoice.ID, &invoice.UserID, &invoice.SubscriptionID,
		&invoice.Number, &invoice.Status, &invoice.Currency,
		&invoice.Subtotal, &invoice.TaxAmount, &invoice.Total,
		&invoice.DueDate, &invoice.PaidAt, &invoice.CreatedAt, &invoice.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoice: %w", err)
	}

	// Load invoice items
	items, err := im.getInvoiceItems(ctx, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoice items: %w", err)
	}
	invoice.Items = items

	return invoice, nil
}

// GetUserInvoices gets invoices for a user
func (im *InvoiceManager) GetUserInvoices(ctx context.Context, userID string, limit, offset int) ([]*Invoice, error) {
	query := `
		SELECT id, user_id, subscription_id, number, status, currency,
		       subtotal, tax_amount, total, due_date, paid_at, created_at, updated_at
		FROM invoices
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`

	rows, err := im.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query user invoices: %w", err)
	}
	defer rows.Close()

	var invoices []*Invoice
	for rows.Next() {
		invoice := &Invoice{}
		err := rows.Scan(
			&invoice.ID, &invoice.UserID, &invoice.SubscriptionID,
			&invoice.Number, &invoice.Status, &invoice.Currency,
			&invoice.Subtotal, &invoice.TaxAmount, &invoice.Total,
			&invoice.DueDate, &invoice.PaidAt, &invoice.CreatedAt, &invoice.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan invoice: %w", err)
		}

		// Load items for each invoice (could be optimized with a join)
		items, err := im.getInvoiceItems(ctx, invoice.ID)
		if err != nil {
			im.logger.WithError(err).Warn("Failed to load invoice items")
		} else {
			invoice.Items = items
		}

		invoices = append(invoices, invoice)
	}

	return invoices, nil
}

// MarkInvoicePaid marks an invoice as paid
func (im *InvoiceManager) MarkInvoicePaid(ctx context.Context, invoiceID string, paidAt time.Time) error {
	query := `
		UPDATE invoices SET
			status = ?, paid_at = ?, updated_at = ?
		WHERE id = ?`

	_, err := im.db.ExecContext(ctx, query, InvoiceStatusPaid, paidAt, time.Now(), invoiceID)
	if err != nil {
		return fmt.Errorf("failed to mark invoice as paid: %w", err)
	}

	im.logger.WithFields(logrus.Fields{
		"invoice_id": invoiceID,
		"paid_at":    paidAt,
	}).Info("Invoice marked as paid")

	return nil
}

// UpdateInvoiceStatus updates an invoice status
func (im *InvoiceManager) UpdateInvoiceStatus(ctx context.Context, invoiceID string, status InvoiceStatus) error {
	query := `
		UPDATE invoices SET
			status = ?, updated_at = ?
		WHERE id = ?`

	_, err := im.db.ExecContext(ctx, query, status, time.Now(), invoiceID)
	if err != nil {
		return fmt.Errorf("failed to update invoice status: %w", err)
	}

	im.logger.WithFields(logrus.Fields{
		"invoice_id": invoiceID,
		"status":     status,
	}).Info("Invoice status updated")

	return nil
}

// GetOverdueInvoices gets overdue invoices
func (im *InvoiceManager) GetOverdueInvoices(ctx context.Context) ([]*Invoice, error) {
	query := `
		SELECT id, user_id, subscription_id, number, status, currency,
		       subtotal, tax_amount, total, due_date, paid_at, created_at, updated_at
		FROM invoices
		WHERE status IN ('sent', 'overdue') AND due_date < ?
		ORDER BY due_date ASC`

	rows, err := im.db.QueryContext(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to query overdue invoices: %w", err)
	}
	defer rows.Close()

	var invoices []*Invoice
	for rows.Next() {
		invoice := &Invoice{}
		err := rows.Scan(
			&invoice.ID, &invoice.UserID, &invoice.SubscriptionID,
			&invoice.Number, &invoice.Status, &invoice.Currency,
			&invoice.Subtotal, &invoice.TaxAmount, &invoice.Total,
			&invoice.DueDate, &invoice.PaidAt, &invoice.CreatedAt, &invoice.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan overdue invoice: %w", err)
		}
		invoices = append(invoices, invoice)
	}

	return invoices, nil
}

// Helper methods

func (im *InvoiceManager) saveInvoice(ctx context.Context, invoice *Invoice) error {
	tx, err := im.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Save invoice
	query := `
		INSERT INTO invoices (
			id, user_id, subscription_id, number, status, currency,
			subtotal, tax_amount, total, due_date, paid_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = tx.ExecContext(ctx, query,
		invoice.ID, invoice.UserID, invoice.SubscriptionID,
		invoice.Number, invoice.Status, invoice.Currency,
		invoice.Subtotal, invoice.TaxAmount, invoice.Total,
		invoice.DueDate, invoice.PaidAt, invoice.CreatedAt, invoice.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save invoice: %w", err)
	}

	// Save invoice items
	for _, item := range invoice.Items {
		itemQuery := `
			INSERT INTO invoice_items (
				id, invoice_id, description, quantity, unit_price, amount, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)`

		_, err = tx.ExecContext(ctx, itemQuery,
			item.ID, item.InvoiceID, item.Description,
			item.Quantity, item.UnitPrice, item.Amount, item.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to save invoice item: %w", err)
		}
	}

	return tx.Commit()
}

func (im *InvoiceManager) getInvoiceItems(ctx context.Context, invoiceID string) ([]*InvoiceItem, error) {
	query := `
		SELECT id, invoice_id, description, quantity, unit_price, amount, created_at
		FROM invoice_items
		WHERE invoice_id = ?
		ORDER BY created_at ASC`

	rows, err := im.db.QueryContext(ctx, query, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query invoice items: %w", err)
	}
	defer rows.Close()

	var items []*InvoiceItem
	for rows.Next() {
		item := &InvoiceItem{}
		err := rows.Scan(
			&item.ID, &item.InvoiceID, &item.Description,
			&item.Quantity, &item.UnitPrice, &item.Amount, &item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan invoice item: %w", err)
		}
		items = append(items, item)
	}

	return items, nil
}

// UsageCharge represents a usage-based charge
type UsageCharge struct {
	Description string
	Quantity    int64
	UnitPrice   int64
	Amount      int64
}

func (im *InvoiceManager) calculateUsageCharges(ctx context.Context, subscription *Subscription) ([]*UsageCharge, error) {
	// This would implement usage-based billing logic
	// For now, return empty charges
	return []*UsageCharge{}, nil
}

func (im *InvoiceManager) calculateTax(amount int64, userID string) int64 {
	// This would implement tax calculation based on user location
	// For now, return 0 (no tax)
	return 0
}

func (im *InvoiceManager) generateInvoiceID() string {
	return fmt.Sprintf("inv_%d", time.Now().UnixNano())
}

func (im *InvoiceManager) generateInvoiceNumber() string {
	return fmt.Sprintf("INV-%d", time.Now().UnixNano()%1000000)
}

func (im *InvoiceManager) generateItemID() string {
	return fmt.Sprintf("item_%d", time.Now().UnixNano())
}