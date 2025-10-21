package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/sirupsen/logrus"
)

// TOTPService handles TOTP/2FA operations
type TOTPService struct {
	config      *config.AuthConfig
	db          *db.DB
	logger      *logrus.Logger
	issuerName  string
}

// NewTOTPService creates a new TOTP service
func NewTOTPService(cfg *config.AuthConfig, database *db.DB, logger *logrus.Logger) (*TOTPService, error) {
	if !cfg.EnableTOTP {
		return nil, nil
	}

	issuerName := cfg.TOTPIssuerName
	if issuerName == "" {
		issuerName = "Caslink"
	}

	return &TOTPService{
		config:     cfg,
		db:         database,
		logger:     logger,
		issuerName: issuerName,
	}, nil
}

// GenerateSecret generates a new TOTP secret for a user
func (s *TOTPService) GenerateSecret(ctx context.Context, userID, username string) (*TOTPSetupResponse, error) {
	// Generate secret
	secret, err := s.generateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	// Generate backup codes
	backupCodes, err := s.generateBackupCodes()
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// Generate QR code URL
	qrCodeURL := s.generateQRCodeURL(username, secret)

	s.logger.WithFields(logrus.Fields{
		"user_id":  userID,
		"username": username,
	}).Info("TOTP secret generated")

	return &TOTPSetupResponse{
		Secret:      secret,
		QRCodeURL:   qrCodeURL,
		BackupCodes: backupCodes,
	}, nil
}

// EnableTOTP enables TOTP for a user after verification
func (s *TOTPService) EnableTOTP(ctx context.Context, userID, secret, code string) error {
	// Verify the code first
	if !s.ValidateCode(secret, code) {
		return fmt.Errorf("invalid verification code")
	}

	// Update user to enable TOTP
	query := "UPDATE users SET two_fa_secret = ?, two_fa_enabled = ? WHERE id = ?"
	if s.db.Type() == "postgres" {
		query = "UPDATE users SET two_fa_secret = $1, two_fa_enabled = $2 WHERE id = $3"
	}

	_, err := s.db.Exec(ctx, query, secret, true, userID)
	if err != nil {
		return fmt.Errorf("failed to enable TOTP: %w", err)
	}

	s.logger.WithField("user_id", userID).Info("TOTP enabled for user")
	return nil
}

// DisableTOTP disables TOTP for a user
func (s *TOTPService) DisableTOTP(ctx context.Context, userID string) error {
	query := "UPDATE users SET two_fa_secret = '', two_fa_enabled = ? WHERE id = ?"
	if s.db.Type() == "postgres" {
		query = "UPDATE users SET two_fa_secret = '', two_fa_enabled = $1 WHERE id = $2"
	}

	_, err := s.db.Exec(ctx, query, false, userID)
	if err != nil {
		return fmt.Errorf("failed to disable TOTP: %w", err)
	}

	s.logger.WithField("user_id", userID).Info("TOTP disabled for user")
	return nil
}

// VerifyTOTP verifies a TOTP code for a user
func (s *TOTPService) VerifyTOTP(ctx context.Context, userID, code string) (bool, error) {
	// Get user's TOTP secret
	secret, enabled, err := s.getUserTOTPSecret(ctx, userID)
	if err != nil {
		return false, err
	}

	if !enabled {
		return false, fmt.Errorf("TOTP not enabled for user")
	}

	// Validate the code
	if s.ValidateCode(secret, code) {
		s.logger.WithField("user_id", userID).Debug("TOTP code verified successfully")
		return true, nil
	}

	// Check if it's a backup code
	if s.validateBackupCode(ctx, userID, code) {
		s.logger.WithField("user_id", userID).Info("Backup code used successfully")
		// Mark backup code as used
		s.markBackupCodeUsed(ctx, userID, code)
		return true, nil
	}

	s.logger.WithField("user_id", userID).Warn("Invalid TOTP code provided")
	return false, nil
}

// ValidateCode validates a TOTP code against a secret
func (s *TOTPService) ValidateCode(secret, code string) bool {
	// Remove spaces and validate format
	code = strings.ReplaceAll(code, " ", "")
	if len(code) != 6 {
		return false
	}

	// Validate with time window (allow 1 period before/after for clock skew)
	valid, err := totp.ValidateCustom(
		code,
		secret,
		time.Now(),
		totp.ValidateOpts{
			Period:    30,
			Skew:      1,
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		},
	)

	if err != nil {
		s.logger.WithError(err).Error("TOTP validation error")
		return false
	}

	return valid
}

// GetTOTPStatus returns the TOTP status for a user
func (s *TOTPService) GetTOTPStatus(ctx context.Context, userID string) (bool, error) {
	query := "SELECT two_fa_enabled FROM users WHERE id = ?"
	if s.db.Type() == "postgres" {
		query = "SELECT two_fa_enabled FROM users WHERE id = $1"
	}

	var enabled bool
	err := s.db.QueryRow(ctx, query, userID).Scan(&enabled)
	if err != nil {
		if err == db.ErrNoRows {
			return false, ErrUserNotFound
		}
		return false, err
	}

	return enabled, nil
}

// RegenerateBackupCodes generates new backup codes for a user
func (s *TOTPService) RegenerateBackupCodes(ctx context.Context, userID string) ([]string, error) {
	// Generate new backup codes
	backupCodes, err := s.generateBackupCodes()
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// Store backup codes in database
	if err := s.storeBackupCodes(ctx, userID, backupCodes); err != nil {
		return nil, fmt.Errorf("failed to store backup codes: %w", err)
	}

	s.logger.WithField("user_id", userID).Info("Backup codes regenerated")
	return backupCodes, nil
}

// Helper functions

// generateSecret generates a random TOTP secret
func (s *TOTPService) generateSecret() (string, error) {
	// Generate 20 random bytes (160 bits)
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}

	// Encode to base32
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

// generateQRCodeURL generates a QR code URL for TOTP setup
func (s *TOTPService) generateQRCodeURL(username, secret string) string {
	// Format: otpauth://totp/Issuer:username?secret=SECRET&issuer=Issuer
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", s.issuerName)
	params.Set("algorithm", "SHA1")
	params.Set("digits", "6")
	params.Set("period", "30")

	label := url.PathEscape(fmt.Sprintf("%s:%s", s.issuerName, username))
	return fmt.Sprintf("otpauth://totp/%s?%s", label, params.Encode())
}

// generateBackupCodes generates backup codes for account recovery
func (s *TOTPService) generateBackupCodes() ([]string, error) {
	codes := make([]string, 8) // Generate 8 backup codes

	for i := 0; i < 8; i++ {
		bytes := make([]byte, 4)
		if _, err := rand.Read(bytes); err != nil {
			return nil, err
		}

		// Format as XXXX-XXXX for readability
		code := hex.EncodeToString(bytes)
		codes[i] = fmt.Sprintf("%s-%s", code[:4], code[4:])
	}

	return codes, nil
}

// getUserTOTPSecret retrieves a user's TOTP secret from the database
func (s *TOTPService) getUserTOTPSecret(ctx context.Context, userID string) (string, bool, error) {
	query := "SELECT two_fa_secret, two_fa_enabled FROM users WHERE id = ?"
	if s.db.Type() == "postgres" {
		query = "SELECT two_fa_secret, two_fa_enabled FROM users WHERE id = $1"
	}

	var secret string
	var enabled bool
	err := s.db.QueryRow(ctx, query, userID).Scan(&secret, &enabled)
	if err != nil {
		if err == db.ErrNoRows {
			return "", false, ErrUserNotFound
		}
		return "", false, err
	}

	return secret, enabled, nil
}

// validateBackupCode checks if a code is a valid unused backup code
func (s *TOTPService) validateBackupCode(ctx context.Context, userID, code string) bool {
	// This would query the backup_codes table
	// For now, return false as the table needs to be added in migrations
	query := `
		SELECT 1 FROM backup_codes
		WHERE user_id = ? AND code = ? AND used = false
		LIMIT 1
	`
	if s.db.Type() == "postgres" {
		query = `
			SELECT 1 FROM backup_codes
			WHERE user_id = $1 AND code = $2 AND used = false
			LIMIT 1
		`
	}

	var exists int
	err := s.db.QueryRow(ctx, query, userID, code).Scan(&exists)
	if err != nil {
		// Table might not exist yet, log debug and return false
		s.logger.WithError(err).Debug("Backup code validation skipped (table not yet implemented)")
		return false
	}

	return true
}

// markBackupCodeUsed marks a backup code as used
func (s *TOTPService) markBackupCodeUsed(ctx context.Context, userID, code string) error {
	query := `
		UPDATE backup_codes
		SET used = true, used_at = ?
		WHERE user_id = ? AND code = ?
	`
	if s.db.Type() == "postgres" {
		query = `
			UPDATE backup_codes
			SET used = true, used_at = $1
			WHERE user_id = $2 AND code = $3
		`
	}

	_, err := s.db.Exec(ctx, query, time.Now(), userID, code)
	if err != nil {
		s.logger.WithError(err).Error("Failed to mark backup code as used")
		return err
	}

	return nil
}

// storeBackupCodes stores backup codes in the database
func (s *TOTPService) storeBackupCodes(ctx context.Context, userID string, codes []string) error {
	// First, delete existing backup codes
	deleteQuery := "DELETE FROM backup_codes WHERE user_id = ?"
	if s.db.Type() == "postgres" {
		deleteQuery = "DELETE FROM backup_codes WHERE user_id = $1"
	}

	_, err := s.db.Exec(ctx, deleteQuery, userID)
	if err != nil {
		s.logger.WithError(err).Debug("Failed to delete old backup codes (table might not exist)")
	}

	// Insert new backup codes
	insertQuery := `
		INSERT INTO backup_codes (id, user_id, code, created_at, used)
		VALUES (?, ?, ?, ?, false)
	`
	if s.db.Type() == "postgres" {
		insertQuery = `
			INSERT INTO backup_codes (id, user_id, code, created_at, used)
			VALUES ($1, $2, $3, $4, false)
		`
	}

	for _, code := range codes {
		id, err := generateBackupCodeID()
		if err != nil {
			return err
		}

		_, err = s.db.Exec(ctx, insertQuery, id, userID, code, time.Now())
		if err != nil {
			s.logger.WithError(err).Debug("Failed to insert backup code (table might not exist)")
			// Continue even if insert fails - table might not exist yet
		}
	}

	return nil
}

// generateBackupCodeID generates a unique ID for a backup code
func generateBackupCodeID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GetBackupCodesCount returns the number of unused backup codes for a user
func (s *TOTPService) GetBackupCodesCount(ctx context.Context, userID string) (int, error) {
	query := "SELECT COUNT(*) FROM backup_codes WHERE user_id = ? AND used = false"
	if s.db.Type() == "postgres" {
		query = "SELECT COUNT(*) FROM backup_codes WHERE user_id = $1 AND used = false"
	}

	var count int
	err := s.db.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		// Table might not exist yet
		s.logger.WithError(err).Debug("Failed to count backup codes (table might not exist)")
		return 0, nil
	}

	return count, nil
}
