// Package metrics registers and exposes Prometheus-compatible metrics for the
// caslink application per AI.md PART 21. All metric names are prefixed with
// "caslink_" and follow Prometheus naming conventions (snake_case, unit suffix).
package metrics

import (
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// durationBuckets are the spec-canonical histogram buckets for request latency.
var durationBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// sizeBuckets are the spec-canonical histogram buckets for request/response bodies.
var sizeBuckets = []float64{100, 1_000, 10_000, 100_000, 1_000_000, 10_000_000}

// dbDurationBuckets are the spec-canonical buckets for DB query latency.
var dbDurationBuckets = []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1}

// Metrics holds all registered Prometheus metrics for caslink.
type Metrics struct {
	registry *prometheus.Registry

	// ---- Application info ----
	AppInfo           *prometheus.GaugeVec
	AppUptimeSeconds  prometheus.GaugeFunc
	AppStartTimestamp prometheus.Gauge

	// ---- HTTP ----
	HTTPRequestsTotal     *prometheus.CounterVec
	HTTPRequestDuration   *prometheus.HistogramVec
	HTTPRequestSizeBytes  *prometheus.HistogramVec
	HTTPResponseSizeBytes *prometheus.HistogramVec
	HTTPActiveRequests    prometheus.Gauge

	// ---- Database ----
	DBQueriesTotal     *prometheus.CounterVec
	DBQueryDuration    *prometheus.HistogramVec
	DBConnectionsOpen  prometheus.Gauge
	DBConnectionsInUse prometheus.Gauge
	DBErrorsTotal      *prometheus.CounterVec

	// ---- Auth ----
	AuthAttemptsTotal  *prometheus.CounterVec
	AuthSessionsActive prometheus.Gauge

	// ---- Scheduler ----
	SchedulerTasksTotal          *prometheus.CounterVec
	SchedulerTaskDurationSeconds *prometheus.HistogramVec
	SchedulerTasksRunning        *prometheus.GaugeVec
	SchedulerLastRunTimestamp    *prometheus.GaugeVec

	// ---- Cache ----
	CacheHitsTotal      *prometheus.CounterVec
	CacheMissesTotal    *prometheus.CounterVec
	CacheEvictionsTotal *prometheus.CounterVec

	// ---- Rate limiting ----
	RatelimitRequestsTotal *prometheus.CounterVec
	RatelimitBlockedTotal  *prometheus.CounterVec
}

// schedulerDurationBuckets are the spec-canonical buckets for scheduler task
// run duration (AI.md PART 21), covering sub-second tasks through the
// longest expected maintenance jobs (10 minutes).
var schedulerDurationBuckets = []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600}

var startTime = time.Now()

// New creates and registers all caslink metrics. It returns the Metrics struct
// and the HTTP handler for the /metrics endpoint.
// When includeRuntime is true the default Go runtime collectors are also registered,
// plus the spec-mandated custom-named go_* gauges/counters.
// When includeSystem is true, system disk usage metrics are registered for dataDir.
func New(version, commit, buildDate string, includeRuntime, includeSystem bool, dataDir string) (*Metrics, http.Handler) {
	reg := prometheus.NewRegistry()

	m := &Metrics{registry: reg}

	// ---- App info ----
	m.AppInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "caslink_app_info",
		Help: "Always 1. Labels carry caslink build information.",
	}, []string{"version", "commit", "build_date", "go_version"})
	m.AppInfo.WithLabelValues(version, commit, buildDate, runtime.Version()).Set(1)

	m.AppStartTimestamp = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "caslink_app_start_timestamp",
		Help: "Unix timestamp when caslink started.",
	})
	m.AppStartTimestamp.Set(float64(startTime.Unix()))

	m.AppUptimeSeconds = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "caslink_app_uptime_seconds",
		Help: "Seconds since caslink started.",
	}, func() float64 { return time.Since(startTime).Seconds() })

	// ---- HTTP ----
	m.HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caslink_http_requests_total",
		Help: "Total HTTP requests processed, partitioned by method, path, and status.",
	}, []string{"method", "path", "status"})

	m.HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "caslink_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: durationBuckets,
	}, []string{"method", "path"})

	m.HTTPRequestSizeBytes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "caslink_http_request_size_bytes",
		Help:    "HTTP request body size in bytes.",
		Buckets: sizeBuckets,
	}, []string{"method", "path"})

	m.HTTPResponseSizeBytes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "caslink_http_response_size_bytes",
		Help:    "HTTP response body size in bytes.",
		Buckets: sizeBuckets,
	}, []string{"method", "path"})

	m.HTTPActiveRequests = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "caslink_http_active_requests",
		Help: "Number of HTTP requests currently being processed.",
	})

	// ---- Database ----
	m.DBQueriesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caslink_db_queries_total",
		Help: "Total database queries, partitioned by operation and table.",
	}, []string{"operation", "table"})

	m.DBQueryDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "caslink_db_query_duration_seconds",
		Help:    "Database query latency in seconds.",
		Buckets: dbDurationBuckets,
	}, []string{"operation", "table"})

	m.DBConnectionsOpen = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "caslink_db_connections_open",
		Help: "Number of open database connections in the pool.",
	})

	m.DBConnectionsInUse = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "caslink_db_connections_in_use",
		Help: "Number of database connections currently in use.",
	})

	m.DBErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caslink_db_errors_total",
		Help: "Total database errors, partitioned by operation and error type.",
	}, []string{"operation", "error_type"})

	// ---- Auth ----
	m.AuthAttemptsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caslink_auth_attempts_total",
		Help: "Total authentication attempts, partitioned by method and status.",
	}, []string{"method", "status"})

	m.AuthSessionsActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "caslink_auth_sessions_active",
		Help: "Number of active user sessions.",
	})

	// ---- Scheduler ----
	m.SchedulerTasksTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caslink_scheduler_tasks_total",
		Help: "Total scheduled task executions, partitioned by task name and status.",
	}, []string{"task", "status"})

	m.SchedulerTaskDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "caslink_scheduler_task_duration_seconds",
		Help:    "Scheduled task run duration in seconds, partitioned by task name.",
		Buckets: schedulerDurationBuckets,
	}, []string{"task"})

	m.SchedulerTasksRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "caslink_scheduler_tasks_running",
		Help: "Whether a scheduled task is currently running (1) or not (0), by task name.",
	}, []string{"task"})

	m.SchedulerLastRunTimestamp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "caslink_scheduler_last_run_timestamp",
		Help: "Unix timestamp of the last run of a scheduled task, by task name.",
	}, []string{"task"})

	// ---- Cache ----
	m.CacheHitsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caslink_cache_hits_total",
		Help: "Total cache hits, partitioned by cache name.",
	}, []string{"cache"})

	m.CacheMissesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caslink_cache_misses_total",
		Help: "Total cache misses, partitioned by cache name.",
	}, []string{"cache"})

	m.CacheEvictionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caslink_cache_evictions_total",
		Help: "Total cache evictions, partitioned by cache name.",
	}, []string{"cache"})

	// ---- Rate limiting ----
	// NOTE: "limit" is a rate-limit rule name (e.g. "login", "register"), never
	// a raw client IP — see AI.md PART 21 cardinality warning.
	m.RatelimitRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caslink_ratelimit_requests_total",
		Help: "Total rate-limited-path requests, partitioned by limit rule and outcome status.",
	}, []string{"limit", "status"})

	m.RatelimitBlockedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caslink_ratelimit_blocked_total",
		Help: "Total requests blocked by rate limiting, partitioned by limit rule.",
	}, []string{"limit"})

	// Register all metrics.
	reg.MustRegister(
		m.AppInfo,
		m.AppStartTimestamp,
		m.AppUptimeSeconds,
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.HTTPRequestSizeBytes,
		m.HTTPResponseSizeBytes,
		m.HTTPActiveRequests,
		m.DBQueriesTotal,
		m.DBQueryDuration,
		m.DBConnectionsOpen,
		m.DBConnectionsInUse,
		m.DBErrorsTotal,
		m.AuthAttemptsTotal,
		m.AuthSessionsActive,
		m.SchedulerTasksTotal,
		m.SchedulerTaskDurationSeconds,
		m.SchedulerTasksRunning,
		m.SchedulerLastRunTimestamp,
		m.CacheHitsTotal,
		m.CacheMissesTotal,
		m.CacheEvictionsTotal,
		m.RatelimitRequestsTotal,
		m.RatelimitBlockedTotal,
	)

	if includeRuntime {
		reg.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			newRuntimeMetricsCollector(),
		)
	}

	if includeSystem && dataDir != "" {
		reg.MustRegister(newDiskCollector(dataDir))
	}

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		Registry: reg,
	})
	return m, handler
}

// idPattern matches path segments that look like numeric IDs or UUIDs so
// they can be replaced with a low-cardinality placeholder before being used
// as Prometheus label values. The UUID alternative is tried before the
// digit alternative so a UUID that happens to start with digits still
// matches in full instead of only its leading numeric prefix.
var idPattern = regexp.MustCompile(
	`/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9]+)`,
)

// normalizePath replaces high-cardinality path segments with ":id" so that
// Prometheus label cardinality stays bounded, per AI.md PART 21. Only
// segments that look like UUIDs or pure integers are replaced; static route
// segments and short slugs are left unchanged.
func normalizePath(p string) string {
	return idPattern.ReplaceAllString(p, "/:id")
}

func isUUID(s string) bool {
	return len(s) == 36 && s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-'
}

func isInt(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

// metricsResponseWriter wraps http.ResponseWriter to capture status code and bytes.
type metricsResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *metricsResponseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// Middleware returns an http.Handler middleware that records HTTP metrics.
// It should be applied after routing (chi's route pattern is used for the path
// label when available) so that path parameters are already resolved.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		m.HTTPActiveRequests.Inc()
		defer m.HTTPActiveRequests.Dec()

		rw := &metricsResponseWriter{ResponseWriter: w, status: http.StatusOK}

		// Record request body size.
		reqSize := float64(r.ContentLength)
		if reqSize < 0 {
			reqSize = 0
		}

		next.ServeHTTP(rw, r)

		// Prefer chi's registered route pattern (e.g. "/{code}") over the raw
		// URL path so high-cardinality path segments — short-slugs, IDs, UUIDs —
		// collapse to a single bounded label value (AI.md PART 21: never use an
		// unbounded value as a metric label). Fall back to numeric-ID
		// normalization when no pattern is available (unmatched routes/404s).
		labelPath := normalizePath(r.URL.Path)
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			if pattern := rctx.RoutePattern(); pattern != "" {
				labelPath = pattern
			}
		}
		duration := time.Since(start).Seconds()
		statusStr := strconv.Itoa(rw.status)

		m.HTTPRequestsTotal.WithLabelValues(r.Method, labelPath, statusStr).Inc()
		m.HTTPRequestDuration.WithLabelValues(r.Method, labelPath).Observe(duration)
		m.HTTPRequestSizeBytes.WithLabelValues(r.Method, labelPath).Observe(reqSize)
		m.HTTPResponseSizeBytes.WithLabelValues(r.Method, labelPath).Observe(float64(rw.bytes))
	})
}
