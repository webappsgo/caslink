package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Middleware constants
const (
	RequestIDHeader = "X-Request-ID"
	UserIDKey       = "user_id"
	IsAdminKey      = "is_admin"
	ClientIPKey     = "client_ip"
)

// RateLimiter implements rate limiting
type RateLimiter struct {
	clients map[string]*ClientInfo
	mutex   sync.RWMutex
}

// ClientInfo stores rate limiting information for a client
type ClientInfo struct {
	Requests  int
	ResetTime time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*ClientInfo),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow(clientIP string, limit int, window time.Duration) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	client, exists := rl.clients[clientIP]

	if !exists || now.After(client.ResetTime) {
		// New client or window expired
		rl.clients[clientIP] = &ClientInfo{
			Requests:  1,
			ResetTime: now.Add(window),
		}
		return true
	}

	if client.Requests >= limit {
		return false
	}

	client.Requests++
	return true
}

// cleanup removes expired entries
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mutex.Lock()
		now := time.Now()
		for ip, client := range rl.clients {
			if now.After(client.ResetTime) {
				delete(rl.clients, ip)
			}
		}
		rl.mutex.Unlock()
	}
}

// Global rate limiter instance
var globalRateLimiter = NewRateLimiter()

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Add request ID
		requestID := generateRequestID()
		wrapped.Header().Set(RequestIDHeader, requestID)

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Log request
		duration := time.Since(start)
		clientIP := s.proxyDetector.GetClientIP(r)

		s.logger.WithFields(logrus.Fields{
			"method":     r.Method,
			"path":       r.URL.Path,
			"status":     wrapped.statusCode,
			"duration":   duration,
			"client_ip":  clientIP,
			"user_agent": r.UserAgent(),
			"request_id": requestID,
		}).Info("HTTP request")
	})
}

// proxyHeadersMiddleware processes proxy headers and extracts client information
func (s *Server) proxyHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract client IP using proxy detector
		clientIP := s.proxyDetector.GetClientIP(r)

		// Store client IP in context
		ctx := context.WithValue(r.Context(), ClientIPKey, clientIP)
		r = r.WithContext(ctx)

		// Process other proxy headers
		s.proxyDetector.ProcessHeaders(r)

		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware adds security headers
func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HSTS header
		if r.TLS != nil || s.isHTTPS(r) {
			w.Header().Set("Strict-Transport-Security",
				fmt.Sprintf("max-age=%d; includeSubDomains", s.config.Security.HSTSMaxAge))
		}

		// Content Security Policy
		if s.config.Security.CSPPolicy != "" {
			w.Header().Set("Content-Security-Policy", s.config.Security.CSPPolicy)
		}

		// Other security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// HTTPS redirect
		if s.config.Security.EnableHTTPSRedirect == "true" ||
		   (s.config.Security.EnableHTTPSRedirect == "auto" && s.isHTTPS(r)) {
			if r.TLS == nil && !s.isHTTPS(r) {
				httpsURL := "https://" + r.Host + r.RequestURI
				http.Redirect(w, r, httpsURL, http.StatusMovedPermanently)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// corsMiddleware handles CORS
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		if s.isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else if len(s.config.Security.AllowedOrigins) == 1 && s.config.Security.AllowedOrigins[0] == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "3600")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware implements rate limiting
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.config.RateLimit.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := s.getClientIP(r)

		// Check rate limit
		if !globalRateLimiter.Allow(clientIP, s.config.RateLimit.RequestsPerMinute, time.Minute) {
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(s.config.RateLimit.RequestsPerMinute))
			w.Header().Set("X-RateLimit-Window", "60")
			w.Header().Set("Retry-After", "60")

			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Add rate limit headers
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(s.config.RateLimit.RequestsPerMinute))
		w.Header().Set("X-RateLimit-Window", "60")

		next.ServeHTTP(w, r)
	})
}

// authMiddleware handles authentication for API routes
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to get user from session or API token
		userID, isAdmin := s.authenticateRequest(r)

		if userID == "" {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Add user info to context
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, IsAdminKey, isAdmin)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// webAuthMiddleware handles authentication for web routes
func (s *Server) webAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to get user from session
		userID, isAdmin := s.authenticateWebRequest(r)

		if userID == "" {
			// Redirect to login page
			http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusFound)
			return
		}

		// Add user info to context
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, IsAdminKey, isAdmin)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// adminAuthMiddleware ensures the user is an admin
func (s *Server) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAdmin, ok := r.Context().Value(IsAdminKey).(bool)
		if !ok || !isAdmin {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "Admin access required", http.StatusForbidden)
			} else {
				http.Error(w, "Admin access required", http.StatusForbidden)
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Helper functions

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// isHTTPS checks if the request was made over HTTPS
func (s *Server) isHTTPS(r *http.Request) bool {
	// Check direct TLS
	if r.TLS != nil {
		return true
	}

	// Check proxy headers
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" {
		return true
	}
	if proto := r.Header.Get("X-Forwarded-Protocol"); proto == "https" {
		return true
	}
	if scheme := r.Header.Get("X-Scheme"); scheme == "https" {
		return true
	}

	// Check Cloudflare
	if visitor := r.Header.Get("CF-Visitor"); strings.Contains(visitor, `"scheme":"https"`) {
		return true
	}

	return false
}

// isOriginAllowed checks if an origin is allowed for CORS
func (s *Server) isOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}

	for _, allowed := range s.config.Security.AllowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
		// Simple wildcard matching for subdomains
		if strings.HasPrefix(allowed, "*.") {
			domain := allowed[2:]
			if strings.HasSuffix(origin, domain) {
				return true
			}
		}
	}

	return false
}

// getClientIP gets the client IP from the request
func (s *Server) getClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(ClientIPKey).(string); ok {
		return ip
	}
	return s.proxyDetector.GetClientIP(r)
}

// authenticateRequest authenticates an API request
func (s *Server) authenticateRequest(r *http.Request) (userID string, isAdmin bool) {
	// Try API token first
	if token := r.Header.Get("Authorization"); token != "" {
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
			// TODO: Validate API token and get user
			return s.validateAPIToken(token)
		}
	}

	// Try session cookie
	return s.authenticateWebRequest(r)
}

// authenticateWebRequest authenticates a web request using session
func (s *Server) authenticateWebRequest(r *http.Request) (userID string, isAdmin bool) {
	// TODO: Implement session-based authentication
	return "", false
}

// validateAPIToken validates an API token and returns user info
func (s *Server) validateAPIToken(token string) (userID string, isAdmin bool) {
	// TODO: Implement API token validation
	return "", false
}

// getUserIDFromContext gets user ID from request context
func getUserIDFromContext(r *http.Request) string {
	if userID, ok := r.Context().Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// isAdminFromContext checks if user is admin from request context
func isAdminFromContext(r *http.Request) bool {
	if isAdmin, ok := r.Context().Value(IsAdminKey).(bool); ok {
		return isAdmin
	}
	return false
}