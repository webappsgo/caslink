package service

import (
	_ "embed"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/config"
)

//go:embed ../../templates/email/password_reset.txt
var passwordResetTemplate string

//go:embed ../../templates/email/password_changed.txt
var passwordChangedTemplate string

//go:embed ../../templates/email/welcome_user.txt
var welcomeUserTemplate string

//go:embed ../../templates/email/welcome_admin.txt
var welcomeAdminTemplate string

//go:embed ../../templates/email/email_verify.txt
var emailVerifyTemplate string

// EmailService handles email sending
type EmailService struct {
	config *config.Config
}

// NewEmailService creates a new email service
func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{
		config: cfg,
	}
}

// SMTPConfigured checks if SMTP is configured and working
// Per PART 26: No SMTP = No emails
func (s *EmailService) SMTPConfigured() bool {
	// Check environment variables first (highest priority per PART 26 line 19316)
	host := getEnvOrDefault("SMTP_HOST", "")
	if host == "" {
		// No SMTP configured
		return false
	}
	
	port := getEnvOrDefault("SMTP_PORT", "587")
	
	// Quick connection test
	address := fmt.Sprintf("%s:%s", host, port)
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// AutoDetectSMTP attempts to auto-detect SMTP server per PART 26 line 19271-19285
func (s *EmailService) AutoDetectSMTP() (string, int, error) {
	// Auto-detection order per spec
	hosts := []string{"localhost", "127.0.0.1", "172.17.0.1"}
	ports := []int{25, 587, 465}
	
	for _, host := range hosts {
		for _, port := range ports {
			address := fmt.Sprintf("%s:%d", host, port)
			conn, err := net.DialTimeout("tcp", address, 2*time.Second)
			if err == nil {
				conn.Close()
				return host, port, nil
			}
		}
	}
	
	return "", 0, fmt.Errorf("no SMTP server found")
}

// SendPasswordReset sends a password reset email
// Per PART 26: Only sends if SMTP is configured
func (s *EmailService) SendPasswordReset(email, resetLink, ip string) error {
	if !s.SMTPConfigured() {
		// Per PART 26 line 22674: Do NOT attempt to send without SMTP
		return fmt.Errorf("SMTP not configured")
	}

	// Load and render template per PART 26 line 22910-22943
	vars := map[string]string{
		"app_name":        "Caslink",
		"app_url":         getEnvOrDefault("APP_URL", "http://localhost:64521"),
		"fqdn":            getEnvOrDefault("FQDN", "localhost"),
		"recipient_email": email,
		"reset_link":      resetLink,
		"ip":              ip,
		"timestamp":       time.Now().Format("2006-01-02 15:04:05 MST"),
		"expires":         "24 hours",
		"admin_email":     getEnvOrDefault("ADMIN_EMAIL", "admin@localhost"),
	}
	
	subject, body := renderTemplate(passwordResetTemplate, vars)
	
	return s.sendEmail(email, subject, body)
}

// SendPasswordChanged sends notification that password was changed
func (s *EmailService) SendPasswordChanged(email, username, ip, method string) error {
	if !s.SMTPConfigured() {
		// Silently skip per PART 26 line 22669
		return nil
	}
	
	vars := map[string]string{
		"app_name":         "Caslink",
		"app_url":          getEnvOrDefault("APP_URL", "http://localhost:64521"),
		"fqdn":             getEnvOrDefault("FQDN", "localhost"),
		"recipient_email":  email,
		"recipient_username": username,
		"ip":               ip,
		"method":           method,
		"timestamp":        time.Now().Format("2006-01-02 15:04:05 MST"),
		"admin_email":      getEnvOrDefault("ADMIN_EMAIL", "admin@localhost"),
	}
	
	subject, body := renderTemplate(passwordChangedTemplate, vars)
	
	return s.sendEmail(email, subject, body)
}

// SendEmailVerification sends an email verification link
// Per PART 26: Only sends if SMTP is configured
func (s *EmailService) SendEmailVerification(email, verifyLink string) error {
	if !s.SMTPConfigured() {
		// Per PART 26 line 22674: Do NOT attempt to send without SMTP
		return fmt.Errorf("SMTP not configured")
	}

	vars := map[string]string{
		"app_name":        "Caslink",
		"app_url":         getEnvOrDefault("APP_URL", "http://localhost:64521"),
		"fqdn":            getEnvOrDefault("FQDN", "localhost"),
		"recipient_email": email,
		"verify_link":     verifyLink,
		"timestamp":       time.Now().Format("2006-01-02 15:04:05 MST"),
		"expires":         "48 hours",
	}
	
	subject, body := renderTemplate(emailVerifyTemplate, vars)
	
	return s.sendEmail(email, subject, body)
}

// SendWelcome sends a welcome email to new users
// Per PART 26: Only sends if SMTP is configured
func (s *EmailService) SendWelcome(email, username string, isAdmin bool) error {
	if !s.SMTPConfigured() {
		// Per PART 26: silently skip if no SMTP (line 22669)
		return nil
	}

	template := welcomeUserTemplate
	vars := map[string]string{
		"app_name":           "Caslink",
		"app_url":            getEnvOrDefault("APP_URL", "http://localhost:64521"),
		"fqdn":               getEnvOrDefault("FQDN", "localhost"),
		"recipient_email":    email,
		"recipient_username": username,
		"login_url":          getEnvOrDefault("APP_URL", "http://localhost:64521") + "/auth/login",
		"profile_url":        getEnvOrDefault("APP_URL", "http://localhost:64521") + "/user/profile",
		"admin_email":        getEnvOrDefault("ADMIN_EMAIL", "admin@localhost"),
	}
	
	if isAdmin {
		template = welcomeAdminTemplate
		vars["admin_url"] = getEnvOrDefault("APP_URL", "http://localhost:64521") + "/admin"
		vars["admin_username"] = username
	}
	
	subject, body := renderTemplate(template, vars)
	
	return s.sendEmail(email, subject, body)
}

// sendEmail sends an email via SMTP
func (s *EmailService) sendEmail(to, subject, body string) error {
	// Get SMTP configuration from environment
	host := getEnvOrDefault("SMTP_HOST", "localhost")
	port := getEnvOrDefault("SMTP_PORT", "587")
	username := getEnvOrDefault("SMTP_USERNAME", "")
	password := getEnvOrDefault("SMTP_PASSWORD", "")
	fromName := getEnvOrDefault("SMTP_FROM_NAME", "Caslink")
	fromEmail := getEnvOrDefault("SMTP_FROM_EMAIL", "no-reply@localhost")
	
	// Build email message per RFC 5322
	from := fmt.Sprintf("%s <%s>", fromName, fromEmail)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", from, to, subject, body)
	
	// Connect to SMTP server
	address := fmt.Sprintf("%s:%s", host, port)
	
	// Attempt connection with auth if credentials provided
	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}
	
	// Send email
	err := smtp.SendMail(address, auth, fromEmail, []string{to}, []byte(msg))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	
	return nil
}

// renderTemplate renders an email template with variables
// Per PART 26 line 22798-22802: Subject line, separator, body with {variable} syntax
func renderTemplate(template string, vars map[string]string) (subject, body string) {
	// Split template into subject and body
	parts := strings.SplitN(template, "---", 2)
	if len(parts) != 2 {
		return "Email", template
	}
	
	// Extract subject (remove "Subject: " prefix)
	subject = strings.TrimSpace(strings.TrimPrefix(parts[0], "Subject:"))
	body = strings.TrimSpace(parts[1])
	
	// Replace variables in both subject and body
	for key, value := range vars {
		placeholder := "{" + key + "}"
		subject = strings.ReplaceAll(subject, placeholder, value)
		body = strings.ReplaceAll(body, placeholder, value)
	}
	
	return subject, body
}

// getEnvOrDefault gets environment variable or returns default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
