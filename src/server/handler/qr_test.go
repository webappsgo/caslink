package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/service"
)

// withChiURLParam attaches a chi route context carrying the given URL
// param, mirroring what chi's router does at request-dispatch time.
//
// If the request already carries a route context (e.g. from a prior call
// to this helper), the param is added to that existing context instead of
// replacing it — otherwise chaining calls to set multiple params (like
// "slug" then "tokenID") would silently discard all but the last one.
func withChiURLParam(r *http.Request, name, value string) *http.Request {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		rctx.URLParams.Add(name, value)
		return r
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(name, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func newQRTestHandler(t *testing.T) (*QRHandler, *model.URL) {
	t.Helper()

	st := newSchemaTestStore(t)
	urlService := service.NewURLService(st)
	qrService := service.NewQRService(st)

	u, err := urlService.CreateURL(context.Background(), &model.CreateURLRequest{
		LongURL: "https://example.com/target",
	})
	if err != nil {
		t.Fatalf("CreateURL failed: %v", err)
	}

	return NewQRHandler(qrService, urlService), u
}

// TestGenerateQRMissingCode verifies a 400 is returned when the chi route
// param is absent/empty.
func TestGenerateQRMissingCode(t *testing.T) {
	h, _ := newQRTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/qr/", nil)
	r = withChiURLParam(r, "code", "")
	w := httptest.NewRecorder()
	h.GenerateQR(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestGenerateQRNotFound verifies a 404 is returned for an unknown code.
func TestGenerateQRNotFound(t *testing.T) {
	h, _ := newQRTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/qr/doesnotexist", nil)
	r = withChiURLParam(r, "code", "doesnotexist")
	w := httptest.NewRecorder()
	h.GenerateQR(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestGenerateQRDefaultPNG verifies the happy path returns a PNG image with
// the expected headers and non-empty body when no query params are given.
func TestGenerateQRDefaultPNG(t *testing.T) {
	h, u := newQRTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/qr/"+u.ShortCode, nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.GenerateQR(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected image/png, got %q", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty QR image body")
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=86400" {
		t.Errorf("expected cache-control public max-age=86400, got %q", cc)
	}
}

// TestGenerateQRSVGFormat verifies format=svg is honored and served as-is.
func TestGenerateQRSVGFormat(t *testing.T) {
	h, u := newQRTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/qr/"+u.ShortCode+"?format=svg", nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.GenerateQR(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("expected image/svg+xml, got %q", ct)
	}
}

// TestGenerateQRUnknownFormatFallsBackToPNG verifies a format outside
// {png,svg} silently falls back to png rather than erroring.
func TestGenerateQRUnknownFormatFallsBackToPNG(t *testing.T) {
	h, u := newQRTestHandler(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/qr/"+u.ShortCode+"?format=pdf", nil)
	r = withChiURLParam(r, "code", u.ShortCode)
	w := httptest.NewRecorder()
	h.GenerateQR(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected fallback image/png, got %q", ct)
	}
}

// TestGenerateQRSizeClamping verifies the DoS-guard clamp: sizes below 64
// clamp up to 64, and sizes above 2048 clamp down to 2048. We can't inspect
// the internal size directly, so we assert both requests still succeed and
// the larger request produces a larger (or equal) response body, which
// would not hold if clamping were broken/inverted.
func TestGenerateQRSizeClamping(t *testing.T) {
	h, u := newQRTestHandler(t)

	small := httptest.NewRequest(http.MethodGet, "/api/v1/qr/"+u.ShortCode+"?size=1", nil)
	small = withChiURLParam(small, "code", u.ShortCode)
	wSmall := httptest.NewRecorder()
	h.GenerateQR(wSmall, small)

	large := httptest.NewRequest(http.MethodGet, "/api/v1/qr/"+u.ShortCode+"?size=99999", nil)
	large = withChiURLParam(large, "code", u.ShortCode)
	wLarge := httptest.NewRecorder()
	h.GenerateQR(wLarge, large)

	if wSmall.Code != http.StatusOK || wLarge.Code != http.StatusOK {
		t.Fatalf("expected both to succeed, got %d and %d", wSmall.Code, wLarge.Code)
	}
	if wLarge.Body.Len() <= wSmall.Body.Len() {
		t.Errorf("expected clamped-large QR body (%d bytes) to exceed clamped-small (%d bytes)", wLarge.Body.Len(), wSmall.Body.Len())
	}
}
