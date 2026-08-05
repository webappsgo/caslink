package service

import (
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/caslink/src/config"
	emailtmpl "github.com/webappsgo/caslink/src/template"
)

// Email templates are sourced from src/template/email/*.txt.
// The embed directive lives in the template package (not here) because
// embed paths cannot traverse parent directories.
var (
	passwordResetTemplate   = emailtmpl.PasswordResetEmail
	passwordChangedTemplate = emailtmpl.PasswordChangedEmail
	welcomeUserTemplate     = emailtmpl.WelcomeUserEmail
	welcomeAdminTemplate    = emailtmpl.WelcomeAdminEmail
	emailVerifyTemplate     = emailtmpl.EmailVerifyEmail
	testEmailTemplate       = emailtmpl.TestEmail
)

// stripHeaderCRLF removes carriage-return and line-feed characters from a
// value destined for an email header, defeating SMTP header-injection.
func stripHeaderCRLF(v string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(v)
}

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

// smtpHost resolves the SMTP host with env-var override taking priority
// over the config file. Returns "" when neither is set.
func (s *EmailService) smtpHost() string {
	if v := os.Getenv("SMTP_HOST"); v != "" {
		return v
	}
	if s.config != nil {
		return s.config.Server.Notifications.Email.SMTP.Host
	}
	return ""
}

// smtpPort resolves the SMTP port. Returns "587" when unset.
func (s *EmailService) smtpPort() string {
	if v := os.Getenv("SMTP_PORT"); v != "" {
		return v
	}
	if s.config != nil && s.config.Server.Notifications.Email.SMTP.Port > 0 {
		return strconv.Itoa(s.config.Server.Notifications.Email.SMTP.Port)
	}
	return "587"
}

// smtpUsername resolves the SMTP username.
func (s *EmailService) smtpUsername() string {
	if v := os.Getenv("SMTP_USERNAME"); v != "" {
		return v
	}
	if s.config != nil {
		return s.config.Server.Notifications.Email.SMTP.Username
	}
	return ""
}

// smtpPassword resolves the SMTP password.
func (s *EmailService) smtpPassword() string {
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		return v
	}
	if s.config != nil {
		return s.config.Server.Notifications.Email.SMTP.Password
	}
	return ""
}

// fromName resolves the From display name.
func (s *EmailService) fromName() string {
	if v := os.Getenv("SMTP_FROM_NAME"); v != "" {
		return v
	}
	if s.config != nil && s.config.Server.Notifications.Email.FromName != "" {
		return s.config.Server.Notifications.Email.FromName
	}
	return "Caslink"
}

// fromEmail resolves the From address.
func (s *EmailService) fromEmail() string {
	if v := os.Getenv("SMTP_FROM_EMAIL"); v != "" {
		return v
	}
	if s.config != nil && s.config.Server.Notifications.Email.From != "" {
		return s.config.Server.Notifications.Email.From
	}
	return "no-reply@localhost"
}

// fqdn resolves the public host name for links in emails. Emails are sent
// outside any HTTP request, so proxy-header detection is unavailable; the
// order is FQDN env > DOMAIN env (first of a comma list) > config
// server.fqdn > "localhost" (per AI.md PART 12 FQDN resolution, minus the
// request-scoped proxy step). Never a hardcoded runtime host.
func (s *EmailService) fqdn() string {
	if v := os.Getenv("FQDN"); v != "" {
		return v
	}
	if v := os.Getenv("DOMAIN"); v != "" {
		if i := strings.IndexByte(v, ','); i >= 0 {
			v = v[:i]
		}
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	if s.config != nil && s.config.Server.FQDN != "" {
		return s.config.Server.FQDN
	}
	return "localhost"
}

// baseURL resolves the {proto}://{fqdn}[:port] base for links in emails.
// An explicit APP_URL env var still wins (operator override); otherwise it
// is built from config: https when SSL is enabled else http, the resolved
// fqdn, and the configured port with :80/:443 stripped per AI.md PART 15.
func (s *EmailService) baseURL() string {
	if v := os.Getenv("APP_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	proto := "http"
	port := 0
	if s.config != nil {
		if s.config.Server.SSL.Enabled {
			proto = "https"
		}
		port = s.config.Server.Port
	}
	host := s.fqdn()
	if port > 0 && !(proto == "http" && port == 80) && !(proto == "https" && port == 443) {
		return fmt.Sprintf("%s://%s:%d", proto, host, port)
	}
	return fmt.Sprintf("%s://%s", proto, host)
}

// SMTPConfigured checks if SMTP is configured and working
// Per PART 26: No SMTP = No emails
func (s *EmailService) SMTPConfigured() bool {
	return s.TestSMTPConnection() == nil
}

// TestSMTPConnection performs a full SMTP-protocol-level pre-flight: it opens
// a TCP connection, speaks EHLO, and — if credentials are configured —
// authenticates. A server that accepts the TCP dial but rejects the SMTP
// session (bad auth, TLS required, EHLO refused) is caught here instead of
// only surfacing at actual send time, so misconfiguration fails loudly at
// config/test time rather than silently dropping mail later.
func (s *EmailService) TestSMTPConnection() error {
	host := s.smtpHost()
	if host == "" {
		return fmt.Errorf("SMTP not configured")
	}

	port := s.smtpPort()
	address := net.JoinHostPort(host, port)

	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to reach SMTP server %s: %w", address, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("SMTP handshake with %s failed: %w", address, err)
	}
	defer client.Close()

	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("SMTP EHLO to %s failed: %w", address, err)
	}

	username := s.smtpUsername()
	if username != "" {
		auth := smtp.PlainAuth("", username, s.smtpPassword(), host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication to %s failed: %w", address, err)
		}
	}

	return client.Quit()
}

// AutoDetectSMTP attempts to auto-detect SMTP server per PART 26 line 19271-19285
func (s *EmailService) AutoDetectSMTP() (string, int, error) {
	// Auto-detection order per spec
	hosts := []string{"localhost", "127.0.0.1", "172.17.0.1"}
	ports := []int{25, 587, 465}

	for _, host := range hosts {
		for _, port := range ports {
			address := net.JoinHostPort(host, strconv.Itoa(port))
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
		"app_url":         s.baseURL(),
		"fqdn":            s.fqdn(),
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
		"app_name":           "Caslink",
		"app_url":            s.baseURL(),
		"fqdn":               s.fqdn(),
		"recipient_email":    email,
		"recipient_username": username,
		"ip":                 ip,
		"method":             method,
		"timestamp":          time.Now().Format("2006-01-02 15:04:05 MST"),
		"admin_email":        getEnvOrDefault("ADMIN_EMAIL", "admin@localhost"),
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
		"app_url":         s.baseURL(),
		"fqdn":            s.fqdn(),
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
		"app_url":            s.baseURL(),
		"fqdn":               s.fqdn(),
		"recipient_email":    email,
		"recipient_username": username,
		"login_url":          s.baseURL() + "/server/auth/login",
		"profile_url":        s.baseURL() + "/users/profile",
		"admin_email":        getEnvOrDefault("ADMIN_EMAIL", "admin@localhost"),
	}

	if isAdmin {
		template = welcomeAdminTemplate
		vars["admin_url"] = s.baseURL() + "/server/admin"
		vars["admin_username"] = username
	}

	subject, body := renderTemplate(template, vars)

	return s.sendEmail(email, subject, body)
}

// SendTestEmail sends a "[TEST]" prefixed test message to verify the current
// SMTP configuration actually delivers mail, not just that the TCP/protocol
// pre-flight in TestSMTPConnection succeeds. Callers (the admin "Test
// Connection" action) are expected to also write an audit log entry per
// AI.md PART 18 ("prefix test emails with [TEST] and log them").
func (s *EmailService) SendTestEmail(to, appVersion string) error {
	if err := s.TestSMTPConnection(); err != nil {
		return err
	}

	vars := map[string]string{
		"app_name":    "Caslink",
		"app_url":     s.baseURL(),
		"fqdn":        s.fqdn(),
		"timestamp":   time.Now().Format("2006-01-02 15:04:05 MST"),
		"app_version": appVersion,
		"smtp_host":   s.smtpHost(),
		"smtp_port":   s.smtpPort(),
	}

	subject, body := renderTemplate(testEmailTemplate, vars)
	subject = "[TEST] " + subject

	return s.sendEmail(to, subject, body)
}

// sendEmail sends an email via SMTP
func (s *EmailService) sendEmail(to, subject, body string) error {
	// Resolve SMTP configuration: env vars take precedence over the
	// config file (per PART 26 line 19316).
	host := s.smtpHost()
	if host == "" {
		host = "localhost"
	}
	port := s.smtpPort()
	username := s.smtpUsername()
	password := s.smtpPassword()
	fromName := s.fromName()
	fromEmail := s.fromEmail()

	// Build email message per RFC 5322. Strip CR/LF from header values to
	// prevent SMTP header injection if an unvalidated recipient or subject
	// ever reaches this path.
	from := fmt.Sprintf("%s <%s>", fromName, fromEmail)
	safeTo := stripHeaderCRLF(to)
	safeSubject := stripHeaderCRLF(subject)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", from, safeTo, safeSubject, body)

	// Connect to SMTP server (IPv6-safe address join).
	address := net.JoinHostPort(host, port)

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
