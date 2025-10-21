package providers

import (
	"context"
	"fmt"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
)

// SESProvider implements email sending via AWS SES
type SESProvider struct {
	config *config.EmailConfig
	logger *logrus.Logger
	dialer *gomail.Dialer
}

// NewSESProvider creates a new AWS SES email provider
func NewSESProvider(cfg *config.EmailConfig, logger *logrus.Logger) (*SESProvider, error) {
	region := cfg.SESRegion
	if region == "" {
		region = "us-east-1" // Default region
	}

	// AWS SES SMTP endpoint
	smtpHost := fmt.Sprintf("email-smtp.%s.amazonaws.com", region)

	dialer := gomail.NewDialer(
		smtpHost,
		587,
		cfg.SESAccessKey,
		cfg.SESSecretKey,
	)

	return &SESProvider{
		config: cfg,
		logger: logger,
		dialer: dialer,
	}, nil
}

// SendEmail sends an email via AWS SES
func (p *SESProvider) SendEmail(ctx context.Context, email *EmailMessage) error {
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
		return fmt.Errorf("failed to send email via AWS SES: %w", err)
	}

	p.logger.WithFields(logrus.Fields{
		"to":       email.To,
		"subject":  email.Subject,
		"provider": "ses",
		"region":   p.config.SESRegion,
	}).Info("Email sent successfully via AWS SES")

	return nil
}

// ValidateConfig validates the AWS SES configuration
func (p *SESProvider) ValidateConfig() error {
	if p.config.SESAccessKey == "" {
		return fmt.Errorf("SES access key is required")
	}

	if p.config.SESSecretKey == "" {
		return fmt.Errorf("SES secret key is required")
	}

	if p.config.FromAddress == "" {
		return fmt.Errorf("from address is required")
	}

	// Basic access key format validation
	if len(p.config.SESAccessKey) < 16 {
		return fmt.Errorf("SES access key appears to be invalid (too short)")
	}

	if len(p.config.SESSecretKey) < 32 {
		return fmt.Errorf("SES secret key appears to be invalid (too short)")
	}

	return nil
}