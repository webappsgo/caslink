package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/caslink/src/common/crypto"
	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service/extauth"
)

// withEncryptionKey installs a real AES-256-GCM key so persistAuth can encrypt
// stored secrets (DefaultConfig leaves the key empty).
func withEncryptionKey(t *testing.T, h *AdminHandler) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	h.cfg.Server.Security.EncryptionKey = key
}

// newAuthReq builds a request carrying chi URL params (and optional body) so a
// handler method can be exercised directly without the router.
func newAuthReq(method, target string, params map[string]string, body string) *http.Request {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, target, rd)
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// mockOIDCDisc is an injectable OIDCDiscoverer for the "Test" action.
type mockOIDCDisc struct{ err error }

func (m mockOIDCDisc) Discover(_ context.Context, issuer string) (extauth.OIDCEndpoints, error) {
	if m.err != nil {
		return extauth.OIDCEndpoints{}, m.err
	}
	return extauth.OIDCEndpoints{Issuer: issuer, AuthorizationEndpoint: "https://idp/auth", TokenEndpoint: "https://idp/token"}, nil
}

func TestAuthProviderAPICRUDRoundTrip(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	withEncryptionKey(t, h)

	// Create.
	body := `{"name":"keycloak","display_name":"KC","issuer":"https://idp.example.com","client_id":"abc","client_secret":"topsecret"}`
	w := httptest.NewRecorder()
	h.APIAuthProviderCreate(w, newAuthReq(http.MethodPost, "/x", map[string]string{"type": "oidc"}, body))
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(h.cfg.Server.Auth.OIDC.Providers) != 1 {
		t.Fatalf("expected 1 provider stored, got %d", len(h.cfg.Server.Auth.OIDC.Providers))
	}
	// Secret must be encrypted at rest, never plaintext.
	stored := h.cfg.Server.Auth.OIDC.Providers[0].ClientSecret
	if !strings.HasPrefix(stored, "enc:") {
		t.Fatalf("stored secret not encrypted: %q", stored)
	}
	// PKCE/state/nonce must be forced on.
	p := h.cfg.Server.Auth.OIDC.Providers[0]
	if !p.PKCE || !p.UseState || !p.UseNonce {
		t.Fatalf("secure flags not forced: pkce=%v state=%v nonce=%v", p.PKCE, p.UseState, p.UseNonce)
	}

	// List returns masked secret, never plaintext.
	w = httptest.NewRecorder()
	h.APIAuthProvidersList(w, newAuthReq(http.MethodGet, "/x", map[string]string{"type": "oidc"}, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "topsecret") {
		t.Fatalf("list leaked plaintext secret: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), maskedSecret) {
		t.Fatalf("list did not mask secret: %s", w.Body.String())
	}

	// Get single.
	w = httptest.NewRecorder()
	h.APIAuthProviderGet(w, newAuthReq(http.MethodGet, "/x", map[string]string{"type": "oidc", "provider": "keycloak"}, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// Update without a secret keeps the stored one.
	upd := `{"display_name":"KC2","issuer":"https://idp.example.com","client_id":"abc"}`
	w = httptest.NewRecorder()
	h.APIAuthProviderUpdate(w, newAuthReq(http.MethodPatch, "/x", map[string]string{"type": "oidc", "provider": "keycloak"}, upd))
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if h.cfg.Server.Auth.OIDC.Providers[0].DisplayName != "KC2" {
		t.Fatalf("update did not apply display name")
	}
	key, _ := crypto.DecodeKey(h.cfg.Server.Security.EncryptionKey)
	dec, err := config.DecryptSecret(key, h.cfg.Server.Auth.OIDC.Providers[0].ClientSecret)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != "topsecret" {
		t.Fatalf("update did not preserve secret, got %q", dec)
	}

	// Delete.
	w = httptest.NewRecorder()
	h.APIAuthProviderDelete(w, newAuthReq(http.MethodDelete, "/x", map[string]string{"type": "oidc", "provider": "keycloak"}, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", w.Code)
	}
	if len(h.cfg.Server.Auth.OIDC.Providers) != 0 {
		t.Fatalf("delete did not remove provider")
	}
}

func TestAuthProviderAPIDuplicateConflict(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	withEncryptionKey(t, h)
	body := `{"name":"kc","issuer":"https://idp.example.com","client_id":"abc"}`
	h.APIAuthProviderCreate(httptest.NewRecorder(), newAuthReq(http.MethodPost, "/x", map[string]string{"type": "oidc"}, body))

	w := httptest.NewRecorder()
	h.APIAuthProviderCreate(w, newAuthReq(http.MethodPost, "/x", map[string]string{"type": "oidc"}, body))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if _, _, code := decodeEnvelope(t, w.Body.Bytes()); code != "CONFLICT" {
		t.Fatalf("expected error CONFLICT, got %q", code)
	}
}

func TestAuthProviderAPIValidationError(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	withEncryptionKey(t, h)
	// Missing issuer.
	body := `{"name":"kc","client_id":"abc"}`
	w := httptest.NewRecorder()
	h.APIAuthProviderCreate(w, newAuthReq(http.MethodPost, "/x", map[string]string{"type": "oidc"}, body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if _, _, code := decodeEnvelope(t, w.Body.Bytes()); code != "VALIDATION_FAILED" {
		t.Fatalf("expected VALIDATION_FAILED, got %q", code)
	}
}

func TestAuthProviderAPINotFound(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	withEncryptionKey(t, h)
	w := httptest.NewRecorder()
	h.APIAuthProviderGet(w, newAuthReq(http.MethodGet, "/x", map[string]string{"type": "oidc", "provider": "nope"}, ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.APIAuthProviderDelete(w, newAuthReq(http.MethodDelete, "/x", map[string]string{"type": "oidc", "provider": "nope"}, ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("delete unknown: expected 404, got %d", w.Code)
	}
}

func TestAuthProviderTestActionSuccessAndFailure(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	withEncryptionKey(t, h)
	body := `{"name":"kc","issuer":"https://idp.example.com","client_id":"abc"}`
	h.APIAuthProviderCreate(httptest.NewRecorder(), newAuthReq(http.MethodPost, "/x", map[string]string{"type": "oidc"}, body))

	// Success via injected mock.
	h.authTesters.OIDC = mockOIDCDisc{}
	w := httptest.NewRecorder()
	h.APIAuthProviderTest(w, newAuthReq(http.MethodPost, "/x", map[string]string{"type": "oidc", "provider": "kc"}, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("test success: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Failure via injected mock returns 502 TEST_FAILED.
	h.authTesters.OIDC = mockOIDCDisc{err: errors.New("unreachable")}
	w = httptest.NewRecorder()
	h.APIAuthProviderTest(w, newAuthReq(http.MethodPost, "/x", map[string]string{"type": "oidc", "provider": "kc"}, ""))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("test failure: expected 502, got %d: %s", w.Code, w.Body.String())
	}
	if _, _, code := decodeEnvelope(t, w.Body.Bytes()); code != "TEST_FAILED" {
		t.Fatalf("expected TEST_FAILED, got %q", code)
	}
}

func TestAuthProviderLDAPRoundTrip(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	withEncryptionKey(t, h)
	body := `{"name":"corp","server":"ldaps://ldap.example.com","base_dn":"dc=example,dc=com","user_filter":"(uid=%s)","bind_password":"binderpw","tls_mode":"ldaps"}`
	w := httptest.NewRecorder()
	h.APIAuthProviderCreate(w, newAuthReq(http.MethodPost, "/x", map[string]string{"type": "ldap"}, body))
	if w.Code != http.StatusOK {
		t.Fatalf("ldap create: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.HasPrefix(h.cfg.Server.Auth.LDAP.Providers[0].BindPassword, "enc:") {
		t.Fatalf("ldap bind password not encrypted")
	}
}

func TestAuthProviderSAMLRoundTrip(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	withEncryptionKey(t, h)
	body := `{"name":"okta","idp_metadata_url":"https://idp.example.com/metadata","auto_generate_cert":true}`
	w := httptest.NewRecorder()
	h.APIAuthProviderCreate(w, newAuthReq(http.MethodPost, "/x", map[string]string{"type": "saml"}, body))
	if w.Code != http.StatusOK {
		t.Fatalf("saml create: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(h.cfg.Server.Auth.SAML.Providers) != 1 {
		t.Fatalf("expected 1 saml provider")
	}
}

func TestAuthProviderWebCreateAndDuplicate(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)
	withEncryptionKey(t, h)

	form := url.Values{
		"action":    {"save"},
		"orig":      {""},
		"name":      {"kc"},
		"issuer":    {"https://idp.example.com"},
		"client_id": {"abc"},
	}
	newForm := func() *http.Request {
		r := newAuthReq(http.MethodPost, "/server/admin/config/security/auth/oidc", map[string]string{"type": "oidc"}, form.Encode())
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}

	w := httptest.NewRecorder()
	h.ConfigAuthProvidersAction(w, newForm())
	if w.Code != http.StatusSeeOther {
		t.Fatalf("web create: expected 303, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "saved=created") {
		t.Fatalf("web create: expected saved=created, got %q", loc)
	}

	// Duplicate name is refused with an error redirect.
	w = httptest.NewRecorder()
	h.ConfigAuthProvidersAction(w, newForm())
	if w.Code != http.StatusSeeOther {
		t.Fatalf("web dup: expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("web dup: expected err redirect, got %q", loc)
	}
	if n := len(h.cfg.Server.Auth.OIDC.Providers); n != 1 {
		t.Fatalf("web dup: expected 1 provider, got %d", n)
	}
}

func TestAuthProviderWebPageRenders(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	withEncryptionKey(t, h)
	cookie := seedAdminSession(t, h, authService)
	// Seed one provider.
	body := `{"name":"kc","issuer":"https://idp.example.com","client_id":"abc"}`
	h.APIAuthProviderCreate(httptest.NewRecorder(), newAuthReq(http.MethodPost, "/x", map[string]string{"type": "oidc"}, body))

	r := newAuthReq(http.MethodGet, "/server/admin/config/security/auth/oidc", map[string]string{"type": "oidc"}, "")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigAuthProviders(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("web page: expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "kc") {
		t.Fatalf("web page did not list provider")
	}
}
