package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/server/service"
)

// seedAdminUserDomain registers a user and a custom domain owned by them via
// the AdminHandler's own domain service, returning the domain name.
func seedAdminUserDomain(t *testing.T, h *AdminHandler, authService *service.AuthService, name string) string {
	t.Helper()
	user, err := authService.RegisterUser(context.Background(), "domainowner", "owner@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	d, err := h.domainService.AddDomain(context.Background(), "user", user.ID, name)
	if err != nil {
		t.Fatalf("AddDomain(%q) failed: %v", name, err)
	}
	return d.Domain
}

// TestConfigDomainsListEmpty renders the admin domain list with no domains.
func TestConfigDomainsListEmpty(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/config/domains", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigDomains(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "No custom domains") {
		t.Errorf("expected empty-state message, got: %s", w.Body.String())
	}
}

// TestConfigDomainsListShowsDomain renders a seeded domain in the table.
func TestConfigDomainsListShowsDomain(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)
	name := seedAdminUserDomain(t, h, authService, "example.com")

	r := httptest.NewRequest(http.MethodGet, "/server/admin/config/domains", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigDomains(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), name) {
		t.Errorf("expected domain %q in list, got: %s", name, w.Body.String())
	}
}

// TestConfigDomainDetailNotFound renders an error for an unknown domain.
func TestConfigDomainDetailNotFound(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	r := httptest.NewRequest(http.MethodGet, "/server/admin/config/domains/nope.example", nil)
	r = withChiURLParam(r, "domain", "nope.example")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigDomainDetail(w, r)

	if !strings.Contains(w.Body.String(), "Domain not found") {
		t.Errorf("expected not-found error, got: %s", w.Body.String())
	}
}

// TestConfigDomainSuspendUnsuspend suspends then unsuspends a seeded domain and
// verifies the persisted status transitions and POST-redirect-GET behaviour.
func TestConfigDomainSuspendUnsuspend(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)
	name := seedAdminUserDomain(t, h, authService, "example.com")

	// Suspend.
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/domains/"+name+"/suspend", nil)
	r = withChiURLParam(r, "domain", name)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigDomainSuspend(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("suspend: expected 303, got %d: %s", w.Code, w.Body.String())
	}
	d, err := h.domainService.GetDomainByName(context.Background(), name)
	if err != nil {
		t.Fatalf("GetDomainByName failed: %v", err)
	}
	if !strings.EqualFold(d.Status, "suspended") {
		t.Fatalf("expected status suspended, got %q", d.Status)
	}

	// Unsuspend.
	r = httptest.NewRequest(http.MethodPost, "/server/admin/config/domains/"+name+"/unsuspend", nil)
	r = withChiURLParam(r, "domain", name)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ConfigDomainUnsuspend(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("unsuspend: expected 303, got %d", w.Code)
	}
	d, err = h.domainService.GetDomainByName(context.Background(), name)
	if err != nil {
		t.Fatalf("GetDomainByName failed: %v", err)
	}
	if strings.EqualFold(d.Status, "suspended") {
		t.Fatalf("expected status not suspended after unsuspend, got %q", d.Status)
	}
}

// TestConfigDomainDelete force-deletes a seeded domain and verifies it is gone.
func TestConfigDomainDelete(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)
	name := seedAdminUserDomain(t, h, authService, "example.com")

	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/domains/"+name+"/delete", nil)
	r = withChiURLParam(r, "domain", name)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigDomainDelete(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("delete: expected 303, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "done=delete") {
		t.Errorf("expected redirect to list with done=delete, got %q", loc)
	}
	if _, err := h.domainService.GetDomainByName(context.Background(), name); err == nil {
		t.Errorf("expected domain %q to be gone after delete", name)
	}
}
