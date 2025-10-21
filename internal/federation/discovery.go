package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// DiscoveryService handles instance discovery via DNS and well-known endpoints
type DiscoveryService struct {
	db     *db.DB
	config *config.FederationConfig
	logger *logrus.Logger
	client *http.Client
}

// InstanceInfo represents discovered instance information
type InstanceInfo struct {
	Domain        string                 `json:"domain"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Version       string                 `json:"version"`
	PublicKey     string                 `json:"public_key"`
	FederationURL string                 `json:"federation_url"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// WellKnownResponse represents the .well-known/caslink response
type WellKnownResponse struct {
	Instance      InstanceInfo `json:"instance"`
	Federation    struct {
		Enabled     bool   `json:"enabled"`
		Version     string `json:"version"`
		Endpoint    string `json:"endpoint"`
		PublicKey   string `json:"public_key"`
	} `json:"federation"`
	Capabilities []string `json:"capabilities"`
}

// NewDiscoveryService creates a new discovery service
func NewDiscoveryService(database *db.DB, cfg *config.FederationConfig, logger *logrus.Logger) (*DiscoveryService, error) {
	return &DiscoveryService{
		db:     database,
		config: cfg,
		logger: logger,
		client: &http.Client{
			Timeout: cfg.DiscoveryTimeout,
		},
	}, nil
}

// DiscoverInstance discovers a specific instance by domain
func (d *DiscoveryService) DiscoverInstance(ctx context.Context, domain string) (*InstanceInfo, error) {
	d.logger.WithField("domain", domain).Debug("Discovering instance")

	// Try well-known endpoint first
	instance, err := d.discoverViaWellKnown(ctx, domain)
	if err != nil {
		d.logger.WithError(err).Debug("Well-known discovery failed, trying DNS")

		// Fallback to DNS discovery
		instance, err = d.discoverViaDNS(ctx, domain)
		if err != nil {
			return nil, fmt.Errorf("failed to discover instance %s: %w", domain, err)
		}
	}

	d.logger.WithField("domain", domain).Info("Successfully discovered instance")
	return instance, nil
}

// DiscoverInstances discovers multiple instances using various methods
func (d *DiscoveryService) DiscoverInstances(ctx context.Context) ([]*FederatedInstance, error) {
	d.logger.Debug("Starting instance discovery")

	var allInstances []*FederatedInstance

	// Discover via DNS TXT records
	dnsInstances, err := d.discoverInstancesViaDNS(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("DNS discovery failed")
	} else {
		allInstances = append(allInstances, dnsInstances...)
	}

	// Discover via known networks (e.g., caslink.network registry)
	networkInstances, err := d.discoverInstancesViaNetwork(ctx)
	if err != nil {
		d.logger.WithError(err).Warn("Network discovery failed")
	} else {
		allInstances = append(allInstances, networkInstances...)
	}

	// Save discovered instances to database
	for _, instance := range allInstances {
		if err := d.saveDiscoveredInstance(ctx, instance); err != nil {
			d.logger.WithError(err).WithField("domain", instance.Domain).Warn("Failed to save discovered instance")
		}
	}

	d.logger.WithField("count", len(allInstances)).Info("Completed instance discovery")
	return allInstances, nil
}

// discoverViaWellKnown discovers instance via .well-known/caslink endpoint
func (d *DiscoveryService) discoverViaWellKnown(ctx context.Context, domain string) (*InstanceInfo, error) {
	url := fmt.Sprintf("https://%s/.well-known/caslink", domain)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Caslink-Federation/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var wellKnown WellKnownResponse
	if err := json.NewDecoder(resp.Body).Decode(&wellKnown); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !wellKnown.Federation.Enabled {
		return nil, fmt.Errorf("federation not enabled on instance")
	}

	instance := &InstanceInfo{
		Domain:        domain,
		Name:          wellKnown.Instance.Name,
		Description:   wellKnown.Instance.Description,
		Version:       wellKnown.Instance.Version,
		PublicKey:     wellKnown.Federation.PublicKey,
		FederationURL: wellKnown.Federation.Endpoint,
		Metadata: map[string]interface{}{
			"capabilities": wellKnown.Capabilities,
			"version":      wellKnown.Federation.Version,
		},
	}

	return instance, nil
}

// discoverViaDNS discovers instance via DNS TXT records
func (d *DiscoveryService) discoverViaDNS(ctx context.Context, domain string) (*InstanceInfo, error) {
	// Look for TXT record at _caslink.<domain>
	txtDomain := fmt.Sprintf("_caslink.%s", domain)

	txtRecords, err := net.LookupTXT(txtDomain)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup TXT records: %w", err)
	}

	for _, record := range txtRecords {
		if strings.HasPrefix(record, "caslink=") {
			// Parse caslink TXT record
			params := d.parseTXTRecord(record)

			if publicKey, ok := params["pubkey"]; ok {
				instance := &InstanceInfo{
					Domain:    domain,
					PublicKey: publicKey,
					Metadata: map[string]interface{}{
						"discovery_method": "dns",
						"txt_record":       record,
					},
				}

				if name, ok := params["name"]; ok {
					instance.Name = name
				}

				if endpoint, ok := params["endpoint"]; ok {
					instance.FederationURL = endpoint
				}

				return instance, nil
			}
		}
	}

	return nil, fmt.Errorf("no valid caslink TXT record found")
}

// discoverInstancesViaDNS discovers instances via DNS enumeration
func (d *DiscoveryService) discoverInstancesViaDNS(ctx context.Context) ([]*FederatedInstance, error) {
	var instances []*FederatedInstance

	// Look for instances in known DNS zones
	knownZones := []string{
		"caslink.network",
		"caslink.org",
		"shortener.network",
	}

	for _, zone := range knownZones {
		zoneInstances, err := d.discoverInstancesInZone(ctx, zone)
		if err != nil {
			d.logger.WithError(err).WithField("zone", zone).Debug("Failed to discover instances in zone")
			continue
		}
		instances = append(instances, zoneInstances...)
	}

	return instances, nil
}

// discoverInstancesInZone discovers instances in a specific DNS zone
func (d *DiscoveryService) discoverInstancesInZone(ctx context.Context, zone string) ([]*FederatedInstance, error) {
	// Look for TXT record at _registry.<zone> that lists instances
	registryDomain := fmt.Sprintf("_registry.%s", zone)

	txtRecords, err := net.LookupTXT(registryDomain)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup registry records: %w", err)
	}

	var instances []*FederatedInstance

	for _, record := range txtRecords {
		if strings.HasPrefix(record, "instances=") {
			// Parse comma-separated list of instance domains
			instanceList := strings.TrimPrefix(record, "instances=")
			domains := strings.Split(instanceList, ",")

			for _, domain := range domains {
				domain = strings.TrimSpace(domain)
				if domain == "" {
					continue
				}

				// Discover each instance
				instanceInfo, err := d.DiscoverInstance(ctx, domain)
				if err != nil {
					d.logger.WithError(err).WithField("domain", domain).Debug("Failed to discover instance from registry")
					continue
				}

				instance := &FederatedInstance{
					ID:           d.generateInstanceID(),
					Domain:       domain,
					PublicKey:    instanceInfo.PublicKey,
					DiscoveredAt: time.Now(),
					Active:       true,
					SyncEnabled:  true,
				}

				instances = append(instances, instance)
			}
		}
	}

	return instances, nil
}

// discoverInstancesViaNetwork discovers instances via known network registries
func (d *DiscoveryService) discoverInstancesViaNetwork(ctx context.Context) ([]*FederatedInstance, error) {
	var instances []*FederatedInstance

	// Known federation networks
	networks := []string{
		"https://registry.caslink.network/api/instances",
		"https://directory.shortener.network/api/instances",
	}

	for _, networkURL := range networks {
		networkInstances, err := d.discoverInstancesFromNetwork(ctx, networkURL)
		if err != nil {
			d.logger.WithError(err).WithField("network", networkURL).Debug("Failed to discover instances from network")
			continue
		}
		instances = append(instances, networkInstances...)
	}

	return instances, nil
}

// discoverInstancesFromNetwork discovers instances from a network registry
func (d *DiscoveryService) discoverInstancesFromNetwork(ctx context.Context, networkURL string) ([]*FederatedInstance, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", networkURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Caslink-Federation/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var networkResponse struct {
		Instances []struct {
			Domain    string `json:"domain"`
			PublicKey string `json:"public_key"`
			Name      string `json:"name"`
			Active    bool   `json:"active"`
		} `json:"instances"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&networkResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var instances []*FederatedInstance

	for _, netInstance := range networkResponse.Instances {
		if !netInstance.Active {
			continue
		}

		instance := &FederatedInstance{
			ID:           d.generateInstanceID(),
			Domain:       netInstance.Domain,
			PublicKey:    netInstance.PublicKey,
			DiscoveredAt: time.Now(),
			Active:       true,
			SyncEnabled:  true,
		}

		instances = append(instances, instance)
	}

	return instances, nil
}

// saveDiscoveredInstance saves a discovered instance to the database
func (d *DiscoveryService) saveDiscoveredInstance(ctx context.Context, instance *FederatedInstance) error {
	// Check if instance already exists
	var existingID string
	query := "SELECT id FROM federation_instances WHERE domain = ?"
	err := d.db.QueryRowContext(ctx, query, instance.Domain).Scan(&existingID)

	if err == nil {
		// Instance exists, update discovered_at timestamp
		updateQuery := "UPDATE federation_instances SET discovered_at = ? WHERE domain = ?"
		_, err := d.db.ExecContext(ctx, updateQuery, time.Now(), instance.Domain)
		return err
	}

	// Insert new instance
	insertQuery := `
		INSERT INTO federation_instances (id, domain, public_key, discovered_at, active, blocked, sync_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err = d.db.ExecContext(ctx, insertQuery,
		instance.ID, instance.Domain, instance.PublicKey,
		instance.DiscoveredAt, instance.Active, instance.Blocked, instance.SyncEnabled,
	)

	if err != nil {
		return fmt.Errorf("failed to save discovered instance: %w", err)
	}

	d.logger.WithField("domain", instance.Domain).Debug("Saved discovered instance")
	return nil
}

// parseTXTRecord parses a caslink TXT record into key-value pairs
func (d *DiscoveryService) parseTXTRecord(record string) map[string]string {
	params := make(map[string]string)

	// Remove "caslink=" prefix
	if !strings.HasPrefix(record, "caslink=") {
		return params
	}

	content := strings.TrimPrefix(record, "caslink=")

	// Split by semicolons
	parts := strings.Split(content, ";")

	for _, part := range parts {
		if strings.Contains(part, "=") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				params[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}

	return params
}

// generateInstanceID generates a unique instance ID
func (d *DiscoveryService) generateInstanceID() string {
	return fmt.Sprintf("disco_%d", time.Now().UnixNano())
}

// GetWellKnownResponse generates the .well-known/caslink response for this instance
func (d *DiscoveryService) GetWellKnownResponse(instanceDomain, publicKeyPEM string) *WellKnownResponse {
	return &WellKnownResponse{
		Instance: InstanceInfo{
			Domain:      instanceDomain,
			Name:        "Caslink URL Shortener",
			Description: "Self-hosted URL shortener with federation support",
			Version:     "1.0.0",
		},
		Federation: struct {
			Enabled     bool   `json:"enabled"`
			Version     string `json:"version"`
			Endpoint    string `json:"endpoint"`
			PublicKey   string `json:"public_key"`
		}{
			Enabled:   d.config.Enabled,
			Version:   "1.0",
			Endpoint:  fmt.Sprintf("https://%s/api/v1/federation", instanceDomain),
			PublicKey: publicKeyPEM,
		},
		Capabilities: []string{
			"url_sharing",
			"instance_discovery",
			"signature_verification",
		},
	}
}

// ValidateInstance validates a discovered instance
func (d *DiscoveryService) ValidateInstance(ctx context.Context, instance *InstanceInfo) error {
	if instance.Domain == "" {
		return fmt.Errorf("instance domain is required")
	}

	if instance.PublicKey == "" {
		return fmt.Errorf("instance public key is required")
	}

	// Try to reach the instance
	if instance.FederationURL != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", instance.FederationURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create validation request: %w", err)
		}

		req.Header.Set("User-Agent", "Caslink-Federation/1.0")

		resp, err := d.client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to validate instance: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			return fmt.Errorf("instance validation failed with status: %d", resp.StatusCode)
		}
	}

	return nil
}