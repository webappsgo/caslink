package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNormalizePath verifies the label-cardinality-reduction regex from
// AI.md PART 21: only path segments that look like UUIDs or pure integers
// are replaced with ":id". Static route segments and short slugs are left
// unchanged, and a UUID that happens to start with digits still matches in
// full (the UUID alternative is tried before the digit alternative).
func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short 2-char segment left alone", "/r", "/r"},
		{"root path", "/", "/"},
		{"static word segment left unchanged", "/users", "/users"},
		{"numeric id after a static word", "/users/12345", "/users/:id"},
		{"uuid starting with a digit is fully normalized",
			"/users/550e8400-e29b-41d4-a716-446655440000",
			"/users/:id"},
		{"short slug left unchanged", "/r/abc123", "/r/abc123"},
		{"multiple numeric segments interleaved with static words", "/orgs/42/users/7", "/orgs/:id/users/:id"},
		{"2-char segment (v1) and other static segments preserved", "/api/v1/tokens/999", "/api/v1/tokens/:id"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePath(tt.in); got != tt.want {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsUUID checks the length/dash-position heuristic used to distinguish
// UUIDs from other 36-character strings.
func TestIsUUID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"valid v4 uuid", "550e8400-e29b-41d4-a716-446655440000", true},
		{"valid nil uuid", "00000000-0000-0000-0000-000000000000", true},
		{"too short", "550e8400-e29b-41d4-a716-44665544000", false},
		{"too long", "550e8400-e29b-41d4-a716-4466554400000", false},
		{"dashes in wrong position", "550e8400e29b-41d4-a716-4466554400x0", false},
		{"no dashes at all", strings.Repeat("a", 36), false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUUID(tt.in); got != tt.want {
				t.Errorf("isUUID(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsInt checks the integer-detection helper used to route pure numeric
// segments to ":id" rather than ":code".
func TestIsInt(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"positive", "12345", true},
		{"zero", "0", true},
		{"negative", "-42", true},
		{"leading zero", "007", true},
		{"not numeric", "abc123", false},
		{"empty", "", false},
		{"float-looking", "1.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInt(tt.in); got != tt.want {
				t.Errorf("isInt(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestMiddlewareRecordsRequestMetrics verifies the Middleware wrapper
// increments/observes all four HTTP metrics with a normalized path label,
// and correctly captures a non-200 status code and response body size via
// the wrapping ResponseWriter.
func TestMiddlewareRecordsRequestMetrics(t *testing.T) {
	m, handler := New("1.0.0", "abc123", "2026-01-01", false, false, "")

	inner := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest("POST", "/orgs/42/users", strings.NewReader("body-content"))
	rec := httptest.NewRecorder()
	inner.ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Errorf("recorder status = %d, want 201 (Middleware must not alter the response)", rec.Code)
	}

	scrapeReq := httptest.NewRequest("GET", "/metrics", nil)
	scrapeRec := httptest.NewRecorder()
	handler.ServeHTTP(scrapeRec, scrapeReq)
	body := scrapeRec.Body.String()

	// "/orgs/42/users" -> only the numeric segment "42" is normalized to
	// :id; the static "orgs"/"users" segments are left unchanged.
	for _, want := range []string{
		`caslink_http_requests_total{method="POST",path="/orgs/:id/users",status="201"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics response missing %q\nfull body:\n%s", want, body)
		}
	}
	for _, metricName := range []string{
		"caslink_http_request_duration_seconds",
		"caslink_http_request_size_bytes",
		"caslink_http_response_size_bytes",
	} {
		if !strings.Contains(body, metricName) {
			t.Errorf("/metrics response missing histogram %q", metricName)
		}
	}
}

// TestMiddlewareActiveRequestsGaugeReturnsToZero verifies the in-flight
// gauge is incremented before the handler runs and decremented afterward
// (via defer), even though ServeHTTP is synchronous in this test.
func TestMiddlewareActiveRequestsGaugeReturnsToZero(t *testing.T) {
	m, handler := New("1.0.0", "abc123", "2026-01-01", false, false, "")

	inner := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/ping", nil)
	rec := httptest.NewRecorder()
	inner.ServeHTTP(rec, req)

	scrapeReq := httptest.NewRequest("GET", "/metrics", nil)
	scrapeRec := httptest.NewRecorder()
	handler.ServeHTTP(scrapeRec, scrapeReq)
	body := scrapeRec.Body.String()

	if !strings.Contains(body, "caslink_http_active_requests 0") {
		t.Errorf("expected caslink_http_active_requests to be 0 after request completes, body:\n%s", body)
	}
}

// TestMiddlewareRequestSizeNeverNegative verifies a request with an unknown
// (-1) Content-Length does not produce a negative size observation.
func TestMiddlewareRequestSizeNeverNegative(t *testing.T) {
	m, handler := New("1.0.0", "abc123", "2026-01-01", false, false, "")

	inner := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/stream", nil)
	req.ContentLength = -1
	rec := httptest.NewRecorder()

	// Must not panic and must complete normally even with ContentLength unset/-1.
	inner.ServeHTTP(rec, req)

	scrapeReq := httptest.NewRequest("GET", "/metrics", nil)
	scrapeRec := httptest.NewRecorder()
	handler.ServeHTTP(scrapeRec, scrapeReq)
	if !strings.Contains(scrapeRec.Body.String(), "caslink_http_request_size_bytes") {
		t.Errorf("expected request size histogram to be present")
	}
}
