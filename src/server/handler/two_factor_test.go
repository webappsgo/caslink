package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/caslink/src/server/service"
)

// computeTOTPCode reimplements RFC 6238/4226 independently of
// TOTPService.generateTOTPCode so tests exercise real crypto against an
// independently derived expected value, not a copy of the production
// algorithm.
func computeTOTPCode(t *testing.T, secretB32 string, timeStep int64) string {
	t.Helper()
	secretBytes, err := base32.StdEncoding.DecodeString(strings.ToUpper(secretB32))
	if err != nil {
		t.Fatalf("failed to decode secret: %v", err)
	}
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(timeStep))
	h := hmac.New(sha1.New, secretBytes)
	h.Write(buf)
	hash := h.Sum(nil)
	offset := hash[len(hash)-1] & 0x0F
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7FFFFFFF
	return fmt.Sprintf("%06d", truncated%1000000)
}

// twoFactorTestFixture bundles everything needed to drive TwoFactorHandler
// against a real store/service stack, with 2FA already enabled for the user.
type twoFactorTestFixture struct {
	handler      *TwoFactorHandler
	authService  *service.AuthService
	totpService  *service.TOTPService
	user         *service.User
	secret       string
	recoveryKeys []string
}

func newTwoFactorTestFixture(t *testing.T) *twoFactorTestFixture {
	t.Helper()

	st := newSchemaTestStore(t)
	authService := service.NewAuthService(st)
	// Pass nil encryption key so the TOTP secret is stored (and read back)
	// in plaintext — deterministic and dependency-free for these handler
	// tests; encryption itself is covered by service/totp_test.go.
	totpService := service.NewTOTPService(st, nil, 0)

	user, err := authService.RegisterUser(context.Background(), "bob", "bob@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	secret, err := totpService.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	recoveryKeys, err := totpService.EnableTOTP(user.ID, secret)
	if err != nil {
		t.Fatalf("EnableTOTP failed: %v", err)
	}

	return &twoFactorTestFixture{
		handler:      NewTwoFactorHandler(authService, totpService),
		authService:  authService,
		totpService:  totpService,
		user:         user,
		secret:       secret,
		recoveryKeys: recoveryKeys,
	}
}

// pendingSession creates a real user_sessions row (as AuthUserHandler.Login
// does for a TOTP-enabled user) and returns its ID for use as the
// "2fa_pending" cookie value.
func (f *twoFactorTestFixture) pendingSession(t *testing.T) string {
	t.Helper()
	sessionID, err := f.authService.CreateUserSession(context.Background(), f.user.ID, false)
	if err != nil {
		t.Fatalf("CreateUserSession failed: %v", err)
	}
	return sessionID
}

func withPendingCookie(r *http.Request, value string) *http.Request {
	r.AddCookie(&http.Cookie{Name: "2fa_pending", Value: value})
	return r
}

// TestVerifyPageNoPendingSession verifies the verify page requires a
// "2fa_pending" cookie.
func TestVerifyPageNoPendingSession(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/2fa", nil)
	w := httptest.NewRecorder()
	f.handler.VerifyPage(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestVerifyPageWithPendingSession verifies the verify page renders once a
// pending session cookie is present.
func TestVerifyPageWithPendingSession(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/2fa", nil)
	r = withPendingCookie(r, f.pendingSession(t))
	w := httptest.NewRecorder()
	f.handler.VerifyPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Two-Factor") {
		t.Errorf("expected verify form in body, got: %s", w.Body.String())
	}
}

// TestVerifyMethodNotAllowed verifies GET is rejected on the verify submit
// endpoint.
func TestVerifyMethodNotAllowed(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/2fa", nil)
	w := httptest.NewRecorder()
	f.handler.Verify(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestVerifyInvalidCodeFormat verifies a non-6-digit code is rejected with
// 400 before any session/secret lookup occurs.
func TestVerifyInvalidCodeFormat(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	form := strings.NewReader("code=12")
	r := httptest.NewRequest(http.MethodPost, "/server/auth/2fa", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.handler.Verify(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestVerifyNoPendingSession verifies a well-formed code is still rejected
// with 401 when there's no pending session cookie.
func TestVerifyNoPendingSession(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	form := strings.NewReader("code=123456")
	r := httptest.NewRequest(http.MethodPost, "/server/auth/2fa", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.handler.Verify(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestVerifyInvalidPendingSession verifies a garbage/unknown pending session
// cookie is rejected with 401.
func TestVerifyInvalidPendingSession(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	form := strings.NewReader("code=123456")
	r := httptest.NewRequest(http.MethodPost, "/server/auth/2fa", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withPendingCookie(r, "not-a-real-session")
	w := httptest.NewRecorder()
	f.handler.Verify(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestVerifyWrongCode verifies a syntactically valid but wrong 6-digit code
// is rejected with 401 against a real TOTP secret.
func TestVerifyWrongCode(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	form := strings.NewReader("code=000000")
	r := httptest.NewRequest(http.MethodPost, "/server/auth/2fa", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withPendingCookie(r, f.pendingSession(t))
	w := httptest.NewRecorder()
	f.handler.Verify(w, r)

	// A random 6-digit guess against a real secret should not collide.
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong code, got %d: %s", w.Code, w.Body.String())
	}
}

// TestVerifySuccess drives the full happy path with a real TOTP code
// computed independently from the enrolled secret: pending session accepted,
// full user_session cookie issued, 2fa_pending cookie cleared, redirect to
// the dashboard, and — critically — the consumed pending session token must
// no longer validate (single-use enforcement across the promotion).
func TestVerifySuccess(t *testing.T) {
	f := newTwoFactorTestFixture(t)
	pending := f.pendingSession(t)

	code := computeTOTPCode(t, f.secret, time.Now().Unix()/30)

	form := strings.NewReader("code=" + code)
	r := httptest.NewRequest(http.MethodPost, "/server/auth/2fa", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withPendingCookie(r, pending)
	w := httptest.NewRecorder()
	f.handler.Verify(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/users/profile" {
		t.Errorf("expected redirect to /users/profile, got %q", loc)
	}

	var sawFullSession, sawClearedPending bool
	for _, c := range w.Result().Cookies() {
		switch c.Name {
		case "user_session":
			if c.Value == "" {
				t.Error("expected non-empty user_session cookie value")
			}
			sawFullSession = true
		case "2fa_pending":
			if c.MaxAge >= 0 {
				t.Errorf("expected 2fa_pending cookie to be cleared (negative MaxAge), got %d", c.MaxAge)
			}
			sawClearedPending = true
		}
	}
	if !sawFullSession {
		t.Error("expected a user_session cookie to be set")
	}
	if !sawClearedPending {
		t.Error("expected the 2fa_pending cookie to be cleared")
	}

	// The consumed pending session must no longer validate — otherwise the
	// short-lived 2FA-pending token would remain usable as a full session
	// credential for the rest of its original lifetime.
	if _, err := f.authService.ValidateUserSession(context.Background(), pending); err == nil {
		t.Error("expected the consumed pending session to be invalidated, but it still validates")
	}
}

// TestRecoveryPageRenders verifies the recovery entry page always renders.
func TestRecoveryPageRenders(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/2fa/recovery", nil)
	w := httptest.NewRecorder()
	f.handler.RecoveryPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "recovery_key") {
		t.Errorf("expected recovery form in body, got: %s", w.Body.String())
	}
}

// TestRecoveryMethodNotAllowed verifies GET is rejected on the recovery
// submit endpoint.
func TestRecoveryMethodNotAllowed(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/2fa/recovery", nil)
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestRecoveryMissingKey verifies an empty recovery_key is rejected with
// 400.
func TestRecoveryMissingKey(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	form := strings.NewReader("recovery_key=")
	r := httptest.NewRequest(http.MethodPost, "/server/auth/2fa/recovery", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestRecoveryNoPendingSession verifies a well-formed key is still rejected
// with 401 when there's no pending session cookie.
func TestRecoveryNoPendingSession(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	form := strings.NewReader("recovery_key=" + f.recoveryKeys[0])
	r := httptest.NewRequest(http.MethodPost, "/server/auth/2fa/recovery", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestRecoveryInvalidKey verifies a bogus recovery key is rejected with 401
// against a real enrolled key set.
func TestRecoveryInvalidKey(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	form := strings.NewReader("recovery_key=00000000-0000")
	r := httptest.NewRequest(http.MethodPost, "/server/auth/2fa/recovery", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withPendingCookie(r, f.pendingSession(t))
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRecoverySuccessSingleUse verifies a real, valid recovery key is
// accepted exactly once: first use succeeds and shows the decremented
// remaining count, second use of the SAME key is rejected.
func TestRecoverySuccessSingleUse(t *testing.T) {
	f := newTwoFactorTestFixture(t)
	key := f.recoveryKeys[0]

	form := strings.NewReader("recovery_key=" + key)
	r := httptest.NewRequest(http.MethodPost, "/server/auth/2fa/recovery", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withPendingCookie(r, f.pendingSession(t))
	w := httptest.NewRecorder()
	f.handler.Recovery(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "9 recovery keys remaining") {
		t.Errorf("expected 9 remaining keys after single use of 10, got: %s", w.Body.String())
	}

	// Reusing the same key must fail (single-use per PART 23).
	form2 := strings.NewReader("recovery_key=" + key)
	r2 := httptest.NewRequest(http.MethodPost, "/server/auth/2fa/recovery", form2)
	r2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r2 = withPendingCookie(r2, f.pendingSession(t))
	w2 := httptest.NewRecorder()
	f.handler.Recovery(w2, r2)

	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected reused recovery key to be rejected with 401, got %d", w2.Code)
	}
}

// TestRecoveryOptionsPageNoPendingSession verifies the options page requires
// a pending session cookie.
func TestRecoveryOptionsPageNoPendingSession(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/2fa/recovery/options", nil)
	w := httptest.NewRecorder()
	f.handler.RecoveryOptionsPage(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestRecoveryOptionsPageWithPendingSession verifies the options page
// renders with the caller's real remaining-key count.
func TestRecoveryOptionsPageWithPendingSession(t *testing.T) {
	f := newTwoFactorTestFixture(t)

	r := httptest.NewRequest(http.MethodGet, "/server/auth/2fa/recovery/options", nil)
	r = withPendingCookie(r, f.pendingSession(t))
	w := httptest.NewRecorder()
	f.handler.RecoveryOptionsPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "10 recovery keys remaining") {
		t.Errorf("expected all 10 recovery keys still remaining, got: %s", w.Body.String())
	}
}
