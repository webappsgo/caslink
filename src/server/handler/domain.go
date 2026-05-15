package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/server/model"
	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// DomainHandler handles custom domain operations
type DomainHandler struct {
	domainService *service.DomainService
	authService   *service.AuthService
}

// NewDomainHandler creates a new domain handler
func NewDomainHandler(domainService *service.DomainService, authService *service.AuthService) *DomainHandler {
	return &DomainHandler{
		domainService: domainService,
		authService:   authService,
	}
}

// ListUserDomains lists all domains for the current user
func (h *DomainHandler) ListUserDomains(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>Custom Domains</h1><p>User: %s</p><p>Domain list will be implemented per PART 17</p>", user.Username)
}

// AddUserDomain handles adding a custom domain for a user
func (h *DomainHandler) AddUserDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user from session
	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
		return
	}

	// Parse request
	var req model.AddDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	// Add domain
	domain, err := h.domainService.AddDomain(ctx, "user", user.ID, req.Domain)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Return created domain
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"domain":  domain,
	})
}

// VerifyUserDomain triggers domain verification for a user-owned domain.
// The actual DNS-TXT verification logic (PART 35) is not implemented yet —
// this endpoint looks up the requested domain to confirm ownership, then
// invokes the service which currently marks the row verified as a
// placeholder. Tracked in TODO.AI.md under "Custom Domains - DNS Verification".
func (h *DomainHandler) VerifyUserDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	domainName := chi.URLParam(r, "domain")

	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
		return
	}

	// Resolve domain ID and confirm the caller owns it.
	domains, err := h.domainService.GetUserDomains(ctx, user.ID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load domains",
		})
		return
	}
	var domainID int64
	for _, d := range domains {
		if d.Domain == domainName {
			domainID = d.ID
			break
		}
	}
	if domainID == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"error": "domain not found for this user",
		})
		return
	}

	if err := h.domainService.VerifyDomain(ctx, domainID); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("verification triggered for %s", domainName),
	})
}

// ListOrgDomains lists all domains for an organization
func (h *DomainHandler) ListOrgDomains(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>Custom Domains for %s</h1><p>Domain list will be implemented per PART 17</p>", slug)
}

// AddOrgDomain handles adding a custom domain for an organization
func (h *DomainHandler) AddOrgDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")

	// Get user from session
	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
		return
	}

	// Note: Org membership verification would happen here in production
	// For now, allow any authenticated user (will be restricted per PART 23)
	_ = user

	// Parse request
	var req model.AddDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}
	_ = req
	_ = ctx

	// Adding an org-scoped domain is not yet wired through the org-membership
	// check or domain service. Return 501 until the org domain handler is
	// implemented end-to-end (tracked in TODO.AI.md).
	respondJSON(w, http.StatusNotImplemented, map[string]interface{}{
		"error": fmt.Sprintf("adding domains for organization %s not yet implemented", slug),
	})
}

