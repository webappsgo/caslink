package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/store"
)

// newSchemaTestStore opens a fresh in-memory SQLite DB per test and runs the
// full InitSchema against it, mirroring src/server/handler's
// handler_testutil_test.go so these tests exercise the real schema instead
// of a hand-duplicated subset.
func newSchemaTestStore(t *testing.T) *store.Store {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	st := store.NewTestStore(db)
	if err := st.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	return st
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// TestJSONErrCode verifies the HTTP status → canonical error-code mapping.
func TestJSONErrCode(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "BAD_REQUEST"},
		{http.StatusUnauthorized, "UNAUTHORIZED"},
		{http.StatusForbidden, "FORBIDDEN"},
		{http.StatusNotFound, "NOT_FOUND"},
		{http.StatusConflict, "CONFLICT"},
		{422, "VALIDATION_FAILED"},
		{http.StatusTooManyRequests, "RATE_LIMITED"},
		{http.StatusServiceUnavailable, "MAINTENANCE"},
		{http.StatusInternalServerError, "SERVER_ERROR"},
		{599, "SERVER_ERROR"},
		{http.StatusTeapot, "ERROR"},
	}
	for _, tt := range tests {
		if got := jsonErrCode(tt.status); got != tt.want {
			t.Errorf("jsonErrCode(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// TestJSONEscape verifies the escaped characters that would otherwise break
// a bare JSON string literal.
func TestJSONEscape(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`plain`, `plain`},
		{`back\slash`, `back\\slash`},
		{`say "hi"`, `say \"hi\"`},
		{"line\nbreak", `line\nbreak`},
		{"carriage\rreturn", `carriage\rreturn`},
	}
	for _, tt := range tests {
		if got := jsonEscape(tt.in); got != tt.want {
			t.Errorf("jsonEscape(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestWriteJSONError verifies the canonical error envelope shape and that
// the message is escaped and the status code is applied.
func TestWriteJSONError(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSONError(w, http.StatusForbidden, `bad "input"`)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	want := `{"ok":false,"error":"FORBIDDEN","message":"bad \"input\""}`
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// TestSecurityHeadersMiddlewareProductionTLS verifies HSTS and the strict
// (blocking) CSP header are set when TLS is enabled and dev mode is off.
func TestSecurityHeadersMiddlewareProductionTLS(t *testing.T) {
	mw := SecurityHeadersMiddleware(true, false)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	h := w.Header()
	if h.Get("Strict-Transport-Security") == "" {
		t.Error("expected Strict-Transport-Security header when tlsEnabled=true")
	}
	if h.Get("Content-Security-Policy") == "" {
		t.Error("expected blocking Content-Security-Policy header in production mode")
	}
	if h.Get("Content-Security-Policy-Report-Only") != "" {
		t.Error("did not expect Content-Security-Policy-Report-Only in production mode")
	}
	if got := h.Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options = %q, want SAMEORIGIN", got)
	}
	if h.Get("X-Request-Id") == "" {
		t.Error("expected an X-Request-Id header to be set")
	}
}

// TestSecurityHeadersMiddlewareDevNoTLS verifies HSTS is absent without TLS
// and CSP is sent as report-only in dev mode, and that an existing
// X-Request-Id is echoed back rather than regenerated.
func TestSecurityHeadersMiddlewareDevNoTLS(t *testing.T) {
	mw := SecurityHeadersMiddleware(false, true)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "existing-id")
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	h := w.Header()
	if h.Get("Strict-Transport-Security") != "" {
		t.Error("did not expect Strict-Transport-Security header when tlsEnabled=false")
	}
	if h.Get("Content-Security-Policy") != "" {
		t.Error("did not expect blocking CSP header in dev mode")
	}
	if h.Get("Content-Security-Policy-Report-Only") == "" {
		t.Error("expected Content-Security-Policy-Report-Only header in dev mode")
	}
	if got := h.Get("X-Request-Id"); got != "existing-id" {
		t.Errorf("X-Request-Id = %q, want echoed existing-id", got)
	}
}

// TestRateBucketAllow verifies the sliding-window limiter allows up to
// `limit` requests and rejects the next one within the same window.
func TestRateBucketAllow(t *testing.T) {
	b := &rateBucket{}
	for i := 0; i < 3; i++ {
		if !b.allow(3, time.Minute) {
			t.Fatalf("request %d unexpectedly denied", i)
		}
	}
	if b.allow(3, time.Minute) {
		t.Error("4th request within limit=3 should have been denied")
	}
}

// TestRateBucketAllowWindowExpiry verifies old requests are evicted once the
// sliding window has passed, freeing capacity again.
func TestRateBucketAllowWindowExpiry(t *testing.T) {
	b := &rateBucket{}
	window := 50 * time.Millisecond
	if !b.allow(1, window) {
		t.Fatal("first request should be allowed")
	}
	if b.allow(1, window) {
		t.Fatal("second immediate request should be denied")
	}
	time.Sleep(window + 20*time.Millisecond)
	if !b.allow(1, window) {
		t.Error("request after window expiry should be allowed")
	}
}

// TestRateLimiterAllowPerKey verifies buckets are tracked independently per
// key (e.g. per IP+path combination).
func TestRateLimiterAllowPerKey(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*rateBucket)}
	if !rl.Allow("1.2.3.4", 1, time.Minute) {
		t.Fatal("first request for key A should be allowed")
	}
	if rl.Allow("1.2.3.4", 1, time.Minute) {
		t.Error("second request for key A should be denied")
	}
	if !rl.Allow("5.6.7.8", 1, time.Minute) {
		t.Error("first request for distinct key B should be allowed")
	}
}

// TestRateLimitMiddlewareGetAlwaysAllowed verifies GET requests bypass rate
// limiting entirely, regardless of path.
func TestRateLimitMiddlewareGetAlwaysAllowed(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*rateBucket)}
	mw := RateLimitMiddleware(rl)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/server/auth/login", nil)
		w := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET request %d: status = %d, want 200", i, w.Code)
		}
	}
}

// TestRateLimitMiddlewareLoginPathLimited verifies POST /server/auth/login
// is limited to 5 requests, with the 6th returning 429 and the canonical
// RATE_LIMITED error envelope.
func TestRateLimitMiddlewareLoginPathLimited(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*rateBucket)}
	mw := RateLimitMiddleware(rl)
	var last *httptest.ResponseRecorder
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/server/auth/login", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		w := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(w, req)
		last = w
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("6th login attempt: status = %d, want 429", last.Code)
	}
	if got := last.Body.String(); !contains(got, `"error":"RATE_LIMITED"`) {
		t.Errorf("body = %q, want RATE_LIMITED envelope", got)
	}
}

// TestRateLimitMiddlewareUnmatchedPathBypasses verifies a POST to a path not
// matching login/register/password/2fa is never rate-limited.
func TestRateLimitMiddlewareUnmatchedPathBypasses(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*rateBucket)}
	mw := RateLimitMiddleware(rl)
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/urls", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		w := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (unmatched path never limited)", i, w.Code)
		}
	}
}

// TestRateLimitMiddlewareDomainAddLimited verifies POST .../domains (add a
// custom domain, PART 36) is limited to 10 per hour, 11th returns 429.
func TestRateLimitMiddlewareDomainAddLimited(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*rateBucket)}
	mw := RateLimitMiddleware(rl)
	var last *httptest.ResponseRecorder
	for i := 0; i < 11; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		w := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(w, req)
		last = w
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("11th domain add: status = %d, want 429", last.Code)
	}
}

// TestRateLimitMiddlewareDomainVerifySharesBucketAcrossDomains verifies that
// verify attempts against DIFFERENT domain strings from one IP share a single
// bucket (per-IP-per-rule key), so an attacker cannot sidestep the 15/hour
// verify limit by cycling distinct domains. Each verify triggers an outbound
// DNS TXT lookup, so this shared-bucket behavior is the abuse guard.
func TestRateLimitMiddlewareDomainVerifySharesBucketAcrossDomains(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*rateBucket)}
	mw := RateLimitMiddleware(rl)
	var last *httptest.ResponseRecorder
	for i := 0; i < 16; i++ {
		// A distinct domain each iteration — the bucket must still be shared.
		path := fmt.Sprintf("/api/v1/users/domains/d%d.example.com/verify", i)
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.RemoteAddr = "9.9.9.9:1234"
		w := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(w, req)
		last = w
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("16th verify (distinct domains): status = %d, want 429 (shared bucket)", last.Code)
	}
}

// TestRateLimitMiddlewareDomainNamedLikeLoginNotMisclassified guards the switch
// ordering: a domain literally named "login.example.com" must be treated as a
// domain operation (10/hour add), not the stricter 5/15min login rule that a
// naive substring match on "/login" would otherwise select.
func TestRateLimitMiddlewareDomainNamedLikeLoginNotMisclassified(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*rateBucket)}
	mw := RateLimitMiddleware(rl)
	// The add rule allows 10; if it were misclassified as "login" it would
	// block after 5. Send 6 and require all 6 to pass.
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains", nil)
		req.RemoteAddr = "8.8.8.8:1234"
		w := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("add request %d: status = %d, want 200 (domain_add rule, not login)", i, w.Code)
		}
	}
	// A verify against a domain named like a login endpoint must classify as
	// domain_verify (15/hour), not login (5/15min): 6 must all pass.
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users/domains/login.example.com/verify", nil)
		req.RemoteAddr = "8.8.8.8:1234"
		w := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("verify request %d: status = %d, want 200 (domain_verify rule, not login)", i, w.Code)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// TestRealIP covers the X-Forwarded-For, X-Real-IP, and RemoteAddr fallback
// precedence.
func TestRealIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xReal      string
		remoteAddr string
		want       string
	}{
		{"x-forwarded-for wins, first entry only", "1.1.1.1, 2.2.2.2", "3.3.3.3", "4.4.4.4:80", "1.1.1.1"},
		{"x-real-ip used when no xff", "", "3.3.3.3", "4.4.4.4:80", "3.3.3.3"},
		{"falls back to RemoteAddr port stripped", "", "", "5.5.5.5:1234", "5.5.5.5"},
		{"RemoteAddr with no port returned as-is", "", "", "no-port-here", "no-port-here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xReal != "" {
				req.Header.Set("X-Real-IP", tt.xReal)
			}
			if got := realIP(req); got != tt.want {
				t.Errorf("realIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCSRFMiddlewareExemptPaths verifies /.well-known/* and healthz routes
// bypass CSRF validation entirely, even on unsafe methods with no token.
func TestCSRFMiddlewareExemptPaths(t *testing.T) {
	mw := CSRFMiddleware()
	paths := []string{"/.well-known/security.txt", "/server/healthz", "/api/v1/server/healthz"}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodPost, p, nil)
		w := httptest.NewRecorder()
		mw(okHandler()).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("path %q: status = %d, want 200 (exempt)", p, w.Code)
		}
	}
}

// TestCSRFMiddlewareSafeMethodSetsCookie verifies a GET request passes
// through and receives a csrf_token cookie when none was present.
func TestCSRFMiddlewareSafeMethodSetsCookie(t *testing.T) {
	mw := CSRFMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/users/settings", nil)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == csrfCookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected csrf_token cookie to be set on safe-method request")
	}
}

// TestCSRFMiddlewareBearerExempt verifies a POST with a Bearer Authorization
// header bypasses cookie CSRF validation.
func TestCSRFMiddlewareBearerExempt(t *testing.T) {
	mw := CSRFMiddleware()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/urls", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (bearer-exempt)", w.Code)
	}
}

// TestCSRFMiddlewareMissingCookieRejected verifies an unsafe-method request
// with no csrf_token cookie at all is rejected with 403.
func TestCSRFMiddlewareMissingCookieRejected(t *testing.T) {
	mw := CSRFMiddleware()
	req := httptest.NewRequest(http.MethodPost, "/users/settings", nil)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if !contains(w.Body.String(), `"error":"FORBIDDEN"`) {
		t.Errorf("body = %q, want FORBIDDEN envelope", w.Body.String())
	}
}

// TestCSRFMiddlewareMismatchedTokenRejected verifies a cookie present but a
// mismatched (or missing) submitted token is rejected.
func TestCSRFMiddlewareMismatchedTokenRejected(t *testing.T) {
	mw := CSRFMiddleware()
	req := httptest.NewRequest(http.MethodPost, "/users/settings", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "cookie-value"})
	req.Header.Set(csrfHeaderName, "different-value")
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

// TestCSRFMiddlewareMatchingHeaderTokenAccepted verifies a matching cookie +
// X-CSRF-Token header pair is accepted.
func TestCSRFMiddlewareMatchingHeaderTokenAccepted(t *testing.T) {
	mw := CSRFMiddleware()
	req := httptest.NewRequest(http.MethodPost, "/users/settings", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "matching-value"})
	req.Header.Set(csrfHeaderName, "matching-value")
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (matching token accepted)", w.Code)
	}
}

// TestCSRFMiddlewareMatchingFormFieldAccepted verifies the form-field
// fallback (_csrf) is used when no X-CSRF-Token header is present.
func TestCSRFMiddlewareMatchingFormFieldAccepted(t *testing.T) {
	mw := CSRFMiddleware()
	req := httptest.NewRequest(http.MethodPost, "/users/settings?"+csrfFormField+"=matching-value", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "matching-value"})
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (matching form field accepted via query)", w.Code)
	}
}

// TestEnsureCSRFCookieNoOpWhenPresent verifies ensureCSRFCookie does not
// overwrite an already-present, non-empty cookie.
func TestEnsureCSRFCookieNoOpWhenPresent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "existing"})
	w := httptest.NewRecorder()
	ensureCSRFCookie(w, req)

	if len(w.Result().Cookies()) != 0 {
		t.Errorf("expected no Set-Cookie when a valid cookie already exists, got %v", w.Result().Cookies())
	}
}

// TestUserAuthMiddlewareNoCookieRedirects verifies a missing user_session
// cookie redirects to the login page without touching the auth service.
func TestUserAuthMiddlewareNoCookieRedirects(t *testing.T) {
	mw := UserAuthMiddleware(nil)
	req := httptest.NewRequest(http.MethodGet, "/users/dashboard", nil)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/auth/login" {
		t.Errorf("Location = %q, want /server/auth/login", loc)
	}
}

// TestUserAuthMiddlewareInvalidSessionRedirects verifies an invalid session
// cookie value (rejected by a real AuthService/store) also redirects.
func TestUserAuthMiddlewareInvalidSessionRedirects(t *testing.T) {
	st := newSchemaTestStore(t)
	authService := service.NewAuthService(st)
	mw := UserAuthMiddleware(authService)

	req := httptest.NewRequest(http.MethodGet, "/users/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "user_session", Value: "does-not-exist"})
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
}

// TestAdminAuthMiddlewareDefaultsAdminPath verifies an empty adminPath falls
// back to "admin" in the redirect URL, and a missing cookie redirects.
func TestAdminAuthMiddlewareDefaultsAdminPath(t *testing.T) {
	mw := AdminAuthMiddleware(nil, "")
	req := httptest.NewRequest(http.MethodGet, "/server/admin", nil)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/admin" {
		t.Errorf("Location = %q, want /server/admin", loc)
	}
}

// TestAdminAuthMiddlewareCustomPathRedirect verifies a custom adminPath is
// reflected in the redirect URL.
func TestAdminAuthMiddlewareCustomPathRedirect(t *testing.T) {
	mw := AdminAuthMiddleware(nil, "control-panel")
	req := httptest.NewRequest(http.MethodGet, "/server/control-panel", nil)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if loc := w.Header().Get("Location"); loc != "/server/control-panel" {
		t.Errorf("Location = %q, want /server/control-panel", loc)
	}
}

// TestOrgMemberMiddlewareNoUserUnauthorized verifies a request with no user
// in context is rejected with 401 before touching orgService.
func TestOrgMemberMiddlewareNoUserUnauthorized(t *testing.T) {
	mw := OrgMemberMiddleware(nil)
	req := httptest.NewRequest(http.MethodGet, "/orgs/acme", nil)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

// TestOrgMemberMiddlewareMissingSlugBadRequest verifies a request with a
// user in context but no chi "slug" URL param returns 400.
func TestOrgMemberMiddlewareMissingSlugBadRequest(t *testing.T) {
	mw := OrgMemberMiddleware(nil)
	req := httptest.NewRequest(http.MethodGet, "/orgs/", nil)
	ctx := context.WithValue(req.Context(), userContextKey, &service.User{ID: 1})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestBearerAuthMiddlewareMissingHeaderUnauthorized verifies a request with
// no Authorization header is rejected 401 with a WWW-Authenticate header,
// without touching tokenService.
func TestBearerAuthMiddlewareMissingHeaderUnauthorized(t *testing.T) {
	mw := BearerAuthMiddleware(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/urls", nil)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header")
	}
}

// TestBearerAuthMiddlewareInvalidTokenUnauthorized verifies an invalid
// bearer token (checked against a real, empty-token TokenService/store) is
// rejected 401.
func TestBearerAuthMiddlewareInvalidTokenUnauthorized(t *testing.T) {
	st := newSchemaTestStore(t)
	tokenService := service.NewTokenService(st)
	mw := BearerAuthMiddleware(tokenService)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/urls", nil)
	req.Header.Set("Authorization", "Bearer adm_doesnotexist")
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

// TestRequireBearerAdmin verifies the admin-scope gate: only a token whose
// OwnerType is "admin" (an adm_ token) passes; usr_/org_ tokens and a missing
// token record are rejected 403 (PART 24: usr_ tokens must be rejected on
// admin endpoints).
func TestRequireBearerAdmin(t *testing.T) {
	cases := []struct {
		name     string
		rec      *service.TokenRecord
		wantCode int
	}{
		{"admin token passes", &service.TokenRecord{OwnerType: "admin"}, http.StatusOK},
		{"admin token case-insensitive", &service.TokenRecord{OwnerType: "Admin"}, http.StatusOK},
		{"user token rejected", &service.TokenRecord{OwnerType: "user"}, http.StatusForbidden},
		{"org token rejected", &service.TokenRecord{OwnerType: "org"}, http.StatusForbidden},
		{"nil record rejected", nil, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/server/admin/config/users", nil)
			if tc.rec != nil {
				ctx := context.WithValue(req.Context(), bearerContextKey, tc.rec)
				req = req.WithContext(ctx)
			}
			w := httptest.NewRecorder()
			RequireBearerAdmin(okHandler()).ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantCode)
			}
		})
	}
}

// TestRequireBearerAdminNoContextValue verifies a request with no bearer
// record in context at all (middleware misordered / never ran) is rejected 403
// rather than panicking.
func TestRequireBearerAdminNoContextValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/admin/config/users", nil)
	w := httptest.NewRecorder()
	RequireBearerAdmin(okHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

// TestGetUserFromContext covers both the present and absent cases.
func TestGetUserFromContext(t *testing.T) {
	user := &service.User{ID: 42}
	ctx := context.WithValue(context.Background(), userContextKey, user)
	got, ok := GetUserFromContext(ctx)
	if !ok || got != user {
		t.Errorf("GetUserFromContext() = %v, %v, want %v, true", got, ok, user)
	}

	_, ok = GetUserFromContext(context.Background())
	if ok {
		t.Error("expected ok=false for empty context")
	}
}

// TestGetAdminFromContext covers both the present and absent cases.
func TestGetAdminFromContext(t *testing.T) {
	admin := &service.Admin{ID: 7}
	ctx := context.WithValue(context.Background(), adminContextKey, admin)
	got, ok := GetAdminFromContext(ctx)
	if !ok || got != admin {
		t.Errorf("GetAdminFromContext() = %v, %v, want %v, true", got, ok, admin)
	}

	_, ok = GetAdminFromContext(context.Background())
	if ok {
		t.Error("expected ok=false for empty context")
	}
}

// TestStatusRecorderDefaultsTo200OnWrite verifies Write() without a prior
// WriteHeader() call records status 200, matching http.ResponseWriter's
// implicit behavior.
func TestStatusRecorderDefaultsTo200OnWrite(t *testing.T) {
	w := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: w}
	n, err := sr.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 5 {
		t.Errorf("Write returned n=%d, want 5", n)
	}
	if sr.status != http.StatusOK {
		t.Errorf("status = %d, want 200", sr.status)
	}
	if sr.bytes != 5 {
		t.Errorf("bytes = %d, want 5", sr.bytes)
	}
}

// TestStatusRecorderCapturesExplicitStatus verifies an explicit WriteHeader
// call is recorded and passed through to the underlying ResponseWriter.
func TestStatusRecorderCapturesExplicitStatus(t *testing.T) {
	w := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: w}
	sr.WriteHeader(http.StatusTeapot)
	_, _ = sr.Write([]byte("abc"))

	if sr.status != http.StatusTeapot {
		t.Errorf("sr.status = %d, want %d", sr.status, http.StatusTeapot)
	}
	if w.Code != http.StatusTeapot {
		t.Errorf("underlying recorder Code = %d, want %d", w.Code, http.StatusTeapot)
	}
	if sr.bytes != 3 {
		t.Errorf("bytes = %d, want 3", sr.bytes)
	}
}

// fakeAccessLogger records the arguments of the last Access() call so tests
// can assert accessLogMiddleware invoked it correctly.
type fakeAccessLogger struct {
	called bool
	ip     string
	method string
	path   string
	status int
	bytes  int
}

func (f *fakeAccessLogger) Access(ip, method, path, proto string, status, bytes int, duration time.Duration) {
	f.called = true
	f.ip = ip
	f.method = method
	f.path = path
	f.status = status
	f.bytes = bytes
}

// TestAccessLogMiddlewareRecordsRequest verifies the middleware invokes the
// AccessLogger with the observed method/path/status/bytes and still lets
// the response through unmodified.
func TestAccessLogMiddlewareRecordsRequest(t *testing.T) {
	logger := &fakeAccessLogger{}
	mw := accessLogMiddleware(logger)
	req := httptest.NewRequest(http.MethodGet, "/server/about", nil)
	req.RemoteAddr = "8.8.8.8:1111"
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("response passthrough broken: code=%d body=%q", w.Code, w.Body.String())
	}
	if !logger.called {
		t.Fatal("expected AccessLogger.Access to be called")
	}
	if logger.method != http.MethodGet || logger.path != "/server/about" {
		t.Errorf("method/path = %q/%q, want GET//server/about", logger.method, logger.path)
	}
	if logger.status != http.StatusOK {
		t.Errorf("status = %d, want 200", logger.status)
	}
	if logger.bytes != 2 {
		t.Errorf("bytes = %d, want 2 (len(\"ok\"))", logger.bytes)
	}
	if logger.ip != "8.8.8.8" {
		t.Errorf("ip = %q, want 8.8.8.8", logger.ip)
	}
}

// TestAccessLogMiddlewareNilLoggerDoesNotPanic verifies a nil AccessLogger
// (the documented "e.g., in tests" case) is safely skipped.
func TestAccessLogMiddlewareNilLoggerDoesNotPanic(t *testing.T) {
	mw := accessLogMiddleware(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestPathSecurityMiddlewareBlocksTraversal verifies both decoded ".." and
// percent-encoded "%2e" traversal attempts are rejected with 400.
func TestPathSecurityMiddlewareBlocksTraversal(t *testing.T) {
	paths := []string{"/../etc/passwd", "/foo/../../bar", "/foo%2e%2e/bar"}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		w := httptest.NewRecorder()
		PathSecurityMiddleware(okHandler()).ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("path %q: status = %d, want 400", p, w.Code)
		}
	}
}

// TestPathSecurityMiddlewareCollapsesDoubleSlashes verifies a clean, valid
// path is normalized (double slashes collapsed) and passed through.
func TestPathSecurityMiddlewareCollapsesDoubleSlashes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/foo//bar", nil)
	var gotPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	PathSecurityMiddleware(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotPath != "/foo/bar" {
		t.Errorf("normalized path = %q, want /foo/bar", gotPath)
	}
}

// TestPathSecurityMiddlewarePreservesTrailingSlash verifies a trailing
// slash on the original path is preserved after path.Clean normalization.
func TestPathSecurityMiddlewarePreservesTrailingSlash(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/foo/bar/", nil)
	var gotPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	PathSecurityMiddleware(h).ServeHTTP(w, req)

	if gotPath != "/foo/bar/" {
		t.Errorf("normalized path = %q, want /foo/bar/ (trailing slash preserved)", gotPath)
	}
}

// TestURLNormalizeMiddlewareRedirectsTrailingSlash verifies a trailing
// slash on a non-file path issues a 301 to the canonical (slash-stripped)
// URL, preserving the query string.
func TestURLNormalizeMiddlewareRedirectsTrailingSlash(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/server/about/?x=1", nil)
	w := httptest.NewRecorder()
	URLNormalizeMiddleware(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/about?x=1" {
		t.Errorf("Location = %q, want /server/about?x=1", loc)
	}
}

// TestURLNormalizeMiddlewareExemptsRoot verifies root "/" is never
// redirected.
func TestURLNormalizeMiddlewareExemptsRoot(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	URLNormalizeMiddleware(okHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("path %q: status = %d, want 200 (root is exempt)", "/", w.Code)
	}
}

// TestURLNormalizeMiddlewareFileLikeTrailingSlashExempt verifies the
// "file-like path" exemption documented on URLNormalizeMiddleware:
// "Requests for paths that end with a file extension are exempt". The
// trailing slash is trimmed before checking the last path segment for a
// dot, so a file-like path with a trailing slash is passed through
// unchanged instead of being redirected.
func TestURLNormalizeMiddlewareFileLikeTrailingSlashExempt(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/static/app.css/", nil)
	w := httptest.NewRecorder()
	URLNormalizeMiddleware(okHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("path %q: status = %d, want 200 (file-like path is exempt from trailing-slash redirect)", "/static/app.css/", w.Code)
	}
}

// TestURLNormalizeMiddlewarePassesThroughCleanPath verifies a path with no
// trailing slash is passed straight through.
func TestURLNormalizeMiddlewarePassesThroughCleanPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/server/about", nil)
	w := httptest.NewRecorder()
	URLNormalizeMiddleware(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
