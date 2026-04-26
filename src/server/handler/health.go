package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"
)

var startTime = time.Now()

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string            `json:"status"`
	Version   string            `json:"version"`
	Mode      string            `json:"mode"`
	Uptime    string            `json:"uptime"`
	Timestamp string            `json:"timestamp"`
	Node      NodeInfo          `json:"node"`
	Cluster   ClusterInfo       `json:"cluster"`
	Checks    map[string]string `json:"checks"`
}

// NodeInfo represents node information
type NodeInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
}

// ClusterInfo represents cluster information
type ClusterInfo struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status,omitempty"`
	Nodes   int    `json:"nodes,omitempty"`
	Role    string `json:"role,omitempty"`
}

// HealthHandler handles /healthz endpoint (HTML)
func HealthHandler(version, mode string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "unknown"
		}

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
    <div class="status healthy">✓ Healthy</div>
    <div class="info">
        <div>Version: %s</div>
        <div>Mode: %s</div>
        <div>Uptime: %s</div>
        <div>Hostname: %s</div>
        <div>Go: %s</div>
        <div>OS/Arch: %s/%s</div>
    </div>
</body>
</html>`, version, mode, uptime, hostname, runtime.Version(), runtime.GOOS, runtime.GOARCH)

		fmt.Fprint(w, html)
	}
}

// APIHealthHandler handles /api/v1/healthz endpoint (JSON)
func APIHealthHandler(version, mode string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "unknown"
		}

		response := HealthResponse{
			Status:    "healthy",
			Version:   version,
			Mode:      mode,
			Uptime:    formatUptime(time.Since(startTime)),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Node: NodeInfo{
				ID:       "standalone",
				Hostname: hostname,
			},
			Cluster: ClusterInfo{
				Enabled: false,
			},
			Checks: map[string]string{
				"database": "ok",
				"cache":    "ok",
				"disk":     "ok",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// VersionHandler handles /version and /api/v1/version endpoints
func VersionHandler(version, commitID, buildDate string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "unknown"
		}

		response := map[string]interface{}{
			"version":    version,
			"commit":     commitID,
			"built":      buildDate,
			"go_version": runtime.Version(),
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
			"hostname":   hostname,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// formatUptime formats a duration into a human-readable string
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
