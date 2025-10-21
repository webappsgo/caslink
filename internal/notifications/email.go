package notifications

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
)

// EmailService handles email notifications
type EmailService struct {
	db     *db.DB
	config *config.Config
	logger *logrus.Logger
	dialer *gomail.Dialer
}

// EmailProvider represents different email providers
type EmailProvider interface {
	SendEmail(ctx context.Context, email *EmailMessage) error
	ValidateConfig() error
}

// EmailMessage represents an email message
type EmailMessage struct {
	To          []string          `json:"to"`
	CC          []string          `json:"cc,omitempty"`
	BCC         []string          `json:"bcc,omitempty"`
	Subject     string            `json:"subject"`
	TextBody    string            `json:"text_body,omitempty"`
	HTMLBody    string            `json:"html_body,omitempty"`
	From        string            `json:"from"`
	ReplyTo     string            `json:"reply_to,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Attachments []EmailAttachment `json:"attachments,omitempty"`
}

// EmailAttachment represents an email attachment
type EmailAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
}

// SMTPProvider implements EmailProvider for SMTP
type SMTPProvider struct {
	config *config.EmailConfig
	logger *logrus.Logger
}

// SendGridProvider implements EmailProvider for SendGrid
type SendGridProvider struct {
	apiKey string
	logger *logrus.Logger
}

// SESProvider implements EmailProvider for AWS SES
type SESProvider struct {
	region    string
	accessKey string
	secretKey string
	logger    *logrus.Logger
}

// NewEmailService creates a new email service
func NewEmailService(database *db.DB, cfg *config.Config, logger *logrus.Logger) (*EmailService, error) {
	var dialer *gomail.Dialer

	if cfg.Email.Enabled {
		switch cfg.Email.Provider {
		case "smtp":
			dialer = gomail.NewDialer(
				cfg.Email.SMTPHost,
				cfg.Email.SMTPPort,
				cfg.Email.SMTPUsername,
				cfg.Email.SMTPPassword,
			)

			if cfg.Email.SMTPTLS {
				dialer.TLSConfig = &tls.Config{
					InsecureSkipVerify: false,
					ServerName:         cfg.Email.SMTPHost,
				}
			}

		case "sendgrid":
			// SendGrid uses SMTP with specific configuration
			dialer = gomail.NewDialer(
				"smtp.sendgrid.net",
				587,
				"apikey",
				cfg.Email.SendGridAPIKey,
			)
			dialer.TLSConfig = &tls.Config{InsecureSkipVerify: false}

		case "ses":
			// AWS SES uses SMTP interface
			region := cfg.Email.SESRegion
			if region == "" {
				region = "us-east-1"
			}
			smtpHost := fmt.Sprintf("email-smtp.%s.amazonaws.com", region)

			dialer = gomail.NewDialer(
				smtpHost,
				587,
				cfg.Email.SESAccessKey,
				cfg.Email.SESSecretKey,
			)
			dialer.TLSConfig = &tls.Config{InsecureSkipVerify: false}

		default:
			return nil, fmt.Errorf("unsupported email provider: %s", cfg.Email.Provider)
		}
	}

	return &EmailService{
		db:     database,
		config: cfg,
		logger: logger,
		dialer: dialer,
	}, nil
}

// SendEmail sends an email notification
func (es *EmailService) SendEmail(ctx context.Context, notification *Notification, prefs *NotificationPreferences) error {
	if !es.config.Email.Enabled {
		return fmt.Errorf("email notifications are disabled")
	}

	if prefs.EmailAddress == "" {
		return fmt.Errorf("user has no email address configured")
	}

	email := &EmailMessage{
		To:       []string{prefs.EmailAddress},
		Subject:  notification.Subject,
		TextBody: notification.Content,
		HTMLBody: es.generateHTMLBody(notification),
		From:     es.config.Email.FromAddress,
		Headers: map[string]string{
			"X-Notification-ID":   notification.ID,
			"X-Notification-Type": notification.Type,
			"X-Mailer":           "Caslink URL Shortener",
		},
	}

	// Add custom headers from notification data
	if notification.Data != nil {
		if customHeaders, ok := notification.Data["headers"].(map[string]interface{}); ok {
			for key, value := range customHeaders {
				if strValue, ok := value.(string); ok {
					email.Headers[key] = strValue
				}
			}
		}
	}

	return es.sendEmailMessage(ctx, email)
}

// SendEmailMessage sends a raw email message
func (es *EmailService) SendEmailMessage(ctx context.Context, email *EmailMessage) error {
	if !es.config.Email.Enabled {
		return fmt.Errorf("email notifications are disabled")
	}

	return es.sendEmailMessage(ctx, email)
}

// sendEmailMessage handles the actual email sending
func (es *EmailService) sendEmailMessage(ctx context.Context, email *EmailMessage) error {
	message := gomail.NewMessage()

	// Set basic headers
	message.SetHeader("From", email.From)
	message.SetHeader("To", email.To...)

	if len(email.CC) > 0 {
		message.SetHeader("Cc", email.CC...)
	}

	if len(email.BCC) > 0 {
		message.SetHeader("Bcc", email.BCC...)
	}

	message.SetHeader("Subject", email.Subject)

	if email.ReplyTo != "" {
		message.SetHeader("Reply-To", email.ReplyTo)
	}

	// Set custom headers
	for key, value := range email.Headers {
		message.SetHeader(key, value)
	}

	// Set body content
	if email.HTMLBody != "" && email.TextBody != "" {
		message.SetBody("text/plain", email.TextBody)
		message.AddAlternative("text/html", email.HTMLBody)
	} else if email.HTMLBody != "" {
		message.SetBody("text/html", email.HTMLBody)
	} else {
		message.SetBody("text/plain", email.TextBody)
	}

	// Add attachments
	for _, attachment := range email.Attachments {
		message.Attach(attachment.Filename, gomail.SetCopyFunc(func(w gomail.Writer) error {
			_, err := w.Write(attachment.Data)
			return err
		}))
	}

	// Send the email
	if err := es.dialer.DialAndSend(message); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	es.logger.WithFields(logrus.Fields{
		"to":      strings.Join(email.To, ", "),
		"subject": email.Subject,
	}).Info("Email sent successfully")

	return nil
}

// generateHTMLBody generates HTML version of email content
func (es *EmailService) generateHTMLBody(notification *Notification) string {
	// Simple HTML wrapper for text content
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .header {
            background-color: #2c3e50;
            color: white;
            padding: 20px;
            text-align: center;
            border-radius: 8px 8px 0 0;
        }
        .content {
            background-color: #f8f9fa;
            padding: 20px;
            border-radius: 0 0 8px 8px;
        }
        .footer {
            margin-top: 20px;
            padding: 10px;
            font-size: 12px;
            color: #666;
            text-align: center;
        }
        .btn {
            display: inline-block;
            padding: 10px 20px;
            background-color: #3498db;
            color: white;
            text-decoration: none;
            border-radius: 4px;
            margin: 10px 0;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>%s</h1>
    </div>
    <div class="content">
        %s
    </div>
    <div class="footer">
        <p>This email was sent by %s</p>
        <p>If you no longer wish to receive these notifications, you can update your preferences in your account settings.</p>
    </div>
</body>
</html>`,
		notification.Subject,
		es.config.Application.BrandName,
		es.convertTextToHTML(notification.Content),
		es.config.Application.BrandName,
	)

	return html
}

// convertTextToHTML converts plain text to HTML
func (es *EmailService) convertTextToHTML(text string) string {
	// Simple text to HTML conversion
	html := strings.ReplaceAll(text, "\n", "<br>")
	html = strings.ReplaceAll(html, "\t", "&nbsp;&nbsp;&nbsp;&nbsp;")
	return html
}

// ValidateEmailConfiguration validates email configuration
func (es *EmailService) ValidateEmailConfiguration() error {
	if !es.config.Email.Enabled {
		return nil // Not enabled, so no need to validate
	}

	switch es.config.Email.Provider {
	case "smtp":
		return es.validateSMTPConfig()
	case "sendgrid":
		return es.validateSendGridConfig()
	case "ses":
		return es.validateSESConfig()
	default:
		return fmt.Errorf("unsupported email provider: %s", es.config.Email.Provider)
	}
}

// validateSMTPConfig validates SMTP configuration
func (es *EmailService) validateSMTPConfig() error {
	if es.config.Email.SMTPHost == "" {
		return fmt.Errorf("SMTP host is required")
	}

	if es.config.Email.SMTPPort == 0 {
		return fmt.Errorf("SMTP port is required")
	}

	if es.config.Email.FromAddress == "" {
		return fmt.Errorf("from address is required")
	}

	// Test connection
	return es.testSMTPConnection()
}

// validateSendGridConfig validates SendGrid configuration
func (es *EmailService) validateSendGridConfig() error {
	if es.config.Email.SendGridAPIKey == "" {
		return fmt.Errorf("SendGrid API key is required")
	}

	if es.config.Email.FromAddress == "" {
		return fmt.Errorf("from address is required")
	}

	return nil
}

// validateSESConfig validates AWS SES configuration
func (es *EmailService) validateSESConfig() error {
	if es.config.Email.SESAccessKey == "" {
		return fmt.Errorf("SES access key is required")
	}

	if es.config.Email.SESSecretKey == "" {
		return fmt.Errorf("SES secret key is required")
	}

	if es.config.Email.FromAddress == "" {
		return fmt.Errorf("from address is required")
	}

	return nil
}

// testSMTPConnection tests SMTP connection
func (es *EmailService) testSMTPConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a test connection
	auth := smtp.PlainAuth("",
		es.config.Email.SMTPUsername,
		es.config.Email.SMTPPassword,
		es.config.Email.SMTPHost,
	)

	addr := fmt.Sprintf("%s:%d", es.config.Email.SMTPHost, es.config.Email.SMTPPort)

	// Test connection
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer client.Close()

	// Start TLS if required
	if es.config.Email.SMTPTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         es.config.Email.SMTPHost,
		}

		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	// Test authentication
	if es.config.Email.SMTPUsername != "" {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	return nil
}

// SendTestEmail sends a test email to verify configuration
func (es *EmailService) SendTestEmail(ctx context.Context, toAddress string) error {
	testEmail := &EmailMessage{
		To:       []string{toAddress},
		Subject:  fmt.Sprintf("Test Email from %s", es.config.Application.BrandName),
		TextBody: fmt.Sprintf("This is a test email to verify your email configuration.\n\nSent at: %s", time.Now().Format(time.RFC3339)),
		From:     es.config.Email.FromAddress,
		Headers: map[string]string{
			"X-Test-Email": "true",
			"X-Mailer":     "Caslink URL Shortener",
		},
	}

	testEmail.HTMLBody = es.generateTestHTMLBody()

	return es.sendEmailMessage(ctx, testEmail)
}

// generateTestHTMLBody generates HTML for test email
func (es *EmailService) generateTestHTMLBody() string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Test Email</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .test-box {
            border: 2px solid #4CAF50;
            padding: 20px;
            border-radius: 8px;
            background-color: #f9f9f9;
        }
        .success { color: #4CAF50; font-weight: bold; }
    </style>
</head>
<body>
    <div class="test-box">
        <h2 class="success">✓ Email Configuration Test Successful</h2>
        <p>This is a test email from <strong>%s</strong>.</p>
        <p>If you received this email, your email configuration is working correctly.</p>
        <p><strong>Sent at:</strong> %s</p>
    </div>
</body>
</html>`,
		es.config.Application.BrandName,
		time.Now().Format("January 2, 2006 at 3:04 PM MST"),
	)
}

// GetEmailStats returns email delivery statistics
func (es *EmailService) GetEmailStats(ctx context.Context, days int) (*EmailStats, error) {
	since := time.Now().AddDate(0, 0, -days)

	stats := &EmailStats{}

	// Get email delivery counts
	err := es.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notifications WHERE channel = 'email' AND created_at >= ? AND status = 'delivered'",
		since).Scan(&stats.TotalDelivered)
	if err != nil {
		es.logger.WithError(err).Warn("Failed to get delivered email count")
	}

	err = es.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notifications WHERE channel = 'email' AND created_at >= ? AND status = 'failed'",
		since).Scan(&stats.TotalFailed)
	if err != nil {
		es.logger.WithError(err).Warn("Failed to get failed email count")
	}

	// Calculate delivery rate
	total := stats.TotalDelivered + stats.TotalFailed
	if total > 0 {
		stats.DeliveryRate = float64(stats.TotalDelivered) / float64(total) * 100
	}

	return stats, nil
}

// EmailStats represents email delivery statistics
type EmailStats struct {
	TotalDelivered int64   `json:"total_delivered"`
	TotalFailed    int64   `json:"total_failed"`
	DeliveryRate   float64 `json:"delivery_rate"`
}