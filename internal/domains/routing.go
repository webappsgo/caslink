package domains

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// RoutingService handles domain-based request routing
type RoutingService struct {
	db          *db.DB
	config      *config.Config
	logger      *logrus.Logger
	ssl         *SSLService
	domainCache map[string]*Domain
	tlsCache    map[string]*tls.Config
	cacheMutex  sync.RWMutex
	cacheExpiry time.Time
}

// DomainRouter provides domain-aware HTTP routing
type DomainRouter struct {
	routing *RoutingService
	routers map[string]*mux.Router
	mutex   sync.RWMutex
}

// DomainMiddleware provides domain validation and routing middleware
type DomainMiddleware struct {
	routing *RoutingService
}

// RouteInfo contains information about a domain route
type RouteInfo struct {
	Domain       string `json:"domain"`
	DomainID     string `json:"domain_id"`
	UserID       string `json:"user_id"`
	SSLEnabled   bool   `json:"ssl_enabled"`
	Verified     bool   `json:"verified"`
	IsDefault    bool   `json:"is_default"`
	RequestCount int64  `json:"request_count"`
	LastAccess   *time.Time `json:"last_access,omitempty"`
}

// NewRoutingService creates a new routing service
func NewRoutingService(database *db.DB, cfg *config.Config, logger *logrus.Logger) (*RoutingService, error) {
	ssl, err := NewSSLService(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSL service: %w", err)
	}

	return &RoutingService{
		db:          database,
		config:      cfg,
		logger:      logger,
		ssl:         ssl,
		domainCache: make(map[string]*Domain),
		tlsCache:    make(map[string]*tls.Config),
		cacheExpiry: time.Now().Add(5 * time.Minute),
	}, nil
}

// GetDomainFromRequest extracts domain information from HTTP request
func (r *RoutingService) GetDomainFromRequest(req *http.Request) (*Domain, error) {
	host := req.Host
	if host == "" {
		return nil, fmt.Errorf("no host header found")
	}

	// Remove port if present
	if colonIndex := strings.LastIndex(host, ":"); colonIndex != -1 {
		host = host[:colonIndex]
	}

	// Check cache first
	r.cacheMutex.RLock()
	if cachedDomain, exists := r.domainCache[host]; exists && time.Now().Before(r.cacheExpiry) {
		r.cacheMutex.RUnlock()
		return cachedDomain, nil
	}
	r.cacheMutex.RUnlock()

	// Query database for domain
	domain, err := r.getDomainByName(context.Background(), host)
	if err != nil {
		// If custom domain not found, check if this is the default domain
		if r.isDefaultDomain(host) {
			return r.createVirtualDefaultDomain(host), nil
		}
		return nil, fmt.Errorf("domain not found: %w", err)
	}

	// Cache the result
	r.cacheMutex.Lock()
	r.domainCache[host] = domain
	r.cacheMutex.Unlock()

	return domain, nil
}

// GetTLSConfig returns TLS configuration for a domain
func (r *RoutingService) GetTLSConfig(ctx context.Context, domain string) (*tls.Config, error) {
	// Check cache first
	r.cacheMutex.RLock()
	if cachedConfig, exists := r.tlsCache[domain]; exists && time.Now().Before(r.cacheExpiry) {
		r.cacheMutex.RUnlock()
		return cachedConfig, nil
	}
	r.cacheMutex.RUnlock()

	// Get domain information
	domainInfo, err := r.getDomainByName(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("domain not found: %w", err)
	}

	if !domainInfo.SSLEnabled {
		return nil, fmt.Errorf("SSL not enabled for domain: %s", domain)
	}

	// Load TLS configuration
	tlsConfig, err := r.ssl.LoadTLSConfig(ctx, domainInfo.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS config: %w", err)
	}

	// Cache the configuration
	r.cacheMutex.Lock()
	r.tlsCache[domain] = tlsConfig
	r.cacheMutex.Unlock()

	return tlsConfig, nil
}

// CreateDomainRouter creates a domain-aware router
func (r *RoutingService) CreateDomainRouter() *DomainRouter {
	return &DomainRouter{
		routing: r,
		routers: make(map[string]*mux.Router),
	}
}

// GetRouterForDomain returns a router for the specified domain
func (dr *DomainRouter) GetRouterForDomain(domain string) *mux.Router {
	dr.mutex.RLock()
	if router, exists := dr.routers[domain]; exists {
		dr.mutex.RUnlock()
		return router
	}
	dr.mutex.RUnlock()

	// Create new router for domain
	dr.mutex.Lock()
	defer dr.mutex.Unlock()

	// Double-check after acquiring write lock
	if router, exists := dr.routers[domain]; exists {
		return router
	}

	router := mux.NewRouter()
	dr.routers[domain] = router
	return router
}

// ServeHTTP implements http.Handler for domain-based routing
func (dr *DomainRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract domain from request
	domain, err := dr.routing.GetDomainFromRequest(r)
	if err != nil {
		http.Error(w, "Domain not found", http.StatusNotFound)
		return
	}

	// Check if domain is verified
	if !domain.Verified && !dr.routing.isDefaultDomain(domain.Domain) {
		http.Error(w, "Domain not verified", http.StatusForbidden)
		return
	}

	// Get router for domain
	router := dr.GetRouterForDomain(domain.Domain)

	// Update access statistics
	go dr.routing.updateDomainAccess(context.Background(), domain.ID)

	// Serve request
	router.ServeHTTP(w, r)
}

// CreateDomainMiddleware creates domain validation middleware
func (r *RoutingService) CreateDomainMiddleware() *DomainMiddleware {
	return &DomainMiddleware{routing: r}
}

// Middleware validates domain and adds domain context to request
func (dm *DomainMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract domain from request
		domain, err := dm.routing.GetDomainFromRequest(r)
		if err != nil {
			dm.routing.logger.WithError(err).WithField("host", r.Host).Warn("Invalid domain in request")
			http.Error(w, "Domain not found", http.StatusNotFound)
			return
		}

		// Check if domain is verified (except for default domain and verification endpoints)
		if !domain.Verified && !dm.routing.isDefaultDomain(domain.Domain) && !dm.isVerificationEndpoint(r.URL.Path) {
			http.Error(w, "Domain not verified", http.StatusForbidden)
			return
		}

		// Add domain to request context
		ctx := context.WithValue(r.Context(), "domain", domain)
		r = r.WithContext(ctx)

		// Update access statistics
		go dm.routing.updateDomainAccess(ctx, domain.ID)

		next.ServeHTTP(w, r)
	})
}

// GetRouteInfo returns routing information for all domains
func (r *RoutingService) GetRouteInfo(ctx context.Context) ([]*RouteInfo, error) {
	query := `
		SELECT d.domain, d.id, d.user_id, d.ssl_enabled, d.verified, d.is_default,
		       COALESCE(stats.request_count, 0) as request_count,
		       stats.last_access
		FROM domains d
		LEFT JOIN (
			SELECT domain_id,
			       COUNT(*) as request_count,
			       MAX(accessed_at) as last_access
			FROM domain_access_logs
			WHERE accessed_at >= ?
			GROUP BY domain_id
		) stats ON d.id = stats.domain_id
		ORDER BY d.is_default DESC, d.created_at ASC`

	since := time.Now().AddDate(0, 0, -30) // Last 30 days
	rows, err := r.db.QueryContext(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query route info: %w", err)
	}
	defer rows.Close()

	var routes []*RouteInfo
	for rows.Next() {
		route := &RouteInfo{}
		err := rows.Scan(
			&route.Domain, &route.DomainID, &route.UserID,
			&route.SSLEnabled, &route.Verified, &route.IsDefault,
			&route.RequestCount, &route.LastAccess,
		)
		if err != nil {
			r.logger.WithError(err).Warn("Failed to scan route info")
			continue
		}
		routes = append(routes, route)
	}

	return routes, nil
}

// ClearCache clears the domain and TLS caches
func (r *RoutingService) ClearCache() {
	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()

	r.domainCache = make(map[string]*Domain)
	r.tlsCache = make(map[string]*tls.Config)
	r.cacheExpiry = time.Now().Add(5 * time.Minute)

	r.logger.Info("Domain routing cache cleared")
}

// ValidateDomainConfiguration validates domain routing configuration
func (r *RoutingService) ValidateDomainConfiguration(ctx context.Context) error {
	// Check for conflicting domain configurations
	conflicts, err := r.findDomainConflicts(ctx)
	if err != nil {
		return fmt.Errorf("failed to check domain conflicts: %w", err)
	}

	if len(conflicts) > 0 {
		return fmt.Errorf("found %d domain configuration conflicts", len(conflicts))
	}

	// Validate SSL certificates for SSL-enabled domains
	expiredCerts, err := r.findExpiredCertificates(ctx)
	if err != nil {
		return fmt.Errorf("failed to check certificate expiration: %w", err)
	}

	if len(expiredCerts) > 0 {
		r.logger.WithField("expired_count", len(expiredCerts)).Warn("Found expired SSL certificates")
	}

	return nil
}

// getDomainByName retrieves a domain by name from database
func (r *RoutingService) getDomainByName(ctx context.Context, domainName string) (*Domain, error) {
	query := `
		SELECT id, user_id, domain, is_default, ssl_enabled, ssl_cert_path, ssl_key_path,
		       verified, verification_token, verification_method, created_at, verified_at
		FROM domains
		WHERE domain = ?`

	row := r.db.QueryRowContext(ctx, query, strings.ToLower(domainName))

	domain := &Domain{}
	err := row.Scan(
		&domain.ID, &domain.UserID, &domain.Domain, &domain.IsDefault, &domain.SSLEnabled,
		&domain.SSLCertPath, &domain.SSLKeyPath, &domain.Verified, &domain.VerificationToken,
		&domain.VerificationMethod, &domain.CreatedAt, &domain.VerifiedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("domain not found: %w", err)
	}

	return domain, nil
}

// isDefaultDomain checks if the given domain is the default application domain
func (r *RoutingService) isDefaultDomain(domain string) bool {
	// Compare with configured base URL domain
	if r.config.Server.BaseURL != "" {
		baseHost := strings.TrimPrefix(r.config.Server.BaseURL, "http://")
		baseHost = strings.TrimPrefix(baseHost, "https://")
		if colonIndex := strings.LastIndex(baseHost, ":"); colonIndex != -1 {
			baseHost = baseHost[:colonIndex]
		}
		if strings.ToLower(domain) == strings.ToLower(baseHost) {
			return true
		}
	}

	// Check for common default domains
	defaultDomains := []string{"localhost", "127.0.0.1", "::1"}
	lowerDomain := strings.ToLower(domain)
	for _, defaultDomain := range defaultDomains {
		if lowerDomain == defaultDomain {
			return true
		}
	}

	return false
}

// createVirtualDefaultDomain creates a virtual domain for the default application domain
func (r *RoutingService) createVirtualDefaultDomain(domain string) *Domain {
	return &Domain{
		ID:                 "default",
		UserID:             "",
		Domain:             domain,
		IsDefault:          true,
		SSLEnabled:         false,
		Verified:           true,
		VerificationMethod: "none",
		CreatedAt:          time.Now(),
		VerifiedAt:         &[]time.Time{time.Now()}[0],
	}
}

// isVerificationEndpoint checks if the path is a domain verification endpoint
func (dm *DomainMiddleware) isVerificationEndpoint(path string) bool {
	verificationPaths := []string{
		"/.well-known/caslink-verification.txt",
		"/verify-domain",
		"/api/v1/domains/verify",
	}

	for _, verificationPath := range verificationPaths {
		if strings.HasPrefix(path, verificationPath) {
			return true
		}
	}

	return false
}

// updateDomainAccess updates domain access statistics
func (r *RoutingService) updateDomainAccess(ctx context.Context, domainID string) {
	if domainID == "default" || domainID == "" {
		return // Don't track access for default domain
	}

	query := `
		INSERT INTO domain_access_logs (domain_id, accessed_at)
		VALUES (?, ?)
		ON CONFLICT(domain_id, date(accessed_at)) DO UPDATE SET
		access_count = access_count + 1,
		last_access = ?`

	now := time.Now()
	_, err := r.db.ExecContext(ctx, query, domainID, now, now)
	if err != nil {
		r.logger.WithError(err).WithField("domain_id", domainID).Error("Failed to update domain access statistics")
	}
}

// findDomainConflicts finds conflicting domain configurations
func (r *RoutingService) findDomainConflicts(ctx context.Context) ([]string, error) {
	query := `
		SELECT domain, COUNT(*) as count
		FROM domains
		GROUP BY domain
		HAVING count > 1`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conflicts []string
	for rows.Next() {
		var domain string
		var count int
		if err := rows.Scan(&domain, &count); err != nil {
			continue
		}
		conflicts = append(conflicts, fmt.Sprintf("%s (%d instances)", domain, count))
	}

	return conflicts, nil
}

// findExpiredCertificates finds domains with expired SSL certificates
func (r *RoutingService) findExpiredCertificates(ctx context.Context) ([]string, error) {
	query := `
		SELECT d.domain
		FROM domains d
		JOIN ssl_certificates c ON d.id = c.domain_id
		WHERE d.ssl_enabled = true AND c.status = 'active' AND c.expires_at <= ?`

	rows, err := r.db.QueryContext(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expiredDomains []string
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			continue
		}
		expiredDomains = append(expiredDomains, domain)
	}

	return expiredDomains, nil
}

// GetDomainFromContext extracts domain from request context
func GetDomainFromContext(ctx context.Context) (*Domain, bool) {
	domain, ok := ctx.Value("domain").(*Domain)
	return domain, ok
}