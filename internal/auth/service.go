package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Service handles authentication and authorization
type Service struct {
	db             *db.DB
	config         *config.AuthConfig
	logger         *logrus.Logger
	passwordHasher *PasswordHasher
	sessionStore   *SessionStore
	tokenManager   *TokenManager
}

// NewService creates a new authentication service
func NewService(database *db.DB, cfg *config.AuthConfig, logger *logrus.Logger) (*Service, error) {
	passwordHasher := NewPasswordHasher(cfg.Password.Cost)

	sessionStore, err := NewSessionStore(database, &cfg.Session, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create session store: %w", err)
	}

	tokenManager, err := NewTokenManager(database, &cfg.APIToken, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create token manager: %w", err)
	}

	return &Service{
		db:             database,
		config:         cfg,
		logger:         logger,
		passwordHasher: passwordHasher,
		sessionStore:   sessionStore,
		tokenManager:   tokenManager,
	}, nil
}

// AuthenticateUser authenticates a user with username/email and password
func (s *Service) AuthenticateUser(ctx context.Context, identifier, password string) (*UserInfo, error) {
	// Find user by username or email
	user, err := s.findUserByIdentifier(ctx, identifier)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"identifier": identifier,
			"error":      err,
		}).Warn("User not found during authentication")
		return nil, ErrInvalidCredentials
	}

	// Verify password
	if !s.passwordHasher.VerifyPassword(password, user.PasswordHash) {
		s.logger.WithFields(logrus.Fields{
			"user_id":    user.ID,
			"username":   user.Username,
			"identifier": identifier,
		}).Warn("Invalid password during authentication")
		return nil, ErrInvalidCredentials
	}

	// Note: Active field check removed as it doesn't exist in db.User model

	// Update last login
	if err := s.updateLastLogin(ctx, user.ID); err != nil {
		s.logger.WithError(err).WithField("user_id", user.ID).Error("Failed to update last login")
		// Don't fail authentication for this
	}

	return &UserInfo{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		IsAdmin:    user.IsAdmin,
		IsPremium:  user.IsPremium,
		CreatedAt:  user.CreatedAt,
		LastLogin:  &time.Time{},
		TwoFAEnabled: user.TwoFAEnabled,
	}, nil
}

// CreateUser creates a new user account
func (s *Service) CreateUser(ctx context.Context, req *CreateUserRequest) (*UserInfo, error) {
	// Validate request
	if err := s.validateCreateUserRequest(req); err != nil {
		return nil, err
	}

	// Check if username already exists
	if exists, err := s.usernameExists(ctx, req.Username); err != nil {
		return nil, fmt.Errorf("failed to check username existence: %w", err)
	} else if exists {
		return nil, ErrUsernameExists
	}

	// Check if email already exists (if provided)
	if req.Email != nil && *req.Email != "" {
		if exists, err := s.emailExists(ctx, *req.Email); err != nil {
			return nil, fmt.Errorf("failed to check email existence: %w", err)
		} else if exists {
			return nil, ErrEmailExists
		}
	}

	// Hash password
	passwordHash, err := s.passwordHasher.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Generate user ID
	userID, err := generateUserID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate user ID: %w", err)
	}

	// Create user in database
	user := &db.User{
		ID:           userID,
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		IsAdmin:      req.IsAdmin,
		IsPremium:    false,
		CreatedAt:    time.Now(),
		TwoFAEnabled: false,
	}

	if err := s.createUserInDB(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user in database: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":  user.ID,
		"username": user.Username,
		"is_admin": user.IsAdmin,
	}).Info("New user created")

	return &UserInfo{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		IsAdmin:    user.IsAdmin,
		IsPremium:  user.IsPremium,
		CreatedAt:  user.CreatedAt,
		TwoFAEnabled: user.TwoFAEnabled,
	}, nil
}

// GetUser retrieves user information by ID
func (s *Service) GetUser(ctx context.Context, userID string) (*UserInfo, error) {
	user, err := s.getUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserInfo{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		IsAdmin:    user.IsAdmin,
		IsPremium:  user.IsPremium,
		CreatedAt:  user.CreatedAt,
		LastLogin:  user.LastLogin,
		TwoFAEnabled: user.TwoFAEnabled,
	}, nil
}

// UpdateUser updates user information
func (s *Service) UpdateUser(ctx context.Context, userID string, req *UpdateUserRequest) (*UserInfo, error) {
	// Get existing user
	user, err := s.getUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Update fields
	updated := false

	if req.Email != nil && (user.Email == nil || *user.Email != *req.Email) {
		// Check if new email already exists
		if *req.Email != "" {
			if exists, err := s.emailExists(ctx, *req.Email); err != nil {
				return nil, fmt.Errorf("failed to check email existence: %w", err)
			} else if exists {
				return nil, ErrEmailExists
			}
		}
		user.Email = req.Email
		updated = true
	}

	if req.Password != nil && *req.Password != "" {
		// Hash new password
		passwordHash, err := s.passwordHasher.HashPassword(*req.Password)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		user.PasswordHash = passwordHash
		updated = true
	}

	if req.IsAdmin != nil && user.IsAdmin != *req.IsAdmin {
		user.IsAdmin = *req.IsAdmin
		updated = true
	}

	// Note: Active field update removed as it doesn't exist in db.User model

	if updated {
		if err := s.updateUserInDB(ctx, user); err != nil {
			return nil, fmt.Errorf("failed to update user in database: %w", err)
		}

		s.logger.WithFields(logrus.Fields{
			"user_id":  user.ID,
			"username": user.Username,
		}).Info("User updated")
	}

	return &UserInfo{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		IsAdmin:    user.IsAdmin,
		IsPremium:  user.IsPremium,
		CreatedAt:  user.CreatedAt,
		LastLogin:  user.LastLogin,
		TwoFAEnabled: user.TwoFAEnabled,
	}, nil
}

// DeleteUser deletes a user account
func (s *Service) DeleteUser(ctx context.Context, userID string) error {
	// Get user to verify existence
	user, err := s.getUserByID(ctx, userID)
	if err != nil {
		return err
	}

	// Delete user from database
	if err := s.deleteUserFromDB(ctx, userID); err != nil {
		return fmt.Errorf("failed to delete user from database: %w", err)
	}

	// Invalidate all sessions for this user
	if err := s.sessionStore.InvalidateUserSessions(ctx, userID); err != nil {
		s.logger.WithError(err).WithField("user_id", userID).Error("Failed to invalidate user sessions")
	}

	// Revoke all API tokens for this user
	if err := s.tokenManager.RevokeUserTokens(ctx, userID); err != nil {
		s.logger.WithError(err).WithField("user_id", userID).Error("Failed to revoke user tokens")
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":  user.ID,
		"username": user.Username,
	}).Info("User deleted")

	return nil
}

// ListUsers retrieves a list of users with pagination
func (s *Service) ListUsers(ctx context.Context, req *ListUsersRequest) (*ListUsersResponse, error) {
	users, total, err := s.listUsersFromDB(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list users from database: %w", err)
	}

	userInfos := make([]*UserInfo, len(users))
	for i, user := range users {
		userInfos[i] = &UserInfo{
			ID:         user.ID,
			Username:   user.Username,
			Email:      user.Email,
			IsAdmin:    user.IsAdmin,
			IsPremium:  user.IsPremium,
			CreatedAt:  user.CreatedAt,
			LastLogin:  user.LastLogin,
			TwoFAEnabled: user.TwoFAEnabled,
		}
	}

	return &ListUsersResponse{
		Users: userInfos,
		Total: total,
		Page:  req.Page,
		Limit: req.Limit,
	}, nil
}

// CreateSession creates a new session for a user
func (s *Service) CreateSession(ctx context.Context, userID string, rememberMe bool, metadata *SessionMetadata) (*Session, error) {
	return s.sessionStore.CreateSession(ctx, userID, rememberMe, metadata)
}

// GetSession retrieves a session by ID
func (s *Service) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	return s.sessionStore.GetSession(ctx, sessionID)
}

// InvalidateSession invalidates a session
func (s *Service) InvalidateSession(ctx context.Context, sessionID string) error {
	return s.sessionStore.InvalidateSession(ctx, sessionID)
}

// CreateAPIToken creates a new API token for a user
func (s *Service) CreateAPIToken(ctx context.Context, req *CreateTokenRequest) (*APIToken, error) {
	return s.tokenManager.CreateToken(ctx, req)
}

// ValidateAPIToken validates an API token and returns user info
func (s *Service) ValidateAPIToken(ctx context.Context, token string) (*UserInfo, error) {
	tokenInfo, err := s.tokenManager.ValidateToken(ctx, token)
	if err != nil {
		return nil, err
	}

	// Get user info
	user, err := s.getUserByID(ctx, tokenInfo.UserID)
	if err != nil {
		return nil, err
	}

	// Note: Active field check removed as it doesn't exist in db.User model

	return &UserInfo{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		IsAdmin:    user.IsAdmin,
		IsPremium:  user.IsPremium,
		CreatedAt:  user.CreatedAt,
		LastLogin:  user.LastLogin,
		TwoFAEnabled: user.TwoFAEnabled,
	}, nil
}

// ListAPITokens lists API tokens for a user
func (s *Service) ListAPITokens(ctx context.Context, userID string) ([]*APIToken, error) {
	return s.tokenManager.ListTokens(ctx, userID)
}

// RevokeAPIToken revokes an API token
func (s *Service) RevokeAPIToken(ctx context.Context, tokenID string) error {
	return s.tokenManager.RevokeToken(ctx, tokenID)
}

// Helper functions

func (s *Service) findUserByIdentifier(ctx context.Context, identifier string) (*db.User, error) {
	query := `
		SELECT id, username, email, password_hash, is_admin, is_premium,
		       created_at, last_login, two_fa_enabled
		FROM users
		WHERE username = ? OR email = ?
		LIMIT 1
	`

	if s.db.Type() == "postgres" {
		query = `
			SELECT id, username, email, password_hash, is_admin, is_premium,
			       created_at, last_login, two_fa_enabled
			FROM users
			WHERE username = $1 OR email = $1
			LIMIT 1
		`
	}

	var user db.User
	var email *string
	var lastLogin *time.Time

	err := s.db.QueryRow(ctx, query, identifier).Scan(
		&user.ID,
		&user.Username,
		&email,
		&user.PasswordHash,
		&user.IsAdmin,
		&user.IsPremium,
		&user.CreatedAt,
		&lastLogin,
		&user.TwoFAEnabled,
	)

	if err != nil {
		if err == db.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	user.Email = email
	user.LastLogin = lastLogin

	return &user, nil
}

func (s *Service) getUserByID(ctx context.Context, userID string) (*db.User, error) {
	query := `
		SELECT id, username, email, password_hash, is_admin, is_premium,
		       created_at, last_login, two_fa_enabled
		FROM users
		WHERE id = ?
	`

	if s.db.Type() == "postgres" {
		query = `
			SELECT id, username, email, password_hash, is_admin, is_premium,
			       created_at, last_login, two_fa_enabled
			FROM users
			WHERE id = $1
		`
	}

	var user db.User
	var email *string
	var lastLogin *time.Time

	err := s.db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Username,
		&email,
		&user.PasswordHash,
		&user.IsAdmin,
		&user.IsPremium,
		&user.CreatedAt,
		&lastLogin,
		&user.TwoFAEnabled,
	)

	if err != nil {
		if err == db.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	user.Email = email
	user.LastLogin = lastLogin

	return &user, nil
}

func (s *Service) usernameExists(ctx context.Context, username string) (bool, error) {
	query := "SELECT 1 FROM users WHERE username = ? LIMIT 1"
	if s.db.Type() == "postgres" {
		query = "SELECT 1 FROM users WHERE username = $1 LIMIT 1"
	}

	var exists int
	err := s.db.QueryRow(ctx, query, username).Scan(&exists)
	if err != nil {
		if err == db.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (s *Service) emailExists(ctx context.Context, email string) (bool, error) {
	query := "SELECT 1 FROM users WHERE email = ? LIMIT 1"
	if s.db.Type() == "postgres" {
		query = "SELECT 1 FROM users WHERE email = $1 LIMIT 1"
	}

	var exists int
	err := s.db.QueryRow(ctx, query, email).Scan(&exists)
	if err != nil {
		if err == db.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (s *Service) createUserInDB(ctx context.Context, user *db.User) error {
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

func (s *Service) updateUserInDB(ctx context.Context, user *db.User) error {
	query := `
		UPDATE users
		SET email = ?, password_hash = ?, is_admin = ?, is_premium = ?,
		    two_fa_enabled = ?
		WHERE id = ?
	`

	if s.db.Type() == "postgres" {
		query = `
			UPDATE users
			SET email = $1, password_hash = $2, is_admin = $3, is_premium = $4,
			    two_fa_enabled = $5
			WHERE id = $6
		`
	}

	_, err := s.db.Exec(ctx, query,
		user.Email,
		user.PasswordHash,
		user.IsAdmin,
		user.IsPremium,
		user.TwoFAEnabled,
		user.ID,
	)

	return err
}

func (s *Service) deleteUserFromDB(ctx context.Context, userID string) error {
	query := "DELETE FROM users WHERE id = ?"
	if s.db.Type() == "postgres" {
		query = "DELETE FROM users WHERE id = $1"
	}

	_, err := s.db.Exec(ctx, query, userID)
	return err
}

func (s *Service) updateLastLogin(ctx context.Context, userID string) error {
	query := "UPDATE users SET last_login = ? WHERE id = ?"
	if s.db.Type() == "postgres" {
		query = "UPDATE users SET last_login = $1 WHERE id = $2"
	}

	_, err := s.db.Exec(ctx, query, time.Now(), userID)
	return err
}

func (s *Service) listUsersFromDB(ctx context.Context, req *ListUsersRequest) ([]*db.User, int64, error) {
	// Count total users
	countQuery := "SELECT COUNT(*) FROM users"
	var total int64
	err := s.db.QueryRow(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Build query
	query := `
		SELECT id, username, email, is_admin, is_premium,
		       created_at, last_login, two_fa_enabled
		FROM users
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	if s.db.Type() == "postgres" {
		query = `
			SELECT id, username, email, is_admin, is_premium,
			       created_at, last_login, two_fa_enabled
			FROM users
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`
	}

	offset := (req.Page - 1) * req.Limit
	rows, err := s.db.Query(ctx, query, req.Limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*db.User
	for rows.Next() {
		var user db.User
		var email *string
		var lastLogin *time.Time

		err := rows.Scan(
			&user.ID,
			&user.Username,
			&email,
			&user.IsAdmin,
			&user.IsPremium,
			&user.CreatedAt,
			&lastLogin,
			&user.TwoFAEnabled,
		)
		if err != nil {
			return nil, 0, err
		}

		user.Email = email
		user.LastLogin = lastLogin
		users = append(users, &user)
	}

	return users, total, rows.Err()
}

func (s *Service) validateCreateUserRequest(req *CreateUserRequest) error {
	if req.Username == "" {
		return ErrInvalidUsername
	}

	if len(req.Username) < 3 || len(req.Username) > 50 {
		return ErrInvalidUsername
	}

	if req.Password == "" {
		return ErrInvalidPassword
	}

	if len(req.Password) < s.config.Password.MinLength {
		return ErrPasswordTooShort
	}

	if req.Email != nil && *req.Email != "" {
		if !isValidEmail(*req.Email) {
			return ErrInvalidEmail
		}
	}

	return nil
}

// generateUserID generates a random user ID
func generateUserID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// isValidEmail checks if an email address is valid
func isValidEmail(email string) bool {
	// Simple email validation - in production, use a proper email validation library
	return len(email) > 0 && len(email) <= 255 &&
		   len(email) > 3 &&
		   email[0] != '@' && email[len(email)-1] != '@' &&
		   countChar(email, '@') == 1
}

func countChar(s string, c byte) int {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			count++
		}
	}
	return count
}