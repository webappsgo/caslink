package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/store"
)

// ErrDomainNotFound is returned by Resolve when the request host does not map
// to a verified, active custom domain (i.e. it is the main application host or
// an unknown host). Callers treat it as "serve the main application", never a
// hard error.
var ErrDomainNotFound = errors.New("custom domain not found")

// domainResolveCacheTTL bounds how long a positive or negative host→owner
// resolution is cached. Short by design: the redirect hot path must not hit
// the DB on every request (PART 36 domain caching), but a freshly verified or
// removed domain must go live within a minute even without explicit
// invalidation.
const domainResolveCacheTTL = 60 * time.Second

// resolveCacheEntry is one cached host→domain resolution. A nil domain is a
// negative cache entry (host is not a custom domain).
type resolveCacheEntry struct {
	domain  *CustomDomain
	expires time.Time
}

// DomainService handles custom domain operations
type DomainService struct {
	store *store.Store
	cfg   config.CustomDomainsConfig

	resolveCacheMu sync.RWMutex
	resolveCache   map[string]resolveCacheEntry
}

// NewDomainService creates a new domain service
func NewDomainService(st *store.Store, cfg config.CustomDomainsConfig) *DomainService {
	return &DomainService{
		store:        st,
		cfg:          cfg,
		resolveCache: make(map[string]resolveCacheEntry),
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

	// Record the domain-lifecycle "created" event (PART 36 audit trail). The
	// owner is the actor; ownerID is loop-safe to address here. Action strings
	// follow the spec's canonical vocabulary (created/verified/ssl_issued/
	// suspended/deleted).
	actor := ownerID
	s.logDomainAudit(ctx, domainID, "created", ownerType, &actor, "")

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

// standardDomainColumns is the fixed SELECT column list scanned into a
// CustomDomain, matching the scan order in scanCustomDomain and the existing
// GetUserDomains/getDomainByID queries.
const standardDomainColumns = `id, owner_type, owner_id, domain, is_apex, is_wildcard,
	verification_status, verified_at, ssl_enabled, ssl_status, ssl_expires_at,
	status, created_at, updated_at`

// scanCustomDomain scans one row (from *sql.Row or *sql.Rows) into d using the
// standardDomainColumns ordering.
func scanCustomDomain(sc interface{ Scan(...any) error }, d *CustomDomain) error {
	return sc.Scan(
		&d.ID, &d.OwnerType, &d.OwnerID, &d.Domain, &d.IsApex, &d.IsWildcard,
		&d.VerificationStatus, &d.VerifiedAt, &d.SSLEnabled, &d.SSLStatus, &d.SSLExpiresAt,
		&d.Status, &d.CreatedAt, &d.UpdatedAt,
	)
}

// GetDomainByName looks up any custom domain by its exact (normalized) name
// across all owners — used by admin management. Returns model.ErrDomainNotFound
// when no such domain exists.
func (s *DomainService) GetDomainByName(ctx context.Context, domain string) (*CustomDomain, error) {
	name := normalizeResolveHost(domain)
	if name == "" {
		return nil, model.ErrDomainNotFound
	}
	var d CustomDomain
	err := scanCustomDomain(
		s.store.UsersDB.QueryRowContext(ctx,
			`SELECT `+standardDomainColumns+` FROM custom_domains WHERE domain = ?`, name),
		&d,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get domain: %w", err)
	}
	return &d, nil
}

// ListAllDomains returns one page of custom domains across every owner, newest
// first, plus the total row count for pagination (PART 36 admin domain list).
// page is 1-based; limit is clamped to [1,250] (PART 14 default 250).
func (s *DomainService) ListAllDomains(ctx context.Context, page, limit int) ([]*CustomDomain, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 250 {
		limit = 250
	}

	var total int
	if err := s.store.UsersDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM custom_domains`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count domains: %w", err)
	}

	offset := (page - 1) * limit
	rows, err := s.store.UsersDB.QueryContext(ctx,
		`SELECT `+standardDomainColumns+` FROM custom_domains ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list domains: %w", err)
	}
	defer rows.Close()

	var domains []*CustomDomain
	for rows.Next() {
		var d CustomDomain
		if err := scanCustomDomain(rows, &d); err != nil {
			return nil, 0, fmt.Errorf("failed to scan domain: %w", err)
		}
		domains = append(domains, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate domains: %w", err)
	}
	return domains, total, nil
}

// logDomainAudit best-effort records a domain lifecycle event to
// custom_domain_audit (PART 36). A write failure is swallowed so it never
// blocks the management action itself.
func (s *DomainService) logDomainAudit(ctx context.Context, domainID int64, action, actorType string, actorID *int64, details string) {
	_, _ = s.store.UsersDB.ExecContext(ctx,
		`INSERT INTO custom_domain_audit (domain_id, action, actor_type, actor_id, details, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		domainID, action, actorType, actorID, details, time.Now(),
	)
}

// SuspendDomain marks a domain suspended (admin action). A suspended domain no
// longer resolves (Resolve requires status='active'), and the resolve cache is
// invalidated so the change takes effect immediately. Returns
// model.ErrDomainNotFound when the domain does not exist.
func (s *DomainService) SuspendDomain(ctx context.Context, domainID int64, actorID *int64) error {
	res, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE custom_domains SET status = 'suspended', updated_at = ? WHERE id = ?`,
		time.Now(), domainID)
	if err != nil {
		return fmt.Errorf("failed to suspend domain: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.ErrDomainNotFound
	}
	s.invalidateResolveCache()
	s.logDomainAudit(ctx, domainID, "suspended", "admin", actorID, "")
	return nil
}

// UnsuspendDomain lifts a suspension. The domain returns to 'active' when its
// ownership is still verified, otherwise to 'pending' (it must re-verify before
// it can resolve again). The resolve cache is invalidated. Returns
// model.ErrDomainNotFound when the domain does not exist.
func (s *DomainService) UnsuspendDomain(ctx context.Context, domainID int64, actorID *int64) error {
	d, err := s.getDomainByID(ctx, domainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ErrDomainNotFound
		}
		return err
	}
	newStatus := "pending"
	if d.VerificationStatus == "verified" {
		newStatus = "active"
	}
	if _, err := s.store.UsersDB.ExecContext(ctx,
		`UPDATE custom_domains SET status = ?, updated_at = ? WHERE id = ?`,
		newStatus, time.Now(), domainID); err != nil {
		return fmt.Errorf("failed to unsuspend domain: %w", err)
	}
	s.invalidateResolveCache()
	s.logDomainAudit(ctx, domainID, "unsuspended", "admin", actorID, "status="+newStatus)
	return nil
}

// AdminDeleteDomain force-deletes any custom domain (admin action). The delete
// cascades to custom_domain_audit rows (schema ON DELETE CASCADE), and the
// resolve cache is invalidated. Returns model.ErrDomainNotFound when the domain
// does not exist.
func (s *DomainService) AdminDeleteDomain(ctx context.Context, domainID int64, actorID *int64) error {
	res, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM custom_domains WHERE id = ?`, domainID)
	if err != nil {
		return fmt.Errorf("failed to delete domain: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.ErrDomainNotFound
	}
	s.invalidateResolveCache()
	return nil
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

	// Record the domain-lifecycle "verified" event (PART 36 audit trail). The
	// domain owner is the actor; domain.OwnerID is loop-safe to address here.
	verifyActor := domain.OwnerID
	s.logDomainAudit(ctx, domainID, "verified", domain.OwnerType, &verifyActor, "")

	// The domain just became verified+active; drop any cached negative
	// resolution so Resolve serves it immediately (PART 36 domain caching).
	s.invalidateResolveCache()

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

// Resolve maps a request Host to the verified, active, non-wildcard custom
// domain that owns it (PART 36 domain resolver). It returns ErrDomainNotFound
// when host is the main application host or any host not registered as a live
// custom domain — callers treat that as "serve the main application", never a
// hard failure. Results (positive and negative) are cached for
// domainResolveCacheTTL so the redirect hot path avoids a DB round-trip per
// request.
func (s *DomainService) Resolve(ctx context.Context, host string) (*CustomDomain, error) {
	host = normalizeResolveHost(host)
	if host == "" {
		return nil, ErrDomainNotFound
	}

	if cd, ok := s.resolveCacheGet(host); ok {
		if cd == nil {
			return nil, ErrDomainNotFound
		}
		return cd, nil
	}

	query := `SELECT id, owner_type, owner_id, domain, is_apex, is_wildcard,
	          verification_status, verified_at, ssl_enabled, ssl_status, ssl_expires_at,
	          status, created_at, updated_at
	          FROM custom_domains
	          WHERE domain = ? AND verification_status = 'verified' AND status = 'active' AND is_wildcard = 0`

	var d CustomDomain
	err := s.store.UsersDB.QueryRowContext(ctx, query, host).Scan(
		&d.ID, &d.OwnerType, &d.OwnerID, &d.Domain, &d.IsApex, &d.IsWildcard,
		&d.VerificationStatus, &d.VerifiedAt, &d.SSLEnabled, &d.SSLStatus, &d.SSLExpiresAt,
		&d.Status, &d.CreatedAt, &d.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		s.resolveCacheSet(host, nil)
		return nil, ErrDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve custom domain: %w", err)
	}

	s.resolveCacheSet(host, &d)
	return &d, nil
}

// normalizeResolveHost strips any port, a trailing dot, and case from a request
// Host so it matches the stored (already lowercased, FQDN) domain value.
func normalizeResolveHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

// resolveCacheGet returns the cached resolution for host. The bool is false on
// a miss or an expired entry; on a hit the *CustomDomain may be nil (a cached
// negative result).
func (s *DomainService) resolveCacheGet(host string) (*CustomDomain, bool) {
	s.resolveCacheMu.RLock()
	entry, ok := s.resolveCache[host]
	s.resolveCacheMu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.domain, true
}

// resolveCacheSet records a host resolution (cd may be nil for a negative
// result) with a TTL of domainResolveCacheTTL.
func (s *DomainService) resolveCacheSet(host string, cd *CustomDomain) {
	s.resolveCacheMu.Lock()
	if s.resolveCache == nil {
		s.resolveCache = make(map[string]resolveCacheEntry)
	}
	s.resolveCache[host] = resolveCacheEntry{domain: cd, expires: time.Now().Add(domainResolveCacheTTL)}
	s.resolveCacheMu.Unlock()
}

// invalidateResolveCache clears every cached resolution. Called after a domain
// transitions to (or from) verified+active so the change is visible immediately
// rather than after the TTL.
func (s *DomainService) invalidateResolveCache() {
	s.resolveCacheMu.Lock()
	s.resolveCache = make(map[string]resolveCacheEntry)
	s.resolveCacheMu.Unlock()
}

// verificationTTL returns the configured verification window as a duration,
// falling back to 24h when unset or invalid (PART 36 verification_ttl: 24h).
func (s *DomainService) verificationTTL() time.Duration {
	if s.cfg.VerificationTTL > 0 {
		return time.Duration(s.cfg.VerificationTTL) * time.Second
	}
	return 24 * time.Hour
}

// RetryPendingVerifications re-runs DNS-TXT ownership verification for every
// custom domain that is not yet active and whose creation is still inside the
// verification_ttl window (PART 36 scheduled verify-retry task). Domains past
// the window are left for CleanupExpiredPendingVerifications. It returns the
// number of domains re-checked. Per-domain verification errors are expected
// (DNS not yet propagated) and do not abort the sweep.
func (s *DomainService) RetryPendingVerifications(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-s.verificationTTL())

	rows, err := s.store.UsersDB.QueryContext(ctx,
		`SELECT id FROM custom_domains
		 WHERE status != 'active'
		   AND verification_status != 'verified'
		   AND created_at >= ?
		 ORDER BY id ASC`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to query pending domains: %w", err)
	}

	var ids []int64
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr != nil {
			rows.Close()
			return 0, fmt.Errorf("failed to scan pending domain id: %w", scanErr)
		}
		ids = append(ids, id)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return 0, closeErr
	}

	for _, id := range ids {
		// A verification failure here is normal (DNS still propagating);
		// VerifyDomain records the failure on the row, so we swallow it and
		// continue re-checking the rest of the batch.
		_ = s.VerifyDomain(ctx, id)
	}

	return len(ids), nil
}

// CleanupExpiredPendingVerifications removes custom domains that never became
// active and whose creation is older than the verification_ttl window (PART 36
// scheduled cleanup task). Verified/active domains are never touched. It
// returns the number of rows deleted.
func (s *DomainService) CleanupExpiredPendingVerifications(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-s.verificationTTL())

	res, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM custom_domains
		 WHERE status != 'active'
		   AND verification_status != 'verified'
		   AND created_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired domains: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
