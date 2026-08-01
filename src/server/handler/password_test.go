package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/tmpl"
)

// newPasswordTestHandler builds a PasswordHandler backed by a real schema
// store, a real (deliberately unconfigured — no SMTP host set) EmailService,
// and a real template renderer, plus an already-registered user for
// password-reset flows.
func newPasswordTestHandler(t *testing.T) (*PasswordHandler, *service.AuthService, *service.User) {
	t.Helper()

	st := newSchemaTestStore(t)
	authService := service.NewAuthService(st)
	cfg := config.DefaultConfig()
	emailService := service.NewEmailService(cfg)
	renderer, err := tmpl.New()
	if err != nil {
		t.Fatalf("tmpl.New failed: %v", err)
	}

	user, err := authService.RegisterUser(context.Background(), "alice", "alice@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	return NewPasswordHandler(authService, emailService, renderer, cfg), authService, user
}

// TestForgotPasswordPageSMTPUnconfigured verifies the request page renders
// (200) with the "email not configured" notice when SMTP isn't set up —
// the default state in this hermetic test environment.
func TestForgotPasswordPageSMTPUnconfigured(t *testing.T) {
	h, _, _ := newPasswordTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/password/forgot", nil)
	w := httptest.NewRecorder()
	h.ForgotPasswordPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "administrator") {
		t.Errorf("expected SMTP-not-configured notice in body, got: %s", w.Body.String())
	}
}

// TestForgotPasswordJSONSMTPUnconfigured verifies the JSON API returns 503
// when SMTP isn't configured.
func TestForgotPasswordJSONSMTPUnconfigured(t *testing.T) {
	h, _, _ := newPasswordTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/auth/password/forgot", strings.NewReader(`{"email":"alice@example.com"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ForgotPassword(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

// TestForgotPasswordFormSMTPUnconfigured verifies the form (no-JS) path
// redirects back to the forgot-password page when SMTP isn't configured.
func TestForgotPasswordFormSMTPUnconfigured(t *testing.T) {
	h, _, _ := newPasswordTestHandler(t)

	form := url.Values{"email": {"alice@example.com"}}
	r := httptest.NewRequest(http.MethodPost, "/server/auth/password/forgot", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ForgotPassword(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", w.Code)
	}
}

// TestResetPasswordPageInvalidToken verifies the reset form still renders
// (200) but flags InvalidToken for a bogus/unknown token — never a hard
// error, since the page must still render the request-new-link fallback.
func TestResetPasswordPageInvalidToken(t *testing.T) {
	h, _, _ := newPasswordTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/password/reset/bogus-token", nil)
	r = withChiURLParam(r, "token", "bogus-token")
	w := httptest.NewRecorder()
	h.ResetPasswordPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestResetPasswordJSONMismatch verifies password/confirm mismatch is
// rejected with 400 over the JSON API.
func TestResetPasswordJSONMismatch(t *testing.T) {
	h, _, _ := newPasswordTestHandler(t)

	body := `{"password":"aaaaaaaa","password_confirm":"bbbbbbbb"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/auth/password/reset/tok", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "token", "tok")
	w := httptest.NewRecorder()
	h.ResetPassword(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestResetPasswordJSONTooShort verifies passwords under 8 characters are
// rejected with 400 over the JSON API.
func TestResetPasswordJSONTooShort(t *testing.T) {
	h, _, _ := newPasswordTestHandler(t)

	body := `{"password":"short","password_confirm":"short"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/auth/password/reset/tok", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "token", "tok")
	w := httptest.NewRecorder()
	h.ResetPassword(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestResetPasswordJSONInvalidToken verifies a well-formed but unknown token
// is rejected with 400 over the JSON API.
func TestResetPasswordJSONInvalidToken(t *testing.T) {
	h, _, _ := newPasswordTestHandler(t)

	body := `{"password":"longenoughpw","password_confirm":"longenoughpw"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/auth/password/reset/nope", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "token", "nope")
	w := httptest.NewRecorder()
	h.ResetPassword(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestResetPasswordJSONSuccess verifies a real reset token — created via the
// real AuthService.CreatePasswordResetToken, never mocked or backdoored —
// successfully resets the password over the JSON API.
func TestResetPasswordJSONSuccess(t *testing.T) {
	h, authService, _ := newPasswordTestHandler(t)

	token, err := authService.CreatePasswordResetToken(context.Background(), "alice@example.com", "user")
	if err != nil {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty reset token")
	}

	body := `{"password":"a-new-strong-password","password_confirm":"a-new-strong-password"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/auth/password/reset/"+token, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withChiURLParam(r, "token", token)
	w := httptest.NewRecorder()
	h.ResetPassword(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// respondJSON always wraps the payload as {"ok":true,"data":{...}}, so
	// unwrap the envelope before inspecting the success flag.
	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Data["success"] != true {
		t.Errorf("expected success=true, got %v", env.Data["success"])
	}

	// Reusing the same (now consumed) token must fail.
	r2 := httptest.NewRequest(http.MethodPost, "/api/v1/server/auth/password/reset/"+token, strings.NewReader(body))
	r2.Header.Set("Content-Type", "application/json")
	r2 = withChiURLParam(r2, "token", token)
	w2 := httptest.NewRecorder()
	h.ResetPassword(w2, r2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected reused token to be rejected with 400, got %d", w2.Code)
	}
}

// TestResetPasswordFormSuccess verifies the form (no-JS) path redirects to
// the login page on success.
func TestResetPasswordFormSuccess(t *testing.T) {
	h, authService, _ := newPasswordTestHandler(t)

	token, err := authService.CreatePasswordResetToken(context.Background(), "alice@example.com", "user")
	if err != nil {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}

	form := url.Values{"password": {"a-new-strong-password"}, "confirm_password": {"a-new-strong-password"}}
	r := httptest.NewRequest(http.MethodPost, "/server/auth/password/reset/"+token, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "token", token)
	w := httptest.NewRecorder()
	h.ResetPassword(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "reset=1") {
		t.Errorf("expected redirect location to contain reset=1, got %q", loc)
	}
}
