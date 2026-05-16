package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	stdLog "log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/server/handler"
	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// Context keys are sourced from the handler package so that middleware
// and handlers refer to the same typed key (Go requires identical key
// types — not just identical string values — for context.Value lookups).
const (
	userContextKey    = handler.UserContextKey
	adminContextKey   = handler.AdminContextKey
	orgContextKey     = handler.ContextKey("org")
	orgRoleContextKey = handler.ContextKey("org_role")
)

// writeJSONError writes a canonical error envelope to w:
//
//	{"ok":false,"error":"CODE","message":"..."}
//
// It mirrors handler.respondError but is accessible within package server
// (handler.respondError is unexported).
func writeJSONError(w http.ResponseWriter, status int, message string) {
	code := jsonErrCode(status)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := `{"ok":false,"error":"` + code + `","message":"` + jsonEscape(message) + `"}`
	_, _ = w.Write([]byte(body))
}

// jsonErrCode maps an HTTP status to the canonical error-code string used in
// the API envelope (mirrors handler.errCodeFromStatus).
func jsonErrCode(status int) string {
	switch status {
	case 400:
		return "BAD_REQUEST"
	case 401:
		return "UNAUTHORIZED"
	case 403:
		return "FORBIDDEN"
	case 404:
		return "NOT_FOUND"
	case 409:
		return "CONFLICT"
	case 422:
		return "VALIDATION_FAILED"
	case 429:
		return "RATE_LIMITED"
	case 503:
		return "MAINTENANCE"
	default:
		if status >= 500 {
			return "SERVER_ERROR"
		}
		return "ERROR"
	}
}

// jsonEscape replaces the handful of characters that would break a bare JSON
// string literal. For middleware error messages (controlled strings) this is
// sufficient; use encoding/json for arbitrary data.
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return s
}

// ---- Security headers middleware ----------------------------------------

// defaultCSP is the spec-canonical Content-Security-Policy (AI.md PART 11).
// 'unsafe-inline' is the pragmatic default for Go template projects; tighten
// with nonces when the project generates them. frame-ancestors / base-uri /
// form-action are defence-in-depth directives that don't vary per request.
const defaultCSP = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https:; " +
	"font-src 'self' https:; " +
	"connect-src 'self'; " +
	"media-src 'self' blob:; " +
	"worker-src 'self' blob:; " +
	"manifest-src 'self'; " +
	"frame-ancestors 'self'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// defaultPermissionsPolicy is the spec-canonical Permissions-Policy header
// (AI.md PART 11 "Permissions-Policy Configuration"). Features required by
// the spec itself are scoped to self; advertising/tracking proposals and all
// sensor/hardware features are locked to () (disabled for everyone).
const defaultPermissionsPolicy = "" +
	"encrypted-media=(self), " +
	"fullscreen=(self), " +
	"payment=(self), " +
	"picture-in-picture=(self), " +
	"publickey-credentials-get=(self), " +
	"storage-access=(self), " +
	"web-share=(self), " +
	"camera=(), " +
	"geolocation=(), " +
	"microphone=(), " +
	"usb=(), " +
	"midi=(), " +
	"interest-cohort=(), " +
	"browsing-topics=(), " +
	"attribution-reporting=()"

// SecurityHeadersMiddleware adds standard security headers to every response
// per AI.md PART 11 "Security Headers". When tlsEnabled is true an HSTS
// header and the CSP upgrade-insecure-requests directive are also emitted.
// In dev mode CSP is sent as Content-Security-Policy-Report-Only so that
// violations are logged without blocking the app.
func SecurityHeadersMiddleware(tlsEnabled, devMode bool) func(http.Handler) http.Handler {
	csp := defaultCSP
	if tlsEnabled {
		csp += "; upgrade-insecure-requests"
	}
	cspHeader := "Content-Security-Policy"
	if devMode {
		cspHeader = "Content-Security-Policy-Report-Only"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			// SAMEORIGIN per AI.md PART 11 → "Security Headers"
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("X-XSS-Protection", "1; mode=block")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("X-Permitted-Cross-Domain-Policies", "none")
			h.Set("Origin-Agent-Cluster", "?1")
			// Cross-origin isolation headers — PART 11 defaults
			h.Set("Cross-Origin-Opener-Policy", "unsafe-none")
			h.Set("Cross-Origin-Resource-Policy", "cross-origin")
			h.Set(cspHeader, csp)
			h.Set("Permissions-Policy", defaultPermissionsPolicy)
			if tlsEnabled {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			}
			// Echo existing request ID (chi sets X-Request-Id) or generate one.
			reqID := r.Header.Get("X-Request-Id")
			if reqID == "" {
				b := make([]byte, 16)
				_, _ = rand.Read(b)
				reqID = hex.EncodeToString(b)
			}
			h.Set("X-Request-Id", reqID)
			next.ServeHTTP(w, r)
		})
	}
}

// ---- Rate limiting middleware -------------------------------------------

// rateBucket tracks request counts within a sliding window.
type rateBucket struct {
	mu       sync.Mutex
	requests []time.Time
}

// allow returns true if the request is within the allowed rate.
func (b *rateBucket) allow(limit int, window time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().Add(-window)

	// Evict expired entries.
	valid := b.requests[:0]
	for _, t := range b.requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	b.requests = valid

	if len(b.requests) >= limit {
		return false
	}
	b.requests = append(b.requests, time.Now())
	return true
}

// RateLimiter holds per-IP buckets for different endpoint groups.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
}

// NewRateLimiter creates a new rate limiter with periodic garbage collection.
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{buckets: make(map[string]*rateBucket)}
	go rl.gc()
	return rl
}

// gc removes buckets that have had no requests for 2 hours.
func (rl *RateLimiter) gc() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-2 * time.Hour)
		rl.mu.Lock()
		for key, b := range rl.buckets {
			b.mu.Lock()
			stale := len(b.requests) == 0 ||
				(len(b.requests) > 0 && b.requests[len(b.requests)-1].Before(cutoff))
			b.mu.Unlock()
			if stale {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) bucket(key string) *rateBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[key]
	if !ok {
		b = &rateBucket{}
		rl.buckets[key] = b
	}
	return b
}

// Allow returns true if the given IP is within the rate limit.
// limit = max requests, window = time window.
func (rl *RateLimiter) Allow(ip string, limit int, window time.Duration) bool {
	return rl.bucket(ip).allow(limit, window)
}

// RateLimitMiddleware applies auth-endpoint rate limits:
//   - /server/auth/login, /api/v1/server/auth/login → 5 / 15 min
//   - /server/auth/register, /api/v1/server/auth/register → 5 / 1 h
//   - /server/auth/password/* → 3 / 1 h
//
// Returns 429 with a generic message; never exposes threshold numbers.
func RateLimitMiddleware(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}
			ip := realIP(r)
			path := r.URL.Path

			var limit int
			var window time.Duration

			switch {
			case strings.Contains(path, "/login"):
				limit, window = 5, 15*time.Minute
			case strings.Contains(path, "/register"):
				limit, window = 5, time.Hour
			case strings.Contains(path, "/password"):
				limit, window = 3, time.Hour
			case strings.Contains(path, "/2fa"):
				limit, window = 5, 15*time.Minute
			default:
				next.ServeHTTP(w, r)
				return
			}

			if !rl.Allow(ip+path, limit, window) {
				writeJSONError(w, http.StatusTooManyRequests, "Too many attempts. Please try again later.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// realIP extracts the real client IP, respecting X-Forwarded-For / X-Real-IP.
func realIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.SplitN(fwd, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return real
	}
	// Strip port
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// ---- CSRF middleware ----------------------------------------------------

const csrfCookieName = "csrf_token"
const csrfHeaderName = "X-CSRF-Token"
const csrfFormField = "_csrf"

// CSRFMiddleware implements the double-submit cookie pattern.
// Safe methods (GET, HEAD, OPTIONS, TRACE) are always allowed.
// Requests with Authorization: Bearer are exempt (API token auth).
// /.well-known/* and /server/healthz are exempt.
func CSRFMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Exempt paths
			if strings.HasPrefix(path, "/.well-known/") ||
				path == "/server/healthz" ||
				strings.HasPrefix(path, "/api/v1/server/healthz") {
				next.ServeHTTP(w, r)
				return
			}

			// Safe methods pass through
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
				ensureCSRFCookie(w, r)
				next.ServeHTTP(w, r)
				return
			}

			// Bearer-token auth routes are exempt from cookie CSRF
			if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				next.ServeHTTP(w, r)
				return
			}

			// Validate CSRF token
			cookie, err := r.Cookie(csrfCookieName)
			if err != nil || cookie.Value == "" {
				writeJSONError(w, http.StatusForbidden, "CSRF validation failed")
				return
			}

			submitted := r.Header.Get(csrfHeaderName)
			if submitted == "" {
				if err2 := r.ParseForm(); err2 == nil {
					submitted = r.FormValue(csrfFormField)
				}
			}

			if submitted == "" || submitted != cookie.Value {
				writeJSONError(w, http.StatusForbidden, "CSRF validation failed")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ensureCSRFCookie sets the csrf_token cookie if not already present.
func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(csrfCookieName); err == nil && c.Value != "" {
		return
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: false, // JS must be able to read it
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
}

// ---- Auth middleware ----------------------------------------------------

// UserAuthMiddleware requires valid user session
func UserAuthMiddleware(authService *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("user_session")
			if err != nil || cookie.Value == "" {
				http.Redirect(w, r, "/server/auth/login", http.StatusSeeOther)
				return
			}

			user, err := authService.ValidateUserSession(r.Context(), cookie.Value)
			if err != nil {
				http.Redirect(w, r, "/server/auth/login", http.StatusSeeOther)
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminAuthMiddleware requires valid admin session. adminPath is the
// configured admin panel path segment (e.g. "admin"), used to build the
// correct login redirect URL per spec PART 17.
func AdminAuthMiddleware(authService *service.AuthService, adminPath string) func(http.Handler) http.Handler {
	if adminPath == "" {
		adminPath = "admin"
	}
	loginURL := "/server/" + adminPath
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("admin_session")
			if err != nil || cookie.Value == "" {
				http.Redirect(w, r, loginURL, http.StatusSeeOther)
				return
			}

			admin, err := authService.ValidateSession(r.Context(), cookie.Value)
			if err != nil {
				http.Redirect(w, r, loginURL, http.StatusSeeOther)
				return
			}

			ctx := context.WithValue(r.Context(), adminContextKey, admin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OrgMemberMiddleware requires user to be a member of the organization
func OrgMemberMiddleware(orgService *service.OrgService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := r.Context().Value(userContextKey).(*service.User)
			if !ok || user == nil {
				writeJSONError(w, http.StatusUnauthorized, "Authentication required")
				return
			}

			slug := chi.URLParam(r, "slug")
			if slug == "" {
				writeJSONError(w, http.StatusBadRequest, "Organization slug required")
				return
			}

			org, err := orgService.GetOrganizationBySlug(r.Context(), slug)
			if err != nil {
				writeJSONError(w, http.StatusNotFound, "Organization not found")
				return
			}

			isMember, role, err := orgService.IsMember(r.Context(), org.ID, user.ID)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "Error checking membership")
				return
			}

			if !isMember {
				writeJSONError(w, http.StatusForbidden, "Not an organization member")
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, orgContextKey, org)
			ctx = context.WithValue(ctx, orgRoleContextKey, role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ---- Bearer token middleware -------------------------------------------

const bearerContextKey = handler.ContextKey("bearer_user")

// BearerAuthMiddleware validates Authorization: Bearer <token> headers.
// On failure it returns 401; on success the TokenRecord is stored in context.
func BearerAuthMiddleware(tokenService *service.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				w.Header().Set("WWW-Authenticate", `Bearer realm="caslink"`)
				writeJSONError(w, http.StatusUnauthorized, "Bearer token required")
				return
			}
			plaintext := strings.TrimPrefix(auth, "Bearer ")
			rec, err := tokenService.ValidateToken(r.Context(), plaintext)
			if err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="caslink", error="invalid_token"`)
				writeJSONError(w, http.StatusUnauthorized, "Invalid or expired token")
				return
			}
			ctx := context.WithValue(r.Context(), bearerContextKey, rec)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ---- Context helpers ---------------------------------------------------

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

// ---- Access log middleware ---------------------------------------------

// statusRecorder is a minimal http.ResponseWriter wrapper that captures
// the response status code and the number of bytes written so the access
// log can include both fields.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if sr.status == 0 {
		sr.status = http.StatusOK
	}
	n, err := sr.ResponseWriter.Write(b)
	sr.bytes += n
	return n, err
}

// accessLogMiddleware writes a compact single-line access log entry for
// each request. The format is space-separated to keep it cheap to parse
// in log aggregators and never includes credentials or cookie values:
//
//	{method} {path} {status} {bytes} {duration_ms} {ip} {request_id}
//
// Used in production; development uses chi's verbose logger instead.
func accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = "-"
		}
		// stdlib log already includes timestamp; just print the fields.
		// Path is logged as-is — handlers must never put credentials in
		// the URL (spec PART 11) so this is safe.
		stdLog.Printf("access %s %s %d %d %dms %s %s",
			r.Method, r.URL.Path, rec.status, rec.bytes,
			time.Since(start).Milliseconds(), realIP(r), reqID,
		)
	})
}

// ---- Path security middleware -------------------------------------------

// PathSecurityMiddleware blocks path-traversal attacks and normalizes paths
// per AI.md PART 5. It must run after URLNormalizeMiddleware so the path is
// already in canonical form before traversal checks run.
func PathSecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		original := r.URL.Path

		// r.URL.Path is already percent-decoded by net/http; check RawPath too.
		rawPath := r.URL.RawPath
		if rawPath == "" {
			rawPath = r.URL.Path
		}

		// Block path traversal — both decoded ("..") and percent-encoded (%2e).
		if strings.Contains(original, "..") ||
			strings.Contains(rawPath, "..") ||
			strings.Contains(strings.ToLower(rawPath), "%2e") {
			writeJSONError(w, http.StatusBadRequest, "path traversal not permitted")
			return
		}

		// Normalize the path (collapses double slashes, etc.).
		cleaned := path.Clean(original)

		// Ensure leading slash.
		if !strings.HasPrefix(cleaned, "/") {
			cleaned = "/" + cleaned
		}

		// Preserve trailing slash when the original had one (router may need it).
		if original != "/" && strings.HasSuffix(original, "/") && !strings.HasSuffix(cleaned, "/") {
			cleaned += "/"
		}

		r.URL.Path = cleaned
		next.ServeHTTP(w, r)
	})
}

// ---- URL normalize middleware -------------------------------------------

// URLNormalizeMiddleware removes trailing slashes and issues a 301 redirect
// to the canonical URL per AI.md PART 5 / PART 16. Root "/" is exempt.
// Requests for paths that end with a file extension are exempt (e.g. /robots.txt).
// Must run before PathSecurityMiddleware in the global middleware stack.
func URLNormalizeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path

		if p != "/" && strings.HasSuffix(p, "/") {
			// Keep the slash if the last path segment looks like a file.
			last := p[strings.LastIndex(p, "/"):]
			if !strings.Contains(last, ".") {
				canonical := strings.TrimSuffix(p, "/")
				if r.URL.RawQuery != "" {
					canonical += "?" + r.URL.RawQuery
				}
				http.Redirect(w, r, canonical, http.StatusMovedPermanently)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
