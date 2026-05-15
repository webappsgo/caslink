package handler

import (
	"net/http"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/tmpl"
)

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
