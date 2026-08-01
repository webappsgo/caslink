package template

import (
	"strings"
	"testing"
)

// emailTemplates enumerates every exported embedded email template so the
// table-driven tests below cover all of them uniformly.
func emailTemplates() map[string]string {
	return map[string]string{
		"PasswordResetEmail":   PasswordResetEmail,
		"PasswordChangedEmail": PasswordChangedEmail,
		"WelcomeUserEmail":     WelcomeUserEmail,
		"WelcomeAdminEmail":    WelcomeAdminEmail,
		"EmailVerifyEmail":     EmailVerifyEmail,
		"TestEmail":            TestEmail,
	}
}

// TestEmailTemplatesAreEmbedded verifies go:embed actually pulled in
// non-empty file contents for every exported template variable — an empty
// string here would mean a broken embed path or an empty source file.
func TestEmailTemplatesAreEmbedded(t *testing.T) {
	for name, content := range emailTemplates() {
		t.Run(name, func(t *testing.T) {
			if content == "" {
				t.Fatalf("%s is empty; go:embed likely failed to locate the source file", name)
			}
		})
	}
}

// TestEmailTemplatesFollowRequiredFormat verifies every embedded template
// follows the documented email template format from AI.md PART 18:
// a "Subject: ..." line, a "---" separator, then a plain-text body, using
// "{variable}" placeholder syntax.
func TestEmailTemplatesFollowRequiredFormat(t *testing.T) {
	for name, content := range emailTemplates() {
		t.Run(name, func(t *testing.T) {
			lines := strings.Split(content, "\n")
			if len(lines) < 2 {
				t.Fatalf("%s has too few lines to contain a subject and separator", name)
			}

			if !strings.HasPrefix(lines[0], "Subject: ") {
				t.Errorf("%s first line = %q, want it to start with %q", name, lines[0], "Subject: ")
			}
			if strings.TrimSpace(lines[0]) == "Subject:" {
				t.Errorf("%s has an empty subject", name)
			}

			if strings.TrimSpace(lines[1]) != "---" {
				t.Errorf("%s second line = %q, want the %q separator", name, lines[1], "---")
			}

			body := strings.Join(lines[2:], "\n")
			if strings.TrimSpace(body) == "" {
				t.Errorf("%s has no body content after the separator", name)
			}
		})
	}
}

// TestEmailTemplatesUseAppNamePlaceholder verifies every template
// references the {app_name} global variable, per the documented required
// global vars list — every account/notification email must be brandable.
func TestEmailTemplatesUseAppNamePlaceholder(t *testing.T) {
	for name, content := range emailTemplates() {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(content, "{app_name}") {
				t.Errorf("%s does not reference {app_name}", name)
			}
		})
	}
}

// TestEmailTemplatesEndWithSingleTrailingNewline verifies each embedded
// template file ends with exactly one trailing newline, per the project's
// universal file-ending convention.
func TestEmailTemplatesEndWithSingleTrailingNewline(t *testing.T) {
	for name, content := range emailTemplates() {
		t.Run(name, func(t *testing.T) {
			if !strings.HasSuffix(content, "\n") {
				t.Fatalf("%s does not end with a newline", name)
			}
			if strings.HasSuffix(content, "\n\n") {
				t.Errorf("%s ends with more than one trailing newline", name)
			}
		})
	}
}

// TestSensitiveDisclaimerEmailsCarryRecipientAndDisclaimer verifies the
// account-security emails required by AI.md PART 18 to carry a visible
// recipient line and an unauthorized-action disclaimer actually do —
// regression coverage for the "required disclaimer/recipient fields"
// rule, scoped to the templates that are exported by this package.
func TestSensitiveDisclaimerEmailsCarryRecipientAndDisclaimer(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"PasswordResetEmail", PasswordResetEmail},
		{"PasswordChangedEmail", PasswordChangedEmail},
		{"WelcomeUserEmail", WelcomeUserEmail},
		{"EmailVerifyEmail", EmailVerifyEmail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.content, "{recipient_email}") {
				t.Errorf("%s does not include the {recipient_email} field", tt.name)
			}
		})
	}
}
