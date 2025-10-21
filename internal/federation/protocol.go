package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// ProtocolHandler implements the federation protocol
type ProtocolHandler struct {
	db     *db.DB
	config *config.FederationConfig
	logger *logrus.Logger
}

// FederationMessage represents a federation protocol message
type FederationMessage struct {
	Version   string                 `json:"version"`
	Type      string                 `json:"type"`
	From      string                 `json:"from"`
	To        string                 `json:"to"`
	Timestamp int64                  `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
	Signature string                 `json:"signature"`
}

// URLAnnouncement represents a URL announcement message
type URLAnnouncement struct {
	URL         string            `json:"url"`
	ShortCode   string            `json:"short_code"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// InstanceAnnouncement represents an instance announcement message
type InstanceAnnouncement struct {
	Domain       string            `json:"domain"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Version      string            `json:"version"`
	PublicKey    string            `json:"public_key"`
	Capabilities []string          `json:"capabilities"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// SyncRequest represents a synchronization request
type SyncRequest struct {
	RequestID string     `json:"request_id"`
	Since     *time.Time `json:"since,omitempty"`
	Until     *time.Time `json:"until,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Filter    *SyncFilter `json:"filter,omitempty"`
}

// SyncFilter represents filters for synchronization
type SyncFilter struct {
	Tags         []string `json:"tags,omitempty"`
	MinClicks    int      `json:"min_clicks,omitempty"`
	OnlyPublic   bool     `json:"only_public"`
	ExcludeBots  bool     `json:"exclude_bots"`
}

// SyncResponse represents a synchronization response
type SyncResponse struct {
	RequestID string           `json:"request_id"`
	URLs      []URLAnnouncement `json:"urls"`
	NextToken string           `json:"next_token,omitempty"`
	Total     int              `json:"total"`
	HasMore   bool             `json:"has_more"`
}

// MessageType constants
const (
	MessageTypeURLAnnouncement      = "url_announcement"
	MessageTypeInstanceAnnouncement = "instance_announcement"
	MessageTypeSyncRequest          = "sync_request"
	MessageTypeSyncResponse         = "sync_response"
	MessageTypePing                 = "ping"
	MessageTypePong                 = "pong"
	MessageTypeError                = "error"
)

// Protocol version
const ProtocolVersion = "1.0"

// NewProtocolHandler creates a new protocol handler
func NewProtocolHandler(database *db.DB, cfg *config.FederationConfig, logger *logrus.Logger) (*ProtocolHandler, error) {
	return &ProtocolHandler{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// CreateURLAnnouncement creates a URL announcement message
func (p *ProtocolHandler) CreateURLAnnouncement(url *FederatedURL, targetDomain string) (*FederationMessage, error) {
	announcement := URLAnnouncement{
		URL:       url.OriginalURL,
		ShortCode: url.ShortCode,
		Title:     url.Title,
		CreatedAt: url.CreatedAt,
		Metadata:  make(map[string]string),
	}

	payload := map[string]interface{}{
		"announcement": announcement,
	}

	message := &FederationMessage{
		Version:   ProtocolVersion,
		Type:      MessageTypeURLAnnouncement,
		From:      p.config.Domain,
		To:        targetDomain,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}

	return message, nil
}

// CreateInstanceAnnouncement creates an instance announcement message
func (p *ProtocolHandler) CreateInstanceAnnouncement(publicKeyPEM, targetDomain string) (*FederationMessage, error) {
	announcement := InstanceAnnouncement{
		Domain:      p.config.Domain,
		Name:        "Caslink URL Shortener",
		Description: "Self-hosted URL shortener with federation support",
		Version:     "1.0.0",
		PublicKey:   publicKeyPEM,
		Capabilities: []string{
			"url_sharing",
			"batch_sharing",
			"instance_discovery",
			"signature_verification",
		},
		Metadata: map[string]string{
			"federation_enabled": fmt.Sprintf("%t", p.config.Enabled),
			"share_public_urls":  fmt.Sprintf("%t", p.config.SharePublicURLs),
		},
	}

	payload := map[string]interface{}{
		"announcement": announcement,
	}

	message := &FederationMessage{
		Version:   ProtocolVersion,
		Type:      MessageTypeInstanceAnnouncement,
		From:      p.config.Domain,
		To:        targetDomain,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}

	return message, nil
}

// CreateSyncRequest creates a synchronization request message
func (p *ProtocolHandler) CreateSyncRequest(targetDomain string, since *time.Time, limit int) (*FederationMessage, error) {
	requestID := fmt.Sprintf("sync_%d_%s", time.Now().UnixNano(), p.config.Domain)

	syncReq := SyncRequest{
		RequestID: requestID,
		Since:     since,
		Limit:     limit,
		Filter: &SyncFilter{
			OnlyPublic:  true,
			ExcludeBots: true,
		},
	}

	payload := map[string]interface{}{
		"request": syncReq,
	}

	message := &FederationMessage{
		Version:   ProtocolVersion,
		Type:      MessageTypeSyncRequest,
		From:      p.config.Domain,
		To:        targetDomain,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}

	return message, nil
}

// CreateSyncResponse creates a synchronization response message
func (p *ProtocolHandler) CreateSyncResponse(requestID, targetDomain string, urls []URLAnnouncement) (*FederationMessage, error) {
	syncResp := SyncResponse{
		RequestID: requestID,
		URLs:      urls,
		Total:     len(urls),
		HasMore:   len(urls) >= p.config.MaxURLsPerSync,
	}

	payload := map[string]interface{}{
		"response": syncResp,
	}

	message := &FederationMessage{
		Version:   ProtocolVersion,
		Type:      MessageTypeSyncResponse,
		From:      p.config.Domain,
		To:        targetDomain,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}

	return message, nil
}

// CreatePingMessage creates a ping message
func (p *ProtocolHandler) CreatePingMessage(targetDomain string) (*FederationMessage, error) {
	payload := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"challenge": fmt.Sprintf("ping_%d", time.Now().UnixNano()),
	}

	message := &FederationMessage{
		Version:   ProtocolVersion,
		Type:      MessageTypePing,
		From:      p.config.Domain,
		To:        targetDomain,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}

	return message, nil
}

// CreatePongMessage creates a pong response message
func (p *ProtocolHandler) CreatePongMessage(targetDomain, challenge string) (*FederationMessage, error) {
	payload := map[string]interface{}{
		"timestamp":        time.Now().Unix(),
		"challenge":        challenge,
		"challenge_response": fmt.Sprintf("pong_%s", challenge),
	}

	message := &FederationMessage{
		Version:   ProtocolVersion,
		Type:      MessageTypePong,
		From:      p.config.Domain,
		To:        targetDomain,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}

	return message, nil
}

// CreateErrorMessage creates an error message
func (p *ProtocolHandler) CreateErrorMessage(targetDomain, errorCode, errorMessage string) (*FederationMessage, error) {
	payload := map[string]interface{}{
		"error_code":    errorCode,
		"error_message": errorMessage,
		"timestamp":     time.Now().Unix(),
	}

	message := &FederationMessage{
		Version:   ProtocolVersion,
		Type:      MessageTypeError,
		From:      p.config.Domain,
		To:        targetDomain,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}

	return message, nil
}

// ParseMessage parses a federation message from JSON
func (p *ProtocolHandler) ParseMessage(data []byte) (*FederationMessage, error) {
	var message FederationMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return nil, fmt.Errorf("failed to parse federation message: %w", err)
	}

	// Validate message structure
	if err := p.validateMessage(&message); err != nil {
		return nil, fmt.Errorf("invalid message: %w", err)
	}

	return &message, nil
}

// SerializeMessage serializes a federation message to JSON
func (p *ProtocolHandler) SerializeMessage(message *FederationMessage) ([]byte, error) {
	// Validate message before serialization
	if err := p.validateMessage(message); err != nil {
		return nil, fmt.Errorf("invalid message: %w", err)
	}

	data, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize federation message: %w", err)
	}

	return data, nil
}

// ProcessMessage processes an incoming federation message
func (p *ProtocolHandler) ProcessMessage(ctx context.Context, message *FederationMessage) error {
	p.logger.WithFields(logrus.Fields{
		"type": message.Type,
		"from": message.From,
		"to":   message.To,
	}).Debug("Processing federation message")

	switch message.Type {
	case MessageTypeURLAnnouncement:
		return p.processURLAnnouncement(ctx, message)
	case MessageTypeInstanceAnnouncement:
		return p.processInstanceAnnouncement(ctx, message)
	case MessageTypeSyncRequest:
		return p.processSyncRequest(ctx, message)
	case MessageTypeSyncResponse:
		return p.processSyncResponse(ctx, message)
	case MessageTypePing:
		return p.processPing(ctx, message)
	case MessageTypePong:
		return p.processPong(ctx, message)
	case MessageTypeError:
		return p.processError(ctx, message)
	default:
		return fmt.Errorf("unknown message type: %s", message.Type)
	}
}

// processURLAnnouncement processes a URL announcement message
func (p *ProtocolHandler) processURLAnnouncement(ctx context.Context, message *FederationMessage) error {
	announcementData, ok := message.Payload["announcement"]
	if !ok {
		return fmt.Errorf("missing announcement in payload")
	}

	// Convert to URLAnnouncement
	announcementBytes, _ := json.Marshal(announcementData)
	var announcement URLAnnouncement
	if err := json.Unmarshal(announcementBytes, &announcement); err != nil {
		return fmt.Errorf("failed to parse URL announcement: %w", err)
	}

	// Create federated URL
	federatedURL := &FederatedURL{
		ID:             p.generateMessageID(),
		OriginalID:     announcement.ShortCode,
		SourceInstance: message.From,
		OriginalURL:    announcement.URL,
		ShortCode:      announcement.ShortCode,
		Title:          announcement.Title,
		CreatedAt:      announcement.CreatedAt,
		SyncedAt:       time.Now(),
	}

	// Save to database
	if err := p.saveFederatedURL(ctx, federatedURL); err != nil {
		return fmt.Errorf("failed to save federated URL: %w", err)
	}

	p.logger.WithFields(logrus.Fields{
		"from":       message.From,
		"short_code": announcement.ShortCode,
		"url":        announcement.URL,
	}).Info("Processed URL announcement")

	return nil
}

// processInstanceAnnouncement processes an instance announcement message
func (p *ProtocolHandler) processInstanceAnnouncement(ctx context.Context, message *FederationMessage) error {
	announcementData, ok := message.Payload["announcement"]
	if !ok {
		return fmt.Errorf("missing announcement in payload")
	}

	// Convert to InstanceAnnouncement
	announcementBytes, _ := json.Marshal(announcementData)
	var announcement InstanceAnnouncement
	if err := json.Unmarshal(announcementBytes, &announcement); err != nil {
		return fmt.Errorf("failed to parse instance announcement: %w", err)
	}

	// Create or update federated instance
	instance := &FederatedInstance{
		ID:           p.generateMessageID(),
		Domain:       announcement.Domain,
		PublicKey:    announcement.PublicKey,
		DiscoveredAt: time.Now(),
		Active:       true,
		SyncEnabled:  true,
	}

	// Save to database
	if err := p.saveOrUpdateInstance(ctx, instance); err != nil {
		return fmt.Errorf("failed to save instance: %w", err)
	}

	p.logger.WithFields(logrus.Fields{
		"domain":  announcement.Domain,
		"name":    announcement.Name,
		"version": announcement.Version,
	}).Info("Processed instance announcement")

	return nil
}

// processSyncRequest processes a sync request message
func (p *ProtocolHandler) processSyncRequest(ctx context.Context, message *FederationMessage) error {
	requestData, ok := message.Payload["request"]
	if !ok {
		return fmt.Errorf("missing request in payload")
	}

	// Convert to SyncRequest
	requestBytes, _ := json.Marshal(requestData)
	var syncReq SyncRequest
	if err := json.Unmarshal(requestBytes, &syncReq); err != nil {
		return fmt.Errorf("failed to parse sync request: %w", err)
	}

	// Get URLs to share
	urls, err := p.getURLsForSync(ctx, &syncReq)
	if err != nil {
		return fmt.Errorf("failed to get URLs for sync: %w", err)
	}

	// Create response message (this would typically be sent back to the requester)
	response, err := p.CreateSyncResponse(syncReq.RequestID, message.From, urls)
	if err != nil {
		return fmt.Errorf("failed to create sync response: %w", err)
	}

	// Log the sync request processing
	p.logger.WithFields(logrus.Fields{
		"from":       message.From,
		"request_id": syncReq.RequestID,
		"url_count":  len(urls),
	}).Info("Processed sync request")

	// Store response for sending (this would be handled by the transport layer)
	return p.storePendingResponse(ctx, response)
}

// processSyncResponse processes a sync response message
func (p *ProtocolHandler) processSyncResponse(ctx context.Context, message *FederationMessage) error {
	responseData, ok := message.Payload["response"]
	if !ok {
		return fmt.Errorf("missing response in payload")
	}

	// Convert to SyncResponse
	responseBytes, _ := json.Marshal(responseData)
	var syncResp SyncResponse
	if err := json.Unmarshal(responseBytes, &syncResp); err != nil {
		return fmt.Errorf("failed to parse sync response: %w", err)
	}

	// Process each URL in the response
	for _, urlAnnouncement := range syncResp.URLs {
		federatedURL := &FederatedURL{
			ID:             p.generateMessageID(),
			OriginalID:     urlAnnouncement.ShortCode,
			SourceInstance: message.From,
			OriginalURL:    urlAnnouncement.URL,
			ShortCode:      urlAnnouncement.ShortCode,
			Title:          urlAnnouncement.Title,
			CreatedAt:      urlAnnouncement.CreatedAt,
			SyncedAt:       time.Now(),
		}

		if err := p.saveFederatedURL(ctx, federatedURL); err != nil {
			p.logger.WithError(err).WithField("url", urlAnnouncement.ShortCode).Warn("Failed to save federated URL from sync response")
			continue
		}
	}

	p.logger.WithFields(logrus.Fields{
		"from":       message.From,
		"request_id": syncResp.RequestID,
		"url_count":  len(syncResp.URLs),
	}).Info("Processed sync response")

	return nil
}

// processPing processes a ping message
func (p *ProtocolHandler) processPing(ctx context.Context, message *FederationMessage) error {
	challenge, ok := message.Payload["challenge"].(string)
	if !ok {
		challenge = "unknown"
	}

	// Create pong response
	pong, err := p.CreatePongMessage(message.From, challenge)
	if err != nil {
		return fmt.Errorf("failed to create pong message: %w", err)
	}

	// Store response for sending
	return p.storePendingResponse(ctx, pong)
}

// processPong processes a pong message
func (p *ProtocolHandler) processPong(ctx context.Context, message *FederationMessage) error {
	p.logger.WithField("from", message.From).Debug("Received pong message")
	// Update instance last seen time or connectivity status
	return p.updateInstanceLastSeen(ctx, message.From)
}

// processError processes an error message
func (p *ProtocolHandler) processError(ctx context.Context, message *FederationMessage) error {
	errorCode, _ := message.Payload["error_code"].(string)
	errorMessage, _ := message.Payload["error_message"].(string)

	p.logger.WithFields(logrus.Fields{
		"from":          message.From,
		"error_code":    errorCode,
		"error_message": errorMessage,
	}).Warn("Received federation error message")

	return nil
}

// validateMessage validates a federation message
func (p *ProtocolHandler) validateMessage(message *FederationMessage) error {
	if message.Version == "" {
		return fmt.Errorf("missing version")
	}

	if message.Type == "" {
		return fmt.Errorf("missing type")
	}

	if message.From == "" {
		return fmt.Errorf("missing from")
	}

	if message.Timestamp == 0 {
		return fmt.Errorf("missing timestamp")
	}

	// Check timestamp is not too old or too far in the future
	now := time.Now().Unix()
	if message.Timestamp < now-3600 || message.Timestamp > now+300 {
		return fmt.Errorf("invalid timestamp")
	}

	return nil
}

// Helper methods

// saveFederatedURL saves a federated URL to the database
func (p *ProtocolHandler) saveFederatedURL(ctx context.Context, url *FederatedURL) error {
	query := `
		INSERT OR IGNORE INTO federated_urls (id, original_id, source_instance, original_url, short_code, title, created_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := p.db.ExecContext(ctx, query,
		url.ID, url.OriginalID, url.SourceInstance, url.OriginalURL,
		url.ShortCode, url.Title, url.CreatedAt, url.SyncedAt,
	)

	return err
}

// saveOrUpdateInstance saves or updates a federated instance
func (p *ProtocolHandler) saveOrUpdateInstance(ctx context.Context, instance *FederatedInstance) error {
	query := `
		INSERT OR REPLACE INTO federation_instances (id, domain, public_key, discovered_at, active, blocked, sync_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := p.db.ExecContext(ctx, query,
		instance.ID, instance.Domain, instance.PublicKey,
		instance.DiscoveredAt, instance.Active, instance.Blocked, instance.SyncEnabled,
	)

	return err
}

// getURLsForSync retrieves URLs matching sync request criteria
func (p *ProtocolHandler) getURLsForSync(ctx context.Context, syncReq *SyncRequest) ([]URLAnnouncement, error) {
	query := `
		SELECT id, original_url, short_code, title, created_at
		FROM urls
		WHERE active = true AND user_id IS NULL` // Only public URLs

	args := []interface{}{}

	if syncReq.Since != nil {
		query += " AND created_at > ?"
		args = append(args, *syncReq.Since)
	}

	if syncReq.Until != nil {
		query += " AND created_at <= ?"
		args = append(args, *syncReq.Until)
	}

	limit := syncReq.Limit
	if limit <= 0 || limit > p.config.MaxURLsPerSync {
		limit = p.config.MaxURLsPerSync
	}

	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query URLs: %w", err)
	}
	defer rows.Close()

	var urls []URLAnnouncement
	for rows.Next() {
		var id, originalURL, shortCode, title string
		var createdAt time.Time

		err := rows.Scan(&id, &originalURL, &shortCode, &title, &createdAt)
		if err != nil {
			continue
		}

		announcement := URLAnnouncement{
			URL:       originalURL,
			ShortCode: shortCode,
			Title:     title,
			CreatedAt: createdAt,
			Metadata:  make(map[string]string),
		}

		urls = append(urls, announcement)
	}

	return urls, nil
}

// storePendingResponse stores a response message for sending
func (p *ProtocolHandler) storePendingResponse(ctx context.Context, message *FederationMessage) error {
	// This is a simplified implementation
	// In a real system, this would queue the message for sending
	p.logger.WithFields(logrus.Fields{
		"type": message.Type,
		"to":   message.To,
	}).Debug("Stored pending federation response")

	return nil
}

// updateInstanceLastSeen updates the last seen time for an instance
func (p *ProtocolHandler) updateInstanceLastSeen(ctx context.Context, domain string) error {
	query := "UPDATE federation_instances SET last_sync = ? WHERE domain = ?"
	_, err := p.db.ExecContext(ctx, query, time.Now(), domain)
	return err
}

// generateMessageID generates a unique message ID
func (p *ProtocolHandler) generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}