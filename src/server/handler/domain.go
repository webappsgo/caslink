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
	orgService    *service.OrgService
}

// NewDomainHandler creates a new domain handler
func NewDomainHandler(domainService *service.DomainService, authService *service.AuthService, orgService *service.OrgService) *DomainHandler {
	return &DomainHandler{
		domainService: domainService,
		authService:   authService,
		orgService:    orgService,
	}
}

// ListUserDomains lists all custom domains for the current user.
func (h *DomainHandler) ListUserDomains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}

	domains, err := h.domainService.GetUserDomains(ctx, user.ID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load domains"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"domains": domains,
	})
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
// Checks for the required DNS TXT record and marks the domain verified on success.
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

// ListOrgDomains lists all custom domains for an organization.
func (h *DomainHandler) ListOrgDomains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")

	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Organization not found"})
		return
	}

	_, role, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || role == "" {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Not a member of this organization"})
		return
	}

	domains, err := h.domainService.GetOrgDomains(ctx, org.ID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load domains"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"domains": domains,
	})
}

// AddOrgDomain adds a custom domain for an organization.
func (h *DomainHandler) AddOrgDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")

	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Organization not found"})
		return
	}

	_, role, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || (role != "owner" && role != "admin") {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Owner or admin role required"})
		return
	}

	var req model.AddDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	domain, err := h.domainService.AddDomain(ctx, "org", org.ID, req.Domain)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"ok":     true,
		"domain": domain,
	})
}

