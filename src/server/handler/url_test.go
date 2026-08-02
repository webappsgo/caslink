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
	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/store"
	"github.com/webappsgo/caslink/src/server/tmpl"
)

// newURLTestHandler builds a URLHandler backed by real URLService and
// AnalyticsService against a fresh in-memory schema store, mirroring the
// pattern in domain_test.go/bulk_test.go/qr_test.go.
func newURLTestHandler(t *testing.T) (*URLHandler, *service.URLService, *store.Store) {
	t.Helper()

	st := newSchemaTestStore(t)
	urlService := service.NewURLService(st)
	analyticsService := service.NewAnalyticsService(st)
	cfg := config.DefaultConfig()
	renderer, err := tmpl.New()
	if err != nil {
		t.Fatalf("tmpl.New failed: %v", err)
	}

	return NewURLHandler(urlService, analyticsService, renderer, cfg), urlService, st
}

// withBearer attaches a *service.TokenRecord to the request context the way
// server.BearerAuthMiddleware does, so handler code paths gated on
// getBearerFromRequest can be exercised directly.
func withBearer(r *http.Request, rec *service.TokenRecord) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), BearerContextKey, rec))
}

func decodeAPIResponse(t *testing.T, w *httptest.ResponseRecorder) APIResponse {
	t.Helper()
	var body APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode failed: %v (body=%s)", err, w.Body.String())
	}
	return body
}

// --- CreateURL (JSON API) -------------------------------------------------

func TestCreateURLJSONSuccess(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	body := `{"url":"https://example.com/target"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateURL(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeAPIResponse(t, w)
	if !resp.OK {
		t.Errorf("expected ok:true")
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", resp.Data)
	}
	if code, _ := data["short_code"].(string); code == "" {
		t.Errorf("expected a non-empty short_code, got %v", data["short_code"])
	}
}

func TestCreateURLInvalidBody(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader("{not json"))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateURL(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	resp := decodeAPIResponse(t, w)
	if resp.Error != "BAD_REQUEST" {
		t.Errorf("expected BAD_REQUEST, got %q", resp.Error)
	}
}

// TestCreateURLMalformedTarget verifies an unparseable target URL is
// rejected. The underlying service returns a generic wrapped error (not one
// of the model sentinel errors), so the handler currently falls through to a
// 500 SERVER_ERROR rather than a 400 — this test locks in that actual
// behavior; see report for the accuracy note.
func TestCreateURLMalformedTarget(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	body := `{"url":"not-a-url"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateURL(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for malformed target URL, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateURLCustomAliasAvailable(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	body := `{"url":"https://example.com/one","custom_code":"myalias"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateURL(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeAPIResponse(t, w)
	data := resp.Data.(map[string]interface{})
	if code, _ := data["short_code"].(string); code != "myalias" {
		t.Errorf("expected short_code=myalias, got %v", data["short_code"])
	}
}

func TestCreateURLCustomAliasTaken(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	body := `{"url":"https://example.com/one","custom_code":"dupe"}`
	r1 := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader(body))
	r1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.CreateURL(w1, r1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("expected first create to succeed with 201, got %d: %s", w1.Code, w1.Body.String())
	}

	body2 := `{"url":"https://example.com/two","custom_code":"dupe"}`
	r2 := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader(body2))
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.CreateURL(w2, r2)

	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w2.Code, w2.Body.String())
	}
	resp := decodeAPIResponse(t, w2)
	if resp.Error != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %q", resp.Error)
	}
}

func TestCreateURLCustomAliasReservedWord(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	body := `{"url":"https://example.com/one","custom_code":"admin"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateURL(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateURLCustomAliasTooShort(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	body := `{"url":"https://example.com/one","custom_code":"ab"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateURL(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateURLCustomAliasInvalidChars(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	body := `{"url":"https://example.com/one","custom_code":"bad-code!"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateURL(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateURLFormEncoded verifies the form-urlencoded (non-JS) submission
// path is parsed correctly.
func TestCreateURLFormEncoded(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader("long_url=https%3A%2F%2Fexample.com%2Fform"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.CreateURL(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateURLOwnedByUserToken verifies a user Bearer token creates a link
// that is then owned by that user (visible via ListURLs/ownership checks).
func TestCreateURLOwnedByUserToken(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	body := `{"url":"https://example.com/owned"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/urls", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 42})
	w := httptest.NewRecorder()
	h.CreateURL(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeAPIResponse(t, w)
	data := resp.Data.(map[string]interface{})
	code := data["short_code"].(string)

	stored, err := urlService.GetURLByCodeAny(context.Background(), code)
	if err != nil {
		t.Fatalf("GetURLByCodeAny failed: %v", err)
	}
	if stored.UserID == nil || *stored.UserID != 42 {
		t.Errorf("expected UserID=42, got %v", stored.UserID)
	}
}

// --- RedirectURL -----------------------------------------------------------

func TestRedirectURLFound(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURL(context.Background(), &model.CreateURLRequest{LongURL: "https://example.com/dest"})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/"+u.ShortCode, nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.RedirectURL(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com/dest" {
		t.Errorf("expected redirect to https://example.com/dest, got %q", loc)
	}
}

func TestRedirectURLNotFound(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/doesnotexist", nil)
	r = withChiURLParam(r, "code", "doesnotexist")
	w := httptest.NewRecorder()
	h.RedirectURL(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRedirectURLEmptyCode(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = withChiURLParam(r, "code", "")
	w := httptest.NewRecorder()
	h.RedirectURL(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty code, got %d", w.Code)
	}
}

// TestRedirectURLExpired verifies an expired link 404s rather than
// redirecting (link expiry must not leak the destination).
func TestRedirectURLExpired(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURL(context.Background(), &model.CreateURLRequest{
		LongURL: "https://example.com/expired",
	})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}
	// Force expiry via UpdateURL rather than reaching into the DB directly,
	// exercising the real code path. UpdateURL itself must report success
	// (the write committed) even though the new expires_at is in the past —
	// it re-fetches via the raw, non-expiry-checking path. The expiry only
	// takes effect on the next read through RedirectURL/GetURLByCode.
	expired := time.Now().Add(-1 * time.Hour)
	if _, err := urlService.UpdateURL(context.Background(), u.ShortCode, &model.UpdateURLRequest{
		ExpiresAt: &expired,
	}); err != nil {
		t.Fatalf("UpdateURL (force expiry): got %v, want nil (write succeeded)", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/"+u.ShortCode, nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.RedirectURL(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for expired link, got %d", w.Code)
	}
}

// TestRedirectURLDeviceTargeting verifies a mobile UA is sent to the
// per-link MobileURL override rather than the default LongURL.
func TestRedirectURLDeviceTargeting(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURL(context.Background(), &model.CreateURLRequest{
		LongURL:   "https://example.com/desktop",
		MobileURL: "https://example.com/mobile",
	})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/"+u.ShortCode, nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.RedirectURL(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com/mobile" {
		t.Errorf("expected mobile override, got %q", loc)
	}
}

// TestRedirectURLUTMPassthrough verifies static UTM params configured on the
// link are appended to the redirect destination.
func TestRedirectURLUTMPassthrough(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURL(context.Background(), &model.CreateURLRequest{
		LongURL:   "https://example.com/utm",
		UTMSource: "newsletter",
	})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/"+u.ShortCode, nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.RedirectURL(w, r)

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "utm_source=newsletter") {
		t.Errorf("expected utm_source=newsletter in redirect location, got %q", loc)
	}
}

// --- ListURLs ----------------------------------------------------------

func TestListURLsUnauthenticated(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls", nil)
	w := httptest.NewRecorder()
	h.ListURLs(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestListURLsRejectsAdminToken verifies an admin-scoped Bearer token is
// rejected (PART 14 scopes /users/* to the caller's own resources).
func TestListURLsRejectsAdminToken(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls", nil)
	r = withBearer(r, &service.TokenRecord{OwnerType: "admin", OwnerID: 1})
	w := httptest.NewRecorder()
	h.ListURLs(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for admin token, got %d", w.Code)
	}
}

// TestListURLsEmpty verifies a fresh user sees an empty, well-formed page.
func TestListURLsEmpty(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls", nil)
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 1})
	w := httptest.NewRecorder()
	h.ListURLs(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeAPIResponse(t, w)
	data := resp.Data.(map[string]interface{})
	urls, _ := data["urls"].([]interface{})
	if len(urls) != 0 {
		t.Errorf("expected empty urls list, got %v", urls)
	}
	if total, _ := data["total"].(float64); total != 0 {
		t.Errorf("expected total=0, got %v", data["total"])
	}
}

// TestListURLsOwnershipScoping verifies user A cannot see user B's links via
// ListURLs — the core authz property of the URL shortener's dashboard.
func TestListURLsOwnershipScoping(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	if _, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{LongURL: "https://example.com/a"}); err != nil {
		t.Fatalf("CreateURLForUser(1) failed: %v", err)
	}
	if _, err := urlService.CreateURLForUser(context.Background(), 2, &model.CreateURLRequest{LongURL: "https://example.com/b"}); err != nil {
		t.Fatalf("CreateURLForUser(2) failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls", nil)
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 1})
	w := httptest.NewRecorder()
	h.ListURLs(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeAPIResponse(t, w)
	data := resp.Data.(map[string]interface{})
	urls, _ := data["urls"].([]interface{})
	if len(urls) != 1 {
		t.Fatalf("expected exactly 1 URL owned by user 1, got %d: %v", len(urls), urls)
	}
	first := urls[0].(map[string]interface{})
	if first["long_url"] != "https://example.com/a" {
		t.Errorf("expected user 1's own link, got %v", first["long_url"])
	}
}

// TestListURLsPagination verifies page/limit query params are honored.
func TestListURLsPagination(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	for i := 0; i < 3; i++ {
		if _, err := urlService.CreateURLForUser(context.Background(), 7, &model.CreateURLRequest{LongURL: "https://example.com/page/" + string(rune('a'+i))}); err != nil {
			t.Fatalf("CreateURLForUser failed: %v", err)
		}
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls?page=1&limit=2", nil)
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 7})
	w := httptest.NewRecorder()
	h.ListURLs(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeAPIResponse(t, w)
	data := resp.Data.(map[string]interface{})
	urls, _ := data["urls"].([]interface{})
	if len(urls) != 2 {
		t.Fatalf("expected 2 urls on page 1 with limit=2, got %d", len(urls))
	}
	if total, _ := data["total"].(float64); total != 3 {
		t.Errorf("expected total=3, got %v", data["total"])
	}
}

// TestListURLsOrgScoping verifies an org token lists the org's links, not a
// user's.
func TestListURLsOrgScoping(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	if _, err := urlService.CreateURLForOrg(context.Background(), 10, &model.CreateURLRequest{LongURL: "https://example.com/org"}); err != nil {
		t.Fatalf("CreateURLForOrg failed: %v", err)
	}
	if _, err := urlService.CreateURLForUser(context.Background(), 10, &model.CreateURLRequest{LongURL: "https://example.com/user"}); err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls", nil)
	r = withBearer(r, &service.TokenRecord{OwnerType: "org", OwnerID: 10})
	w := httptest.NewRecorder()
	h.ListURLs(w, r)

	resp := decodeAPIResponse(t, w)
	data := resp.Data.(map[string]interface{})
	urls, _ := data["urls"].([]interface{})
	if len(urls) != 1 {
		t.Fatalf("expected exactly 1 org-owned URL, got %d: %v", len(urls), urls)
	}
	first := urls[0].(map[string]interface{})
	if first["long_url"] != "https://example.com/org" {
		t.Errorf("expected the org's link, got %v", first["long_url"])
	}
}

// --- GetURL --------------------------------------------------------------

func TestGetURLEmptyCode(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls/", nil)
	r = withChiURLParam(r, "code", "")
	w := httptest.NewRecorder()
	h.GetURL(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetURLNotFound(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls/nope", nil)
	r = withChiURLParam(r, "code", "nope")
	w := httptest.NewRecorder()
	h.GetURL(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetURLPublicVisibleToAnyone(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURL(context.Background(), &model.CreateURLRequest{LongURL: "https://example.com/public"})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls/"+u.ShortCode, nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.GetURL(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGetURLPrivateHiddenFromStranger verifies a "private" link 404s for an
// unauthenticated caller and for a Bearer caller who isn't the owner.
func TestGetURLPrivateHiddenFromStranger(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{
		LongURL:    "https://example.com/private",
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	// Unauthenticated.
	r1 := httptest.NewRequest(http.MethodGet, "/api/v1/urls/"+u.ShortCode, nil)
	r1 = withChiURLParam(r1, "code", u.ShortCode)
	w1 := httptest.NewRecorder()
	h.GetURL(w1, r1)
	if w1.Code != http.StatusNotFound {
		t.Errorf("expected 404 for anonymous caller, got %d", w1.Code)
	}

	// Different user's Bearer token.
	r2 := httptest.NewRequest(http.MethodGet, "/api/v1/urls/"+u.ShortCode, nil)
	r2 = withChiURLParam(r2, "code", u.ShortCode)
	r2 = withBearer(r2, &service.TokenRecord{OwnerType: "user", OwnerID: 2})
	w2 := httptest.NewRecorder()
	h.GetURL(w2, r2)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-owner caller, got %d", w2.Code)
	}

	// Owning user's Bearer token.
	r3 := httptest.NewRequest(http.MethodGet, "/api/v1/urls/"+u.ShortCode, nil)
	r3 = withChiURLParam(r3, "code", u.ShortCode)
	r3 = withBearer(r3, &service.TokenRecord{OwnerType: "user", OwnerID: 1})
	w3 := httptest.NewRecorder()
	h.GetURL(w3, r3)
	if w3.Code != http.StatusOK {
		t.Errorf("expected 200 for owner, got %d: %s", w3.Code, w3.Body.String())
	}
}

// --- UpdateURL -----------------------------------------------------------

func TestUpdateURLNoBearerToken(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURL(context.Background(), &model.CreateURLRequest{LongURL: "https://example.com/x"})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/urls/"+u.ShortCode, strings.NewReader(`{"title":"new"}`))
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.UpdateURL(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestUpdateURLEmptyCode(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/urls/", strings.NewReader(`{}`))
	r = withChiURLParam(r, "code", "")
	w := httptest.NewRecorder()
	h.UpdateURL(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpdateURLNotFound(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/urls/nope", strings.NewReader(`{"title":"x"}`))
	r = withChiURLParam(r, "code", "nope")
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 1})
	w := httptest.NewRecorder()
	h.UpdateURL(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateURLNotOwner(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{LongURL: "https://example.com/mine"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/urls/"+u.ShortCode, strings.NewReader(`{"title":"hijacked"}`))
	r = withChiURLParam(r, "code", u.ShortCode)
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 2})
	w := httptest.NewRecorder()
	h.UpdateURL(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (ownership hidden as not-found), got %d: %s", w.Code, w.Body.String())
	}

	// Confirm the underlying record was NOT modified by the rejected attempt.
	stored, err := urlService.GetURLByCodeAny(context.Background(), u.ShortCode)
	if err != nil {
		t.Fatalf("GetURLByCodeAny failed: %v", err)
	}
	if stored.Title != nil {
		t.Errorf("expected title to remain unset after rejected update, got %v", *stored.Title)
	}
}

func TestUpdateURLSuccessByOwner(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{LongURL: "https://example.com/mine"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/urls/"+u.ShortCode, strings.NewReader(`{"title":"Updated Title"}`))
	r = withChiURLParam(r, "code", u.ShortCode)
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 1})
	w := httptest.NewRecorder()
	h.UpdateURL(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeAPIResponse(t, w)
	data := resp.Data.(map[string]interface{})
	if data["title"] != "Updated Title" {
		t.Errorf("expected title=Updated Title, got %v", data["title"])
	}
}

func TestUpdateURLByAdminTokenAlwaysAllowed(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{LongURL: "https://example.com/mine"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/urls/"+u.ShortCode, strings.NewReader(`{"title":"Admin Edit"}`))
	r = withChiURLParam(r, "code", u.ShortCode)
	r = withBearer(r, &service.TokenRecord{OwnerType: "admin", OwnerID: 999})
	w := httptest.NewRecorder()
	h.UpdateURL(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin token, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateURLOrgOwnershipScoping(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURLForOrg(context.Background(), 5, &model.CreateURLRequest{LongURL: "https://example.com/org-link"})
	if err != nil {
		t.Fatalf("CreateURLForOrg failed: %v", err)
	}

	// Wrong org.
	r1 := httptest.NewRequest(http.MethodPatch, "/api/v1/urls/"+u.ShortCode, strings.NewReader(`{"title":"x"}`))
	r1 = withChiURLParam(r1, "code", u.ShortCode)
	r1 = withBearer(r1, &service.TokenRecord{OwnerType: "org", OwnerID: 6})
	w1 := httptest.NewRecorder()
	h.UpdateURL(w1, r1)
	if w1.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong org, got %d", w1.Code)
	}

	// Correct org.
	r2 := httptest.NewRequest(http.MethodPatch, "/api/v1/urls/"+u.ShortCode, strings.NewReader(`{"title":"x"}`))
	r2 = withChiURLParam(r2, "code", u.ShortCode)
	r2 = withBearer(r2, &service.TokenRecord{OwnerType: "org", OwnerID: 5})
	w2 := httptest.NewRecorder()
	h.UpdateURL(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for correct org, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestUpdateURLInvalidBody(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{LongURL: "https://example.com/mine"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/urls/"+u.ShortCode, strings.NewReader("{not json"))
	r = withChiURLParam(r, "code", u.ShortCode)
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 1})
	w := httptest.NewRecorder()
	h.UpdateURL(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- DeleteURL -------------------------------------------------------------

func TestDeleteURLNoBearerToken(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURL(context.Background(), &model.CreateURLRequest{LongURL: "https://example.com/x"})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/urls/"+u.ShortCode, nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.DeleteURL(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestDeleteURLEmptyCode(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/urls/", nil)
	r = withChiURLParam(r, "code", "")
	w := httptest.NewRecorder()
	h.DeleteURL(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteURLNotFound(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/urls/nope", nil)
	r = withChiURLParam(r, "code", "nope")
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 1})
	w := httptest.NewRecorder()
	h.DeleteURL(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDeleteURLNotOwner verifies user B cannot delete user A's link, and
// that the link genuinely still exists afterward.
func TestDeleteURLNotOwner(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{LongURL: "https://example.com/mine"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/urls/"+u.ShortCode, nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 2})
	w := httptest.NewRecorder()
	h.DeleteURL(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (ownership hidden as not-found), got %d: %s", w.Code, w.Body.String())
	}

	if _, err := urlService.GetURLByCodeAny(context.Background(), u.ShortCode); err != nil {
		t.Errorf("expected link to still exist after rejected delete, got err: %v", err)
	}
}

func TestDeleteURLSuccessByOwner(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{LongURL: "https://example.com/mine"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/urls/"+u.ShortCode, nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 1})
	w := httptest.NewRecorder()
	h.DeleteURL(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	if _, err := urlService.GetURLByCodeAny(context.Background(), u.ShortCode); err != model.ErrURLNotFound {
		t.Errorf("expected ErrURLNotFound after delete, got %v", err)
	}
}

// TestDeleteURLIdempotency verifies deleting the same (already-deleted) code
// twice yields 404 the second time rather than a 204/panic.
func TestDeleteURLIdempotency(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{LongURL: "https://example.com/mine"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	r1 := httptest.NewRequest(http.MethodDelete, "/api/v1/urls/"+u.ShortCode, nil)
	r1 = withChiURLParam(r1, "code", u.ShortCode)
	r1 = withBearer(r1, &service.TokenRecord{OwnerType: "user", OwnerID: 1})
	w1 := httptest.NewRecorder()
	h.DeleteURL(w1, r1)
	if w1.Code != http.StatusNoContent {
		t.Fatalf("expected first delete to succeed with 204, got %d", w1.Code)
	}

	// Second delete: checkURLOwnership now fails to find the URL at all (it
	// was deleted), so this must 404, not re-succeed or 500.
	r2 := httptest.NewRequest(http.MethodDelete, "/api/v1/urls/"+u.ShortCode, nil)
	r2 = withChiURLParam(r2, "code", u.ShortCode)
	r2 = withBearer(r2, &service.TokenRecord{OwnerType: "user", OwnerID: 1})
	w2 := httptest.NewRecorder()
	h.DeleteURL(w2, r2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("expected second delete to 404, got %d: %s", w2.Code, w2.Body.String())
	}
}

// --- Stats -----------------------------------------------------------------

func TestStatsEmptyCode(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls//stats", nil)
	r = withChiURLParam(r, "code", "")
	w := httptest.NewRecorder()
	h.Stats(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestStatsNotFound(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls/nope/stats", nil)
	r = withChiURLParam(r, "code", "nope")
	w := httptest.NewRecorder()
	h.Stats(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestStatsPublicVisibleToAnyone(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURL(context.Background(), &model.CreateURLRequest{LongURL: "https://example.com/stats"})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls/"+u.ShortCode+"/stats", nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.Stats(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestStatsPrivateHiddenFromNonOwner verifies stats on a private link 404
// for a caller who is not the owner (mirrors GetURL's visibility rule).
func TestStatsPrivateHiddenFromNonOwner(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{
		LongURL:    "https://example.com/private-stats",
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/urls/"+u.ShortCode+"/stats", nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	r = withBearer(r, &service.TokenRecord{OwnerType: "user", OwnerID: 2})
	w := httptest.NewRecorder()
	h.Stats(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-owner, got %d", w.Code)
	}
}

// --- WebCreateURL (HTML form / PRG) ----------------------------------------

func TestWebCreateURLMissingURL(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/urls", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.WebCreateURL(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWebCreateURLAnonymousSuccess(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/urls", strings.NewReader("long_url=https%3A%2F%2Fexample.com%2Fweb"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.WebCreateURL(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.HasPrefix(loc, "/users/dashboard?created=") {
		t.Errorf("expected PRG redirect to dashboard, got %q", loc)
	}
}

// TestWebCreateURLOwnedBySessionUser verifies a signed-in user's created
// link is attributed to them (so it appears on their dashboard).
func TestWebCreateURLOwnedBySessionUser(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	user := &service.User{ID: 3, Username: "carol"}
	r := httptest.NewRequest(http.MethodPost, "/urls", strings.NewReader("long_url=https%3A%2F%2Fexample.com%2Fmine"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.WebCreateURL(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}

	urls, err := urlService.ListByUserPage(context.Background(), 3, 10, 0)
	if err != nil {
		t.Fatalf("ListByUserPage failed: %v", err)
	}
	if len(urls) != 1 {
		t.Fatalf("expected exactly 1 URL owned by user 3, got %d", len(urls))
	}
}

func TestWebCreateURLInvalidFormData(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	// A body that fails ParseForm: invalid percent-encoding.
	r := httptest.NewRequest(http.MethodPost, "/urls", strings.NewReader("long_url=%zz"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.WebCreateURL(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- WebURLManage ------------------------------------------------------

func TestWebURLManageRequiresLogin(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	u, err := urlService.CreateURL(context.Background(), &model.CreateURLRequest{LongURL: "https://example.com/x"})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/users/urls/"+u.ShortCode, nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.WebURLManage(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 login redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/auth/login" {
		t.Errorf("expected redirect to login, got %q", loc)
	}
}

func TestWebURLManageEmptyCode(t *testing.T) {
	h, _, _ := newURLTestHandler(t)

	user := &service.User{ID: 1, Username: "alice"}
	r := httptest.NewRequest(http.MethodGet, "/users/urls/", nil)
	r = withChiURLParam(r, "code", "")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.WebURLManage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestWebURLManagePostDeleteNotOwner verifies a POST delete from a
// non-owning session user 404s and leaves the link intact (mirrors the
// Bearer-token DeleteURL ownership test, but on the session-auth path).
func TestWebURLManagePostDeleteNotOwner(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	owned, err := urlService.CreateURLForUser(context.Background(), 1, &model.CreateURLRequest{LongURL: "https://example.com/owned"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	stranger := &service.User{ID: 2, Username: "mallory"}
	form := strings.NewReader("action=delete")
	r := httptest.NewRequest(http.MethodPost, "/users/urls/"+owned.ShortCode, form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "code", owned.ShortCode)
	r = withUser(r, stranger)
	w := httptest.NewRecorder()
	h.WebURLManage(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-owner delete attempt, got %d", w.Code)
	}
	if _, err := urlService.GetURLByCodeAny(context.Background(), owned.ShortCode); err != nil {
		t.Errorf("expected link to survive rejected delete, got err: %v", err)
	}
}

// TestWebURLManagePostDeleteByOwner verifies the owner can delete their own
// link via the session-authenticated form path and is redirected to the
// dashboard.
func TestWebURLManagePostDeleteByOwner(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	user := &service.User{ID: 1, Username: "alice"}
	owned, err := urlService.CreateURLForUser(context.Background(), user.ID, &model.CreateURLRequest{LongURL: "https://example.com/owned"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	form := strings.NewReader("action=delete")
	r := httptest.NewRequest(http.MethodPost, "/users/urls/"+owned.ShortCode, form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "code", owned.ShortCode)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.WebURLManage(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect to dashboard, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := urlService.GetURLByCodeAny(context.Background(), owned.ShortCode); err != model.ErrURLNotFound {
		t.Errorf("expected link to be gone after owner delete, got %v", err)
	}
}

// TestWebURLManagePostUpdateByOwner verifies the owner can update fields via
// the session-authenticated form path and the change is persisted.
func TestWebURLManagePostUpdateByOwner(t *testing.T) {
	h, urlService, _ := newURLTestHandler(t)

	user := &service.User{ID: 1, Username: "alice"}
	owned, err := urlService.CreateURLForUser(context.Background(), user.ID, &model.CreateURLRequest{LongURL: "https://example.com/owned"})
	if err != nil {
		t.Fatalf("CreateURLForUser failed: %v", err)
	}

	form := strings.NewReader("action=update&title=" + "New+Title")
	r := httptest.NewRequest(http.MethodPost, "/users/urls/"+owned.ShortCode, form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = withChiURLParam(r, "code", owned.ShortCode)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.WebURLManage(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (in-place re-render), got %d: %s", w.Code, w.Body.String())
	}

	stored, err := urlService.GetURLByCodeAny(context.Background(), owned.ShortCode)
	if err != nil {
		t.Fatalf("GetURLByCodeAny failed: %v", err)
	}
	if stored.Title == nil || *stored.Title != "New Title" {
		t.Errorf("expected title to be updated to %q, got %v", "New Title", stored.Title)
	}
}
