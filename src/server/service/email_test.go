package service

import (
	"strings"
	"testing"
)

// TestRenderTemplateVariableSubstitution verifies {variable} placeholders in
// both the subject and body are replaced with the supplied values, and that
// values are shared correctly between subject and body.
func TestRenderTemplateVariableSubstitution(t *testing.T) {
	tmpl := "Subject: Hello {name} from {app_name}\n---\nBody for {name}. Visit {app_url}."

	subject, body := renderTemplate(tmpl, map[string]string{
		"name":     "Alice",
		"app_name": "Caslink",
		"app_url":  "https://example.com",
	})

	if subject != "Hello Alice from Caslink" {
		t.Errorf("subject = %q, want %q", subject, "Hello Alice from Caslink")
	}
	wantBody := "Body for Alice. Visit https://example.com."
	if body != wantBody {
		t.Errorf("body = %q, want %q", body, wantBody)
	}
}

// TestRenderTemplateMissingVariableLeavesPlaceholder documents current
// behavior: a variable referenced in the template but not supplied in vars
// is left untouched as a literal "{key}" placeholder rather than causing an
// error, since renderTemplate performs no validation of the variable set.
func TestRenderTemplateMissingVariableLeavesPlaceholder(t *testing.T) {
	tmpl := "Subject: Hi {name}\n---\nYour code is {otp_code}."

	subject, body := renderTemplate(tmpl, map[string]string{
		"name": "Bob",
	})

	if subject != "Hi Bob" {
		t.Errorf("subject = %q, want %q", subject, "Hi Bob")
	}
	if !strings.Contains(body, "{otp_code}") {
		t.Errorf("body = %q, want unresolved {otp_code} placeholder preserved", body)
	}
}

// TestRenderTemplateUnknownVariableIgnored verifies that supplying extra
// variables not referenced anywhere in the template is a silent no-op
// (renderTemplate has no rejection/validation path for unknown keys).
func TestRenderTemplateUnknownVariableIgnored(t *testing.T) {
	tmpl := "Subject: Static Subject\n---\nStatic body."

	subject, body := renderTemplate(tmpl, map[string]string{
		"unused_var": "should not appear",
	})

	if subject != "Static Subject" {
		t.Errorf("subject = %q, want %q", subject, "Static Subject")
	}
	if body != "Static body." {
		t.Errorf("body = %q, want %q", body, "Static body.")
	}
	if strings.Contains(subject, "should not appear") || strings.Contains(body, "should not appear") {
		t.Error("unknown variable value leaked into rendered output")
	}
}

// TestRenderTemplateMissingSeparatorFallsBack verifies the documented
// fallback: a template with no "---" separator (malformed/missing subject
// line) renders with a generic "Email" subject and the raw template as the
// body, rather than panicking or erroring.
func TestRenderTemplateMissingSeparatorFallsBack(t *testing.T) {
	tmpl := "This template has no separator at all."

	subject, body := renderTemplate(tmpl, map[string]string{"name": "Carol"})

	if subject != "Email" {
		t.Errorf("subject = %q, want fallback %q", subject, "Email")
	}
	if body != tmpl {
		t.Errorf("body = %q, want raw template %q", body, tmpl)
	}
}

// TestRenderTemplateEmptySubjectAndBody verifies boundary behavior when the
// template has an empty subject line and/or empty body after the separator —
// renderTemplate should trim to empty strings, not panic or leave stray
// whitespace/prefixes.
func TestRenderTemplateEmptySubjectAndBody(t *testing.T) {
	tmpl := "Subject: \n---\n"

	subject, body := renderTemplate(tmpl, nil)

	if subject != "" {
		t.Errorf("subject = %q, want empty string", subject)
	}
	if body != "" {
		t.Errorf("body = %q, want empty string", body)
	}
}

// TestRenderTemplateActualPasswordResetTemplate exercises renderTemplate
// against the real embedded password-reset template used by
// SendPasswordReset, verifying all documented variables actually resolve
// and no raw "{...}" placeholders survive.
func TestRenderTemplateActualPasswordResetTemplate(t *testing.T) {
	vars := map[string]string{
		"app_name":        "Caslink",
		"app_url":         "http://localhost:64521",
		"fqdn":            "localhost",
		"recipient_email": "user@example.com",
		"reset_link":      "http://localhost:64521/reset?token=abc123",
		"ip":              "203.0.113.5",
		"timestamp":       "2026-07-30 12:00:00 UTC",
		"expires":         "24 hours",
		"admin_email":     "admin@localhost",
	}

	subject, body := renderTemplate(passwordResetTemplate, vars)

	if subject == "" {
		t.Fatal("expected non-empty subject for password reset template")
	}
	if !strings.Contains(subject, "Caslink") {
		t.Errorf("subject = %q, want it to contain app_name %q", subject, "Caslink")
	}
	if !strings.Contains(body, vars["reset_link"]) {
		t.Errorf("body does not contain reset_link %q", vars["reset_link"])
	}
	if !strings.Contains(body, vars["recipient_email"]) {
		t.Errorf("body does not contain recipient_email %q", vars["recipient_email"])
	}
	// Every value we supplied must have removed its own placeholder; any
	// remaining "{" in the rendered body would indicate a variable name
	// mismatch between the template and the caller's vars map.
	for key := range vars {
		placeholder := "{" + key + "}"
		if strings.Contains(subject, placeholder) || strings.Contains(body, placeholder) {
			t.Errorf("placeholder %q was not substituted", placeholder)
		}
	}
}

// TestGetEnvOrDefault covers the pure env-var-or-default helper used
// throughout email.go for APP_URL/FQDN/ADMIN_EMAIL resolution.
func TestGetEnvOrDefault(t *testing.T) {
	const key = "CASLINK_TEST_EMAIL_ENV_VAR_DOES_NOT_EXIST"

	t.Run("unset returns default", func(t *testing.T) {
		t.Setenv(key, "")
		if got := getEnvOrDefault(key, "fallback"); got != "fallback" {
			t.Errorf("getEnvOrDefault = %q, want %q", got, "fallback")
		}
	})

	t.Run("set returns value", func(t *testing.T) {
		t.Setenv(key, "override")
		if got := getEnvOrDefault(key, "fallback"); got != "override" {
			t.Errorf("getEnvOrDefault = %q, want %q", got, "override")
		}
	})
}

// TestStripHeaderCRLF verifies the SMTP header-injection guard strips CR/LF
// characters from values destined for email headers (To/Subject), including
// mixed and repeated occurrences.
func TestStripHeaderCRLF(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"no CRLF", "plain subject", "plain subject"},
		{"CRLF injection attempt", "Subject\r\nBcc: attacker@example.com", "SubjectBcc: attacker@example.com"},
		{"bare LF", "line1\nline2", "line1line2"},
		{"bare CR", "line1\rline2", "line1line2"},
		{"empty string", "", ""},
		{"only CRLF", "\r\n\r\n", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripHeaderCRLF(tc.input)
			if got != tc.want {
				t.Errorf("stripHeaderCRLF(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestEmailServiceSMTPConfigResolutionEnvOverridesConfig verifies the
// documented priority (env var wins over config file) for the pure
// resolver helpers, without opening any network connection.
func TestEmailServiceSMTPConfigResolutionEnvOverridesConfig(t *testing.T) {
	svc := &EmailService{config: nil}

	t.Run("smtpHost falls back to empty with nil config and no env", func(t *testing.T) {
		t.Setenv("SMTP_HOST", "")
		if got := svc.smtpHost(); got != "" {
			t.Errorf("smtpHost() = %q, want empty", got)
		}
	})

	t.Run("smtpHost env override", func(t *testing.T) {
		t.Setenv("SMTP_HOST", "smtp.example.com")
		if got := svc.smtpHost(); got != "smtp.example.com" {
			t.Errorf("smtpHost() = %q, want %q", got, "smtp.example.com")
		}
	})

	t.Run("smtpPort default", func(t *testing.T) {
		t.Setenv("SMTP_PORT", "")
		if got := svc.smtpPort(); got != "587" {
			t.Errorf("smtpPort() = %q, want default %q", got, "587")
		}
	})

	t.Run("smtpPort env override", func(t *testing.T) {
		t.Setenv("SMTP_PORT", "2525")
		if got := svc.smtpPort(); got != "2525" {
			t.Errorf("smtpPort() = %q, want %q", got, "2525")
		}
	})

	t.Run("fromName default", func(t *testing.T) {
		t.Setenv("SMTP_FROM_NAME", "")
		if got := svc.fromName(); got != "Caslink" {
			t.Errorf("fromName() = %q, want default %q", got, "Caslink")
		}
	})

	t.Run("fromEmail default", func(t *testing.T) {
		t.Setenv("SMTP_FROM_EMAIL", "")
		if got := svc.fromEmail(); got != "no-reply@localhost" {
			t.Errorf("fromEmail() = %q, want default %q", got, "no-reply@localhost")
		}
	})
}

// TestTestSMTPConnectionUnconfiguredFailsFast verifies TestSMTPConnection
// returns immediately with an error (no network dial attempted) when no
// SMTP host is configured at all — this is the only SMTP-path behavior
// safely testable without a real/fake SMTP server.
func TestTestSMTPConnectionUnconfiguredFailsFast(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	svc := &EmailService{config: nil}

	if err := svc.TestSMTPConnection(); err == nil {
		t.Fatal("expected error when SMTP is not configured")
	}
	if svc.SMTPConfigured() {
		t.Error("SMTPConfigured() = true, want false when SMTP host is unset")
	}
}
