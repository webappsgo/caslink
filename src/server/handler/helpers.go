package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/webappsgo/caslink/src/common/i18n"
	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/tmpl"
)

// realClientIP extracts the real client IP, respecting X-Forwarded-For /
// X-Real-IP. Trust is enforced upstream: realIPMiddleware strips these
// headers for any peer that is not a configured trusted proxy, so by the
// time a handler calls this the headers are only present for trusted
// proxy chains (mirrors server.realIP, which this package cannot import
// directly without an import cycle).
func realClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.SplitN(fwd, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return real
	}
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// apiActorID returns the bearer token owner's ID for audit-log attribution
// on Bearer-authenticated API requests, or nil if unauthenticated.
func apiActorID(r *http.Request) *int64 {
	rec, ok := getBearerFromRequest(r)
	if !ok || rec == nil {
		return nil
	}
	id := rec.OwnerID
	return &id
}

// splitFormList splits a comma-separated form field (tags, geo_countries)
// into a trimmed, non-empty slice.
func splitFormList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// APIResponse is the canonical envelope for all JSON responses per
// AI.md PART 9 ("Response Format") and IDEA.md "API surface".
//
// Success (single):  {"ok": true, "data": {...}}
// Success (list):    {"ok": true, "data": [...], "pagination": {...}}
// Error:             {"ok": false, "error": "CODE", "message": "..."}
type APIResponse struct {
	OK         bool        `json:"ok"`
	Data       interface{} `json:"data,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
	Error      string      `json:"error,omitempty"`
	Message    string      `json:"message,omitempty"`
}

// Pagination holds list pagination metadata per AI.md PART 9.
type Pagination struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
	Pages int `json:"pages"`
}

// NewPagination builds a Pagination value and clamps limit to sane bounds.
func NewPagination(page, limit, total int) *Pagination {
	if limit <= 0 {
		limit = 250
	}
	if limit > 250 {
		limit = 250
	}
	if page <= 0 {
		page = 1
	}
	pages := total / limit
	if total%limit != 0 {
		pages++
	}
	return &Pagination{Page: page, Limit: limit, Total: total, Pages: pages}
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
	// BearerContextKey is the context key for the *service.TokenRecord
	// attached by server.BearerAuthMiddleware on Bearer-authenticated API
	// requests.
	BearerContextKey ContextKey = "bearer_user"
)

// getUserFromRequest returns the authenticated user attached by the
// UserAuthMiddleware, or (nil, false) if no user is in context.
func getUserFromRequest(r *http.Request) (*service.User, bool) {
	user, ok := r.Context().Value(UserContextKey).(*service.User)
	return user, ok
}

// getBearerFromRequest returns the *service.TokenRecord attached by
// server.BearerAuthMiddleware, or (nil, false) if the request was not
// Bearer-authenticated.
func getBearerFromRequest(r *http.Request) (*service.TokenRecord, bool) {
	rec, ok := r.Context().Value(BearerContextKey).(*service.TokenRecord)
	return rec, ok
}

// csrfToken returns the CSRF token from the csrf_token cookie.
func csrfToken(r *http.Request) string {
	if c, err := r.Cookie("csrf_token"); err == nil {
		return c.Value
	}
	return ""
}

// newPageData builds a base tmpl.Data from config and request, optionally
// attaching the authenticated user. Includes language info for the UI selector
// per AI.md PART 31.
func newPageData(cfg *config.Config, r *http.Request, title string, user *service.User) tmpl.Data {
	appName := cfg.Server.Branding.Title
	if appName == "" {
		appName = "Caslink"
	}
	appDesc := cfg.Server.Branding.Description
	if appDesc == "" {
		appDesc = "Self-hosted URL shortener"
	}

	// Populate language selector data (PART 31).
	activeLang := i18n.LangFromContext(r.Context())
	langs := i18n.Languages()
	var opts []tmpl.LangOption
	for _, l := range langs {
		opts = append(opts, tmpl.LangOption{
			Code:       l.Code,
			NativeName: l.NativeName,
			Active:     l.Code == activeLang,
		})
	}

	return tmpl.Data{
		AppName:            appName,
		AppDesc:            appDesc,
		Title:              title,
		CSRFToken:          csrfToken(r),
		Theme:              "dark",
		Lang:               activeLang,
		AvailableLanguages: opts,
		User:               user,
	}
}
