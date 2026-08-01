package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/server/service"
)

func newBulkTestHandler(t *testing.T) (*BulkHandler, *service.User) {
	t.Helper()

	st := newSchemaTestStore(t)
	urlService := service.NewURLService(st)
	bulkService := service.NewBulkService(st, urlService)

	user := &service.User{ID: 1, Username: "alice"}
	return NewBulkHandler(bulkService), user
}

func withUser(r *http.Request, u *service.User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), UserContextKey, u))
}

// TestBulkExportUnauthenticated verifies Export requires an authenticated
// user in context.
func TestBulkExportUnauthenticated(t *testing.T) {
	h, _ := newBulkTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/urls/export", nil)
	w := httptest.NewRecorder()
	h.Export(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestBulkExportInvalidFormat verifies an unsupported format value is
// rejected with 400.
func TestBulkExportInvalidFormat(t *testing.T) {
	h, user := newBulkTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/urls/export?format=xml", nil)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Export(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestBulkExportJSONDefaultEmpty verifies the default (no format param)
// export returns JSON with 200 even for a user with zero URLs.
func TestBulkExportJSONDefaultEmpty(t *testing.T) {
	h, user := newBulkTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/urls/export", nil)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Export(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

// TestBulkExportCSV verifies format=csv returns a CSV attachment.
func TestBulkExportCSV(t *testing.T) {
	h, user := newBulkTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/users/urls/export?format=csv", nil)
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Export(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("expected text/csv content type, got %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "urls.csv") {
		t.Errorf("expected attachment filename urls.csv, got %q", cd)
	}
}

// TestBulkImportUnauthenticated verifies Import requires an authenticated
// user in context.
func TestBulkImportUnauthenticated(t *testing.T) {
	h, _ := newBulkTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/urls/import", strings.NewReader("[]"))
	w := httptest.NewRecorder()
	h.Import(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// TestBulkImportJSONRawBody verifies a raw JSON-array body (no multipart)
// is accepted, creates URLs, and returns the canonical success envelope
// with success/error counts.
func TestBulkImportJSONRawBody(t *testing.T) {
	h, user := newBulkTestHandler(t)

	body := `[{"url":"https://example.com/one"},{"url":"https://example.com/two"}]`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/urls/import", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Import(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// respondJSON always wraps the payload as {"ok":true,"data":{...}}, so
	// unwrap the envelope before inspecting the success count.
	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if success, _ := env.Data["success"].(float64); success != 2 {
		t.Errorf("expected success=2, got %v", env.Data["success"])
	}
}

// TestBulkImportCSVRawBody verifies a raw CSV body (Content-Type: text/csv)
// is parsed and imported.
func TestBulkImportCSVRawBody(t *testing.T) {
	h, user := newBulkTestHandler(t)

	body := "url,custom_code,title\nhttps://example.com/three,,My Link\n"
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/urls/import", strings.NewReader(body))
	r.Header.Set("Content-Type", "text/csv")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Import(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// respondJSON always wraps the payload as {"ok":true,"data":{...}}, so
	// unwrap the envelope before inspecting the success count.
	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if success, _ := env.Data["success"].(float64); success != 1 {
		t.Errorf("expected success=1, got %v", env.Data["success"])
	}
}

// TestBulkImportInvalidJSONBody verifies malformed JSON returns 400 via the
// canonical error envelope (non-form request path).
func TestBulkImportInvalidJSONBody(t *testing.T) {
	h, user := newBulkTestHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/urls/import", strings.NewReader("{not valid json"))
	r.Header.Set("Content-Type", "application/json")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Import(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestBulkImportFormatOverride verifies the ?format= query param overrides
// content-type based detection.
func TestBulkImportFormatOverride(t *testing.T) {
	h, user := newBulkTestHandler(t)

	body := `[{"url":"https://example.com/four"}]`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/urls/import?format=json", strings.NewReader(body))
	r.Header.Set("Content-Type", "text/plain")
	r = withUser(r, user)
	w := httptest.NewRecorder()
	h.Import(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
