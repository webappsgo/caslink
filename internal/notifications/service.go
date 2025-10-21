package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/casjaysdevdocker/caslink/internal/notifications/providers"
	"github.com/sirupsen/logrus"
)

// Service manages notification delivery
type Service struct {
	db            *db.DB
	config        *config.Config
	logger        *logrus.Logger
	email         *EmailService
	templates     *TemplateService
	smsProvider   *providers.TwilioProvider
	fcmProvider   *providers.FCMProvider
	apnsProvider  *providers.APNSProvider
	webhookProvider *providers.WebhookProvider
}

// Notification represents a notification message
type Notification struct {
	ID          string                 `json:"id" db:"id"`
	UserID      string                 `json:"user_id" db:"user_id"`
	Type        string                 `json:"type" db:"type"`
	Channel     string                 `json:"channel" db:"channel"`
	Subject     string                 `json:"subject" db:"subject"`
	Content     string                 `json:"content" db:"content"`
	Data        map[string]interface{} `json:"data" db:"data"`
	Status      string                 `json:"status" db:"status"`
	Priority    int                    `json:"priority" db:"priority"`
	ScheduledAt *time.Time             `json:"scheduled_at,omitempty" db:"scheduled_at"`
	SentAt      *time.Time             `json:"sent_at,omitempty" db:"sent_at"`
	DeliveredAt *time.Time             `json:"delivered_at,omitempty" db:"delivered_at"`
	Error       string                 `json:"error,omitempty" db:"error"`
	Attempts    int                    `json:"attempts" db:"attempts"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
}

// NotificationRequest represents a request to send a notification
type NotificationRequest struct {
	UserID      string                 `json:"user_id" validate:"required"`
	Type        string                 `json:"type" validate:"required"`
	Channel     string                 `json:"channel" validate:"required,oneof=email sms push webhook"`
	Subject     string                 `json:"subject"`
	Content     string                 `json:"content"`
	Template    string                 `json:"template,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Priority    int                    `json:"priority"`
	ScheduledAt *time.Time             `json:"scheduled_at,omitempty"`
}

// NotificationPreferences is now defined in db/models.go
type NotificationPreferences = db.NotificationPreferences

// NotificationStats represents notification statistics
type NotificationStats struct {
	TotalSent       int64              `json:"total_sent"`
	TotalDelivered  int64              `json:"total_delivered"`
	TotalFailed     int64              `json:"total_failed"`
	TotalPending    int64              `json:"total_pending"`
	DeliveryRate    float64            `json:"delivery_rate"`
	ChannelStats    map[string]int64   `json:"channel_stats"`
	TypeStats       map[string]int64   `json:"type_stats"`
	RecentActivity  []NotificationStat `json:"recent_activity"`
}

// NotificationStat represents a single notification statistic
type NotificationStat struct {
	Date      string `json:"date"`
	Sent      int64  `json:"sent"`
	Delivered int64  `json:"delivered"`
	Failed    int64  `json:"failed"`
}

// NotificationEvent represents notification events for webhooks
type NotificationEvent struct {
	Type           string       `json:"type"`
	Notification   *Notification `json:"notification"`
	Timestamp      time.Time    `json:"timestamp"`
	DeliveryStatus string       `json:"delivery_status"`
}

// NewService creates a new notification service
func NewService(database *db.DB, cfg *config.Config, logger *logrus.Logger) (*Service, error) {
	emailService, err := NewEmailService(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create email service: %w", err)
	}

	templateService, err := NewTemplateService(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create template service: %w", err)
	}

	// Initialize SMS provider if enabled
	var smsProvider *providers.TwilioProvider
	if cfg.Notifications.EnableSMS {
		smsProvider, err = providers.NewTwilioProvider(&cfg.Notifications, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create SMS provider: %w", err)
		}
	}

	// Initialize FCM provider if enabled
	var fcmProvider *providers.FCMProvider
	if cfg.Notifications.EnablePush && cfg.Notifications.PushProvider == "fcm" {
		fcmProvider, err = providers.NewFCMProvider(&cfg.Notifications, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create FCM provider: %w", err)
		}
	}

	// Initialize APNS provider if enabled
	var apnsProvider *providers.APNSProvider
	if cfg.Notifications.EnablePush && cfg.Notifications.PushProvider == "apns" {
		apnsProvider, err = providers.NewAPNSProvider(&cfg.Notifications, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create APNS provider: %w", err)
		}
	}

	// Initialize webhook provider if enabled
	var webhookProvider *providers.WebhookProvider
	if cfg.Notifications.EnableWebhook {
		webhookProvider, err = providers.NewWebhookProvider(&cfg.Notifications, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create webhook provider: %w", err)
		}
	}

	return &Service{
		db:              database,
		config:          cfg,
		logger:          logger,
		email:           emailService,
		templates:       templateService,
		smsProvider:     smsProvider,
		fcmProvider:     fcmProvider,
		apnsProvider:    apnsProvider,
		webhookProvider: webhookProvider,
	}, nil
}

// SendNotification sends a notification to a user
func (s *Service) SendNotification(ctx context.Context, req *NotificationRequest) (*Notification, error) {
	// Get user preferences
	prefs, err := s.GetUserPreferences(ctx, req.UserID)
	if err != nil {
		s.logger.WithError(err).WithField("user_id", req.UserID).Warn("Failed to get user preferences, using defaults")
		prefs = s.getDefaultPreferences(req.UserID)
	}

	// Check if notifications are enabled for this type and channel
	if !s.isNotificationAllowed(req, prefs) {
		return nil, fmt.Errorf("notification not allowed for user %s, type %s, channel %s", req.UserID, req.Type, req.Channel)
	}

	// Create notification record
	notification := &Notification{
		ID:        s.generateNotificationID(),
		UserID:    req.UserID,
		Type:      req.Type,
		Channel:   req.Channel,
		Subject:   req.Subject,
		Content:   req.Content,
		Data:      req.Data,
		Status:    "pending",
		Priority:  req.Priority,
		Attempts:  0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if req.ScheduledAt != nil {
		notification.ScheduledAt = req.ScheduledAt
		notification.Status = "scheduled"
	}

	// Process template if specified
	if req.Template != "" {
		if err := s.processTemplate(ctx, notification, req.Template, req.Data); err != nil {
			return nil, fmt.Errorf("failed to process template: %w", err)
		}
	}

	// Save to database
	if err := s.saveNotification(ctx, notification); err != nil {
		return nil, fmt.Errorf("failed to save notification: %w", err)
	}

	// Send immediately if not scheduled
	if req.ScheduledAt == nil {
		go s.deliverNotification(ctx, notification, prefs)
	}

	s.logger.WithFields(logrus.Fields{
		"notification_id": notification.ID,
		"user_id":        req.UserID,
		"type":           req.Type,
		"channel":        req.Channel,
	}).Info("Notification created")

	return notification, nil
}

// SendBulkNotification sends notifications to multiple users
func (s *Service) SendBulkNotification(ctx context.Context, userIDs []string, req *NotificationRequest) error {
	for _, userID := range userIDs {
		bulkReq := *req
		bulkReq.UserID = userID

		_, err := s.SendNotification(ctx, &bulkReq)
		if err != nil {
			s.logger.WithError(err).WithField("user_id", userID).Error("Failed to send bulk notification")
		}
	}

	return nil
}

// GetNotification retrieves a notification by ID
func (s *Service) GetNotification(ctx context.Context, notificationID string) (*Notification, error) {
	query := `
		SELECT id, user_id, type, channel, subject, content, data, status, priority,
		       scheduled_at, sent_at, delivered_at, error, attempts, created_at, updated_at
		FROM notifications
		WHERE id = ?`

	row := s.db.QueryRowContext(ctx, query, notificationID)

	notification := &Notification{}
	var dataJSON string

	err := row.Scan(
		&notification.ID, &notification.UserID, &notification.Type, &notification.Channel,
		&notification.Subject, &notification.Content, &dataJSON, &notification.Status,
		&notification.Priority, &notification.ScheduledAt, &notification.SentAt,
		&notification.DeliveredAt, &notification.Error, &notification.Attempts,
		&notification.CreatedAt, &notification.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("notification not found: %w", err)
	}

	// Parse data JSON
	if dataJSON != "" {
		if err := json.Unmarshal([]byte(dataJSON), &notification.Data); err != nil {
			s.logger.WithError(err).Warn("Failed to parse notification data")
		}
	}

	return notification, nil
}

// ListUserNotifications lists notifications for a user
func (s *Service) ListUserNotifications(ctx context.Context, userID string, limit, offset int) ([]*Notification, error) {
	query := `
		SELECT id, user_id, type, channel, subject, content, data, status, priority,
		       scheduled_at, sent_at, delivered_at, error, attempts, created_at, updated_at
		FROM notifications
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`

	rows, err := s.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*Notification
	for rows.Next() {
		notification := &Notification{}
		var dataJSON string

		err := rows.Scan(
			&notification.ID, &notification.UserID, &notification.Type, &notification.Channel,
			&notification.Subject, &notification.Content, &dataJSON, &notification.Status,
			&notification.Priority, &notification.ScheduledAt, &notification.SentAt,
			&notification.DeliveredAt, &notification.Error, &notification.Attempts,
			&notification.CreatedAt, &notification.UpdatedAt,
		)

		if err != nil {
			s.logger.WithError(err).Warn("Failed to scan notification")
			continue
		}

		// Parse data JSON
		if dataJSON != "" {
			if err := json.Unmarshal([]byte(dataJSON), &notification.Data); err != nil {
				s.logger.WithError(err).Warn("Failed to parse notification data")
			}
		}

		notifications = append(notifications, notification)
	}

	return notifications, nil
}

// GetUserPreferences retrieves user notification preferences
func (s *Service) GetUserPreferences(ctx context.Context, userID string) (*NotificationPreferences, error) {
	query := `
		SELECT user_id, email_enabled, email_address, sms_enabled, phone_number,
		       push_enabled, webhook_enabled, webhook_url, notification_types,
		       quiet_hours_enabled, quiet_hours_start, quiet_hours_end,
		       timezone, language, updated_at
		FROM notification_preferences
		WHERE user_id = ?`

	row := s.db.QueryRowContext(ctx, query, userID)

	prefs := &NotificationPreferences{}
	var notificationTypesJSON string

	err := row.Scan(
		&prefs.UserID, &prefs.EmailEnabled, &prefs.EmailAddress,
		&prefs.SMSEnabled, &prefs.PhoneNumber, &prefs.PushEnabled,
		&prefs.WebhookEnabled, &prefs.WebhookURL, &notificationTypesJSON,
		&prefs.QuietHoursEnabled, &prefs.QuietHoursStart, &prefs.QuietHoursEnd,
		&prefs.Timezone, &prefs.Language, &prefs.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("preferences not found: %w", err)
	}

	// Parse notification types JSON
	if notificationTypesJSON != "" {
		if err := json.Unmarshal([]byte(notificationTypesJSON), &prefs.NotificationTypes); err != nil {
			s.logger.WithError(err).Warn("Failed to parse notification types")
			prefs.NotificationTypes = make(map[string]bool)
		}
	}

	return prefs, nil
}

// UpdateUserPreferences updates user notification preferences
func (s *Service) UpdateUserPreferences(ctx context.Context, prefs *NotificationPreferences) error {
	notificationTypesJSON, err := json.Marshal(prefs.NotificationTypes)
	if err != nil {
		return fmt.Errorf("failed to marshal notification types: %w", err)
	}

	prefs.UpdatedAt = time.Now()

	query := `
		INSERT OR REPLACE INTO notification_preferences
		(user_id, email_enabled, email_address, sms_enabled, phone_number,
		 push_enabled, webhook_enabled, webhook_url, notification_types,
		 quiet_hours_enabled, quiet_hours_start, quiet_hours_end,
		 timezone, language, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		prefs.UserID, prefs.EmailEnabled, prefs.EmailAddress,
		prefs.SMSEnabled, prefs.PhoneNumber, prefs.PushEnabled,
		prefs.WebhookEnabled, prefs.WebhookURL, string(notificationTypesJSON),
		prefs.QuietHoursEnabled, prefs.QuietHoursStart, prefs.QuietHoursEnd,
		prefs.Timezone, prefs.Language, prefs.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update preferences: %w", err)
	}

	s.logger.WithField("user_id", prefs.UserID).Info("Notification preferences updated")
	return nil
}

// GetNotificationStats returns notification statistics
func (s *Service) GetNotificationStats(ctx context.Context, days int) (*NotificationStats, error) {
	since := time.Now().AddDate(0, 0, -days)

	stats := &NotificationStats{
		ChannelStats: make(map[string]int64),
		TypeStats:    make(map[string]int64),
	}

	// Get total counts
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE created_at >= ? AND status IN ('sent', 'delivered')", since).Scan(&stats.TotalSent)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get total sent count")
	}

	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE created_at >= ? AND status = 'delivered'", since).Scan(&stats.TotalDelivered)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get total delivered count")
	}

	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE created_at >= ? AND status = 'failed'", since).Scan(&stats.TotalFailed)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get total failed count")
	}

	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE status IN ('pending', 'scheduled')", since).Scan(&stats.TotalPending)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get total pending count")
	}

	// Calculate delivery rate
	if stats.TotalSent > 0 {
		stats.DeliveryRate = float64(stats.TotalDelivered) / float64(stats.TotalSent) * 100
	}

	return stats, nil
}

// ProcessScheduledNotifications processes notifications that are ready to be sent
func (s *Service) ProcessScheduledNotifications(ctx context.Context) error {
	query := `
		SELECT id FROM notifications
		WHERE status = 'scheduled' AND scheduled_at <= ?
		ORDER BY priority DESC, scheduled_at ASC
		LIMIT 100`

	rows, err := s.db.QueryContext(ctx, query, time.Now())
	if err != nil {
		return fmt.Errorf("failed to query scheduled notifications: %w", err)
	}
	defer rows.Close()

	var notificationIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			s.logger.WithError(err).Warn("Failed to scan notification ID")
			continue
		}
		notificationIDs = append(notificationIDs, id)
	}

	// Process each notification
	for _, id := range notificationIDs {
		notification, err := s.GetNotification(ctx, id)
		if err != nil {
			s.logger.WithError(err).WithField("notification_id", id).Error("Failed to get notification")
			continue
		}

		prefs, err := s.GetUserPreferences(ctx, notification.UserID)
		if err != nil {
			prefs = s.getDefaultPreferences(notification.UserID)
		}

		go s.deliverNotification(ctx, notification, prefs)
	}

	return nil
}

// Helper methods

func (s *Service) deliverNotification(ctx context.Context, notification *Notification, prefs *NotificationPreferences) {
	// Check quiet hours
	if s.isQuietHours(prefs) {
		s.logger.WithField("notification_id", notification.ID).Debug("Skipping notification due to quiet hours")
		return
	}

	notification.Status = "sending"
	notification.Attempts++
	notification.UpdatedAt = time.Now()

	var err error
	switch notification.Channel {
	case "email":
		err = s.email.SendEmail(ctx, notification, prefs)
	case "sms":
		err = s.sendSMS(ctx, notification, prefs)
	case "push":
		err = s.sendPushNotification(ctx, notification, prefs)
	case "webhook":
		err = s.sendWebhookNotification(ctx, notification, prefs)
	default:
		err = fmt.Errorf("unsupported channel: %s", notification.Channel)
	}

	if err != nil {
		notification.Status = "failed"
		notification.Error = err.Error()
		s.logger.WithError(err).WithField("notification_id", notification.ID).Error("Failed to deliver notification")
	} else {
		notification.Status = "delivered"
		now := time.Now()
		notification.SentAt = &now
		notification.DeliveredAt = &now
		s.logger.WithField("notification_id", notification.ID).Info("Notification delivered successfully")
	}

	s.saveNotification(ctx, notification)
}

func (s *Service) isNotificationAllowed(req *NotificationRequest, prefs *NotificationPreferences) bool {
	// Check channel-specific preferences
	switch req.Channel {
	case "email":
		return prefs.EmailEnabled
	case "sms":
		return prefs.SMSEnabled
	case "push":
		return prefs.PushEnabled
	case "webhook":
		return prefs.WebhookEnabled
	}

	// Check notification type preferences
	if prefs.NotificationTypes != nil {
		if allowed, exists := prefs.NotificationTypes[req.Type]; exists {
			return allowed
		}
	}

	return true // Default to allowing notifications
}

func (s *Service) isQuietHours(prefs *NotificationPreferences) bool {
	if !prefs.QuietHoursEnabled {
		return false
	}

	// TODO: Implement quiet hours checking based on timezone
	return false
}

func (s *Service) processTemplate(ctx context.Context, notification *Notification, templateName string, data map[string]interface{}) error {
	template, err := s.templates.GetTemplate(ctx, templateName)
	if err != nil {
		return err
	}

	subject, content, err := s.templates.RenderTemplate(template, data)
	if err != nil {
		return err
	}

	notification.Subject = subject
	notification.Content = content
	return nil
}

func (s *Service) saveNotification(ctx context.Context, notification *Notification) error {
	dataJSON, err := json.Marshal(notification.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	query := `
		INSERT OR REPLACE INTO notifications
		(id, user_id, type, channel, subject, content, data, status, priority,
		 scheduled_at, sent_at, delivered_at, error, attempts, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		notification.ID, notification.UserID, notification.Type, notification.Channel,
		notification.Subject, notification.Content, string(dataJSON), notification.Status,
		notification.Priority, notification.ScheduledAt, notification.SentAt,
		notification.DeliveredAt, notification.Error, notification.Attempts,
		notification.CreatedAt, notification.UpdatedAt,
	)

	return err
}

func (s *Service) getDefaultPreferences(userID string) *NotificationPreferences {
	return &NotificationPreferences{
		UserID:            userID,
		EmailEnabled:      true,
		SMSEnabled:        false,
		PushEnabled:       false,
		WebhookEnabled:    false,
		NotificationTypes: make(map[string]bool),
		Timezone:          "UTC",
		Language:          "en",
		UpdatedAt:         time.Now(),
	}
}

func (s *Service) generateNotificationID() string {
	return fmt.Sprintf("notif_%d", time.Now().UnixNano())
}

func (s *Service) sendSMS(ctx context.Context, notification *Notification, prefs *NotificationPreferences) error {
	if s.smsProvider == nil {
		return fmt.Errorf("SMS provider not configured")
	}

	if prefs.PhoneNumber == nil || *prefs.PhoneNumber == "" {
		return fmt.Errorf("user has no phone number configured")
	}

	smsMessage := &providers.SMSMessage{
		To:   *prefs.PhoneNumber,
		Body: notification.Content,
	}

	err := s.smsProvider.SendSMS(ctx, smsMessage)
	if err != nil {
		return fmt.Errorf("failed to send SMS: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"notification_id": notification.ID,
		"phone_number":   *prefs.PhoneNumber,
		"provider":       "twilio",
	}).Info("SMS notification sent successfully")

	return nil
}

func (s *Service) sendPushNotification(ctx context.Context, notification *Notification, prefs *NotificationPreferences) error {
	if !s.config.Notifications.EnablePush {
		return fmt.Errorf("push notifications not enabled")
	}

	if prefs.PushToken == nil || *prefs.PushToken == "" {
		return fmt.Errorf("user has no push token configured")
	}

	var err error
	pushMessage := &providers.PushMessage{
		Token: *prefs.PushToken,
		Title: notification.Subject,
		Body:  notification.Content,
		Data:  make(map[string]string),
	}

	// Add notification data
	if notification.Data != nil {
		for key, value := range notification.Data {
			if strValue, ok := value.(string); ok {
				pushMessage.Data[key] = strValue
			}
		}
	}

	// Add notification metadata
	pushMessage.Data["notification_id"] = notification.ID
	pushMessage.Data["notification_type"] = notification.Type

	switch s.config.Notifications.PushProvider {
	case "fcm":
		if s.fcmProvider == nil {
			return fmt.Errorf("FCM provider not configured")
		}
		err = s.fcmProvider.SendPush(ctx, pushMessage)
	case "apns":
		if s.apnsProvider == nil {
			return fmt.Errorf("APNS provider not configured")
		}
		err = s.apnsProvider.SendPush(ctx, pushMessage)
	default:
		return fmt.Errorf("unsupported push provider: %s", s.config.Notifications.PushProvider)
	}

	if err != nil {
		return fmt.Errorf("failed to send push notification: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"notification_id": notification.ID,
		"push_token":     *prefs.PushToken,
		"provider":       s.config.Notifications.PushProvider,
	}).Info("Push notification sent successfully")

	return nil
}

func (s *Service) sendWebhookNotification(ctx context.Context, notification *Notification, prefs *NotificationPreferences) error {
	if s.webhookProvider == nil {
		return fmt.Errorf("webhook provider not configured")
	}

	if prefs.WebhookURL == nil || *prefs.WebhookURL == "" {
		return fmt.Errorf("user has no webhook URL configured")
	}

	// Create standard webhook payload
	payload := s.webhookProvider.CreateStandardPayload(
		notification.ID,
		notification.Type,
		notification.Subject,
		notification.Content,
		&notification.UserID,
		notification.Data,
	)

	webhookMessage := &providers.WebhookMessage{
		URL:     *prefs.WebhookURL,
		Payload: map[string]interface{}{
			"event":       "notification",
			"timestamp":   payload.Timestamp,
			"source":      payload.Source,
			"id":          payload.ID,
			"type":        payload.Type,
			"subject":     payload.Subject,
			"content":     payload.Content,
			"data":        payload.Data,
			"user_id":     payload.UserID,
			"retry":       payload.Retry,
		},
	}

	// Add custom headers if any
	if notification.Data != nil {
		if headers, ok := notification.Data["webhook_headers"].(map[string]interface{}); ok {
			webhookMessage.Headers = make(map[string]string)
			for key, value := range headers {
				if strValue, ok := value.(string); ok {
					webhookMessage.Headers[key] = strValue
				}
			}
		}

		// Add webhook secret if configured
		if secret, ok := notification.Data["webhook_secret"].(string); ok {
			webhookMessage.Secret = secret
		}
	}

	err := s.webhookProvider.SendWebhook(ctx, webhookMessage)
	if err != nil {
		return fmt.Errorf("failed to send webhook notification: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"notification_id": notification.ID,
		"webhook_url":    *prefs.WebhookURL,
		"provider":       "webhook",
	}).Info("Webhook notification sent successfully")

	return nil
}