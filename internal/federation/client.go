package federation

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// FederationClient handles communication with other federated instances
type FederationClient struct {
	db     *db.DB
	config *config.FederationConfig
	logger *logrus.Logger
	keys   *KeyManager
	client *http.Client
}

// URLShareRequest represents a request to share a URL with another instance
type URLShareRequest struct {
	URL         string            `json:"url"`
	ShortCode   string            `json:"short_code"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Signature   string            `json:"signature"`
}

// URLSyncRequest represents a request to synchronize URLs
type URLSyncRequest struct {
	Since     *time.Time `json:"since,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Signature string     `json:"signature"`
}

// URLSyncResponse represents a response to URL synchronization
type URLSyncResponse struct {
	URLs      []FederatedURL `json:"urls"`
	NextToken string         `json:"next_token,omitempty"`
	Total     int            `json:"total"`
}

// InstanceVerifyRequest represents a request to verify an instance
type InstanceVerifyRequest struct {
	Domain    string `json:"domain"`
	PublicKey string `json:"public_key"`
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature"`
}

// InstanceVerifyResponse represents a response to instance verification
type InstanceVerifyResponse struct {
	Verified  bool   `json:"verified"`
	Challenge string `json:"challenge,omitempty"`
	Message   string `json:"message,omitempty"`
}

// NewFederationClient creates a new federation client
func NewFederationClient(database *db.DB, cfg *config.FederationConfig, logger *logrus.Logger, keyManager *KeyManager) (*FederationClient, error) {
	return &FederationClient{
		db:     database,
		config: cfg,
		logger: logger,
		keys:   keyManager,
		client: &http.Client{
			Timeout: cfg.SyncTimeout,
		},
	}, nil
}

// ShareURL shares a URL with a specific federated instance
func (c *FederationClient) ShareURL(ctx context.Context, instance *FederatedInstance, urlData map[string]interface{}) error {
	c.logger.WithFields(logrus.Fields{
		"instance": instance.Domain,
		"url":      urlData["short_code"],
	}).Debug("Sharing URL with instance")

	// Prepare share request
	request := URLShareRequest{
		URL:         urlData["original_url"].(string),
		ShortCode:   urlData["short_code"].(string),
		CreatedAt:   time.Now(),
		Metadata:    make(map[string]string),
	}

	// Add optional fields
	if title, ok := urlData["title"].(string); ok {
		request.Title = title
	}

	if description, ok := urlData["description"].(string); ok {
		request.Description = description
	}

	// Sign the request
	requestData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	signature, err := c.keys.Sign(requestData)
	if err != nil {
		return fmt.Errorf("failed to sign request: %w", err)
	}

	request.Signature = string(signature)

	// Send request to instance
	endpoint := fmt.Sprintf("https://%s/api/v1/federation/share", instance.Domain)
	if err := c.sendRequest(ctx, "POST", endpoint, request, nil); err != nil {
		return fmt.Errorf("failed to share URL: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"instance": instance.Domain,
		"url":      request.ShortCode,
	}).Info("Successfully shared URL with instance")

	return nil
}

// FetchURLs fetches URLs from a federated instance
func (c *FederationClient) FetchURLs(ctx context.Context, instance *FederatedInstance) ([]*FederatedURL, error) {
	c.logger.WithField("instance", instance.Domain).Debug("Fetching URLs from instance")

	// Prepare sync request
	request := URLSyncRequest{
		Since: instance.LastSync,
		Limit: c.config.MaxURLsPerSync,
	}

	// Sign the request
	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	signature, err := c.keys.Sign(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	request.Signature = string(signature)

	// Send request to instance
	endpoint := fmt.Sprintf("https://%s/api/v1/federation/sync", instance.Domain)
	var response URLSyncResponse
	if err := c.sendRequest(ctx, "POST", endpoint, request, &response); err != nil {
		return nil, fmt.Errorf("failed to fetch URLs: %w", err)
	}

	// Convert to pointers
	urls := make([]*FederatedURL, len(response.URLs))
	for i, url := range response.URLs {
		urls[i] = &url
	}

	c.logger.WithFields(logrus.Fields{
		"instance": instance.Domain,
		"count":    len(urls),
	}).Info("Successfully fetched URLs from instance")

	return urls, nil
}

// VerifyInstance verifies a federated instance
func (c *FederationClient) VerifyInstance(ctx context.Context, instance *FederatedInstance) error {
	c.logger.WithField("instance", instance.Domain).Debug("Verifying instance")

	// Get our public key PEM
	publicKeyPEM, err := c.keys.GetPublicKeyPEM()
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Prepare verify request
	request := InstanceVerifyRequest{
		Domain:    c.config.Domain,
		PublicKey: publicKeyPEM,
		Timestamp: time.Now().Unix(),
	}

	// Sign the request
	requestData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	signature, err := c.keys.Sign(requestData)
	if err != nil {
		return fmt.Errorf("failed to sign request: %w", err)
	}

	request.Signature = string(signature)

	// Send request to instance
	endpoint := fmt.Sprintf("https://%s/api/v1/federation/verify", instance.Domain)
	var response InstanceVerifyResponse
	if err := c.sendRequest(ctx, "POST", endpoint, request, &response); err != nil {
		return fmt.Errorf("failed to verify instance: %w", err)
	}

	if !response.Verified {
		return fmt.Errorf("instance verification failed: %s", response.Message)
	}

	c.logger.WithField("instance", instance.Domain).Info("Successfully verified instance")
	return nil
}

// GetInstanceInfo retrieves information about a federated instance
func (c *FederationClient) GetInstanceInfo(ctx context.Context, domain string) (*InstanceInfo, error) {
	c.logger.WithField("domain", domain).Debug("Getting instance info")

	endpoint := fmt.Sprintf("https://%s/api/v1/federation/info", domain)
	var response InstanceInfo
	if err := c.sendRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get instance info: %w", err)
	}

	return &response, nil
}

// PingInstance sends a ping to verify instance connectivity
func (c *FederationClient) PingInstance(ctx context.Context, instance *FederatedInstance) error {
	c.logger.WithField("instance", instance.Domain).Debug("Pinging instance")

	endpoint := fmt.Sprintf("https://%s/api/v1/federation/ping", instance.Domain)

	// Create ping request with timestamp
	pingRequest := map[string]interface{}{
		"domain":    c.config.Domain,
		"timestamp": time.Now().Unix(),
	}

	// Sign the ping request
	requestData, err := json.Marshal(pingRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal ping request: %w", err)
	}

	signature, err := c.keys.Sign(requestData)
	if err != nil {
		return fmt.Errorf("failed to sign ping request: %w", err)
	}

	pingRequest["signature"] = string(signature)

	// Send ping
	var response map[string]interface{}
	if err := c.sendRequest(ctx, "POST", endpoint, pingRequest, &response); err != nil {
		return fmt.Errorf("failed to ping instance: %w", err)
	}

	// Verify pong response
	if pong, ok := response["pong"].(bool); !ok || !pong {
		return fmt.Errorf("invalid ping response")
	}

	c.logger.WithField("instance", instance.Domain).Debug("Successfully pinged instance")
	return nil
}

// sendRequest sends an HTTP request to a federated instance
func (c *FederationClient) sendRequest(ctx context.Context, method, url string, body interface{}, response interface{}) error {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Caslink-Federation/1.0")
	req.Header.Set("X-Federation-Domain", c.config.Domain)

	// Add authentication header with our public key
	publicKeyPEM, err := c.keys.GetPublicKeyPEM()
	if err == nil {
		req.Header.Set("X-Federation-Public-Key", publicKeyPEM)
	}

	// Send request with retry logic
	var resp *http.Response
	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		resp, err = c.client.Do(req)
		if err == nil {
			break
		}

		if attempt < c.config.RetryAttempts {
			backoff := time.Duration(attempt+1) * c.config.RetryBackoff
			c.logger.WithFields(logrus.Fields{
				"attempt": attempt + 1,
				"backoff": backoff,
				"error":   err,
			}).Debug("Request failed, retrying")

			time.Sleep(backoff)
		}
	}

	if err != nil {
		return fmt.Errorf("request failed after %d attempts: %w", c.config.RetryAttempts+1, err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	// Decode response if needed
	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// BatchShareURLs shares multiple URLs with an instance efficiently
func (c *FederationClient) BatchShareURLs(ctx context.Context, instance *FederatedInstance, urls []map[string]interface{}) error {
	c.logger.WithFields(logrus.Fields{
		"instance": instance.Domain,
		"count":    len(urls),
	}).Debug("Batch sharing URLs with instance")

	// Prepare batch share request
	batchRequest := struct {
		URLs      []URLShareRequest `json:"urls"`
		Timestamp int64             `json:"timestamp"`
		Signature string            `json:"signature"`
	}{
		URLs:      make([]URLShareRequest, len(urls)),
		Timestamp: time.Now().Unix(),
	}

	// Convert URLs to share requests
	for i, urlData := range urls {
		shareReq := URLShareRequest{
			URL:       urlData["original_url"].(string),
			ShortCode: urlData["short_code"].(string),
			CreatedAt: time.Now(),
			Metadata:  make(map[string]string),
		}

		if title, ok := urlData["title"].(string); ok {
			shareReq.Title = title
		}

		if description, ok := urlData["description"].(string); ok {
			shareReq.Description = description
		}

		batchRequest.URLs[i] = shareReq
	}

	// Sign the batch request
	requestData, err := json.Marshal(batchRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal batch request: %w", err)
	}

	signature, err := c.keys.Sign(requestData)
	if err != nil {
		return fmt.Errorf("failed to sign batch request: %w", err)
	}

	batchRequest.Signature = string(signature)

	// Send batch request
	endpoint := fmt.Sprintf("https://%s/api/v1/federation/batch-share", instance.Domain)
	if err := c.sendRequest(ctx, "POST", endpoint, batchRequest, nil); err != nil {
		return fmt.Errorf("failed to batch share URLs: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"instance": instance.Domain,
		"count":    len(urls),
	}).Info("Successfully batch shared URLs with instance")

	return nil
}

// GetInstanceStatus retrieves the status of a federated instance
func (c *FederationClient) GetInstanceStatus(ctx context.Context, instance *FederatedInstance) (*InstanceStatus, error) {
	c.logger.WithField("instance", instance.Domain).Debug("Getting instance status")

	endpoint := fmt.Sprintf("https://%s/api/v1/federation/status", instance.Domain)
	var response InstanceStatus
	if err := c.sendRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get instance status: %w", err)
	}

	return &response, nil
}

// InstanceStatus represents the status of a federated instance
type InstanceStatus struct {
	Online        bool      `json:"online"`
	LastSeen      time.Time `json:"last_seen"`
	Version       string    `json:"version"`
	URLCount      int64     `json:"url_count"`
	FederatedURLs int64     `json:"federated_urls"`
	Uptime        int64     `json:"uptime"`
	Load          float64   `json:"load,omitempty"`
}

// TestConnection tests the connection to a federated instance
func (c *FederationClient) TestConnection(ctx context.Context, domain string) error {
	c.logger.WithField("domain", domain).Debug("Testing connection to instance")

	endpoint := fmt.Sprintf("https://%s/.well-known/caslink", domain)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	req.Header.Set("User-Agent", "Caslink-Federation/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("connection test failed with status %d", resp.StatusCode)
	}

	var wellKnown WellKnownResponse
	if err := json.NewDecoder(resp.Body).Decode(&wellKnown); err != nil {
		return fmt.Errorf("failed to decode well-known response: %w", err)
	}

	if !wellKnown.Federation.Enabled {
		return fmt.Errorf("federation not enabled on target instance")
	}

	c.logger.WithField("domain", domain).Info("Connection test successful")
	return nil
}