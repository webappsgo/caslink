package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/caslink/src/common/crypto"
	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service/extauth"
)

// External identity-provider (OIDC/LDAP/SAML) configuration management for the
// admin panel per AI.md PART 34 "External Identity Provider Requirements".
//
// This is the CONFIG + ADMIN MANAGEMENT layer only: it lets a Server Admin
// create, edit, delete, and test-connectivity of named providers under
// /server/{admin_path}/config/security/auth/{oidc,ldap,saml}. Browser login
// flows (authorization-code exchange, LDAP bind/search, SAML ACS/SLO) are a
// separate later feature and are deliberately NOT implemented here.
//
// Reversible secrets (OIDC client_secret, LDAP bind_password, SAML SP private
// key) are encrypted at rest via AuthConfig.EncryptSecrets and never returned
// decrypted or logged; API/UI output is masked.

// maskedSecret is the placeholder shown for a stored secret in edit forms and
// API output. A submitted value equal to this (or empty) means "keep existing".
const maskedSecret = "********"

// errAuthDuplicate/errAuthNotFound are sentinels the API layer maps to
// CONFLICT/NOT_FOUND; validation errors flow through unwrapped as 400.
var (
	errAuthDuplicate = errors.New("a provider with that name already exists")
	errAuthNotFound  = errors.New("provider not found")
)

// validAuthType returns the canonical provider type or false.
func validAuthType(s string) (string, bool) {
	switch s {
	case "oidc", "ldap", "saml":
		return s, true
	}
	return "", false
}

// splitCommaList parses a comma-separated form value into a trimmed slice.
func splitCommaList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// authKey decodes the at-rest AES-256-GCM encryption key from config.
func (h *AdminHandler) authKey() ([]byte, error) {
	return crypto.DecodeKey(h.cfg.Server.Security.EncryptionKey)
}

// persistAuth encrypts any plaintext secrets and writes server.yml. It is safe
// to call repeatedly: EncryptSecrets skips already-encrypted values.
func (h *AdminHandler) persistAuth() error {
	key, err := h.authKey()
	if err != nil {
		return fmt.Errorf("encryption key unavailable: %w", err)
	}
	if err := h.cfg.Server.Auth.EncryptSecrets(key); err != nil {
		return err
	}
	return config.Save(h.configDir, h.cfg)
}

// --------------------------------------------------------------------------
// Upsert / delete core (shared by web + API)
// --------------------------------------------------------------------------

// upsertOIDC creates (orig=="") or updates an OIDC provider, forcing the
// non-negotiable PKCE/state/nonce security flags, keeping an unchanged secret,
// validating, and persisting.
func (h *AdminHandler) upsertOIDC(orig string, p config.OIDCProvider) error {
	p.PKCE, p.UseState, p.UseNonce = true, true, true
	p.Normalize()
	sec := &h.cfg.Server.Auth.OIDC
	if orig == "" {
		if sec.Index(p.Name) != -1 {
			return errAuthDuplicate
		}
		if err := p.Validate(); err != nil {
			return err
		}
		sec.Providers = append(sec.Providers, p)
		return h.persistAuth()
	}
	idx := sec.Index(orig)
	if idx == -1 {
		return errAuthNotFound
	}
	if p.Name != orig && sec.Index(p.Name) != -1 {
		return errAuthDuplicate
	}
	if p.ClientSecret == "" || p.ClientSecret == maskedSecret {
		p.ClientSecret = sec.Providers[idx].ClientSecret
	}
	if err := p.Validate(); err != nil {
		return err
	}
	sec.Providers[idx] = p
	return h.persistAuth()
}

// upsertLDAP creates or updates an LDAP provider.
func (h *AdminHandler) upsertLDAP(orig string, p config.LDAPProvider) error {
	p.Normalize()
	sec := &h.cfg.Server.Auth.LDAP
	if orig == "" {
		if sec.Index(p.Name) != -1 {
			return errAuthDuplicate
		}
		if err := p.Validate(); err != nil {
			return err
		}
		sec.Providers = append(sec.Providers, p)
		return h.persistAuth()
	}
	idx := sec.Index(orig)
	if idx == -1 {
		return errAuthNotFound
	}
	if p.Name != orig && sec.Index(p.Name) != -1 {
		return errAuthDuplicate
	}
	if p.BindPassword == "" || p.BindPassword == maskedSecret {
		p.BindPassword = sec.Providers[idx].BindPassword
	}
	if err := p.Validate(); err != nil {
		return err
	}
	sec.Providers[idx] = p
	return h.persistAuth()
}

// upsertSAML creates or updates a SAML provider.
func (h *AdminHandler) upsertSAML(orig string, p config.SAMLProvider) error {
	p.Normalize()
	sec := &h.cfg.Server.Auth.SAML
	if orig == "" {
		if sec.Index(p.Name) != -1 {
			return errAuthDuplicate
		}
		if err := p.Validate(); err != nil {
			return err
		}
		sec.Providers = append(sec.Providers, p)
		return h.persistAuth()
	}
	idx := sec.Index(orig)
	if idx == -1 {
		return errAuthNotFound
	}
	if p.Name != orig && sec.Index(p.Name) != -1 {
		return errAuthDuplicate
	}
	if p.SPPrivateKey == "" || p.SPPrivateKey == maskedSecret {
		p.SPPrivateKey = sec.Providers[idx].SPPrivateKey
	}
	if err := p.Validate(); err != nil {
		return err
	}
	sec.Providers[idx] = p
	return h.persistAuth()
}

// deleteAuthProvider removes a provider by name for the given type.
func (h *AdminHandler) deleteAuthProvider(typ, name string) error {
	switch typ {
	case "oidc":
		sec := &h.cfg.Server.Auth.OIDC
		idx := sec.Index(name)
		if idx == -1 {
			return errAuthNotFound
		}
		sec.Providers = append(sec.Providers[:idx], sec.Providers[idx+1:]...)
	case "ldap":
		sec := &h.cfg.Server.Auth.LDAP
		idx := sec.Index(name)
		if idx == -1 {
			return errAuthNotFound
		}
		sec.Providers = append(sec.Providers[:idx], sec.Providers[idx+1:]...)
	case "saml":
		sec := &h.cfg.Server.Auth.SAML
		idx := sec.Index(name)
		if idx == -1 {
			return errAuthNotFound
		}
		sec.Providers = append(sec.Providers[:idx], sec.Providers[idx+1:]...)
	default:
		return errAuthNotFound
	}
	return h.persistAuth()
}

// testAuthProvider runs the connectivity test for a stored provider. The result
// map is JSON-safe (no secrets). It never needs a decrypted secret: OIDC uses
// the issuer, LDAP dials the server, SAML fetches/parses metadata.
func (h *AdminHandler) testAuthProvider(ctx context.Context, typ, name string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	switch typ {
	case "oidc":
		idx := h.cfg.Server.Auth.OIDC.Index(name)
		if idx == -1 {
			return nil, errAuthNotFound
		}
		ep, err := h.authTesters.OIDC.Discover(ctx, h.cfg.Server.Auth.OIDC.Providers[idx].Issuer)
		if err != nil {
			return nil, err
		}
		return map[string]any{"endpoints": ep}, nil
	case "ldap":
		idx := h.cfg.Server.Auth.LDAP.Index(name)
		if idx == -1 {
			return nil, errAuthNotFound
		}
		if err := h.authTesters.LDAP.TestBind(ctx, h.cfg.Server.Auth.LDAP.Providers[idx]); err != nil {
			return nil, err
		}
		return map[string]any{"reachable": true}, nil
	case "saml":
		idx := h.cfg.Server.Auth.SAML.Index(name)
		if idx == -1 {
			return nil, errAuthNotFound
		}
		p := h.cfg.Server.Auth.SAML.Providers[idx]
		if strings.TrimSpace(p.IDPMetadataURL) != "" {
			md, err := h.authTesters.SAML.Fetch(ctx, p.IDPMetadataURL)
			if err != nil {
				return nil, err
			}
			return map[string]any{"metadata": md}, nil
		}
		md, err := extauth.ParseSAMLMetadata([]byte(p.IDPMetadataXML))
		if err != nil {
			return nil, err
		}
		return map[string]any{"metadata": md}, nil
	}
	return nil, errAuthNotFound
}

// --------------------------------------------------------------------------
// Web pages
// --------------------------------------------------------------------------

// ConfigAuthProviders handles GET /server/{adminPath}/config/security/auth/{type}
func (h *AdminHandler) ConfigAuthProviders(w http.ResponseWriter, r *http.Request) {
	typ, ok := validAuthType(chi.URLParam(r, "type"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	flash, errMsg := authFlash(r.URL.Query())
	edit := strings.TrimSpace(r.URL.Query().Get("edit"))

	var b strings.Builder
	fmt.Fprintf(&b, "<h1>%s Providers</h1>", strings.ToUpper(typ))
	b.WriteString(h.authSectionToggle(r, typ))
	b.WriteString(h.authProviderTable(r, typ))
	b.WriteString(h.authProviderForm(r, typ, edit))

	h.adminLayout(w, r, strings.ToUpper(typ)+" Providers",
		"/config/security/auth/"+typ, template.HTML(b.String()), flash, errMsg)
}

// authFlash maps ?saved=/?err= query tokens to messages.
func authFlash(q url.Values) (flash, errMsg string) {
	switch q.Get("saved") {
	case "created":
		return "Provider created.", ""
	case "updated":
		return "Provider updated.", ""
	case "deleted":
		return "Provider deleted.", ""
	case "section":
		return "Section updated.", ""
	}
	if q.Get("test") == "ok" {
		return "Connection test succeeded.", ""
	}
	if m := q.Get("err"); m != "" {
		return "", m
	}
	return "", ""
}

// authSectionToggle renders the enable/disable form for the whole provider
// section (e.g. server.auth.oidc.enabled).
func (h *AdminHandler) authSectionToggle(r *http.Request, typ string) string {
	var enabled bool
	switch typ {
	case "oidc":
		enabled = h.cfg.Server.Auth.OIDC.Enabled
	case "ldap":
		enabled = h.cfg.Server.Auth.LDAP.Enabled
	case "saml":
		enabled = h.cfg.Server.Auth.SAML.Enabled
	}
	csrf := template.HTMLEscapeString(csrfToken(r))
	action := template.HTMLEscapeString(h.basePath() + "/config/security/auth/" + typ)
	label, next := "Enable", "on"
	badge := `<span class="badge badge-red">disabled</span>`
	if enabled {
		label, next = "Disable", "off"
		badge = `<span class="badge badge-green">enabled</span>`
	}
	return fmt.Sprintf(`<div class="card"><p>Section status: %s</p>`+
		`<form method="POST" action="%s">`+
		`<input type="hidden" name="_csrf" value="%s">`+
		`<input type="hidden" name="action" value="toggle_section">`+
		`<input type="hidden" name="enabled" value="%s">`+
		`<button type="submit" class="btn btn-secondary">%s Section</button></form></div>`,
		badge, action, csrf, next, label)
}

// authProviderTable lists existing providers with edit/test/delete controls.
func (h *AdminHandler) authProviderTable(r *http.Request, typ string) string {
	csrf := template.HTMLEscapeString(csrfToken(r))
	base := template.HTMLEscapeString(h.basePath() + "/config/security/auth/" + typ)

	type row struct{ name, display, detail string }
	var rows []row
	switch typ {
	case "oidc":
		for _, p := range h.cfg.Server.Auth.OIDC.Providers {
			rows = append(rows, row{p.Name, p.DisplayName, p.Issuer})
		}
	case "ldap":
		for _, p := range h.cfg.Server.Auth.LDAP.Providers {
			rows = append(rows, row{p.Name, p.DisplayName, p.Server})
		}
	case "saml":
		for _, p := range h.cfg.Server.Auth.SAML.Providers {
			d := p.IDPMetadataURL
			if d == "" {
				d = "(inline metadata)"
			}
			rows = append(rows, row{p.Name, p.DisplayName, d})
		}
	}

	if len(rows) == 0 {
		return `<div class="card"><p>No providers configured yet.</p></div>`
	}

	var b strings.Builder
	b.WriteString(`<div class="card"><table class="table"><thead><tr>` +
		`<th>Name</th><th>Display</th><th>Detail</th><th>Actions</th></tr></thead><tbody>`)
	for _, rw := range rows {
		en := template.HTMLEscapeString(rw.name)
		editURL := base + "?edit=" + template.HTMLEscapeString(url.QueryEscape(rw.name))
		actions := fmt.Sprintf(
			`<a class="btn btn-secondary" href="%s">Edit</a> `+
				`<form method="POST" action="%s" style="display:inline">`+
				`<input type="hidden" name="_csrf" value="%s">`+
				`<input type="hidden" name="action" value="test">`+
				`<input type="hidden" name="name" value="%s">`+
				`<button type="submit" class="btn btn-secondary">Test</button></form> `+
				`<form method="POST" action="%s" style="display:inline">`+
				`<input type="hidden" name="_csrf" value="%s">`+
				`<input type="hidden" name="action" value="delete">`+
				`<input type="hidden" name="name" value="%s">`+
				`<button type="submit" class="btn btn-danger">Delete</button></form>`,
			editURL, base, csrf, en, base, csrf, en)
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td style="font-family:monospace">%s</td><td style="white-space:nowrap">%s</td></tr>`,
			en, template.HTMLEscapeString(rw.display), template.HTMLEscapeString(rw.detail), actions)
	}
	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

// textRow renders a labelled text input for the provider form.
func textRow(label, name, value, placeholder string) string {
	return fmt.Sprintf(`<div class="form-group"><label for="f_%s">%s</label>`+
		`<input type="text" id="f_%s" name="%s" value="%s" placeholder="%s"></div>`,
		name, template.HTMLEscapeString(label), name, name,
		template.HTMLEscapeString(value), template.HTMLEscapeString(placeholder))
}

// checkRow renders a labelled checkbox for the provider form.
func checkRow(label, name string, checked bool) string {
	c := ""
	if checked {
		c = " checked"
	}
	return fmt.Sprintf(`<div class="form-group"><label>`+
		`<input type="checkbox" name="%s" value="1"%s> %s</label></div>`,
		name, c, template.HTMLEscapeString(label))
}

// authProviderForm renders the add-or-edit form for a provider type. When edit
// names an existing provider its values are pre-filled and secrets are masked.
func (h *AdminHandler) authProviderForm(r *http.Request, typ, edit string) string {
	csrf := template.HTMLEscapeString(csrfToken(r))
	action := template.HTMLEscapeString(h.basePath() + "/config/security/auth/" + typ)

	var fields strings.Builder
	title := "Add Provider"
	orig := ""

	switch typ {
	case "oidc":
		var p config.OIDCProvider
		if idx := h.cfg.Server.Auth.OIDC.Index(edit); edit != "" && idx != -1 {
			p, orig, title = h.cfg.Server.Auth.OIDC.Providers[idx], edit, "Edit Provider"
		}
		secVal := ""
		if p.ClientSecret != "" {
			secVal = maskedSecret
		}
		fields.WriteString(textRow("Name (slug)", "name", p.Name, "keycloak"))
		fields.WriteString(textRow("Display Name", "display_name", p.DisplayName, "Company SSO"))
		fields.WriteString(textRow("Issuer URL", "issuer", p.Issuer, "https://idp.example.com/realms/main"))
		fields.WriteString(textRow("Client ID", "client_id", p.ClientID, ""))
		fields.WriteString(textRow("Client Secret", "client_secret", secVal, "leave blank to keep"))
		fields.WriteString(textRow("Scopes (comma)", "scopes", strings.Join(p.Scopes, ", "), "openid, profile, email"))
		fields.WriteString(textRow("Admin Groups (comma)", "admin_groups", strings.Join(p.AdminGroups, ", "), ""))
		fields.WriteString(textRow("Claim: username", "claim_username", p.ClaimsMapping.Username, "preferred_username"))
		fields.WriteString(textRow("Claim: email", "claim_email", p.ClaimsMapping.Email, "email"))
		fields.WriteString(textRow("Claim: name", "claim_name", p.ClaimsMapping.Name, "name"))
		fields.WriteString(textRow("Claim: groups", "claim_groups", p.ClaimsMapping.Groups, "groups"))
		fields.WriteString(checkRow("Auto-register users on first login", "auto_register", p.AutoRegister))
		fields.WriteString(`<p class="muted">PKCE (S256), state, and nonce are always enforced.</p>`)
	case "ldap":
		var p config.LDAPProvider
		if idx := h.cfg.Server.Auth.LDAP.Index(edit); edit != "" && idx != -1 {
			p, orig, title = h.cfg.Server.Auth.LDAP.Providers[idx], edit, "Edit Provider"
		}
		secVal := ""
		if p.BindPassword != "" {
			secVal = maskedSecret
		}
		fields.WriteString(textRow("Name (slug)", "name", p.Name, "corp"))
		fields.WriteString(textRow("Display Name", "display_name", p.DisplayName, "Corp Directory"))
		fields.WriteString(textRow("Server URL", "server", p.Server, "ldaps://ldap.example.com"))
		fields.WriteString(textRow("Bind DN", "bind_dn", p.BindDN, "cn=svc,dc=example,dc=com"))
		fields.WriteString(textRow("Bind Password", "bind_password", secVal, "leave blank to keep"))
		fields.WriteString(textRow("Base DN", "base_dn", p.BaseDN, "dc=example,dc=com"))
		fields.WriteString(textRow("User Filter", "user_filter", p.UserFilter, "(uid=%s)"))
		fields.WriteString(textRow("TLS Mode (ldaps|starttls|none)", "tls_mode", p.TLSMode, "starttls"))
		fields.WriteString(textRow("Admin Groups (comma)", "admin_groups", strings.Join(p.AdminGroups, ", "), ""))
		fields.WriteString(textRow("Attr: username", "attr_username", p.Attributes.Username, "uid"))
		fields.WriteString(textRow("Attr: email", "attr_email", p.Attributes.Email, "mail"))
		fields.WriteString(textRow("Attr: name", "attr_name", p.Attributes.Name, "cn"))
		fields.WriteString(textRow("Attr: groups", "attr_groups", p.Attributes.Groups, "memberOf"))
		fields.WriteString(`<input type="hidden" name="tls_verify_present" value="1">`)
		fields.WriteString(checkRow("Verify TLS certificate", "tls_verify", p.EffectiveTLSVerify()))
		fields.WriteString(checkRow("Auto-register users on first login", "auto_register", p.AutoRegister))
	case "saml":
		var p config.SAMLProvider
		if idx := h.cfg.Server.Auth.SAML.Index(edit); edit != "" && idx != -1 {
			p, orig, title = h.cfg.Server.Auth.SAML.Providers[idx], edit, "Edit Provider"
		}
		fields.WriteString(textRow("Name (slug)", "name", p.Name, "okta"))
		fields.WriteString(textRow("Display Name", "display_name", p.DisplayName, "Okta SSO"))
		fields.WriteString(textRow("IdP Metadata URL", "idp_metadata_url", p.IDPMetadataURL, "https://idp.example.com/metadata"))
		fields.WriteString(fmt.Sprintf(`<div class="form-group"><label for="f_idp_metadata_xml">IdP Metadata XML (alternative to URL)</label>`+
			`<textarea id="f_idp_metadata_xml" name="idp_metadata_xml" rows="4">%s</textarea></div>`,
			template.HTMLEscapeString(p.IDPMetadataXML)))
		fields.WriteString(textRow("SP Entity ID", "sp_entity_id", p.SPEntityID, ""))
		fields.WriteString(textRow("ACS URL", "acs_url", p.ACSURL, ""))
		fields.WriteString(textRow("Admin Groups (comma)", "admin_groups", strings.Join(p.AdminGroups, ", "), ""))
		fields.WriteString(textRow("Attr: username", "attr_username", p.AttributeMapping.Username, ""))
		fields.WriteString(textRow("Attr: email", "attr_email", p.AttributeMapping.Email, ""))
		fields.WriteString(textRow("Attr: name", "attr_name", p.AttributeMapping.Name, ""))
		fields.WriteString(textRow("Attr: groups", "attr_groups", p.AttributeMapping.Groups, "groups"))
		fields.WriteString(textRow("SP Cert Path (manual cert)", "sp_cert_path", p.SPCertPath, ""))
		fields.WriteString(textRow("SP Key Path (manual cert)", "sp_key_path", p.SPKeyPath, ""))
		fields.WriteString(checkRow("Auto-generate SP certificate", "auto_generate_cert", p.AutoGenerateCert))
		fields.WriteString(checkRow("Auto-register users on first login", "auto_register", p.AutoRegister))
	}

	return fmt.Sprintf(`<div class="card"><h2>%s</h2>`+
		`<form method="POST" action="%s">`+
		`<input type="hidden" name="_csrf" value="%s">`+
		`<input type="hidden" name="action" value="save">`+
		`<input type="hidden" name="orig" value="%s">`+
		`%s`+
		`<button type="submit" class="btn btn-primary">Save Provider</button></form></div>`,
		template.HTMLEscapeString(title), action, csrf,
		template.HTMLEscapeString(orig), fields.String())
}

// ConfigAuthProvidersAction handles POST /server/{adminPath}/config/security/auth/{type}
func (h *AdminHandler) ConfigAuthProvidersAction(w http.ResponseWriter, r *http.Request) {
	typ, ok := validAuthType(chi.URLParam(r, "type"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	dest := h.basePath() + "/config/security/auth/" + typ
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, dest+"?err="+url.QueryEscape("Invalid form data."), http.StatusSeeOther)
		return
	}
	admin := h.getAdminFromSession(r)
	var actorID *int64
	if admin != nil {
		actorID = &admin.ID
	}
	action := r.FormValue("action")

	switch action {
	case "toggle_section":
		enabled := config.IsTruthy(r.FormValue("enabled"))
		switch typ {
		case "oidc":
			h.cfg.Server.Auth.OIDC.Enabled = enabled
		case "ldap":
			h.cfg.Server.Auth.LDAP.Enabled = enabled
		case "saml":
			h.cfg.Server.Auth.SAML.Enabled = enabled
		}
		if err := h.persistAuth(); err != nil {
			http.Redirect(w, r, dest+"?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		h.recordAudit(r, actorID, "auth_"+typ+"_section_toggle", "auth:"+typ, r.FormValue("enabled"))
		http.Redirect(w, r, dest+"?saved=section", http.StatusSeeOther)
		return

	case "delete":
		name := r.FormValue("name")
		if err := h.deleteAuthProvider(typ, name); err != nil {
			http.Redirect(w, r, dest+"?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		h.recordAudit(r, actorID, "auth_provider_delete", typ+":"+name, "")
		http.Redirect(w, r, dest+"?saved=deleted", http.StatusSeeOther)
		return

	case "test":
		name := r.FormValue("name")
		if _, err := h.testAuthProvider(r.Context(), typ, name); err != nil {
			h.recordAudit(r, actorID, "auth_provider_test_failed", typ+":"+name, err.Error())
			http.Redirect(w, r, dest+"?err="+url.QueryEscape("Test failed: "+err.Error()), http.StatusSeeOther)
			return
		}
		h.recordAudit(r, actorID, "auth_provider_test", typ+":"+name, "")
		http.Redirect(w, r, dest+"?test=ok", http.StatusSeeOther)
		return

	case "save":
		orig := strings.TrimSpace(r.FormValue("orig"))
		var err error
		var name string
		switch typ {
		case "oidc":
			p := oidcFromForm(r)
			name = p.Name
			err = h.upsertOIDC(orig, p)
		case "ldap":
			p := ldapFromForm(r)
			name = p.Name
			err = h.upsertLDAP(orig, p)
		case "saml":
			p := samlFromForm(r)
			name = p.Name
			err = h.upsertSAML(orig, p)
		}
		if err != nil {
			http.Redirect(w, r, dest+"?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		savedTok := "created"
		auditAction := "auth_provider_create"
		if orig != "" {
			savedTok, auditAction = "updated", "auth_provider_update"
		}
		h.recordAudit(r, actorID, auditAction, typ+":"+name, "")
		http.Redirect(w, r, dest+"?saved="+savedTok, http.StatusSeeOther)
		return

	default:
		http.Redirect(w, r, dest+"?err="+url.QueryEscape("Unknown action."), http.StatusSeeOther)
	}
}

// oidcFromForm builds an OIDC provider from form values (secrets untouched here).
func oidcFromForm(r *http.Request) config.OIDCProvider {
	return config.OIDCProvider{
		Name:         strings.TrimSpace(r.FormValue("name")),
		DisplayName:  strings.TrimSpace(r.FormValue("display_name")),
		Issuer:       strings.TrimSpace(r.FormValue("issuer")),
		ClientID:     strings.TrimSpace(r.FormValue("client_id")),
		ClientSecret: r.FormValue("client_secret"),
		Scopes:       splitCommaList(r.FormValue("scopes")),
		AutoRegister: config.IsTruthy(r.FormValue("auto_register")),
		AdminGroups:  splitCommaList(r.FormValue("admin_groups")),
		ClaimsMapping: config.OIDCClaimsMapping{
			Username: strings.TrimSpace(r.FormValue("claim_username")),
			Email:    strings.TrimSpace(r.FormValue("claim_email")),
			Name:     strings.TrimSpace(r.FormValue("claim_name")),
			Groups:   strings.TrimSpace(r.FormValue("claim_groups")),
		},
	}
}

// ldapFromForm builds an LDAP provider from form values. tls_verify is a
// checkbox, which the browser omits entirely when unchecked — so a hidden
// "tls_verify_present" sentinel (always submitted by our own form) is used
// to distinguish "checkbox rendered and left unchecked" (explicit false)
// from "field never submitted at all" (leave nil so Normalize applies the
// secure tls_verify:true default instead of silently disabling verification).
func ldapFromForm(r *http.Request) config.LDAPProvider {
	var tlsVerify *bool
	if r.FormValue("tls_verify_present") != "" {
		v := config.IsTruthy(r.FormValue("tls_verify"))
		tlsVerify = &v
	}
	return config.LDAPProvider{
		Name:         strings.TrimSpace(r.FormValue("name")),
		DisplayName:  strings.TrimSpace(r.FormValue("display_name")),
		Server:       strings.TrimSpace(r.FormValue("server")),
		BindDN:       strings.TrimSpace(r.FormValue("bind_dn")),
		BindPassword: r.FormValue("bind_password"),
		BaseDN:       strings.TrimSpace(r.FormValue("base_dn")),
		UserFilter:   strings.TrimSpace(r.FormValue("user_filter")),
		TLSMode:      strings.TrimSpace(r.FormValue("tls_mode")),
		TLSVerify:    tlsVerify,
		AutoRegister: config.IsTruthy(r.FormValue("auto_register")),
		AdminGroups:  splitCommaList(r.FormValue("admin_groups")),
		Attributes: config.LDAPAttributes{
			Username: strings.TrimSpace(r.FormValue("attr_username")),
			Email:    strings.TrimSpace(r.FormValue("attr_email")),
			Name:     strings.TrimSpace(r.FormValue("attr_name")),
			Groups:   strings.TrimSpace(r.FormValue("attr_groups")),
		},
	}
}

// samlFromForm builds a SAML provider from form values.
func samlFromForm(r *http.Request) config.SAMLProvider {
	return config.SAMLProvider{
		Name:             strings.TrimSpace(r.FormValue("name")),
		DisplayName:      strings.TrimSpace(r.FormValue("display_name")),
		IDPMetadataURL:   strings.TrimSpace(r.FormValue("idp_metadata_url")),
		IDPMetadataXML:   strings.TrimSpace(r.FormValue("idp_metadata_xml")),
		SPEntityID:       strings.TrimSpace(r.FormValue("sp_entity_id")),
		ACSURL:           strings.TrimSpace(r.FormValue("acs_url")),
		AutoRegister:     config.IsTruthy(r.FormValue("auto_register")),
		AdminGroups:      splitCommaList(r.FormValue("admin_groups")),
		AutoGenerateCert: config.IsTruthy(r.FormValue("auto_generate_cert")),
		SPCertPath:       strings.TrimSpace(r.FormValue("sp_cert_path")),
		SPKeyPath:        strings.TrimSpace(r.FormValue("sp_key_path")),
		AttributeMapping: config.SAMLAttributeMapping{
			Username: strings.TrimSpace(r.FormValue("attr_username")),
			Email:    strings.TrimSpace(r.FormValue("attr_email")),
			Name:     strings.TrimSpace(r.FormValue("attr_name")),
			Groups:   strings.TrimSpace(r.FormValue("attr_groups")),
		},
	}
}

// --------------------------------------------------------------------------
// Admin API
// --------------------------------------------------------------------------

// mapAuthErr writes the canonical error for an upsert/delete/test failure.
func mapAuthErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAuthDuplicate):
		jsonAdminErr(w, http.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, errAuthNotFound):
		jsonAdminErr(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	default:
		jsonAdminErr(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
	}
}

// APIAuthProvidersList handles GET .../config/security/auth/{type}/providers
func (h *AdminHandler) APIAuthProvidersList(w http.ResponseWriter, r *http.Request) {
	typ, ok := validAuthType(chi.URLParam(r, "type"))
	if !ok {
		jsonAdminErr(w, http.StatusNotFound, "NOT_FOUND", "unknown provider type")
		return
	}
	masked := h.cfg.Server.Auth.MaskedCopy()
	switch typ {
	case "oidc":
		jsonAdminOK(w, masked.OIDC.Providers)
	case "ldap":
		jsonAdminOK(w, masked.LDAP.Providers)
	case "saml":
		jsonAdminOK(w, masked.SAML.Providers)
	}
}

// APIAuthProviderGet handles GET .../providers/{provider}
func (h *AdminHandler) APIAuthProviderGet(w http.ResponseWriter, r *http.Request) {
	typ, ok := validAuthType(chi.URLParam(r, "type"))
	if !ok {
		jsonAdminErr(w, http.StatusNotFound, "NOT_FOUND", "unknown provider type")
		return
	}
	name := chi.URLParam(r, "provider")
	masked := h.cfg.Server.Auth.MaskedCopy()
	switch typ {
	case "oidc":
		if i := masked.OIDC.Index(name); i != -1 {
			jsonAdminOK(w, masked.OIDC.Providers[i])
			return
		}
	case "ldap":
		if i := masked.LDAP.Index(name); i != -1 {
			jsonAdminOK(w, masked.LDAP.Providers[i])
			return
		}
	case "saml":
		if i := masked.SAML.Index(name); i != -1 {
			jsonAdminOK(w, masked.SAML.Providers[i])
			return
		}
	}
	jsonAdminErr(w, http.StatusNotFound, "NOT_FOUND", "provider not found")
}

// APIAuthProviderCreate handles POST .../providers
func (h *AdminHandler) APIAuthProviderCreate(w http.ResponseWriter, r *http.Request) {
	h.apiAuthUpsert(w, r, "")
}

// APIAuthProviderUpdate handles PATCH .../providers/{provider}
func (h *AdminHandler) APIAuthProviderUpdate(w http.ResponseWriter, r *http.Request) {
	h.apiAuthUpsert(w, r, chi.URLParam(r, "provider"))
}

// apiAuthUpsert decodes the JSON body and creates (orig=="") or updates a
// provider. On update the path name identifies the target and defaults the body
// name when omitted.
func (h *AdminHandler) apiAuthUpsert(w http.ResponseWriter, r *http.Request, orig string) {
	typ, ok := validAuthType(chi.URLParam(r, "type"))
	if !ok {
		jsonAdminErr(w, http.StatusNotFound, "NOT_FOUND", "unknown provider type")
		return
	}
	var err error
	var name string
	switch typ {
	case "oidc":
		var p config.OIDCProvider
		if derr := json.NewDecoder(r.Body).Decode(&p); derr != nil {
			jsonAdminErr(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body.")
			return
		}
		if orig != "" && p.Name == "" {
			p.Name = orig
		}
		name = p.Name
		err = h.upsertOIDC(orig, p)
	case "ldap":
		var p config.LDAPProvider
		if derr := json.NewDecoder(r.Body).Decode(&p); derr != nil {
			jsonAdminErr(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body.")
			return
		}
		if orig != "" && p.Name == "" {
			p.Name = orig
		}
		name = p.Name
		err = h.upsertLDAP(orig, p)
	case "saml":
		var p config.SAMLProvider
		if derr := json.NewDecoder(r.Body).Decode(&p); derr != nil {
			jsonAdminErr(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body.")
			return
		}
		if orig != "" && p.Name == "" {
			p.Name = orig
		}
		name = p.Name
		err = h.upsertSAML(orig, p)
	}
	if err != nil {
		mapAuthErr(w, err)
		return
	}
	auditAction := "auth_provider_create"
	if orig != "" {
		auditAction = "auth_provider_update"
	}
	h.recordAudit(r, apiActorID(r), auditAction, typ+":"+name, "")
	jsonAdminOK(w, map[string]any{"saved": true, "name": name})
}

// APIAuthProviderDelete handles DELETE .../providers/{provider}
func (h *AdminHandler) APIAuthProviderDelete(w http.ResponseWriter, r *http.Request) {
	typ, ok := validAuthType(chi.URLParam(r, "type"))
	if !ok {
		jsonAdminErr(w, http.StatusNotFound, "NOT_FOUND", "unknown provider type")
		return
	}
	name := chi.URLParam(r, "provider")
	if err := h.deleteAuthProvider(typ, name); err != nil {
		mapAuthErr(w, err)
		return
	}
	h.recordAudit(r, apiActorID(r), "auth_provider_delete", typ+":"+name, "")
	jsonAdminOK(w, map[string]any{"deleted": true, "name": name})
}

// APIAuthProviderTest handles POST .../providers/{provider}/test
func (h *AdminHandler) APIAuthProviderTest(w http.ResponseWriter, r *http.Request) {
	typ, ok := validAuthType(chi.URLParam(r, "type"))
	if !ok {
		jsonAdminErr(w, http.StatusNotFound, "NOT_FOUND", "unknown provider type")
		return
	}
	name := chi.URLParam(r, "provider")
	result, err := h.testAuthProvider(r.Context(), typ, name)
	if err != nil {
		if errors.Is(err, errAuthNotFound) {
			jsonAdminErr(w, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		h.recordAudit(r, apiActorID(r), "auth_provider_test_failed", typ+":"+name, err.Error())
		jsonAdminErr(w, http.StatusBadGateway, "TEST_FAILED", err.Error())
		return
	}
	h.recordAudit(r, apiActorID(r), "auth_provider_test", typ+":"+name, "")
	jsonAdminOK(w, result)
}
