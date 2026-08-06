package swagger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetTheme covers the priority order documented in swagger/theme.go:
// request > config > default(dark), with invalid values at either level
// falling through to the next source rather than being accepted verbatim.
func TestGetTheme(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		config    string
		want      string
	}{
		{"empty request and config defaults to dark", "", "", ThemeDark},
		{"valid request wins over valid config", ThemeLight, ThemeDark, ThemeLight},
		{"valid request wins over empty config", ThemeAuto, "", ThemeAuto},
		{"invalid request falls through to valid config", "neon", ThemeLight, ThemeLight},
		{"invalid request and empty config defaults to dark", "neon", "", ThemeDark},
		{"empty request falls through to valid config", "", ThemeAuto, ThemeAuto},
		{"empty request falls through to invalid config, defaults to dark", "", "neon", ThemeDark},
		{"invalid request and invalid config defaults to dark", "neon", "also-neon", ThemeDark},
		{"request dark explicit", ThemeDark, ThemeLight, ThemeDark},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetTheme(tt.requested, tt.config); got != tt.want {
				t.Errorf("GetTheme(%q, %q) = %q, want %q", tt.requested, tt.config, got, tt.want)
			}
		})
	}
}

// TestGenerateOpenAPISpec_VersionSubstitution verifies the version parameter
// flows through into info.version rather than being hardcoded or dropped.
func TestGenerateOpenAPISpec_VersionSubstitution(t *testing.T) {
	spec := generateOpenAPISpec("1.2.3", "/api/v1")

	info, ok := spec["info"].(map[string]interface{})
	if !ok {
		t.Fatalf("spec[\"info\"] is not a map: %T", spec["info"])
	}
	if got := info["version"]; got != "1.2.3" {
		t.Errorf("info.version = %v, want %q", got, "1.2.3")
	}
	if info["title"] != "Caslink API" {
		t.Errorf("info.title = %v, want %q", info["title"], "Caslink API")
	}
}

// TestGenerateOpenAPISpec_RequiredPaths verifies the documented REST surface
// (PART 14 route scopes) is present with the correct HTTP methods, since a
// swagger spec that silently drops an endpoint is worse than no spec at all.
func TestGenerateOpenAPISpec_RequiredPaths(t *testing.T) {
	spec := generateOpenAPISpec("dev", "/api/v1")
	paths, ok := spec["paths"].(map[string]interface{})
	if !ok {
		t.Fatalf("spec[\"paths\"] is not a map: %T", spec["paths"])
	}

	wantPathMethods := map[string][]string{
		"/healthz":     {"get"},
		"/version":     {"get"},
		"/urls":        {"get", "post"},
		"/urls/{code}": {"get", "patch", "delete"},
	}
	for path, methods := range wantPathMethods {
		entry, ok := paths[path].(map[string]interface{})
		if !ok {
			t.Errorf("paths[%q] missing or not a map", path)
			continue
		}
		for _, method := range methods {
			if _, ok := entry[method]; !ok {
				t.Errorf("paths[%q] missing method %q", path, method)
			}
		}
	}
}

// TestSpecHandler_ServesValidJSON verifies the HTTP handler wraps
// generateOpenAPISpec correctly: 200 status, application/json content type,
// and a body that decodes back into the same version/paths shape.
func TestSpecHandler_ServesValidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/swagger", nil)
	rec := httptest.NewRecorder()

	SpecHandler("9.9.9", "/api/v1")(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	info, ok := decoded["info"].(map[string]interface{})
	if !ok {
		t.Fatalf("decoded[\"info\"] is not a map: %T", decoded["info"])
	}
	if info["version"] != "9.9.9" {
		t.Errorf("decoded info.version = %v, want %q", info["version"], "9.9.9")
	}
}

// TestHandler_ThemeSelection verifies the Swagger UI page renders the
// correct embedded CSS block depending on the ?theme= query param, and that
// the documented default (dark, when the param is omitted) actually applies.
func TestHandler_ThemeSelection(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantSubstr string
	}{
		{"no theme param defaults to dark", "", "#282a36"},
		{"explicit dark theme", "?theme=dark", "#282a36"},
		{"explicit light theme", "?theme=light", "#f5f5f5"},
		// Handler does not validate theme values the way GetTheme does — any
		// non-"dark" string falls through to the light-theme branch in the
		// template's {{if eq .Theme "dark"}} check. This test documents that
		// actual (permissive) behavior rather than assuming validation exists.
		{"unrecognized theme falls back to light branch", "?theme=neon", "#f5f5f5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/server/docs/swagger"+tt.query, nil)
			rec := httptest.NewRecorder()

			Handler("1.0.0", "/api/v1")(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("Content-Type = %q, want text/html prefix", ct)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.wantSubstr) {
				t.Errorf("body missing expected theme marker %q", tt.wantSubstr)
			}
			if !strings.Contains(body, "Caslink API Documentation") {
				t.Error("body missing page title")
			}
		})
	}
}

// TestStaticHandler_ServesEmbeddedAssets verifies the vendored Swagger UI
// assets are reachable at the documented prefix and that the handler 404s
// for paths that don't exist in the embedded FS (rather than panicking or
// serving a directory listing).
func TestStaticHandler_ServesEmbeddedAssets(t *testing.T) {
	handler := StaticHandler()

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"bundle js served", "/server/docs/swagger/static/swagger-ui-bundle.js", http.StatusOK},
		{"css served", "/server/docs/swagger/static/swagger-ui.css", http.StatusOK},
		{"unknown asset 404s", "/server/docs/swagger/static/does-not-exist.js", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK && rec.Body.Len() == 0 {
				t.Error("expected non-empty body for served asset")
			}
		})
	}
}
