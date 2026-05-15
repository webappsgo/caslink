package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/server/model"
	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// OrgHandler handles organization operations
type OrgHandler struct {
	orgService  *service.OrgService
	authService *service.AuthService
}

// NewOrgHandler creates a new organization handler
func NewOrgHandler(orgService *service.OrgService, authService *service.AuthService) *OrgHandler {
	return &OrgHandler{
		orgService:  orgService,
		authService: authService,
	}
}

// ListOrgs renders the list of user's organizations
func (h *OrgHandler) ListOrgs(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	
	// Render placeholder per PART 17
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>Organizations</h1><p>User: %s</p><p>Organization list will be implemented per PART 17</p>", user.Username)
}

// CreateOrgPage renders the create organization page
func (h *OrgHandler) CreateOrgPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "<h1>Create Organization</h1><p>Create org form will be implemented per PART 17</p>")
}

// CreateOrg handles organization creation
func (h *OrgHandler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user from session (authentication middleware sets this)
	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
		return
	}

	// Parse request
	var req model.CreateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	// Validate name
	if len(req.Name) < 3 || len(req.Name) > 100 {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Organization name must be between 3 and 100 characters",
		})
		return
	}

	// Create organization
	org, err := h.orgService.CreateOrganization(ctx, user.ID, req.Name, req.Slug)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Return created organization
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"organization": org,
	})
}

// OrgDashboard renders the organization dashboard
func (h *OrgHandler) OrgDashboard(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>Organization: %s</h1><p>Dashboard will be implemented per PART 17</p>", slug)
}

// OrgSettings renders the organization settings page
func (h *OrgHandler) OrgSettings(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>Organization Settings: %s</h1><p>Settings will be implemented per PART 17</p>", slug)
}

// OrgMembers renders the organization members page
func (h *OrgHandler) OrgMembers(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>Organization Members: %s</h1><p>Members list will be implemented per PART 17</p>", slug)
}

