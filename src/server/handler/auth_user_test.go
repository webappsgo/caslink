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
	"github.com/webappsgo/caslink/src/server/store"
	"github.com/webappsgo/caslink/src/server/tmpl"
)

// newAuthUserTestHandler builds an AuthUserHandler backed by a real schema
// store, a real AuthService, and a real template renderer. It also returns
// the underlying store so callers can wire up other real services (e.g.
// TOTPService) against the exact same data.
func newAuthUserTestHandler(t *testing.T) (*AuthUserHandler, *service.AuthService, *store.Store) {
	t.Helper()

	st := newSchemaTestStore(t)
	authService := service.NewAuthService(st)
	cfg := config.DefaultConfig()
	renderer, err := tmpl.New()
	if err != nil {
		t.Fatalf("tmpl.New failed: %v", err)
	}

	inviteService := service.NewInviteService(st)
	return NewAuthUserHandler(authService, inviteService, renderer, cfg), authService, st
}

// TestRegisterPageRenders verifies the registration page always renders.
func TestRegisterPageRenders(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/register", nil)
	w := httptest.NewRecorder()
	h.RegisterPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestRegisterJSONInvalidUsername verifies an invalid username is rejected
// with 400 over the JSON API, via the canonical error envelope.
func TestRegisterJSONInvalidUsername(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)

	body := `{"username":"a","email":"carol@example.com","password":"longenoughpw"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/register", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.OK {
		t.Error("expected ok=false in the canonical error envelope")
	}
	if env.Error != "BAD_REQUEST" {
		t.Errorf("expected error code BAD_REQUEST, got %q", env.Error)
	}
}

// TestRegisterJSONInvalidEmail verifies an invalid email is rejected with
// 400 over the JSON API.
func TestRegisterJSONInvalidEmail(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)

	body := `{"username":"carol","email":"not-an-email","password":"longenoughpw"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/register", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegisterJSONShortPassword verifies a too-short password is rejected
// with 400 over the JSON API.
func TestRegisterJSONShortPassword(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)

	body := `{"username":"carol","email":"carol@example.com","password":"short"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/register", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegisterJSONDuplicateUsername verifies registering an already-taken
// username/email fails with 400 against the real AuthService uniqueness
// check.
func TestRegisterJSONDuplicateUsername(t *testing.T) {
	h, authService, _ := newAuthUserTestHandler(t)

	if _, err := authService.RegisterUser(context.Background(), "carol", "carol@example.com", "correct-horse-battery-staple"); err != nil {
		t.Fatalf("seed RegisterUser failed: %v", err)
	}

	body := `{"username":"carol","email":"carol2@example.com","password":"longenoughpw"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/register", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate username, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegisterJSONSuccess verifies a well-formed registration succeeds,
// returns 201 with the canonical success envelope, and sets a user_session
// cookie usable via the real AuthService.
func TestRegisterJSONSuccess(t *testing.T) {
	h, authService, _ := newAuthUserTestHandler(t)

	body := `{"username":"dave","email":"dave@example.com","password":"longenoughpw"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/register", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Data["success"] != true {
		t.Errorf("expected success=true, got %v", env.Data["success"])
	}

	var sessionCookie string
	for _, c := range w.Result().Cookies() {
		if c.Name == "user_session" {
			sessionCookie = c.Value
		}
	}
	if sessionCookie == "" {
		t.Fatal("expected a user_session cookie to be set")
	}
	if _, err := authService.ValidateUserSession(context.Background(), sessionCookie); err != nil {
		t.Errorf("expected the issued session to validate, got error: %v", err)
	}
}

// TestRegisterFormSuccess verifies the form (no-JS) path redirects to the
// homepage on success.
func TestRegisterFormSuccess(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)

	form := url.Values{"username": {"erin"}, "email": {"erin@example.com"}, "password": {"longenoughpw"}}
	r := httptest.NewRequest(http.MethodPost, "/server/auth/register", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Errorf("expected redirect to /, got %q", loc)
	}
}

// TestRegisterFormValidationError verifies the form (no-JS) path re-renders
// the registration page with a 422 status on a validation failure, instead
// of redirecting.
func TestRegisterFormValidationError(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)

	form := url.Values{"username": {"a"}, "email": {"frank@example.com"}, "password": {"longenoughpw"}}
	r := httptest.NewRequest(http.MethodPost, "/server/auth/register", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

// TestLoginPageRenders verifies the login page always renders.
func TestLoginPageRenders(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/login", nil)
	w := httptest.NewRecorder()
	h.LoginPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestLoginJSONInvalidCredentials verifies a wrong password is rejected
// with 401 via the canonical error envelope.
func TestLoginJSONInvalidCredentials(t *testing.T) {
	h, authService, _ := newAuthUserTestHandler(t)

	if _, err := authService.RegisterUser(context.Background(), "gina", "gina@example.com", "correct-horse-battery-staple"); err != nil {
		t.Fatalf("seed RegisterUser failed: %v", err)
	}

	body := `{"identifier":"gina","password":"wrong-password"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/login", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.OK {
		t.Error("expected ok=false in the canonical error envelope")
	}
}

// TestLoginJSONSuccessNo2FA verifies a correct-password login without 2FA
// enabled returns 200 with a real, validating user_session cookie.
func TestLoginJSONSuccessNo2FA(t *testing.T) {
	h, authService, _ := newAuthUserTestHandler(t)

	if _, err := authService.RegisterUser(context.Background(), "hank", "hank@example.com", "correct-horse-battery-staple"); err != nil {
		t.Fatalf("seed RegisterUser failed: %v", err)
	}

	body := `{"identifier":"hank","password":"correct-horse-battery-staple"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/login", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sessionCookie string
	for _, c := range w.Result().Cookies() {
		if c.Name == "user_session" {
			sessionCookie = c.Value
		}
	}
	if sessionCookie == "" {
		t.Fatal("expected a user_session cookie to be set")
	}
	if _, err := authService.ValidateUserSession(context.Background(), sessionCookie); err != nil {
		t.Errorf("expected the issued session to validate, got error: %v", err)
	}
}

// TestLoginJSONRequires2FA verifies a correct-password login for a
// TOTP-enabled user does NOT issue a full user_session cookie, but instead
// a short-lived 2fa_pending cookie and a requires_2fa JSON response.
func TestLoginJSONRequires2FA(t *testing.T) {
	h, authService, st := newAuthUserTestHandler(t)

	user, err := authService.RegisterUser(context.Background(), "ivan", "ivan@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("seed RegisterUser failed: %v", err)
	}

	// Enable 2FA directly via a real TOTPService bound to the same store the
	// handler's AuthService uses, so user.TOTPEnabled reflects reality on
	// the next AuthenticateUser call.
	totpService := service.NewTOTPService(st, nil, 0)
	secret, err := totpService.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	if _, err := totpService.EnableTOTP(user.ID, secret); err != nil {
		t.Fatalf("EnableTOTP failed: %v", err)
	}

	body := `{"identifier":"ivan","password":"correct-horse-battery-staple"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/login", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Data["requires_2fa"] != true {
		t.Errorf("expected requires_2fa=true, got %v", env.Data["requires_2fa"])
	}

	var sawPending bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "2fa_pending" {
			sawPending = true
		}
		if c.Name == "user_session" {
			t.Error("did not expect a full user_session cookie before 2FA is completed")
		}
	}
	if !sawPending {
		t.Error("expected a 2fa_pending cookie to be set")
	}
}

// TestLoginFormValidationError verifies the form (no-JS) path re-renders the
// login page with a 401 status on invalid credentials, instead of
// redirecting.
func TestLoginFormValidationError(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)

	form := url.Values{"identifier": {"nobody"}, "password": {"whatever12"}}
	r := httptest.NewRequest(http.MethodPost, "/server/auth/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Login(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestLogoutWithCookieInvalidatesSession verifies Logout revokes the real
// session server-side (not just clearing the client cookie) — this is the
// regression test for the DeleteSession/RevokeSession table-mismatch bug
// fixed in auth_user.go.
func TestLogoutWithCookieInvalidatesSession(t *testing.T) {
	h, authService, _ := newAuthUserTestHandler(t)

	user, err := authService.RegisterUser(context.Background(), "judy", "judy@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("seed RegisterUser failed: %v", err)
	}
	sessionID, err := authService.CreateUserSession(context.Background(), user.ID, false)
	if err != nil {
		t.Fatalf("CreateUserSession failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/server/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: "user_session", Value: sessionID})
	w := httptest.NewRecorder()
	h.Logout(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}

	for _, c := range w.Result().Cookies() {
		if c.Name == "user_session" && c.MaxAge >= 0 {
			t.Errorf("expected user_session cookie to be cleared (negative MaxAge), got %d", c.MaxAge)
		}
	}

	if _, err := authService.ValidateUserSession(context.Background(), sessionID); err == nil {
		t.Error("expected the session to be invalidated server-side after logout, but it still validates")
	}
}

// TestLogoutWithoutCookie verifies Logout is a no-op success (not an error)
// when no session cookie is present.
func TestLogoutWithoutCookie(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/server/auth/logout", nil)
	w := httptest.NewRecorder()
	h.Logout(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d", w.Code)
	}
}

// TestLogoutJSONAccept verifies Logout returns a JSON success body instead
// of redirecting when the client sends Accept: application/json.
func TestLogoutJSONAccept(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/server/auth/logout", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	h.Logout(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Data["success"] != true {
		t.Errorf("expected success=true, got %v", env.Data["success"])
	}
}

// TestRegisterFormClosedWithoutInviteForbidden verifies that when public
// self-registration is closed (invite mode), a form POST with no invite token
// is rejected with 403 and no account is created (PART 34).
func TestRegisterFormClosedWithoutInviteForbidden(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)
	h.cfg.Server.Features.Users.Registration.Mode = "invite"

	form := url.Values{"username": {"frank"}, "email": {"frank@example.com"}, "password": {"longenoughpw"}}
	r := httptest.NewRequest(http.MethodPost, "/server/auth/register", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when registration is closed, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegisterFormInviteAllowsClosedRegistration verifies that a valid
// user-registration invite permits account creation even when public
// self-registration is closed, and that the invite is consumed on success
// (PART 34).
func TestRegisterFormInviteAllowsClosedRegistration(t *testing.T) {
	h, _, st := newAuthUserTestHandler(t)
	h.cfg.Server.Features.Users.Registration.Mode = "invite"

	inviteSvc := service.NewInviteService(st)
	plaintext, _, err := inviteSvc.CreateInvite(context.Background(), service.CreateInviteParams{
		Kind: service.InviteKindUserRegistration,
	})
	if err != nil {
		t.Fatalf("CreateInvite failed: %v", err)
	}

	form := url.Values{
		"username": {"grace"},
		"email":    {"grace@example.com"},
		"password": {"longenoughpw"},
		"invite":   {plaintext},
	}
	r := httptest.NewRequest(http.MethodPost, "/server/auth/register", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect after invited registration, got %d: %s", w.Code, w.Body.String())
	}

	// The single-use invite must now be consumed.
	if _, verr := inviteSvc.ValidateInvite(context.Background(), plaintext, service.InviteKindUserRegistration); verr == nil {
		t.Fatal("expected invite to be consumed after registration, but it is still valid")
	}
}

// TestRegisterFormInviteRejectedWhenDisabled verifies that even an otherwise
// valid user-registration invite is rejected once the mode is disabled — the
// account is not created and the invite is left unconsumed (PART 34).
func TestRegisterFormInviteRejectedWhenDisabled(t *testing.T) {
	h, _, st := newAuthUserTestHandler(t)
	h.cfg.Server.Features.Users.Registration.Mode = "disabled"

	inviteSvc := service.NewInviteService(st)
	plaintext, _, err := inviteSvc.CreateInvite(context.Background(), service.CreateInviteParams{
		Kind: service.InviteKindUserRegistration,
	})
	if err != nil {
		t.Fatalf("CreateInvite failed: %v", err)
	}

	form := url.Values{
		"username": {"mallory"},
		"email":    {"mallory@example.com"},
		"password": {"longenoughpw"},
		"invite":   {plaintext},
	}
	r := httptest.NewRequest(http.MethodPost, "/server/auth/register", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Register(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when registration is disabled, got %d: %s", w.Code, w.Body.String())
	}
	// The invite must remain unconsumed — the account was never created.
	if _, verr := inviteSvc.ValidateInvite(context.Background(), plaintext, service.InviteKindUserRegistration); verr != nil {
		t.Fatalf("expected invite to remain valid after a rejected disabled-mode registration, got: %v", verr)
	}
}

// TestRegisterPageInviteRendersWhenClosed verifies the registration page renders
// (200) with a valid invite token even when public registration is closed, and
// carries the token forward in a hidden field (PART 34).
func TestRegisterPageInviteRendersWhenClosed(t *testing.T) {
	h, _, st := newAuthUserTestHandler(t)
	h.cfg.Server.Features.Users.Registration.Mode = "invite"

	inviteSvc := service.NewInviteService(st)
	plaintext, _, err := inviteSvc.CreateInvite(context.Background(), service.CreateInviteParams{
		Kind: service.InviteKindUserRegistration,
	})
	if err != nil {
		t.Fatalf("CreateInvite failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/server/auth/register?invite="+url.QueryEscape(plaintext), nil)
	w := httptest.NewRecorder()
	h.RegisterPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid invite, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), plaintext) {
		t.Error("expected the invite token to be carried in the register form")
	}
}

// TestRegisterPageClosedWithoutInviteForbidden verifies the registration page is
// served with 403 when registration is closed and no invite is present.
func TestRegisterPageClosedWithoutInviteForbidden(t *testing.T) {
	h, _, _ := newAuthUserTestHandler(t)
	h.cfg.Server.Features.Users.Registration.Mode = "admin_only"

	r := httptest.NewRequest(http.MethodGet, "/server/auth/register", nil)
	w := httptest.NewRecorder()
	h.RegisterPage(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when registration is closed, got %d", w.Code)
	}
}
