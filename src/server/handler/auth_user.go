package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/tmpl"
	"github.com/webappsgo/caslink/src/server/validate"
)

// AuthUserHandler handles user authentication and registration
type AuthUserHandler struct {
	authService   *service.AuthService
	inviteService *service.InviteService
	renderer      *tmpl.Renderer
	cfg           *config.Config
}

// NewAuthUserHandler creates a new user auth handler
func NewAuthUserHandler(authService *service.AuthService, inviteService *service.InviteService, renderer *tmpl.Renderer, cfg *config.Config) *AuthUserHandler {
	return &AuthUserHandler{
		authService:   authService,
		inviteService: inviteService,
		renderer:      renderer,
		cfg:           cfg,
	}
}

// registrationClosedMessage explains why public self-registration is
// unavailable under the current registration mode (PART 34).
func (h *AuthUserHandler) registrationClosedMessage() string {
	switch h.cfg.Server.Features.Users.Registration.NormalizedMode() {
	case "invite":
		return "Registration is invite-only. Please use the invite link sent to you by an administrator."
	case "admin_only":
		return "Accounts are created by an administrator. Contact your administrator to request access."
	default:
		return "Registration is currently closed."
	}
}

// RegisterPage renders the registration page
func (h *AuthUserHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	inviteToken := strings.TrimSpace(r.URL.Query().Get("invite"))
	// A valid user-registration invite permits account creation even when public
	// self-registration is closed (invite / admin_only modes, PART 34).
	if !h.cfg.Server.Features.Users.Registration.PublicSelfRegistrationAllowed() && !h.hasValidRegistrationInvite(r, inviteToken) {
		data := struct {
			tmpl.Data
			Error    string
			Username string
			Email    string
			Invite   string
		}{
			Data:  newPageData(h.cfg, r, "Create Account", nil),
			Error: h.registrationClosedMessage(),
		}
		w.WriteHeader(http.StatusForbidden)
		h.renderer.Render(w, "template/page/auth/register.html", data)
		return
	}

	data := struct {
		tmpl.Data
		Error    string
		Username string
		Email    string
		Invite   string
	}{
		Data:   newPageData(h.cfg, r, "Create Account", nil),
		Invite: inviteToken,
	}
	h.renderer.Render(w, "template/page/auth/register.html", data)
}

// hasValidRegistrationInvite reports whether the supplied plaintext token is a
// currently-consumable user-registration invite (PART 34). An empty token or an
// unconfigured invite service returns false.
func (h *AuthUserHandler) hasValidRegistrationInvite(r *http.Request, token string) bool {
	if token == "" || h.inviteService == nil {
		return false
	}
	// disabled mode rejects every existing unused invite/activation link,
	// even one that is otherwise still valid (PART 34).
	if !h.cfg.Server.Features.Users.Registration.InviteAcceptanceAllowed() {
		return false
	}
	_, err := h.inviteService.ValidateInvite(r.Context(), token, service.InviteKindUserRegistration)
	return err == nil
}

// Register handles user registration (JSON and form)
func (h *AuthUserHandler) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	isForm := strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")

	var username, email, password, inviteToken string

	if isForm {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		username = r.PostFormValue("username")
		email = r.PostFormValue("email")
		password = r.PostFormValue("password")
		inviteToken = strings.TrimSpace(r.PostFormValue("invite"))
	} else {
		var req model.RegisterUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		username = req.Username
		email = req.Email
		password = req.Password
		inviteToken = strings.TrimSpace(req.Invite)
	}

	// Registration is permitted when public self-registration is open, OR the
	// caller presents a valid user-registration invite (invite / admin_only
	// modes, PART 34). The invite is only consumed after the account is created.
	hasInvite := h.hasValidRegistrationInvite(r, inviteToken)
	if !h.cfg.Server.Features.Users.Registration.PublicSelfRegistrationAllowed() && !hasInvite {
		if isForm {
			data := struct {
				tmpl.Data
				Error    string
				Username string
				Email    string
				Invite   string
			}{
				Data:  newPageData(h.cfg, r, "Create Account", nil),
				Error: h.registrationClosedMessage(),
			}
			w.WriteHeader(http.StatusForbidden)
			h.renderer.Render(w, "template/page/auth/register.html", data)
			return
		}
		respondError(w, http.StatusForbidden, h.registrationClosedMessage())
		return
	}

	renderErr := func(msg, savedUser, savedEmail string) {
		data := struct {
			tmpl.Data
			Error    string
			Username string
			Email    string
			Invite   string
		}{
			Data:     newPageData(h.cfg, r, "Create Account", nil),
			Error:    msg,
			Username: savedUser,
			Email:    savedEmail,
			Invite:   inviteToken,
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderer.Render(w, "template/page/auth/register.html", data)
	}

	if err := validate.ValidateUsername(username, false); err != nil {
		if isForm {
			renderErr(err.Error(), username, email)
			return
		}
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validate.ValidateEmail(email); err != nil {
		if isForm {
			renderErr(err.Error(), username, email)
			return
		}
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(password) < 8 {
		if isForm {
			renderErr("Password must be at least 8 characters", username, email)
			return
		}
		respondError(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}

	user, err := h.authService.RegisterUser(ctx, username, email, password)
	if err != nil {
		if isForm {
			renderErr("Unable to complete registration", username, email)
			return
		}
		respondError(w, http.StatusBadRequest, "Unable to complete registration")
		return
	}

	// Consume the invite now that the account exists. A consumption failure here
	// (e.g. the invite was used concurrently) must not orphan the created
	// account, so it is logged best-effort rather than surfaced to the user.
	if hasInvite && h.inviteService != nil {
		if _, cerr := h.inviteService.ConsumeInvite(ctx, inviteToken, service.InviteKindUserRegistration, user.ID); cerr != nil {
			log.Printf("[register] failed to consume invite for user %d: %v", user.ID, cerr)
		}
	}

	sessionID, err := h.authService.CreateUserSession(ctx, user.ID, false)
	if err != nil {
		if isForm {
			renderErr("Registration succeeded but session creation failed", username, email)
			return
		}
		respondError(w, http.StatusInternalServerError, "Registration succeeded but session creation failed")
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
		respondError(w, http.StatusUnauthorized, "Invalid credentials")
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
			respondError(w, http.StatusInternalServerError, "Failed to create 2FA session")
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
		respondError(w, http.StatusInternalServerError, "Authentication succeeded but session creation failed")
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
		// cookie.Value is a user_sessions row (created via
		// CreateUserSession), not an admin_sessions row, so RevokeSession
		// (not DeleteSession, which only targets admin_sessions) is
		// required to actually invalidate it on logout.
		_ = h.authService.RevokeSession(ctx, cookie.Value)
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
