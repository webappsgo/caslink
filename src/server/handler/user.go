package handler

import (
	"fmt"
	"net/http"

	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// UserHandler handles user profile and settings
type UserHandler struct {
	authService *service.AuthService
}

// NewUserHandler creates a new user handler
func NewUserHandler(authService *service.AuthService) *UserHandler {
	return &UserHandler{
		authService: authService,
	}
}

// Profile renders the user profile page
func (h *UserHandler) Profile(w http.ResponseWriter, r *http.Request) {
	// Get user from context (set by UserAuthMiddleware)
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// TODO: Render profile HTML page per PART 17
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>User Profile: %s</h1><p>Profile page will be implemented per PART 17</p>", user.Username)
}

// Settings renders the user settings page
func (h *UserHandler) Settings(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// TODO: Render settings HTML page per PART 17
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>Settings: %s</h1><p>Settings page will be implemented per PART 17</p>", user.Username)
}

// Tokens renders the API tokens management page
func (h *UserHandler) Tokens(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// TODO: Render tokens HTML page per PART 17
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>API Tokens: %s</h1><p>Token management will be implemented per PART 17 and PART 23</p>", user.Username)
}

// Security renders the security settings page
func (h *UserHandler) Security(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// TODO: Render security HTML page per PART 17
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>Security: %s</h1><p>Security page (2FA, passkeys, sessions) will be implemented per PART 17 and PART 23</p>", user.Username)
}

// getUserFromRequest is a helper to get user from middleware context
func getUserFromRequest(r *http.Request) (*service.User, bool) {
	// The user is set by UserAuthMiddleware in server/middleware.go
	// Access via context key "user"
	type contextKey string
	user, ok := r.Context().Value(contextKey("user")).(*service.User)
	return user, ok
}
