package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/store"
)

// DomainService handles custom domain operations
type DomainService struct {
	store *store.Store
	cfg   config.CustomDomainsConfig
}

// NewDomainService creates a new domain service
func NewDomainService(st *store.Store, cfg config.CustomDomainsConfig) *DomainService {
	return &DomainService{
		store: st,
		cfg:   cfg,
	}
}

// isReservedDomain reports whether domain matches any configured reserved
// entry. An entry beginning with "*." matches the bare suffix ("*.local" →
// "local") and any domain ending in that suffix ("host.local"); every other
// entry is an exact (case-insensitive) match.
func (s *DomainService) isReservedDomain(domain string) bool {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	for _, entry := range s.cfg.Reserved {
		e := strings.ToLower(strings.TrimSpace(entry))
		if e == "" {
			continue
		}
		if strings.HasPrefix(e, "*.") {
			suffix := e[1:] // ".local"
			if d == e[2:] || strings.HasSuffix(d, suffix) {
				return true
			}
			continue
		}
		if d == e {
			return true
		}
	}
	return false
}

// matchesBlockedPattern reports whether domain matches any configured
// blocked_patterns regex. Invalid patterns are skipped (fail-open on the
// individual pattern, never on the whole check).
func (s *DomainService) matchesBlockedPattern(domain string) bool {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	for _, pat := range s.cfg.BlockedPatterns {
		p := strings.TrimSpace(pat)
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		if re.MatchString(d) {
			return true
		}
	}
	return false
}

// domainLimitFor returns the configured max domains for the given owner type
// (0 = unlimited).
func (s *DomainService) domainLimitFor(ownerType string) int {
	if ownerType == "org" {
		return s.cfg.MaxDomainsPerOrg
	}
	return s.cfg.MaxDomainsPerUser
}

// CustomDomain represents a custom domain
type CustomDomain struct {
	ID                 int64
	OwnerType          string
	OwnerID            int64
	Domain             string
	IsApex             bool
	IsWildcard         bool
	VerificationStatus string
	VerificationToken  string
	VerifiedAt         *time.Time
	VerifiedIP         *string
	LastCheckAt        *time.Time
	CheckCount         int
	SSLEnabled         bool
	SSLStatus          string
	SSLExpiresAt       *time.Time
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// generateVerificationToken returns a random ownership-proof token of the form
// "verify_" + 24 hex chars, published by the owner at _verify.{domain} as a TXT
// record (PART 36 Verification Flow).
func generateVerificationToken() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate verification token: %w", err)
	}
	return "verify_" + hex.EncodeToString(b), nil
}

// DNSRecord describes a single DNS record the owner must configure.
type DNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DNSRouting describes the routing options (CNAME/A/AAAA) that direct traffic
// to this server. Routing is not the ownership proof (see PART 36).
type DNSRouting struct {
	Target       string   `json:"target"`
	TargetIPs    []string `json:"target_ips"`
	Instructions string   `json:"instructions"`
}

// DNSInstructions is the full set of DNS records returned to the owner after a
// domain is added: the required ownership TXT record plus routing options.
type DNSInstructions struct {
	Verification DNSRecord  `json:"verification"`
	Routing      DNSRouting `json:"routing"`
}

// BuildDNSInstructions assembles the DNS setup instructions for a domain: the
// ownership verification TXT record (_verify.{domain} = token) and the routing
// targets (this server's public IPs). Routing IP discovery failing is
// non-fatal — the verification record is always returned.
func (s *DomainService) BuildDNSInstructions(ctx context.Context, cd *CustomDomain) DNSInstructions {
	var ipStrs []string
	for _, ip := range serverPublicIPs(ctx) {
		ipStrs = append(ipStrs, ip.String())
	}
	return DNSInstructions{
		Verification: DNSRecord{
			Type:  "TXT",
			Name:  "_verify." + cd.Domain,
			Value: cd.VerificationToken,
		},
		Routing: DNSRouting{
			TargetIPs:    ipStrs,
			Instructions: "Point this domain at the server: add A/AAAA records for the IPs above (required for apex domains), or a CNAME to the server's hostname (subdomains).",
		},
	}
}

// AddDomain adds a new custom domain for a user or organization
func (s *DomainService) AddDomain(ctx context.Context, ownerType string, ownerID int64, domain string) (*CustomDomain, error) {
	// Reject reserved domains and blocked TLD patterns before any DB work
	// (PART 36). These are policy rejections independent of ownership.
	if s.isReservedDomain(domain) {
		return nil, model.ErrDomainReserved
	}
	if s.matchesBlockedPattern(domain) {
		return nil, model.ErrDomainBlockedPattern
	}

	// Check if domain already exists
	var count int
	err := s.store.UsersDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM custom_domains WHERE domain = ?", domain).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check domain: %w", err)
	}
	if count > 0 {
		return nil, model.ErrDomainAlreadyExists
	}

	// Enforce per-owner domain limit (0 = unlimited, PART 36).
	if limit := s.domainLimitFor(ownerType); limit > 0 {
		var owned int
		err := s.store.UsersDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM custom_domains WHERE owner_type = ? AND owner_id = ?",
			ownerType, ownerID).Scan(&owned)
		if err != nil {
			return nil, fmt.Errorf("failed to count owner domains: %w", err)
		}
		if owned >= limit {
			return nil, model.ErrDomainLimitReached
		}
	}

	// Determine if apex or subdomain. An apex (registrable root) domain has
	// exactly one dot, e.g. "example.com" (2 labels); anything with more dots,
	// e.g. "www.example.com", is a subdomain. This is a heuristic — it
	// misclassifies second-level-TLD apexes like "example.co.uk" (2 dots) as
	// a subdomain — but is materially more correct than the prior check
	// (`!strings.Contains(domain, ".")`), which only matched single-label
	// strings like "localhost" and treated every real domain as non-apex.
	isApex := strings.Count(domain, ".") == 1

	// Generate the ownership-proof token published at _verify.{domain}.
	token, err := generateVerificationToken()
	if err != nil {
		return nil, err
	}

	// Insert domain
	query := `INSERT INTO custom_domains (
		owner_type, owner_id, domain, is_apex, is_wildcard,
		verification_status, verification_token, ssl_status, status, created_at, updated_at
	) VALUES (?, ?, ?, ?, 0, 'pending', ?, 'none', 'pending', ?, ?)`

	result, err := s.store.UsersDB.ExecContext(ctx, query,
		ownerType, ownerID, domain, isApex, token, time.Now(), time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to add domain: %w", err)
	}

	domainID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get domain ID: %w", err)
	}

	// Return created domain
	cd := &CustomDomain{
		ID:                 domainID,
		OwnerType:          ownerType,
		OwnerID:            ownerID,
		Domain:             domain,
		IsApex:             isApex,
		IsWildcard:         false,
		VerificationStatus: "pending",
		VerificationToken:  token,
		SSLEnabled:         false,
		SSLStatus:          "none",
		Status:             "pending",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	return cd, nil
}

// GetUserDomains gets all domains for a user
func (s *DomainService) GetUserDomains(ctx context.Context, userID int64) ([]*CustomDomain, error) {
	query := `SELECT id, owner_type, owner_id, domain, is_apex, is_wildcard,
	          verification_status, verified_at, ssl_enabled, ssl_status, ssl_expires_at,
	          status, created_at, updated_at
	          FROM custom_domains
	          WHERE owner_type = 'user' AND owner_id = ?
	          ORDER BY created_at DESC`

	rows, err := s.store.UsersDB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query domains: %w", err)
	}
	defer rows.Close()

	var domains []*CustomDomain
	for rows.Next() {
		var d CustomDomain
		err := rows.Scan(
			&d.ID, &d.OwnerType, &d.OwnerID, &d.Domain, &d.IsApex, &d.IsWildcard,
			&d.VerificationStatus, &d.VerifiedAt, &d.SSLEnabled, &d.SSLStatus, &d.SSLExpiresAt,
			&d.Status, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan domain: %w", err)
		}
		domains = append(domains, &d)
	}

	return domains, nil
}

// GetOrgDomains gets all domains for an organization
func (s *DomainService) GetOrgDomains(ctx context.Context, orgID int64) ([]*CustomDomain, error) {
	query := `SELECT id, owner_type, owner_id, domain, is_apex, is_wildcard,
	          verification_status, verified_at, ssl_enabled, ssl_status, ssl_expires_at,
	          status, created_at, updated_at
	          FROM custom_domains
	          WHERE owner_type = 'org' AND owner_id = ?
	          ORDER BY created_at DESC`

	rows, err := s.store.UsersDB.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query domains: %w", err)
	}
	defer rows.Close()

	var domains []*CustomDomain
	for rows.Next() {
		var d CustomDomain
		err := rows.Scan(
			&d.ID, &d.OwnerType, &d.OwnerID, &d.Domain, &d.IsApex, &d.IsWildcard,
			&d.VerificationStatus, &d.VerifiedAt, &d.SSLEnabled, &d.SSLStatus, &d.SSLExpiresAt,
			&d.Status, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan domain: %w", err)
		}
		domains = append(domains, &d)
	}

	return domains, nil
}

// getDomainByID retrieves a custom domain record by its primary key.
func (s *DomainService) getDomainByID(ctx context.Context, domainID int64) (*CustomDomain, error) {
	query := `SELECT id, owner_type, owner_id, domain, is_apex, is_wildcard,
	          verification_status, verified_at, ssl_enabled, ssl_status, ssl_expires_at,
	          status, created_at, updated_at
	          FROM custom_domains WHERE id = ?`

	var d CustomDomain
	err := s.store.UsersDB.QueryRowContext(ctx, query, domainID).Scan(
		&d.ID, &d.OwnerType, &d.OwnerID, &d.Domain, &d.IsApex, &d.IsWildcard,
		&d.VerificationStatus, &d.VerifiedAt, &d.SSLEnabled, &d.SSLStatus, &d.SSLExpiresAt,
		&d.Status, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("domain not found: %w", err)
	}
	return &d, nil
}

// discoverPublicIPv4 fetches the server's outbound IPv4 address from an
// external service. It tries multiple providers in order and returns the
// first usable address. Returns nil when all attempts fail.
func discoverPublicIPv4(ctx context.Context) net.IP {
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
		"https://checkip.amazonaws.com",
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for _, url := range services {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		_ = resp.Body.Close()
		if err != nil {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(string(body)))
		if ip != nil && ip.To4() != nil {
			return ip
		}
	}
	return nil
}

// serverPublicIPs returns all public IP addresses the server is reachable on.
// It discovers the outbound IPv4 via external HTTP and collects global unicast
// IPv6 addresses from local interfaces.
func serverPublicIPs(ctx context.Context) []net.IP {
	var ips []net.IP

	if v4 := discoverPublicIPv4(ctx); v4 != nil {
		ips = append(ips, v4)
	}

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip == nil || ip.To4() != nil {
					continue // only IPv6
				}
				if ip.IsGlobalUnicast() && !ip.IsPrivate() {
					ips = append(ips, ip)
				}
			}
		}
	}

	return ips
}

// VerifyDomain proves domain ownership via a DNS TXT record. The owner must
// publish the domain's verification_token at _verify.{domain}; this method
// looks it up and constant-time compares against the stored token. TXT
// verification proves control of the domain and works behind CDNs/proxies
// (PART 36 Verification Flow). Routing records (A/AAAA/CNAME) are NOT the
// ownership proof and are never used here. On success the domain is marked
// verified and activated; on any failure the check metadata is updated and a
// coded error is returned.
func (s *DomainService) VerifyDomain(ctx context.Context, domainID int64) error {
	domain, err := s.getDomainByID(ctx, domainID)
	if err != nil {
		return err
	}

	// Load the stored ownership token for this domain.
	var token string
	if err := s.store.UsersDB.QueryRowContext(ctx,
		"SELECT verification_token FROM custom_domains WHERE id = ?", domainID,
	).Scan(&token); err != nil {
		return fmt.Errorf("failed to load verification token: %w", err)
	}

	now := time.Now()
	markFailed := func() {
		_, _ = s.store.UsersDB.ExecContext(ctx,
			`UPDATE custom_domains
			 SET verification_status = 'failed',
			     last_check_at = ?,
			     check_count = check_count + 1,
			     updated_at = ?
			 WHERE id = ?`,
			now, now, domainID,
		)
	}

	// Look up the ownership TXT record at _verify.{domain}.
	record := "_verify." + domain.Domain
	values, err := net.DefaultResolver.LookupTXT(ctx, record)
	if err != nil {
		markFailed()
		return fmt.Errorf("DNS_LOOKUP_FAILED: TXT lookup for %s failed: %w", record, err)
	}

	// Constant-time compare each TXT value against the verification token.
	matched := false
	for _, v := range values {
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(v)), []byte(token)) == 1 {
			matched = true
			break
		}
	}

	if !matched {
		markFailed()
		return fmt.Errorf("TXT_RECORD_MISSING: TXT record %s not found or does not match the verification token. DNS propagation can take up to 48 hours", record)
	}

	// Mark domain as verified and active. Non-wildcard domains become
	// eligible for automatic Let's Encrypt issuance the moment they go
	// active (see IsDomainVerifiedActive / server.go's autocert HostPolicy),
	// so flip ssl_status from "none" to "pending" here rather than leaving it
	// permanently "none" — the cert itself is obtained lazily on first HTTPS
	// handshake by autocert.Manager, not by this method. Wildcard domains
	// require DNS-01, which is not implemented (TODO.AI.md PART 36), so their
	// ssl_status is left untouched.
	sslStatus := domain.SSLStatus
	sslEnabled := domain.SSLEnabled
	if !domain.IsWildcard && sslStatus == "none" {
		sslStatus = "pending"
		sslEnabled = true
	}

	_, err = s.store.UsersDB.ExecContext(ctx,
		`UPDATE custom_domains
		 SET verification_status = 'verified',
		     verified_at = ?,
		     last_check_at = ?,
		     check_count = check_count + 1,
		     status = 'active',
		     ssl_enabled = ?,
		     ssl_status = ?,
		     updated_at = ?
		 WHERE id = ?`,
		now, now, sslEnabled, sslStatus, now, domainID,
	)
	if err != nil {
		return fmt.Errorf("failed to update domain: %w", err)
	}

	return nil
}

// IsDomainVerifiedActive reports whether host is a verified, active,
// non-wildcard custom domain — the set of domains autocert.Manager's dynamic
// HostPolicy (server.go) permits automatic ACME issuance for. Wildcard
// domains are excluded: HTTP-01 and TLS-ALPN-01 cannot issue wildcard certs
// (RFC 8555 requires DNS-01), and DNS-01 is not implemented.
func (s *DomainService) IsDomainVerifiedActive(ctx context.Context, host string) (bool, error) {
	var count int
	err := s.store.UsersDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM custom_domains
		 WHERE domain = ? AND verification_status = 'verified' AND status = 'active' AND is_wildcard = 0`,
		host,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check domain SSL eligibility: %w", err)
	}
	return count > 0, nil
}
