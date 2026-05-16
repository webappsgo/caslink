package handler

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/store"
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
	Enabled bool `json:"enabled"`
}

// FeaturesInfo lists optional feature toggles.
type FeaturesInfo struct {
	GeoIP         bool `json:"geoip"`
	CustomSlugs   bool `json:"custom_slugs"`
	Analytics     bool `json:"analytics"`
	MultiUser     bool `json:"multi_user"`
	Organizations bool `json:"organizations"`
	CustomDomains bool `json:"custom_domains"`
}

// ChecksInfo holds sub-system liveness results.
type ChecksInfo struct {
	Database  string `json:"database"`
	Scheduler string `json:"scheduler"`
	Disk      string `json:"disk"`
}

// CaslinkStatsInfo extends StatsInfo with URL-shortener specific fields.
type CaslinkStatsInfo struct {
	LinksTotal   int64 `json:"links_total"`
	Redirects24h int64 `json:"redirects_24h"`
}

// StatsInfo wraps caslink-specific stats.
type StatsInfo struct {
	CaslinkStatsInfo
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
func APIHealthHandler(version, commitID, buildDate, mode string, st *store.Store) http.HandlerFunc {
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

		status := "healthy"
		if checks.Database != "ok" {
			status = "degraded"
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
			Cluster:   ClusterInfo{Enabled: false},
			Features: FeaturesInfo{
				GeoIP:         false,
				CustomSlugs:   true,
				Analytics:     true,
				MultiUser:     true,
				Organizations: true,
				CustomDomains: true,
			},
			Checks: checks,
			Stats: StatsInfo{
				CaslinkStatsInfo: CaslinkStatsInfo{
					LinksTotal:   linksTotal,
					Redirects24h: redirects24h,
				},
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
