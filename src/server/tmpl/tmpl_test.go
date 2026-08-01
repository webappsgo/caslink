package tmpl

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewParsesAllTemplates(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if r == nil {
		t.Fatal("New() returned nil Renderer")
	}
	if len(r.pages) == 0 {
		t.Fatal("New() parsed zero pages")
	}
	if _, ok := r.pages["template/page/dashboard.html"]; !ok {
		t.Error("expected template/page/dashboard.html to be parsed")
	}
}

func TestRenderKnownPage(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	// error.html's own field set is small and stable (Code/Title/Message),
	// unlike most pages which need handler-specific data structs.
	const name = "template/page/error.html"
	if _, ok := r.pages[name]; !ok {
		t.Fatalf("expected %s to be parsed", name)
	}

	w := httptest.NewRecorder()
	r.Render(w, name, map[string]interface{}{
		"Code":    404,
		"Title":   "Not Found",
		"Message": "The page you requested does not exist.",
	})

	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body=%q", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Not Found") {
		t.Errorf("body does not contain rendered Title: %q", w.Body.String())
	}
}

func TestRenderUnknownPageReturns500(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	w := httptest.NewRecorder()
	r.Render(w, "template/page/does-not-exist.html", nil)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(w.Body.String(), "Internal Server Error") {
		t.Errorf("body = %q, want it to contain Internal Server Error", w.Body.String())
	}
}

func TestStaticHandlerServesEmbeddedFiles(t *testing.T) {
	h := StaticHandler()

	req := httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%q", w.Code, http.StatusOK, w.Body.String())
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty app.css body")
	}
}

func TestStaticHandlerMissingFile404s(t *testing.T) {
	h := StaticHandler()

	req := httptest.NewRequest(http.MethodGet, "/static/does-not-exist.css", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestUptimeIncreasesOverTime(t *testing.T) {
	first := Uptime()
	time.Sleep(1100 * time.Millisecond)
	second := Uptime()

	if second <= first {
		t.Errorf("Uptime did not increase: first=%d second=%d", first, second)
	}
	if first < 0 || second < 0 {
		t.Errorf("Uptime returned negative value: first=%d second=%d", first, second)
	}
}
