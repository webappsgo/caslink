package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// AdminHandler handles admin panel endpoints
type AdminHandler struct {
	authService     *service.AuthService
	userAdminService *service.UserAdminService
	version         string
	mode            string
	adminPath       string // configurable admin path segment, default "admin"
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(authService *service.AuthService, userAdminService *service.UserAdminService, version, mode, adminPath string) *AdminHandler {
	if adminPath == "" {
		adminPath = "admin"
	}
	return &AdminHandler{
		authService:     authService,
		userAdminService: userAdminService,
		version:         version,
		mode:            mode,
		adminPath:       adminPath,
	}
}

// basePath returns the full admin URL prefix (e.g., "/server/admin").
func (h *AdminHandler) basePath() string {
	return "/server/" + h.adminPath
}

// LoginPage handles GET /admin - shows login form if not authenticated
func (h *AdminHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	// Check if setup is needed
	needsSetup, err := h.authService.NeedsSetup(r.Context())
	if err == nil && needsSetup {
		// Redirect to setup wizard
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}

	// Check if already authenticated
	if admin := h.getAdminFromSession(r); admin != nil {
		http.Redirect(w, r, h.basePath()+"/dashboard", http.StatusFound)
		return
	}

	h.renderLogin(w, "", "")
}

// Login handles POST /admin/login - processes login form
func (h *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderLogin(w, "", "Invalid form data")
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	rememberMe := r.FormValue("remember_me") == "on"

	// Authenticate
	admin, err := h.authService.AuthenticateAdmin(r.Context(), username, password)
	if err != nil {
		h.renderLogin(w, username, "Invalid username or password")
		return
	}

	// Create session
	sessionID, err := h.authService.CreateSession(r.Context(), admin.ID, rememberMe)
	if err != nil {
		h.renderLogin(w, username, "Failed to create session")
		return
	}

	// Set session cookie
	expiration := 30 * 24 * time.Hour
	if rememberMe {
		expiration = 90 * 24 * time.Hour
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    sessionID,
		Path:     h.basePath(),
		Expires:  time.Now().Add(expiration),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})

	// Redirect to dashboard
	http.Redirect(w, r, h.basePath()+"/dashboard", http.StatusFound)
}

// Logout handles GET /admin/logout
func (h *AdminHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Get session ID from cookie
	cookie, err := r.Cookie("admin_session")
	if err == nil {
		// Delete session from database
		_ = h.authService.DeleteSession(r.Context(), cookie.Value)
	}

	// Delete cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    "",
		Path:     h.basePath(),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Redirect to login
	http.Redirect(w, r, h.basePath(), http.StatusFound)
}

// Dashboard handles GET /admin/dashboard
func (h *AdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	admin := h.getAdminFromSession(r)
	if admin == nil {
		http.Redirect(w, r, h.basePath(), http.StatusFound)
		return
	}

	h.renderDashboard(w, admin)
}

// getAdminFromSession retrieves the authenticated admin from session
func (h *AdminHandler) getAdminFromSession(r *http.Request) *service.Admin {
	cookie, err := r.Cookie("admin_session")
	if err != nil {
		return nil
	}

	admin, err := h.authService.ValidateSession(r.Context(), cookie.Value)
	if err != nil {
		return nil
	}

	return admin
}

// renderLogin renders the login page
func (h *AdminHandler) renderLogin(w http.ResponseWriter, username, errorMsg string) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Admin Login - Caslink</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #0d1117;
            color: #c9d1d9;
            display: flex;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
        }
        .login-card {
            background: #161b22;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 40px;
            width: 100%%;
            max-width: 400px;
            box-shadow: 0 8px 24px rgba(0,0,0,0.3);
        }
        h1 { color: #58a6ff; margin-bottom: 8px; text-align: center; }
        .subtitle { color: #8b949e; text-align: center; margin-bottom: 30px; font-size: 14px; }
        .form-group { margin-bottom: 20px; }
        label { display: block; margin-bottom: 6px; color: #c9d1d9; font-size: 14px; }
        input[type="text"], input[type="password"] {
            width: 100%%;
            padding: 10px;
            background: #0d1117;
            border: 1px solid #30363d;
            border-radius: 6px;
            color: #c9d1d9;
            font-size: 14px;
        }
        input:focus { outline: none; border-color: #58a6ff; }
        .checkbox-group { display: flex; align-items: center; margin-bottom: 20px; }
        .checkbox-group input { margin-right: 8px; }
        .checkbox-group label { margin-bottom: 0; }
        button {
            width: 100%%;
            padding: 10px;
            background: #238636;
            color: white;
            border: none;
            border-radius: 6px;
            font-size: 14px;
            cursor: pointer;
            font-weight: 600;
        }
        button:hover { background: #2ea043; }
        .error {
            background: #da3633;
            color: white;
            padding: 10px;
            border-radius: 6px;
            margin-bottom: 20px;
            font-size: 14px;
        }
        .version { text-align: center; margin-top: 30px; color: #6e7681; font-size: 12px; }
    </style>
</head>
<body>
    <div class="login-card">
        <h1>Caslink</h1>
        <div class="subtitle">Admin Panel</div>
        {{if .Error}}
        <div class="error">{{.Error}}</div>
        {{end}}
        <form method="POST" action="{{.BasePath}}/login">
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" value="{{.Username}}" required autofocus>
            </div>
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
            </div>
            <div class="checkbox-group">
                <input type="checkbox" id="remember_me" name="remember_me">
                <label for="remember_me">Remember me (90 days)</label>
            </div>
            <button type="submit">Login</button>
        </form>
        <div class="version">Version {{.Version}}</div>
    </div>
</body>
</html>`

	t := template.Must(template.New("login").Parse(tmpl))
	data := map[string]interface{}{
		"Username": username,
		"Error":    errorMsg,
		"Version":  h.version,
		"BasePath": h.basePath(),
	}
	_ = t.Execute(w, data)
}

// renderDashboard renders the admin dashboard
func (h *AdminHandler) renderDashboard(w http.ResponseWriter, admin *service.Admin) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Dashboard - Caslink Admin</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #0d1117;
            color: #c9d1d9;
        }
        .header {
            background: #161b22;
            border-bottom: 1px solid #30363d;
            padding: 16px 24px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .logo { color: #58a6ff; font-size: 20px; font-weight: 600; }
        .user-info { display: flex; align-items: center; gap: 16px; }
        .username { color: #8b949e; }
        .logout { color: #f85149; text-decoration: none; }
        .logout:hover { text-decoration: underline; }
        .container { padding: 24px; max-width: 1400px; margin: 0 auto; }
        h1 { margin-bottom: 24px; }
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 16px;
            margin-bottom: 24px;
        }
        .stat-card {
            background: #161b22;
            border: 1px solid #30363d;
            border-radius: 6px;
            padding: 20px;
        }
        .stat-label { color: #8b949e; font-size: 14px; margin-bottom: 8px; }
        .stat-value { font-size: 32px; font-weight: 600; color: #58a6ff; }
        .info-card {
            background: #161b22;
            border: 1px solid #30363d;
            border-radius: 6px;
            padding: 20px;
            margin-bottom: 16px;
        }
        .info-card h2 { margin-bottom: 16px; color: #58a6ff; font-size: 18px; }
        .info-row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #21262d; }
        .info-row:last-child { border-bottom: none; }
        .info-label { color: #8b949e; }
        .info-value { color: #c9d1d9; }
    </style>
</head>
<body>
    <div class="header">
        <div class="logo">Caslink Admin</div>
        <div class="user-info">
            <span class="username">{{.Admin.Username}}</span>
            <a href="{{.BasePath}}/logout" class="logout">Logout</a>
        </div>
    </div>
    <div class="container">
        <h1>Dashboard</h1>
        <div class="stats">
            <div class="stat-card">
                <div class="stat-label">Status</div>
                <div class="stat-value" style="color: #3fb950;">● Online</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Mode</div>
                <div class="stat-value">{{.Mode}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Version</div>
                <div class="stat-value" style="font-size: 24px;">{{.Version}}</div>
            </div>
        </div>
        <div class="info-card">
            <h2>Server Information</h2>
            <div class="info-row">
                <span class="info-label">Server Status</span>
                <span class="info-value">Running</span>
            </div>
            <div class="info-row">
                <span class="info-label">Admin Logged In</span>
                <span class="info-value">{{.Admin.Username}}</span>
            </div>
            <div class="info-row">
                <span class="info-label">Mode</span>
                <span class="info-value">{{.Mode}}</span>
            </div>
        </div>
        <div class="info-card">
            <h2>Quick Actions</h2>
            <p style="color: #8b949e; padding: 16px;">Admin panel pages coming in next phases...</p>
            <p style="color: #8b949e; padding: 0 16px;">This is a skeleton admin panel created during Phase 7.</p>
        </div>
    </div>
</body>
</html>`

	t := template.Must(template.New("dashboard").Parse(tmpl))
	data := map[string]interface{}{
		"Admin":    admin,
		"Version":  h.version,
		"Mode":     h.mode,
		"BasePath": h.basePath(),
	}
	_ = t.Execute(w, data)
}

// UserList handles GET /server/{adminPath}/config/users
func (h *AdminHandler) UserList(w http.ResponseWriter, r *http.Request) {
	admin := h.getAdminFromSession(r)
	if admin == nil {
		http.Redirect(w, r, h.basePath(), http.StatusFound)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	search := r.URL.Query().Get("q")

	users, total, err := h.userAdminService.ListUsers(r.Context(), page, 50, search)
	if err != nil {
		http.Error(w, "Failed to load users", http.StatusInternalServerError)
		return
	}

	userListTmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Users - Caslink Admin</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0d1117; color: #c9d1d9; }
        .header { background: #161b22; border-bottom: 1px solid #30363d; padding: 16px 24px; display: flex; justify-content: space-between; align-items: center; }
        .logo { color: #58a6ff; font-size: 20px; font-weight: 600; }
        .container { padding: 24px; max-width: 1400px; margin: 0 auto; }
        h1 { margin-bottom: 24px; }
        .search-bar { margin-bottom: 16px; }
        .search-bar input { padding: 8px 12px; background: #0d1117; border: 1px solid #30363d; border-radius: 6px; color: #c9d1d9; width: 300px; }
        .search-bar button { padding: 8px 16px; background: #238636; color: white; border: none; border-radius: 6px; cursor: pointer; margin-left: 8px; }
        table { width: 100%%; border-collapse: collapse; background: #161b22; border-radius: 6px; overflow: hidden; }
        th { background: #21262d; padding: 12px 16px; text-align: left; color: #8b949e; font-size: 13px; }
        td { padding: 12px 16px; border-bottom: 1px solid #21262d; font-size: 14px; }
        tr:last-child td { border-bottom: none; }
        .badge-suspended { background: #da3633; color: white; padding: 2px 8px; border-radius: 12px; font-size: 12px; }
        .badge-active { background: #238636; color: white; padding: 2px 8px; border-radius: 12px; font-size: 12px; }
        .action-link { color: #58a6ff; text-decoration: none; margin-right: 8px; }
        .total { color: #8b949e; margin-bottom: 12px; font-size: 14px; }
    </style>
</head>
<body>
    <div class="header">
        <div class="logo">Caslink Admin</div>
        <a href="{{.BasePath}}/logout" style="color:#f85149;">Logout</a>
    </div>
    <div class="container">
        <h1>Users ({{.Total}})</h1>
        <div class="search-bar">
            <form method="GET">
                <input type="text" name="q" value="{{.Search}}" placeholder="Search username or email">
                <button type="submit">Search</button>
            </form>
        </div>
        <table>
            <thead><tr>
                <th>ID</th><th>Username</th><th>Email</th><th>Status</th><th>Created</th><th>Actions</th>
            </tr></thead>
            <tbody>
            {{range .Users}}
            <tr>
                <td>{{.ID}}</td>
                <td>{{.Username}}</td>
                <td>{{.Email}}</td>
                <td>{{if .Suspended}}<span class="badge-suspended">Suspended</span>{{else}}<span class="badge-active">Active</span>{{end}}</td>
                <td>{{.CreatedAt.Format "2006-01-02"}}</td>
                <td>
                    <a class="action-link" href="{{$.BasePath}}/config/users/{{.ID}}">View</a>
                    {{if .Suspended}}
                    <form style="display:inline" method="POST" action="{{$.BasePath}}/config/users/{{.ID}}/activate">
                        <button style="background:none;border:none;color:#58a6ff;cursor:pointer;">Activate</button>
                    </form>
                    {{else}}
                    <form style="display:inline" method="POST" action="{{$.BasePath}}/config/users/{{.ID}}/suspend">
                        <button style="background:none;border:none;color:#f85149;cursor:pointer;">Suspend</button>
                    </form>
                    {{end}}
                </td>
            </tr>
            {{end}}
            </tbody>
        </table>
    </div>
</body>
</html>`

	t := template.Must(template.New("userlist").Parse(userListTmpl))
	_ = t.Execute(w, map[string]interface{}{
		"Admin":    admin,
		"Users":    users,
		"Total":    total,
		"Search":   search,
		"BasePath": h.basePath(),
	})
}

// UserDetail handles GET /server/{adminPath}/config/users/{id}
func (h *AdminHandler) UserDetail(w http.ResponseWriter, r *http.Request) {
	admin := h.getAdminFromSession(r)
	if admin == nil {
		http.Redirect(w, r, h.basePath(), http.StatusFound)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	user, err := h.userAdminService.GetUser(r.Context(), id)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	userDetailTmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>User {{.User.Username}} - Caslink Admin</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0d1117; color: #c9d1d9; }
        .header { background: #161b22; border-bottom: 1px solid #30363d; padding: 16px 24px; display: flex; justify-content: space-between; }
        .logo { color: #58a6ff; font-size: 20px; font-weight: 600; }
        .container { padding: 24px; max-width: 800px; margin: 0 auto; }
        .card { background: #161b22; border: 1px solid #30363d; border-radius: 6px; padding: 24px; margin-bottom: 16px; }
        .row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #21262d; }
        .row:last-child { border-bottom: none; }
        .label { color: #8b949e; }
        .actions { margin-top: 16px; display: flex; gap: 8px; }
        .btn { padding: 8px 16px; border: none; border-radius: 6px; cursor: pointer; font-size: 14px; }
        .btn-danger { background: #da3633; color: white; }
        .btn-success { background: #238636; color: white; }
        .back { color: #58a6ff; text-decoration: none; }
    </style>
</head>
<body>
    <div class="header">
        <div class="logo">Caslink Admin</div>
        <a href="{{.BasePath}}/logout" style="color:#f85149;">Logout</a>
    </div>
    <div class="container">
        <p style="margin-bottom:16px;"><a class="back" href="{{.BasePath}}/config/users">← Users</a></p>
        <div class="card">
            <div class="row"><span class="label">ID</span><span>{{.User.ID}}</span></div>
            <div class="row"><span class="label">Username</span><span>{{.User.Username}}</span></div>
            <div class="row"><span class="label">Email</span><span>{{.User.Email}}</span></div>
            <div class="row"><span class="label">Email Verified</span><span>{{.User.EmailVerified}}</span></div>
            <div class="row"><span class="label">TOTP Enabled</span><span>{{.User.TOTPEnabled}}</span></div>
            <div class="row"><span class="label">Suspended</span><span>{{.User.Suspended}}</span></div>
            {{if .User.SuspendReason}}<div class="row"><span class="label">Suspend Reason</span><span>{{.User.SuspendReason}}</span></div>{{end}}
            <div class="row"><span class="label">Created</span><span>{{.User.CreatedAt.Format "2006-01-02 15:04"}}</span></div>
        </div>
        <div class="actions">
            {{if .User.Suspended}}
            <form method="POST" action="{{.BasePath}}/config/users/{{.User.ID}}/activate">
                <button class="btn btn-success" type="submit">Activate Account</button>
            </form>
            {{else}}
            <form method="POST" action="{{.BasePath}}/config/users/{{.User.ID}}/suspend">
                <button class="btn btn-danger" type="submit">Suspend Account</button>
            </form>
            {{end}}
        </div>
    </div>
</body>
</html>`

	t := template.Must(template.New("userdetail").Parse(userDetailTmpl))
	_ = t.Execute(w, map[string]interface{}{
		"User":     user,
		"BasePath": h.basePath(),
	})
}

// SuspendUser handles POST /server/{adminPath}/config/users/{id}/suspend
func (h *AdminHandler) SuspendUser(w http.ResponseWriter, r *http.Request) {
	admin := h.getAdminFromSession(r)
	if admin == nil {
		http.Redirect(w, r, h.basePath(), http.StatusFound)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := r.ParseForm(); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid form")
		return
	}
	reason := r.FormValue("reason")

	if err := h.userAdminService.SuspendUser(r.Context(), id, reason); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to suspend user: %v", err))
		return
	}

	http.Redirect(w, r, h.basePath()+"/config/users/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// ActivateUser handles POST /server/{adminPath}/config/users/{id}/activate
func (h *AdminHandler) ActivateUser(w http.ResponseWriter, r *http.Request) {
	admin := h.getAdminFromSession(r)
	if admin == nil {
		http.Redirect(w, r, h.basePath(), http.StatusFound)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := h.userAdminService.ActivateUser(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to activate user: %v", err))
		return
	}

	http.Redirect(w, r, h.basePath()+"/config/users/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// apiUserList handles GET /api/v1/server/{adminPath}/config/users
func (h *AdminHandler) APIUserList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	search := r.URL.Query().Get("q")

	users, total, err := h.userAdminService.ListUsers(r.Context(), page, 50, search)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load users")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"users": users,
		"total": total,
		"page":  page,
	})
}

// APIUserDetail handles GET /api/v1/server/{adminPath}/config/users/{id}
func (h *AdminHandler) APIUserDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	user, err := h.userAdminService.GetUser(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found")
		return
	}

	respondJSON(w, http.StatusOK, user)
}

// APISuspendUser handles POST /api/v1/server/{adminPath}/config/users/{id}/suspend
func (h *AdminHandler) APISuspendUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Reason = ""
	}

	if err := h.userAdminService.SuspendUser(r.Context(), id, req.Reason); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to suspend user")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}

// APIActivateUser handles POST /api/v1/server/{adminPath}/config/users/{id}/activate
func (h *AdminHandler) APIActivateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := h.userAdminService.ActivateUser(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to activate user")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

// RegenerateRecoveryKeys handles POST /server/{adminPath}/config/users/{id}/recovery-keys
// Admin-override: force-regenerate all recovery keys for a user regardless of 2FA state.
func (h *AdminHandler) RegenerateRecoveryKeys(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	keys, err := h.userAdminService.ForceRegenerateRecoveryKeys(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to regenerate recovery keys", http.StatusInternalServerError)
		return
	}

	// Render a simple admin confirmation page showing the new keys once.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t := template.Must(template.New("rk").Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>Recovery Keys Regenerated</title></head>
<body>
<h1>Recovery Keys Regenerated</h1>
<p><strong>User ID {{.UserID}}</strong> — new recovery keys (shown once only):</p>
<ol>{{range .Keys}}<li><code>{{.}}</code></li>{{end}}</ol>
<p><a href="{{.BackURL}}">Back to user</a></p>
</body></html>`))
	_ = t.Execute(w, map[string]interface{}{
		"UserID":  id,
		"Keys":    keys,
		"BackURL": fmt.Sprintf("%s/config/users/%d", h.basePath(), id),
	})
}

// APIRegenerateRecoveryKeys handles POST /api/v1/server/{adminPath}/config/users/{id}/recovery-keys
func (h *AdminHandler) APIRegenerateRecoveryKeys(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	keys, err := h.userAdminService.ForceRegenerateRecoveryKeys(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to regenerate recovery keys")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"recovery_keys": keys,
	})
}
