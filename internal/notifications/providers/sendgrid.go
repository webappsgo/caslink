package providers

import (
	"context"
	"fmt"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
)

// SendGridProvider implements email sending via SendGrid
type SendGridProvider struct {
	config *config.EmailConfig
	logger *logrus.Logger
	dialer *gomail.Dialer
}

// NewSendGridProvider creates a new SendGrid email provider
func NewSendGridProvider(cfg *config.EmailConfig, logger *logrus.Logger) (*SendGridProvider, error) {
	// SendGrid uses SMTP with API key authentication
	dialer := gomail.NewDialer(
		"smtp.sendgrid.net",
		587,
		"apikey", // SendGrid username is always "apikey"
		cfg.SendGridAPIKey,
	)

	return &SendGridProvider{
		config: cfg,
		logger: logger,
		dialer: dialer,
	}, nil
}

// SendEmail sends an email via SendGrid
func (p *SendGridProvider) SendEmail(ctx context.Context, email *EmailMessage) error {
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
	if err := p.dialer.DialAndSend(message); err != nil {
		return fmt.Errorf("failed to send email via SendGrid: %w", err)
	}

	p.logger.WithFields(logrus.Fields{
		"to":       email.To,
		"subject":  email.Subject,
		"provider": "sendgrid",
	}).Info("Email sent successfully via SendGrid")

	return nil
}

// ValidateConfig validates the SendGrid configuration
func (p *SendGridProvider) ValidateConfig() error {
	if p.config.SendGridAPIKey == "" {
		return fmt.Errorf("SendGrid API key is required")
	}

	if p.config.FromAddress == "" {
		return fmt.Errorf("from address is required")
	}

	// Basic API key format validation
	if len(p.config.SendGridAPIKey) < 20 {
		return fmt.Errorf("SendGrid API key appears to be invalid (too short)")
	}

	return nil
}