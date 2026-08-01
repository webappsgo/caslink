package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/config"
	apktor "github.com/webappsgo/caslink/src/tor"
)

// TestAutodiscoverHandlerNoTor verifies the canonical success envelope, the
// FQDN-derived primary URL, and that features.tor is false when no Tor
// manager callback is supplied.
func TestAutodiscoverHandlerNoTor(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.FQDN = "example.test"

	handler := AutodiscoverHandler("1.2.3", cfg, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.OK {
		t.Fatalf("expected ok:true, got %+v", body)
	}

	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be an object, got %T", body.Data)
	}
	if data["primary"] != "http://example.test" {
		t.Errorf("expected primary=http://example.test, got %v", data["primary"])
	}

	cfgData, ok := data["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected config to be an object, got %T", data["config"])
	}
	features, ok := cfgData["features"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected features to be an object, got %T", cfgData["features"])
	}
	if features["tor"] != false {
		t.Errorf("expected features.tor=false, got %v", features["tor"])
	}
}

// TestAutodiscoverHandlerTorRunning verifies features.tor flips to true when
// the supplied getTorManager callback returns a non-nil manager.
func TestAutodiscoverHandlerTorRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.FQDN = "example.test"

	handler := AutodiscoverHandler("1.2.3", cfg, func() *apktor.TorManager {
		return &apktor.TorManager{}
	})

	r := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	var body APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	data := body.Data.(map[string]interface{})
	cfgData := data["config"].(map[string]interface{})
	features := cfgData["features"].(map[string]interface{})
	if features["tor"] != true {
		t.Errorf("expected features.tor=true, got %v", features["tor"])
	}
}

// TestAutodiscoverHandlerFQDNFallback verifies the handler falls back to
// r.Host when cfg.Server.FQDN is empty, and honors X-Forwarded-Proto.
func TestAutodiscoverHandlerFQDNFallback(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.FQDN = ""

	handler := AutodiscoverHandler("1.2.3", cfg, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
	r.Host = "fallback.test"
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	handler(w, r)

	var body APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	data := body.Data.(map[string]interface{})
	if data["primary"] != "https://fallback.test" {
		t.Errorf("expected primary=https://fallback.test, got %v", data["primary"])
	}
}

// TestAutodiscoverHandlerNeverExposesAdminPath ensures the response never
// leaks the admin path, tokens, or internal fields per the handler's own
// security comment and AI.md PART 14.
func TestAutodiscoverHandlerNeverExposesAdminPath(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Admin.Path = "super-secret-admin"

	handler := AutodiscoverHandler("1.2.3", cfg, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if body := w.Body.String(); strings.Contains(body, "super-secret-admin") {
		t.Errorf("response leaked admin path: %s", body)
	}
}
