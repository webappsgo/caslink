package federation

import (
	"context"
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Service coordinates federation operations
type Service struct {
	db        *db.DB
	config    *config.FederationConfig
	logger    *logrus.Logger
	discovery *DiscoveryService
	client    *FederationClient
	server    *FederationServer
	sync      *SyncService
	protocol  *ProtocolHandler
	keys      *KeyManager
}

// NewService creates a new federation service
func NewService(database *db.DB, cfg *config.Config, logger *logrus.Logger) (*Service, error) {
	if !cfg.Federation.Enabled {
		return &Service{
			db:     database,
			config: &cfg.Federation,
			logger: logger,
		}, nil
	}

	service := &Service{
		db:     database,
		config: &cfg.Federation,
		logger: logger,
	}

	// Initialize key manager
	var err error
	service.keys, err = NewKeyManager(&cfg.Federation, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create key manager: %w", err)
	}

	// Initialize components
	service.discovery, err = NewDiscoveryService(database, &cfg.Federation, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery service: %w", err)
	}

	service.client, err = NewFederationClient(database, &cfg.Federation, logger, service.keys)
	if err != nil {
		return nil, fmt.Errorf("failed to create federation client: %w", err)
	}

	service.server, err = NewFederationServer(database, &cfg.Federation, logger, service.keys)
	if err != nil {
		return nil, fmt.Errorf("failed to create federation server: %w", err)
	}

	service.sync, err = NewSyncService(database, &cfg.Federation, logger, service.client, service.server)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync service: %w", err)
	}

	service.protocol, err = NewProtocolHandler(database, &cfg.Federation, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create protocol handler: %w", err)
	}

	return service, nil
}

// IsEnabled returns whether federation is enabled
func (s *Service) IsEnabled() bool {
	return s.config.Enabled
}

// GetDiscovery returns the discovery service
func (s *Service) GetDiscovery() *DiscoveryService {
	return s.discovery
}

// GetClient returns the federation client
func (s *Service) GetClient() *FederationClient {
	return s.client
}

// GetServer returns the federation server
func (s *Service) GetServer() *FederationServer {
	return s.server
}

// GetSync returns the sync service
func (s *Service) GetSync() *SyncService {
	return s.sync
}

// GetProtocol returns the protocol handler
func (s *Service) GetProtocol() *ProtocolHandler {
	return s.protocol
}

// DiscoverInstances discovers other federation instances
func (s *Service) DiscoverInstances(ctx context.Context) ([]*FederatedInstance, error) {
	if !s.IsEnabled() {
		return []*FederatedInstance{}, nil
	}

	return s.discovery.DiscoverInstances(ctx)
}

// ShareURL shares a URL with federated instances
func (s *Service) ShareURL(ctx context.Context, url *FederatedURL) error {
	if !s.IsEnabled() {
		return nil
	}

	if !s.config.SharePublicURLs {
		return nil // Sharing disabled
	}

	return s.sync.ShareURL(ctx, url)
}

// SyncFromInstance syncs URLs from a specific instance
func (s *Service) SyncFromInstance(ctx context.Context, instanceDomain string) error {
	if !s.IsEnabled() {
		return fmt.Errorf("federation is not enabled")
	}

	return s.sync.SyncFromInstance(ctx, instanceDomain)
}

// SyncWithAllInstances syncs with all known instances
func (s *Service) SyncWithAllInstances(ctx context.Context) error {
	if !s.IsEnabled() {
		return nil
	}

	return s.sync.SyncWithAllInstances(ctx)
}

// AddInstance adds a federated instance
func (s *Service) AddInstance(ctx context.Context, domain, publicKey string) (*FederatedInstance, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("federation is not enabled")
	}

	// Parse and validate public key
	parsedKey, err := s.keys.ParsePublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	instance := &FederatedInstance{
		ID:           s.generateInstanceID(),
		Domain:       domain,
		PublicKey:    publicKey,
		DiscoveredAt: time.Now(),
		Active:       true,
		SyncEnabled:  true,
	}

	// Verify instance by making a discovery request
	if err := s.client.VerifyInstance(ctx, instance); err != nil {
		return nil, fmt.Errorf("failed to verify instance: %w", err)
	}

	// Save instance
	if err := s.saveInstance(ctx, instance); err != nil {
		return nil, fmt.Errorf("failed to save instance: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"domain":     domain,
		"public_key": publicKey[:20] + "...",
	}).Info("Federated instance added")

	return instance, nil
}

// RemoveInstance removes a federated instance
func (s *Service) RemoveInstance(ctx context.Context, domain string) error {
	if !s.IsEnabled() {
		return fmt.Errorf("federation is not enabled")
	}

	query := `UPDATE federation_instances SET active = false WHERE domain = ?`
	_, err := s.db.ExecContext(ctx, query, domain)
	if err != nil {
		return fmt.Errorf("failed to remove instance: %w", err)
	}

	s.logger.WithField("domain", domain).Info("Federated instance removed")
	return nil
}

// BlockInstance blocks a federated instance
func (s *Service) BlockInstance(ctx context.Context, domain string) error {
	if !s.IsEnabled() {
		return fmt.Errorf("federation is not enabled")
	}

	query := `UPDATE federation_instances SET blocked = true WHERE domain = ?`
	_, err := s.db.ExecContext(ctx, query, domain)
	if err != nil {
		return fmt.Errorf("failed to block instance: %w", err)
	}

	s.logger.WithField("domain", domain).Info("Federated instance blocked")
	return nil
}

// GetInstances returns all federated instances
func (s *Service) GetInstances(ctx context.Context) ([]*FederatedInstance, error) {
	if !s.IsEnabled() {
		return []*FederatedInstance{}, nil
	}

	query := `
		SELECT id, domain, public_key, discovered_at, last_sync, active, blocked, sync_enabled
		FROM federation_instances
		WHERE active = true
		ORDER BY domain ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query instances: %w", err)
	}
	defer rows.Close()

	var instances []*FederatedInstance
	for rows.Next() {
		instance := &FederatedInstance{}
		err := rows.Scan(
			&instance.ID, &instance.Domain, &instance.PublicKey,
			&instance.DiscoveredAt, &instance.LastSync,
			&instance.Active, &instance.Blocked, &instance.SyncEnabled,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan instance: %w", err)
		}
		instances = append(instances, instance)
	}

	return instances, nil
}

// GetFederatedURLs returns federated URLs
func (s *Service) GetFederatedURLs(ctx context.Context, limit, offset int) ([]*FederatedURL, error) {
	if !s.IsEnabled() {
		return []*FederatedURL{}, nil
	}

	query := `
		SELECT id, original_id, source_instance, original_url, short_code,
		       title, created_at, synced_at
		FROM federated_urls
		ORDER BY synced_at DESC
		LIMIT ? OFFSET ?`

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query federated URLs: %w", err)
	}
	defer rows.Close()

	var urls []*FederatedURL
	for rows.Next() {
		url := &FederatedURL{}
		err := rows.Scan(
			&url.ID, &url.OriginalID, &url.SourceInstance,
			&url.OriginalURL, &url.ShortCode, &url.Title,
			&url.CreatedAt, &url.SyncedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan federated URL: %w", err)
		}
		urls = append(urls, url)
	}

	return urls, nil
}

// GetPublicKey returns the public key for this instance
func (s *Service) GetPublicKey() (*rsa.PublicKey, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("federation is not enabled")
	}

	return s.keys.GetPublicKey()
}

// GetPublicKeyPEM returns the public key in PEM format
func (s *Service) GetPublicKeyPEM() (string, error) {
	if !s.IsEnabled() {
		return "", fmt.Errorf("federation is not enabled")
	}

	return s.keys.GetPublicKeyPEM()
}

// SignData signs data with the private key
func (s *Service) SignData(data []byte) ([]byte, error) {
	if !s.IsEnabled() {
		return nil, fmt.Errorf("federation is not enabled")
	}

	return s.keys.Sign(data)
}

// VerifySignature verifies a signature from another instance
func (s *Service) VerifySignature(data, signature []byte, instanceDomain string) error {
	if !s.IsEnabled() {
		return fmt.Errorf("federation is not enabled")
	}

	// Get instance public key
	instance, err := s.getInstanceByDomain(context.Background(), instanceDomain)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	publicKey, err := s.keys.ParsePublicKey(instance.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	return s.keys.Verify(data, signature, publicKey)
}

// GetFederationStats returns federation statistics
func (s *Service) GetFederationStats(ctx context.Context) (*FederationStats, error) {
	if !s.IsEnabled() {
		return &FederationStats{}, nil
	}

	stats := &FederationStats{}

	// Count active instances
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM federation_instances WHERE active = true AND blocked = false").Scan(&stats.ActiveInstances)
	if err != nil {
		return nil, fmt.Errorf("failed to count active instances: %w", err)
	}

	// Count blocked instances
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM federation_instances WHERE blocked = true").Scan(&stats.BlockedInstances)
	if err != nil {
		return nil, fmt.Errorf("failed to count blocked instances: %w", err)
	}

	// Count federated URLs
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM federated_urls").Scan(&stats.FederatedURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to count federated URLs: %w", err)
	}

	// Count shared URLs (our URLs shared with others)
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM urls WHERE federated = true").Scan(&stats.SharedURLs)
	if err != nil {
		// This column might not exist, ignore error
		stats.SharedURLs = 0
	}

	// Get last sync time
	var lastSyncStr string
	err = s.db.QueryRowContext(ctx, "SELECT MAX(last_sync) FROM federation_instances WHERE last_sync IS NOT NULL").Scan(&lastSyncStr)
	if err == nil && lastSyncStr != "" {
		lastSync, err := time.Parse("2006-01-02 15:04:05", lastSyncStr)
		if err == nil {
			stats.LastSync = &lastSync
		}
	}

	return stats, nil
}

// Helper methods

func (s *Service) saveInstance(ctx context.Context, instance *FederatedInstance) error {
	query := `
		INSERT INTO federation_instances (id, domain, public_key, discovered_at, active, blocked, sync_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		public_key = VALUES(public_key), discovered_at = VALUES(discovered_at), active = true`

	_, err := s.db.ExecContext(ctx, query,
		instance.ID, instance.Domain, instance.PublicKey,
		instance.DiscoveredAt, instance.Active, instance.Blocked, instance.SyncEnabled,
	)
	if err != nil {
		return fmt.Errorf("failed to save instance: %w", err)
	}

	return nil
}

func (s *Service) getInstanceByDomain(ctx context.Context, domain string) (*FederatedInstance, error) {
	query := `
		SELECT id, domain, public_key, discovered_at, last_sync, active, blocked, sync_enabled
		FROM federation_instances
		WHERE domain = ? AND active = true`

	row := s.db.QueryRowContext(ctx, query, domain)

	instance := &FederatedInstance{}
	err := row.Scan(
		&instance.ID, &instance.Domain, &instance.PublicKey,
		&instance.DiscoveredAt, &instance.LastSync,
		&instance.Active, &instance.Blocked, &instance.SyncEnabled,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	return instance, nil
}

func (s *Service) generateInstanceID() string {
	return fmt.Sprintf("inst_%d", time.Now().UnixNano())
}

// FederationStats represents federation statistics
type FederationStats struct {
	ActiveInstances  int64      `json:"active_instances"`
	BlockedInstances int64      `json:"blocked_instances"`
	FederatedURLs    int64      `json:"federated_urls"`
	SharedURLs       int64      `json:"shared_urls"`
	LastSync         *time.Time `json:"last_sync,omitempty"`
}

// FederatedInstance represents a federated instance
type FederatedInstance struct {
	ID           string     `json:"id" db:"id"`
	Domain       string     `json:"domain" db:"domain"`
	PublicKey    string     `json:"public_key" db:"public_key"`
	DiscoveredAt time.Time  `json:"discovered_at" db:"discovered_at"`
	LastSync     *time.Time `json:"last_sync" db:"last_sync"`
	Active       bool       `json:"active" db:"active"`
	Blocked      bool       `json:"blocked" db:"blocked"`
	SyncEnabled  bool       `json:"sync_enabled" db:"sync_enabled"`
}

// FederatedURL represents a URL from a federated instance
type FederatedURL struct {
	ID             string    `json:"id" db:"id"`
	OriginalID     string    `json:"original_id" db:"original_id"`
	SourceInstance string    `json:"source_instance" db:"source_instance"`
	OriginalURL    string    `json:"original_url" db:"original_url"`
	ShortCode      string    `json:"short_code" db:"short_code"`
	Title          string    `json:"title" db:"title"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	SyncedAt       time.Time `json:"synced_at" db:"synced_at"`
}