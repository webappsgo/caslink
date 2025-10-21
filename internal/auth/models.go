package auth

import (
	"errors"
	"strings"
	"time"
)

// Errors
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserInactive       = errors.New("user is inactive")
	ErrUsernameExists     = errors.New("username already exists")
	ErrEmailExists        = errors.New("email already exists")
	ErrInvalidUsername    = errors.New("invalid username")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrPasswordTooShort   = errors.New("password is too short")
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionExpired     = errors.New("session expired")
	ErrTokenNotFound      = errors.New("token not found")
	ErrTokenExpired       = errors.New("token expired")
	ErrTokenInactive      = errors.New("token is inactive")
	ErrInvalidUserID      = errors.New("invalid user ID")
	ErrInvalidTokenName   = errors.New("invalid token name")
	ErrTokenNameTooLong   = errors.New("token name is too long")
	ErrInvalidRateLimit   = errors.New("invalid rate limit")
	ErrInsufficientPermissions = errors.New("insufficient permissions")
)

// UserInfo represents user information
type UserInfo struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	Email        *string    `json:"email,omitempty"`
	IsAdmin      bool       `json:"is_admin"`
	IsPremium    bool       `json:"is_premium"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLogin    *time.Time `json:"last_login,omitempty"`
	TwoFAEnabled bool       `json:"two_fa_enabled"`
}

// CreateUserRequest represents a request to create a new user
type CreateUserRequest struct {
	Username string  `json:"username"`
	Email    *string `json:"email,omitempty"`
	Password string  `json:"password"`
	IsAdmin  bool    `json:"is_admin,omitempty"`
}

// UpdateUserRequest represents a request to update user information
type UpdateUserRequest struct {
	Email    *string `json:"email,omitempty"`
	Password *string `json:"password,omitempty"`
	IsAdmin  *bool   `json:"is_admin,omitempty"`
	Active   *bool   `json:"active,omitempty"`
}

// ListUsersRequest represents a request to list users
type ListUsersRequest struct {
	Page   int `json:"page"`
	Limit  int `json:"limit"`
	Search string `json:"search,omitempty"`
}

// ListUsersResponse represents a response to list users
type ListUsersResponse struct {
	Users []*UserInfo `json:"users"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Limit int         `json:"limit"`
}

// Session represents a user session
type Session struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	ExpiresAt    time.Time  `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
	LastAccessed time.Time  `json:"last_accessed"`
	RememberMe   bool       `json:"remember_me"`
	IPAddress    *string    `json:"ip_address,omitempty"`
	UserAgent    *string    `json:"user_agent,omitempty"`
	Active       bool       `json:"active"`
}

// SessionMetadata contains metadata about a session
type SessionMetadata struct {
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
}

// SessionCookie represents a session cookie
type SessionCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path"`
	Domain   string `json:"domain"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"http_only"`
	SameSite string `json:"same_site"`
	MaxAge   int    `json:"max_age"`
}

// SessionStats represents session statistics
type SessionStats struct {
	ActiveSessions    int64 `json:"active_sessions"`
	ExpiredSessions   int64 `json:"expired_sessions"`
	SessionsToday     int64 `json:"sessions_today"`
	RememberMeSessions int64 `json:"remember_me_sessions"`
}

// APIToken represents an API token
type APIToken struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Name        string     `json:"name"`
	Token       string     `json:"token,omitempty"` // Only included when creating
	Permissions []string   `json:"permissions"`
	RateLimit   int        `json:"rate_limit"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsed    *time.Time `json:"last_used,omitempty"`
	LastUsedIP  *string    `json:"last_used_ip,omitempty"`
	Active      bool       `json:"active"`
}

// CreateTokenRequest represents a request to create a new API token
type CreateTokenRequest struct {
	UserID      string        `json:"user_id"`
	Name        string        `json:"name"`
	Permissions []string      `json:"permissions"`
	RateLimit   int           `json:"rate_limit"`
	ExpiresIn   *time.Duration `json:"expires_in,omitempty"`
}

// UpdateTokenRequest represents a request to update an API token
type UpdateTokenRequest struct {
	Name        *string    `json:"name,omitempty"`
	Permissions *[]string  `json:"permissions,omitempty"`
	RateLimit   *int       `json:"rate_limit,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Active      *bool      `json:"active,omitempty"`
}

// TokenStats represents API token statistics
type TokenStats struct {
	ActiveTokens       int64 `json:"active_tokens"`
	ExpiredTokens      int64 `json:"expired_tokens"`
	TokensToday        int64 `json:"tokens_today"`
	TokensUsedRecently int64 `json:"tokens_used_recently"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RememberMe bool   `json:"remember_me"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	User      *UserInfo `json:"user"`
	SessionID string    `json:"session_id,omitempty"`
	Token     string    `json:"token,omitempty"` // For API access
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Username string  `json:"username"`
	Email    *string `json:"email,omitempty"`
	Password string  `json:"password"`
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// Permission constants
const (
	PermissionURLCreate    = "url:create"
	PermissionURLRead      = "url:read"
	PermissionURLUpdate    = "url:update"
	PermissionURLDelete    = "url:delete"
	PermissionURLAnalytics = "url:analytics"
	PermissionQRGenerate   = "qr:generate"
	PermissionBulkImport   = "bulk:import"
	PermissionBulkExport   = "bulk:export"
	PermissionUserRead     = "user:read"
	PermissionUserUpdate   = "user:update"
	PermissionAdminRead    = "admin:read"
	PermissionAdminWrite   = "admin:write"
	PermissionAll          = "*"
)

// DefaultPermissions returns the default permissions for different user types
func DefaultPermissions(isAdmin bool) []string {
	if isAdmin {
		return []string{PermissionAll}
	}

	return []string{
		PermissionURLCreate,
		PermissionURLRead,
		PermissionURLUpdate,
		PermissionURLDelete,
		PermissionURLAnalytics,
		PermissionQRGenerate,
		PermissionBulkImport,
		PermissionBulkExport,
		PermissionUserRead,
		PermissionUserUpdate,
	}
}

// HasPermission checks if a user has a specific permission
func HasPermission(userPermissions []string, requiredPermission string) bool {
	for _, permission := range userPermissions {
		if permission == PermissionAll || permission == requiredPermission {
			return true
		}

		// Check wildcard permissions (e.g., "url:*" covers "url:create")
		if strings.HasSuffix(permission, ":*") {
			prefix := strings.TrimSuffix(permission, "*")
			if strings.HasPrefix(requiredPermission, prefix) {
				return true
			}
		}
	}

	return false
}

// ValidatePermissions validates a list of permissions
func ValidatePermissions(permissions []string) error {
	validPermissions := map[string]bool{
		PermissionURLCreate:    true,
		PermissionURLRead:      true,
		PermissionURLUpdate:    true,
		PermissionURLDelete:    true,
		PermissionURLAnalytics: true,
		PermissionQRGenerate:   true,
		PermissionBulkImport:   true,
		PermissionBulkExport:   true,
		PermissionUserRead:     true,
		PermissionUserUpdate:   true,
		PermissionAdminRead:    true,
		PermissionAdminWrite:   true,
		PermissionAll:          true,
	}

	for _, permission := range permissions {
		// Allow wildcard permissions
		if strings.HasSuffix(permission, ":*") {
			continue
		}

		if !validPermissions[permission] {
			return errors.New("invalid permission: " + permission)
		}
	}

	return nil
}

// AuthContext represents the authentication context for a request
type AuthContext struct {
	User        *UserInfo `json:"user"`
	Session     *Session  `json:"session,omitempty"`
	Token       *APIToken `json:"token,omitempty"`
	Permissions []string  `json:"permissions"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
}

// IsAuthenticated checks if the context represents an authenticated user
func (ac *AuthContext) IsAuthenticated() bool {
	return ac.User != nil
}

// IsAdmin checks if the authenticated user is an admin
func (ac *AuthContext) IsAdmin() bool {
	return ac.User != nil && ac.User.IsAdmin
}

// HasPermission checks if the authenticated user has a specific permission
func (ac *AuthContext) HasPermission(permission string) bool {
	if ac.User == nil {
		return false
	}

	if ac.User.IsAdmin {
		return true // Admins have all permissions
	}

	return HasPermission(ac.Permissions, permission)
}

// GetUserID returns the user ID from the context
func (ac *AuthContext) GetUserID() string {
	if ac.User == nil {
		return ""
	}
	return ac.User.ID
}

// GetUsername returns the username from the context
func (ac *AuthContext) GetUsername() string {
	if ac.User == nil {
		return ""
	}
	return ac.User.Username
}

// OAuth related structures

// OAuthProvider represents an OAuth provider configuration
type OAuthProvider struct {
	Name         string `json:"name"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_url"`
	AuthorizeURL string `json:"authorize_url"`
	TokenURL     string `json:"token_url"`
	UserInfoURL  string `json:"user_info_url"`
	Scopes       []string `json:"scopes"`
}

// OAuthUserInfo represents user information from OAuth provider
type OAuthUserInfo struct {
	ID       string  `json:"id"`
	Username string  `json:"username"`
	Email    *string `json:"email,omitempty"`
	Name     string  `json:"name"`
	Picture  string  `json:"picture,omitempty"`
}

// TwoFactor related structures

// TOTPSetupRequest represents a request to set up TOTP
type TOTPSetupRequest struct {
	UserID string `json:"user_id"`
}

// TOTPSetupResponse represents a response to TOTP setup
type TOTPSetupResponse struct {
	Secret    string `json:"secret"`
	QRCodeURL string `json:"qr_code_url"`
	BackupCodes []string `json:"backup_codes"`
}

// TOTPVerifyRequest represents a request to verify TOTP
type TOTPVerifyRequest struct {
	UserID string `json:"user_id"`
	Code   string `json:"code"`
}

// WebAuthnCredential represents a WebAuthn credential
type WebAuthnCredential struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	CredentialID []byte   `json:"credential_id"`
	PublicKey   []byte    `json:"public_key"`
	CreatedAt   time.Time `json:"created_at"`
	LastUsed    *time.Time `json:"last_used,omitempty"`
	Active      bool      `json:"active"`
}

// WebAuthnSetupRequest represents a request to set up WebAuthn
type WebAuthnSetupRequest struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

// Rate limiting structures

// RateLimitInfo represents rate limiting information
type RateLimitInfo struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
	Window    time.Duration `json:"window"`
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID         string                 `json:"id"`
	UserID     *string                `json:"user_id,omitempty"`
	Action     string                 `json:"action"`
	Resource   string                 `json:"resource"`
	ResourceID *string                `json:"resource_id,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	IPAddress  string                 `json:"ip_address"`
	UserAgent  string                 `json:"user_agent"`
	Success    bool                   `json:"success"`
	Error      *string                `json:"error,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
}