package domains

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// VerificationService handles domain ownership verification
type VerificationService struct {
	db     *db.DB
	config *config.Config
	logger *logrus.Logger
	client *http.Client
}

// VerificationRecord represents a domain verification record
type VerificationRecord struct {
	ID         string    `json:"id" db:"id"`
	DomainID   string    `json:"domain_id" db:"domain_id"`
	Domain     string    `json:"domain" db:"domain"`
	Method     string    `json:"method" db:"method"`
	Token      string    `json:"token" db:"token"`
	Challenge  string    `json:"challenge" db:"challenge"`
	Status     string    `json:"status" db:"status"`
	Error      string    `json:"error,omitempty" db:"error"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	VerifiedAt *time.Time `json:"verified_at,omitempty" db:"verified_at"`
	ExpiresAt  time.Time `json:"expires_at" db:"expires_at"`
	LastCheck  time.Time `json:"last_check" db:"last_check"`
	NextCheck  time.Time `json:"next_check" db:"next_check"`
}

// DNSVerificationInfo contains DNS verification instructions
type DNSVerificationInfo struct {
	RecordType string `json:"record_type"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	TTL        int    `json:"ttl"`
}

// FileVerificationInfo contains file verification instructions
type FileVerificationInfo struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
	URL      string `json:"url"`
}

// EmailVerificationInfo contains email verification instructions
type EmailVerificationInfo struct {
	AdminEmails []string `json:"admin_emails"`
	Subject     string   `json:"subject"`
	TokenURL    string   `json:"token_url"`
}

// NewVerificationService creates a new verification service
func NewVerificationService(database *db.DB, cfg *config.Config, logger *logrus.Logger) (*VerificationService, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}

	return &VerificationService{
		db:     database,
		config: cfg,
		logger: logger,
		client: client,
	}, nil
}

// VerifyDomain initiates or checks domain verification
func (v *VerificationService) VerifyDomain(ctx context.Context, domain *Domain) (*VerificationStatus, error) {
	// Get or create verification record
	record, err := v.getOrCreateVerificationRecord(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to get verification record: %w", err)
	}

	// Check if verification has expired
	if time.Now().After(record.ExpiresAt) {
		// Generate new token and reset verification
		newToken, err := v.generateVerificationToken()
		if err != nil {
			return nil, fmt.Errorf("failed to generate new token: %w", err)
		}

		record.Token = newToken
		record.Challenge = v.generateChallenge(domain.Domain, newToken, domain.VerificationMethod)
		record.Status = "pending"
		record.Error = ""
		record.ExpiresAt = time.Now().Add(7 * 24 * time.Hour)
		record.NextCheck = time.Now().Add(5 * time.Minute)

		if err := v.updateVerificationRecord(ctx, record); err != nil {
			return nil, fmt.Errorf("failed to update verification record: %w", err)
		}
	}

	// Perform verification check based on method
	status := &VerificationStatus{
		Domain:    domain.Domain,
		Verified:  domain.Verified,
		Method:    domain.VerificationMethod,
		Token:     record.Token,
		LastCheck: record.LastCheck,
		NextCheck: record.NextCheck,
	}

	// Add verification instructions
	instructions, err := v.getVerificationInstructions(domain, record)
	if err != nil {
		v.logger.WithError(err).Warn("Failed to get verification instructions")
	} else {
		status.Instructions = instructions
	}

	// Only check verification if not already verified and it's time to check
	if !domain.Verified && time.Now().After(record.NextCheck) {
		verified, checkErr := v.performVerificationCheck(ctx, domain, record)
		if checkErr != nil {
			status.Error = checkErr.Error()
			record.Error = checkErr.Error()
		} else if verified {
			// Mark domain as verified
			if err := v.markDomainVerified(ctx, domain.ID); err != nil {
				v.logger.WithError(err).Error("Failed to mark domain as verified")
			} else {
				status.Verified = true
				record.Status = "verified"
				record.VerifiedAt = &[]time.Time{time.Now()}[0]
			}
		}

		// Update next check time
		record.LastCheck = time.Now()
		if verified {
			record.NextCheck = time.Now().Add(24 * time.Hour)
		} else {
			record.NextCheck = time.Now().Add(v.calculateNextCheckInterval(record))
		}

		if err := v.updateVerificationRecord(ctx, record); err != nil {
			v.logger.WithError(err).Error("Failed to update verification record")
		}
	}

	return status, nil
}

// performVerificationCheck performs the actual verification check
func (v *VerificationService) performVerificationCheck(ctx context.Context, domain *Domain, record *VerificationRecord) (bool, error) {
	switch domain.VerificationMethod {
	case "dns":
		return v.verifyDNS(ctx, domain, record)
	case "file":
		return v.verifyFile(ctx, domain, record)
	case "email":
		return v.verifyEmail(ctx, domain, record)
	default:
		return false, fmt.Errorf("unsupported verification method: %s", domain.VerificationMethod)
	}
}

// verifyDNS verifies domain ownership via DNS TXT record
func (v *VerificationService) verifyDNS(ctx context.Context, domain *Domain, record *VerificationRecord) (bool, error) {
	txtRecords, err := net.LookupTXT("_caslink." + domain.Domain)
	if err != nil {
		return false, fmt.Errorf("DNS lookup failed: %w", err)
	}

	expectedValue := record.Challenge
	for _, txt := range txtRecords {
		if strings.TrimSpace(txt) == expectedValue {
			v.logger.WithFields(logrus.Fields{
				"domain": domain.Domain,
				"method": "dns",
			}).Info("Domain verification successful")
			return true, nil
		}
	}

	return false, fmt.Errorf("verification token not found in DNS TXT records")
}

// verifyFile verifies domain ownership via file upload
func (v *VerificationService) verifyFile(ctx context.Context, domain *Domain, record *VerificationRecord) (bool, error) {
	verificationURL := fmt.Sprintf("http://%s/.well-known/caslink-verification.txt", domain.Domain)

	req, err := http.NewRequestWithContext(ctx, "GET", verificationURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		// Try HTTPS if HTTP fails
		httpsURL := fmt.Sprintf("https://%s/.well-known/caslink-verification.txt", domain.Domain)
		req, err = http.NewRequestWithContext(ctx, "GET", httpsURL, nil)
		if err != nil {
			return false, fmt.Errorf("failed to create HTTPS request: %w", err)
		}

		resp, err = v.client.Do(req)
		if err != nil {
			return false, fmt.Errorf("HTTP verification failed: %w", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("verification file not found (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read verification file: %w", err)
	}

	content := strings.TrimSpace(string(body))
	if content == record.Challenge {
		v.logger.WithFields(logrus.Fields{
			"domain": domain.Domain,
			"method": "file",
		}).Info("Domain verification successful")
		return true, nil
	}

	return false, fmt.Errorf("verification file content does not match expected token")
}

// verifyEmail verifies domain ownership via email confirmation
func (v *VerificationService) verifyEmail(ctx context.Context, domain *Domain, record *VerificationRecord) (bool, error) {
	// Email verification is handled separately via email confirmation links
	// This method checks if the verification was completed via email
	return record.Status == "verified", nil
}

// ConfirmEmailVerification confirms email verification via token
func (v *VerificationService) ConfirmEmailVerification(ctx context.Context, domain string, token string) error {
	query := `
		UPDATE domain_verification_records
		SET status = 'verified', verified_at = ?, error = ''
		WHERE domain = ? AND token = ? AND method = 'email' AND expires_at > ?`

	now := time.Now()
	result, err := v.db.ExecContext(ctx, query, now, domain, token, now)
	if err != nil {
		return fmt.Errorf("failed to confirm email verification: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("invalid or expired verification token")
	}

	// Get domain ID and mark domain as verified
	var domainID string
	err = v.db.QueryRowContext(ctx, "SELECT id FROM domains WHERE domain = ?", domain).Scan(&domainID)
	if err != nil {
		return fmt.Errorf("failed to get domain ID: %w", err)
	}

	if err := v.markDomainVerified(ctx, domainID); err != nil {
		return fmt.Errorf("failed to mark domain as verified: %w", err)
	}

	v.logger.WithFields(logrus.Fields{
		"domain": domain,
		"method": "email",
	}).Info("Domain verification successful")

	return nil
}

// getVerificationInstructions returns verification instructions based on method
func (v *VerificationService) getVerificationInstructions(domain *Domain, record *VerificationRecord) (map[string]interface{}, error) {
	switch domain.VerificationMethod {
	case "dns":
		return map[string]interface{}{
			"dns": DNSVerificationInfo{
				RecordType: "TXT",
				Name:       "_caslink." + domain.Domain,
				Value:      record.Challenge,
				TTL:        300,
			},
		}, nil

	case "file":
		return map[string]interface{}{
			"file": FileVerificationInfo{
				FilePath: "/.well-known/caslink-verification.txt",
				Content:  record.Challenge,
				URL:      fmt.Sprintf("http://%s/.well-known/caslink-verification.txt", domain.Domain),
			},
		}, nil

	case "email":
		adminEmails := []string{
			"admin@" + domain.Domain,
			"administrator@" + domain.Domain,
			"webmaster@" + domain.Domain,
			"hostmaster@" + domain.Domain,
			"postmaster@" + domain.Domain,
		}
		tokenURL := fmt.Sprintf("%s/verify-domain?domain=%s&token=%s", v.config.Server.BaseURL, domain.Domain, record.Token)

		return map[string]interface{}{
			"email": EmailVerificationInfo{
				AdminEmails: adminEmails,
				Subject:     "Domain Verification for " + domain.Domain,
				TokenURL:    tokenURL,
			},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported verification method: %s", domain.VerificationMethod)
	}
}

// getOrCreateVerificationRecord gets or creates a verification record
func (v *VerificationService) getOrCreateVerificationRecord(ctx context.Context, domain *Domain) (*VerificationRecord, error) {
	// Try to get existing record
	query := `
		SELECT id, domain_id, domain, method, token, challenge, status, error,
		       created_at, verified_at, expires_at, last_check, next_check
		FROM domain_verification_records
		WHERE domain_id = ? AND method = ?`

	row := v.db.QueryRowContext(ctx, query, domain.ID, domain.VerificationMethod)

	record := &VerificationRecord{}
	err := row.Scan(
		&record.ID, &record.DomainID, &record.Domain, &record.Method,
		&record.Token, &record.Challenge, &record.Status, &record.Error,
		&record.CreatedAt, &record.VerifiedAt, &record.ExpiresAt,
		&record.LastCheck, &record.NextCheck,
	)

	if err == nil {
		return record, nil
	}

	// Create new record
	record = &VerificationRecord{
		ID:        v.generateRecordID(),
		DomainID:  domain.ID,
		Domain:    domain.Domain,
		Method:    domain.VerificationMethod,
		Token:     domain.VerificationToken,
		Status:    "pending",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		LastCheck: time.Time{},
		NextCheck: time.Now().Add(5 * time.Minute),
	}

	record.Challenge = v.generateChallenge(domain.Domain, record.Token, domain.VerificationMethod)

	insertQuery := `
		INSERT INTO domain_verification_records
		(id, domain_id, domain, method, token, challenge, status, error,
		 created_at, expires_at, last_check, next_check)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = v.db.ExecContext(ctx, insertQuery,
		record.ID, record.DomainID, record.Domain, record.Method,
		record.Token, record.Challenge, record.Status, record.Error,
		record.CreatedAt, record.ExpiresAt, record.LastCheck, record.NextCheck,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create verification record: %w", err)
	}

	return record, nil
}

// updateVerificationRecord updates a verification record
func (v *VerificationService) updateVerificationRecord(ctx context.Context, record *VerificationRecord) error {
	query := `
		UPDATE domain_verification_records
		SET token = ?, challenge = ?, status = ?, error = ?,
		    verified_at = ?, expires_at = ?, last_check = ?, next_check = ?
		WHERE id = ?`

	_, err := v.db.ExecContext(ctx, query,
		record.Token, record.Challenge, record.Status, record.Error,
		record.VerifiedAt, record.ExpiresAt, record.LastCheck, record.NextCheck,
		record.ID,
	)

	return err
}

// markDomainVerified marks a domain as verified
func (v *VerificationService) markDomainVerified(ctx context.Context, domainID string) error {
	now := time.Now()
	_, err := v.db.ExecContext(ctx, "UPDATE domains SET verified = true, verified_at = ? WHERE id = ?", now, domainID)
	return err
}

// generateChallenge generates a verification challenge based on method
func (v *VerificationService) generateChallenge(domain, token, method string) string {
	switch method {
	case "dns":
		return fmt.Sprintf("caslink-verification=%s", token)
	case "file":
		return token
	case "email":
		return token
	default:
		return token
	}
}

// calculateNextCheckInterval calculates the next check interval with exponential backoff
func (v *VerificationService) calculateNextCheckInterval(record *VerificationRecord) time.Duration {
	baseInterval := 5 * time.Minute

	// Calculate how many checks have been performed
	elapsed := time.Since(record.CreatedAt)
	checks := int(elapsed / baseInterval)

	// Exponential backoff with max of 1 hour
	interval := baseInterval * time.Duration(1<<min(checks, 6))
	if interval > time.Hour {
		interval = time.Hour
	}

	return interval
}

// generateVerificationToken generates a random verification token
func (v *VerificationService) generateVerificationToken() (string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// generateRecordID generates a unique record ID
func (v *VerificationService) generateRecordID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}