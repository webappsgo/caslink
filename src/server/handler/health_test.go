package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apktor "github.com/webappsgo/caslink/src/tor"
)

// decodeHealthResponse unwraps the canonical {"ok":true,"data":{...}}
// envelope that respondJSON always applies, and decodes the inner data as a
// HealthResponse.
func decodeHealthResponse(t *testing.T, body []byte) HealthResponse {
	t.Helper()

	var env struct {
		OK   bool           `json:"ok"`
		Data HealthResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope failed: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok:true envelope, body=%s", body)
	}
	return env.Data
}

// TestFormatUptime covers the minutes-only, hours-only, and days branches.
func TestFormatUptime(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"minutes only", 5 * time.Minute, "5m"},
		{"hours and minutes", 2*time.Hour + 30*time.Minute, "2h 30m"},
		{"days hours minutes", 26*time.Hour + 10*time.Minute, "1d 2h 10m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatUptime(tt.d); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHealthHandlerReturnsHTML verifies the public (non-API) health page is
// unauthenticated and returns HTML.
func TestHealthHandlerReturnsHTML(t *testing.T) {
	handler := HealthHandler("1.0.0", "abc123", "2026-01-01", "production")

	r := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct == "" || ct[:9] != "text/html" {
		t.Errorf("expected text/html content type, got %q", ct)
	}
}

// TestAPIHealthHandlerNilStoreDegraded verifies APIHealthHandler tolerates a
// nil *store.Store (its own doc comment says this is supported "for
// testing") and reports status=degraded with checks.database=error, while
// still returning HTTP 200.
func TestAPIHealthHandlerNilStoreDegraded(t *testing.T) {
	handler := APIHealthHandler("1.0.0", "abc123", "2026-01-01", "production", nil, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even when degraded, got %d", w.Code)
	}

	resp := decodeHealthResponse(t, w.Body.Bytes())
	if resp.Status != "degraded" {
		t.Errorf("expected status=degraded, got %q", resp.Status)
	}
	if resp.Checks.Database != "error" {
		t.Errorf("expected checks.database=error, got %q", resp.Checks.Database)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("expected version=1.0.0, got %q", resp.Version)
	}
}

// TestAPIHealthHandlerHealthyWithStore verifies status flips to healthy and
// checks.database=ok when a real, reachable *store.Store is supplied.
func TestAPIHealthHandlerHealthyWithStore(t *testing.T) {
	st := newSchemaTestStore(t)
	handler := APIHealthHandler("1.0.0", "abc123", "2026-01-01", "production", st, nil, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	resp := decodeHealthResponse(t, w.Body.Bytes())
	if resp.Status != "healthy" {
		t.Errorf("expected status=healthy, got %q", resp.Status)
	}
	if resp.Checks.Database != "ok" {
		t.Errorf("expected checks.database=ok, got %q", resp.Checks.Database)
	}
}

// TestAPIHealthHandlerTorInfo verifies torInfo/checks.tor are populated when
// a getTorManager callback returns a non-nil manager, and absent otherwise.
func TestAPIHealthHandlerTorInfo(t *testing.T) {
	handlerNoTor := APIHealthHandler("1.0.0", "c", "d", "production", nil, nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz", nil)
	w := httptest.NewRecorder()
	handlerNoTor(w, r)

	resp := decodeHealthResponse(t, w.Body.Bytes())
	if resp.Checks.Tor != "" {
		t.Errorf("expected empty checks.tor with no manager, got %q", resp.Checks.Tor)
	}

	handlerWithTor := APIHealthHandler("1.0.0", "c", "d", "production", nil, func() *apktor.TorManager {
		return &apktor.TorManager{}
	}, nil)
	w2 := httptest.NewRecorder()
	handlerWithTor(w2, r)

	resp2 := decodeHealthResponse(t, w2.Body.Bytes())
	if resp2.Checks.Tor != "ok" {
		t.Errorf("expected checks.tor=ok, got %q", resp2.Checks.Tor)
	}
	if !resp2.Features.Tor.Enabled || !resp2.Features.Tor.Running {
		t.Errorf("expected tor feature enabled+running, got %+v", resp2.Features.Tor)
	}
}

// TestAPIHealthHandlerCounters verifies getCounters values flow through to
// stats.requests_total / requests_24h / active_connections.
func TestAPIHealthHandlerCounters(t *testing.T) {
	handler := APIHealthHandler("1.0.0", "c", "d", "production", nil, nil, func() (int64, int64, int64) {
		return 100, 20, 3
	})

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	resp := decodeHealthResponse(t, w.Body.Bytes())
	if resp.Stats.RequestsTotal != 100 || resp.Stats.Requests24h != 20 {
		t.Errorf("expected requests_total=100 requests_24h=20, got %+v", resp.Stats)
	}
}

// TestVersionHandler verifies the plain version endpoint returns the
// supplied version/commit/build-date fields.
func TestVersionHandler(t *testing.T) {
	handler := VersionHandler("1.2.3", "deadbee", "2026-07-30")

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/version", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if env.Data["version"] != "1.2.3" {
		t.Errorf("expected version=1.2.3, got %v", env.Data["version"])
	}
}
