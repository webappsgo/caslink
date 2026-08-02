package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/store"
	"github.com/webappsgo/caslink/src/server/tmpl"
)

// userSecurityTestFixture bundles a UserSecurityHandler backed by real
// services (AuthService, TOTPService, QRService, EmailService,
// WebAuthnService) against a single in-memory schema store, mirroring
// two_factor_test.go and auth_user_test.go so these tests exercise real
// business logic rather than mocks.
type userSecurityTestFixture struct {
	handler     *UserSecurityHandler
	authService *service.AuthService
	totpService *service.TOTPService
	webauthnSvc *service.WebAuthnService
	store       *store.Store
}

func newUserSecurityTestFixture(t *testing.T) *userSecurityTestFixture {
	t.Helper()
	return buildUserSecurityFixture(t, true)
}

// newUserSecurityTestFixtureNoWebAuthn builds a fixture with a nil
// WebAuthnService, mirroring a server that has not configured WebAuthn
// (rpid/origin unset) per NewUserSecurityHandler's optional dependency.
func newUserSecurityTestFixtureNoWebAuthn(t *testing.T) *userSecurityTestFixture {
	t.Helper()
	return buildUserSecurityFixture(t, false)
}

func buildUserSecurityFixture(t *testing.T, withWebAuthn bool) *userSecurityTestFixture {
	t.Helper()

	st := newSchemaTestStore(t)
	authService := service.NewAuthService(st)
	// Nil encryption key: TOTP secrets stored/read in plaintext, deterministic
	// for tests; encryption itself is covered by service/totp_test.go.
	totpService := service.NewTOTPService(st, nil, 0)
	qrService := service.NewQRService(st)
	cfg := config.DefaultConfig()
	emailService := service.NewEmailService(cfg)

	var webauthnSvc *service.WebAuthnService
	if withWebAuthn {
		var err error
		webauthnSvc, err = service.NewWebAuthnService(st, "localhost", "http://localhost")
		if err != nil {
			t.Fatalf("NewWebAuthnService failed: %v", err)
		}
	}

	renderer, err := tmpl.New()
	if err != nil {
		t.Fatalf("tmpl.New failed: %v", err)
	}

	h := NewUserSecurityHandler(authService, totpService, qrService, emailService, webauthnSvc, renderer, cfg)

	return &userSecurityTestFixture{
		handler:     h,
		authService: authService,
		totpService: totpService,
		webauthnSvc: webauthnSvc,
		store:       st,
	}
}

func (f *userSecurityTestFixture) registerUser(t *testing.T, username, email, password string) *service.User {
	t.Helper()
	u, err := f.authService.RegisterUser(context.Background(), username, email, password)
	if err != nil {
		t.Fatalf("RegisterUser(%s) failed: %v", username, err)
	}
	return u
}

func formRequest(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

// ---------------------------------------------------------------------
// Password
// ---------------------------------------------------------------------

func TestPasswordGetUnauthenticated(t *testing.T) {
	f := newUserSecurityTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/users/security/password", nil)
	w := httptest.NewRecorder()
	f.handler.Password(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPasswordGetRenders(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/security/password", nil), user)
	w := httptest.NewRecorder()
	f.handler.Password(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPasswordMethodNotAllowed(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodPut, "/users/security/password", nil), user)
	w := httptest.NewRecorder()
	f.handler.Password(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestPasswordChangeMissingFields(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/password", "current_password=&new_password=&confirm_password="), user)
	w := httptest.NewRecorder()
	f.handler.Password(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPasswordChangeMismatch(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	body := "current_password=correct-horse-battery-staple&new_password=new-password-1&confirm_password=new-password-2"
	r := withUser(formRequest(http.MethodPost, "/users/security/password", body), user)
	w := httptest.NewRecorder()
	f.handler.Password(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPasswordChangeTooShort(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	body := "current_password=correct-horse-battery-staple&new_password=short&confirm_password=short"
	r := withUser(formRequest(http.MethodPost, "/users/security/password", body), user)
	w := httptest.NewRecorder()
	f.handler.Password(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPasswordChangeWrongCurrentPassword(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	body := "current_password=totally-wrong-password&new_password=new-password-123&confirm_password=new-password-123"
	r := withUser(formRequest(http.MethodPost, "/users/security/password", body), user)
	w := httptest.NewRecorder()
	f.handler.Password(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	// The password must not have changed.
	if err := f.authService.VerifyPassword(user.ID, "correct-horse-battery-staple"); err != nil {
		t.Errorf("expected original password to still verify, got: %v", err)
	}
}

func TestPasswordChangeSuccess(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	body := "current_password=correct-horse-battery-staple&new_password=new-password-123&confirm_password=new-password-123"
	r := withUser(formRequest(http.MethodPost, "/users/security/password", body), user)
	w := httptest.NewRecorder()
	f.handler.Password(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/users/security?msg=password_changed" {
		t.Errorf("unexpected redirect location: %q", loc)
	}

	// The new password must verify and the old one must no longer work.
	if err := f.authService.VerifyPassword(user.ID, "new-password-123"); err != nil {
		t.Errorf("expected new password to verify, got: %v", err)
	}
	if err := f.authService.VerifyPassword(user.ID, "correct-horse-battery-staple"); err == nil {
		t.Error("expected old password to no longer verify")
	}
}

// ---------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------

func TestSessionsGetUnauthenticated(t *testing.T) {
	f := newUserSecurityTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/users/security/sessions", nil)
	w := httptest.NewRecorder()
	f.handler.Sessions(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSessionsGetRenders(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")
	if _, err := f.authService.CreateUserSession(context.Background(), user.ID, false); err != nil {
		t.Fatalf("CreateUserSession failed: %v", err)
	}

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/security/sessions", nil), user)
	w := httptest.NewRecorder()
	f.handler.Sessions(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSessionsRevokeMissingID(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/sessions", "session_id="), user)
	w := httptest.NewRecorder()
	f.handler.Sessions(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSessionsRevokeOwnSession(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")
	sessionID, err := f.authService.CreateUserSession(context.Background(), user.ID, false)
	if err != nil {
		t.Fatalf("CreateUserSession failed: %v", err)
	}

	r := withUser(formRequest(http.MethodPost, "/users/security/sessions", "session_id="+sessionID), user)
	w := httptest.NewRecorder()
	f.handler.Sessions(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}

	sessions, err := f.authService.GetUserSessions(context.Background(), user.ID, "user")
	if err != nil {
		t.Fatalf("GetUserSessions failed: %v", err)
	}
	for _, s := range sessions {
		if s.ID == sessionID {
			t.Fatalf("expected session %s to be revoked, but it is still active", sessionID)
		}
	}
}

// TestSessionsRevokeAnotherUsersSessionForbidden is a regression test for an
// IDOR: AuthService.RevokeSession deletes a session by ID alone with no
// owner check, so the handler itself must verify the session belongs to the
// caller before revoking it. Without that check, any authenticated user
// could revoke any other user's session by supplying its ID.
func TestSessionsRevokeAnotherUsersSessionForbidden(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	victim := f.registerUser(t, "victim", "victim@example.com", "correct-horse-battery-staple")
	attacker := f.registerUser(t, "attacker", "attacker@example.com", "correct-horse-battery-staple")

	victimSessionID, err := f.authService.CreateUserSession(context.Background(), victim.ID, false)
	if err != nil {
		t.Fatalf("CreateUserSession failed: %v", err)
	}

	r := withUser(formRequest(http.MethodPost, "/users/security/sessions", "session_id="+victimSessionID), attacker)
	w := httptest.NewRecorder()
	f.handler.Sessions(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when revoking another user's session, got %d: %s", w.Code, w.Body.String())
	}

	sessions, err := f.authService.GetUserSessions(context.Background(), victim.ID, "user")
	if err != nil {
		t.Fatalf("GetUserSessions failed: %v", err)
	}
	found := false
	for _, s := range sessions {
		if s.ID == victimSessionID {
			found = true
		}
	}
	if !found {
		t.Error("expected the victim's session to remain active after a forbidden cross-user revoke attempt")
	}
}

// ---------------------------------------------------------------------
// TwoFactor
// ---------------------------------------------------------------------

func TestTwoFactorGetUnauthenticated(t *testing.T) {
	f := newUserSecurityTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/users/security/2fa", nil)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestTwoFactorGetRendersDisabled(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/security/2fa", nil), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTwoFactorInvalidAction(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/2fa", "action=bogus"), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTwoFactorEnableNoPasswordShowsConfirm(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/2fa", "action=enable"), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTwoFactorEnableWrongPassword(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/2fa", "action=enable&password=wrong-password"), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestTwoFactorEnableCorrectPasswordShowsQR(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/2fa", "action=enable&password=correct-horse-battery-staple"), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// renderTOTPSetup passes QRDataURL as a plain string, so html/template's
	// URL sanitizer (isSafeUrl in html/template/url.go) rejects the "data:"
	// scheme — it only allows http/https/mailto unless the value is typed
	// template.URL — and replaces the whole attribute value with the
	// "#ZgotmplZ" safety placeholder. The QR image therefore never actually
	// renders. See TODO.AI.md's user_security.go QRDataURL entry.
	if !strings.Contains(w.Body.String(), "#ZgotmplZ") {
		t.Error("expected the data: URI to be filtered to the html/template safety placeholder")
	}
	if strings.Contains(w.Body.String(), "data:image/png;base64,") {
		t.Error("data: URI unexpectedly rendered unfiltered — QRDataURL may now be template.URL-typed; update this test to assert the real image renders")
	}
}

func TestTwoFactorVerifyMissingFields(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/2fa", "action=verify"), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTwoFactorVerifyInvalidCode(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	secret, err := f.totpService.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}

	body := "action=verify&secret=" + secret + "&code=000000"
	r := withUser(formRequest(http.MethodPost, "/users/security/2fa", body), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for wrong code, got %d: %s", w.Code, w.Body.String())
	}
	if f.totpService.HasTOTP(user.ID) {
		t.Error("2FA must not be enabled after a failed verification")
	}
}

func TestTwoFactorVerifySuccessEnablesAndShowsRecoveryKeys(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	secret, err := f.totpService.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	code := computeTOTPCode(t, secret, time.Now().Unix()/30)

	body := "action=verify&secret=" + secret + "&code=" + code
	r := withUser(formRequest(http.MethodPost, "/users/security/2fa", body), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !f.totpService.HasTOTP(user.ID) {
		t.Error("expected 2FA to be enabled after successful verification")
	}
	remaining, err := f.totpService.GetRemainingRecoveryKeyCount(user.ID)
	if err != nil {
		t.Fatalf("GetRemainingRecoveryKeyCount failed: %v", err)
	}
	if remaining != 10 {
		t.Errorf("expected 10 fresh recovery keys, got %d", remaining)
	}
}

func TestTwoFactorDisableNoPassword(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/2fa", "action=disable"), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTwoFactorDisableWrongPassword(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")
	secret, _ := f.totpService.GenerateTOTPSecret()
	if _, err := f.totpService.EnableTOTP(user.ID, secret); err != nil {
		t.Fatalf("EnableTOTP failed: %v", err)
	}

	r := withUser(formRequest(http.MethodPost, "/users/security/2fa", "action=disable&password=wrong-password"), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if !f.totpService.HasTOTP(user.ID) {
		t.Error("2FA must remain enabled after a failed disable attempt")
	}
}

func TestTwoFactorDisableSuccess(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")
	secret, _ := f.totpService.GenerateTOTPSecret()
	if _, err := f.totpService.EnableTOTP(user.ID, secret); err != nil {
		t.Fatalf("EnableTOTP failed: %v", err)
	}

	r := withUser(formRequest(http.MethodPost, "/users/security/2fa", "action=disable&password=correct-horse-battery-staple"), user)
	w := httptest.NewRecorder()
	f.handler.TwoFactor(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/users/security?msg=2fa_disabled" {
		t.Errorf("unexpected redirect location: %q", loc)
	}
	if f.totpService.HasTOTP(user.ID) {
		t.Error("expected 2FA to be disabled")
	}
}

// ---------------------------------------------------------------------
// Passkeys
// ---------------------------------------------------------------------

func TestPasskeysGetUnauthenticated(t *testing.T) {
	f := newUserSecurityTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/users/security/passkeys", nil)
	w := httptest.NewRecorder()
	f.handler.Passkeys(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPasskeysGetRendersWithNoCredentials(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/security/passkeys", nil), user)
	w := httptest.NewRecorder()
	f.handler.Passkeys(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPasskeysGetRendersWhenWebAuthnNotConfigured(t *testing.T) {
	f := newUserSecurityTestFixtureNoWebAuthn(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/security/passkeys", nil), user)
	w := httptest.NewRecorder()
	f.handler.Passkeys(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPasskeyActionWebAuthnNotConfigured(t *testing.T) {
	f := newUserSecurityTestFixtureNoWebAuthn(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/passkeys", "action=delete&credential_id=1"), user)
	w := httptest.NewRecorder()
	f.handler.Passkeys(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	// handlePasskeyAction calls respondJSON with an already-wrapped
	// {"ok":false,"error":...} map, but respondJSON unconditionally wraps
	// its argument again as {"ok":true,"data":{...}} (helpers.go:100-101
	// forbids pre-wrapped callers, but this call site does it anyway) — the
	// same respondJSON-misuse pattern already logged for org.go. The real
	// error code therefore lands at .data.error, and the top-level "ok" is
	// always true. See TODO.AI.md's user_security.go respondJSON entry.
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !env.OK || env.Data.OK || env.Data.Error != "WEBAUTHN_NOT_CONFIGURED" {
		t.Errorf("unexpected envelope: %+v", env)
	}
}

func TestPasskeyActionInvalidAction(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/passkeys", "action=bogus"), user)
	w := httptest.NewRecorder()
	f.handler.Passkeys(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPasskeyActionDeleteMissingCredentialID(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/passkeys", "action=delete&credential_id="), user)
	w := httptest.NewRecorder()
	f.handler.Passkeys(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPasskeyActionDeleteNoSuchCredentialFails(t *testing.T) {
	// DeleteCredential's SQL scopes the DELETE by "WHERE user_id = ? AND id
	// = ?" and treats zero rows affected as an error ("not found or not
	// owned by user"), so a bogus/foreign credential ID must surface as a
	// DELETE_FAILED error, never a silent success redirect. This also means
	// the delete path itself is NOT vulnerable to IDOR: it can never delete
	// another user's credential even given their credential ID.
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/passkeys", "action=delete&credential_id=does-not-exist"), user)
	w := httptest.NewRecorder()
	f.handler.Passkeys(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	// Same respondJSON double-wrap as TestPasskeyActionWebAuthnNotConfigured
	// above — see TODO.AI.md's user_security.go respondJSON entry.
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !env.OK || env.Data.OK || env.Data.Error != "DELETE_FAILED" {
		t.Errorf("unexpected envelope: %+v", env)
	}
}

func TestPasskeyBeginRegisterUnauthenticated(t *testing.T) {
	f := newUserSecurityTestFixture(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/passkeys/begin-register", nil)
	w := httptest.NewRecorder()
	f.handler.PasskeyBeginRegister(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPasskeyBeginRegisterWebAuthnNotConfigured(t *testing.T) {
	f := newUserSecurityTestFixtureNoWebAuthn(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodPost, "/api/v1/users/passkeys/begin-register", nil), user)
	w := httptest.NewRecorder()
	f.handler.PasskeyBeginRegister(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestPasskeyBeginRegisterSuccessSetsCeremonyCookie(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodPost, "/api/v1/users/passkeys/begin-register", nil), user)
	w := httptest.NewRecorder()
	f.handler.PasskeyBeginRegister(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sawCookie bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "wa_reg_session" {
			sawCookie = true
			if c.Value == "" {
				t.Error("expected a non-empty ceremony session cookie value")
			}
		}
	}
	if !sawCookie {
		t.Error("expected a wa_reg_session cookie to be set")
	}
}

func TestPasskeyFinishRegisterMissingCookie(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodPost, "/api/v1/users/passkeys/finish-register", nil), user)
	w := httptest.NewRecorder()
	f.handler.PasskeyFinishRegister(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPasskeyFinishRegisterSessionUserMismatch(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	owner := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")
	intruder := f.registerUser(t, "mallory", "mallory@example.com", "correct-horse-battery-staple")

	// Begin a registration ceremony as the owner to obtain a real, stored
	// session, then attempt to finish it while authenticated as a different
	// user with the same cookie value.
	beginReq := withUser(httptest.NewRequest(http.MethodPost, "/api/v1/users/passkeys/begin-register", nil), owner)
	beginW := httptest.NewRecorder()
	f.handler.PasskeyBeginRegister(beginW, beginReq)
	if beginW.Code != http.StatusOK {
		t.Fatalf("begin-register setup failed: %d: %s", beginW.Code, beginW.Body.String())
	}
	var cookieValue string
	for _, c := range beginW.Result().Cookies() {
		if c.Name == "wa_reg_session" {
			cookieValue = c.Value
		}
	}
	if cookieValue == "" {
		t.Fatal("expected a wa_reg_session cookie from begin-register")
	}

	finishReq := withUser(httptest.NewRequest(http.MethodPost, "/api/v1/users/passkeys/finish-register", nil), intruder)
	finishReq.AddCookie(&http.Cookie{Name: "wa_reg_session", Value: cookieValue})
	finishW := httptest.NewRecorder()
	f.handler.PasskeyFinishRegister(finishW, finishReq)

	if finishW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on session/user mismatch, got %d: %s", finishW.Code, finishW.Body.String())
	}
}

func TestPasskeyBeginLoginUnauthenticated(t *testing.T) {
	f := newUserSecurityTestFixture(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/passkeys/begin-login", nil)
	w := httptest.NewRecorder()
	f.handler.PasskeyBeginLogin(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPasskeyBeginLoginNoCredentialsRegistered(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodPost, "/api/v1/users/passkeys/begin-login", nil), user)
	w := httptest.NewRecorder()
	f.handler.PasskeyBeginLogin(w, r)

	// BeginLogin errors when the user has no registered passkey credentials.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPasskeyFinishLoginMissingCookie(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodPost, "/api/v1/users/passkeys/finish-login", nil), user)
	w := httptest.NewRecorder()
	f.handler.PasskeyFinishLogin(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------
// Recovery
// ---------------------------------------------------------------------

func TestRecoveryGetUnauthenticated(t *testing.T) {
	f := newUserSecurityTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/users/security/recovery", nil)
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRecoveryGetRendersWithoutMFA(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(httptest.NewRequest(http.MethodGet, "/users/security/recovery", nil), user)
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRecoveryInvalidAction(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/recovery", "action=bogus"), user)
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRecoveryRegenerateNoPassword(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/recovery", "action=regenerate"), user)
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRecoveryRegenerateWrongPassword(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")
	secret, _ := f.totpService.GenerateTOTPSecret()
	if _, err := f.totpService.EnableTOTP(user.ID, secret); err != nil {
		t.Fatalf("EnableTOTP failed: %v", err)
	}

	r := withUser(formRequest(http.MethodPost, "/users/security/recovery", "action=regenerate&password=wrong-password"), user)
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRecoveryRegenerateWithoutMFAEnabled(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")

	r := withUser(formRequest(http.MethodPost, "/users/security/recovery", "action=regenerate&password=correct-horse-battery-staple"), user)
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when 2FA isn't enabled, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRecoveryRegenerateSuccess(t *testing.T) {
	f := newUserSecurityTestFixture(t)
	user := f.registerUser(t, "alice", "alice@example.com", "correct-horse-battery-staple")
	secret, _ := f.totpService.GenerateTOTPSecret()
	oldKeys, err := f.totpService.EnableTOTP(user.ID, secret)
	if err != nil {
		t.Fatalf("EnableTOTP failed: %v", err)
	}

	r := withUser(formRequest(http.MethodPost, "/users/security/recovery", "action=regenerate&password=correct-horse-battery-staple"), user)
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	remaining, err := f.totpService.GetRemainingRecoveryKeyCount(user.ID)
	if err != nil {
		t.Fatalf("GetRemainingRecoveryKeyCount failed: %v", err)
	}
	if remaining != 10 {
		t.Errorf("expected 10 fresh recovery keys after regeneration, got %d", remaining)
	}

	// The response body must contain the new keys, not the old ones.
	body := w.Body.String()
	if strings.Contains(body, oldKeys[0]) {
		t.Error("expected the old recovery key to no longer be shown")
	}
}
