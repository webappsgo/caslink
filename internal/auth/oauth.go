package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

// OAuthService handles OAuth authentication
type OAuthService struct {
	config   *config.OAuthConfig
	db       *db.DB
	logger   *logrus.Logger
	providers map[string]*oauth2.Config
}

// NewOAuthService creates a new OAuth service
func NewOAuthService(cfg *config.OAuthConfig, database *db.DB, logger *logrus.Logger) (*OAuthService, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	service := &OAuthService{
		config:    cfg,
		db:        database,
		logger:    logger,
		providers: make(map[string]*oauth2.Config),
	}

	// Initialize OAuth provider
	if err := service.initializeProvider(cfg.Provider); err != nil {
		return nil, fmt.Errorf("failed to initialize OAuth provider: %w", err)
	}

	return service, nil
}

// initializeProvider initializes an OAuth provider configuration
func (s *OAuthService) initializeProvider(providerName string) error {
	var authorizeURL, tokenURL, userInfoURL string
	var scopes []string

	// Set provider-specific defaults
	switch strings.ToLower(providerName) {
	case "google":
		authorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
		tokenURL = "https://oauth2.googleapis.com/token"
		userInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
		scopes = []string{"openid", "profile", "email"}
	case "github":
		authorizeURL = "https://github.com/login/oauth/authorize"
		tokenURL = "https://github.com/login/oauth/access_token"
		userInfoURL = "https://api.github.com/user"
		scopes = []string{"user:email"}
	case "microsoft":
		authorizeURL = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
		tokenURL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
		userInfoURL = "https://graph.microsoft.com/v1.0/me"
		scopes = []string{"openid", "profile", "email"}
	case "authelia":
		// Authelia requires custom URLs to be configured
		authorizeURL = s.config.AuthorizeURL
		tokenURL = s.config.TokenURL
		userInfoURL = s.config.UserInfoURL
		scopes = s.config.Scopes
		if authorizeURL == "" || tokenURL == "" || userInfoURL == "" {
			return fmt.Errorf("authelia provider requires custom authorize_url, token_url, and userinfo_url")
		}
	case "generic":
		// Generic OAuth provider uses custom URLs
		authorizeURL = s.config.AuthorizeURL
		tokenURL = s.config.TokenURL
		userInfoURL = s.config.UserInfoURL
		scopes = s.config.Scopes
		if authorizeURL == "" || tokenURL == "" || userInfoURL == "" {
			return fmt.Errorf("generic provider requires custom authorize_url, token_url, and userinfo_url")
		}
	default:
		return fmt.Errorf("unsupported OAuth provider: %s", providerName)
	}

	// Override with custom URLs if provided
	if s.config.AuthorizeURL != "" {
		authorizeURL = s.config.AuthorizeURL
	}
	if s.config.TokenURL != "" {
		tokenURL = s.config.TokenURL
	}
	if s.config.UserInfoURL != "" {
		userInfoURL = s.config.UserInfoURL
	}
	if len(s.config.Scopes) > 0 {
		scopes = s.config.Scopes
	}

	// Create OAuth2 config
	oauthConfig := &oauth2.Config{
		ClientID:     s.config.ClientID,
		ClientSecret: s.config.ClientSecret,
		RedirectURL:  s.config.RedirectURL,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authorizeURL,
			TokenURL: tokenURL,
		},
	}

	s.providers[providerName] = oauthConfig

	s.logger.WithFields(logrus.Fields{
		"provider":      providerName,
		"authorize_url": authorizeURL,
		"token_url":     tokenURL,
		"userinfo_url":  userInfoURL,
		"scopes":        scopes,
	}).Info("OAuth provider initialized")

	return nil
}

// GetAuthorizationURL generates an authorization URL for OAuth login
func (s *OAuthService) GetAuthorizationURL(state string) (string, error) {
	provider := s.providers[s.config.Provider]
	if provider == nil {
		return "", fmt.Errorf("OAuth provider not configured")
	}

	return provider.AuthCodeURL(state, oauth2.AccessTypeOffline), nil
}

// ExchangeCodeForToken exchanges an authorization code for an access token
func (s *OAuthService) ExchangeCodeForToken(ctx context.Context, code string) (*oauth2.Token, error) {
	provider := s.providers[s.config.Provider]
	if provider == nil {
		return nil, fmt.Errorf("OAuth provider not configured")
	}

	token, err := provider.Exchange(ctx, code)
	if err != nil {
		s.logger.WithError(err).Error("Failed to exchange OAuth code for token")
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	return token, nil
}

// GetUserInfo retrieves user information from the OAuth provider
func (s *OAuthService) GetUserInfo(ctx context.Context, token *oauth2.Token) (*OAuthUserInfo, error) {
	provider := s.providers[s.config.Provider]
	if provider == nil {
		return nil, fmt.Errorf("OAuth provider not configured")
	}

	// Create HTTP client with token
	client := provider.Client(ctx, token)

	// Determine userinfo URL
	userInfoURL := s.config.UserInfoURL
	if userInfoURL == "" {
		switch strings.ToLower(s.config.Provider) {
		case "google":
			userInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
		case "github":
			userInfoURL = "https://api.github.com/user"
		case "microsoft":
			userInfoURL = "https://graph.microsoft.com/v1.0/me"
		default:
			return nil, fmt.Errorf("userinfo URL not configured for provider: %s", s.config.Provider)
		}
	}

	// Fetch user info
	resp, err := client.Get(userInfoURL)
	if err != nil {
		s.logger.WithError(err).Error("Failed to fetch OAuth user info")
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"body":        string(body),
		}).Error("OAuth userinfo request failed")
		return nil, fmt.Errorf("userinfo request failed with status %d", resp.StatusCode)
	}

	// Parse response based on provider
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	userInfo, err := s.parseUserInfo(s.config.Provider, body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user info: %w", err)
	}

	// For GitHub, fetch email separately if not included
	if strings.ToLower(s.config.Provider) == "github" && (userInfo.Email == nil || *userInfo.Email == "") {
		email, err := s.fetchGitHubEmail(ctx, client)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to fetch GitHub email")
		} else {
			userInfo.Email = &email
		}
	}

	s.logger.WithFields(logrus.Fields{
		"provider":    s.config.Provider,
		"oauth_id":    userInfo.ID,
		"username":    userInfo.Username,
		"has_email":   userInfo.Email != nil && *userInfo.Email != "",
	}).Info("OAuth user info retrieved")

	return userInfo, nil
}

// parseUserInfo parses user information from OAuth provider response
func (s *OAuthService) parseUserInfo(provider string, body []byte) (*OAuthUserInfo, error) {
	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	userInfo := &OAuthUserInfo{}

	switch strings.ToLower(provider) {
	case "google":
		userInfo.ID = getStringField(rawData, "id")
		userInfo.Username = getStringField(rawData, "email")
		userInfo.Name = getStringField(rawData, "name")
		email := getStringField(rawData, "email")
		if email != "" {
			userInfo.Email = &email
		}
		userInfo.Picture = getStringField(rawData, "picture")

	case "github":
		userInfo.ID = fmt.Sprintf("%v", rawData["id"])
		userInfo.Username = getStringField(rawData, "login")
		userInfo.Name = getStringField(rawData, "name")
		email := getStringField(rawData, "email")
		if email != "" {
			userInfo.Email = &email
		}
		userInfo.Picture = getStringField(rawData, "avatar_url")

	case "microsoft":
		userInfo.ID = getStringField(rawData, "id")
		userInfo.Username = getStringField(rawData, "userPrincipalName")
		userInfo.Name = getStringField(rawData, "displayName")
		email := getStringField(rawData, "mail")
		if email == "" {
			email = getStringField(rawData, "userPrincipalName")
		}
		if email != "" {
			userInfo.Email = &email
		}

	case "authelia", "generic":
		// Generic OAuth parsing
		userInfo.ID = getStringField(rawData, "sub", "id", "user_id")
		userInfo.Username = getStringField(rawData, "preferred_username", "username", "login", "email")
		userInfo.Name = getStringField(rawData, "name", "display_name", "displayName")
		email := getStringField(rawData, "email", "mail")
		if email != "" {
			userInfo.Email = &email
		}
		userInfo.Picture = getStringField(rawData, "picture", "avatar", "avatar_url")

	default:
		return nil, fmt.Errorf("unsupported provider for parsing: %s", provider)
	}

	if userInfo.ID == "" {
		return nil, fmt.Errorf("could not extract user ID from OAuth response")
	}

	return userInfo, nil
}

// fetchGitHubEmail fetches the primary email from GitHub API
func (s *OAuthService) fetchGitHubEmail(ctx context.Context, client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch emails, status: %d", resp.StatusCode)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	// Find primary verified email
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}

	// Find any verified email
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}

	// Return first email if no verified email found
	if len(emails) > 0 {
		return emails[0].Email, nil
	}

	return "", fmt.Errorf("no email found")
}

// GenerateState generates a random state parameter for CSRF protection
func (s *OAuthService) GenerateState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ValidateState validates the OAuth state parameter
func (s *OAuthService) ValidateState(ctx context.Context, state, expectedState string) error {
	if state == "" || expectedState == "" {
		return fmt.Errorf("invalid state parameters")
	}

	if state != expectedState {
		return fmt.Errorf("state mismatch: CSRF protection failed")
	}

	return nil
}

// FindOrCreateUserFromOAuth finds an existing user or creates a new one from OAuth info
func (s *OAuthService) FindOrCreateUserFromOAuth(ctx context.Context, oauthInfo *OAuthUserInfo, providerName string) (*UserInfo, error) {
	// Try to find existing user by OAuth ID
	user, err := s.findUserByOAuthID(ctx, providerName, oauthInfo.ID)
	if err == nil {
		// User found, update last login
		s.updateLastLogin(ctx, user.ID)
		return user, nil
	}

	// Try to find by email if provided
	if oauthInfo.Email != nil && *oauthInfo.Email != "" {
		user, err = s.findUserByEmail(ctx, *oauthInfo.Email)
		if err == nil {
			// Link OAuth account to existing user
			if err := s.linkOAuthAccount(ctx, user.ID, providerName, oauthInfo.ID); err != nil {
				s.logger.WithError(err).Error("Failed to link OAuth account")
			}
			s.updateLastLogin(ctx, user.ID)
			return user, nil
		}
	}

	// Create new user
	username := s.generateUniqueUsername(ctx, oauthInfo.Username)
	userID, err := generateUserID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate user ID: %w", err)
	}

	// Generate a random password (user won't use it, OAuth only)
	randomPassword, err := s.generateRandomPassword()
	if err != nil {
		return nil, fmt.Errorf("failed to generate random password: %w", err)
	}

	newUser := &db.User{
		ID:           userID,
		Username:     username,
		Email:        oauthInfo.Email,
		PasswordHash: randomPassword, // Random hash, OAuth users won't use password login
		IsAdmin:      false,
		IsPremium:    false,
		CreatedAt:    time.Now(),
		TwoFAEnabled: false,
	}

	if err := s.createUserInDB(ctx, newUser); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Link OAuth account
	if err := s.linkOAuthAccount(ctx, userID, providerName, oauthInfo.ID); err != nil {
		s.logger.WithError(err).Error("Failed to link OAuth account after user creation")
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":  userID,
		"username": username,
		"provider": providerName,
	}).Info("New user created from OAuth")

	return &UserInfo{
		ID:           newUser.ID,
		Username:     newUser.Username,
		Email:        newUser.Email,
		IsAdmin:      newUser.IsAdmin,
		IsPremium:    newUser.IsPremium,
		CreatedAt:    newUser.CreatedAt,
		TwoFAEnabled: newUser.TwoFAEnabled,
	}, nil
}

// Helper functions

func getStringField(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := data[key]; ok && val != nil {
			if str, ok := val.(string); ok && str != "" {
				return str
			}
		}
	}
	return ""
}

func (s *OAuthService) findUserByOAuthID(ctx context.Context, provider, oauthID string) (*UserInfo, error) {
	// This would query the oauth_accounts table (to be added in migrations)
	// For now, return not found to trigger user creation
	return nil, ErrUserNotFound
}

func (s *OAuthService) findUserByEmail(ctx context.Context, email string) (*UserInfo, error) {
	query := "SELECT id, username, email, is_admin, is_premium, created_at, two_fa_enabled FROM users WHERE email = ? LIMIT 1"
	if s.db.Type() == "postgres" {
		query = "SELECT id, username, email, is_admin, is_premium, created_at, two_fa_enabled FROM users WHERE email = $1 LIMIT 1"
	}

	var user UserInfo
	var emailPtr *string
	err := s.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Username,
		&emailPtr,
		&user.IsAdmin,
		&user.IsPremium,
		&user.CreatedAt,
		&user.TwoFAEnabled,
	)

	if err != nil {
		if err == db.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	user.Email = emailPtr
	return &user, nil
}

func (s *OAuthService) linkOAuthAccount(ctx context.Context, userID, provider, oauthID string) error {
	// This would insert into oauth_accounts table
	// To be implemented when oauth_accounts table is added in migrations
	s.logger.WithFields(logrus.Fields{
		"user_id":  userID,
		"provider": provider,
		"oauth_id": oauthID,
	}).Debug("OAuth account linking skipped (table not yet implemented)")
	return nil
}

func (s *OAuthService) generateUniqueUsername(ctx context.Context, baseUsername string) string {
	// Clean username
	username := strings.ToLower(baseUsername)
	username = strings.ReplaceAll(username, " ", "_")
	username = url.QueryEscape(username)
	if len(username) > 50 {
		username = username[:50]
	}

	// Check if username exists
	query := "SELECT 1 FROM users WHERE username = ? LIMIT 1"
	if s.db.Type() == "postgres" {
		query = "SELECT 1 FROM users WHERE username = $1 LIMIT 1"
	}

	var exists int
	err := s.db.QueryRow(ctx, query, username).Scan(&exists)
	if err == db.ErrNoRows {
		return username // Username is available
	}

	// Append random suffix
	for i := 1; i <= 100; i++ {
		candidate := fmt.Sprintf("%s_%d", username, i)
		err := s.db.QueryRow(ctx, query, candidate).Scan(&exists)
		if err == db.ErrNoRows {
			return candidate
		}
	}

	// Last resort: append random string
	randBytes := make([]byte, 4)
	rand.Read(randBytes)
	return fmt.Sprintf("%s_%s", username, hex.EncodeToString(randBytes))
}

func (s *OAuthService) generateRandomPassword() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (s *OAuthService) createUserInDB(ctx context.Context, user *db.User) error {
	query := `
		INSERT INTO users (id, username, email, password_hash, is_admin, is_premium,
		                  created_at, two_fa_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	if s.db.Type() == "postgres" {
		query = `
			INSERT INTO users (id, username, email, password_hash, is_admin, is_premium,
			                  created_at, two_fa_enabled)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`
	}

	_, err := s.db.Exec(ctx, query,
		user.ID,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.IsAdmin,
		user.IsPremium,
		user.CreatedAt,
		user.TwoFAEnabled,
	)

	return err
}

func (s *OAuthService) updateLastLogin(ctx context.Context, userID string) error {
	query := "UPDATE users SET last_login = ? WHERE id = ?"
	if s.db.Type() == "postgres" {
		query = "UPDATE users SET last_login = $1 WHERE id = $2"
	}

	_, err := s.db.Exec(ctx, query, time.Now(), userID)
	return err
}
