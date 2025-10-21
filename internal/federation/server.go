package federation

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// FederationServer handles incoming federation requests
type FederationServer struct {
	db     *db.DB
	config *config.FederationConfig
	logger *logrus.Logger
	keys   *KeyManager
}

// ShareRequestHandler represents the structure for incoming URL share requests
type ShareRequestHandler struct {
	URL         string            `json:"url"`
	ShortCode   string            `json:"short_code"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Signature   string            `json:"signature"`
	FromDomain  string            `json:"from_domain"`
}

// SyncRequestHandler represents the structure for URL sync requests
type SyncRequestHandler struct {
	Since     *time.Time `json:"since,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Signature string     `json:"signature"`
	FromDomain string    `json:"from_domain"`
}

// BatchShareRequestHandler represents the structure for batch URL share requests
type BatchShareRequestHandler struct {
	URLs      []ShareRequestHandler `json:"urls"`
	Timestamp int64                 `json:"timestamp"`
	Signature string                `json:"signature"`
	FromDomain string               `json:"from_domain"`
}

// NewFederationServer creates a new federation server
func NewFederationServer(database *db.DB, cfg *config.FederationConfig, logger *logrus.Logger, keyManager *KeyManager) (*FederationServer, error) {
	return &FederationServer{
		db:     database,
		config: cfg,
		logger: logger,
		keys:   keyManager,
	}, nil
}

// RegisterRoutes registers federation API routes
func (s *FederationServer) RegisterRoutes(router *mux.Router) {
	// Create federation subrouter
	fedRouter := router.PathPrefix("/api/v1/federation").Subrouter()

	// Public endpoints
	fedRouter.HandleFunc("/info", s.handleInfo).Methods("GET")
	fedRouter.HandleFunc("/ping", s.handlePing).Methods("POST")
	fedRouter.HandleFunc("/.well-known/caslink", s.handleWellKnown).Methods("GET")

	// Authenticated endpoints
	fedRouter.HandleFunc("/share", s.authenticateRequest(s.handleShare)).Methods("POST")
	fedRouter.HandleFunc("/sync", s.authenticateRequest(s.handleSync)).Methods("POST")
	fedRouter.HandleFunc("/batch-share", s.authenticateRequest(s.handleBatchShare)).Methods("POST")
	fedRouter.HandleFunc("/verify", s.authenticateRequest(s.handleVerify)).Methods("POST")
	fedRouter.HandleFunc("/status", s.authenticateRequest(s.handleStatus)).Methods("GET")
}

// handleInfo returns instance information
func (s *FederationServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling federation info request")

	publicKeyPEM, err := s.keys.GetPublicKeyPEM()
	if err != nil {
		s.logger.WithError(err).Error("Failed to get public key")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	info := InstanceInfo{
		Domain:        s.config.Domain,
		Name:          "Caslink URL Shortener",
		Description:   "Self-hosted URL shortener with federation support",
		Version:       "1.0.0",
		PublicKey:     publicKeyPEM,
		FederationURL: fmt.Sprintf("https://%s/api/v1/federation", s.config.Domain),
		Metadata: map[string]interface{}{
			"federation_enabled": s.config.Enabled,
			"share_public_urls":  s.config.SharePublicURLs,
			"max_urls_per_sync":  s.config.MaxURLsPerSync,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleWellKnown returns the .well-known/caslink response
func (s *FederationServer) handleWellKnown(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling well-known request")

	publicKeyPEM, err := s.keys.GetPublicKeyPEM()
	if err != nil {
		s.logger.WithError(err).Error("Failed to get public key")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := WellKnownResponse{
		Instance: InstanceInfo{
			Domain:      s.config.Domain,
			Name:        "Caslink URL Shortener",
			Description: "Self-hosted URL shortener with federation support",
			Version:     "1.0.0",
		},
		Federation: struct {
			Enabled   bool   `json:"enabled"`
			Version   string `json:"version"`
			Endpoint  string `json:"endpoint"`
			PublicKey string `json:"public_key"`
		}{
			Enabled:   s.config.Enabled,
			Version:   "1.0",
			Endpoint:  fmt.Sprintf("https://%s/api/v1/federation", s.config.Domain),
			PublicKey: publicKeyPEM,
		},
		Capabilities: []string{
			"url_sharing",
			"batch_sharing",
			"instance_discovery",
			"signature_verification",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handlePing handles ping requests from other instances
func (s *FederationServer) handlePing(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling ping request")

	var pingRequest map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&pingRequest); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify signature if present
	if signature, ok := pingRequest["signature"].(string); ok {
		fromDomain, _ := pingRequest["domain"].(string)
		if fromDomain != "" {
			// Verify the ping signature
			delete(pingRequest, "signature")
			requestData, _ := json.Marshal(pingRequest)

			if err := s.verifyRequestSignature(requestData, []byte(signature), fromDomain); err != nil {
				s.logger.WithError(err).Warn("Ping signature verification failed")
				http.Error(w, "Invalid signature", http.StatusUnauthorized)
				return
			}
		}
	}

	// Return pong response
	response := map[string]interface{}{
		"pong":      true,
		"timestamp": time.Now().Unix(),
		"domain":    s.config.Domain,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleShare handles URL sharing requests
func (s *FederationServer) handleShare(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling URL share request")

	var shareReq ShareRequestHandler
	if err := json.NewDecoder(r.Body).Decode(&shareReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get the requesting domain from headers or authentication
	shareReq.FromDomain = r.Header.Get("X-Federation-Domain")
	if shareReq.FromDomain == "" {
		http.Error(w, "Federation domain header required", http.StatusBadRequest)
		return
	}

	// Store the federated URL
	federatedURL := &FederatedURL{
		ID:             s.generateFederatedURLID(),
		OriginalID:     shareReq.ShortCode, // Use short code as original ID
		SourceInstance: shareReq.FromDomain,
		OriginalURL:    shareReq.URL,
		ShortCode:      shareReq.ShortCode,
		Title:          shareReq.Title,
		CreatedAt:      shareReq.CreatedAt,
		SyncedAt:       time.Now(),
	}

	if err := s.saveFederatedURL(r.Context(), federatedURL); err != nil {
		s.logger.WithError(err).Error("Failed to save federated URL")
		http.Error(w, "Failed to save URL", http.StatusInternalServerError)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"from_domain": shareReq.FromDomain,
		"short_code":  shareReq.ShortCode,
		"url":         shareReq.URL,
	}).Info("Saved federated URL")

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"message": "URL shared successfully",
		"id":      federatedURL.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSync handles URL synchronization requests
func (s *FederationServer) handleSync(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling URL sync request")

	var syncReq SyncRequestHandler
	if err := json.NewDecoder(r.Body).Decode(&syncReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get the requesting domain
	syncReq.FromDomain = r.Header.Get("X-Federation-Domain")
	if syncReq.FromDomain == "" {
		http.Error(w, "Federation domain header required", http.StatusBadRequest)
		return
	}

	// Set default limit if not specified
	if syncReq.Limit <= 0 || syncReq.Limit > s.config.MaxURLsPerSync {
		syncReq.Limit = s.config.MaxURLsPerSync
	}

	// Fetch URLs to share
	urls, err := s.getURLsToShare(r.Context(), syncReq.Since, syncReq.Limit)
	if err != nil {
		s.logger.WithError(err).Error("Failed to fetch URLs for sync")
		http.Error(w, "Failed to fetch URLs", http.StatusInternalServerError)
		return
	}

	// Convert to federated URLs
	federatedURLs := make([]FederatedURL, len(urls))
	for i, url := range urls {
		federatedURLs[i] = FederatedURL{
			ID:          url["id"].(string),
			OriginalID:  url["id"].(string),
			SourceInstance: s.config.Domain,
			OriginalURL: url["original_url"].(string),
			ShortCode:   url["short_code"].(string),
			Title:       getStringField(url, "title"),
			CreatedAt:   url["created_at"].(time.Time),
			SyncedAt:    time.Now(),
		}
	}

	response := URLSyncResponse{
		URLs:  federatedURLs,
		Total: len(federatedURLs),
	}

	s.logger.WithFields(logrus.Fields{
		"from_domain": syncReq.FromDomain,
		"count":       len(federatedURLs),
	}).Info("Provided URLs for sync")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleBatchShare handles batch URL sharing requests
func (s *FederationServer) handleBatchShare(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling batch URL share request")

	var batchReq BatchShareRequestHandler
	if err := json.NewDecoder(r.Body).Decode(&batchReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get the requesting domain
	batchReq.FromDomain = r.Header.Get("X-Federation-Domain")
	if batchReq.FromDomain == "" {
		http.Error(w, "Federation domain header required", http.StatusBadRequest)
		return
	}

	// Process each URL in the batch
	savedCount := 0
	for _, shareReq := range batchReq.URLs {
		federatedURL := &FederatedURL{
			ID:             s.generateFederatedURLID(),
			OriginalID:     shareReq.ShortCode,
			SourceInstance: batchReq.FromDomain,
			OriginalURL:    shareReq.URL,
			ShortCode:      shareReq.ShortCode,
			Title:          shareReq.Title,
			CreatedAt:      shareReq.CreatedAt,
			SyncedAt:       time.Now(),
		}

		if err := s.saveFederatedURL(r.Context(), federatedURL); err != nil {
			s.logger.WithError(err).WithField("short_code", shareReq.ShortCode).Warn("Failed to save federated URL in batch")
			continue
		}

		savedCount++
	}

	s.logger.WithFields(logrus.Fields{
		"from_domain": batchReq.FromDomain,
		"total":       len(batchReq.URLs),
		"saved":       savedCount,
	}).Info("Processed batch URL share")

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Processed %d URLs successfully", savedCount),
		"total":   len(batchReq.URLs),
		"saved":   savedCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleVerify handles instance verification requests
func (s *FederationServer) handleVerify(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling instance verification request")

	var verifyReq InstanceVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&verifyReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Verify the signature
	verifyData := struct {
		Domain    string `json:"domain"`
		PublicKey string `json:"public_key"`
		Timestamp int64  `json:"timestamp"`
	}{
		Domain:    verifyReq.Domain,
		PublicKey: verifyReq.PublicKey,
		Timestamp: verifyReq.Timestamp,
	}

	requestData, _ := json.Marshal(verifyData)
	if err := s.verifyRequestSignature(requestData, []byte(verifyReq.Signature), verifyReq.Domain); err != nil {
		s.logger.WithError(err).Warn("Instance verification signature failed")

		response := InstanceVerifyResponse{
			Verified: false,
			Message:  "Signature verification failed",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Verification successful
	response := InstanceVerifyResponse{
		Verified: true,
		Message:  "Instance verified successfully",
	}

	s.logger.WithField("domain", verifyReq.Domain).Info("Instance verification successful")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleStatus handles status requests
func (s *FederationServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("Handling status request")

	// Get instance statistics
	stats, err := s.getInstanceStats(r.Context())
	if err != nil {
		s.logger.WithError(err).Error("Failed to get instance stats")
		http.Error(w, "Failed to get status", http.StatusInternalServerError)
		return
	}

	status := InstanceStatus{
		Online:        true,
		LastSeen:      time.Now(),
		Version:       "1.0.0",
		URLCount:      stats.URLCount,
		FederatedURLs: stats.FederatedURLs,
		Uptime:        stats.Uptime,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// authenticateRequest middleware for federation request authentication
func (s *FederationServer) authenticateRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check for federation domain header
		fromDomain := r.Header.Get("X-Federation-Domain")
		if fromDomain == "" {
			http.Error(w, "Federation domain header required", http.StatusBadRequest)
			return
		}

		// Check for public key header
		publicKeyPEM := r.Header.Get("X-Federation-Public-Key")
		if publicKeyPEM == "" {
			http.Error(w, "Federation public key header required", http.StatusBadRequest)
			return
		}

		// Verify the public key format
		_, err := s.keys.ParsePublicKey(publicKeyPEM)
		if err != nil {
			s.logger.WithError(err).Warn("Invalid federation public key")
			http.Error(w, "Invalid public key", http.StatusBadRequest)
			return
		}

		// TODO: Additional authentication checks can be added here
		// such as checking if the instance is known and trusted

		next(w, r)
	}
}

// Helper methods

// saveFederatedURL saves a federated URL to the database
func (s *FederationServer) saveFederatedURL(ctx context.Context, url *FederatedURL) error {
	query := `
		INSERT OR IGNORE INTO federated_urls (id, original_id, source_instance, original_url, short_code, title, created_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		url.ID, url.OriginalID, url.SourceInstance, url.OriginalURL,
		url.ShortCode, url.Title, url.CreatedAt, url.SyncedAt,
	)

	return err
}

// getURLsToShare retrieves URLs that can be shared with federated instances
func (s *FederationServer) getURLsToShare(ctx context.Context, since *time.Time, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT id, original_url, short_code, title, created_at
		FROM urls
		WHERE active = true AND user_id IS NULL`  // Only share anonymous/public URLs

	args := []interface{}{}

	if since != nil {
		query += " AND created_at > ?"
		args = append(args, *since)
	}

	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query URLs: %w", err)
	}
	defer rows.Close()

	var urls []map[string]interface{}

	for rows.Next() {
		var id, originalURL, shortCode, title string
		var createdAt time.Time

		err := rows.Scan(&id, &originalURL, &shortCode, &title, &createdAt)
		if err != nil {
			continue
		}

		url := map[string]interface{}{
			"id":           id,
			"original_url": originalURL,
			"short_code":   shortCode,
			"title":        title,
			"created_at":   createdAt,
		}

		urls = append(urls, url)
	}

	return urls, nil
}

// getInstanceStats retrieves instance statistics
func (s *FederationServer) getInstanceStats(ctx context.Context) (*InstanceStats, error) {
	stats := &InstanceStats{}

	// Count total URLs
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM urls WHERE active = true").Scan(&stats.URLCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count URLs: %w", err)
	}

	// Count federated URLs
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM federated_urls").Scan(&stats.FederatedURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to count federated URLs: %w", err)
	}

	// Calculate uptime (simplified - should be tracked properly)
	stats.Uptime = time.Now().Unix() // Placeholder

	return stats, nil
}

// verifyRequestSignature verifies a request signature from another instance
func (s *FederationServer) verifyRequestSignature(data, signature []byte, fromDomain string) error {
	// Get the public key for the requesting instance
	instance, err := s.getInstanceByDomain(fromDomain)
	if err != nil {
		return fmt.Errorf("unknown instance: %w", err)
	}

	publicKey, err := s.keys.ParsePublicKey(instance.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}

	return s.keys.Verify(data, signature, publicKey)
}

// getInstanceByDomain retrieves an instance by domain
func (s *FederationServer) getInstanceByDomain(domain string) (*FederatedInstance, error) {
	query := `
		SELECT id, domain, public_key, discovered_at, last_sync, active, blocked, sync_enabled
		FROM federation_instances
		WHERE domain = ? AND active = true`

	var instance FederatedInstance
	row := s.db.QueryRowContext(context.Background(), query, domain)

	err := row.Scan(
		&instance.ID, &instance.Domain, &instance.PublicKey,
		&instance.DiscoveredAt, &instance.LastSync,
		&instance.Active, &instance.Blocked, &instance.SyncEnabled,
	)

	if err != nil {
		return nil, fmt.Errorf("instance not found: %w", err)
	}

	return &instance, nil
}

// generateFederatedURLID generates a unique ID for federated URLs
func (s *FederationServer) generateFederatedURLID() string {
	return fmt.Sprintf("fed_%d", time.Now().UnixNano())
}

// getStringField safely gets a string field from a map
func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// InstanceStats represents instance statistics
type InstanceStats struct {
	URLCount      int64 `json:"url_count"`
	FederatedURLs int64 `json:"federated_urls"`
	Uptime        int64 `json:"uptime"`
}