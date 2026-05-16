package handler

import (
	"html/template"
	"net/http"
	"strings"
	"unicode"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// SetupHandler handles first-run setup wizard
type SetupHandler struct {
	authService *service.AuthService
	cfg         *config.Config
	version     string
}

// NewSetupHandler creates a new setup handler
func NewSetupHandler(authService *service.AuthService, cfg *config.Config, version string) *SetupHandler {
	return &SetupHandler{
		authService: authService,
		cfg:         cfg,
		version:     version,
	}
}

// SetupPage handles GET /setup - shows setup wizard if needed
func (h *SetupHandler) SetupPage(w http.ResponseWriter, r *http.Request) {
	h.renderSetupForm(w, r, "", "")
}

// Setup handles POST /setup - creates primary admin
func (h *SetupHandler) Setup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderSetupForm(w, r, "", "Invalid form data")
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")
	email := r.FormValue("email")

	// Required fields
	if username == "" || password == "" || email == "" {
		h.renderSetupForm(w, r, username, "All fields are required")
		return
	}

	if len(username) < 3 {
		h.renderSetupForm(w, r, username, "Username must be at least 3 characters")
		return
	}

	// Validate password against policy from config
	if errMsg := h.validatePassword(password); errMsg != "" {
		h.renderSetupForm(w, r, username, errMsg)
		return
	}

	if password != confirmPassword {
		h.renderSetupForm(w, r, username, "Passwords do not match")
		return
	}

	// Create primary admin
	err := h.authService.CreatePrimaryAdmin(r.Context(), username, password, email)
	if err != nil {
		h.renderSetupForm(w, r, username, "Failed to create admin account")
		return
	}

	// Redirect to admin login
	h.renderSetupComplete(w, username)
}

// validatePassword checks the submitted password against the configured
// password policy (AI.md PART 17 — Password Policy Sane Defaults).
// Returns an empty string on success, or a human-readable error message.
func (h *SetupHandler) validatePassword(pw string) string {
	policy := h.cfg.Server.Security.Password
	minLen := policy.MinLength
	if minLen <= 0 {
		minLen = 8
	}

	if len(pw) < minLen {
		return strings.Join([]string{
			"Password must be at least",
			itoa(minLen),
			"characters",
		}, " ")
	}

	if policy.RequireUppercase {
		ok := false
		for _, r := range pw {
			if unicode.IsUpper(r) {
				ok = true
				break
			}
		}
		if !ok {
			return "Password must contain at least one uppercase letter"
		}
	}

	if policy.RequireLowercase {
		ok := false
		for _, r := range pw {
			if unicode.IsLower(r) {
				ok = true
				break
			}
		}
		if !ok {
			return "Password must contain at least one lowercase letter"
		}
	}

	if policy.RequireNumber {
		ok := false
		for _, r := range pw {
			if unicode.IsDigit(r) {
				ok = true
				break
			}
		}
		if !ok {
			return "Password must contain at least one number"
		}
	}

	if policy.RequireSpecial {
		ok := false
		for _, r := range pw {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				ok = true
				break
			}
		}
		if !ok {
			return "Password must contain at least one special character"
		}
	}

	return ""
}

// itoa converts a non-negative integer to a decimal string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// csrfTokenFromRequest reads the CSRF cookie value so the form can include it
// as a hidden field (double-submit cookie pattern per AI.md PART 11).
// Returns "" if no cookie is present (GET handler calls ensureCSRFCookie first,
// so the cookie should always exist by the time this is called).
func csrfTokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie("csrf_token"); err == nil {
		return c.Value
	}
	return ""
}

// renderSetupForm renders the setup wizard form
func (h *SetupHandler) renderSetupForm(w http.ResponseWriter, r *http.Request, prefillUsername, errorMsg string) {
	csrfToken := csrfTokenFromRequest(r)

	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Setup - Caslink</title>
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
        .setup-card {
            background: #161b22;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 40px;
            width: 100%%;
            max-width: 500px;
            box-shadow: 0 8px 24px rgba(0,0,0,0.3);
        }
        h1 { color: #58a6ff; margin-bottom: 8px; text-align: center; }
        .subtitle { color: #8b949e; text-align: center; margin-bottom: 30px; font-size: 14px; }
        .step-info {
            background: #0d1117;
            border: 1px solid #30363d;
            border-radius: 6px;
            padding: 16px;
            margin-bottom: 24px;
            color: #8b949e;
            font-size: 14px;
            line-height: 1.6;
        }
        .form-group { margin-bottom: 20px; }
        label { display: block; margin-bottom: 6px; color: #c9d1d9; font-size: 14px; font-weight: 500; }
        input[type="text"], input[type="password"], input[type="email"] {
            width: 100%%;
            padding: 10px;
            background: #0d1117;
            border: 1px solid #30363d;
            border-radius: 6px;
            color: #c9d1d9;
            font-size: 14px;
        }
        input:focus { outline: none; border-color: #58a6ff; }
        button {
            width: 100%%;
            padding: 12px;
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
        .help-text {
            color: #6e7681;
            font-size: 12px;
            margin-top: 4px;
        }
        .version { text-align: center; margin-top: 30px; color: #6e7681; font-size: 12px; }
    </style>
</head>
<body>
    <div class="setup-card">
        <h1>Welcome to Caslink</h1>
        <div class="subtitle">First-Time Setup</div>

        <div class="step-info">
            Let's create your admin account. This account will have full control over the server.
        </div>

        {{if .Error}}
        <div class="error">{{.Error}}</div>
        {{end}}

        <form method="POST" action="/setup">
            <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" value="{{.Username}}" required autofocus>
                <div class="help-text">At least 3 characters</div>
            </div>

            <div class="form-group">
                <label for="email">Email</label>
                <input type="email" id="email" name="email" required>
                <div class="help-text">Used for notifications and password recovery</div>
            </div>

            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
                <div class="help-text">{{.PasswordHint}}</div>
            </div>

            <div class="form-group">
                <label for="confirm_password">Confirm Password</label>
                <input type="password" id="confirm_password" name="confirm_password" required>
            </div>

            <button type="submit">Create Admin Account</button>
        </form>

        <div class="version">Version {{.Version}}</div>
    </div>
</body>
</html>`

	t := template.Must(template.New("setup").Parse(tmpl))
	data := map[string]interface{}{
		"Username":     prefillUsername,
		"Error":        errorMsg,
		"Version":      h.version,
		"CSRFToken":    csrfToken,
		"PasswordHint": h.passwordHint(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = t.Execute(w, data)
}

// passwordHint builds a human-readable description of the active password
// policy to show below the password field.
func (h *SetupHandler) passwordHint() string {
	p := h.cfg.Server.Security.Password
	minLen := p.MinLength
	if minLen <= 0 {
		minLen = 8
	}

	parts := []string{"At least " + itoa(minLen) + " characters"}
	if p.RequireUppercase {
		parts = append(parts, "one uppercase letter")
	}
	if p.RequireLowercase {
		parts = append(parts, "one lowercase letter")
	}
	if p.RequireNumber {
		parts = append(parts, "one number")
	}
	if p.RequireSpecial {
		parts = append(parts, "one special character")
	}
	return strings.Join(parts, ", ")
}

// renderSetupComplete renders the setup completion page
func (h *SetupHandler) renderSetupComplete(w http.ResponseWriter, username string) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Setup Complete - Caslink</title>
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
        .complete-card {
            background: #161b22;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 40px;
            width: 100%%;
            max-width: 500px;
            box-shadow: 0 8px 24px rgba(0,0,0,0.3);
            text-align: center;
        }
        .success-icon {
            font-size: 64px;
            color: #3fb950;
            margin-bottom: 20px;
        }
        h1 { color: #3fb950; margin-bottom: 16px; }
        .message {
            color: #8b949e;
            margin-bottom: 30px;
            line-height: 1.6;
        }
        .info-box {
            background: #0d1117;
            border: 1px solid #30363d;
            border-radius: 6px;
            padding: 20px;
            margin-bottom: 30px;
            text-align: left;
        }
        .info-box strong { color: #58a6ff; }
        a {
            display: inline-block;
            padding: 12px 24px;
            background: #238636;
            color: white;
            text-decoration: none;
            border-radius: 6px;
            font-weight: 600;
        }
        a:hover { background: #2ea043; }
    </style>
</head>
<body>
    <div class="complete-card">
        <div class="success-icon">✓</div>
        <h1>Setup Complete!</h1>
        <div class="message">
            Your admin account has been created successfully.
        </div>
        <div class="info-box">
            <p><strong>Username:</strong> {{.Username}}</p>
            <p style="margin-top: 12px; color: #8b949e; font-size: 14px;">
                You can now log in to the admin panel to configure your server.
            </p>
        </div>
        <a href="/server/admin">Go to Admin Login</a>
    </div>
</body>
</html>`

	t := template.Must(template.New("complete").Parse(tmpl))
	data := map[string]interface{}{
		"Username": username,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = t.Execute(w, data)
}
