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

// VerifyUserDomain triggers domain verification
func (h *DomainHandler) VerifyUserDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	domain := chi.URLParam(r, "domain")

	// Get user from session
	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
		return
	}

	// Verify domain ownership and trigger verification per PART 35
	err := h.domainService.VerifyDomain(ctx, "user", user.ID, domain)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Verification triggered for %s", domain),
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

	// Note: Would look up org ID from slug and add domain
	// For now, return placeholder response
	respondJSON(w, http.StatusNotImplemented, map[string]interface{}{
		"error": fmt.Sprintf("Adding domains for organization %s not yet fully implemented", slug),

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"message": "Domain added (placeholder)",
	})
}

// getUserFromRequest is a helper to get user from middleware context
func getUserFromRequest(r *http.Request) (*service.User, bool) {
type contextKey string
user, ok := r.Context().Value(contextKey("user")).(*service.User)
return user, ok
}
