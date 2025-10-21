package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Queue manages webhook delivery queue
type Queue struct {
	db       *db.DB
	config   *config.WebhooksConfig
	logger   *logrus.Logger
	items    chan *QueueItem
	workers  int
	mutex    sync.RWMutex
	running  bool
}

// QueueItem represents an item in the webhook delivery queue
type QueueItem struct {
	Delivery *Delivery `json:"delivery"`
	Event    *Event    `json:"event"`
	Webhook  *Webhook  `json:"webhook"`
	Priority int       `json:"priority"`
	Retries  int       `json:"retries"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

// QueueStatus represents the current status of the queue
type QueueStatus struct {
	QueueSize      int   `json:"queue_size"`
	ActiveWorkers  int   `json:"active_workers"`
	ProcessedTotal int64 `json:"processed_total"`
	FailedTotal    int64 `json:"failed_total"`
	LastProcessed  *time.Time `json:"last_processed,omitempty"`
}

// NewQueue creates a new webhook delivery queue
func NewQueue(database *db.DB, cfg *config.WebhooksConfig, logger *logrus.Logger) (*Queue, error) {
	return &Queue{
		db:      database,
		config:  cfg,
		logger:  logger,
		items:   make(chan *QueueItem, 1000), // Buffer up to 1000 items
		workers: 5, // Default number of worker goroutines
	}, nil
}

// Start starts the queue workers
func (q *Queue) Start(ctx context.Context) {
	q.mutex.Lock()
	if q.running {
		q.mutex.Unlock()
		return
	}
	q.running = true
	q.mutex.Unlock()

	q.logger.WithField("workers", q.workers).Info("Starting webhook queue workers")

	// Start worker goroutines
	for i := 0; i < q.workers; i++ {
		go q.worker(ctx, i)
	}

	// Start queue persistence worker
	go q.persistenceWorker(ctx)

	// Start cleanup worker
	go q.cleanupWorker(ctx)
}

// Stop stops the queue workers
func (q *Queue) Stop() {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if !q.running {
		return
	}

	q.running = false
	close(q.items)
	q.logger.Info("Webhook queue stopped")
}

// Enqueue adds a delivery to the queue
func (q *Queue) Enqueue(ctx context.Context, delivery *Delivery, event *Event, webhook *Webhook) error {
	q.mutex.RLock()
	running := q.running
	q.mutex.RUnlock()

	if !running {
		return fmt.Errorf("queue is not running")
	}

	item := &QueueItem{
		Delivery:   delivery,
		Event:      event,
		Webhook:    webhook,
		Priority:   q.calculatePriority(event.Type),
		Retries:    0,
		EnqueuedAt: time.Now(),
	}

	// Try to add to in-memory queue
	select {
	case q.items <- item:
		q.logger.WithFields(logrus.Fields{
			"delivery_id": delivery.ID,
			"event_type":  event.Type,
			"webhook_id":  webhook.ID,
		}).Debug("Item enqueued")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Queue is full, persist to database
		return q.persistQueueItem(ctx, item)
	}
}

// EnqueueWithPriority adds a delivery to the queue with custom priority
func (q *Queue) EnqueueWithPriority(ctx context.Context, delivery *Delivery, event *Event, webhook *Webhook, priority int) error {
	q.mutex.RLock()
	running := q.running
	q.mutex.RUnlock()

	if !running {
		return fmt.Errorf("queue is not running")
	}

	item := &QueueItem{
		Delivery:   delivery,
		Event:      event,
		Webhook:    webhook,
		Priority:   priority,
		Retries:    0,
		EnqueuedAt: time.Now(),
	}

	select {
	case q.items <- item:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return q.persistQueueItem(ctx, item)
	}
}

// GetStatus returns the current queue status
func (q *Queue) GetStatus(ctx context.Context) (*QueueStatus, error) {
	status := &QueueStatus{
		QueueSize:     len(q.items),
		ActiveWorkers: q.workers,
	}

	// Get statistics from database
	if err := q.getQueueStats(ctx, status); err != nil {
		q.logger.WithError(err).Warn("Failed to get queue stats from database")
	}

	return status, nil
}

// worker processes items from the queue
func (q *Queue) worker(ctx context.Context, workerID int) {
	q.logger.WithField("worker_id", workerID).Debug("Queue worker started")

	for {
		select {
		case <-ctx.Done():
			q.logger.WithField("worker_id", workerID).Debug("Queue worker stopped")
			return
		case item, ok := <-q.items:
			if !ok {
				q.logger.WithField("worker_id", workerID).Debug("Queue worker stopped - channel closed")
				return
			}

			q.processQueueItem(ctx, item, workerID)
		}
	}
}

// processQueueItem processes a single queue item
func (q *Queue) processQueueItem(ctx context.Context, item *QueueItem, workerID int) {
	q.logger.WithFields(logrus.Fields{
		"worker_id":   workerID,
		"delivery_id": item.Delivery.ID,
		"event_type":  item.Event.Type,
		"attempt":     item.Retries + 1,
	}).Debug("Processing queue item")

	// Check if webhook is still active
	if !item.Webhook.Active {
		q.logger.WithField("webhook_id", item.Webhook.ID).Debug("Skipping inactive webhook")
		return
	}

	// Create dispatcher for immediate delivery
	dispatcher, err := NewDispatcher(q.db, q.config, q.logger, nil, nil)
	if err != nil {
		q.logger.WithError(err).Error("Failed to create dispatcher")
		return
	}

	// Attempt delivery
	success, err := dispatcher.Dispatch(ctx, item.Delivery, item.Event, item.Webhook)
	if err != nil {
		q.logger.WithError(err).WithField("delivery_id", item.Delivery.ID).Error("Delivery failed")
	}

	// Handle retry logic
	if !success && item.Retries < q.config.RetryAttempts {
		item.Retries++

		// Calculate backoff delay
		backoffDelay := q.calculateBackoffDelay(item.Retries)

		// Re-enqueue with delay
		go func() {
			time.Sleep(backoffDelay)
			if err := q.Enqueue(ctx, item.Delivery, item.Event, item.Webhook); err != nil {
				q.logger.WithError(err).Error("Failed to re-enqueue item")
			}
		}()

		q.logger.WithFields(logrus.Fields{
			"delivery_id": item.Delivery.ID,
			"retries":     item.Retries,
			"backoff":     backoffDelay,
		}).Info("Re-queuing failed delivery")
	}

	// Update statistics
	q.updateQueueStats(ctx, success)
}

// persistenceWorker periodically loads items from database into memory queue
func (q *Queue) persistenceWorker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := q.loadPersistedItems(ctx); err != nil {
				q.logger.WithError(err).Error("Failed to load persisted queue items")
			}
		}
	}
}

// cleanupWorker periodically cleans up old queue items and statistics
func (q *Queue) cleanupWorker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := q.cleanupOldItems(ctx); err != nil {
				q.logger.WithError(err).Error("Failed to cleanup old queue items")
			}
		}
	}
}

// persistQueueItem saves a queue item to database when in-memory queue is full
func (q *Queue) persistQueueItem(ctx context.Context, item *QueueItem) error {
	// Serialize the queue item
	itemData, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal queue item: %w", err)
	}

	query := `
		INSERT INTO webhook_queue (id, data, priority, retries, enqueued_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err = q.db.ExecContext(ctx, query,
		q.generateQueueItemID(), string(itemData), item.Priority,
		item.Retries, item.EnqueuedAt, time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to persist queue item: %w", err)
	}

	q.logger.WithField("delivery_id", item.Delivery.ID).Debug("Queue item persisted to database")
	return nil
}

// loadPersistedItems loads items from database into memory queue
func (q *Queue) loadPersistedItems(ctx context.Context) error {
	// Check if in-memory queue has space
	queueSpace := cap(q.items) - len(q.items)
	if queueSpace <= 0 {
		return nil // No space in queue
	}

	// Limit to available space
	if queueSpace > 100 {
		queueSpace = 100
	}

	query := `
		SELECT id, data FROM webhook_queue
		ORDER BY priority DESC, enqueued_at ASC
		LIMIT ?`

	rows, err := q.db.QueryContext(ctx, query, queueSpace)
	if err != nil {
		return fmt.Errorf("failed to query persisted queue items: %w", err)
	}
	defer rows.Close()

	var loadedItems []string

	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			q.logger.WithError(err).Warn("Failed to scan queue item")
			continue
		}

		// Deserialize queue item
		var item QueueItem
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			q.logger.WithError(err).WithField("item_id", id).Warn("Failed to unmarshal queue item")
			continue
		}

		// Try to add to in-memory queue
		select {
		case q.items <- &item:
			loadedItems = append(loadedItems, id)
		default:
			// Queue is full, stop loading
			break
		}
	}

	// Remove loaded items from database
	if len(loadedItems) > 0 {
		for _, id := range loadedItems {
			_, err := q.db.ExecContext(ctx, "DELETE FROM webhook_queue WHERE id = ?", id)
			if err != nil {
				q.logger.WithError(err).WithField("item_id", id).Error("Failed to delete loaded queue item")
			}
		}

		q.logger.WithField("count", len(loadedItems)).Debug("Loaded persisted queue items")
	}

	return nil
}

// cleanupOldItems removes old queue items and statistics
func (q *Queue) cleanupOldItems(ctx context.Context) error {
	cutoff := time.Now().AddDate(0, 0, -7) // 7 days old

	// Clean up old persisted queue items
	result, err := q.db.ExecContext(ctx, "DELETE FROM webhook_queue WHERE created_at < ?", cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup old queue items: %w", err)
	}

	if rowsAffected, _ := result.RowsAffected(); rowsAffected > 0 {
		q.logger.WithField("rows_affected", rowsAffected).Info("Cleaned up old queue items")
	}

	return nil
}

// calculatePriority calculates priority based on event type
func (q *Queue) calculatePriority(eventType string) int {
	switch eventType {
	case "ping":
		return 100 // Highest priority for ping events
	case EventTypeURLClicked:
		return 90 // High priority for click events
	case EventTypeURLCreated, EventTypeURLUpdated, EventTypeURLDeleted:
		return 80 // High priority for URL events
	case EventTypeUserCreated, EventTypeUserUpdated, EventTypeUserDeleted:
		return 70 // Medium priority for user events
	case EventTypeBulkImported, EventTypeBulkExported:
		return 50 // Lower priority for bulk events
	default:
		return 60 // Default priority
	}
}

// calculateBackoffDelay calculates exponential backoff delay
func (q *Queue) calculateBackoffDelay(retries int) time.Duration {
	// Exponential backoff: 1s, 2s, 4s, 8s, 16s, etc.
	seconds := 1 << (retries - 1)
	if seconds > 300 { // Cap at 5 minutes
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

// generateQueueItemID generates a unique ID for queue items
func (q *Queue) generateQueueItemID() string {
	return fmt.Sprintf("queue_%d", time.Now().UnixNano())
}

// getQueueStats retrieves queue statistics from database
func (q *Queue) getQueueStats(ctx context.Context, status *QueueStatus) error {
	// Get processed count
	err := q.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM webhook_deliveries WHERE success = true").Scan(&status.ProcessedTotal)
	if err != nil {
		q.logger.WithError(err).Warn("Failed to get processed count")
	}

	// Get failed count
	err = q.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM webhook_deliveries WHERE success = false").Scan(&status.FailedTotal)
	if err != nil {
		q.logger.WithError(err).Warn("Failed to get failed count")
	}

	// Get last processed time
	var lastProcessed *time.Time
	err = q.db.QueryRowContext(ctx, "SELECT MAX(delivered_at) FROM webhook_deliveries WHERE success = true").Scan(&lastProcessed)
	if err == nil && lastProcessed != nil {
		status.LastProcessed = lastProcessed
	}

	return nil
}

// updateQueueStats updates queue processing statistics
func (q *Queue) updateQueueStats(ctx context.Context, success bool) {
	// This is a simplified implementation
	// In a production system, you might want to maintain more detailed statistics
	q.logger.WithField("success", success).Debug("Updated queue stats")
}

// Drain waits for all queued items to be processed
func (q *Queue) Drain(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if len(q.items) == 0 {
			// Check database queue as well
			var count int
			err := q.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM webhook_queue").Scan(&count)
			if err != nil {
				return fmt.Errorf("failed to check database queue: %w", err)
			}

			if count == 0 {
				q.logger.Info("Queue drained successfully")
				return nil
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for queue to drain")
}

// SetWorkerCount adjusts the number of worker goroutines
func (q *Queue) SetWorkerCount(count int) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if count < 1 {
		count = 1
	}
	if count > 50 {
		count = 50
	}

	q.workers = count
	q.logger.WithField("workers", count).Info("Updated worker count")
}