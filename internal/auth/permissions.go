package auth

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// PermissionChecker provides methods for checking permissions
type PermissionChecker struct {
	service *Service
}

// NewPermissionChecker creates a new permission checker
func NewPermissionChecker(service *Service) *PermissionChecker {
	return &PermissionChecker{
		service: service,
	}
}

// RequirePermission returns a middleware that requires a specific permission
func (pc *PermissionChecker) RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := GetAuthContext(r)
			if authCtx == nil || !authCtx.IsAuthenticated() {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			if !authCtx.HasPermission(permission) {
				http.Error(w, "Insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin returns a middleware that requires admin privileges
func (pc *PermissionChecker) RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := GetAuthContext(r)
			if authCtx == nil || !authCtx.IsAuthenticated() {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			if !authCtx.IsAdmin() {
				http.Error(w, "Admin privileges required", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireOwnershipOrAdmin returns a middleware that requires the user to be the owner or admin
func (pc *PermissionChecker) RequireOwnershipOrAdmin(getResourceOwnerID func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := GetAuthContext(r)
			if authCtx == nil || !authCtx.IsAuthenticated() {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Admin can access everything
			if authCtx.IsAdmin() {
				next.ServeHTTP(w, r)
				return
			}

			// Check ownership
			resourceOwnerID := getResourceOwnerID(r)
			if resourceOwnerID == "" || resourceOwnerID != authCtx.GetUserID() {
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OptionalAuth returns a middleware that adds auth context if available but doesn't require it
func (pc *PermissionChecker) OptionalAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try to authenticate but don't fail if not authenticated
			authCtx := pc.tryAuthenticate(r)
			if authCtx != nil {
				r = SetAuthContext(r, authCtx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuthOrAnonymous allows access if user is authenticated OR anonymous access is enabled
func (pc *PermissionChecker) RequireAuthOrAnonymous(allowAnonymous bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := GetAuthContext(r)

			// If authenticated, proceed
			if authCtx != nil && authCtx.IsAuthenticated() {
				next.ServeHTTP(w, r)
				return
			}

			// If anonymous access is allowed, proceed
			if allowAnonymous {
				next.ServeHTTP(w, r)
				return
			}

			// Otherwise, require authentication
			http.Error(w, "Authentication required", http.StatusUnauthorized)
		})
	}
}

// SimpleRateLimiter interface for rate limiting
type SimpleRateLimiter interface {
	Allow(identifier string, limit int) bool
}

// RateLimitByUser applies rate limiting based on user or IP
func (pc *PermissionChecker) RateLimitByUser(rateLimiter SimpleRateLimiter, defaultLimit int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := GetAuthContext(r)

			var identifier string
			var limit int

			if authCtx != nil && authCtx.IsAuthenticated() {
				// Use user ID for authenticated users
				identifier = authCtx.GetUserID()

				// Use token rate limit if available, otherwise user default
				if authCtx.Token != nil {
					limit = authCtx.Token.RateLimit
				} else {
					limit = defaultLimit
				}
			} else {
				// Use IP address for anonymous users
				identifier = authCtx.IPAddress
				limit = defaultLimit / 2 // Lower limit for anonymous users
			}

			// Check rate limit
			if !rateLimiter.Allow(identifier, limit) {
				w.Header().Set("X-RateLimit-Limit", string(rune(limit)))
				w.Header().Set("X-RateLimit-Remaining", "0")
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AuditLog logs actions for auditing purposes
func (pc *PermissionChecker) AuditLog(action, resource string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := GetAuthContext(r)

			// Create audit log entry
			auditLog := &AuditLog{
				Action:    action,
				Resource:  resource,
				IPAddress: authCtx.IPAddress,
				UserAgent: authCtx.UserAgent,
				Timestamp: time.Now(),
				Success:   true, // Will be updated if request fails
			}

			if authCtx != nil && authCtx.IsAuthenticated() {
				auditLog.UserID = &authCtx.User.ID
			}

			// Extract resource ID from URL if available
			if resourceID := extractResourceID(r); resourceID != "" {
				auditLog.ResourceID = &resourceID
			}

			// Wrap response writer to capture status
			wrapped := &auditResponseWriter{
				ResponseWriter: w,
				auditLog:       auditLog,
			}

			next.ServeHTTP(wrapped, r)

			// Log the audit entry (in a real implementation, this would go to a database)
			pc.service.logger.WithFields(logrus.Fields{
				"audit_action":     auditLog.Action,
				"audit_resource":   auditLog.Resource,
				"audit_user_id":    auditLog.UserID,
				"audit_ip":         auditLog.IPAddress,
				"audit_success":    auditLog.Success,
				"audit_timestamp":  auditLog.Timestamp,
			}).Info("Audit log entry")
		})
	}
}

// tryAuthenticate attempts to authenticate a request without failing
func (pc *PermissionChecker) tryAuthenticate(r *http.Request) *AuthContext {
	ctx := r.Context()

	// Try API token authentication first
	if token := extractBearerToken(r); token != "" {
		if user, err := pc.service.ValidateAPIToken(ctx, token); err == nil {
			tokenInfo, _ := pc.service.tokenManager.ValidateToken(ctx, token)
			return &AuthContext{
				User:        user,
				Token:       tokenInfo,
				Permissions: tokenInfo.Permissions,
				IPAddress:   getClientIP(r),
				UserAgent:   r.UserAgent(),
			}
		}
	}

	// Try session authentication
	if sessionID := extractSessionID(r); sessionID != "" {
		if session, err := pc.service.GetSession(ctx, sessionID); err == nil {
			if user, err := pc.service.GetUser(ctx, session.UserID); err == nil {
				return &AuthContext{
					User:        user,
					Session:     session,
					Permissions: DefaultPermissions(user.IsAdmin),
					IPAddress:   getClientIP(r),
					UserAgent:   r.UserAgent(),
				}
			}
		}
	}

	return &AuthContext{
		IPAddress: getClientIP(r),
		UserAgent: r.UserAgent(),
	}
}

// Helper functions

// extractBearerToken extracts the bearer token from Authorization header
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}

	return parts[1]
}

// extractSessionID extracts the session ID from cookie
func extractSessionID(r *http.Request) string {
	cookie, err := r.Cookie("session_id") // Default cookie name
	if err != nil {
		return ""
	}

	return cookie.Value
}

// getClientIP gets the client IP address from request
func getClientIP(r *http.Request) string {
	// Try X-Forwarded-For first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// Try X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}

	return r.RemoteAddr
}

// extractResourceID extracts resource ID from URL path
func extractResourceID(r *http.Request) string {
	// Simple extraction - look for UUIDs or IDs in path
	parts := strings.Split(r.URL.Path, "/")
	for _, part := range parts {
		if len(part) > 6 && (isUUID(part) || isID(part)) {
			return part
		}
	}
	return ""
}

// isUUID checks if a string looks like a UUID
func isUUID(s string) bool {
	return len(s) == 36 && strings.Count(s, "-") == 4
}

// isID checks if a string looks like an ID
func isID(s string) bool {
	return len(s) >= 6 && len(s) <= 64
}

// auditResponseWriter wraps http.ResponseWriter to capture response status
type auditResponseWriter struct {
	http.ResponseWriter
	auditLog   *AuditLog
	statusCode int
}

func (w *auditResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode

	// Update audit log based on status code
	if statusCode >= 400 {
		w.auditLog.Success = false
		errorMsg := http.StatusText(statusCode)
		w.auditLog.Error = &errorMsg
	}

	w.ResponseWriter.WriteHeader(statusCode)
}

// Context keys for storing auth context
type contextKey string

const (
	authContextKey contextKey = "auth_context"
)

// SetAuthContext sets the auth context in the request context
func SetAuthContext(r *http.Request, authCtx *AuthContext) *http.Request {
	ctx := context.WithValue(r.Context(), authContextKey, authCtx)
	return r.WithContext(ctx)
}

// GetAuthContext gets the auth context from the request context
func GetAuthContext(r *http.Request) *AuthContext {
	if authCtx, ok := r.Context().Value(authContextKey).(*AuthContext); ok {
		return authCtx
	}
	return nil
}

// PermissionLevel represents different permission levels
type PermissionLevel int

const (
	PermissionNone PermissionLevel = iota
	PermissionRead
	PermissionWrite
	PermissionAdmin
)

// CheckResourcePermission checks if user has permission for a specific resource
func CheckResourcePermission(authCtx *AuthContext, resource, action string, resourceOwnerID string) bool {
	if authCtx == nil || !authCtx.IsAuthenticated() {
		return false
	}

	// Admin can do everything
	if authCtx.IsAdmin() {
		return true
	}

	// Check ownership for non-admin users
	if resourceOwnerID != "" && resourceOwnerID != authCtx.GetUserID() {
		return false
	}

	// Check specific permission
	permission := resource + ":" + action
	return authCtx.HasPermission(permission)
}

// GetEffectivePermissions returns the effective permissions for a user
func GetEffectivePermissions(authCtx *AuthContext) []string {
	if authCtx == nil || !authCtx.IsAuthenticated() {
		return []string{}
	}

	if authCtx.IsAdmin() {
		return []string{PermissionAll}
	}

	return authCtx.Permissions
}

// CanAccessResource checks if user can access a resource based on ownership and permissions
func CanAccessResource(authCtx *AuthContext, resourceOwnerID string, requiredPermission string) bool {
	if authCtx == nil || !authCtx.IsAuthenticated() {
		return false
	}

	// Admin can access everything
	if authCtx.IsAdmin() {
		return true
	}

	// Check ownership
	if resourceOwnerID != "" && resourceOwnerID != authCtx.GetUserID() {
		return false
	}

	// Check permission
	return authCtx.HasPermission(requiredPermission)
}