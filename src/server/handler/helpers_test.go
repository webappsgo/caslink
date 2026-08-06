package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service"
)

// TestRealClientIPXForwardedFor verifies the first (leftmost) address in a
// comma-separated X-Forwarded-For header wins.
func TestRealClientIPXForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	r.RemoteAddr = "192.0.2.1:5555"

	got := realClientIP(r)
	if got != "203.0.113.5" {
		t.Errorf("expected 203.0.113.5, got %q", got)
	}
}

// TestRealClientIPXRealIP verifies X-Real-IP is used when no
// X-Forwarded-For header is present.
func TestRealClientIPXRealIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Real-IP", "198.51.100.9")
	r.RemoteAddr = "192.0.2.1:5555"

	got := realClientIP(r)
	if got != "198.51.100.9" {
		t.Errorf("expected 198.51.100.9, got %q", got)
	}
}

// TestRealClientIPRemoteAddrFallback verifies RemoteAddr's host portion is
// used (port stripped) when no proxy headers are present.
func TestRealClientIPRemoteAddrFallback(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:5555"

	got := realClientIP(r)
	if got != "192.0.2.1" {
		t.Errorf("expected 192.0.2.1, got %q", got)
	}
}

// TestApiActorIDNoBearer verifies apiActorID returns nil when the request
// has no bearer token attached to its context.
func TestApiActorIDNoBearer(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if id := apiActorID(r); id != nil {
		t.Errorf("expected nil, got %v", *id)
	}
}

// TestApiActorIDWithBearer verifies apiActorID returns the bearer record's
// owner ID when present in context.
func TestApiActorIDWithBearer(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := &service.TokenRecord{OwnerID: 42}
	r = r.WithContext(context.WithValue(r.Context(), BearerContextKey, rec))

	id := apiActorID(r)
	if id == nil || *id != 42 {
		t.Errorf("expected 42, got %v", id)
	}
}

// TestSplitFormList covers empty input, single value, and trimming of
// whitespace/empty segments in a comma-separated list.
func TestSplitFormList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single", "foo", []string{"foo"}},
		{"multiple with spaces", " foo , bar ,baz", []string{"foo", "bar", "baz"}},
		{"empty segments dropped", "foo,,bar", []string{"foo", "bar"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitFormList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestNewPaginationClamping verifies limit clamping (default 250 on <=0,
// cap 250) and page defaulting to 1 on <=0.
func TestNewPaginationClamping(t *testing.T) {
	tests := []struct {
		name        string
		page, limit int
		total       int
		wantPage    int
		wantLimit   int
		wantPages   int
	}{
		{"defaults", 0, 0, 100, 1, 250, 1},
		{"limit too high", 1, 1000, 10, 1, 250, 1},
		{"exact division", 2, 25, 50, 2, 25, 2},
		{"remainder rounds up", 1, 10, 25, 1, 10, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPagination(tt.page, tt.limit, tt.total)
			if p.Page != tt.wantPage || p.Limit != tt.wantLimit || p.Pages != tt.wantPages {
				t.Errorf("got %+v, want page=%d limit=%d pages=%d", p, tt.wantPage, tt.wantLimit, tt.wantPages)
			}
		})
	}
}

// TestRespondJSONEnvelope verifies the canonical success envelope shape.
func TestRespondJSONEnvelope(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusCreated, map[string]string{"foo": "bar"})

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	var body APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !body.OK {
		t.Errorf("expected ok:true")
	}
	if body.Error != "" {
		t.Errorf("expected no error field, got %q", body.Error)
	}
}

// TestRespondErrorEnvelope verifies the canonical error envelope shape and
// that errCodeFromStatus maps the status to the expected code.
func TestRespondErrorEnvelope(t *testing.T) {
	tests := []struct {
		status   int
		wantCode string
	}{
		{http.StatusBadRequest, "BAD_REQUEST"},
		{http.StatusUnauthorized, "UNAUTHORIZED"},
		{http.StatusForbidden, "FORBIDDEN"},
		{http.StatusNotFound, "NOT_FOUND"},
		{http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{http.StatusConflict, "CONFLICT"},
		{http.StatusGone, "GONE"},
		{http.StatusUnprocessableEntity, "VALIDATION_FAILED"},
		{http.StatusTooManyRequests, "RATE_LIMITED"},
		{http.StatusServiceUnavailable, "MAINTENANCE"},
		{http.StatusInternalServerError, "SERVER_ERROR"},
		{http.StatusTeapot, "ERROR"},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		respondError(w, tt.status, "boom")

		var body APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode failed for status %d: %v", tt.status, err)
		}
		if body.OK {
			t.Errorf("status %d: expected ok:false", tt.status)
		}
		if body.Error != tt.wantCode {
			t.Errorf("status %d: expected error=%q, got %q", tt.status, tt.wantCode, body.Error)
		}
		if body.Message != "boom" {
			t.Errorf("status %d: expected message=boom, got %q", tt.status, body.Message)
		}
	}
}

// TestGetUserFromRequestAbsent verifies (nil, false) is returned when no
// user has been attached to the request context.
func TestGetUserFromRequestAbsent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	user, ok := getUserFromRequest(r)
	if ok || user != nil {
		t.Errorf("expected (nil, false), got (%v, %v)", user, ok)
	}
}

// TestGetUserFromRequestPresent verifies the exact *service.User value
// attached via UserContextKey is returned.
func TestGetUserFromRequestPresent(t *testing.T) {
	want := &service.User{ID: 7, Username: "alice"}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), UserContextKey, want))

	got, ok := getUserFromRequest(r)
	if !ok || got != want {
		t.Errorf("expected (%v, true), got (%v, %v)", want, got, ok)
	}
}

// TestGetBearerFromRequest covers both the absent and present cases.
func TestGetBearerFromRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := getBearerFromRequest(r); ok {
		t.Errorf("expected false when no bearer attached")
	}

	want := &service.TokenRecord{OwnerID: 3}
	r = r.WithContext(context.WithValue(r.Context(), BearerContextKey, want))
	got, ok := getBearerFromRequest(r)
	if !ok || got != want {
		t.Errorf("expected (%v, true), got (%v, %v)", want, got, ok)
	}
}

// TestCsrfToken covers both the missing-cookie and present-cookie cases.
func TestCsrfToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if tok := csrfToken(r); tok != "" {
		t.Errorf("expected empty token, got %q", tok)
	}

	r.AddCookie(&http.Cookie{Name: "csrf_token", Value: "abc123"})
	if tok := csrfToken(r); tok != "abc123" {
		t.Errorf("expected abc123, got %q", tok)
	}
}

// TestNewPageDataDefaults verifies the branding fallback defaults are
// applied when config branding fields are empty.
func TestNewPageDataDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Branding.Title = ""
	cfg.Server.Branding.Description = ""

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	data := newPageData(cfg, r, "My Title", nil)

	if data.AppName != "Caslink" {
		t.Errorf("expected AppName fallback Caslink, got %q", data.AppName)
	}
	if data.AppDesc != "Self-hosted URL shortener" {
		t.Errorf("expected AppDesc fallback, got %q", data.AppDesc)
	}
	if data.Title != "My Title" {
		t.Errorf("expected Title=My Title, got %q", data.Title)
	}
	// data.User is `interface{}` populated from a typed *service.User, so a
	// nil pointer boxed into the interface is itself non-nil; assert the
	// underlying pointer is nil instead of comparing the interface to nil.
	if u, ok := data.User.(*service.User); !ok || u != nil {
		t.Errorf("expected nil *service.User, got %v", data.User)
	}
}

// TestNewPageDataWithUser verifies a supplied user is attached to the page
// data unchanged.
func TestNewPageDataWithUser(t *testing.T) {
	cfg := config.DefaultConfig()
	user := &service.User{ID: 1, Username: "bob"}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	data := newPageData(cfg, r, "T", user)

	if data.User != user {
		t.Errorf("expected user to be attached, got %v", data.User)
	}
}
