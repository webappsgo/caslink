package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/tmpl"
)

// UserHandler handles user profile and settings pages
type UserHandler struct {
	authService  *service.AuthService
	tokenService *service.TokenService
	urlService   *service.URLService
	renderer     *tmpl.Renderer
	cfg          *config.Config
}

// NewUserHandler creates a new user handler
func NewUserHandler(
	authService *service.AuthService,
	tokenService *service.TokenService,
	urlService *service.URLService,
	renderer *tmpl.Renderer,
	cfg *config.Config,
) *UserHandler {
	return &UserHandler{
		authService:  authService,
		tokenService: tokenService,
		urlService:   urlService,
		renderer:     renderer,
		cfg:          cfg,
	}
}

// Profile renders the user profile page
func (h *UserHandler) Profile(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data := struct {
		tmpl.Data
	}{
		Data: newPageData(h.cfg, r, "Profile", user),
	}
	h.renderer.Render(w, "template/page/users/profile.html", data)
}

// Settings renders the user settings page
func (h *UserHandler) Settings(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data := struct {
		tmpl.Data
		DisplayName string
		Bio         string
	}{
		Data: newPageData(h.cfg, r, "Settings", user),
	}
	h.renderer.Render(w, "template/page/users/settings.html", data)
}

// Tokens renders the API tokens management page (GET) and handles create/revoke (POST)
func (h *UserHandler) Tokens(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()

	type pageData struct {
		tmpl.Data
		Tokens   []*service.TokenRecord
		NewToken string
		Flash    *tmpl.Flash
	}

	renderPage := func(newToken string, flash *tmpl.Flash) {
		tokens, _ := h.tokenService.ListTokens(ctx, user.ID)
		base := newPageData(h.cfg, r, "API Tokens", user)
		base.Flash = flash
		d := pageData{
			Data:     base,
			Tokens:   tokens,
			NewToken: newToken,
		}
		h.renderer.Render(w, "template/page/users/tokens.html", d)
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		action := r.PostFormValue("action")
		switch action {
		case "create":
			name := strings.TrimSpace(r.PostFormValue("token_name"))
			if name == "" {
				renderPage("", &tmpl.Flash{Type: "danger", Message: "Token name is required."})
				return
			}
			var expiresAt *time.Time
			if days := r.PostFormValue("expires_in"); days != "" && days != "0" {
				if n, err := strconv.Atoi(days); err == nil && n > 0 {
					t := time.Now().AddDate(0, 0, n)
					expiresAt = &t
				}
			}
			plain, err := h.tokenService.CreateToken(ctx, user.ID, "user", name, nil, expiresAt)
			if err != nil {
				renderPage("", &tmpl.Flash{Type: "danger", Message: "Failed to create token."})
				return
			}
			renderPage(plain, &tmpl.Flash{Type: "success", Message: "Token created. Copy it now — it will not be shown again."})
			return

		case "revoke":
			idStr := r.PostFormValue("token_id")
			tokenID, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				renderPage("", &tmpl.Flash{Type: "danger", Message: "Invalid token ID."})
				return
			}
			if err := h.tokenService.RevokeToken(ctx, tokenID, user.ID); err != nil {
				renderPage("", &tmpl.Flash{Type: "danger", Message: "Failed to revoke token."})
				return
			}
			http.Redirect(w, r, "/users/tokens", http.StatusSeeOther)
			return
		}
	}

	renderPage("", nil)
}

// Security renders the security settings page
func (h *UserHandler) Security(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data := struct {
		tmpl.Data
		TOTPEnabled      bool
		RecoveryKeyCount int
	}{
		Data:        newPageData(h.cfg, r, "Security Settings", user),
		TOTPEnabled: user.TOTPEnabled,
	}
	h.renderer.Render(w, "template/page/users/security.html", data)
}

// Dashboard renders the user dashboard with their URLs
func (h *UserHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusFound)
		return
	}
	ctx := r.Context()

	urls, _ := h.urlService.ListByUser(ctx, user.ID, 50)

	data := struct {
		tmpl.Data
		URLs interface{}
	}{
		Data: newPageData(h.cfg, r, "Dashboard", user),
		URLs: urls,
	}
	h.renderer.Render(w, "template/page/dashboard.html", data)
}
