package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/store"
	apktor "github.com/webappsgo/caslink/src/tor"
)

// newAdminTestHandler builds an AdminHandler backed by real, in-memory
// schema-backed services (no mocks), mirroring newAuthUserTestHandler in
// auth_user_test.go.
func newAdminTestHandler(t *testing.T) (*AdminHandler, *service.AuthService, *store.Store) {
	t.Helper()

	st := newSchemaTestStore(t)
	authService := service.NewAuthService(st)
	userAdminService := service.NewUserAdminService(st)
	auditService := service.NewAuditService(st)
	cfg := config.DefaultConfig()

	noTor := func() *apktor.TorManager { return nil }

	h := NewAdminHandler(authService, userAdminService, auditService, "test-version", "development", "admin", cfg, st, noTor)
	return h, authService, st
}

// seedAdminSession creates a primary admin and a valid session cookie for it.
func seedAdminSession(t *testing.T, h *AdminHandler, authService *service.AuthService) *http.Cookie {
	t.Helper()
	if err := authService.CreatePrimaryAdmin(t.Context(), "admin", "SuperSecret123!", "admin@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}
	admins, err := authService.AuthenticateAdmin(t.Context(), "admin", "SuperSecret123!")
	if err != nil {
		t.Fatalf("AuthenticateAdmin failed: %v", err)
	}
	sessionID, err := authService.CreateSession(t.Context(), admins.ID, false)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	return &http.Cookie{Name: "admin_session", Value: sessionID}
}

func TestLoginPageRendersFormWhenNoAdminExists(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/admin", nil)
	w := httptest.NewRecorder()
	h.LoginPage(w, r)

	// No admin exists yet, so NeedsSetup() is true and we redirect to /setup.
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/setup" {
		t.Fatalf("expected redirect to /setup, got %q", loc)
	}
}

func TestLoginPageRendersLoginFormWhenAdminExists(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	if err := authService.CreatePrimaryAdmin(t.Context(), "admin", "SuperSecret123!", "admin@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/server/admin", nil)
	w := httptest.NewRecorder()
	h.LoginPage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Admin Login") {
		t.Fatalf("expected login form in body, got %q", w.Body.String())
	}
}

func TestLoginPageRedirectsWhenAlreadyAuthenticated(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	r := httptest.NewRequest(http.MethodGet, "/server/admin", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.LoginPage(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/admin/dashboard" {
		t.Fatalf("expected redirect to dashboard, got %q", loc)
	}
}

func TestLoginSuccessSetsSessionCookieAndRedirects(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	if err := authService.CreatePrimaryAdmin(t.Context(), "admin", "SuperSecret123!", "admin@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}

	form := url.Values{"username": {"admin"}, "password": {"SuperSecret123!"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Login(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/server/admin/dashboard" {
		t.Fatalf("expected redirect to dashboard, got %q", loc)
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "admin_session" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected admin_session cookie to be set")
	}
}

func TestLoginFailureWrongPasswordShowsError(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	if err := authService.CreatePrimaryAdmin(t.Context(), "admin", "SuperSecret123!", "admin@example.com"); err != nil {
		t.Fatalf("CreatePrimaryAdmin failed: %v", err)
	}

	form := url.Values{"username": {"admin"}, "password": {"totally-wrong"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Login(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render login with error), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Invalid username or password") {
		t.Fatalf("expected invalid-credentials error in body, got %q", w.Body.String())
	}
}

func TestLoginFailureUnknownUserShowsError(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	form := url.Values{"username": {"nobody"}, "password": {"whatever12345"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Login(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Invalid username or password") {
		t.Fatalf("expected invalid-credentials error in body, got %q", w.Body.String())
	}
}

func TestLoginInvalidFormDataShowsError(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/server/admin/login", strings.NewReader("%zz"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.Login(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Invalid form data") {
		t.Fatalf("expected invalid form data message, got %q", w.Body.String())
	}
}

func TestLogoutClearsSessionAndRedirects(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/logout", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.Logout(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/admin" {
		t.Fatalf("expected redirect to base path, got %q", loc)
	}

	// The session should now be invalid.
	admin, err := authService.ValidateSession(t.Context(), cookie.Value)
	if err == nil && admin != nil {
		t.Fatalf("expected session to be invalidated after logout")
	}
}

func TestGetAdminFromSessionNilWithoutCookie(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/dashboard", nil)
	if admin := h.getAdminFromSession(r); admin != nil {
		t.Fatalf("expected nil admin without a session cookie, got %+v", admin)
	}
}

func TestGetAdminFromSessionNilWithInvalidCookie(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/dashboard", nil)
	r.AddCookie(&http.Cookie{Name: "admin_session", Value: "not-a-real-session"})
	if admin := h.getAdminFromSession(r); admin != nil {
		t.Fatalf("expected nil admin with an invalid session cookie, got %+v", admin)
	}
}

func TestGetAdminFromSessionValidCookie(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/dashboard", nil)
	r.AddCookie(cookie)
	admin := h.getAdminFromSession(r)
	if admin == nil {
		t.Fatalf("expected a valid admin from a valid session cookie")
	}
	if admin.Username != "admin" {
		t.Fatalf("expected username 'admin', got %q", admin.Username)
	}
}

func TestDashboardUnauthenticatedRedirects(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/dashboard", nil)
	w := httptest.NewRecorder()
	h.Dashboard(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/admin" {
		t.Fatalf("expected redirect to base path, got %q", loc)
	}
}

func TestDashboardAuthenticatedRenders(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/dashboard", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.Dashboard(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Dashboard") {
		t.Fatalf("expected dashboard content in body")
	}
}

func TestUserListUnauthenticatedRedirects(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/config/users", nil)
	w := httptest.NewRecorder()
	h.UserList(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
}

func TestUserListAuthenticatedRenders(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/config/users", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.UserList(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// withChiParam attaches a chi route param to the request context, mirroring
// what the chi router would inject at match time — required since these
// handlers are invoked directly rather than through the router.
func withChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestUserDetailAuthenticatedNotFound(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/config/users/999", nil)
	r.AddCookie(cookie)
	r = withChiParam(r, "id", "999")
	w := httptest.NewRecorder()
	h.UserDetail(w, r)

	// UserDetail renders a raw http.Error (plain text, no themed error card)
	// for a missing user rather than a 200 error card.
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPIUserListReturnsCanonicalEnvelope(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/admin/config/users", nil)
	w := httptest.NewRecorder()
	h.APIUserList(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// APIUserList must use the canonical list envelope per AI.md PART 9 /
	// .claude/rules/api-rules.md: {"ok":true,"data":[...],"pagination":{...}}.
	var body struct {
		OK         bool `json:"ok"`
		Data       any  `json:"data"`
		Pagination struct {
			Page  int `json:"page"`
			Limit int `json:"limit"`
			Total int `json:"total"`
			Pages int `json:"pages"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.OK {
		t.Fatalf("expected ok:true, got %s", w.Body.String())
	}
	if body.Pagination.Page != 1 {
		t.Errorf("pagination.page = %d, want 1", body.Pagination.Page)
	}
	if body.Pagination.Limit != 250 {
		t.Errorf("pagination.limit = %d, want 250", body.Pagination.Limit)
	}
}

func TestAPIUserDetailNotFoundReturnsError(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/admin/config/users/999", nil)
	r = withChiParam(r, "id", "999")
	w := httptest.NewRecorder()
	h.APIUserDetail(w, r)

	if w.Code == http.StatusOK {
		t.Fatalf("expected a non-200 error status for a missing user, got %d", w.Code)
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.OK {
		t.Fatalf("expected ok:false for a not-found user")
	}
}

func TestAPISuspendAndActivateUserNotFound(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/admin/config/users/999/suspend", strings.NewReader(`{}`))
	r = withChiParam(r, "id", "999")
	w := httptest.NewRecorder()
	h.APISuspendUser(w, r)
	if w.Code == http.StatusOK {
		t.Fatalf("expected a non-200 error status suspending a missing user, got %d", w.Code)
	}

	r2 := httptest.NewRequest(http.MethodPost, "/api/v1/server/admin/config/users/999/activate", nil)
	r2 = withChiParam(r2, "id", "999")
	w2 := httptest.NewRecorder()
	h.APIActivateUser(w2, r2)
	if w2.Code == http.StatusOK {
		t.Fatalf("expected a non-200 error status activating a missing user, got %d", w2.Code)
	}
}

func TestAPIRegenerateRecoveryKeysMissingUserReturnsNotFound(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	// ForceRegenerateRecoveryKeys verifies the target user exists before
	// deleting/inserting recovery_keys rows, per AI.md PART 9's standard
	// error code table (NOT_FOUND -> 404).
	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/admin/config/users/999/recovery-keys", nil)
	r = withChiParam(r, "id", "999")
	w := httptest.NewRecorder()
	h.APIRegenerateRecoveryKeys(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing user, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSuspendActivateUserHTMLUnauthenticatedRedirect(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/users/1/suspend", nil)
	r = withChiParam(r, "id", "1")
	w := httptest.NewRecorder()
	h.SuspendUser(w, r)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 for unauthenticated suspend, got %d", w.Code)
	}

	r2 := httptest.NewRequest(http.MethodPost, "/server/admin/config/users/1/activate", nil)
	r2 = withChiParam(r2, "id", "1")
	w2 := httptest.NewRecorder()
	h.ActivateUser(w2, r2)
	if w2.Code != http.StatusFound {
		t.Fatalf("expected 302 for unauthenticated activate, got %d", w2.Code)
	}
}
