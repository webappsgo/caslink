package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/tmpl"
)

// newPagesTestHandler builds a PagesHandler with a real renderer and a nil
// Tor manager getter (disabling all Tor sections), mirroring the other
// handler tests' convention of using real dependencies over mocks.
func newPagesTestHandler(t *testing.T) *PagesHandler {
	t.Helper()

	cfg := config.DefaultConfig()
	renderer, err := tmpl.New()
	if err != nil {
		t.Fatalf("tmpl.New failed: %v", err)
	}

	return NewPagesHandler(cfg, renderer, "1.2.3", "2026-01-01", nil)
}

// ---- About ----

func TestAboutRenders(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/about", nil)
	w := httptest.NewRecorder()
	h.About(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), caslinkTagline) {
		t.Error("expected the tagline to appear in the About page")
	}
}

// ---- Help ----

func TestHelpRendersWithNilTorManager(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/help", nil)
	w := httptest.NewRecorder()
	h.Help(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Getting Started") {
		t.Error("expected the Getting Started section to be rendered")
	}
}

func TestHelpUsesHTTPSWhenSSLEnabled(t *testing.T) {
	h := newPagesTestHandler(t)
	h.cfg.Server.SSL.Enabled = true
	h.cfg.Server.FQDN = "example.test"

	r := httptest.NewRequest(http.MethodGet, "/server/help", nil)
	w := httptest.NewRecorder()
	h.Help(w, r)

	if !strings.Contains(w.Body.String(), "https://example.test") {
		t.Error("expected the API base to use https:// when SSL is enabled")
	}
}

func TestHelpUsesHTTPWhenSSLDisabled(t *testing.T) {
	h := newPagesTestHandler(t)
	h.cfg.Server.SSL.Enabled = false
	h.cfg.Server.FQDN = "example.test"

	r := httptest.NewRequest(http.MethodGet, "/server/help", nil)
	w := httptest.NewRecorder()
	h.Help(w, r)

	if !strings.Contains(w.Body.String(), "http://example.test") {
		t.Error("expected the API base to use http:// when SSL is disabled")
	}
}

func TestHelpFallsBackToDefaultFQDN(t *testing.T) {
	h := newPagesTestHandler(t)
	h.cfg.Server.FQDN = ""

	r := httptest.NewRequest(http.MethodGet, "/server/help", nil)
	w := httptest.NewRecorder()
	h.Help(w, r)

	if !strings.Contains(w.Body.String(), "caslink.casapps.us") {
		t.Error("expected the default FQDN fallback to be used when FQDN is unset")
	}
}

// ---- buildHelpSections ----

func TestBuildHelpSectionsIncludesAPIBase(t *testing.T) {
	sections := buildHelpSections("https://example.test", "/api/v1")
	if len(sections) == 0 {
		t.Fatal("expected at least one help section")
	}
	found := false
	for _, s := range sections {
		for _, item := range s.Items {
			if strings.Contains(item.Content, "https://example.test") {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected the API base to appear in at least one help item")
	}
}

// ---- Privacy ----

func TestPrivacyRenders(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/privacy", nil)
	w := httptest.NewRecorder()
	h.Privacy(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ---- Contact (GET) ----

func TestContactRendersWithoutSent(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/contact", nil)
	w := httptest.NewRecorder()
	h.Contact(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestContactRendersWithSentQueryParam(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/contact?sent=1", nil)
	w := httptest.NewRecorder()
	h.Contact(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ---- ContactSubmit ----

func TestContactSubmitBadForm(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/server/contact?a=%zz", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ContactSubmit(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unparseable form, got %d", w.Code)
	}
}

func TestContactSubmitMissingFields(t *testing.T) {
	h := newPagesTestHandler(t)

	form := url.Values{"name": {"Alice"}}
	r := httptest.NewRequest(http.MethodPost, "/server/contact", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ContactSubmit(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing fields, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Email is required") {
		t.Error("expected the email-required error to be rendered")
	}
}

func TestContactSubmitSuccessRedirects(t *testing.T) {
	h := newPagesTestHandler(t)

	form := url.Values{
		"name":    {"Alice"},
		"email":   {"alice@example.com"},
		"subject": {"Hello"},
		"message": {"This is a test message."},
	}
	r := httptest.NewRequest(http.MethodPost, "/server/contact", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ContactSubmit(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/server/contact?sent=1" {
		t.Errorf("expected redirect to /server/contact?sent=1, got %q", loc)
	}
}

// ---- Terms ----

func TestTermsRenders(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/server/terms", nil)
	w := httptest.NewRecorder()
	h.Terms(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ---- JSON API handlers ----

func TestAPIAboutReturnsJSON(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/about", nil)
	w := httptest.NewRecorder()
	h.APIAbout(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got %v", env.OK)
	}
	if env.Data["name"] != "Caslink" {
		t.Errorf("expected name=Caslink, got %v", env.Data["name"])
	}
	if env.Data["version"] != "1.2.3" {
		t.Errorf("expected version=1.2.3, got %v", env.Data["version"])
	}
}

func TestAPIHelpReturnsJSON(t *testing.T) {
	h := newPagesTestHandler(t)
	h.cfg.Server.FQDN = "example.test"
	h.cfg.Server.SSL.Enabled = true

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/help", nil)
	w := httptest.NewRecorder()
	h.APIHelp(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got %v", env.OK)
	}
	if env.Data["api_base"] != "https://example.test" {
		t.Errorf("expected api_base=https://example.test, got %v", env.Data["api_base"])
	}
	if torEnabled, ok := env.Data["tor_enabled"].(bool); !ok || torEnabled {
		t.Errorf("expected tor_enabled=false with a nil Tor manager, got %v", env.Data["tor_enabled"])
	}
}

func TestAPIPrivacyReturnsJSON(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/privacy", nil)
	w := httptest.NewRecorder()
	h.APIPrivacy(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got %v", env.OK)
	}
	if env.Data["data_sold"] != false {
		t.Errorf("expected data_sold=false, got %v", env.Data["data_sold"])
	}
}

func TestAPITermsReturnsJSON(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/terms", nil)
	w := httptest.NewRecorder()
	h.APITerms(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got %v", env.OK)
	}
	if env.Data["service"] != "Caslink" {
		t.Errorf("expected service=Caslink, got %v", env.Data["service"])
	}
}

func TestAPIContactInvalidJSON(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", strings.NewReader("{not-json"))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.APIContact(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if body["ok"] != false {
		t.Errorf("expected ok=false, got %v", body["ok"])
	}
}

func TestAPIContactMissingFields(t *testing.T) {
	h := newPagesTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", strings.NewReader(`{"name":"Alice"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.APIContact(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing fields, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	msg, _ := body["message"].(string)
	for _, want := range []string{"email", "subject", "message"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected the missing-fields message %q to mention %q", msg, want)
		}
	}
}

func TestAPIContactSuccess(t *testing.T) {
	h := newPagesTestHandler(t)

	body := `{"name":"Alice","email":"alice@example.com","subject":"Hello","message":"A test message."}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.APIContact(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got %v", env.OK)
	}
	if env.Data["sent"] != true {
		t.Errorf("expected sent=true, got %v", env.Data["sent"])
	}
}
