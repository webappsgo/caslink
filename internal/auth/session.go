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

// SessionStore manages user sessions
type SessionStore struct {
	db     *db.DB
	config *config.SessionConfig
	logger *logrus.Logger
}

// NewSessionStore creates a new session store
func NewSessionStore(database *db.DB, cfg *config.SessionConfig, logger *logrus.Logger) (*SessionStore, error) {
	return &SessionStore{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// CreateSession creates a new session for a user
func (ss *SessionStore) CreateSession(ctx context.Context, userID string, rememberMe bool, metadata *SessionMetadata) (*Session, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	now := time.Now()
	var expiresAt time.Time

	if rememberMe {
		expiresAt = now.Add(ss.config.RememberMeDuration)
	} else {
		expiresAt = now.Add(ss.config.Timeout)
	}

	var ipAddr, userAgent *string
	if metadata != nil {
		if metadata.IPAddress != "" {
			ipAddr = &metadata.IPAddress
		}
		if metadata.UserAgent != "" {
			userAgent = &metadata.UserAgent
		}
	}

	session := &Session{
		ID:           sessionID,
		UserID:       userID,
		ExpiresAt:    expiresAt,
		CreatedAt:    now,
		LastAccessed: now,
		RememberMe:   rememberMe,
		IPAddress:    ipAddr,
		UserAgent:    userAgent,
		Active:       true,
	}

	if err := ss.createSessionInDB(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session in database: %w", err)
	}

	ss.logger.WithFields(logrus.Fields{
		"session_id":  session.ID,
		"user_id":     userID,
		"remember_me": rememberMe,
		"expires_at":  expiresAt,
		"ip_address":  metadata.IPAddress,
	}).Info("New session created")

	return session, nil
}

// GetSession retrieves a session by ID
func (ss *SessionStore) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := ss.getSessionFromDB(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		// Invalidate expired session
		if err := ss.InvalidateSession(ctx, sessionID); err != nil {
			ss.logger.WithError(err).WithField("session_id", sessionID).Error("Failed to invalidate expired session")
		}
		return nil, ErrSessionExpired
	}

	// Update last accessed time
	if err := ss.updateLastAccessed(ctx, sessionID); err != nil {
		ss.logger.WithError(err).WithField("session_id", sessionID).Error("Failed to update session last accessed time")
		// Don't fail the request for this
	}

	return session, nil
}

// InvalidateSession invalidates a session
func (ss *SessionStore) InvalidateSession(ctx context.Context, sessionID string) error {
	query := "UPDATE sessions SET active = false WHERE id = ?"
	if ss.db.Type() == "postgres" {
		query = "UPDATE sessions SET active = false WHERE id = $1"
	}

	_, err := ss.db.Exec(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to invalidate session: %w", err)
	}

	ss.logger.WithField("session_id", sessionID).Info("Session invalidated")
	return nil
}

// InvalidateUserSessions invalidates all sessions for a user
func (ss *SessionStore) InvalidateUserSessions(ctx context.Context, userID string) error {
	query := "UPDATE sessions SET active = false WHERE user_id = ?"
	if ss.db.Type() == "postgres" {
		query = "UPDATE sessions SET active = false WHERE user_id = $1"
	}

	result, err := ss.db.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to invalidate user sessions: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	ss.logger.WithFields(logrus.Fields{
		"user_id":       userID,
		"rows_affected": rowsAffected,
	}).Info("User sessions invalidated")

	return nil
}

// CleanupExpiredSessions removes expired sessions from the database
func (ss *SessionStore) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	query := "DELETE FROM sessions WHERE expires_at <= ? OR active = false"
	if ss.db.Type() == "postgres" {
		query = "DELETE FROM sessions WHERE expires_at <= $1 OR active = false"
	}

	result, err := ss.db.Exec(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	ss.logger.WithField("rows_affected", rowsAffected).Info("Expired sessions cleaned up")

	return rowsAffected, nil
}

// ListUserSessions lists active sessions for a user
func (ss *SessionStore) ListUserSessions(ctx context.Context, userID string) ([]*Session, error) {
	query := `
		SELECT id, user_id, expires_at, created_at, last_accessed,
		       remember_me, ip_address, user_agent, active
		FROM sessions
		WHERE user_id = ? AND active = true AND expires_at > ?
		ORDER BY last_accessed DESC
	`

	if ss.db.Type() == "postgres" {
		query = `
			SELECT id, user_id, expires_at, created_at, last_accessed,
			       remember_me, ip_address, user_agent, active
			FROM sessions
			WHERE user_id = $1 AND active = true AND expires_at > $2
			ORDER BY last_accessed DESC
		`
	}

	rows, err := ss.db.Query(ctx, query, userID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to list user sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var session Session
		var ipAddress, userAgent *string

		err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.ExpiresAt,
			&session.CreatedAt,
			&session.LastAccessed,
			&session.RememberMe,
			&ipAddress,
			&userAgent,
			&session.Active,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session.IPAddress = ipAddress
		session.UserAgent = userAgent
		sessions = append(sessions, &session)
	}

	return sessions, rows.Err()
}

// GetSessionStats returns session statistics
func (ss *SessionStore) GetSessionStats(ctx context.Context) (*SessionStats, error) {
	stats := &SessionStats{}

	// Count active sessions
	query := "SELECT COUNT(*) FROM sessions WHERE active = true AND expires_at > ?"
	if ss.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM sessions WHERE active = true AND expires_at > $1"
	}

	err := ss.db.QueryRow(ctx, query, time.Now()).Scan(&stats.ActiveSessions)
	if err != nil {
		return nil, fmt.Errorf("failed to count active sessions: %w", err)
	}

	// Count expired sessions
	query = "SELECT COUNT(*) FROM sessions WHERE expires_at <= ?"
	if ss.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM sessions WHERE expires_at <= $1"
	}

	err = ss.db.QueryRow(ctx, query, time.Now()).Scan(&stats.ExpiredSessions)
	if err != nil {
		return nil, fmt.Errorf("failed to count expired sessions: %w", err)
	}

	// Count sessions created today
	today := time.Now().Truncate(24 * time.Hour)
	query = "SELECT COUNT(*) FROM sessions WHERE created_at >= ?"
	if ss.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM sessions WHERE created_at >= $1"
	}

	err = ss.db.QueryRow(ctx, query, today).Scan(&stats.SessionsToday)
	if err != nil {
		return nil, fmt.Errorf("failed to count sessions today: %w", err)
	}

	// Count remember me sessions
	query = "SELECT COUNT(*) FROM sessions WHERE remember_me = true AND active = true AND expires_at > ?"
	if ss.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM sessions WHERE remember_me = true AND active = true AND expires_at > $1"
	}

	err = ss.db.QueryRow(ctx, query, time.Now()).Scan(&stats.RememberMeSessions)
	if err != nil {
		return nil, fmt.Errorf("failed to count remember me sessions: %w", err)
	}

	return stats, nil
}

// Helper functions

func (ss *SessionStore) createSessionInDB(ctx context.Context, session *Session) error {
	query := `
		INSERT INTO sessions (id, user_id, expires_at, created_at, last_accessed,
		                     remember_me, ip_address, user_agent, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	if ss.db.Type() == "postgres" {
		query = `
			INSERT INTO sessions (id, user_id, expires_at, created_at, last_accessed,
			                     remember_me, ip_address, user_agent, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`
	}

	_, err := ss.db.Exec(ctx, query,
		session.ID,
		session.UserID,
		session.ExpiresAt,
		session.CreatedAt,
		session.LastAccessed,
		session.RememberMe,
		session.IPAddress,
		session.UserAgent,
		session.Active,
	)

	return err
}

func (ss *SessionStore) getSessionFromDB(ctx context.Context, sessionID string) (*Session, error) {
	query := `
		SELECT id, user_id, expires_at, created_at, last_accessed,
		       remember_me, ip_address, user_agent, active
		FROM sessions
		WHERE id = ? AND active = true
	`

	if ss.db.Type() == "postgres" {
		query = `
			SELECT id, user_id, expires_at, created_at, last_accessed,
			       remember_me, ip_address, user_agent, active
			FROM sessions
			WHERE id = $1 AND active = true
		`
	}

	var session Session
	var ipAddress, userAgent *string

	err := ss.db.QueryRow(ctx, query, sessionID).Scan(
		&session.ID,
		&session.UserID,
		&session.ExpiresAt,
		&session.CreatedAt,
		&session.LastAccessed,
		&session.RememberMe,
		&ipAddress,
		&userAgent,
		&session.Active,
	)

	if err != nil {
		if err == db.ErrNoRows {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	session.IPAddress = ipAddress
	session.UserAgent = userAgent

	return &session, nil
}

func (ss *SessionStore) updateLastAccessed(ctx context.Context, sessionID string) error {
	query := "UPDATE sessions SET last_accessed = ? WHERE id = ?"
	if ss.db.Type() == "postgres" {
		query = "UPDATE sessions SET last_accessed = $1 WHERE id = $2"
	}

	_, err := ss.db.Exec(ctx, query, time.Now(), sessionID)
	return err
}

// generateSessionID generates a cryptographically secure session ID
func generateSessionID() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// SessionCookie creates a session cookie
func (ss *SessionStore) CreateSessionCookie(sessionID string, secure bool, domain string) *SessionCookie {
	cookie := &SessionCookie{
		Name:     ss.config.CookieName,
		Value:    sessionID,
		Path:     "/",
		Domain:   domain,
		Secure:   secure,
		HTTPOnly: true,
		SameSite: "Lax",
	}

	// Don't set MaxAge for session cookies - let browser handle it
	// For remember me sessions, the expiry is handled server-side

	return cookie
}

// ClearSessionCookie creates a cookie that clears the session
func (ss *SessionStore) ClearSessionCookie(domain string) *SessionCookie {
	return &SessionCookie{
		Name:     ss.config.CookieName,
		Value:    "",
		Path:     "/",
		Domain:   domain,
		Secure:   true,
		HTTPOnly: true,
		SameSite: "Lax",
		MaxAge:   -1, // Immediately expire
	}
}

// ValidateSessionID checks if a session ID is valid format
func ValidateSessionID(sessionID string) bool {
	// Session ID should be 64 hex characters (32 bytes)
	if len(sessionID) != 64 {
		return false
	}

	// Check if all characters are valid hex
	for _, char := range sessionID {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}

	return true
}

// GetSessionFromCookie extracts session ID from cookie value
func GetSessionFromCookie(cookieValue string) string {
	// In a more complex implementation, this might decrypt or verify the cookie
	// For now, we assume the cookie value is the session ID directly
	if ValidateSessionID(cookieValue) {
		return cookieValue
	}
	return ""
}