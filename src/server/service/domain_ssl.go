package service

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	appcrypto "github.com/webappsgo/caslink/src/common/crypto"
	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/service/acmedns"
)

// sslLastErrorMax bounds how much of an issuance failure is persisted to
// ssl_last_error. ACME/DNS errors never contain the provider credentials (those
// live in the provider object, not the returned error), but the cap keeps a
// pathological error string from bloating the row.
const sslLastErrorMax = 500

// EnableDNS01SSL wires the DNS-01 issuance capability onto the service. issuer
// performs the ACME order (a mock in tests, ACMEIssuer in production);
// encryptionKey is the decoded 32-byte server.security.encryption_key used to
// encrypt stored DNS credentials and the issued private key; email/staging
// configure the ACME account; onCertChange (optional) purges any cached TLS
// certificate for a domain after a (re)issue. Calling it with a nil issuer or a
// wrong-length key is a no-op guard that leaves SSL disabled.
func (s *DomainService) EnableDNS01SSL(issuer acmedns.Issuer, encryptionKey []byte, keyVersion int, email string, staging bool, onCertChange func(host string)) {
	if issuer == nil || len(encryptionKey) != appcrypto.KeySize {
		return
	}
	s.sslIssuer = issuer
	s.encryptionKey = encryptionKey
	s.encKeyVersion = keyVersion
	s.leEmail = email
	s.leStaging = staging
	s.onCertChange = onCertChange
}

// sslConfigured reports whether DNS-01 issuance has been wired up.
func (s *DomainService) sslConfigured() bool {
	return s.sslIssuer != nil && len(s.encryptionKey) == appcrypto.KeySize
}

// getOwnedDomainByName loads a domain scoped to a single owner, including its
// SSL provider/credential columns. Cross-owner (or unknown) names return
// ErrDomainNotFound so one owner can never touch another owner's SSL config.
func (s *DomainService) getOwnedDomainByName(ctx context.Context, ownerType string, ownerID int64, domain string) (*CustomDomain, error) {
	name := normalizeResolveHost(domain)
	if name == "" {
		return nil, model.ErrDomainNotFound
	}
	var d CustomDomain
	var provider, credentials, certPEM, keyPEM, lastErr sql.NullString
	var challenge sql.NullString
	var issuedAt sql.NullTime
	err := s.store.UsersDB.QueryRowContext(ctx,
		`SELECT id, owner_type, owner_id, domain, is_apex, is_wildcard,
		        verification_status, verified_at, ssl_enabled, ssl_status, ssl_expires_at,
		        ssl_provider, ssl_challenge, ssl_credentials, ssl_cert_pem, ssl_key_pem,
		        ssl_issued_at, ssl_last_error, status, created_at, updated_at
		 FROM custom_domains WHERE domain = ? AND owner_type = ? AND owner_id = ?`,
		name, ownerType, ownerID).Scan(
		&d.ID, &d.OwnerType, &d.OwnerID, &d.Domain, &d.IsApex, &d.IsWildcard,
		&d.VerificationStatus, &d.VerifiedAt, &d.SSLEnabled, &d.SSLStatus, &d.SSLExpiresAt,
		&provider, &challenge, &credentials, &certPEM, &keyPEM,
		&issuedAt, &lastErr, &d.Status, &d.CreatedAt, &d.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get domain: %w", err)
	}
	d.SSLProvider = provider.String
	d.SSLChallenge = challenge.String
	d.SSLCredentials = credentials.String
	d.SSLCertPEM = certPEM.String
	d.SSLKeyPEM = keyPEM.String
	d.SSLLastError = lastErr.String
	if issuedAt.Valid {
		t := issuedAt.Time
		d.SSLIssuedAt = &t
	}
	return &d, nil
}

// SetDNSProvider validates and stores an encrypted DNS provider configuration
// for a domain's DNS-01 SSL challenge (PART 36). The provider must be a
// registered acmedns provider and its factory must accept the supplied
// credentials (this is the credential validation step); credentials are then
// JSON-encoded and AES-256-GCM encrypted before storage — never persisted in
// plaintext. Returns model.ErrSSLProviderInvalid / ErrSSLCredentialsInvalid for
// clean 400 mapping, and ErrDomainNotFound if the domain is not owned by caller.
func (s *DomainService) SetDNSProvider(ctx context.Context, ownerType string, ownerID int64, domain, provider string, creds map[string]string) error {
	if !s.sslConfigured() {
		return model.ErrSSLNotConfigured
	}
	cd, err := s.getOwnedDomainByName(ctx, ownerType, ownerID, domain)
	if err != nil {
		return err
	}

	provider = strings.ToLower(strings.TrimSpace(provider))
	// Validate provider name and credentials by constructing the provider; the
	// factory rejects an unknown provider (ErrProviderUnsupported) and a missing
	// required credential.
	if _, err := acmedns.NewProvider(provider, creds); err != nil {
		if errors.Is(err, acmedns.ErrProviderUnsupported) {
			return model.ErrSSLProviderInvalid
		}
		return model.ErrSSLCredentialsInvalid
	}

	credJSON, err := json.Marshal(creds)
	if err != nil {
		return model.ErrSSLCredentialsInvalid
	}
	encCreds, err := appcrypto.EncryptGCM(s.encryptionKey, credJSON)
	if err != nil {
		return fmt.Errorf("encrypt dns credentials: %w", err)
	}

	if _, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE custom_domains
		 SET ssl_provider = ?, ssl_credentials = ?, ssl_challenge = 'dns-01', updated_at = ?
		 WHERE id = ? AND owner_type = ? AND owner_id = ?`,
		provider, encCreds, time.Now(), cd.ID, ownerType, ownerID); err != nil {
		return fmt.Errorf("store dns provider: %w", err)
	}
	return nil
}

// IssueDNS01Cert performs a full DNS-01 certificate issuance (or force-renewal)
// for an owned, verified, active domain (PART 36). It decrypts the stored DNS
// credentials, builds the provider, runs the ACME order via the configured
// Issuer, then encrypts and persists the issued chain and private key and marks
// the domain SSL-active. Any failure is recorded in ssl_last_error and mapped to
// model.ErrSSLChallengeFailed / ErrSSLIssuanceFailed. On success the resolve
// cache is invalidated and the onCertChange hook (if set) purges any cached TLS
// certificate so the new cert is served immediately.
func (s *DomainService) IssueDNS01Cert(ctx context.Context, ownerType string, ownerID int64, domain string) (*CustomDomain, error) {
	if !s.sslConfigured() {
		return nil, model.ErrSSLNotConfigured
	}
	cd, err := s.getOwnedDomainByName(ctx, ownerType, ownerID, domain)
	if err != nil {
		return nil, err
	}
	if cd.VerificationStatus != "verified" || cd.Status != "active" {
		return nil, model.ErrDomainNotVerified
	}
	if cd.SSLProvider == "" || cd.SSLCredentials == "" {
		return nil, model.ErrSSLNotConfigured
	}

	credBytes, err := appcrypto.DecryptGCM(s.encryptionKey, cd.SSLCredentials)
	if err != nil {
		return nil, s.recordSSLError(ctx, cd, model.ErrSSLCredentialsInvalid)
	}
	var creds map[string]string
	if err := json.Unmarshal(credBytes, &creds); err != nil {
		return nil, s.recordSSLError(ctx, cd, model.ErrSSLCredentialsInvalid)
	}
	provider, err := acmedns.NewProvider(cd.SSLProvider, creds)
	if err != nil {
		return nil, s.recordSSLError(ctx, cd, model.ErrSSLProviderInvalid)
	}

	result, err := s.sslIssuer.IssueDNS01(ctx, provider, sslDomainNames(cd)...)
	if err != nil {
		// Persist the raw failure detail for the owner, but return a stable coded
		// error to the caller for 400/500 mapping.
		_ = s.storeSSLLastError(ctx, cd.ID, err.Error())
		return nil, fmt.Errorf("%w: %v", model.ErrSSLIssuanceFailed, err)
	}

	encCert, err := appcrypto.EncryptGCM(s.encryptionKey, result.CertPEM)
	if err != nil {
		return nil, fmt.Errorf("encrypt certificate: %w", err)
	}
	encKey, err := appcrypto.EncryptGCM(s.encryptionKey, result.KeyPEM)
	if err != nil {
		return nil, fmt.Errorf("encrypt certificate key: %w", err)
	}

	now := time.Now()
	if _, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE custom_domains
		 SET ssl_cert_pem = ?, ssl_key_pem = ?, ssl_status = 'active', ssl_enabled = 1,
		     ssl_issued_at = ?, ssl_expires_at = ?, ssl_last_error = '', updated_at = ?
		 WHERE id = ? AND owner_type = ? AND owner_id = ?`,
		encCert, encKey, now, result.NotAfter, now, cd.ID, ownerType, ownerID); err != nil {
		return nil, fmt.Errorf("persist issued certificate: %w", err)
	}

	cd.SSLStatus = "active"
	cd.SSLEnabled = true
	cd.SSLIssuedAt = &now
	expiry := result.NotAfter
	cd.SSLExpiresAt = &expiry
	cd.SSLLastError = ""

	s.invalidateResolveCache()
	s.purgeCachedCert(cd.Domain)
	if s.onCertChange != nil {
		s.onCertChange(cd.Domain)
	}
	return cd, nil
}

// CertificateFor returns the DNS-01–issued TLS certificate for host from the
// database, decrypting and memoising it on first use. It is intended for a
// tls.Config.GetCertificate hook: a nil certificate with a nil error means "no
// app-managed cert for this host", so the caller falls through to the autocert
// manager. Only active, SSL-enabled domains resolve; expired/errored ones do
// not. host is the SNI server name (no port).
func (s *DomainService) CertificateFor(ctx context.Context, host string) (*tls.Certificate, error) {
	name := normalizeResolveHost(host)
	if name == "" {
		return nil, nil
	}

	s.certCacheMu.RLock()
	cached, ok := s.certCache[name]
	s.certCacheMu.RUnlock()
	if ok {
		return cached, nil
	}
	if !s.sslConfigured() {
		return nil, nil
	}

	var certPEM, keyPEM sql.NullString
	err := s.store.UsersDB.QueryRowContext(ctx,
		`SELECT ssl_cert_pem, ssl_key_pem FROM custom_domains
		 WHERE domain = ? AND ssl_enabled = 1 AND ssl_status = 'active'`,
		name).Scan(&certPEM, &keyPEM)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load certificate for %s: %w", name, err)
	}
	if certPEM.String == "" || keyPEM.String == "" {
		return nil, nil
	}

	certBytes, err := appcrypto.DecryptGCM(s.encryptionKey, certPEM.String)
	if err != nil {
		return nil, fmt.Errorf("decrypt certificate for %s: %w", name, err)
	}
	keyBytes, err := appcrypto.DecryptGCM(s.encryptionKey, keyPEM.String)
	if err != nil {
		return nil, fmt.Errorf("decrypt certificate key for %s: %w", name, err)
	}
	pair, err := tls.X509KeyPair(certBytes, keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate for %s: %w", name, err)
	}

	s.certCacheMu.Lock()
	s.certCache[name] = &pair
	s.certCacheMu.Unlock()
	return &pair, nil
}

// purgeCachedCert drops any memoised certificate for host so the next
// handshake re-reads the freshly issued cert from the database.
func (s *DomainService) purgeCachedCert(host string) {
	name := normalizeResolveHost(host)
	if name == "" {
		return
	}
	s.certCacheMu.Lock()
	delete(s.certCache, name)
	s.certCacheMu.Unlock()
}

// sslDomainNames returns the certificate SAN list for a domain: the domain
// itself, plus its wildcard form when the domain is flagged wildcard.
func sslDomainNames(cd *CustomDomain) []string {
	if cd.IsWildcard && !strings.HasPrefix(cd.Domain, "*.") {
		return []string{cd.Domain, "*." + cd.Domain}
	}
	return []string{cd.Domain}
}

// recordSSLError persists a coded issuance failure to ssl_last_error/ssl_status
// and returns the error for the caller to map. Used for pre-ACME validation
// failures (bad credentials/provider) where err is a stable model error.
func (s *DomainService) recordSSLError(ctx context.Context, cd *CustomDomain, codedErr error) error {
	_ = s.storeSSLLastError(ctx, cd.ID, codedErr.Error())
	if errors.Is(codedErr, model.ErrSSLProviderInvalid) || errors.Is(codedErr, model.ErrSSLCredentialsInvalid) {
		return codedErr
	}
	return fmt.Errorf("%w: %v", model.ErrSSLChallengeFailed, codedErr)
}

// storeSSLLastError writes ssl_status='error' plus a truncated failure message.
func (s *DomainService) storeSSLLastError(ctx context.Context, domainID int64, msg string) error {
	if len(msg) > sslLastErrorMax {
		msg = msg[:sslLastErrorMax]
	}
	_, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE custom_domains SET ssl_status = 'error', ssl_last_error = ?, updated_at = ? WHERE id = ?`,
		msg, time.Now(), domainID)
	return err
}
