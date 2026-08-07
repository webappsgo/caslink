package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// withSchedulerID injects a chi URL param {id} so the scheduler API handlers,
// which read chi.URLParam(r,"id"), resolve the task under test.
func withSchedulerID(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func decodeEnvelope(t *testing.T, body []byte) (ok bool, data map[string]any, code string) {
	t.Helper()
	var env struct {
		OK    bool           `json:"ok"`
		Error string         `json:"error"`
		Data  map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, body)
	}
	return env.OK, env.Data, env.Error
}

// TestConfigSchedulerActionEnableDisable exercises the web POST action layer:
// a skippable task can be disabled and re-enabled (PRG redirect with ?saved=),
// while a non-skippable critical task is refused with ?err=.
func TestConfigSchedulerActionEnableDisable(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	post := func(action, task string) *httptest.ResponseRecorder {
		form := url.Values{"action": {action}, "task": {task}}
		r := httptest.NewRequest(http.MethodPost, "/server/admin/config/scheduler", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		h.ConfigSchedulerAction(w, r)
		return w
	}

	// Disable a skippable task → 303 redirect carrying ?saved=disabled.
	w := post("disable", "backup_daily")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("disable: expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "saved=disabled") {
		t.Fatalf("disable: expected saved=disabled redirect, got %q", loc)
	}

	// Re-enable it.
	w = post("enable", "backup_daily")
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "saved=enabled") {
		t.Fatalf("enable: expected saved=enabled redirect, got %q", w.Header().Get("Location"))
	}

	// Attempting to disable a non-skippable critical task is refused.
	w = post("disable", "session_cleanup")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("critical disable: expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("critical disable: expected err= redirect, got %q", loc)
	}

	// Unknown action is rejected.
	w = post("frobnicate", "backup_daily")
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("unknown action: expected err= redirect, got %q", w.Header().Get("Location"))
	}
}

// TestAPISchedulerUpdateTogglesEnabled covers PATCH {id} with {"enabled":false}.
func TestAPISchedulerUpdateTogglesEnabled(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/server/admin/config/scheduler/backup_daily", strings.NewReader(`{"enabled":false}`))
	r.Header.Set("Content-Type", "application/json")
	r = withSchedulerID(r, "backup_daily")
	w := httptest.NewRecorder()
	h.APIConfigSchedulerUpdate(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ok, data, _ := decodeEnvelope(t, w.Body.Bytes())
	if !ok {
		t.Fatal("expected ok:true")
	}
	if enabled, _ := data["enabled"].(bool); enabled {
		t.Errorf("expected enabled:false in returned task, got %v", data["enabled"])
	}

	// Missing mutable field → 400 BAD_REQUEST.
	r2 := httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{}`))
	r2 = withSchedulerID(r2, "backup_daily")
	w2 := httptest.NewRecorder()
	h.APIConfigSchedulerUpdate(w2, r2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("empty body: expected 400, got %d", w2.Code)
	}
}

// TestAPISchedulerUpdateCriticalForbidden confirms disabling a non-skippable
// task via the API maps ErrTaskNotSkippable → 403 FORBIDDEN.
func TestAPISchedulerUpdateCriticalForbidden(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	r := httptest.NewRequest(http.MethodPatch, "/x", strings.NewReader(`{"enabled":false}`))
	r = withSchedulerID(r, "session_cleanup")
	w := httptest.NewRecorder()
	h.APIConfigSchedulerUpdate(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	ok, _, code := decodeEnvelope(t, w.Body.Bytes())
	if ok || code != "FORBIDDEN" {
		t.Fatalf("expected ok:false error:FORBIDDEN, got ok=%v code=%q", ok, code)
	}
}

// TestAPISchedulerTaskAndHistoryUnknownReturns404 covers the not-found mapping.
func TestAPISchedulerTaskAndHistoryUnknownReturns404(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"task", h.APIConfigSchedulerTask},
		{"history", h.APIConfigSchedulerHistory},
		{"run", h.APIConfigSchedulerRun},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := withSchedulerID(httptest.NewRequest(http.MethodGet, "/x", nil), "no_such_task")
			w := httptest.NewRecorder()
			tc.handler(w, r)
			if w.Code != http.StatusNotFound {
				t.Fatalf("%s: expected 404, got %d: %s", tc.name, w.Code, w.Body.String())
			}
			ok, _, code := decodeEnvelope(t, w.Body.Bytes())
			if ok || code != "NOT_FOUND" {
				t.Fatalf("%s: expected ok:false NOT_FOUND, got ok=%v code=%q", tc.name, ok, code)
			}
		})
	}
}

// TestAPISchedulerRunQueues confirms a valid RunNow returns queued:true.
func TestAPISchedulerRunQueues(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	r := withSchedulerID(httptest.NewRequest(http.MethodPost, "/x", nil), "healthcheck_self")
	w := httptest.NewRecorder()
	h.APIConfigSchedulerRun(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ok, data, _ := decodeEnvelope(t, w.Body.Bytes())
	if !ok {
		t.Fatal("expected ok:true")
	}
	if queued, _ := data["queued"].(bool); !queued {
		t.Errorf("expected queued:true, got %v", data["queued"])
	}
}
