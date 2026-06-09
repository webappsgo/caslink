package handler

import (
	"net/http"

	"github.com/casjaysdevdocker/caslink/src/config"
	apktor "github.com/casjaysdevdocker/caslink/src/tor"
)

// AutodiscoverHandler returns the /api/autodiscover endpoint per AI.md PART 14.
// This endpoint is NOT versioned — clients need it BEFORE they know the API version.
// SECURITY: NEVER include admin_path, tokens, passwords, or internal IPs.
func AutodiscoverHandler(version string, cfg *config.Config, getTorManager func() *apktor.TorManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Determine public server URL from config FQDN or Host header.
		fqdn := cfg.Server.FQDN
		if fqdn == "" {
			fqdn = r.Host
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		primary := scheme + "://" + fqdn

		// Tor availability signal (opt-in: only include when running).
		torEnabled := false
		if getTorManager != nil {
			if tm := getTorManager(); tm != nil {
				torEnabled = true
			}
		}

		resp := map[string]any{
			"primary":     primary,
			"cluster":     []string{},
			"api_version": "v1",
			"timeout":     30,
			"retry":       3,
			"retry_delay": 1,
			"config": map[string]any{
				"database": map[string]any{
					"drivers": []string{"file", "sqlite", "postgres", "mysql", "mssql"},
					"aliases": map[string]string{
						"sqlite2":    "sqlite",
						"sqlite3":    "sqlite",
						"pgsql":      "postgres",
						"postgresql": "postgres",
						"mariadb":    "mysql",
					},
					"ssl_modes": []string{"disable", "require", "verify-full"},
				},
				"cache": map[string]any{
					"types": []string{"none", "memory"},
				},
				"formats": map[string]any{
					"duration": []string{"s", "m", "h", "d"},
					"size":     []string{"KB", "MB", "GB"},
				},
				"logging": map[string]any{
					"levels": []string{"debug", "info", "warn", "error"},
				},
				"smtp": map[string]any{
					"tls_modes": []string{"auto", "starttls", "tls", "none"},
				},
				"features": map[string]any{
					"clustering": false,
					"tor":        torEnabled,
					"webauthn":   true,
					"oauth":      []string{},
				},
			},
		}

		w.Header().Set("Cache-Control", "public, max-age=3600")
		respondJSON(w, http.StatusOK, resp)
	}
}
