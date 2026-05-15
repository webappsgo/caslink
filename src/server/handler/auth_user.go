package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/server/model"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/tmpl"
	"github.com/casjaysdevdocker/caslink/src/server/validate"
)

// AuthUserHandler handles user authentication and registration
type AuthUserHandler struct {
	authService *service.AuthService
	renderer    *tmpl.Renderer
	cfg         *config.Config
}

// NewAuthUserHandler creates a new user auth handler
func NewAuthUserHandler(authService *service.AuthService, renderer *tmpl.Renderer, cfg *config.Config) *AuthUserHandler {
	return &AuthUserHandler{
		authService: authService,
		renderer:    renderer,
		cfg:         cfg,
	}
}

// RegisterPage renders the registration page
func (h *AuthUserHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		tmpl.Data
		Error    string
		Username string
		Email    string
	}{
		Data: newPageData(h.cfg, r, "Create Account", nil),
	}
	h.renderer.Render(w, "template/page/auth/register.html", data)
}

// Register handles user registration (JSON and form)
func (h *AuthUserHandler) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	isForm := strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")

	var username, email, password string

	if isForm {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		username = r.PostFormValue("username")
		email = r.PostFormValue("email")
		password = r.PostFormValue("password")
	} else {
		var req model.RegisterUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		username = req.Username
		email = req.Email
		password = req.Password
	}

	renderErr := func(msg, savedUser, savedEmail string) {
		data := struct {
			tmpl.Data
			Error    string
			Username string
			Email    string
		}{
			Data:     newPageData(h.cfg, r, "Create Account", nil),
			Error:    msg,
			Username: savedUser,
			Email:    savedEmail,
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderer.Render(w, "template/page/auth/register.html", data)
	}

	if err := validate.ValidateUsername(username, false); err != nil {
		if isForm {
			renderErr(err.Error(), username, email)
			return
		}
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := validate.ValidateEmail(email); err != nil {
		if isForm {
			renderErr(err.Error(), username, email)
			return
		}
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if len(password) < 8 {
		if isForm {
			renderErr("Password must be at least 8 characters", username, email)
			return
		}
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Password must be at least 8 characters"})
		return
	}

	user, err := h.authService.RegisterUser(ctx, username, email, password)
	if err != nil {
		if isForm {
			renderErr("Unable to complete registration", username, email)
			return
		}
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Unable to complete registration"})
		return
	}

	sessionID, err := h.authService.CreateUserSession(ctx, user.ID, false)
	if err != nil {
		if isForm {
			renderErr("Registration succeeded but session creation failed", username, email)
			return
		}
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Registration succeeded but session creation failed",
		})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "user_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	if isForm {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"user":    user,
	})
}

// LoginPage renders the login page
func (h *AuthUserHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		tmpl.Data
		Error      string
		Identifier string
	}{
		Data: newPageData(h.cfg, r, "Sign In", nil),
	}
	h.renderer.Render(w, "template/page/auth/login.html", data)
}

// Login handles user login (JSON and form)
func (h *AuthUserHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	isForm := strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")

	var identifier, password string
	var rememberMe bool

	if isForm {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		identifier = r.PostFormValue("identifier")
		password = r.PostFormValue("password")
		rememberMe = r.PostFormValue("remember_me") == "on"
	} else {
		var req model.LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		identifier = req.Identifier
		password = req.Password
		rememberMe = req.RememberMe
	}

	renderErr := func(msg, savedID string) {
		data := struct {
			tmpl.Data
			Error      string
			Identifier string
		}{
			Data:       newPageData(h.cfg, r, "Sign In", nil),
			Error:      msg,
			Identifier: savedID,
		}
		w.WriteHeader(http.StatusUnauthorized)
		h.renderer.Render(w, "template/page/auth/login.html", data)
	}

	user, err := h.authService.AuthenticateUser(ctx, identifier, password)
	if err != nil {
		if isForm {
			renderErr("Invalid credentials", identifier)
			return
		}
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid credentials"})
		return
	}

	// Check if user has 2FA enabled
	if user.TOTPEnabled {
		tempSession, err := h.authService.CreateUserSession(ctx, user.ID, false)
		if err != nil {
			if isForm {
				renderErr("Failed to create 2FA session", identifier)
				return
			}
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Failed to create 2FA session",
			})
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "2fa_pending",
			Value:    tempSession,
			Path:     "/",
			MaxAge:   300,
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
		})

		if isForm {
			http.Redirect(w, r, "/server/auth/2fa", http.StatusSeeOther)
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"requires_2fa":  true,
			"session_token": tempSession,
			"redirect":      "/server/auth/2fa",
		})
		return
	}

	maxAge := 7 * 24 * 60 * 60
	if rememberMe {
		maxAge = 30 * 24 * 60 * 60
	}

	sessionID, err := h.authService.CreateUserSession(ctx, user.ID, rememberMe)
	if err != nil {
		if isForm {
			renderErr("Authentication succeeded but session creation failed", identifier)
			return
		}
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Authentication succeeded but session creation failed",
		})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "user_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	if isForm {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"user":    user,
	})
}

// Logout handles user logout
func (h *AuthUserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cookie, err := r.Cookie("user_session")
	if err == nil && cookie.Value != "" {
		h.authService.DeleteSession(ctx, cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "user_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	if r.Header.Get("Accept") == "application/json" {
		respondJSON(w, http.StatusOK, map[string]bool{"success": true})
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}
