package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/caslink/src/config"
)

// newTestServer builds a *Server with just enough state populated to
// exercise the pure/near-pure handler methods below, without going through
// New() (which requires a full runtime: DB, scheduler, Tor, etc. — out of
// scope for unit tests per AI.md PART 29).
func newTestServer(t *testing.T, cfg *config.Config) *Server {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &Server{
		config: cfg,
		store:  newSchemaTestStore(t),
	}
}

// TestIsPortAvailable verifies a port with an active listener reports
// unavailable, and reports available again once the listener closes.
func TestIsPortAvailable(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to acquire an ephemeral port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	if isPortAvailable(port) {
		t.Errorf("expected port %d to be unavailable while listener is open", port)
	}

	ln.Close()

	if !isPortAvailable(port) {
		t.Errorf("expected port %d to be available after listener closed", port)
	}
}

// TestSelectRandomPort verifies the returned port is either 0 (exhausted
// fallback) or within the documented 64580-64999 range, and is actually
// available at the moment it's returned.
func TestSelectRandomPort(t *testing.T) {
	got := selectRandomPort()
	if got == 0 {
		return
	}
	if got < 64580 || got >= 65000 {
		t.Errorf("selectRandomPort() = %d, want 0 or in [64580,65000)", got)
	}
	if !isPortAvailable(got) {
		t.Errorf("selectRandomPort() returned %d which is not available", got)
	}
}

// TestServerBaseURL covers the scheme/host resolution precedence: TLS
// presence, X-Forwarded-Proto, and X-Forwarded-Host.
func TestServerBaseURL(t *testing.T) {
	s := newTestServer(t, nil)

	tests := []struct {
		name    string
		setup   func(r *http.Request)
		host    string
		want    string
	}{
		{
			name: "plain http, no forwarding",
			setup: func(r *http.Request) {},
			host:  "example.com",
			want:  "http://example.com",
		},
		{
			name: "forwarded proto https",
			setup: func(r *http.Request) {
				r.Header.Set("X-Forwarded-Proto", "https")
			},
			host: "example.com",
			want: "https://example.com",
		},
		{
			name: "forwarded host overrides Host",
			setup: func(r *http.Request) {
				r.Header.Set("X-Forwarded-Host", "public.example.com")
			},
			host: "internal.example.com",
			want: "http://public.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Host = tt.host
			tt.setup(r)
			if got := s.baseURL(r); got != tt.want {
				t.Errorf("baseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestServerRobotsTxtDefaultAdminPath verifies the default "admin" segment
// is disallowed when Server.Admin.Path is unset.
func TestServerRobotsTxtDefaultAdminPath(t *testing.T) {
	s := newTestServer(t, &config.Config{})

	r := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()
	s.robotsTxt(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "Disallow: /server/admin") {
		t.Errorf("expected default admin path disallowed, body=%q", body)
	}
	if !strings.Contains(body, "Sitemap: http://example.com/sitemap.xml") {
		t.Errorf("expected sitemap reference, body=%q", body)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix", ct)
	}
}

// TestServerRobotsTxtCustomAdminPath verifies a configured admin path is
// reflected in the Disallow line instead of the default.
func TestServerRobotsTxtCustomAdminPath(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Admin.Path = "secret-panel"
	s := newTestServer(t, cfg)

	r := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()
	s.robotsTxt(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "Disallow: /server/secret-panel") {
		t.Errorf("expected custom admin path disallowed, body=%q", body)
	}
	if strings.Contains(body, "/server/admin") {
		t.Errorf("did not expect default admin path in body=%q", body)
	}
}

// TestServerSitemapXML verifies the sitemap is well-formed XML rooted at
// the resolved base URL, and includes the expected static pages.
func TestServerSitemapXML(t *testing.T) {
	s := newTestServer(t, &config.Config{})

	r := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()
	s.sitemapXML(w, r)

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("Content-Type = %q, want application/xml prefix", ct)
	}
	body := w.Body.String()
	for _, want := range []string{
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>",
		"<loc>http://example.com/</loc>",
		"<loc>http://example.com/server/about</loc>",
		"<loc>http://example.com/server/help</loc>",
		"<loc>http://example.com/server/privacy</loc>",
		"<loc>http://example.com/server/terms</loc>",
		"<loc>http://example.com/server/docs/swagger</loc>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sitemap missing %q, body=%q", want, body)
		}
	}
}

// TestServerWellKnownSecurityTxtDefaultContact verifies the fallback
// admin@{fqdn} contact is used when Server.Admin.Email is unset, and the
// scheme reflects SSL.Enabled.
func TestServerWellKnownSecurityTxtDefaultContact(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.FQDN = "example.com"
	s := newTestServer(t, cfg)

	r := httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil)
	w := httptest.NewRecorder()
	s.wellKnownSecurityTxt(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "Contact: mailto:admin@example.com") {
		t.Errorf("expected default contact, body=%q", body)
	}
	if !strings.Contains(body, "Policy: http://example.com/.well-known/security.txt") {
		t.Errorf("expected http scheme with SSL disabled, body=%q", body)
	}
}

// TestServerWellKnownSecurityTxtConfiguredContactAndSSL verifies a
// configured admin email is used verbatim and the scheme switches to https
// when SSL is enabled.
func TestServerWellKnownSecurityTxtConfiguredContactAndSSL(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.FQDN = "example.com"
	cfg.Server.Admin.Email = "security@example.com"
	cfg.Server.SSL.Enabled = true
	s := newTestServer(t, cfg)

	r := httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil)
	w := httptest.NewRecorder()
	s.wellKnownSecurityTxt(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "Contact: mailto:security@example.com") {
		t.Errorf("expected configured contact, body=%q", body)
	}
	if !strings.Contains(body, "Policy: https://example.com/.well-known/security.txt") {
		t.Errorf("expected https scheme with SSL enabled, body=%q", body)
	}
}

// TestServerWellKnownACMEChallengeNilManagerReturns404 verifies the
// handler falls back to 404 (so ACME can fall back to DNS-01) when no
// autocert.Manager is active.
func TestServerWellKnownACMEChallengeNilManagerReturns404(t *testing.T) {
	s := newTestServer(t, nil)

	r := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/token123", nil)
	w := httptest.NewRecorder()
	s.wellKnownACMEChallenge(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestServerHandleRootRedirectsToSetupWhenNoAdmin verifies a fresh store
// (no Server Admin created yet) redirects to /setup.
func TestServerHandleRootRedirectsToSetupWhenNoAdmin(t *testing.T) {
	s := newTestServer(t, nil)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.handleRoot(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/setup" {
		t.Errorf("Location = %q, want /setup", loc)
	}
}

// TestServerWellKnownChangePasswordNoSessionRedirectsToForgot verifies a
// request with no (or an invalid) user_session cookie is sent to the
// password-reset flow rather than the authenticated change-password page.
func TestServerWellKnownChangePasswordNoSessionRedirectsToForgot(t *testing.T) {
	s := newTestServer(t, nil)

	r := httptest.NewRequest(http.MethodGet, "/.well-known/change-password", nil)
	w := httptest.NewRecorder()
	s.wellKnownChangePassword(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/server/auth/password/forgot" {
		t.Errorf("Location = %q, want /server/auth/password/forgot", loc)
	}
}

// TestServerWellKnownChangePasswordInvalidCookieRedirectsToForgot verifies
// a present-but-invalid session cookie is treated the same as no cookie.
func TestServerWellKnownChangePasswordInvalidCookieRedirectsToForgot(t *testing.T) {
	s := newTestServer(t, nil)

	r := httptest.NewRequest(http.MethodGet, "/.well-known/change-password", nil)
	r.AddCookie(&http.Cookie{Name: "user_session", Value: "does-not-exist"})
	w := httptest.NewRecorder()
	s.wellKnownChangePassword(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/server/auth/password/forgot" {
		t.Errorf("Location = %q, want /server/auth/password/forgot", loc)
	}
}
