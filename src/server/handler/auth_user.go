package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/casjaysdevdocker/caslink/src/server/model"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/validate"
)

// AuthUserHandler handles user authentication and registration
type AuthUserHandler struct {
	authService *service.AuthService
}

// NewAuthUserHandler creates a new user auth handler
func NewAuthUserHandler(authService *service.AuthService) *AuthUserHandler {
	return &AuthUserHandler{
		authService: authService,
	}
}

// RegisterPage renders the registration page
func (h *AuthUserHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	// TODO: Render registration HTML page per PART 17 (Web Frontend)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "<h1>User Registration</h1><p>Registration form will be implemented per PART 17</p>")
}

// Register handles user registration
func (h *AuthUserHandler) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var req model.RegisterUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate username per PART 23
	if err := validate.ValidateUsername(req.Username, false); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Validate email per PART 23
	if err := validate.ValidateEmail(req.Email); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Validate password (minimum 8 characters per PART 23)
	if len(req.Password) < 8 {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Password must be at least 8 characters",
		})
		return
	}

	// Register user
	user, err := h.authService.RegisterUser(ctx, req.Username, req.Email, req.Password)
	if err != nil {
		// Generic error per PART 23 (don't reveal if username/email exists)
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Unable to complete registration",
		})
		return
	}

	// Create session
	sessionID, err := h.authService.CreateUserSession(ctx, user.ID, false)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Registration succeeded but session creation failed",
		})
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "user_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60, // 7 days
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Return success
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"user":    user,
	})
}

// LoginPage renders the login page
func (h *AuthUserHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	// TODO: Render login HTML page per PART 17 (Web Frontend)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "<h1>User Login</h1><p>Login form will be implemented per PART 17</p>")
}

// Login handles user login
func (h *AuthUserHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Authenticate user (accepts username or email per PART 23)
	user, err := h.authService.AuthenticateUser(ctx, req.Identifier, req.Password)
	if err != nil {
		// Generic error per PART 23
		respondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Invalid credentials",
		})
		return
	}

	// Check if user has 2FA enabled per PART 23 line 20214-20229
	if user.TOTPEnabled {
		// Create temporary session for 2FA verification per PART 23 line 20287
		tempSession, err := h.authService.CreateUserSession(ctx, user.ID, false)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "Failed to create 2FA session",
			})
			return
		}

		// Set temporary cookie (5 minute expiry)
		http.SetCookie(w, &http.Cookie{
			Name:     "2fa_pending",
			Value:    tempSession,
			Path:     "/",
			MaxAge:   300, // 5 minutes
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
		})

		// Return requires_2fa response per PART 23 line 20287
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"requires_2fa":    true,
			"session_token":   tempSession,
			"redirect":        "/auth/2fa",
		})
		return
	}

	// No 2FA - create full session
	sessionID, err := h.authService.CreateUserSession(ctx, user.ID, req.RememberMe)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Authentication succeeded but session creation failed",
		})
		return
	}

	// Set session cookie
	maxAge := 7 * 24 * 60 * 60 // 7 days
	if req.RememberMe {
		maxAge = 30 * 24 * 60 * 60 // 30 days
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

	// Return success
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"user":    user,
	})
}

// Logout handles user logout
func (h *AuthUserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get session cookie
	cookie, err := r.Cookie("user_session")
	if err == nil && cookie.Value != "" {
		// Delete session from database
		h.authService.DeleteSession(ctx, cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "user_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to home or return success
	if r.Header.Get("Accept") == "application/json" {
		respondJSON(w, http.StatusOK, map[string]bool{"success": true})
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}
