package service

import (
	"context"
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

	// Insert domain
	query := `INSERT INTO custom_domains (
		owner_type, owner_id, domain, is_apex, is_wildcard,
		verification_status, ssl_status, status, created_at, updated_at
	) VALUES (?, ?, ?, ?, 0, 'pending', 'none', 'pending', ?, ?)`

	result, err := s.store.UsersDB.ExecContext(ctx, query,
		ownerType, ownerID, domain, isApex, time.Now(), time.Now())
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

// VerifyDomain verifies that a custom domain resolves to this server's public
// IP. On success the domain is marked verified and activated. On DNS failure
// an error is returned describing the mismatch.
func (s *DomainService) VerifyDomain(ctx context.Context, domainID int64) error {
	domain, err := s.getDomainByID(ctx, domainID)
	if err != nil {
		return err
	}

	// Discover what IPs this server is reachable on.
	srvIPs := serverPublicIPs(ctx)

	// Resolve the custom domain.
	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, domain.Domain)
	if err != nil {
		// Update check metadata even on failure.
		now := time.Now()
		_, _ = s.store.UsersDB.ExecContext(ctx,
			`UPDATE custom_domains
			 SET verification_status = 'failed',
			     last_check_at = ?,
			     check_count = check_count + 1,
			     updated_at = ?
			 WHERE id = ?`,
			now, now, domainID,
		)
		return fmt.Errorf("DNS_LOOKUP_FAILED: could not resolve %s: %w", domain.Domain, err)
	}

	// Check if any resolved IP matches a server IP.
	var resolvedStrs []string
	matched := false
	var matchedIP string
	for _, addr := range resolved {
		resolvedStrs = append(resolvedStrs, addr.IP.String())
		for _, srv := range srvIPs {
			if addr.IP.Equal(srv) {
				matched = true
				matchedIP = addr.IP.String()
				break
			}
		}
		if matched {
			break
		}
	}

	now := time.Now()
	if !matched {
		_, _ = s.store.UsersDB.ExecContext(ctx,
			`UPDATE custom_domains
			 SET verification_status = 'failed',
			     last_check_at = ?,
			     check_count = check_count + 1,
			     updated_at = ?
			 WHERE id = ?`,
			now, now, domainID,
		)
		return fmt.Errorf("DNS_MISMATCH: %s resolves to %v, not this server",
			domain.Domain, resolvedStrs)
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
		     verified_ip = ?,
		     last_check_at = ?,
		     check_count = check_count + 1,
		     status = 'active',
		     ssl_enabled = ?,
		     ssl_status = ?,
		     updated_at = ?
		 WHERE id = ?`,
		now, matchedIP, now, sslEnabled, sslStatus, now, domainID,
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
