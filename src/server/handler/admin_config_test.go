package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// configGETCase describes one Config*/AdminHelp GET handler under test.
type configGETCase struct {
	name    string
	path    string
	handler func(h *AdminHandler) http.HandlerFunc
}

func adminConfigGETCases() []configGETCase {
	return []configGETCase{
		{"ConfigSettings", "/server/admin/config/settings", func(h *AdminHandler) http.HandlerFunc { return h.ConfigSettings }},
		{"ConfigBranding", "/server/admin/config/branding", func(h *AdminHandler) http.HandlerFunc { return h.ConfigBranding }},
		{"ConfigSSL", "/server/admin/config/ssl", func(h *AdminHandler) http.HandlerFunc { return h.ConfigSSL }},
		{"ConfigScheduler", "/server/admin/config/scheduler", func(h *AdminHandler) http.HandlerFunc { return h.ConfigScheduler }},
		{"ConfigEmail", "/server/admin/config/email", func(h *AdminHandler) http.HandlerFunc { return h.ConfigEmail }},
		{"ConfigLogs", "/server/admin/config/logs", func(h *AdminHandler) http.HandlerFunc { return h.ConfigLogs }},
		{"ConfigLogsAudit", "/server/admin/config/logs/audit", func(h *AdminHandler) http.HandlerFunc { return h.ConfigLogsAudit }},
		{"ConfigBackup", "/server/admin/config/backup", func(h *AdminHandler) http.HandlerFunc { return h.ConfigBackup }},
		{"ConfigMaintenance", "/server/admin/config/maintenance", func(h *AdminHandler) http.HandlerFunc { return h.ConfigMaintenance }},
		{"ConfigUpdates", "/server/admin/config/updates", func(h *AdminHandler) http.HandlerFunc { return h.ConfigUpdates }},
		{"ConfigInfo", "/server/admin/config/info", func(h *AdminHandler) http.HandlerFunc { return h.ConfigInfo }},
		{"ConfigSecurityAuth", "/server/admin/config/security/auth", func(h *AdminHandler) http.HandlerFunc { return h.ConfigSecurityAuth }},
		{"ConfigSecurityTokens", "/server/admin/config/security/tokens", func(h *AdminHandler) http.HandlerFunc { return h.ConfigSecurityTokens }},
		{"ConfigSecurityRateLimit", "/server/admin/config/security/rate-limit", func(h *AdminHandler) http.HandlerFunc { return h.ConfigSecurityRateLimit }},
		{"ConfigSecurityFirewall", "/server/admin/config/security/firewall", func(h *AdminHandler) http.HandlerFunc { return h.ConfigSecurityFirewall }},
		{"ConfigSecurityAllowlist", "/server/admin/config/security/allowlist", func(h *AdminHandler) http.HandlerFunc { return h.ConfigSecurityAllowlist }},
		{"ConfigNetworkTor", "/server/admin/config/network/tor", func(h *AdminHandler) http.HandlerFunc { return h.ConfigNetworkTor }},
		{"ConfigNetworkGeoIP", "/server/admin/config/network/geoip", func(h *AdminHandler) http.HandlerFunc { return h.ConfigNetworkGeoIP }},
		{"ConfigNetworkBlocklists", "/server/admin/config/network/blocklists", func(h *AdminHandler) http.HandlerFunc { return h.ConfigNetworkBlocklists }},
		{"ConfigUsersInvites", "/server/admin/config/users/invites", func(h *AdminHandler) http.HandlerFunc { return h.ConfigUsersInvites }},
		{"ConfigModerationUsers", "/server/admin/config/moderation/users", func(h *AdminHandler) http.HandlerFunc { return h.ConfigModerationUsers }},
		{"ConfigClusterNodes", "/server/admin/config/cluster/nodes", func(h *AdminHandler) http.HandlerFunc { return h.ConfigClusterNodes }},
		{"ConfigClusterAdd", "/server/admin/config/cluster/add", func(h *AdminHandler) http.HandlerFunc { return h.ConfigClusterAdd }},
		{"AdminHelp", "/server/admin/help", func(h *AdminHandler) http.HandlerFunc { return h.AdminHelp }},
	}
}

func TestConfigGETHandlersUnauthenticatedRedirect(t *testing.T) {
	for _, tc := range adminConfigGETCases() {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := newAdminTestHandler(t)
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			tc.handler(h)(w, r)
			if w.Code != http.StatusFound {
				t.Fatalf("%s: expected 302 unauthenticated, got %d", tc.name, w.Code)
			}
		})
	}
}

func TestConfigGETHandlersAuthenticatedRender(t *testing.T) {
	for _, tc := range adminConfigGETCases() {
		t.Run(tc.name, func(t *testing.T) {
			h, authService, _ := newAdminTestHandler(t)
			cookie := seedAdminSession(t, h, authService)
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			r.AddCookie(cookie)
			w := httptest.NewRecorder()
			tc.handler(h)(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("%s: expected 200, got %d: %s", tc.name, w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "Caslink Admin") {
				t.Fatalf("%s: expected rendered admin layout, got: %s", tc.name, w.Body.String())
			}
		})
	}
}

func TestConfigSettingsSavePersistsAndRedirects(t *testing.T) {
	h, authService, st := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	form := url.Values{"address": {"0.0.0.0"}, "port": {"8443"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/settings", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigSettingsSave(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "saved=1") {
		t.Fatalf("expected redirect with saved=1, got %q", loc)
	}
	val, ok, err := st.GetConfigValue("server.port")
	if err != nil {
		t.Fatalf("GetConfigValue failed: %v", err)
	}
	if !ok || val != "8443" {
		t.Fatalf("expected server.port=8443 persisted, got %q (ok=%v)", val, ok)
	}
}

func TestConfigBrandingSavePersists(t *testing.T) {
	h, authService, st := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	form := url.Values{"site_name": {"MySite"}, "default_theme": {"dark"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/branding", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigBrandingSave(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	val, ok, _ := st.GetConfigValue("branding.site_name")
	if !ok || val != "MySite" {
		t.Fatalf("expected branding.site_name=MySite persisted, got %q (ok=%v)", val, ok)
	}
}

func TestConfigSecurityAuthSavePersists(t *testing.T) {
	h, authService, st := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	form := url.Values{"pwd_min_length": {"12"}, "totp_issuer": {"Caslink"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/security/auth", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigSecurityAuthSave(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	val, ok, _ := st.GetConfigValue("security.pwd_min_length")
	if !ok || val != "12" {
		t.Fatalf("expected security.pwd_min_length=12 persisted, got %q (ok=%v)", val, ok)
	}
}

func TestConfigNetworkGeoIPSavePersists(t *testing.T) {
	h, authService, st := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	form := url.Values{"dir": {"/data/geoip"}, "deny_countries": {"CN\nRU"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/network/geoip", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigNetworkGeoIPSave(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	val, ok, _ := st.GetConfigValue("geoip.dir")
	if !ok || val != "/data/geoip" {
		t.Fatalf("expected geoip.dir persisted, got %q (ok=%v)", val, ok)
	}
}

func TestConfigNetworkBlocklistsSavePersistsSourceEntry(t *testing.T) {
	h, authService, st := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	form := url.Values{"name": {"MySource"}, "url": {"https://example.com/list.txt"}, "type": {"ip"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/network/blocklists", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigNetworkBlocklistsSave(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	val, ok, _ := st.GetConfigValue("blocklist.source.mysource")
	if !ok || !strings.Contains(val, "MySource") {
		t.Fatalf("expected blocklist.source.mysource persisted with entry, got %q (ok=%v)", val, ok)
	}
}

func TestConfigSecurityRateLimitSavePersists(t *testing.T) {
	h, authService, st := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	form := url.Values{"rpm": {"200"}, "burst": {"400"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/security/rate-limit", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigSecurityRateLimitSave(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	val, ok, _ := st.GetConfigValue("rate_limit.rpm")
	if !ok || val != "200" {
		t.Fatalf("expected rate_limit.rpm=200 persisted, got %q (ok=%v)", val, ok)
	}
}

func TestConfigSecurityFirewallAndAllowlistSaveRedirect(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	form := url.Values{"blocked_ips": {"1.2.3.4"}, "blocked_countries": {"KP"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/security/firewall", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigSecurityFirewallSave(w, r)
	if w.Code != http.StatusFound {
		t.Fatalf("firewall save: expected 302, got %d", w.Code)
	}

	form2 := url.Values{"allowlist_ips": {"127.0.0.1"}}
	r2 := httptest.NewRequest(http.MethodPost, "/server/admin/config/security/allowlist", strings.NewReader(form2.Encode()))
	r2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	h.ConfigSecurityAllowlistSave(w2, r2)
	if w2.Code != http.StatusFound {
		t.Fatalf("allowlist save: expected 302, got %d", w2.Code)
	}
}

func TestConfigMaintenanceSaveAndUpdatesActionRedirect(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	form := url.Values{"enabled": {"true"}, "message": {"down for maintenance"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/maintenance", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigMaintenanceSave(w, r)
	if w.Code != http.StatusFound {
		t.Fatalf("maintenance save: expected 302, got %d", w.Code)
	}

	form2 := url.Values{"action": {"channel"}, "channel": {"beta"}}
	r2 := httptest.NewRequest(http.MethodPost, "/server/admin/config/updates", strings.NewReader(form2.Encode()))
	r2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	h.ConfigUpdatesAction(w2, r2)
	if w2.Code != http.StatusFound {
		t.Fatalf("updates action: expected 302, got %d", w2.Code)
	}
}

func TestConfigBackupActionRedirect(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/backup", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigBackupAction(w, r)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}
}

func TestConfigStubActionHandlersRedirectWithoutPersisting(t *testing.T) {
	// ConfigSecurityTokensAction, ConfigUsersInvitesAction, and
	// ConfigClusterAddAction are documented stubs: they parse the form and
	// redirect, but do not yet implement real token/invite/join-token
	// creation. This test locks in the observed (non-crashing) behavior.
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	cases := []struct {
		name    string
		path    string
		handler http.HandlerFunc
	}{
		{"ConfigSecurityTokensAction", "/server/admin/config/security/tokens", nil},
		{"ConfigUsersInvitesAction", "/server/admin/config/users/invites", nil},
		{"ConfigClusterAddAction", "/server/admin/config/cluster/add", nil},
	}
	cases[0].handler = h.ConfigSecurityTokensAction
	cases[1].handler = h.ConfigUsersInvitesAction
	cases[2].handler = h.ConfigClusterAddAction

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(""))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.AddCookie(cookie)
			w := httptest.NewRecorder()
			tc.handler(w, r)
			if w.Code != http.StatusFound {
				t.Fatalf("%s: expected 302, got %d", tc.name, w.Code)
			}
		})
	}
}

func TestConfigEmailSaveAndTestNoAdminEmail(t *testing.T) {
	h, authService, st := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	form := url.Values{"smtp_host": {"smtp.example.com"}, "smtp_port": {"587"}}
	r := httptest.NewRequest(http.MethodPost, "/server/admin/config/email", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigEmailSave(w, r)
	if w.Code != http.StatusFound {
		t.Fatalf("email save: expected 302, got %d", w.Code)
	}
	val, ok, _ := st.GetConfigValue("email.smtp_host")
	if !ok || val != "smtp.example.com" {
		t.Fatalf("expected email.smtp_host persisted, got %q (ok=%v)", val, ok)
	}

	// ConfigEmailTest with no admin contact email configured should not
	// attempt a real SMTP connection; it should redirect/render safely.
	r2 := httptest.NewRequest(http.MethodPost, "/server/admin/config/email/test", nil)
	r2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	h.ConfigEmailTest(w2, r2)
	if w2.Code != http.StatusFound && w2.Code != http.StatusOK {
		t.Fatalf("email test: expected 200 or 302, got %d: %s", w2.Code, w2.Body.String())
	}
}

// --- JSON API handlers ---

func TestAPIConfigReadEndpointsReturnOK(t *testing.T) {
	h, _, _ := newAdminTestHandler(t)

	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"APIConfigSettings", h.APIConfigSettings},
		{"APIConfigBranding", h.APIConfigBranding},
		{"APIConfigInfo", h.APIConfigInfo},
		{"APIConfigScheduler", h.APIConfigScheduler},
		{"APIConfigMaintenance", h.APIConfigMaintenance},
		{"APIConfigNetworkTor", h.APIConfigNetworkTor},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/server/admin/config/x", nil)
			w := httptest.NewRecorder()
			tc.handler(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("%s: expected 200, got %d: %s", tc.name, w.Code, w.Body.String())
			}
			if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
				t.Fatalf("%s: expected JSON content type, got %q", tc.name, ct)
			}
		})
	}
}

func TestAPIConfigSettingsSaveValidAndInvalidJSON(t *testing.T) {
	h, _, st := newAdminTestHandler(t)

	// Valid JSON body saves and returns ok:true.
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/server/admin/config/settings", strings.NewReader(`{"address":"1.2.3.4"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.APIConfigSettingsSave(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var ok struct {
		OK   bool `json:"ok"`
		Data struct {
			Saved bool `json:"saved"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ok); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !ok.OK || !ok.Data.Saved {
		t.Fatalf("expected ok:true, data.saved:true, got %s", w.Body.String())
	}
	val, found, _ := st.GetConfigValue("server.address")
	if !found || val != "1.2.3.4" {
		t.Fatalf("expected server.address=1.2.3.4 persisted, got %q (found=%v)", val, found)
	}

	// Invalid JSON body returns 400 INVALID_JSON.
	r2 := httptest.NewRequest(http.MethodPatch, "/api/v1/server/admin/config/settings", strings.NewReader(`{not-json`))
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.APIConfigSettingsSave(w2, r2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w2.Code, w2.Body.String())
	}
	var errBody struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if errBody.OK || errBody.Error != "INVALID_JSON" {
		t.Fatalf("expected ok:false error:INVALID_JSON, got %s", w2.Body.String())
	}
}

func TestAPIConfigBrandingSaveValidAndInvalidJSON(t *testing.T) {
	h, _, st := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/server/admin/config/branding", strings.NewReader(`{"site_name":"NewName"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.APIConfigBrandingSave(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	val, found, _ := st.GetConfigValue("branding.site_name")
	if !found || val != "NewName" {
		t.Fatalf("expected branding.site_name=NewName persisted, got %q (found=%v)", val, found)
	}

	r2 := httptest.NewRequest(http.MethodPatch, "/api/v1/server/admin/config/branding", strings.NewReader(`[[[`))
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.APIConfigBrandingSave(w2, r2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w2.Code)
	}
}

func TestAPIConfigMaintenanceSaveValidAndInvalidJSON(t *testing.T) {
	h, _, st := newAdminTestHandler(t)

	r := httptest.NewRequest(http.MethodPatch, "/api/v1/server/admin/config/maintenance", strings.NewReader(`{"enabled":"true","message":"brb"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.APIConfigMaintenanceSave(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	val, found, _ := st.GetConfigValue("maintenance.message")
	if !found || val != "brb" {
		t.Fatalf("expected maintenance.message=brb persisted, got %q (found=%v)", val, found)
	}

	r2 := httptest.NewRequest(http.MethodPatch, "/api/v1/server/admin/config/maintenance", strings.NewReader(`notjson`))
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.APIConfigMaintenanceSave(w2, r2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w2.Code)
	}
}

func TestConfigLogsAuditRendersSeededEvents(t *testing.T) {
	h, authService, _ := newAdminTestHandler(t)
	cookie := seedAdminSession(t, h, authService)

	if err := h.auditService.RecordEvent(t.Context(), nil, "admin", "test.event", "resource", "details here", "127.0.0.1", "test-agent"); err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/server/admin/config/logs/audit", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ConfigLogsAudit(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "test.event") {
		t.Fatalf("expected seeded audit event in body, got %q", w.Body.String())
	}
}
