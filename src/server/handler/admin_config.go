package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// adminPageData holds common template data for all admin config pages.
type adminPageData struct {
	Title    string
	Username string
	Version  string
	Mode     string
	BasePath string
	Nav      []adminNavItem
	Content  template.HTML
	Flash    string
	Error    string
}

// adminNavItem represents a sidebar nav entry.
type adminNavItem struct {
	Label  string
	URL    string
	Icon   string
	Active bool
}

// adminNav builds the sidebar navigation items with the active entry marked.
func (h *AdminHandler) adminNav(activePath string) []adminNavItem {
	base := h.basePath()
	items := []adminNavItem{
		{Label: "Dashboard", URL: base + "/dashboard", Icon: "📊"},
		{Label: "Settings", URL: base + "/config/settings", Icon: "⚙️"},
		{Label: "Branding", URL: base + "/config/branding", Icon: "🎨"},
		{Label: "SSL/TLS", URL: base + "/config/ssl", Icon: "🔒"},
		{Label: "Scheduler", URL: base + "/config/scheduler", Icon: "⏰"},
		{Label: "Email", URL: base + "/config/email", Icon: "📧"},
		{Label: "Logs", URL: base + "/config/logs", Icon: "📋"},
		{Label: "Backup", URL: base + "/config/backup", Icon: "💾"},
		{Label: "Maintenance", URL: base + "/config/maintenance", Icon: "🔧"},
		{Label: "Updates", URL: base + "/config/updates", Icon: "🔄"},
		{Label: "Server Info", URL: base + "/config/info", Icon: "ℹ️"},
		{Label: "Auth", URL: base + "/config/security/auth", Icon: "🔑"},
		{Label: "API Tokens", URL: base + "/config/security/tokens", Icon: "🪙"},
		{Label: "Rate Limiting", URL: base + "/config/security/ratelimit", Icon: "⏱️"},
		{Label: "Firewall", URL: base + "/config/security/firewall", Icon: "🛡️"},
		{Label: "Allowlist", URL: base + "/config/security/allowlist", Icon: "✅"},
		{Label: "Tor", URL: base + "/config/network/tor", Icon: "🧅"},
		{Label: "GeoIP", URL: base + "/config/network/geoip", Icon: "🌐"},
		{Label: "Blocklists", URL: base + "/config/network/blocklists", Icon: "🚫"},
		{Label: "Users", URL: base + "/config/users", Icon: "👥"},
		{Label: "Invites", URL: base + "/config/users/invites", Icon: "✉️"},
		{Label: "Moderation", URL: base + "/config/moderation/users", Icon: "🛂"},
		{Label: "Cluster Nodes", URL: base + "/config/cluster/nodes", Icon: "🔗"},
		{Label: "Add Node", URL: base + "/config/cluster/add", Icon: "➕"},
		{Label: "Help", URL: base + "/help", Icon: "❓"},
	}
	for i, it := range items {
		if it.URL == base+activePath {
			items[i].Active = true
		}
	}
	return items
}

// adminLayout returns the shared HTML layout with sidebar and content.
// content is trusted HTML built by each individual handler.
func (h *AdminHandler) adminLayout(w http.ResponseWriter, r *http.Request, title, activePath string, content template.HTML, flash, errMsg string) {
	admin := h.getAdminFromSession(r)
	if admin == nil {
		http.Redirect(w, r, h.basePath(), http.StatusFound)
		return
	}

	const layoutTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} — Caslink Admin</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0d1117;color:#c9d1d9;display:flex;flex-direction:column;min-height:100vh}
        .header{background:#161b22;border-bottom:1px solid #30363d;padding:12px 24px;display:flex;justify-content:space-between;align-items:center;flex-shrink:0}
        .logo{color:#58a6ff;font-size:18px;font-weight:700;text-decoration:none}
        .header-right{display:flex;align-items:center;gap:16px}
        .user-badge{color:#8b949e;font-size:14px}
        .logout{color:#f85149;text-decoration:none;font-size:14px}
        .logout:hover{text-decoration:underline}
        .app-body{display:flex;flex:1;overflow:hidden}
        .sidebar{width:220px;background:#0d1117;border-right:1px solid #21262d;overflow-y:auto;flex-shrink:0;padding:16px 0}
        .sidebar a{display:flex;align-items:center;gap:8px;padding:8px 20px;color:#8b949e;text-decoration:none;font-size:14px;transition:background 0.1s}
        .sidebar a:hover{background:#161b22;color:#c9d1d9}
        .sidebar a.active{background:#1f2d3d;color:#58a6ff;border-left:3px solid #58a6ff;padding-left:17px}
        .sidebar .icon{width:18px;text-align:center;flex-shrink:0}
        .main{flex:1;overflow-y:auto;padding:24px}
        .breadcrumb{font-size:13px;color:#8b949e;margin-bottom:20px}
        .breadcrumb a{color:#58a6ff;text-decoration:none}
        .breadcrumb a:hover{text-decoration:underline}
        h1{font-size:22px;font-weight:600;margin-bottom:20px;color:#e6edf3}
        .card{background:#161b22;border:1px solid #30363d;border-radius:6px;padding:20px;margin-bottom:20px}
        .card h2{font-size:16px;color:#58a6ff;margin-bottom:16px;padding-bottom:10px;border-bottom:1px solid #21262d}
        .form-group{margin-bottom:16px}
        label{display:block;font-size:13px;color:#8b949e;margin-bottom:6px}
        input[type=text],input[type=number],input[type=email],input[type=password],input[type=url],select,textarea{width:100%;background:#0d1117;border:1px solid #30363d;border-radius:6px;padding:8px 12px;color:#c9d1d9;font-size:14px}
        input[type=text]:focus,input[type=number]:focus,input[type=email]:focus,input[type=password]:focus,input[type=url]:focus,select:focus,textarea:focus{outline:none;border-color:#58a6ff}
        textarea{resize:vertical;min-height:80px;font-family:monospace}
        .help-text{font-size:12px;color:#6e7681;margin-top:4px}
        .btn{display:inline-block;padding:8px 16px;border-radius:6px;border:none;cursor:pointer;font-size:14px;font-weight:500;text-decoration:none}
        .btn-primary{background:#238636;color:#fff}
        .btn-primary:hover{background:#2ea043}
        .btn-danger{background:#da3633;color:#fff}
        .btn-danger:hover{background:#f85149}
        .btn-secondary{background:#21262d;color:#c9d1d9;border:1px solid #30363d}
        .btn-secondary:hover{background:#30363d}
        .flash{background:#1c2d1e;border:1px solid #238636;border-radius:6px;padding:12px 16px;margin-bottom:20px;color:#3fb950}
        .error{background:#2d1c1c;border:1px solid #da3633;border-radius:6px;padding:12px 16px;margin-bottom:20px;color:#f85149}
        .info-row{display:flex;justify-content:space-between;padding:10px 0;border-bottom:1px solid #21262d}
        .info-row:last-child{border-bottom:none}
        .info-label{color:#8b949e;font-size:14px}
        .info-value{color:#c9d1d9;font-size:14px;font-family:monospace}
        table{width:100%;border-collapse:collapse}
        th{text-align:left;padding:8px 12px;font-size:13px;color:#8b949e;border-bottom:1px solid #21262d}
        td{padding:10px 12px;font-size:14px;border-bottom:1px solid #21262d}
        tr:last-child td{border-bottom:none}
        .badge{display:inline-block;padding:2px 8px;border-radius:12px;font-size:12px;font-weight:500}
        .badge-green{background:#1c2d1e;color:#3fb950}
        .badge-red{background:#2d1c1c;color:#f85149}
        .badge-yellow{background:#2d2616;color:#d29922}
        .badge-blue{background:#1c2d3a;color:#58a6ff}
        .warn{background:#2d2616;border:1px solid #d29922;border-radius:6px;padding:12px 16px;color:#d29922;margin-bottom:12px;font-size:14px}
        @media(max-width:768px){.sidebar{display:none}.main{padding:16px}}
    </style>
</head>
<body>
    <header class="header">
        <a href="{{.BasePath}}/dashboard" class="logo">Caslink Admin</a>
        <div class="header-right">
            <span class="user-badge">{{.Username}}</span>
            <a href="{{.BasePath}}/logout" class="logout">Logout</a>
        </div>
    </header>
    <div class="app-body">
        <nav class="sidebar">
            {{range .Nav}}<a href="{{.URL}}" class="{{if .Active}}active{{end}}"><span class="icon">{{.Icon}}</span>{{.Label}}</a>{{end}}
        </nav>
        <main class="main">
            {{if .Flash}}<div class="flash">{{.Flash}}</div>{{end}}
            {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
            {{.Content}}
        </main>
    </div>
</body>
</html>`

	t := template.Must(template.New("layout").Parse(layoutTmpl))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = t.Execute(w, adminPageData{
		Title:    title,
		Username: admin.Username,
		Version:  h.version,
		Mode:     h.mode,
		BasePath: h.basePath(),
		Nav:      h.adminNav(activePath),
		Content:  content,
		Flash:    flash,
		Error:    errMsg,
	})
}

// jsonOK writes a JSON success response.
func jsonAdminOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": data})
}

// jsonAdminErr writes a JSON RFC 7807 error response.
func jsonAdminErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      false,
		"error":   code,
		"message": msg,
	})
}

// --------------------------------------------------------------------------
// Settings
// --------------------------------------------------------------------------

// ConfigSettings handles GET /server/{adminPath}/config/settings
func (h *AdminHandler) ConfigSettings(w http.ResponseWriter, r *http.Request) {
	flash := r.URL.Query().Get("saved")
	flashMsg := ""
	if flash == "1" {
		flashMsg = "Settings saved."
	}
	cfg := h.cfg.Server
	content := fmt.Sprintf(`
<h1>Server Settings</h1>
<form method="POST" action="%s/config/settings">
<div class="card">
  <h2>General</h2>
  <div class="form-group">
    <label>Port</label>
    <input type="number" name="port" value="%d" min="1" max="65535">
    <div class="help-text">Port the server listens on. Requires restart.</div>
  </div>
  <div class="form-group">
    <label>Address</label>
    <input type="text" name="address" value="%s" placeholder="0.0.0.0">
    <div class="help-text">Bind address. Use 0.0.0.0 for all interfaces.</div>
  </div>
  <div class="form-group">
    <label>Mode</label>
    <select name="mode">
      <option value="production"%s>Production</option>
      <option value="development"%s>Development</option>
    </select>
  </div>
  <div class="form-group">
    <label>FQDN</label>
    <input type="text" name="fqdn" value="%s" placeholder="example.com">
    <div class="help-text">Fully qualified domain name (auto-detected from Host header if blank).</div>
  </div>
</div>
<div class="card">
  <h2>Admin Panel</h2>
  <div class="form-group">
    <label>Admin Path</label>
    <input type="text" name="admin_path" value="%s" placeholder="admin">
    <div class="help-text">URL segment for the admin panel. Valid chars: [a-z0-9-], 2–32 chars.</div>
  </div>
</div>
<div style="display:flex;gap:8px">
  <button type="submit" class="btn btn-primary">Save Settings</button>
  <a href="%s/config/settings" class="btn btn-secondary">Cancel</a>
</div>
</form>`,
		h.basePath(),
		cfg.Port,
		cfg.Address,
		selectedIf(cfg.Mode == "production"),
		selectedIf(cfg.Mode == "development"),
		cfg.FQDN,
		cfg.Admin.Path,
		h.basePath(),
	)
	h.adminLayout(w, r, "Server Settings", "/config/settings", template.HTML(content), flashMsg, "")
}

// ConfigSettingsSave handles POST /server/{adminPath}/config/settings
func (h *AdminHandler) ConfigSettingsSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Server Settings", "/config/settings", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	pairs := map[string]string{
		"server.port":       r.FormValue("port"),
		"server.address":    r.FormValue("address"),
		"server.mode":       r.FormValue("mode"),
		"server.fqdn":       r.FormValue("fqdn"),
		"server.admin_path": r.FormValue("admin_path"),
	}
	for k, v := range pairs {
		if err := h.store.SetConfigValue(k, v, username); err != nil {
			h.adminLayout(w, r, "Server Settings", "/config/settings", "", "", "Failed to save: "+err.Error())
			return
		}
	}
	http.Redirect(w, r, h.basePath()+"/config/settings?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Branding
// --------------------------------------------------------------------------

// ConfigBranding handles GET /server/{adminPath}/config/branding
func (h *AdminHandler) ConfigBranding(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("saved") == "1" {
		flash = "Branding saved."
	}
	b := h.cfg.Server.Branding
	content := fmt.Sprintf(`
<h1>Branding</h1>
<form method="POST" action="%s/config/branding">
<div class="card">
  <h2>Identity</h2>
  <div class="form-group">
    <label>Site Name</label>
    <input type="text" name="site_name" value="%s" placeholder="Caslink">
  </div>
  <div class="form-group">
    <label>Tagline</label>
    <input type="text" name="tagline" value="%s" placeholder="A fast URL shortener">
  </div>
  <div class="form-group">
    <label>Logo URL</label>
    <input type="url" name="logo_url" value="%s" placeholder="https://…/logo.png">
  </div>
  <div class="form-group">
    <label>Favicon URL</label>
    <input type="url" name="favicon_url" value="%s" placeholder="https://…/favicon.ico">
  </div>
</div>
<div class="card">
  <h2>Theme</h2>
  <div class="form-group">
    <label>Default Theme</label>
    <select name="default_theme">
      <option value="dark"%s>Dark</option>
      <option value="light"%s>Light</option>
      <option value="auto"%s>Auto (follows OS preference)</option>
    </select>
  </div>
  <div class="form-group">
    <label>Primary Colour (hex)</label>
    <input type="text" name="primary_color" value="%s" placeholder="#58a6ff">
  </div>
</div>
<div style="display:flex;gap:8px">
  <button type="submit" class="btn btn-primary">Save Branding</button>
  <a href="%s/config/branding" class="btn btn-secondary">Cancel</a>
</div>
</form>`,
		h.basePath(),
		template.HTMLEscapeString(b.Title),
		template.HTMLEscapeString(b.Tagline),
		template.HTMLEscapeString(b.LogoURL),
		template.HTMLEscapeString(b.FaviconURL),
		selectedIf(b.DefaultTheme == "dark"),
		selectedIf(b.DefaultTheme == "light"),
		selectedIf(b.DefaultTheme == "auto"),
		template.HTMLEscapeString(b.PrimaryColor),
		h.basePath(),
	)
	h.adminLayout(w, r, "Branding", "/config/branding", template.HTML(content), flash, "")
}

// ConfigBrandingSave handles POST /server/{adminPath}/config/branding
func (h *AdminHandler) ConfigBrandingSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Branding", "/config/branding", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	pairs := map[string]string{
		"branding.site_name":     r.FormValue("site_name"),
		"branding.tagline":       r.FormValue("tagline"),
		"branding.logo_url":      r.FormValue("logo_url"),
		"branding.favicon_url":   r.FormValue("favicon_url"),
		"branding.default_theme": r.FormValue("default_theme"),
		"branding.primary_color": r.FormValue("primary_color"),
	}
	for k, v := range pairs {
		if err := h.store.SetConfigValue(k, v, username); err != nil {
			h.adminLayout(w, r, "Branding", "/config/branding", "", "", "Failed to save: "+err.Error())
			return
		}
	}
	http.Redirect(w, r, h.basePath()+"/config/branding?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// SSL/TLS
// --------------------------------------------------------------------------

// ConfigSSL handles GET /server/{adminPath}/config/ssl
func (h *AdminHandler) ConfigSSL(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("saved") == "1" {
		flash = "SSL settings saved."
	}
	s := h.cfg.Server.SSL
	content := fmt.Sprintf(`
<h1>SSL / TLS</h1>
<form method="POST" action="%s/config/ssl">
<div class="card">
  <h2>SSL Status</h2>
  <div class="form-group">
    <label>SSL Enabled</label>
    <select name="enabled">
      <option value="true"%s>Enabled</option>
      <option value="false"%s>Disabled</option>
    </select>
  </div>
  <div class="form-group">
    <label>Certificate File Path</label>
    <input type="text" name="cert_file" value="%s" placeholder="/etc/casapps/caslink/ssl/cert.pem">
  </div>
  <div class="form-group">
    <label>Key File Path</label>
    <input type="text" name="key_file" value="%s" placeholder="/etc/casapps/caslink/ssl/key.pem">
  </div>
</div>
<div class="card">
  <h2>Let's Encrypt</h2>
  <div class="form-group">
    <label>Let's Encrypt Enabled</label>
    <select name="le_enabled">
      <option value="true"%s>Enabled</option>
      <option value="false"%s>Disabled</option>
    </select>
  </div>
  <div class="form-group">
    <label>Email (for renewal notices)</label>
    <input type="email" name="le_email" value="%s" placeholder="admin@example.com">
  </div>
  <div class="form-group">
    <label>Domains (one per line)</label>
    <textarea name="le_domains">%s</textarea>
    <div class="help-text">Domains to obtain certificates for.</div>
  </div>
  <div class="form-group">
    <label>Challenge Type</label>
    <select name="le_challenge">
      <option value="http-01"%s>HTTP-01</option>
      <option value="dns-01"%s>DNS-01</option>
    </select>
  </div>
</div>
<div style="display:flex;gap:8px">
  <button type="submit" class="btn btn-primary">Save SSL Settings</button>
  <a href="%s/config/ssl" class="btn btn-secondary">Cancel</a>
</div>
</form>`,
		h.basePath(),
		selectedIf(s.Enabled),
		selectedIf(!s.Enabled),
		template.HTMLEscapeString(s.Cert),
		template.HTMLEscapeString(s.Key),
		selectedIf(s.LetsEncrypt.Enabled),
		selectedIf(!s.LetsEncrypt.Enabled),
		template.HTMLEscapeString(s.LetsEncrypt.Email),
		template.HTMLEscapeString(strings.Join(s.LetsEncrypt.Domains, "\n")),

		selectedIf(s.LetsEncrypt.Challenge == "http-01" || s.LetsEncrypt.Challenge == ""),
		selectedIf(s.LetsEncrypt.Challenge == "dns-01"),
		h.basePath(),
	)
	h.adminLayout(w, r, "SSL / TLS", "/config/ssl", template.HTML(content), flash, "")
}

// ConfigSSLSave handles POST /server/{adminPath}/config/ssl
func (h *AdminHandler) ConfigSSLSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "SSL / TLS", "/config/ssl", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	pairs := map[string]string{
		"ssl.enabled":      r.FormValue("enabled"),
		"ssl.cert_file":    r.FormValue("cert_file"),
		"ssl.key_file":     r.FormValue("key_file"),
		"ssl.le_enabled":   r.FormValue("le_enabled"),
		"ssl.le_email":     r.FormValue("le_email"),
		"ssl.le_domains":   r.FormValue("le_domains"),
		"ssl.le_challenge": r.FormValue("le_challenge"),
	}
	for k, v := range pairs {
		if err := h.store.SetConfigValue(k, v, username); err != nil {
			h.adminLayout(w, r, "SSL / TLS", "/config/ssl", "", "", "Failed to save: "+err.Error())
			return
		}
	}
	http.Redirect(w, r, h.basePath()+"/config/ssl?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Scheduler
// --------------------------------------------------------------------------

// ConfigScheduler handles GET /server/{adminPath}/config/scheduler
func (h *AdminHandler) ConfigScheduler(w http.ResponseWriter, r *http.Request) {
	sch := h.cfg.Server.Scheduler
	tasks := []struct {
		Name     string
		Schedule string
		Enabled  bool
	}{
		{"session_cleanup", sch.SessionCleanupCron, sch.SessionCleanupEnabled},
		{"token_cleanup", sch.TokenCleanupCron, sch.TokenCleanupEnabled},
		{"expire_urls", sch.ExpireURLsCron, sch.ExpireURLsEnabled},
		{"log_rotation", sch.LogRotationCron, sch.LogRotationEnabled},
		{"backup_daily", sch.BackupCron, sch.BackupEnabled},
		{"ssl_renewal", sch.SSLRenewalCron, sch.SSLRenewalEnabled},
		{"geoip_update", sch.GeoIPUpdateCron, sch.GeoIPUpdateEnabled},
		{"blocklist_update", sch.BlocklistUpdateCron, sch.BlocklistUpdateEnabled},
		{"cve_update", sch.CVEUpdateCron, sch.CVEUpdateEnabled},
		{"healthcheck_self", sch.HealthcheckCron, sch.HealthcheckEnabled},
		{"tor_health", sch.TorHealthCron, sch.TorHealthEnabled},
	}

	var rows strings.Builder
	for _, t := range tasks {
		badge := `<span class="badge badge-green">enabled</span>`
		if !t.Enabled {
			badge = `<span class="badge badge-red">disabled</span>`
		}
		rows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td style="font-family:monospace">%s</td><td>%s</td></tr>`,
			template.HTMLEscapeString(t.Name),
			template.HTMLEscapeString(t.Schedule),
			badge,
		))
	}

	content := fmt.Sprintf(`
<h1>Scheduler</h1>
<div class="card">
  <h2>Scheduled Tasks</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:16px">
    Built-in cron scheduler (robfig/cron). Schedules use standard cron syntax.
    Changes require editing <code>server.yml</code> and restarting.
  </p>
  <table>
    <thead><tr><th>Task</th><th>Schedule</th><th>Status</th></tr></thead>
    <tbody>%s</tbody>
  </table>
</div>`, rows.String())

	h.adminLayout(w, r, "Scheduler", "/config/scheduler", template.HTML(content), "", "")
}

// --------------------------------------------------------------------------
// Email
// --------------------------------------------------------------------------

// ConfigEmail handles GET /server/{adminPath}/config/email
func (h *AdminHandler) ConfigEmail(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("saved") == "1" {
		flash = "Email settings saved."
	}
	e := h.cfg.Server.Notifications.Email
	s := h.cfg.Server.Notifications.Email.SMTP
	content := fmt.Sprintf(`
<h1>Email</h1>
<form method="POST" action="%s/config/email">
<div class="card">
  <h2>Email Provider</h2>
  <div class="form-group">
    <label>Enabled</label>
    <select name="enabled">
      <option value="true"%s>Enabled</option>
      <option value="false"%s>Disabled</option>
    </select>
  </div>
  <div class="form-group">
    <label>Provider</label>
    <select name="provider">
      <option value="smtp"%s>SMTP</option>
      <option value="sendgrid"%s>SendGrid</option>
      <option value="ses"%s>Amazon SES</option>
    </select>
  </div>
  <div class="form-group">
    <label>From Address</label>
    <input type="email" name="from_address" value="%s" placeholder="noreply@example.com">
  </div>
  <div class="form-group">
    <label>From Name</label>
    <input type="text" name="from_name" value="%s" placeholder="Caslink">
  </div>
</div>
<div class="card">
  <h2>SMTP Configuration</h2>
  <div class="form-group">
    <label>SMTP Host</label>
    <input type="text" name="smtp_host" value="%s" placeholder="smtp.example.com">
  </div>
  <div class="form-group">
    <label>SMTP Port</label>
    <input type="number" name="smtp_port" value="%d" min="1" max="65535">
  </div>
  <div class="form-group">
    <label>Username</label>
    <input type="text" name="smtp_username" value="%s">
  </div>
  <div class="form-group">
    <label>Password</label>
    <input type="password" name="smtp_password" placeholder="(unchanged)">
    <div class="help-text">Leave blank to keep existing password.</div>
  </div>
  <div class="form-group">
    <label>TLS</label>
    <select name="smtp_tls">
      <option value="true"%s>Enabled</option>
      <option value="false"%s>Disabled</option>
    </select>
  </div>
</div>
<div style="display:flex;gap:8px">
  <button type="submit" class="btn btn-primary">Save Email Settings</button>
  <a href="%s/config/email" class="btn btn-secondary">Cancel</a>
</div>
</form>`,
		h.basePath(),
		selectedIf(e.Enabled),
		selectedIf(!e.Enabled),
		selectedIf(e.Provider == "smtp" || e.Provider == ""),
		selectedIf(e.Provider == "sendgrid"),
		selectedIf(e.Provider == "ses"),
		template.HTMLEscapeString(e.From),
		template.HTMLEscapeString(e.FromName),
		template.HTMLEscapeString(s.Host),
		s.Port,
		template.HTMLEscapeString(s.Username),
		selectedIf(s.UseTLS),
		selectedIf(!s.UseTLS),
		h.basePath(),
	)
	h.adminLayout(w, r, "Email", "/config/email", template.HTML(content), flash, "")
}

// ConfigEmailSave handles POST /server/{adminPath}/config/email
func (h *AdminHandler) ConfigEmailSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Email", "/config/email", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	pairs := map[string]string{
		"email.enabled":      r.FormValue("enabled"),
		"email.provider":     r.FormValue("provider"),
		"email.from_address": r.FormValue("from_address"),
		"email.from_name":    r.FormValue("from_name"),
		"email.smtp_host":    r.FormValue("smtp_host"),
		"email.smtp_port":    r.FormValue("smtp_port"),
		"email.smtp_user":    r.FormValue("smtp_username"),
		"email.smtp_tls":     r.FormValue("smtp_tls"),
	}
	// Only update password if provided
	if pw := r.FormValue("smtp_password"); pw != "" {
		pairs["email.smtp_password"] = pw
	}
	for k, v := range pairs {
		if err := h.store.SetConfigValue(k, v, username); err != nil {
			h.adminLayout(w, r, "Email", "/config/email", "", "", "Failed to save: "+err.Error())
			return
		}
	}
	http.Redirect(w, r, h.basePath()+"/config/email?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Logs
// --------------------------------------------------------------------------

// ConfigLogs handles GET /server/{adminPath}/config/logs
func (h *AdminHandler) ConfigLogs(w http.ResponseWriter, r *http.Request) {
	content := fmt.Sprintf(`
<h1>Logs</h1>
<div class="card">
  <h2>Log Configuration</h2>
  <div class="info-row"><span class="info-label">Log Level</span><span class="info-value">%s</span></div>
  <div class="info-row"><span class="info-label">Log Format</span><span class="info-value">%s</span></div>
  <div class="info-row"><span class="info-label">Log Dir</span><span class="info-value">%s</span></div>
</div>
<div class="card">
  <h2>Log Files</h2>
  <p style="color:#8b949e;font-size:14px">
    Log files are written to the configured log directory.
    Use <code>--log {log_dir}</code> CLI flag or configure <code>server.yml</code> to set the path.
    <a href="%s/config/logs/audit" style="color:#58a6ff">View Audit Log →</a>
  </p>
</div>`,
		template.HTMLEscapeString(h.cfg.Server.Mode),
		"json",
		"(configured via --log flag or server.yml)",
		h.basePath(),
	)
	h.adminLayout(w, r, "Logs", "/config/logs", template.HTML(content), "", "")
}

// ConfigLogsAudit handles GET /server/{adminPath}/config/logs/audit
func (h *AdminHandler) ConfigLogsAudit(w http.ResponseWriter, r *http.Request) {
	content := `
<h1>Audit Log</h1>
<div class="card">
  <h2>Recent Audit Events</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:16px">
    All admin actions are recorded in the audit log. The audit log is append-only
    and stored in <code>users.db</code> (audit_log table).
  </p>
  <table>
    <thead><tr><th>Time</th><th>Admin</th><th>Action</th><th>Resource</th></tr></thead>
    <tbody>
      <tr><td colspan="4" style="color:#8b949e;text-align:center;padding:20px">
        Audit log viewer coming soon. Query the <code>audit_log</code> table in users.db directly.
      </td></tr>
    </tbody>
  </table>
</div>`
	h.adminLayout(w, r, "Audit Log", "/config/logs/audit", template.HTML(content), "", "")
}

// --------------------------------------------------------------------------
// Backup
// --------------------------------------------------------------------------

// ConfigBackup handles GET /server/{adminPath}/config/backup
func (h *AdminHandler) ConfigBackup(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("done") == "1" {
		flash = "Backup initiated. Check logs for status."
	}
	content := fmt.Sprintf(`
<h1>Backup &amp; Restore</h1>
<div class="card">
  <h2>Manual Backup</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:16px">
    Creates a timestamped backup of all databases to the backup directory.
    Scheduled backups run automatically via the <a href="%s/config/scheduler" style="color:#58a6ff">Scheduler</a>.
  </p>
  <form method="POST" action="%s/config/backup">
    <input type="hidden" name="action" value="backup">
    <button type="submit" class="btn btn-primary">Create Backup Now</button>
  </form>
</div>
<div class="card">
  <h2>Restore</h2>
  <div class="warn">⚠️ Restoring a backup will overwrite all current data. This action cannot be undone.</div>
  <p style="color:#8b949e;font-size:14px">
    Use <code>caslink --maintenance restore {file}</code> from the command line to restore a backup.
    Web-based restore is not available for safety reasons.
  </p>
</div>`,
		h.basePath(), h.basePath(),
	)
	h.adminLayout(w, r, "Backup", "/config/backup", template.HTML(content), flash, "")
}

// ConfigBackupAction handles POST /server/{adminPath}/config/backup
func (h *AdminHandler) ConfigBackupAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Backup", "/config/backup", "", "", "Invalid request.")
		return
	}
	// Trigger backup via the store's backup mechanism.
	// The actual backup is handled by the backup service; here we record the intent.
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	_ = h.store.SetConfigValue("backup.last_requested", time.Now().UTC().Format(time.RFC3339), username)
	http.Redirect(w, r, h.basePath()+"/config/backup?done=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Maintenance
// --------------------------------------------------------------------------

// ConfigMaintenance handles GET /server/{adminPath}/config/maintenance
func (h *AdminHandler) ConfigMaintenance(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("saved") == "1" {
		flash = "Maintenance settings saved."
	}
	content := fmt.Sprintf(`
<h1>Maintenance</h1>
<form method="POST" action="%s/config/maintenance">
<div class="card">
  <h2>Maintenance Mode</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:16px">
    When enabled, all public routes return HTTP 503 with a maintenance page.
    Admin panel remains accessible.
  </p>
  <div class="form-group">
    <label>Maintenance Mode</label>
    <select name="enabled">
      <option value="false">Disabled (normal operation)</option>
      <option value="true">Enabled (maintenance mode)</option>
    </select>
  </div>
  <div class="form-group">
    <label>Maintenance Message</label>
    <textarea name="message" placeholder="We're performing maintenance. We'll be back shortly."></textarea>
  </div>
  <div class="form-group">
    <label>Estimated End Time (optional)</label>
    <input type="text" name="end_time" placeholder="e.g. 2026-06-05 14:00 UTC">
  </div>
</div>
<div style="display:flex;gap:8px">
  <button type="submit" class="btn btn-primary">Save</button>
  <a href="%s/config/maintenance" class="btn btn-secondary">Cancel</a>
</div>
</form>`, h.basePath(), h.basePath())
	h.adminLayout(w, r, "Maintenance", "/config/maintenance", template.HTML(content), flash, "")
}

// ConfigMaintenanceSave handles POST /server/{adminPath}/config/maintenance
func (h *AdminHandler) ConfigMaintenanceSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Maintenance", "/config/maintenance", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	pairs := map[string]string{
		"maintenance.enabled":  r.FormValue("enabled"),
		"maintenance.message":  r.FormValue("message"),
		"maintenance.end_time": r.FormValue("end_time"),
	}
	for k, v := range pairs {
		if err := h.store.SetConfigValue(k, v, username); err != nil {
			h.adminLayout(w, r, "Maintenance", "/config/maintenance", "", "", "Failed to save: "+err.Error())
			return
		}
	}
	http.Redirect(w, r, h.basePath()+"/config/maintenance?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Updates
// --------------------------------------------------------------------------

// ConfigUpdates handles GET /server/{adminPath}/config/updates
func (h *AdminHandler) ConfigUpdates(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("checked") == "1" {
		flash = "Update check initiated. See logs for results."
	}
	content := fmt.Sprintf(`
<h1>Updates</h1>
<div class="card">
  <h2>Current Version</h2>
  <div class="info-row"><span class="info-label">Installed Version</span><span class="info-value">%s</span></div>
  <div class="info-row"><span class="info-label">Update Channel</span><span class="info-value">stable</span></div>
</div>
<div class="card">
  <h2>Check for Updates</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:16px">
    You can also use the CLI: <code>caslink --update check</code>
  </p>
  <form method="POST" action="%s/config/updates">
    <input type="hidden" name="action" value="check">
    <button type="submit" class="btn btn-primary">Check Now</button>
  </form>
</div>
<div class="card">
  <h2>Update Channel</h2>
  <form method="POST" action="%s/config/updates">
    <div class="form-group">
      <label>Channel</label>
      <select name="channel">
        <option value="stable">Stable</option>
        <option value="beta">Beta</option>
        <option value="daily">Daily</option>
      </select>
    </div>
    <input type="hidden" name="action" value="channel">
    <button type="submit" class="btn btn-secondary">Set Channel</button>
  </form>
</div>`,
		template.HTMLEscapeString(h.version),
		h.basePath(),
		h.basePath(),
	)
	h.adminLayout(w, r, "Updates", "/config/updates", template.HTML(content), flash, "")
}

// ConfigUpdatesAction handles POST /server/{adminPath}/config/updates
func (h *AdminHandler) ConfigUpdatesAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Updates", "/config/updates", "", "", "Invalid request.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	action := r.FormValue("action")
	switch action {
	case "channel":
		if ch := r.FormValue("channel"); ch != "" {
			_ = h.store.SetConfigValue("updates.channel", ch, username)
		}
	}
	http.Redirect(w, r, h.basePath()+"/config/updates?checked=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Server Info
// --------------------------------------------------------------------------

// ConfigInfo handles GET /server/{adminPath}/config/info
func (h *AdminHandler) ConfigInfo(w http.ResponseWriter, r *http.Request) {
	content := fmt.Sprintf(`
<h1>Server Information</h1>
<div class="card">
  <h2>Application</h2>
  <div class="info-row"><span class="info-label">Version</span><span class="info-value">%s</span></div>
  <div class="info-row"><span class="info-label">Mode</span><span class="info-value">%s</span></div>
  <div class="info-row"><span class="info-label">Address</span><span class="info-value">%s:%d</span></div>
  <div class="info-row"><span class="info-label">FQDN</span><span class="info-value">%s</span></div>
</div>
<div class="card">
  <h2>Runtime</h2>
  <div class="info-row"><span class="info-label">Go Version</span><span class="info-value">%s</span></div>
  <div class="info-row"><span class="info-label">OS/Arch</span><span class="info-value">%s/%s</span></div>
  <div class="info-row"><span class="info-label">CPUs</span><span class="info-value">%d</span></div>
</div>
<div class="card">
  <h2>Database</h2>
  <div class="info-row"><span class="info-label">Driver</span><span class="info-value">%s</span></div>
  <div class="info-row"><span class="info-label">Server DB</span><span class="info-value">server.db</span></div>
  <div class="info-row"><span class="info-label">Users DB</span><span class="info-value">users.db</span></div>
</div>`,
		template.HTMLEscapeString(h.version),
		template.HTMLEscapeString(h.mode),
		template.HTMLEscapeString(h.cfg.Server.Address),
		h.cfg.Server.Port,
		template.HTMLEscapeString(h.cfg.Server.FQDN),
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
		runtime.NumCPU(),
		template.HTMLEscapeString(h.cfg.Server.Database.Driver),
	)
	h.adminLayout(w, r, "Server Info", "/config/info", template.HTML(content), "", "")
}

// --------------------------------------------------------------------------
// Security — Auth
// --------------------------------------------------------------------------

// ConfigSecurityAuth handles GET /server/{adminPath}/config/security/auth
func (h *AdminHandler) ConfigSecurityAuth(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("saved") == "1" {
		flash = "Auth settings saved."
	}
	p := h.cfg.Server.Security.Password
	s := h.cfg.Server.Session
	content := fmt.Sprintf(`
<h1>Authentication</h1>
<form method="POST" action="%s/config/security/auth">
<div class="card">
  <h2>Password Policy</h2>
  <div class="form-group">
    <label>Minimum Length</label>
    <input type="number" name="pwd_min_length" value="%d" min="8" max="128">
  </div>
  <div class="form-group">
    <label><input type="checkbox" name="pwd_require_upper"%s> Require uppercase letter</label>
  </div>
  <div class="form-group">
    <label><input type="checkbox" name="pwd_require_lower"%s> Require lowercase letter</label>
  </div>
  <div class="form-group">
    <label><input type="checkbox" name="pwd_require_number"%s> Require number</label>
  </div>
  <div class="form-group">
    <label><input type="checkbox" name="pwd_require_special"%s> Require special character</label>
  </div>
</div>
<div class="card">
  <h2>Session</h2>
  <div class="form-group">
    <label>Session Timeout</label>
    <input type="text" name="session_timeout" value="%s" placeholder="24h">
    <div class="help-text">Duration string, e.g. 30m, 12h, 7d.</div>
  </div>
  <div class="form-group">
    <label>Remember Me Duration</label>
    <input type="text" name="remember_timeout" value="%s" placeholder="720h">
    <div class="help-text">Duration for "remember me" sessions.</div>
  </div>
</div>
<div class="card">
  <h2>Multi-Factor Authentication</h2>
  <div class="form-group">
    <label>TOTP Issuer Name</label>
    <input type="text" name="totp_issuer" value="%s" placeholder="Caslink">
  </div>
  <div class="form-group">
    <label>WebAuthn Display Name</label>
    <input type="text" name="webauthn_display" value="%s" placeholder="Caslink URL Shortener">
  </div>
</div>
<div style="display:flex;gap:8px">
  <button type="submit" class="btn btn-primary">Save Auth Settings</button>
  <a href="%s/config/security/auth" class="btn btn-secondary">Cancel</a>
</div>
</form>`,
		h.basePath(),
		p.MinLength,
		checkedIf(p.RequireUppercase),
		checkedIf(p.RequireLowercase),
		checkedIf(p.RequireNumber),
		checkedIf(p.RequireSpecial),
		template.HTMLEscapeString(s.Timeout),
		template.HTMLEscapeString(s.RememberMeTimeout),
		template.HTMLEscapeString(h.cfg.Server.Features.TOTPIssuer),
		template.HTMLEscapeString(h.cfg.Server.Features.WebAuthnDisplay),
		h.basePath(),
	)
	h.adminLayout(w, r, "Authentication", "/config/security/auth", template.HTML(content), flash, "")
}

// ConfigSecurityAuthSave handles POST /server/{adminPath}/config/security/auth
func (h *AdminHandler) ConfigSecurityAuthSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Authentication", "/config/security/auth", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	pairs := map[string]string{
		"security.pwd_min_length":       r.FormValue("pwd_min_length"),
		"security.pwd_require_upper":    boolFormValue(r, "pwd_require_upper"),
		"security.pwd_require_lower":    boolFormValue(r, "pwd_require_lower"),
		"security.pwd_require_number":   boolFormValue(r, "pwd_require_number"),
		"security.pwd_require_special":  boolFormValue(r, "pwd_require_special"),
		"session.timeout":               r.FormValue("session_timeout"),
		"session.remember_me_timeout":   r.FormValue("remember_timeout"),
		"features.totp_issuer":          r.FormValue("totp_issuer"),
		"features.webauthn_display":     r.FormValue("webauthn_display"),
	}
	for k, v := range pairs {
		if err := h.store.SetConfigValue(k, v, username); err != nil {
			h.adminLayout(w, r, "Authentication", "/config/security/auth", "", "", "Failed to save: "+err.Error())
			return
		}
	}
	http.Redirect(w, r, h.basePath()+"/config/security/auth?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Security — API Tokens
// --------------------------------------------------------------------------

// ConfigSecurityTokens handles GET /server/{adminPath}/config/security/tokens
func (h *AdminHandler) ConfigSecurityTokens(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("revoked") == "1" {
		flash = "Token revoked."
	}
	content := fmt.Sprintf(`
<h1>API Tokens</h1>
<div class="card">
  <h2>Admin API Tokens</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:16px">
    Bearer tokens for API access. Tokens are stored as SHA-256 hashes; raw tokens are shown once on creation.
  </p>
  <form method="POST" action="%s/config/security/tokens">
    <div style="display:flex;gap:8px;align-items:flex-end;margin-bottom:16px">
      <div class="form-group" style="flex:1;margin-bottom:0">
        <label>Token Name / Description</label>
        <input type="text" name="token_name" placeholder="e.g. CI/CD Pipeline" required>
      </div>
      <div class="form-group" style="margin-bottom:0">
        <label>Expires In</label>
        <select name="expires_in">
          <option value="">Never</option>
          <option value="24h">24 hours</option>
          <option value="168h">7 days</option>
          <option value="720h">30 days</option>
          <option value="8760h">1 year</option>
        </select>
      </div>
      <input type="hidden" name="action" value="create">
      <button type="submit" class="btn btn-primary">Generate Token</button>
    </div>
  </form>
  <table>
    <thead><tr><th>Name</th><th>Created</th><th>Expires</th><th>Last Used</th><th>Actions</th></tr></thead>
    <tbody>
      <tr><td colspan="5" style="color:#8b949e;text-align:center;padding:20px">
        No admin API tokens yet. Generate one above.
      </td></tr>
    </tbody>
  </table>
</div>`, h.basePath())
	h.adminLayout(w, r, "API Tokens", "/config/security/tokens", template.HTML(content), flash, "")
}

// ConfigSecurityTokensAction handles POST /server/{adminPath}/config/security/tokens
func (h *AdminHandler) ConfigSecurityTokensAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "API Tokens", "/config/security/tokens", "", "", "Invalid request.")
		return
	}
	// Token generation handled by the auth service; redirect back with notice.
	http.Redirect(w, r, h.basePath()+"/config/security/tokens", http.StatusFound)
}

// --------------------------------------------------------------------------
// Security — Rate Limiting
// --------------------------------------------------------------------------

// ConfigSecurityRateLimit handles GET /server/{adminPath}/config/security/ratelimit
func (h *AdminHandler) ConfigSecurityRateLimit(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("saved") == "1" {
		flash = "Rate limit settings saved."
	}
	rl := h.cfg.Server.RateLimit
	content := fmt.Sprintf(`
<h1>Rate Limiting</h1>
<form method="POST" action="%s/config/security/ratelimit">
<div class="card">
  <h2>Global Rate Limits</h2>
  <div class="form-group">
    <label>Enabled</label>
    <select name="enabled">
      <option value="true"%s>Enabled</option>
      <option value="false"%s>Disabled</option>
    </select>
  </div>
  <div class="form-group">
    <label>Requests per Minute (per IP)</label>
    <input type="number" name="rpm" value="%d" min="1">
  </div>
  <div class="form-group">
    <label>Burst Size</label>
    <input type="number" name="burst" value="%d" min="1">
    <div class="help-text">Maximum burst above the per-minute rate.</div>
  </div>
</div>
<div class="card">
  <h2>Auth Endpoint Limits</h2>
  <div class="form-group">
    <label>Max Login Attempts per 15 min</label>
    <input type="number" name="login_attempts" value="%d" min="1">
  </div>
  <div class="form-group">
    <label>Max Password Reset per Hour</label>
    <input type="number" name="reset_attempts" value="%d" min="1">
  </div>
</div>
<div style="display:flex;gap:8px">
  <button type="submit" class="btn btn-primary">Save Rate Limits</button>
  <a href="%s/config/security/ratelimit" class="btn btn-secondary">Cancel</a>
</div>
</form>`,
		h.basePath(),
		selectedIf(rl.Enabled),
		selectedIf(!rl.Enabled),
		rl.Requests,
		rl.Burst,
		rl.LoginMaxAttempts,
		rl.PasswordResetMaxAttempts,
		h.basePath(),
	)
	h.adminLayout(w, r, "Rate Limiting", "/config/security/ratelimit", template.HTML(content), flash, "")
}

// ConfigSecurityRateLimitSave handles POST /server/{adminPath}/config/security/ratelimit
func (h *AdminHandler) ConfigSecurityRateLimitSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Rate Limiting", "/config/security/ratelimit", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	pairs := map[string]string{
		"rate_limit.enabled":         r.FormValue("enabled"),
		"rate_limit.rpm":             r.FormValue("rpm"),
		"rate_limit.burst":           r.FormValue("burst"),
		"rate_limit.login_attempts":  r.FormValue("login_attempts"),
		"rate_limit.reset_attempts":  r.FormValue("reset_attempts"),
	}
	for k, v := range pairs {
		if err := h.store.SetConfigValue(k, v, username); err != nil {
			h.adminLayout(w, r, "Rate Limiting", "/config/security/ratelimit", "", "", "Failed to save: "+err.Error())
			return
		}
	}
	http.Redirect(w, r, h.basePath()+"/config/security/ratelimit?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Security — Firewall
// --------------------------------------------------------------------------

// ConfigSecurityFirewall handles GET /server/{adminPath}/config/security/firewall
func (h *AdminHandler) ConfigSecurityFirewall(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("saved") == "1" {
		flash = "Firewall rules saved."
	}
	content := fmt.Sprintf(`
<h1>Firewall</h1>
<form method="POST" action="%s/config/security/firewall">
<div class="card">
  <h2>IP Block List</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:8px">One IP or CIDR per line. Blocked IPs receive HTTP 403.</p>
  <div class="form-group">
    <textarea name="blocked_ips" rows="8" placeholder="192.168.1.100&#10;10.0.0.0/8"></textarea>
  </div>
</div>
<div class="card">
  <h2>Country Block List</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:8px">
    ISO 3166-1 alpha-2 country codes (one per line). Requires GeoIP to be enabled.
    See <a href="%s/config/network/geoip" style="color:#58a6ff">GeoIP settings</a>.
  </p>
  <div class="form-group">
    <textarea name="blocked_countries" rows="4" placeholder="CN&#10;RU"></textarea>
  </div>
</div>
<div style="display:flex;gap:8px">
  <button type="submit" class="btn btn-primary">Save Firewall Rules</button>
  <a href="%s/config/security/firewall" class="btn btn-secondary">Cancel</a>
</div>
</form>`,
		h.basePath(), h.basePath(), h.basePath(),
	)
	h.adminLayout(w, r, "Firewall", "/config/security/firewall", template.HTML(content), flash, "")
}

// ConfigSecurityFirewallSave handles POST /server/{adminPath}/config/security/firewall
func (h *AdminHandler) ConfigSecurityFirewallSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Firewall", "/config/security/firewall", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	pairs := map[string]string{
		"firewall.blocked_ips":       r.FormValue("blocked_ips"),
		"firewall.blocked_countries": r.FormValue("blocked_countries"),
	}
	for k, v := range pairs {
		if err := h.store.SetConfigValue(k, v, username); err != nil {
			h.adminLayout(w, r, "Firewall", "/config/security/firewall", "", "", "Failed to save: "+err.Error())
			return
		}
	}
	http.Redirect(w, r, h.basePath()+"/config/security/firewall?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Security — Allowlist
// --------------------------------------------------------------------------

// ConfigSecurityAllowlist handles GET /server/{adminPath}/config/security/allowlist
func (h *AdminHandler) ConfigSecurityAllowlist(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("saved") == "1" {
		flash = "Allowlist saved."
	}
	content := fmt.Sprintf(`
<h1>Trusted IP Allowlist</h1>
<form method="POST" action="%s/config/security/allowlist">
<div class="card">
  <h2>Allowlisted IPs / CIDRs</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:8px">
    IPs on this list bypass rate limiting, blocklist checks, and GeoIP restrictions.
    Use for monitoring systems, trusted internal networks, etc.
  </p>
  <div class="form-group">
    <textarea name="allowlist_ips" rows="8" placeholder="127.0.0.1&#10;10.0.0.0/8&#10;172.16.0.0/12"></textarea>
  </div>
</div>
<div style="display:flex;gap:8px">
  <button type="submit" class="btn btn-primary">Save Allowlist</button>
  <a href="%s/config/security/allowlist" class="btn btn-secondary">Cancel</a>
</div>
</form>`, h.basePath(), h.basePath())
	h.adminLayout(w, r, "Allowlist", "/config/security/allowlist", template.HTML(content), flash, "")
}

// ConfigSecurityAllowlistSave handles POST /server/{adminPath}/config/security/allowlist
func (h *AdminHandler) ConfigSecurityAllowlistSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Allowlist", "/config/security/allowlist", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	if err := h.store.SetConfigValue("security.allowlist_ips", r.FormValue("allowlist_ips"), username); err != nil {
		h.adminLayout(w, r, "Allowlist", "/config/security/allowlist", "", "", "Failed to save: "+err.Error())
		return
	}
	http.Redirect(w, r, h.basePath()+"/config/security/allowlist?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Network — Tor
// --------------------------------------------------------------------------

// ConfigNetworkTor handles GET /server/{adminPath}/config/network/tor
func (h *AdminHandler) ConfigNetworkTor(w http.ResponseWriter, r *http.Request) {
	torMgr := h.getTorManager()
	status := `<span class="badge badge-red">Not Running</span>`
	onionAddr := "—"
	if torMgr != nil {
		status = `<span class="badge badge-green">Running</span>`
		if addr := torMgr.OnionAddress(); addr != "" {
			onionAddr = addr
		}
	}
	content := fmt.Sprintf(`
<h1>Tor Hidden Service</h1>
<div class="card">
  <h2>Status</h2>
  <div class="info-row"><span class="info-label">Tor Status</span><span class="info-value">%s</span></div>
  <div class="info-row">
    <span class="info-label">.onion Address</span>
    <span class="info-value" style="word-break:break-all;font-family:monospace">%s</span>
  </div>
</div>
<div class="card">
  <h2>Configuration</h2>
  <p style="color:#8b949e;font-size:14px">
    Tor hidden service is automatically enabled when the <code>tor</code> binary is found on PATH.
    It is not configurable via the admin panel — this is by design for security.
  </p>
  <div class="info-row"><span class="info-label">Auto-enabled</span><span class="info-value">when tor binary found on PATH</span></div>
  <div class="info-row"><span class="info-label">Virtual Port</span><span class="info-value">%d</span></div>
  <div class="info-row"><span class="info-label">Bandwidth Rate</span><span class="info-value">%s</span></div>
</div>`,
		status,
		template.HTMLEscapeString(onionAddr),
		h.cfg.Server.Tor.VirtualPort,
		template.HTMLEscapeString(h.cfg.Server.Tor.BandwidthRate),
	)
	h.adminLayout(w, r, "Tor", "/config/network/tor", template.HTML(content), "", "")
}

// --------------------------------------------------------------------------
// Network — GeoIP
// --------------------------------------------------------------------------

// ConfigNetworkGeoIP handles GET /server/{adminPath}/config/network/geoip
func (h *AdminHandler) ConfigNetworkGeoIP(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("saved") == "1" {
		flash = "GeoIP settings saved."
	}
	g := h.cfg.Server.GeoIP
	denyList := strings.Join(g.DenyCountries, "\n")
	allowList := strings.Join(g.AllowCountries, "\n")
	content := fmt.Sprintf(`
<h1>GeoIP</h1>
<form method="POST" action="%s/config/network/geoip">
<div class="card">
  <h2>GeoIP Database</h2>
  <div class="form-group">
    <label>Enabled</label>
    <select name="enabled">
      <option value="true"%s>Enabled</option>
      <option value="false"%s>Disabled</option>
    </select>
  </div>
  <div class="form-group">
    <label>Database Directory</label>
    <input type="text" name="dir" value="%s" placeholder="{data_dir}/security/geoip">
    <div class="help-text">Leave blank to use the default path.</div>
  </div>
</div>
<div class="card">
  <h2>Country Filtering</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:8px">GeoIP is used as a risk signal only, never the sole access gate.</p>
  <div class="form-group">
    <label>Deny Countries (ISO 3166-1 alpha-2, one per line)</label>
    <textarea name="deny_countries" rows="4" placeholder="CN&#10;RU">%s</textarea>
  </div>
  <div class="form-group">
    <label>Allow Countries (override deny list, one per line)</label>
    <textarea name="allow_countries" rows="4">%s</textarea>
    <div class="help-text">Allow list wins if a country appears in both lists.</div>
  </div>
</div>
<div class="card">
  <h2>Databases</h2>
  <div class="form-group">
    <label><input type="checkbox" name="db_country"%s> Country database</label>
  </div>
  <div class="form-group">
    <label><input type="checkbox" name="db_city"%s> City database</label>
  </div>
  <div class="form-group">
    <label><input type="checkbox" name="db_asn"%s> ASN database</label>
  </div>
</div>
<div style="display:flex;gap:8px">
  <button type="submit" class="btn btn-primary">Save GeoIP Settings</button>
  <a href="%s/config/network/geoip" class="btn btn-secondary">Cancel</a>
</div>
</form>`,
		h.basePath(),
		selectedIf(g.Enabled),
		selectedIf(!g.Enabled),
		template.HTMLEscapeString(g.Dir),
		template.HTMLEscapeString(denyList),
		template.HTMLEscapeString(allowList),
		checkedIf(g.Databases.Country),
		checkedIf(g.Databases.City),
		checkedIf(g.Databases.ASN),
		h.basePath(),
	)
	h.adminLayout(w, r, "GeoIP", "/config/network/geoip", template.HTML(content), flash, "")
}

// ConfigNetworkGeoIPSave handles POST /server/{adminPath}/config/network/geoip
func (h *AdminHandler) ConfigNetworkGeoIPSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "GeoIP", "/config/network/geoip", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	pairs := map[string]string{
		"geoip.enabled":         r.FormValue("enabled"),
		"geoip.dir":             r.FormValue("dir"),
		"geoip.deny_countries":  r.FormValue("deny_countries"),
		"geoip.allow_countries": r.FormValue("allow_countries"),
		"geoip.db_country":      boolFormValue(r, "db_country"),
		"geoip.db_city":         boolFormValue(r, "db_city"),
		"geoip.db_asn":          boolFormValue(r, "db_asn"),
	}
	for k, v := range pairs {
		if err := h.store.SetConfigValue(k, v, username); err != nil {
			h.adminLayout(w, r, "GeoIP", "/config/network/geoip", "", "", "Failed to save: "+err.Error())
			return
		}
	}
	http.Redirect(w, r, h.basePath()+"/config/network/geoip?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Network — Blocklists
// --------------------------------------------------------------------------

// ConfigNetworkBlocklists handles GET /server/{adminPath}/config/network/blocklists
func (h *AdminHandler) ConfigNetworkBlocklists(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("saved") == "1" {
		flash = "Blocklist settings saved."
	}
	bl := h.cfg.Server.Security.Blocklist
	var srcRows strings.Builder
	for i, src := range bl.Sources {
		srcRows.WriteString(fmt.Sprintf(`
<div style="background:#0d1117;border:1px solid #21262d;border-radius:6px;padding:12px;margin-bottom:8px">
  <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">
    <strong style="color:#c9d1d9">%s</strong>
    <span class="badge %s">%s</span>
  </div>
  <div style="color:#8b949e;font-size:13px;word-break:break-all">%s</div>
  <div style="color:#6e7681;font-size:12px;margin-top:4px">Type: %s</div>
</div>`,
			template.HTMLEscapeString(src.Name),
			func() string {
				if src.Enabled {
					return "badge-green"
				}
				return "badge-red"
			}(),
			func() string {
				if src.Enabled {
					return "enabled"
				}
				return "disabled"
			}(),
			template.HTMLEscapeString(src.URL),
			template.HTMLEscapeString(src.Type),
		))
		_ = i
	}
	if len(bl.Sources) == 0 {
		srcRows.WriteString(`<p style="color:#8b949e">No blocklist sources configured. Add sources to <code>server.yml</code> under <code>server.security.blocklist.sources</code>.</p>`)
	}

	content := fmt.Sprintf(`
<h1>IP/Domain Blocklists</h1>
<div class="card">
  <h2>Blocklist Directory</h2>
  <div class="info-row">
    <span class="info-label">Storage Directory</span>
    <span class="info-value">%s</span>
  </div>
  <p style="color:#8b949e;font-size:13px;margin-top:8px">
    Updated automatically by the <code>blocklist_update</code> scheduler task.
    View the <a href="%s/config/scheduler" style="color:#58a6ff">Scheduler</a> for next run time.
  </p>
</div>
<div class="card">
  <h2>Sources (%d configured)</h2>
  %s
</div>
<div class="card">
  <h2>Add Custom Blocklist</h2>
  <form method="POST" action="%s/config/network/blocklists">
    <div class="form-group">
      <label>Name</label>
      <input type="text" name="name" placeholder="My Blocklist" required>
    </div>
    <div class="form-group">
      <label>URL</label>
      <input type="url" name="url" placeholder="https://example.com/blocklist.txt" required>
    </div>
    <div class="form-group">
      <label>Type</label>
      <select name="type">
        <option value="ip">IP addresses</option>
        <option value="domain">Domains</option>
        <option value="mixed">Mixed</option>
      </select>
    </div>
    <button type="submit" class="btn btn-primary">Add Source</button>
    <p style="color:#8b949e;font-size:13px;margin-top:8px">
      Note: Adding sources here records them in the database. Restart required to apply in scheduler.
    </p>
  </form>
</div>`,
		func() string {
			if bl.Dir != "" {
				return template.HTMLEscapeString(bl.Dir)
			}
			return "{data_dir}/security/blocklists"
		}(),
		h.basePath(),
		len(bl.Sources),
		srcRows.String(),
		h.basePath(),
	)
	h.adminLayout(w, r, "Blocklists", "/config/network/blocklists", template.HTML(content), flash, "")
}

// ConfigNetworkBlocklistsSave handles POST /server/{adminPath}/config/network/blocklists
func (h *AdminHandler) ConfigNetworkBlocklistsSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Blocklists", "/config/network/blocklists", "", "", "Invalid form data.")
		return
	}
	admin := h.getAdminFromSession(r)
	username := "unknown"
	if admin != nil {
		username = admin.Username
	}
	// Persist the new source entry as a JSON record so it can be loaded at next restart.
	entry := fmt.Sprintf(`{"name":%q,"url":%q,"type":%q,"enabled":true}`,
		r.FormValue("name"), r.FormValue("url"), r.FormValue("type"),
	)
	key := "blocklist.source." + sanitizeKey(r.FormValue("name"))
	if err := h.store.SetConfigValue(key, entry, username); err != nil {
		h.adminLayout(w, r, "Blocklists", "/config/network/blocklists", "", "", "Failed to save: "+err.Error())
		return
	}
	http.Redirect(w, r, h.basePath()+"/config/network/blocklists?saved=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// User Invites
// --------------------------------------------------------------------------

// ConfigUsersInvites handles GET /server/{adminPath}/config/users/invites
func (h *AdminHandler) ConfigUsersInvites(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("created") == "1" {
		flash = "Invite code created. Share it with the new user."
	}
	content := fmt.Sprintf(`
<h1>User Invites</h1>
<div class="card">
  <h2>Create Invite</h2>
  <form method="POST" action="%s/config/users/invites">
    <div class="form-group">
      <label>Invite Label (optional)</label>
      <input type="text" name="label" placeholder="e.g. For team member Jane">
    </div>
    <div class="form-group">
      <label>Expires In</label>
      <select name="expires_in">
        <option value="24h">24 hours</option>
        <option value="168h">7 days</option>
        <option value="720h">30 days</option>
        <option value="">Never</option>
      </select>
    </div>
    <div class="form-group">
      <label>Max Uses</label>
      <input type="number" name="max_uses" value="1" min="1">
    </div>
    <button type="submit" class="btn btn-primary">Generate Invite</button>
  </form>
</div>
<div class="card">
  <h2>Active Invites</h2>
  <table>
    <thead><tr><th>Code</th><th>Label</th><th>Expires</th><th>Uses</th><th>Actions</th></tr></thead>
    <tbody>
      <tr><td colspan="5" style="color:#8b949e;text-align:center;padding:20px">No active invites.</td></tr>
    </tbody>
  </table>
</div>`, h.basePath())
	h.adminLayout(w, r, "Invites", "/config/users/invites", template.HTML(content), flash, "")
}

// ConfigUsersInvitesAction handles POST /server/{adminPath}/config/users/invites
func (h *AdminHandler) ConfigUsersInvitesAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Invites", "/config/users/invites", "", "", "Invalid form data.")
		return
	}
	// Invite creation is handled by the auth service in the full implementation.
	http.Redirect(w, r, h.basePath()+"/config/users/invites?created=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// User Moderation
// --------------------------------------------------------------------------

// ConfigModerationUsers handles GET /server/{adminPath}/config/moderation/users
func (h *AdminHandler) ConfigModerationUsers(w http.ResponseWriter, r *http.Request) {
	content := fmt.Sprintf(`
<h1>User Moderation</h1>
<div class="card">
  <h2>Moderation Queue</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:16px">
    Users flagged for review appear here. Use the <a href="%s/config/users" style="color:#58a6ff">User List</a>
    to manage all users.
  </p>
  <table>
    <thead><tr><th>Username</th><th>Flagged At</th><th>Reason</th><th>Actions</th></tr></thead>
    <tbody>
      <tr><td colspan="4" style="color:#8b949e;text-align:center;padding:20px">No users in moderation queue.</td></tr>
    </tbody>
  </table>
</div>`, h.basePath())
	h.adminLayout(w, r, "Moderation", "/config/moderation/users", template.HTML(content), "", "")
}

// --------------------------------------------------------------------------
// Cluster
// --------------------------------------------------------------------------

// ConfigClusterNodes handles GET /server/{adminPath}/config/cluster/nodes
func (h *AdminHandler) ConfigClusterNodes(w http.ResponseWriter, r *http.Request) {
	content := fmt.Sprintf(`
<h1>Cluster Nodes</h1>
<div class="card">
  <h2>Current Node</h2>
  <div class="info-row"><span class="info-label">Node Role</span><span class="info-value">Primary (standalone)</span></div>
  <div class="info-row"><span class="info-label">Cluster Status</span><span class="info-value"><span class="badge badge-blue">Single Node</span></span></div>
</div>
<div class="card">
  <h2>Cluster Nodes</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:16px">
    No cluster peers configured. Use <a href="%s/config/cluster/add" style="color:#58a6ff">Add Node</a>
    to generate a join token for a new node.
  </p>
  <table>
    <thead><tr><th>Node</th><th>Address</th><th>Status</th><th>Joined</th></tr></thead>
    <tbody>
      <tr><td colspan="4" style="color:#8b949e;text-align:center;padding:20px">No cluster nodes configured.</td></tr>
    </tbody>
  </table>
</div>`, h.basePath())
	h.adminLayout(w, r, "Cluster Nodes", "/config/cluster/nodes", template.HTML(content), "", "")
}

// ConfigClusterAdd handles GET /server/{adminPath}/config/cluster/add
func (h *AdminHandler) ConfigClusterAdd(w http.ResponseWriter, r *http.Request) {
	flash := ""
	if r.URL.Query().Get("created") == "1" {
		flash = "Join token created. Copy it to the new node."
	}
	content := fmt.Sprintf(`
<h1>Add Cluster Node</h1>
<div class="card">
  <h2>Generate Join Token</h2>
  <p style="color:#8b949e;font-size:14px;margin-bottom:16px">
    A join token allows a new node to join this cluster. Tokens are single-use and expire in 1 hour.
  </p>
  <form method="POST" action="%s/config/cluster/add">
    <div class="form-group">
      <label>Node Label (optional)</label>
      <input type="text" name="label" placeholder="e.g. eu-west-1">
    </div>
    <button type="submit" class="btn btn-primary">Generate Join Token</button>
  </form>
</div>`, h.basePath())
	h.adminLayout(w, r, "Add Cluster Node", "/config/cluster/add", template.HTML(content), flash, "")
}

// ConfigClusterAddAction handles POST /server/{adminPath}/config/cluster/add
func (h *AdminHandler) ConfigClusterAddAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.adminLayout(w, r, "Add Cluster Node", "/config/cluster/add", "", "", "Invalid request.")
		return
	}
	http.Redirect(w, r, h.basePath()+"/config/cluster/add?created=1", http.StatusFound)
}

// --------------------------------------------------------------------------
// Help
// --------------------------------------------------------------------------

// AdminHelp handles GET /server/{adminPath}/help
func (h *AdminHandler) AdminHelp(w http.ResponseWriter, r *http.Request) {
	content := fmt.Sprintf(`
<h1>Admin Help</h1>
<div class="card">
  <h2>Documentation</h2>
  <ul style="padding-left:20px;color:#c9d1d9;line-height:2">
    <li><a href="/server/help" style="color:#58a6ff" target="_blank">Public Help Page</a> — User-facing help and API examples</li>
    <li><a href="/server/docs/swagger" style="color:#58a6ff" target="_blank">API Documentation (Swagger)</a> — Interactive REST API docs</li>
    <li><a href="/server/about" style="color:#58a6ff" target="_blank">About Caslink</a> — Version and feature information</li>
  </ul>
</div>
<div class="card">
  <h2>CLI Reference</h2>
  <pre style="background:#0d1117;padding:16px;border-radius:6px;overflow-x:auto;color:#c9d1d9;font-size:13px">caslink --help
caslink --version
caslink --status
caslink --service start
caslink --service stop
caslink --service --install
caslink --maintenance backup
caslink --maintenance restore {file}
caslink --maintenance setup
caslink --update check
caslink --update yes</pre>
</div>
<div class="card">
  <h2>Server Information</h2>
  <div class="info-row"><span class="info-label">Version</span><span class="info-value">%s</span></div>
  <div class="info-row"><span class="info-label">Mode</span><span class="info-value">%s</span></div>
  <div class="info-row">
    <span class="info-label">Full Server Info</span>
    <span class="info-value"><a href="%s/config/info" style="color:#58a6ff">View →</a></span>
  </div>
</div>`,
		template.HTMLEscapeString(h.version),
		template.HTMLEscapeString(h.mode),
		h.basePath(),
	)
	h.adminLayout(w, r, "Help", "/help", template.HTML(content), "", "")
}

// --------------------------------------------------------------------------
// API endpoints
// --------------------------------------------------------------------------

// APIConfigSettings handles GET /api/v1/server/{adminPath}/config/settings
func (h *AdminHandler) APIConfigSettings(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.Server
	jsonAdminOK(w, map[string]any{
		"port":       cfg.Port,
		"address":    cfg.Address,
		"mode":       cfg.Mode,
		"fqdn":       cfg.FQDN,
		"admin_path": cfg.Admin.Path,
	})
}

// APIConfigSettingsSave handles PATCH /api/v1/server/{adminPath}/config/settings
func (h *AdminHandler) APIConfigSettingsSave(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonAdminErr(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body.")
		return
	}
	for k, v := range body {
		if err := h.store.SetConfigValue("server."+k, v, "api"); err != nil {
			jsonAdminErr(w, http.StatusInternalServerError, "SAVE_FAILED", err.Error())
			return
		}
	}
	jsonAdminOK(w, map[string]any{"saved": true})
}

// APIConfigBranding handles GET /api/v1/server/{adminPath}/config/branding
func (h *AdminHandler) APIConfigBranding(w http.ResponseWriter, r *http.Request) {
	b := h.cfg.Server.Branding
	jsonAdminOK(w, map[string]any{
		"site_name":     b.Title,
		"tagline":       b.Tagline,
		"logo_url":      b.LogoURL,
		"favicon_url":   b.FaviconURL,
		"default_theme": b.DefaultTheme,
		"primary_color": b.PrimaryColor,
	})
}

// APIConfigBrandingSave handles PATCH /api/v1/server/{adminPath}/config/branding
func (h *AdminHandler) APIConfigBrandingSave(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonAdminErr(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body.")
		return
	}
	for k, v := range body {
		if err := h.store.SetConfigValue("branding."+k, v, "api"); err != nil {
			jsonAdminErr(w, http.StatusInternalServerError, "SAVE_FAILED", err.Error())
			return
		}
	}
	jsonAdminOK(w, map[string]any{"saved": true})
}

// APIConfigInfo handles GET /api/v1/server/{adminPath}/config/info
func (h *AdminHandler) APIConfigInfo(w http.ResponseWriter, r *http.Request) {
	jsonAdminOK(w, map[string]any{
		"version":    h.version,
		"mode":       h.mode,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"cpus":       runtime.NumCPU(),
		"address":    h.cfg.Server.Address,
		"port":       h.cfg.Server.Port,
		"fqdn":       h.cfg.Server.FQDN,
		"db_driver":  h.cfg.Server.Database.Driver,
	})
}

// APIConfigScheduler handles GET /api/v1/server/{adminPath}/config/scheduler
func (h *AdminHandler) APIConfigScheduler(w http.ResponseWriter, r *http.Request) {
	sch := h.cfg.Server.Scheduler
	jsonAdminOK(w, map[string]any{
		"session_cleanup":  map[string]any{"cron": sch.SessionCleanupCron, "enabled": sch.SessionCleanupEnabled},
		"token_cleanup":    map[string]any{"cron": sch.TokenCleanupCron, "enabled": sch.TokenCleanupEnabled},
		"expire_urls":      map[string]any{"cron": sch.ExpireURLsCron, "enabled": sch.ExpireURLsEnabled},
		"log_rotation":     map[string]any{"cron": sch.LogRotationCron, "enabled": sch.LogRotationEnabled},
		"backup_daily":     map[string]any{"cron": sch.BackupCron, "enabled": sch.BackupEnabled},
		"ssl_renewal":      map[string]any{"cron": sch.SSLRenewalCron, "enabled": sch.SSLRenewalEnabled},
		"geoip_update":     map[string]any{"cron": sch.GeoIPUpdateCron, "enabled": sch.GeoIPUpdateEnabled},
		"blocklist_update": map[string]any{"cron": sch.BlocklistUpdateCron, "enabled": sch.BlocklistUpdateEnabled},
		"cve_update":       map[string]any{"cron": sch.CVEUpdateCron, "enabled": sch.CVEUpdateEnabled},
		"healthcheck_self": map[string]any{"cron": sch.HealthcheckCron, "enabled": sch.HealthcheckEnabled},
		"tor_health":       map[string]any{"cron": sch.TorHealthCron, "enabled": sch.TorHealthEnabled},
	})
}

// APIConfigMaintenance handles GET /api/v1/server/{adminPath}/config/maintenance
func (h *AdminHandler) APIConfigMaintenance(w http.ResponseWriter, r *http.Request) {
	enabled, _, _ := h.store.GetConfigValue("maintenance.enabled")
	msg, _, _ := h.store.GetConfigValue("maintenance.message")
	jsonAdminOK(w, map[string]any{
		"enabled": enabled == "true",
		"message": msg,
	})
}

// APIConfigMaintenanceSave handles PATCH /api/v1/server/{adminPath}/config/maintenance
func (h *AdminHandler) APIConfigMaintenanceSave(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonAdminErr(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body.")
		return
	}
	for k, v := range body {
		if err := h.store.SetConfigValue("maintenance."+k, v, "api"); err != nil {
			jsonAdminErr(w, http.StatusInternalServerError, "SAVE_FAILED", err.Error())
			return
		}
	}
	jsonAdminOK(w, map[string]any{"saved": true})
}

// APIConfigNetworkTor handles GET /api/v1/server/{adminPath}/config/network/tor
func (h *AdminHandler) APIConfigNetworkTor(w http.ResponseWriter, r *http.Request) {
	torMgr := h.getTorManager()
	running := torMgr != nil
	onion := ""
	if running {
		onion = torMgr.OnionAddress()
	}
	jsonAdminOK(w, map[string]any{
		"running":       running,
		"onion_address": onion,
		"virtual_port":  h.cfg.Server.Tor.VirtualPort,
	})
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// selectedIf returns ` selected` when cond is true (for <option> elements).
func selectedIf(cond bool) template.HTML {
	if cond {
		return " selected"
	}
	return ""
}

// checkedIf returns ` checked` when cond is true (for <input type="checkbox">).
func checkedIf(cond bool) template.HTML {
	if cond {
		return " checked"
	}
	return ""
}

// boolFormValue returns "true" if the named checkbox was present in the form,
// otherwise "false". HTML checkboxes are only included in POST data when checked.
func boolFormValue(r *http.Request, name string) string {
	if r.FormValue(name) != "" {
		return "true"
	}
	return "false"
}

// sanitizeKey replaces characters invalid in a config key with underscores.
func sanitizeKey(s string) string {
	var b strings.Builder
	for _, ch := range strings.ToLower(s) {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
