package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

var startTime = time.Now()

// apiHealthData is the data payload for the spec-required JSON health response.
type apiHealthData struct {
	AppName    string `json:"app_name"`
	Version    string `json:"version"`
	CommitHash string `json:"commit_hash"`
	BuildDate  string `json:"build_date"`
	Uptime     int64  `json:"uptime"`
	Mode       string `json:"mode"`
	DBType     string `json:"db_type"`
	DBLocality string `json:"db_locality"`
}

// HealthHandler handles /server/healthz endpoint (HTML)
func HealthHandler(version, mode string) http.HandlerFunc {
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
        <div>Mode: %s</div>
        <div>Uptime: %s</div>
        <div>Go: %s</div>
    </div>
</body>
</html>`, version, mode, uptime, runtime.Version())

		fmt.Fprint(w, html)
	}
}

// APIHealthHandler handles /api/v1/server/healthz endpoint (JSON).
// The store parameter is used to detect the DB type; pass nil for testing.
func APIHealthHandler(version, mode string, st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbType := "sqlite"
		dbLocality := "local"
		if st != nil {
			dbType = st.DBType()
			dbLocality = st.DBLocality()
		}

		data := apiHealthData{
			AppName:    "caslink",
			Version:    version,
			CommitHash: "",
			BuildDate:  "",
			Uptime:     int64(time.Since(startTime).Seconds()),
			Mode:       mode,
			DBType:     dbType,
			DBLocality: dbLocality,
		}

		response := map[string]interface{}{
			"ok":   true,
			"data": data,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// VersionHandler handles /version and /api/v1/version endpoints
func VersionHandler(version, commitID, buildDate string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"version":    version,
			"commit":     commitID,
			"built":      buildDate,
			"go_version": runtime.Version(),
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
