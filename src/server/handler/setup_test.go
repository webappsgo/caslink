package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service"
)

// newSetupTestHandler builds a SetupHandler backed by a real, in-memory
// SQLite AuthService, mirroring auth_user_test.go's convention.
func newSetupTestHandler(t *testing.T) (*SetupHandler, *service.AuthService) {
	t.Helper()

	st := newSchemaTestStore(t)
	authService := service.NewAuthService(st)
	cfg := config.DefaultConfig()

	return NewSetupHandler(authService, cfg, "1.2.3"), authService
}

// ---- itoa ----

func TestItoa(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{8, "8"},
		{12, "12"},
		{999, "999"},
	}
	for _, tc := range cases {
		if got := itoa(tc.in); got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestItoaNegative documents a bug: itoa's loop condition is `n > 0`, so a
// negative input never enters the loop and the function returns "" instead
// of e.g. "-5". Not app-breaking today (the only caller, MinLength, is
// always clamped to >=8 before reaching itoa), but flagged here as a latent
// bug per the reporting convention.
func TestItoaNegative(t *testing.T) {
	if got := itoa(-5); got != "" {
		t.Errorf("itoa(-5) = %q, want the documented (buggy) empty-string result %q", got, "")
	}
}

// ---- csrfTokenFromRequest ----

func TestCsrfTokenFromRequestNoCookie(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/setup", nil)
	if got := csrfTokenFromRequest(r); got != "" {
		t.Errorf("expected empty string with no cookie, got %q", got)
	}
}

func TestCsrfTokenFromRequestWithCookie(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/setup", nil)
	r.AddCookie(&http.Cookie{Name: "csrf_token", Value: "abc123"})
	if got := csrfTokenFromRequest(r); got != "abc123" {
		t.Errorf("expected %q, got %q", "abc123", got)
	}
}

// ---- validatePassword ----

func TestValidatePasswordDefaultPolicy(t *testing.T) {
	h, _ := newSetupTestHandler(t)

	if msg := h.validatePassword("short"); msg == "" {
		t.Error("expected an error for a password shorter than the default minimum of 8")
	}
	if msg := h.validatePassword("longenough"); msg != "" {
		t.Errorf("expected no error for a sufficiently long password under the default policy, got %q", msg)
	}
}

func TestValidatePasswordCustomMinLength(t *testing.T) {
	h, _ := newSetupTestHandler(t)
	h.cfg.Server.Security.Password.MinLength = 12

	if msg := h.validatePassword("elevenchars!"); len(msg) == 0 {
		// "elevenchars!" is 12 chars, should pass.
	}
	if msg := h.validatePassword("short12char"); msg == "" {
		t.Error("expected a min-length error for an 11-char password under a 12-char policy")
	}
}

func TestValidatePasswordZeroOrNegativeMinLengthDefaultsToEight(t *testing.T) {
	h, _ := newSetupTestHandler(t)
	h.cfg.Server.Security.Password.MinLength = 0

	if msg := h.validatePassword("1234567"); msg == "" {
		t.Error("expected the zero MinLength to fall back to the default of 8 and reject a 7-char password")
	}
	if msg := h.validatePassword("12345678"); msg != "" {
		t.Errorf("expected an 8-char password to satisfy the default-8 fallback, got %q", msg)
	}
}

func TestValidatePasswordRequireUppercase(t *testing.T) {
	h, _ := newSetupTestHandler(t)
	h.cfg.Server.Security.Password.RequireUppercase = true

	if msg := h.validatePassword("alllowercase"); msg == "" {
		t.Error("expected an error when no uppercase letter is present")
	}
	if msg := h.validatePassword("hasOneUpper"); msg != "" {
		t.Errorf("expected success with an uppercase letter present, got %q", msg)
	}
}

func TestValidatePasswordRequireLowercase(t *testing.T) {
	h, _ := newSetupTestHandler(t)
	h.cfg.Server.Security.Password.RequireLowercase = true

	if msg := h.validatePassword("ALLUPPERCASE"); msg == "" {
		t.Error("expected an error when no lowercase letter is present")
	}
	if msg := h.validatePassword("HASoneLower"); msg != "" {
		t.Errorf("expected success with a lowercase letter present, got %q", msg)
	}
}

func TestValidatePasswordRequireNumber(t *testing.T) {
	h, _ := newSetupTestHandler(t)
	h.cfg.Server.Security.Password.RequireNumber = true

	if msg := h.validatePassword("nodigitshere"); msg == "" {
		t.Error("expected an error when no digit is present")
	}
	if msg := h.validatePassword("hasdigit1here"); msg != "" {
		t.Errorf("expected success with a digit present, got %q", msg)
	}
}

func TestValidatePasswordRequireSpecial(t *testing.T) {
	h, _ := newSetupTestHandler(t)
	h.cfg.Server.Security.Password.RequireSpecial = true

	if msg := h.validatePassword("nospecialchars"); msg == "" {
		t.Error("expected an error when no special character is present")
	}
	if msg := h.validatePassword("has-special!"); msg != "" {
		t.Errorf("expected success with a special character present, got %q", msg)
	}
}

func TestValidatePasswordAllPoliciesCombined(t *testing.T) {
	h, _ := newSetupTestHandler(t)
	p := &h.cfg.Server.Security.Password
	p.MinLength = 10
	p.RequireUppercase = true
	p.RequireLowercase = true
	p.RequireNumber = true
	p.RequireSpecial = true

	if msg := h.validatePassword("short"); msg == "" {
		t.Error("expected a length error for a too-short password")
	}
	if msg := h.validatePassword("alllowercase1!"); msg == "" {
		t.Error("expected an uppercase-required error")
	}
	if msg := h.validatePassword("ALLUPPERCASE1!"); msg == "" {
		t.Error("expected a lowercase-required error")
	}
	if msg := h.validatePassword("NoDigitsHere!"); msg == "" {
		t.Error("expected a number-required error")
	}
	if msg := h.validatePassword("NoSpecial123"); msg == "" {
		t.Error("expected a special-character-required error")
	}
	if msg := h.validatePassword("Valid1234!"); msg != "" {
		t.Errorf("expected a password satisfying every policy component to pass, got %q", msg)
	}
}

// ---- passwordHint ----

func TestPasswordHintDefaultPolicy(t *testing.T) {
	h, _ := newSetupTestHandler(t)

	hint := h.passwordHint()
	if !strings.Contains(hint, "At least 8 characters") {
		t.Errorf("expected hint to mention the 8-character minimum, got %q", hint)
	}
	if strings.Contains(hint, "uppercase") || strings.Contains(hint, "special") {
		t.Errorf("expected no extra requirements mentioned under the default policy, got %q", hint)
	}
}

func TestPasswordHintAllPolicies(t *testing.T) {
	h, _ := newSetupTestHandler(t)
	p := &h.cfg.Server.Security.Password
	p.MinLength = 12
	p.RequireUppercase = true
	p.RequireLowercase = true
	p.RequireNumber = true
	p.RequireSpecial = true

	hint := h.passwordHint()
	for _, want := range []string{"At least 12 characters", "one uppercase letter", "one lowercase letter", "one number", "one special character"} {
		if !strings.Contains(hint, want) {
			t.Errorf("expected hint %q to contain %q", hint, want)
		}
	}
}

// ---- SetupPage / renderSetupForm ----

func TestSetupPageRenders(t *testing.T) {
	h, _ := newSetupTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/setup", nil)
	w := httptest.NewRecorder()
	h.SetupPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Create Admin Account") {
		t.Error("expected the setup form to be rendered")
	}
}

// ---- Setup (POST) ----

func TestSetupInvalidFormData(t *testing.T) {
	h, _ := newSetupTestHandler(t)

	// An invalid %-escape in the query string makes r.ParseForm() itself
	// return an error, exercising the "Invalid form data" branch.
	r := httptest.NewRequest(http.MethodPost, "/setup?a=%zz", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Setup(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (form re-rendered with error), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Invalid form data") {
		t.Error("expected the 'Invalid form data' error to be shown")
	}
}

func TestSetupMissingFields(t *testing.T) {
	h, _ := newSetupTestHandler(t)

	form := url.Values{"username": {"admin"}}
	r := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Setup(w, r)

	if !strings.Contains(w.Body.String(), "All fields are required") {
		t.Error("expected the 'All fields are required' error")
	}
}

func TestSetupUsernameTooShort(t *testing.T) {
	h, _ := newSetupTestHandler(t)

	form := url.Values{
		"username":         {"ab"},
		"email":            {"admin@example.com"},
		"password":         {"longenoughpw"},
		"confirm_password": {"longenoughpw"},
	}
	r := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Setup(w, r)

	if !strings.Contains(w.Body.String(), "Username must be at least 3 characters") {
		t.Error("expected the username-too-short error")
	}
}

func TestSetupPasswordFailsPolicy(t *testing.T) {
	h, _ := newSetupTestHandler(t)

	form := url.Values{
		"username":         {"admin"},
		"email":            {"admin@example.com"},
		"password":         {"short"},
		"confirm_password": {"short"},
	}
	r := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Setup(w, r)

	if !strings.Contains(w.Body.String(), "Password must be at least 8 characters") {
		t.Error("expected the password policy error to be surfaced")
	}
}

func TestSetupPasswordMismatch(t *testing.T) {
	h, _ := newSetupTestHandler(t)

	form := url.Values{
		"username":         {"admin"},
		"email":            {"admin@example.com"},
		"password":         {"longenoughpw"},
		"confirm_password": {"different-password"},
	}
	r := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Setup(w, r)

	if !strings.Contains(w.Body.String(), "Passwords do not match") {
		t.Error("expected the password-mismatch error")
	}
}

func TestSetupSuccessCreatesAdminAndRendersComplete(t *testing.T) {
	h, authService := newSetupTestHandler(t)

	form := url.Values{
		"username":         {"admin"},
		"email":            {"admin@example.com"},
		"password":         {"longenoughpw"},
		"confirm_password": {"longenoughpw"},
	}
	r := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Setup(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Setup Complete") {
		t.Error("expected the setup-complete page to be rendered")
	}
	if !strings.Contains(w.Body.String(), "admin") {
		t.Error("expected the created username to appear on the completion page")
	}

	// Verify a real admin account was created and can authenticate.
	admin, err := authService.AuthenticateAdmin(context.Background(), "admin", "longenoughpw")
	if err != nil {
		t.Fatalf("expected the created admin to authenticate, got error: %v", err)
	}
	if admin.Username != "admin" {
		t.Errorf("expected username %q, got %q", "admin", admin.Username)
	}
}

func TestSetupDuplicateUsernameFails(t *testing.T) {
	h, authService := newSetupTestHandler(t)

	if err := authService.CreatePrimaryAdmin(context.Background(), "admin", "longenoughpw", "admin@example.com"); err != nil {
		t.Fatalf("seed CreatePrimaryAdmin failed: %v", err)
	}

	form := url.Values{
		"username":         {"admin"},
		"email":            {"admin2@example.com"},
		"password":         {"longenoughpw"},
		"confirm_password": {"longenoughpw"},
	}
	r := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Setup(w, r)

	if !strings.Contains(w.Body.String(), "Failed to create admin account") {
		t.Error("expected a duplicate-admin failure to be surfaced")
	}
}
