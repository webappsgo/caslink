package handler

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/store"
	apktor "github.com/casjaysdevdocker/caslink/src/tor"
)

var startTime = time.Now()

// HealthResponse is the canonical /api/v1/server/healthz JSON shape (AI.md PART 13).
type HealthResponse struct {
	Project   ProjectInfo  `json:"project"`
	Status    string       `json:"status"`
	Version   string       `json:"version"`
	GoVersion string       `json:"go_version"`
	Build     BuildInfo    `json:"build"`
	Uptime    string       `json:"uptime"`
	Mode      string       `json:"mode"`
	Timestamp time.Time    `json:"timestamp"`
	Cluster   ClusterInfo  `json:"cluster"`
	Features  FeaturesInfo `json:"features"`
	Checks    ChecksInfo   `json:"checks"`
	Stats     StatsInfo    `json:"stats"`
}

// ProjectInfo holds the project identity fields.
type ProjectInfo struct {
	Name        string `json:"name"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
}

// BuildInfo holds the VCS and build metadata.
type BuildInfo struct {
	Commit string `json:"commit"`
	Date   string `json:"date"`
}

// ClusterInfo describes cluster/HA state.
type ClusterInfo struct {
	Enabled bool     `json:"enabled"`
	Status  string   `json:"status,omitempty"`
	Primary string   `json:"primary,omitempty"`
	Nodes   []string `json:"nodes"`
	Role    string   `json:"role,omitempty"`
}

// TorInfo describes the Tor hidden service status per AI.md PART 13 + PART 32.
type TorInfo struct {
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	Status   string `json:"status"`
	Hostname string `json:"hostname"`
}

// FeaturesInfo lists optional feature toggles.
type FeaturesInfo struct {
	GeoIP         bool    `json:"geoip"`
	CustomSlugs   bool    `json:"custom_slugs"`
	Analytics     bool    `json:"analytics"`
	MultiUser     bool    `json:"multi_user"`
	Organizations bool    `json:"organizations"`
	CustomDomains bool    `json:"custom_domains"`
	Tor           TorInfo `json:"tor"`
}

// ChecksInfo holds sub-system liveness results.
type ChecksInfo struct {
	Database  string `json:"database"`
	Scheduler string `json:"scheduler"`
	Disk      string `json:"disk"`
	Tor       string `json:"tor,omitempty"`
}

// StatsInfo holds request and operational statistics for the health response.
// Includes generic fields required by AI.md PART 13 plus caslink-specific fields.
type StatsInfo struct {
	// Standard fields required by AI.md PART 13
	RequestsTotal    int64 `json:"requests_total"`
	Requests24h      int64 `json:"requests_24h"`
	ActiveConnections int64 `json:"active_connections"`
	// Caslink-specific fields
	LinksTotal   int64 `json:"links_total"`
	Redirects24h int64 `json:"redirects_24h"`
}

// HealthHandler handles /server/healthz endpoint (HTML).
func HealthHandler(version, commitID, buildDate, mode string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uptime := formatUptime(time.Since(startTime))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Health Check - Caslink</title>
    <style>
        body { font-family: monospace; margin: 40px; background: #1a1a1a; color: #00ff00; }
        .status { font-size: 24px; margin-bottom: 20px; }
        .info { line-height: 1.8; }
        .healthy { color: #00ff00; }
    </style>
</head>
<body>
    <div class="status healthy">OK</div>
    <div class="info">
        <div>Version: %s</div>
        <div>Commit: %s</div>
        <div>Built: %s</div>
        <div>Mode: %s</div>
        <div>Uptime: %s</div>
        <div>Go: %s</div>
    </div>
</body>
</html>`, version, commitID, buildDate, mode, uptime, runtime.Version())

		fmt.Fprint(w, html)
	}
}

// APIHealthHandler handles /api/v1/server/healthz endpoint (JSON).
// The store parameter is used to probe the DB; pass nil for testing.
// getTorManager may be nil when Tor is not available.
// getCounters returns (requestsTotal, requests24h, activeConnections) from server-level atomics.
func APIHealthHandler(version, commitID, buildDate, mode string, st *store.Store, getTorManager func() *apktor.TorManager, getCounters func() (int64, int64, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := ChecksInfo{
			Scheduler: "ok",
			Disk:      "ok",
		}

		if st != nil && st.Ping() == nil {
			checks.Database = "ok"
		} else {
			checks.Database = "error"
		}

		// Tor health check
		torInfo := TorInfo{}
		if getTorManager != nil {
			if tm := getTorManager(); tm != nil {
				torInfo.Enabled = true
				torInfo.Running = true
				torInfo.Status = "healthy"
				torInfo.Hostname = tm.OnionAddress()
				checks.Tor = "ok"
			} else {
				torInfo.Status = ""
			}
		}

		status := "healthy"
		if checks.Database != "ok" {
			status = "degraded"
		}

		var reqTotal, reqs24h, activeConn int64
		if getCounters != nil {
			reqTotal, reqs24h, activeConn = getCounters()
		}

		var linksTotal, redirects24h int64
		if st != nil {
			if n, err := st.CountURLs(); err == nil {
				linksTotal = n
			}
			if n, err := st.CountClicks24h(); err == nil {
				redirects24h = n
			}
		}

		resp := HealthResponse{
			Project: ProjectInfo{
				Name:        "Caslink",
				Tagline:     "Self-hosted URL shortener",
				Description: "A secure, mobile-first, fully self-hosted URL shortener",
			},
			Status:    status,
			Version:   version,
			GoVersion: runtime.Version(),
			Build: BuildInfo{
				Commit: commitID,
				Date:   buildDate,
			},
			Uptime:    formatUptime(time.Since(startTime)),
			Mode:      mode,
			Timestamp: time.Now().UTC(),
			Cluster: ClusterInfo{
				Enabled: false,
				Nodes:   []string{},
			},
			Features: FeaturesInfo{
				GeoIP:         false,
				CustomSlugs:   true,
				Analytics:     true,
				MultiUser:     true,
				Organizations: true,
				CustomDomains: true,
				Tor:           torInfo,
			},
			Checks: checks,
			Stats: StatsInfo{
				RequestsTotal:     reqTotal,
				Requests24h:       reqs24h,
				ActiveConnections: activeConn,
				LinksTotal:        linksTotal,
				Redirects24h:      redirects24h,
			},
		}

		respondJSON(w, http.StatusOK, resp)
	}
}

// VersionHandler handles /version and /api/v1/version endpoints.
func VersionHandler(version, commitID, buildDate string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]any{
			"version":    version,
			"commit":     commitID,
			"built":      buildDate,
			"go_version": runtime.Version(),
		})
	}
}

// formatUptime formats a duration into a human-readable string.
func formatUptime(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
