package providers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
	"gopkg.in/gomail.v2"
)

// SMTPProvider implements email sending via SMTP
type SMTPProvider struct {
	config *config.EmailConfig
	logger *logrus.Logger
	dialer *gomail.Dialer
}

// EmailMessage represents an email message for providers
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

// NewSMTPProvider creates a new SMTP email provider
func NewSMTPProvider(cfg *config.EmailConfig, logger *logrus.Logger) (*SMTPProvider, error) {
	dialer := gomail.NewDialer(
		cfg.SMTPHost,
		cfg.SMTPPort,
		cfg.SMTPUsername,
		cfg.SMTPPassword,
	)

	if cfg.SMTPTLS {
		dialer.TLSConfig = &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         cfg.SMTPHost,
		}
	}

	return &SMTPProvider{
		config: cfg,
		logger: logger,
		dialer: dialer,
	}, nil
}

// SendEmail sends an email via SMTP
func (p *SMTPProvider) SendEmail(ctx context.Context, email *EmailMessage) error {
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
		return fmt.Errorf("failed to send email via SMTP: %w", err)
	}

	p.logger.WithFields(logrus.Fields{
		"to":      strings.Join(email.To, ", "),
		"subject": email.Subject,
	}).Info("Email sent successfully via SMTP")

	return nil
}

// ValidateConfig validates the SMTP configuration
func (p *SMTPProvider) ValidateConfig() error {
	if p.config.SMTPHost == "" {
		return fmt.Errorf("SMTP host is required")
	}

	if p.config.SMTPPort == 0 {
		return fmt.Errorf("SMTP port is required")
	}

	if p.config.FromAddress == "" {
		return fmt.Errorf("from address is required")
	}

	return p.testConnection()
}

// testConnection tests the SMTP connection
func (p *SMTPProvider) testConnection() error {
	// Create a test connection
	auth := smtp.PlainAuth("",
		p.config.SMTPUsername,
		p.config.SMTPPassword,
		p.config.SMTPHost,
	)

	addr := fmt.Sprintf("%s:%d", p.config.SMTPHost, p.config.SMTPPort)

	// Test connection
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer client.Close()

	// Start TLS if required
	if p.config.SMTPTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         p.config.SMTPHost,
		}

		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	// Test authentication
	if p.config.SMTPUsername != "" {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	return nil
}