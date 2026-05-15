package handler

import (
	"encoding/json"
	"net/http"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/tmpl"
)

// APIResponse is the canonical envelope for all JSON responses per
// AI.md PART 9 ("Response Format") and IDEA.md "API surface".
//
// Success: {"ok": true, "data": {...}}
// Error:   {"ok": false, "error": "CODE", "message": "..."}
type APIResponse struct {
	OK      bool        `json:"ok"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// respondJSON sends a canonical success envelope: {"ok":true,"data":data}.
// Pass http.StatusOK / StatusCreated etc.; the status is written before the
// body. The shape never varies — callers MUST NOT pre-wrap data themselves.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIResponse{OK: true, Data: data})
}

// respondError sends a canonical error envelope:
// {"ok":false,"error":"CODE","message":"..."}.
//
// The HTTP status determines the error code via errCodeFromStatus so that
// existing call sites (which historically passed only an HTTP status + a
// message) keep working unchanged while emitting the canonical shape.
func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIResponse{
		OK:      false,
		Error:   errCodeFromStatus(status),
		Message: message,
	})
}

// errCodeFromStatus maps an HTTP status code to the canonical error code
// listed in AI.md PART 9 → "Error Codes".
func errCodeFromStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusMethodNotAllowed:
		return "METHOD_NOT_ALLOWED"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusGone:
		return "GONE"
	case http.StatusUnprocessableEntity:
		return "VALIDATION_FAILED"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusServiceUnavailable:
		return "MAINTENANCE"
	default:
		if status >= 500 {
			return "SERVER_ERROR"
		}
		return "ERROR"
	}
}

// ContextKey is the typed key used to attach values to request contexts.
// It MUST match the key used by the server.UserAuthMiddleware so that
// handlers see the value the middleware stored.
type ContextKey string

const (
	// UserContextKey is the context key for the authenticated *service.User.
	UserContextKey ContextKey = "user"
	// AdminContextKey is the context key for the authenticated *service.Admin.
	AdminContextKey ContextKey = "admin"
)

// getUserFromRequest returns the authenticated user attached by the
// UserAuthMiddleware, or (nil, false) if no user is in context.
func getUserFromRequest(r *http.Request) (*service.User, bool) {
	user, ok := r.Context().Value(UserContextKey).(*service.User)
	return user, ok
}

// csrfToken returns the CSRF token from the csrf_token cookie.
func csrfToken(r *http.Request) string {
	if c, err := r.Cookie("csrf_token"); err == nil {
		return c.Value
	}
	return ""
}

// newPageData builds a base tmpl.Data from config and request, optionally
// attaching the authenticated user. Extra fields are set by the caller.
func newPageData(cfg *config.Config, r *http.Request, title string, user *service.User) tmpl.Data {
	appName := cfg.Server.Branding.Title
	if appName == "" {
		appName = "Caslink"
	}
	appDesc := cfg.Server.Branding.Description
	if appDesc == "" {
		appDesc = "Self-hosted URL shortener"
	}
	return tmpl.Data{
		AppName:   appName,
		AppDesc:   appDesc,
		Title:     title,
		CSRFToken: csrfToken(r),
		Theme:     "dark",
		User:      user,
	}
}
