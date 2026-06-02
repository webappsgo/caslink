package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

const (
	recoveryKeyDBTimeout    = 30 * time.Second
	recoveryKeyCount        = 10
	webauthnSessionTTL      = 5 * time.Minute
	webauthnSessionCookieAge = int(webauthnSessionTTL / time.Second)
)

// webauthnSessionEntry holds in-memory WebAuthn ceremony state between
// the begin and finish calls. Sessions expire after webauthnSessionTTL.
type webauthnSessionEntry struct {
	SessionData *webauthn.SessionData
	UserID      string
	Expiry      time.Time
}

// PasskeyCredential is a stored WebAuthn credential record returned to callers.
type PasskeyCredential struct {
	ID             string
	UserID         string
	CredentialID   string
	Name           string
	AAGUID         string
	SignCount       uint32
	UserVerified   bool
	BackupEligible bool
	BackupState    bool
	CreatedAt      time.Time
	LastUsed       *time.Time
}

// webAuthnUser is the internal adapter that satisfies webauthn.User for a given
// user's ID, display name, and current credential set.
type webAuthnUser struct {
	id          []byte
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte                         { return u.id }
func (u *webAuthnUser) WebAuthnName() string                       { return u.name }
func (u *webAuthnUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// WebAuthnService wraps go-webauthn to provide registration, login, credential
// management, and recovery-key operations against the project's UsersDB.
type WebAuthnService struct {
	store    *store.Store
	wauth    *webauthn.WebAuthn
	rpid     string
	origin   string
	sessMu   sync.Mutex
	sessions map[string]*webauthnSessionEntry
}

// NewWebAuthnService creates and validates a new WebAuthnService.
// rpid must be a bare domain (e.g. "example.com"); origin must be the fully
// qualified origin (e.g. "https://example.com").
func NewWebAuthnService(st *store.Store, rpid, origin string) (*WebAuthnService, error) {
	wauth, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Caslink URL Shortener",
		RPID:          rpid,
		RPOrigins:     []string{origin},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create WebAuthn instance: %w", err)
	}

	return &WebAuthnService{
		store:    st,
		wauth:    wauth,
		rpid:     rpid,
		origin:   origin,
		sessions: make(map[string]*webauthnSessionEntry),
	}, nil
}

// StoreSession persists WebAuthn ceremony session data in memory and returns
// the session ID that must be sent to the client as a cookie.
func (s *WebAuthnService) StoreSession(userID string, data *webauthn.SessionData) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	sessID := hex.EncodeToString(raw)

	s.sessMu.Lock()
	s.pruneExpiredSessions()
	s.sessions[sessID] = &webauthnSessionEntry{
		SessionData: data,
		UserID:      userID,
		Expiry:      time.Now().Add(webauthnSessionTTL),
	}
	s.sessMu.Unlock()

	return sessID, nil
}

// LoadSession retrieves and removes the WebAuthn ceremony session keyed by
// sessID. Returns (nil, "", nil) when the session does not exist or has expired.
func (s *WebAuthnService) LoadSession(sessID string) (*webauthn.SessionData, string, error) {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()

	entry, ok := s.sessions[sessID]
	if !ok {
		return nil, "", nil
	}
	delete(s.sessions, sessID)

	if time.Now().After(entry.Expiry) {
		return nil, "", nil
	}

	return entry.SessionData, entry.UserID, nil
}

// pruneExpiredSessions removes stale entries. Caller must hold sessMu.
func (s *WebAuthnService) pruneExpiredSessions() {
	now := time.Now()
	for id, e := range s.sessions {
		if now.After(e.Expiry) {
			delete(s.sessions, id)
		}
	}
}

// WebAuthnSessionCookieAge is the Max-Age (seconds) to use for the ceremony cookie.
const WebAuthnSessionCookieAge = webauthnSessionCookieAge

// BeginRegistration starts a WebAuthn credential-creation ceremony and returns
// the options that must be delivered to the browser along with session data the
// caller must persist (e.g. in the HTTP session) until FinishRegistration.
func (s *WebAuthnService) BeginRegistration(userID, username, displayName string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	credentials, err := s.loadCredentials(userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load credentials for registration: %w", err)
	}

	user := &webAuthnUser{
		id:          []byte(userID),
		name:        username,
		displayName: displayName,
		credentials: credentials,
	}

	// Exclude already-registered credentials so the same authenticator cannot
	// be registered twice for the same account.
	var exclusions []protocol.CredentialDescriptor
	for _, c := range credentials {
		exclusions = append(exclusions, c.Descriptor())
	}

	options, sessionData, err := s.wauth.BeginRegistration(user, webauthn.WithExclusions(exclusions))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin WebAuthn registration: %w", err)
	}

	return options, sessionData, nil
}

// FinishRegistration validates the authenticator response and persists the new
// credential in the database using credentialName as the human-readable label.
func (s *WebAuthnService) FinishRegistration(userID, credentialName string, sessionData *webauthn.SessionData, r *http.Request) error {
	credentials, err := s.loadCredentials(userID)
	if err != nil {
		return fmt.Errorf("failed to load credentials for finish registration: %w", err)
	}

	user := &webAuthnUser{
		id:          []byte(userID),
		credentials: credentials,
	}

	credential, err := s.wauth.FinishRegistration(user, *sessionData, r)
	if err != nil {
		return fmt.Errorf("WebAuthn registration failed: %w", err)
	}

	return s.saveCredential(userID, credentialName, credential)
}

// BeginLogin starts an assertion ceremony for a specific user and returns the
// options to send to the browser and session data to persist until FinishLogin.
func (s *WebAuthnService) BeginLogin(userID, username string) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	credentials, err := s.loadCredentials(userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load credentials for login: %w", err)
	}

	if len(credentials) == 0 {
		return nil, nil, fmt.Errorf("no passkey credentials registered for user")
	}

	user := &webAuthnUser{
		id:          []byte(userID),
		name:        username,
		displayName: username,
		credentials: credentials,
	}

	options, sessionData, err := s.wauth.BeginLogin(user)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin WebAuthn login: %w", err)
	}

	return options, sessionData, nil
}

// FinishLogin validates the assertion response and updates the credential's
// sign counter and flags in the database.
func (s *WebAuthnService) FinishLogin(userID string, sessionData *webauthn.SessionData, r *http.Request) error {
	credentials, err := s.loadCredentials(userID)
	if err != nil {
		return fmt.Errorf("failed to load credentials for finish login: %w", err)
	}

	user := &webAuthnUser{
		id:          []byte(userID),
		credentials: credentials,
	}

	updatedCredential, err := s.wauth.FinishLogin(user, *sessionData, r)
	if err != nil {
		return fmt.Errorf("WebAuthn login failed: %w", err)
	}

	return s.updateCredentialAfterLogin(userID, updatedCredential)
}

// GetCredentials returns all passkey credentials stored for userID.
func (s *WebAuthnService) GetCredentials(userID string) ([]PasskeyCredential, error) {
	ctx, cancel := context.WithTimeout(context.Background(), recoveryKeyDBTimeout)
	defer cancel()

	query := `SELECT id, user_id, credential_id, name, aaguid, sign_count,
	                 user_verified, backup_eligible, backup_state, created_at, last_used
	          FROM passkey_credentials
	          WHERE user_id = ?
	          ORDER BY created_at DESC`

	rows, err := s.store.UsersDB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query passkey credentials: %w", err)
	}
	defer rows.Close()

	var result []PasskeyCredential
	for rows.Next() {
		var pc PasskeyCredential
		var userVerified, backupEligible, backupState int
		var lastUsed sql.NullTime

		err := rows.Scan(
			&pc.ID, &pc.UserID, &pc.CredentialID, &pc.Name, &pc.AAGUID,
			&pc.SignCount, &userVerified, &backupEligible, &backupState,
			&pc.CreatedAt, &lastUsed,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan passkey credential: %w", err)
		}

		pc.UserVerified = userVerified != 0
		pc.BackupEligible = backupEligible != 0
		pc.BackupState = backupState != 0
		if lastUsed.Valid {
			pc.LastUsed = &lastUsed.Time
		}

		result = append(result, pc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating passkey credentials: %w", err)
	}

	return result, nil
}

// DeleteCredential removes a passkey credential belonging to userID by its record ID.
func (s *WebAuthnService) DeleteCredential(userID, credentialID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), recoveryKeyDBTimeout)
	defer cancel()

	query := `DELETE FROM passkey_credentials WHERE user_id = ? AND id = ?`
	result, err := s.store.UsersDB.ExecContext(ctx, query, userID, credentialID)
	if err != nil {
		return fmt.Errorf("failed to delete passkey credential: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm credential deletion: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("credential not found or not owned by user")
	}

	return nil
}

// GenerateRecoveryKeys generates exactly 10 single-use recovery keys for userID,
// replaces any previously stored unused keys, and returns the plaintext keys
// (which must be shown to the user exactly once and never stored in plaintext).
//
// Format: {8-hex-chars}-{4-hex-chars}, e.g. "a1b2c3d4-e5f6".
// Stored as SHA-256 hex hashes; validated case-insensitively.
func (s *WebAuthnService) GenerateRecoveryKeys(userID string) ([]string, error) {
	plainKeys := make([]string, recoveryKeyCount)
	for i := range plainKeys {
		key, err := generateRecoveryKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate recovery key %d: %w", i+1, err)
		}
		plainKeys[i] = key
	}

	ctx, cancel := context.WithTimeout(context.Background(), recoveryKeyDBTimeout)
	defer cancel()

	tx, err := s.store.UsersDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction for recovery keys: %w", err)
	}
	defer tx.Rollback()

	// Delete all existing unused recovery keys for this user.
	_, err = tx.ExecContext(ctx, `DELETE FROM recovery_keys WHERE user_id = ? AND used = 0`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to clear existing recovery keys: %w", err)
	}

	for _, key := range plainKeys {
		hash := hashRecoveryKey(key)
		id := uuid.New().String()

		_, err = tx.ExecContext(ctx,
			`INSERT INTO recovery_keys (id, user_id, key_hash, used, created_at) VALUES (?, ?, ?, 0, CURRENT_TIMESTAMP)`,
			id, userID, hash,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert recovery key: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit recovery keys: %w", err)
	}

	return plainKeys, nil
}

// ValidateRecoveryKey checks whether key is a valid unused recovery key for
// userID, marks it used on success, and returns the remaining unused key count.
// Returns (false, 0, nil) when the key is simply invalid or already used.
func (s *WebAuthnService) ValidateRecoveryKey(userID, key string) (bool, error) {
	// Normalise to lowercase for case-insensitive comparison (per PART 34).
	key = strings.ToLower(strings.TrimSpace(key))
	hash := hashRecoveryKey(key)

	ctx, cancel := context.WithTimeout(context.Background(), recoveryKeyDBTimeout)
	defer cancel()

	tx, err := s.store.UsersDB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var recordID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM recovery_keys WHERE user_id = ? AND key_hash = ? AND used = 0`,
		userID, hash,
	).Scan(&recordID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to query recovery key: %w", err)
	}

	// Mark the key used (single-use per PART 34).
	_, err = tx.ExecContext(ctx,
		`UPDATE recovery_keys SET used = 1, used_at = CURRENT_TIMESTAMP WHERE id = ?`,
		recordID,
	)
	if err != nil {
		return false, fmt.Errorf("failed to mark recovery key as used: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit recovery key usage: %w", err)
	}

	return true, nil
}

// GetRecoveryKeyCount returns the number of unused recovery keys remaining for userID.
func (s *WebAuthnService) GetRecoveryKeyCount(userID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), recoveryKeyDBTimeout)
	defer cancel()

	var count int
	err := s.store.UsersDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recovery_keys WHERE user_id = ? AND used = 0`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count recovery keys: %w", err)
	}

	return count, nil
}

// loadCredentials fetches all persisted webauthn.Credential values for userID
// by deserialising the JSON blob stored in the public_key column.
func (s *WebAuthnService) loadCredentials(userID string) ([]webauthn.Credential, error) {
	ctx, cancel := context.WithTimeout(context.Background(), recoveryKeyDBTimeout)
	defer cancel()

	rows, err := s.store.UsersDB.QueryContext(ctx,
		`SELECT public_key FROM passkey_credentials WHERE user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query credentials: %w", err)
	}
	defer rows.Close()

	var credentials []webauthn.Credential
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("failed to scan credential blob: %w", err)
		}

		var cred webauthn.Credential
		if err := json.Unmarshal(blob, &cred); err != nil {
			return nil, fmt.Errorf("failed to deserialise credential: %w", err)
		}

		credentials = append(credentials, cred)
	}

	return credentials, rows.Err()
}

// saveCredential persists a newly-registered webauthn.Credential to the database.
func (s *WebAuthnService) saveCredential(userID, credentialName string, cred *webauthn.Credential) error {
	blob, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("failed to serialise credential: %w", err)
	}

	aaguid := base64.StdEncoding.EncodeToString(cred.Authenticator.AAGUID)

	userVerified := 0
	if cred.Flags.UserVerified {
		userVerified = 1
	}

	backupEligible := 0
	if cred.Flags.BackupEligible {
		backupEligible = 1
	}

	backupState := 0
	if cred.Flags.BackupState {
		backupState = 1
	}

	id := uuid.New().String()
	credentialID := base64.URLEncoding.EncodeToString(cred.ID)

	ctx, cancel := context.WithTimeout(context.Background(), recoveryKeyDBTimeout)
	defer cancel()

	_, err = s.store.UsersDB.ExecContext(ctx,
		`INSERT INTO passkey_credentials
		 (id, user_id, credential_id, public_key, attestation_type, aaguid,
		  sign_count, user_verified, backup_eligible, backup_state, name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		id, userID, credentialID, blob,
		cred.AttestationType, aaguid,
		cred.Authenticator.SignCount, userVerified, backupEligible, backupState,
		credentialName,
	)
	if err != nil {
		return fmt.Errorf("failed to save passkey credential: %w", err)
	}

	return nil
}

// updateCredentialAfterLogin updates the mutable credential fields (sign count,
// backup state, last-used timestamp) that may change on every successful assertion.
func (s *WebAuthnService) updateCredentialAfterLogin(userID string, cred *webauthn.Credential) error {
	blob, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("failed to serialise updated credential: %w", err)
	}

	backupState := 0
	if cred.Flags.BackupState {
		backupState = 1
	}

	credentialID := base64.URLEncoding.EncodeToString(cred.ID)

	ctx, cancel := context.WithTimeout(context.Background(), recoveryKeyDBTimeout)
	defer cancel()

	_, err = s.store.UsersDB.ExecContext(ctx,
		`UPDATE passkey_credentials
		 SET public_key = ?, sign_count = ?, backup_state = ?, last_used = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND credential_id = ?`,
		blob, cred.Authenticator.SignCount, backupState, userID, credentialID,
	)
	if err != nil {
		return fmt.Errorf("failed to update credential after login: %w", err)
	}

	return nil
}

// generateRecoveryKey returns one cryptographically random recovery key in the
// format "{8-hex-chars}-{4-hex-chars}" (e.g. "a1b2c3d4-e5f6").
func generateRecoveryKey() (string, error) {
	// 6 random bytes → 12 hex chars, split as 8+4.
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand failed: %w", err)
	}

	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s", h[:8], h[8:]), nil
}

// hashRecoveryKey returns the lowercase hex SHA-256 of key (also lowercased
// first so comparison is always case-insensitive).
func hashRecoveryKey(key string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(key)))
	return hex.EncodeToString(sum[:])
}
