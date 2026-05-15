package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// TOTPService handles Two-Factor Authentication (TOTP)
type TOTPService struct {
	store *store.Store
}

// NewTOTPService creates a new TOTP service
func NewTOTPService(st *store.Store) *TOTPService {
	return &TOTPService{
		store: st,
	}
}

// GenerateTOTPSecret generates a new TOTP secret per PART 23
// Returns base32-encoded secret (e.g., "JBSWY3DPEHPK3PXP")
func (s *TOTPService) GenerateTOTPSecret() (string, error) {
	// Generate 20 random bytes (160 bits) for TOTP secret
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("failed to generate TOTP secret: %w", err)
	}
	
	// Encode as base32 (standard for TOTP)
	encoded := base32.StdEncoding.EncodeToString(secret)
	
	// Remove padding (= characters)
	encoded = strings.TrimRight(encoded, "=")
	
	return encoded, nil
}

// GenerateRecoveryKeys generates 10 recovery keys per PART 23 line 20024
// Format: {8-hex-chars}-{4-hex-chars} (e.g., "a1b2c3d4-e5f6")
func (s *TOTPService) GenerateRecoveryKeys() ([]string, error) {
	keys := make([]string, 10)
	
	for i := 0; i < 10; i++ {
		// Generate 6 random bytes (8 hex chars + 4 hex chars)
		randomBytes := make([]byte, 6)
		if _, err := rand.Read(randomBytes); err != nil {
			return nil, fmt.Errorf("failed to generate recovery key: %w", err)
		}
		
		// Convert to hex
		hexStr := hex.EncodeToString(randomBytes)
		
		// Format as {8-hex}-{4-hex}
		key := fmt.Sprintf("%s-%s", hexStr[:8], hexStr[8:])
		keys[i] = key
	}
	
	return keys, nil
}

// HashRecoveryKey hashes a recovery key with Argon2id per PART 23 line 20027.
// Returns an empty string if hashing fails; callers must treat that as an
// error and refuse to persist the recovery key.
func (s *TOTPService) HashRecoveryKey(key string) (string, error) {
	return hashPasswordArgon2id(key)
}

// VerifyRecoveryKey verifies a recovery key against its hash
func (s *TOTPService) VerifyRecoveryKey(key, hash string) bool {
	// Normalize: case-insensitive per PART 23 line 20030
	key = strings.ToLower(strings.TrimSpace(key))
	
	return verifyPasswordArgon2id(key, hash)
}

// GenerateQRCodeURL generates a TOTP QR code URL for authenticator apps
// Per PART 23: Format is otpauth://totp/{issuer}:{account}?secret={secret}&issuer={issuer}
func (s *TOTPService) GenerateQRCodeURL(secret, issuer, accountName string) string {
	// URL format: otpauth://totp/Issuer:account@example.com?secret=SECRET&issuer=Issuer
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s",
		issuer,
		accountName,
		secret,
		issuer,
	)
}

// VerifyTOTPCode verifies a 6-digit TOTP code per RFC 6238
// Allows ±1 time step window (90 seconds total) to account for clock drift
func (s *TOTPService) VerifyTOTPCode(secret, code string) bool {
	// Validate input
	if len(code) != 6 {
		return false
	}
	
	// Decode base32 secret
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secretBytes, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return false
	}
	
	// Get current time step (30 seconds per step per RFC 6238)
	currentStep := time.Now().Unix() / 30
	
	// Try current time step and ±1 window (to handle clock drift)
	for offset := -1; offset <= 1; offset++ {
		timeStep := currentStep + int64(offset)
		if s.generateTOTPCode(secretBytes, timeStep) == code {
			return true
		}
	}
	
	return false
}

// generateTOTPCode generates a 6-digit TOTP code for a given time step per RFC 6238
func (s *TOTPService) generateTOTPCode(secret []byte, timeStep int64) string {
	// Convert time step to 8-byte big-endian
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(timeStep))
	
	// HMAC-SHA1 per RFC 6238
	h := hmac.New(sha1.New, secret)
	h.Write(buf)
	hash := h.Sum(nil)
	
	// Dynamic truncation per RFC 4226
	offset := hash[len(hash)-1] & 0x0F
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7FFFFFFF
	
	// Generate 6-digit code
	code := truncated % 1000000
	
	// Format with leading zeros
	return fmt.Sprintf("%06d", code)
}

// EnableTOTP enables TOTP for a user and generates recovery keys
func (s *TOTPService) EnableTOTP(userID int64, secret string) ([]string, error) {
	// Generate recovery keys per PART 23 line 20024
	recoveryKeys, err := s.GenerateRecoveryKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to generate recovery keys: %w", err)
	}
	
	// Hash each recovery key before storing per PART 23 line 20027
	hashedKeys := make([]string, len(recoveryKeys))
	for i, key := range recoveryKeys {
		hashed, err := s.HashRecoveryKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to hash recovery key: %w", err)
		}
		hashedKeys[i] = hashed
	}
	
	// Note: Secret stored in plaintext for now.
	// Future enhancement: Encrypt with AES-256-GCM using server-generated key per PART 23 line 18723
	
	// Convert hashed keys to JSON for storage
	// Format: ["hash1", "hash2", ...]
	backupCodesJSON := fmt.Sprintf("[\"%s\"]", strings.Join(hashedKeys, "\",\""))
	
	// Store TOTP secret and recovery keys in database per PART 23 line 7010-7021
	query := `INSERT INTO totp_secrets (user_type, user_id, secret, enabled, backup_codes, created_at)
	          VALUES ('user', ?, ?, 1, ?, strftime('%s', 'now'))
	          ON CONFLICT(user_type, user_id) DO UPDATE SET
	          secret = excluded.secret,
	          enabled = 1,
	          backup_codes = excluded.backup_codes,
	          created_at = strftime('%s', 'now')`
	
	_, err = s.store.UsersDB.Exec(query, userID, secret, backupCodesJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to store TOTP secret: %w", err)
	}
	
	// Return plaintext recovery keys to show user ONCE per PART 23 line 20026
	return recoveryKeys, nil
}

// DisableTOTP disables TOTP for a user
func (s *TOTPService) DisableTOTP(userID int64) error {
	// Delete TOTP secret and recovery keys from database per PART 23
	query := `DELETE FROM totp_secrets WHERE user_type = 'user' AND user_id = ?`
	
	_, err := s.store.UsersDB.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete TOTP secret: %w", err)
	}
	
	// Note: Email notification handled by caller (handler layer has access to EmailService)
	
	return nil
}

// UseRecoveryKey marks a recovery key as used (single-use per PART 23 line 20029)
func (s *TOTPService) UseRecoveryKey(userID int64, key string) error {
	// Normalize key: case-insensitive per PART 23 line 20030
	key = strings.ToLower(strings.TrimSpace(key))
	
	// Get TOTP record with backup codes
	var backupCodesJSON string
	query := `SELECT backup_codes FROM totp_secrets WHERE user_type = 'user' AND user_id = ? AND enabled = 1`
	
	err := s.store.UsersDB.QueryRow(query, userID).Scan(&backupCodesJSON)
	if err != nil {
		return fmt.Errorf("no active 2FA found for user")
	}
	
	// Parse backup codes JSON
	// Format: ["hash1", "hash2", ...]
	// Simple parsing: strip brackets and quotes, split by comma
	backupCodesJSON = strings.Trim(backupCodesJSON, "[]")
	hashes := strings.Split(backupCodesJSON, "\",\"")
	for i := range hashes {
		hashes[i] = strings.Trim(hashes[i], "\"")
	}
	
	// Find matching hash
	matchedIndex := -1
	for i, hash := range hashes {
		if s.VerifyRecoveryKey(key, hash) {
			matchedIndex = i
			break
		}
	}
	
	if matchedIndex == -1 {
		return fmt.Errorf("invalid or already used recovery key")
	}
	
	// Remove used key from array (single-use per PART 23 line 20029)
	hashes = append(hashes[:matchedIndex], hashes[matchedIndex+1:]...)
	
	// Rebuild JSON
	newBackupCodesJSON := fmt.Sprintf("[\"%s\"]", strings.Join(hashes, "\",\""))
	if len(hashes) == 0 {
		newBackupCodesJSON = "[]"
	}
	
	// Update database
	updateQuery := `UPDATE totp_secrets SET backup_codes = ? WHERE user_type = 'user' AND user_id = ?`
	_, err = s.store.UsersDB.Exec(updateQuery, newBackupCodesJSON, userID)
	if err != nil {
		return fmt.Errorf("failed to update backup codes: %w", err)
	}
	
	return nil
}

// GetRemainingRecoveryKeyCount returns count of unused recovery keys
func (s *TOTPService) GetRemainingRecoveryKeyCount(userID int64) (int, error) {
	var backupCodesJSON string
	query := `SELECT backup_codes FROM totp_secrets WHERE user_type = 'user' AND user_id = ? AND enabled = 1`
	
	err := s.store.UsersDB.QueryRow(query, userID).Scan(&backupCodesJSON)
	if err != nil {
		return 0, fmt.Errorf("no active 2FA found")
	}
	
	// Count keys in JSON array
	// Simple count: remove brackets, count comma-separated items
	backupCodesJSON = strings.Trim(backupCodesJSON, "[]")
	if backupCodesJSON == "" {
		return 0, nil
	}
	
	hashes := strings.Split(backupCodesJSON, "\",\"")
	return len(hashes), nil
}

// GetTOTPSecret retrieves the TOTP secret for a user
func (s *TOTPService) GetTOTPSecret(userID int64) (string, error) {
	var secret string
	var enabled int
	query := `SELECT secret, enabled FROM totp_secrets WHERE user_type = 'user' AND user_id = ?`
	
	err := s.store.UsersDB.QueryRow(query, userID).Scan(&secret, &enabled)
	if err != nil {
		return "", fmt.Errorf("no TOTP secret found")
	}
	
	if enabled != 1 {
		return "", fmt.Errorf("TOTP not enabled")
	}
	
	return secret, nil
}

// HasTOTP checks if a user has TOTP enabled
func (s *TOTPService) HasTOTP(userID int64) bool {
	var enabled int
	query := `SELECT enabled FROM totp_secrets WHERE user_type = 'user' AND user_id = ?`
	
	err := s.store.UsersDB.QueryRow(query, userID).Scan(&enabled)
	if err != nil {
		return false
	}
	
	return enabled == 1
}
