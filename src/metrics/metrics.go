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
	AppInfo          *prometheus.GaugeVec
	AppUptimeSeconds prometheus.GaugeFunc
	AppStartTimestamp prometheus.Gauge

	// ---- HTTP ----
	HTTPRequestsTotal        *prometheus.CounterVec
	HTTPRequestDuration      *prometheus.HistogramVec
	HTTPRequestSizeBytes     *prometheus.HistogramVec
	HTTPResponseSizeBytes    *prometheus.HistogramVec
	HTTPActiveRequests       prometheus.Gauge

	// ---- Database ----
	DBQueriesTotal       *prometheus.CounterVec
	DBQueryDuration      *prometheus.HistogramVec
	DBConnectionsOpen    prometheus.Gauge
	DBConnectionsInUse   prometheus.Gauge
	DBErrorsTotal        *prometheus.CounterVec

	// ---- Auth ----
	AuthAttemptsTotal  *prometheus.CounterVec
	AuthSessionsActive prometheus.Gauge

	// ---- Scheduler ----
	SchedulerTasksTotal *prometheus.CounterVec
}

var startTime = time.Now()

// New creates and registers all caslink metrics. It returns the Metrics struct
// and the HTTP handler for the /metrics endpoint.
// When includeRuntime is true the default Go runtime collectors are also registered.
func New(version, commit, buildDate string, includeRuntime bool) (*Metrics, http.Handler) {
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
	)

	if includeRuntime {
		reg.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)
	}

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		Registry: reg,
	})
	return m, handler
}

// idPattern matches path segments that look like numeric IDs, UUIDs, or
// short codes (3–50 chars of word chars/hyphens) so they can be replaced with
// a low-cardinality placeholder before being used as Prometheus label values.
var idPattern = regexp.MustCompile(
	`/([0-9]+|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[A-Za-z0-9_-]{3,50})`,
)

// normalizePath replaces high-cardinality path segments with ":id" or ":code"
// so that the Prometheus label cardinality stays bounded. Segments that look
// like UUIDs or pure integers become ":id"; short alphanumeric slugs become ":code".
func normalizePath(p string) string {
	return idPattern.ReplaceAllStringFunc(p, func(seg string) string {
		inner := seg[1:] // strip leading /
		// UUID or pure integer → :id
		if isUUID(inner) || isInt(inner) {
			return "/:id"
		}
		// Short slug (like a short URL code) → :code
		return "/:code"
	})
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

		labelPath := normalizePath(r.URL.Path)
		duration := time.Since(start).Seconds()
		statusStr := strconv.Itoa(rw.status)

		m.HTTPRequestsTotal.WithLabelValues(r.Method, labelPath, statusStr).Inc()
		m.HTTPRequestDuration.WithLabelValues(r.Method, labelPath).Observe(duration)
		m.HTTPRequestSizeBytes.WithLabelValues(r.Method, labelPath).Observe(reqSize)
		m.HTTPResponseSizeBytes.WithLabelValues(r.Method, labelPath).Observe(float64(rw.bytes))
	})
}
