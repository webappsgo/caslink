package validate

import (
	"strings"
	"testing"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		// Valid
		{name: "simple", email: "alice@example.com", wantErr: false},
		{name: "with dot in local part", email: "alice.bob@example.com", wantErr: false},
		{name: "with plus tag", email: "alice+tag@example.com", wantErr: false},
		{name: "with subdomain", email: "alice@mail.example.com", wantErr: false},
		{name: "with numbers", email: "alice123@example123.com", wantErr: false},
		{name: "uppercase normalized", email: "Alice@Example.COM", wantErr: false},
		{name: "leading/trailing whitespace trimmed", email: "  alice@example.com  ", wantErr: false},
		{name: "long TLD", email: "alice@example.technology", wantErr: false},

		// Empty / whitespace-only
		{name: "empty", email: "", wantErr: true},
		{name: "whitespace only", email: "   ", wantErr: true},

		// Malformed
		{name: "missing at sign", email: "aliceexample.com", wantErr: true},
		{name: "missing domain", email: "alice@", wantErr: true},
		{name: "missing local part", email: "@example.com", wantErr: true},
		{name: "missing tld", email: "alice@example", wantErr: true},
		{name: "single char tld", email: "alice@example.c", wantErr: true},
		{name: "double at sign", email: "alice@@example.com", wantErr: true},
		{name: "space in address", email: "alice bob@example.com", wantErr: true},
		{name: "trailing dot on domain", email: "alice@example.com.", wantErr: true},

		// Injection / abuse attempts
		{name: "header injection newline", email: "alice@example.com\nBcc: evil@evil.com", wantErr: true},
		{name: "header injection crlf", email: "alice@example.com\r\nBcc: evil@evil.com", wantErr: true},
		{name: "script tag", email: "<script>@example.com", wantErr: true},
		{name: "sql injection attempt", email: "alice'; DROP TABLE users;--@example.com", wantErr: true},

		// Unicode
		{name: "unicode local part rejected", email: "álice@example.com", wantErr: true},
		{name: "unicode domain rejected", email: "alice@exämple.com", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEmail(tc.email)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateEmail(%q) = nil, want error", tc.email)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateEmail(%q) = %v, want nil", tc.email, err)
			}
		})
	}
}

// TestValidateEmailNormalizesCase confirms an uppercase/whitespace-padded
// address still validates once normalized internally.
func TestValidateEmailNormalizesCase(t *testing.T) {
	if err := ValidateEmail("ALICE@EXAMPLE.COM"); err != nil {
		t.Errorf("expected uppercase email to be valid after normalization, got: %v", err)
	}
}

// TestValidateEmailRejectsHeaderInjectionSubstring guards against a
// regression where a valid-looking prefix before injected header content
// could slip past a naive regex.
func TestValidateEmailRejectsHeaderInjectionSubstring(t *testing.T) {
	err := ValidateEmail("alice@example.com\nBcc: evil@evil.com")
	if err == nil {
		t.Fatal("expected header-injection email to be rejected")
	}
	if !strings.Contains(err.Error(), "valid email") {
		t.Errorf("unexpected error message: %v", err)
	}
}
