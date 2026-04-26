package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/server/service"
)

type contextKey string

const (
	userContextKey  contextKey = "user"
	adminContextKey contextKey = "admin"
)

// UserAuthMiddleware requires valid user session
func UserAuthMiddleware(authService *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get user_session cookie per PART 23
			cookie, err := r.Cookie("user_session")
			if err != nil || cookie.Value == "" {
				// No session - redirect to login
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				return
			}

			// Validate session
			user, err := authService.ValidateUserSession(r.Context(), cookie.Value)
			if err != nil {
				// Invalid or expired session - redirect to login
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				return
			}

			// Add user to request context
			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminAuthMiddleware requires valid admin session
func AdminAuthMiddleware(authService *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get admin_session cookie per PART 23
			cookie, err := r.Cookie("admin_session")
			if err != nil || cookie.Value == "" {
				// No session - redirect to admin login
				http.Redirect(w, r, "/admin", http.StatusSeeOther)
				return
			}

			// Validate session
			admin, err := authService.ValidateSession(r.Context(), cookie.Value)
			if err != nil {
				// Invalid or expired session - redirect to login
				http.Redirect(w, r, "/admin", http.StatusSeeOther)
				return
			}

			// Add admin to request context
			ctx := context.WithValue(r.Context(), adminContextKey, admin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OrgMemberMiddleware requires user to be a member of the organization
func OrgMemberMiddleware(orgService *service.OrgService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get user from context (must be set by UserAuthMiddleware)
			user, ok := r.Context().Value(userContextKey).(*service.User)
			if !ok || user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Get org slug from URL
			slug := chi.URLParam(r, "slug")
			if slug == "" {
				http.Error(w, "Organization slug required", http.StatusBadRequest)
				return
			}

			// Get organization
			org, err := orgService.GetOrganizationBySlug(r.Context(), slug)
			if err != nil {
				http.Error(w, "Organization not found", http.StatusNotFound)
				return
			}

			// Check membership
			isMember, role, err := orgService.IsMember(r.Context(), org.ID, user.ID)
			if err != nil {
				http.Error(w, "Error checking membership", http.StatusInternalServerError)
				return
			}

			if !isMember {
				http.Error(w, "Not an organization member", http.StatusForbidden)
				return
			}

			// Add org and role to context
			ctx := r.Context()
			ctx = context.WithValue(ctx, "org", org)
			ctx = context.WithValue(ctx, "org_role", role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserFromContext retrieves user from request context
func GetUserFromContext(ctx context.Context) (*service.User, bool) {
	user, ok := ctx.Value(userContextKey).(*service.User)
	return user, ok
}

// GetAdminFromContext retrieves admin from request context
func GetAdminFromContext(ctx context.Context) (*service.Admin, bool) {
	admin, ok := ctx.Value(adminContextKey).(*service.Admin)
	return admin, ok
}
