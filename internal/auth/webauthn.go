package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/sirupsen/logrus"
)

// WebAuthnService handles WebAuthn/FIDO2 authentication
type WebAuthnService struct {
	config      *config.AuthConfig
	db          *db.DB
	logger      *logrus.Logger
	webAuthn    *webauthn.WebAuthn
}

// WebAuthnUser implements the webauthn.User interface
type WebAuthnUser struct {
	ID          []byte
	Username    string
	DisplayName string
	Credentials []webauthn.Credential
}

// WebAuthnID returns the user's ID
func (u *WebAuthnUser) WebAuthnID() []byte {
	return u.ID
}

// WebAuthnName returns the user's username
func (u *WebAuthnUser) WebAuthnName() string {
	return u.Username
}

// WebAuthnDisplayName returns the user's display name
func (u *WebAuthnUser) WebAuthnDisplayName() string {
	return u.DisplayName
}

// WebAuthnIcon returns the user's icon URL (optional)
func (u *WebAuthnUser) WebAuthnIcon() string {
	return ""
}

// WebAuthnCredentials returns the user's credentials
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

// NewWebAuthnService creates a new WebAuthn service
func NewWebAuthnService(cfg *config.AuthConfig, database *db.DB, logger *logrus.Logger, baseURL string) (*WebAuthnService, error) {
	if !cfg.EnableWebAuthn {
		return nil, nil
	}

	// Determine WebAuthn configuration
	displayName := cfg.WebAuthnDisplayName
	if displayName == "" {
		displayName = "Caslink URL Shortener"
	}

	// Extract domain from base URL for WebAuthn ID
	rpID := cfg.WebAuthnID
	if rpID == "" || rpID == "auto" {
		// Parse base URL to get domain
		rpID = extractDomain(baseURL)
		if rpID == "" {
			rpID = "localhost"
		}
	}

	// Create WebAuthn configuration
	wconfig := &webauthn.Config{
		RPDisplayName: displayName,
		RPID:          rpID,
		RPOrigins:     []string{baseURL},
		AttestationPreference: protocol.PreferDirectAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			RequireResidentKey: protocol.ResidentKeyNotRequired(),
			ResidentKey:        protocol.ResidentKeyRequirementDiscouraged,
			UserVerification:   protocol.VerificationPreferred,
		},
	}

	webAuthnInstance, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create WebAuthn instance: %w", err)
	}

	service := &WebAuthnService{
		config:   cfg,
		db:       database,
		logger:   logger,
		webAuthn: webAuthnInstance,
	}

	logger.WithFields(logrus.Fields{
		"rp_id":          rpID,
		"rp_display_name": displayName,
		"rp_origins":     baseURL,
	}).Info("WebAuthn service initialized")

	return service, nil
}

// BeginRegistration starts the WebAuthn registration process
func (s *WebAuthnService) BeginRegistration(ctx context.Context, userID, username, credentialName string) (*protocol.CredentialCreation, string, error) {
	// Get user's existing credentials
	user, err := s.getWebAuthnUser(ctx, userID, username)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get user: %w", err)
	}

	// Generate registration options
	options, session, err := s.webAuthn.BeginRegistration(user)
	if err != nil {
		return nil, "", fmt.Errorf("failed to begin registration: %w", err)
	}

	// Store session for verification
	sessionID, err := s.storeWebAuthnSession(ctx, userID, session, "registration")
	if err != nil {
		return nil, "", fmt.Errorf("failed to store session: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":  userID,
		"username": username,
	}).Info("WebAuthn registration started")

	return options, sessionID, nil
}

// FinishRegistration completes the WebAuthn registration process
func (s *WebAuthnService) FinishRegistration(ctx context.Context, userID, username, sessionID, credentialName string, response *protocol.ParsedCredentialCreationData) error {
	// Get user
	user, err := s.getWebAuthnUser(ctx, userID, username)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Retrieve session
	session, err := s.getWebAuthnSession(ctx, sessionID, "registration")
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Verify registration
	credential, err := s.webAuthn.CreateCredential(user, *session, response)
	if err != nil {
		s.logger.WithError(err).Error("WebAuthn registration verification failed")
		return fmt.Errorf("failed to create credential: %w", err)
	}

	// Store credential
	if err := s.storeCredential(ctx, userID, credentialName, credential); err != nil {
		return fmt.Errorf("failed to store credential: %w", err)
	}

	// Delete session
	s.deleteWebAuthnSession(ctx, sessionID)

	s.logger.WithFields(logrus.Fields{
		"user_id":         userID,
		"username":        username,
		"credential_name": credentialName,
	}).Info("WebAuthn credential registered successfully")

	return nil
}

// BeginLogin starts the WebAuthn authentication process
func (s *WebAuthnService) BeginLogin(ctx context.Context, userID, username string) (*protocol.CredentialAssertion, string, error) {
	// Get user with credentials
	user, err := s.getWebAuthnUser(ctx, userID, username)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get user: %w", err)
	}

	if len(user.Credentials) == 0 {
		return nil, "", fmt.Errorf("user has no registered credentials")
	}

	// Generate authentication options
	options, session, err := s.webAuthn.BeginLogin(user)
	if err != nil {
		return nil, "", fmt.Errorf("failed to begin login: %w", err)
	}

	// Store session for verification
	sessionID, err := s.storeWebAuthnSession(ctx, userID, session, "authentication")
	if err != nil {
		return nil, "", fmt.Errorf("failed to store session: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":  userID,
		"username": username,
	}).Debug("WebAuthn authentication started")

	return options, sessionID, nil
}

// FinishLogin completes the WebAuthn authentication process
func (s *WebAuthnService) FinishLogin(ctx context.Context, userID, username, sessionID string, response *protocol.ParsedCredentialAssertionData) error {
	// Get user
	user, err := s.getWebAuthnUser(ctx, userID, username)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Retrieve session
	session, err := s.getWebAuthnSession(ctx, sessionID, "authentication")
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Verify authentication
	credential, err := s.webAuthn.ValidateLogin(user, *session, response)
	if err != nil {
		s.logger.WithError(err).Error("WebAuthn authentication verification failed")
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Update credential usage
	if err := s.updateCredentialUsage(ctx, credential.ID); err != nil {
		s.logger.WithError(err).Warn("Failed to update credential usage")
	}

	// Delete session
	s.deleteWebAuthnSession(ctx, sessionID)

	s.logger.WithFields(logrus.Fields{
		"user_id":  userID,
		"username": username,
	}).Info("WebAuthn authentication successful")

	return nil
}

// ListCredentials returns all WebAuthn credentials for a user
func (s *WebAuthnService) ListCredentials(ctx context.Context, userID string) ([]WebAuthnCredential, error) {
	query := `
		SELECT id, name, credential_id, public_key, created_at, last_used, active
		FROM webauthn_credentials
		WHERE user_id = ? AND active = true
		ORDER BY created_at DESC
	`
	if s.db.Type() == "postgres" {
		query = `
			SELECT id, name, credential_id, public_key, created_at, last_used, active
			FROM webauthn_credentials
			WHERE user_id = $1 AND active = true
			ORDER BY created_at DESC
		`
	}

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		s.logger.WithError(err).Debug("Failed to list credentials (table might not exist)")
		return []WebAuthnCredential{}, nil
	}
	defer rows.Close()

	var credentials []WebAuthnCredential
	for rows.Next() {
		var cred WebAuthnCredential
		var lastUsed *time.Time

		err := rows.Scan(
			&cred.ID,
			&cred.Name,
			&cred.CredentialID,
			&cred.PublicKey,
			&cred.CreatedAt,
			&lastUsed,
			&cred.Active,
		)
		if err != nil {
			return nil, err
		}

		cred.UserID = userID
		cred.LastUsed = lastUsed
		credentials = append(credentials, cred)
	}

	return credentials, rows.Err()
}

// DeleteCredential deletes a WebAuthn credential
func (s *WebAuthnService) DeleteCredential(ctx context.Context, userID, credentialID string) error {
	query := "UPDATE webauthn_credentials SET active = false WHERE id = ? AND user_id = ?"
	if s.db.Type() == "postgres" {
		query = "UPDATE webauthn_credentials SET active = false WHERE id = $1 AND user_id = $2"
	}

	result, err := s.db.Exec(ctx, query, credentialID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("credential not found")
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":       userID,
		"credential_id": credentialID,
	}).Info("WebAuthn credential deleted")

	return nil
}

// Helper functions

// getWebAuthnUser retrieves a user and their credentials for WebAuthn
func (s *WebAuthnService) getWebAuthnUser(ctx context.Context, userID, username string) (*WebAuthnUser, error) {
	// Get user's credentials
	credentials, err := s.loadUserCredentials(ctx, userID)
	if err != nil {
		s.logger.WithError(err).Debug("Failed to load credentials (table might not exist)")
		credentials = []webauthn.Credential{}
	}

	// Convert user ID to bytes
	userIDBytes := []byte(userID)

	return &WebAuthnUser{
		ID:          userIDBytes,
		Username:    username,
		DisplayName: username,
		Credentials: credentials,
	}, nil
}

// loadUserCredentials loads all active credentials for a user
func (s *WebAuthnService) loadUserCredentials(ctx context.Context, userID string) ([]webauthn.Credential, error) {
	query := `
		SELECT credential_id, public_key, created_at
		FROM webauthn_credentials
		WHERE user_id = ? AND active = true
	`
	if s.db.Type() == "postgres" {
		query = `
			SELECT credential_id, public_key, created_at
			FROM webauthn_credentials
			WHERE user_id = $1 AND active = true
		`
	}

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var credentials []webauthn.Credential
	for rows.Next() {
		var credID, pubKey []byte
		var createdAt time.Time

		err := rows.Scan(&credID, &pubKey, &createdAt)
		if err != nil {
			return nil, err
		}

		// Reconstruct webauthn.Credential
		cred := webauthn.Credential{
			ID:        credID,
			PublicKey: pubKey,
			// Other fields would be populated from stored data
		}

		credentials = append(credentials, cred)
	}

	return credentials, rows.Err()
}

// storeCredential stores a WebAuthn credential
func (s *WebAuthnService) storeCredential(ctx context.Context, userID, name string, credential *webauthn.Credential) error {
	id, err := generateCredentialID()
	if err != nil {
		return err
	}

	query := `
		INSERT INTO webauthn_credentials (id, user_id, name, credential_id, public_key, created_at, active)
		VALUES (?, ?, ?, ?, ?, ?, true)
	`
	if s.db.Type() == "postgres" {
		query = `
			INSERT INTO webauthn_credentials (id, user_id, name, credential_id, public_key, created_at, active)
			VALUES ($1, $2, $3, $4, $5, $6, true)
		`
	}

	_, err = s.db.Exec(ctx, query,
		id,
		userID,
		name,
		credential.ID,
		credential.PublicKey,
		time.Now(),
	)

	if err != nil {
		s.logger.WithError(err).Debug("Failed to store credential (table might not exist)")
		return err
	}

	return nil
}

// updateCredentialUsage updates the last used time for a credential
func (s *WebAuthnService) updateCredentialUsage(ctx context.Context, credentialID []byte) error {
	query := "UPDATE webauthn_credentials SET last_used = ? WHERE credential_id = ?"
	if s.db.Type() == "postgres" {
		query = "UPDATE webauthn_credentials SET last_used = $1 WHERE credential_id = $2"
	}

	_, err := s.db.Exec(ctx, query, time.Now(), credentialID)
	return err
}

// storeWebAuthnSession stores a WebAuthn session for later verification
func (s *WebAuthnService) storeWebAuthnSession(ctx context.Context, userID string, session *webauthn.SessionData, sessionType string) (string, error) {
	sessionID, err := generateWebAuthnSessionID()
	if err != nil {
		return "", err
	}

	// Serialize session data
	sessionData, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}

	query := `
		INSERT INTO webauthn_sessions (id, user_id, session_data, session_type, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	if s.db.Type() == "postgres" {
		query = `
			INSERT INTO webauthn_sessions (id, user_id, session_data, session_type, created_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`
	}

	expiresAt := time.Now().Add(5 * time.Minute) // Sessions expire in 5 minutes

	_, err = s.db.Exec(ctx, query,
		sessionID,
		userID,
		sessionData,
		sessionType,
		time.Now(),
		expiresAt,
	)

	if err != nil {
		s.logger.WithError(err).Debug("Failed to store WebAuthn session (table might not exist)")
		return "", err
	}

	return sessionID, nil
}

// getWebAuthnSession retrieves a WebAuthn session
func (s *WebAuthnService) getWebAuthnSession(ctx context.Context, sessionID, sessionType string) (*webauthn.SessionData, error) {
	query := `
		SELECT session_data, expires_at
		FROM webauthn_sessions
		WHERE id = ? AND session_type = ?
	`
	if s.db.Type() == "postgres" {
		query = `
			SELECT session_data, expires_at
			FROM webauthn_sessions
			WHERE id = $1 AND session_type = $2
		`
	}

	var sessionData []byte
	var expiresAt time.Time

	err := s.db.QueryRow(ctx, query, sessionID, sessionType).Scan(&sessionData, &expiresAt)
	if err != nil {
		if err == db.ErrNoRows {
			return nil, fmt.Errorf("session not found")
		}
		return nil, err
	}

	// Check expiration
	if time.Now().After(expiresAt) {
		return nil, fmt.Errorf("session expired")
	}

	// Deserialize session data
	var session webauthn.SessionData
	if err := json.Unmarshal(sessionData, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// deleteWebAuthnSession deletes a WebAuthn session
func (s *WebAuthnService) deleteWebAuthnSession(ctx context.Context, sessionID string) {
	query := "DELETE FROM webauthn_sessions WHERE id = ?"
	if s.db.Type() == "postgres" {
		query = "DELETE FROM webauthn_sessions WHERE id = $1"
	}

	_, err := s.db.Exec(ctx, query, sessionID)
	if err != nil {
		s.logger.WithError(err).Debug("Failed to delete WebAuthn session")
	}
}

// generateCredentialID generates a unique ID for a credential
func generateCredentialID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// generateWebAuthnSessionID generates a unique session ID for WebAuthn
func generateWebAuthnSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// extractDomain extracts the domain from a URL
func extractDomain(baseURL string) string {
	// Simple domain extraction - remove protocol and path
	domain := baseURL

	// Remove protocol
	if idx := strings.Index(domain, "://"); idx != -1 {
		domain = domain[idx+3:]
	}

	// Remove path
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}

	// Remove port
	if idx := strings.Index(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}

	return domain
}
