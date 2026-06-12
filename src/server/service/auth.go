package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// AuthService handles authentication operations
type AuthService struct {
	store *store.Store
}

// NewAuthService creates a new authentication service
func NewAuthService(st *store.Store) *AuthService {
	return &AuthService{
		store: st,
	}
}

// Admin represents an admin account
type Admin struct {
	ID          int64
	Username    string
	Email       string
	IsPrimary   bool
	TOTPEnabled bool
	CreatedAt   time.Time
	LastLogin   *time.Time
}

// User represents a regular user account
type User struct {
	ID            int64
	Username      string
	Email         string
	EmailVerified bool
	DisplayName   *string
	Avatar        *string
	Bio           *string
	TOTPEnabled   bool
	CreatedAt     time.Time
	LastLogin     *time.Time
}

// AuthenticateAdmin authenticates an admin by username and password
func (s *AuthService) AuthenticateAdmin(ctx context.Context, username, password string) (*Admin, error) {
	query := `SELECT id, username, email, password_hash, is_primary, totp_enabled, created_at, last_login
	          FROM admins WHERE username = ?`

	var admin Admin
	var passwordHash string

	err := s.store.UsersDB.QueryRowContext(ctx, query, username).Scan(
		&admin.ID, &admin.Username, &admin.Email, &passwordHash,
		&admin.IsPrimary, &admin.TOTPEnabled, &admin.CreatedAt, &admin.LastLogin,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid credentials")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query admin: %w", err)
	}

	// Verify password using Argon2id (SPEC line 129 - NOT bcrypt)
	if !verifyPasswordArgon2id(password, passwordHash) {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Update last login
	updateQuery := `UPDATE admins SET last_login = ? WHERE id = ?`
	// Best-effort update — don't fail auth if last_login update fails
	_, _ = s.store.UsersDB.ExecContext(ctx, updateQuery, time.Now(), admin.ID)

	return &admin, nil
}

// CreatePrimaryAdmin creates the first admin account during setup
func (s *AuthService) CreatePrimaryAdmin(ctx context.Context, username, password, email string) error {
	// Check if any admin exists
	var count int
	err := s.store.UsersDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM admins").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check existing admins: %w", err)
	}

	if count > 0 {
		return fmt.Errorf("admin account already exists")
	}

	// Hash password with Argon2id
	passwordHash, err := hashPasswordArgon2id(password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert primary admin
	query := `INSERT INTO admins (username, email, password_hash, is_primary, created_at)
	          VALUES (?, ?, ?, 1, ?)`

	_, err = s.store.UsersDB.ExecContext(ctx, query, username, email, passwordHash, time.Now())
	if err != nil {
		return fmt.Errorf("failed to create admin: %w", err)
	}

	return nil
}

// CreateSession creates a new admin session in server.db (admin_sessions table).
func (s *AuthService) CreateSession(ctx context.Context, adminID int64, rememberMe bool) (string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}

	var expiresAt int64
	if rememberMe {
		expiresAt = time.Now().Add(90 * 24 * time.Hour).Unix()
	} else {
		expiresAt = time.Now().Add(30 * 24 * time.Hour).Unix()
	}

	_, err = s.store.ServerDB.ExecContext(ctx,
		`INSERT INTO admin_sessions (id, admin_id, expires_at) VALUES (?, ?, ?)`,
		sessionID, adminID, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	return sessionID, nil
}

// ValidateSession validates an admin session from server.db and returns the admin
// from users.db. SQLite does not support cross-file JOINs, so two queries are used.
func (s *AuthService) ValidateSession(ctx context.Context, sessionID string) (*Admin, error) {
	// Step 1: look up session in server.db
	var adminID int64
	err := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT admin_id FROM admin_sessions WHERE id = ? AND expires_at > ?`,
		sessionID, time.Now().Unix(),
	).Scan(&adminID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid or expired session")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to validate session: %w", err)
	}

	// Step 2: fetch admin from users.db
	var admin Admin
	err = s.store.UsersDB.QueryRowContext(ctx,
		`SELECT id, username, email, is_primary, totp_enabled, created_at, last_login
		 FROM admins WHERE id = ?`,
		adminID,
	).Scan(&admin.ID, &admin.Username, &admin.Email,
		&admin.IsPrimary, &admin.TOTPEnabled, &admin.CreatedAt, &admin.LastLogin)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid or expired session")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load admin: %w", err)
	}

	return &admin, nil
}

// DeleteSession deletes an admin session from server.db (logout).
func (s *AuthService) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.store.ServerDB.ExecContext(ctx,
		`DELETE FROM admin_sessions WHERE id = ?`, sessionID)
	return err
}

// NeedsSetup checks if the application needs first-run setup
// Returns true if no admin accounts exist
func (s *AuthService) NeedsSetup(ctx context.Context) (bool, error) {
	var count int
	err := s.store.UsersDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM admins").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check admin count: %w", err)
	}
	return count == 0, nil
}

// generateSessionID generates a cryptographically secure session ID
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// RegisterUser creates a new regular user account
func (s *AuthService) RegisterUser(ctx context.Context, username, email, password string) (*User, error) {
	// Normalize username and email (case-insensitive per PART 23)
	username = strings.ToLower(strings.TrimSpace(username))
	email = strings.ToLower(strings.TrimSpace(email))

	// Check if username already exists
	var count int
	err := s.store.UsersDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check username: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("unable to complete registration")
	}

	// Check if email already exists
	err = s.store.UsersDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE email = ?", email).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check email: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("unable to complete registration")
	}

	// Hash password with Argon2id (per spec - NOT bcrypt)
	passwordHash, err := hashPasswordArgon2id(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert user
	query := `INSERT INTO users (username, email, password_hash, email_verified, created_at)
	          VALUES (?, ?, ?, 0, ?)`

	result, err := s.store.UsersDB.ExecContext(ctx, query, username, email, passwordHash, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get user ID: %w", err)
	}

	// Return created user
	user := &User{
		ID:            userID,
		Username:      username,
		Email:         email,
		EmailVerified: false,
		TOTPEnabled:   false,
		CreatedAt:     time.Now(),
	}

	return user, nil
}

// AuthenticateUser authenticates a regular user by username/email and password
func (s *AuthService) AuthenticateUser(ctx context.Context, identifier, password string) (*User, error) {
	// Normalize identifier (case-insensitive)
	identifier = strings.ToLower(strings.TrimSpace(identifier))

	// Try username or email
	query := `SELECT id, username, email, email_verified, totp_enabled, created_at, last_login, password_hash
	          FROM users WHERE (username = ? OR email = ?)`

	var user User
	var passwordHash string

	err := s.store.UsersDB.QueryRowContext(ctx, query, identifier, identifier).Scan(
		&user.ID, &user.Username, &user.Email, &user.EmailVerified,
		&user.TOTPEnabled, &user.CreatedAt, &user.LastLogin, &passwordHash,
	)

	if err == sql.ErrNoRows {
		// Run Argon2id against a dummy hash so timing is identical to
		// the wrong-password path and leaks no account-existence signal.
		const dummyHash = "$argon2id$v=19$m=65536,t=1,p=1$dGVzdHNhbHQ$dGVzdGhhc2g"
		verifyPasswordArgon2id(password, dummyHash)
		return nil, fmt.Errorf("invalid credentials")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	if !verifyPasswordArgon2id(password, passwordHash) {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Update last login
	updateQuery := `UPDATE users SET last_login = ? WHERE id = ?`
	// Best-effort update — don't fail auth if last_login update fails
	_, _ = s.store.UsersDB.ExecContext(ctx, updateQuery, time.Now(), user.ID)

	return &user, nil
}

// CreateUserSession creates a new regular user session in users.db (user_sessions table).
func (s *AuthService) CreateUserSession(ctx context.Context, userID int64, rememberMe bool) (string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}

	// 7 days default, 30 days with rememberMe (per PART 23)
	var expiresAt int64
	if rememberMe {
		expiresAt = time.Now().Add(30 * 24 * time.Hour).Unix()
	} else {
		expiresAt = time.Now().Add(7 * 24 * time.Hour).Unix()
	}

	_, err = s.store.UsersDB.ExecContext(ctx,
		`INSERT INTO user_sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		sessionID, userID, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	return sessionID, nil
}

// ValidateUserSession validates a user session from user_sessions and returns the user.
// Both tables are in users.db so a JOIN is used.
func (s *AuthService) ValidateUserSession(ctx context.Context, sessionID string) (*User, error) {
	query := `SELECT u.id, u.username, u.email, u.email_verified, u.totp_enabled, u.created_at, u.last_login
	          FROM user_sessions s
	          JOIN users u ON s.user_id = u.id
	          WHERE s.id = ? AND s.expires_at > ?`

	var user User
	err := s.store.UsersDB.QueryRowContext(ctx, query, sessionID, time.Now().Unix()).Scan(
		&user.ID, &user.Username, &user.Email, &user.EmailVerified,
		&user.TOTPEnabled, &user.CreatedAt, &user.LastLogin,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid or expired session")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to validate session: %w", err)
	}

	return &user, nil
}

// CreatePasswordResetToken creates a password reset token for a user
// Per PART 23: expires in 1 hour (actually 24 hours per PART 26 line 22750)
func (s *AuthService) CreatePasswordResetToken(ctx context.Context, email string, userType string) (string, error) {
	// Find user by email (case-insensitive per PART 23)
	email = strings.ToLower(strings.TrimSpace(email))

	var userID int64
	var query string
	if userType == "admin" {
		query = "SELECT id FROM admins WHERE email = ?"
	} else {
		query = "SELECT id FROM users WHERE email = ?"
	}

	err := s.store.UsersDB.QueryRowContext(ctx, query, email).Scan(&userID)
	if err == sql.ErrNoRows {
		// Don't reveal if email exists per PART 23 (generic error)
		// Still return success to prevent enumeration
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query user: %w", err)
	}

	// Generate token (32 random bytes)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Hash token for storage (SHA256 per spec line 6975)
	tokenHash := hashToken(token)

	// Insert reset token (expires in 24 hours per PART 26 line 22750)
	insertQuery := `INSERT INTO password_resets (token_hash, user_type, user_id, expires_at)
	                VALUES (?, ?, ?, ?)`

	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	_, err = s.store.UsersDB.ExecContext(ctx, insertQuery, tokenHash, userType, userID, expiresAt)
	if err != nil {
		return "", fmt.Errorf("failed to create reset token: %w", err)
	}

	return token, nil
}

// ValidatePasswordResetToken validates a reset token and returns the user ID
func (s *AuthService) ValidatePasswordResetToken(ctx context.Context, token string) (int64, string, error) {
	tokenHash := hashToken(token)

	query := `SELECT user_id, user_type FROM password_resets
	          WHERE token_hash = ? AND expires_at > ? AND used_at IS NULL`

	var userID int64
	var userType string
	err := s.store.UsersDB.QueryRowContext(ctx, query, tokenHash, time.Now().Unix()).Scan(&userID, &userType)

	if err == sql.ErrNoRows {
		return 0, "", fmt.Errorf("invalid or expired reset token")
	}
	if err != nil {
		return 0, "", fmt.Errorf("failed to validate token: %w", err)
	}

	return userID, userType, nil
}

// ResetPassword resets a user's password using a valid token
// Per PART 23: invalidates all existing sessions
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	// Validate token
	userID, userType, err := s.ValidatePasswordResetToken(ctx, token)
	if err != nil {
		return err
	}

	// Hash new password with Argon2id (per spec - NOT bcrypt)
	passwordHash, err := hashPasswordArgon2id(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	var updateQuery string
	if userType == "admin" {
		updateQuery = "UPDATE admins SET password_hash = ? WHERE id = ?"
	} else {
		updateQuery = "UPDATE users SET password_hash = ? WHERE id = ?"
	}

	_, err = s.store.UsersDB.ExecContext(ctx, updateQuery, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Mark token as used
	tokenHash := hashToken(token)
	markUsedQuery := `UPDATE password_resets SET used_at = ? WHERE token_hash = ?`
	// Best-effort update — don't fail password reset if marking token fails
	_, _ = s.store.UsersDB.ExecContext(ctx, markUsedQuery, time.Now().Unix(), tokenHash)

	// Invalidate all existing sessions per PART 23 line 20534.
	// Admin sessions live in server.db; user sessions live in users.db.
	if userType == "admin" {
		_, _ = s.store.ServerDB.ExecContext(ctx,
			`DELETE FROM admin_sessions WHERE admin_id = ?`, userID)
	} else {
		_, _ = s.store.UsersDB.ExecContext(ctx,
			`DELETE FROM user_sessions WHERE user_id = ?`, userID)
	}

	return nil
}

// VerifyPassword verifies a user's current password
func (s *AuthService) VerifyPassword(userID int64, password string) error {
	query := `SELECT password_hash FROM users WHERE id = ?`
	
	var passwordHash string
	err := s.store.UsersDB.QueryRow(query, userID).Scan(&passwordHash)
	if err != nil {
		return fmt.Errorf("failed to get password hash: %w", err)
	}
	
	if !verifyPasswordArgon2id(password, passwordHash) {
		return fmt.Errorf("incorrect password")
	}
	
	return nil
}

// ChangePassword changes a user's password
func (s *AuthService) ChangePassword(userID int64, newPassword string) error {
	// Hash new password with Argon2id
	passwordHash, err := hashPasswordArgon2id(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	query := `UPDATE users SET password_hash = ? WHERE id = ?`
	if _, err := s.store.UsersDB.Exec(query, passwordHash, userID); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// hashToken hashes a token using SHA256 for storage per PART 23
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}


// Session represents an active session with metadata.
type Session struct {
	ID         string
	UserID     int64
	UserType   string
	IPAddress  string
	UserAgent  string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastActive time.Time
}

// GetUserSessions retrieves all active sessions for a user.
// Admin sessions are read from server.db; user sessions from users.db.
func (s *AuthService) GetUserSessions(ctx context.Context, userID int64, userType string) ([]Session, error) {
	now := time.Now().Unix()

	var rows *sql.Rows
	var err error

	if userType == "admin" {
		rows, err = s.store.ServerDB.QueryContext(ctx,
			`SELECT id, admin_id, expires_at, created_at
			 FROM admin_sessions
			 WHERE admin_id = ? AND expires_at > ?
			 ORDER BY created_at DESC`,
			userID, now)
	} else {
		rows, err = s.store.UsersDB.QueryContext(ctx,
			`SELECT id, user_id, expires_at, created_at
			 FROM user_sessions
			 WHERE user_id = ? AND expires_at > ?
			 ORDER BY created_at DESC`,
			userID, now)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		var expiresUnix, createdUnix int64
		if err := rows.Scan(&sess.ID, &sess.UserID, &expiresUnix, &createdUnix); err != nil {
			continue
		}
		sess.UserType = userType
		sess.ExpiresAt = time.Unix(expiresUnix, 0)
		sess.CreatedAt = time.Unix(createdUnix, 0)
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("session row iteration failed: %w", err)
	}

	return sessions, nil
}

// RevokeSession revokes a specific user session from users.db.
// For admin session revocation use DeleteSession.
func (s *AuthService) RevokeSession(ctx context.Context, sessionID string) error {
	_, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM user_sessions WHERE id = ?`, sessionID)
	return err
}

// RevokeAllUserSessions revokes all sessions for a user except the current one.
// Routes to admin_sessions (server.db) or user_sessions (users.db) based on userType.
func (s *AuthService) RevokeAllUserSessions(ctx context.Context, userID int64, userType string, exceptSessionID string) error {
	if userType == "admin" {
		_, err := s.store.ServerDB.ExecContext(ctx,
			`DELETE FROM admin_sessions WHERE admin_id = ? AND id != ?`,
			userID, exceptSessionID)
		return err
	}
	_, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM user_sessions WHERE user_id = ? AND id != ?`,
		userID, exceptSessionID)
	return err
}
