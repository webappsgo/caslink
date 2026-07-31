package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewRegistersWithoutPanicAndServesMetrics verifies that New() wires a
// single registry shared by both the returned Metrics struct and the HTTP
// handler — i.e. that a metric recorded on m is actually visible on the
// /metrics response body, guarding against the registry-duplication bug
// where two separate appmetrics.New() calls would leave /metrics empty.
func TestNewRegistersWithoutPanicAndServesMetrics(t *testing.T) {
	m, handler := New("1.0.0", "abc123", "2026-01-01", true, true, t.TempDir())

	m.HTTPRequestsTotal.WithLabelValues("GET", "/:id", "200").Inc()
	m.CacheHitsTotal.WithLabelValues("qr_codes").Inc()
	m.SchedulerTasksTotal.WithLabelValues("token_cleanup", "success").Inc()
	m.RatelimitRequestsTotal.WithLabelValues("login", "allowed").Inc()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{
		"caslink_http_requests_total",
		"caslink_cache_hits_total",
		"caslink_scheduler_tasks_total",
		"caslink_ratelimit_requests_total",
		"caslink_app_info",
		"caslink_go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics response missing %q", want)
		}
	}
}

func TestNewWithoutSystemOrRuntime(t *testing.T) {
	_, handler := New("1.0.0", "abc123", "2026-01-01", false, false, "")

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "caslink_go_goroutines") {
		t.Errorf("/metrics response should not contain go_goroutines when includeRuntime is false")
	}
	if strings.Contains(body, "caslink_system_disk_usage_percent") {
		t.Errorf("/metrics response should not contain disk metrics when includeSystem is false")
	}
}
