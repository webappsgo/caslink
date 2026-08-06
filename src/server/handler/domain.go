package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/service"
)

// DomainHandler handles custom domain operations
type DomainHandler struct {
	domainService *service.DomainService
	authService   *service.AuthService
	orgService    *service.OrgService
	auditService  *service.AuditService
}

// NewDomainHandler creates a new domain handler
func NewDomainHandler(domainService *service.DomainService, authService *service.AuthService, orgService *service.OrgService, auditService *service.AuditService) *DomainHandler {
	return &DomainHandler{
		domainService: domainService,
		authService:   authService,
		orgService:    orgService,
		auditService:  auditService,
	}
}

// ListUserDomains lists all custom domains for the current user.
func (h *DomainHandler) ListUserDomains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	domains, err := h.domainService.GetUserDomains(ctx, user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load domains")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"domains": domains,
	})
}

// AddUserDomain handles adding a custom domain for a user
func (h *DomainHandler) AddUserDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user from session
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Parse request
	var req model.AddDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Add domain
	domain, err := h.domainService.AddDomain(ctx, "user", user.ID, req.Domain)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Return created domain with the DNS records the owner must configure.
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"domain":           domain,
		"dns_instructions": h.domainService.BuildDNSInstructions(ctx, domain),
	})
}

// VerifyUserDomain triggers domain verification for a user-owned domain.
// Checks for the required DNS TXT record and marks the domain verified on success.
func (h *DomainHandler) VerifyUserDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	domainName := chi.URLParam(r, "domain")

	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	// Resolve domain ID and confirm the caller owns it.
	domains, err := h.domainService.GetUserDomains(ctx, user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domains")
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
		respondError(w, http.StatusNotFound, "domain not found for this user")
		return
	}

	if err := h.domainService.VerifyDomain(ctx, domainID); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("verification triggered for %s", domainName),
	})
}

// ListOrgDomains lists all custom domains for an organization.
func (h *DomainHandler) ListOrgDomains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")

	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	_, role, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || role == "" {
		respondError(w, http.StatusForbidden, "Not a member of this organization")
		return
	}

	domains, err := h.domainService.GetOrgDomains(ctx, org.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load domains")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"domains": domains,
	})
}

// AddOrgDomain adds a custom domain for an organization.
func (h *DomainHandler) AddOrgDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")

	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	_, role, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || (role != "owner" && role != "admin") {
		respondError(w, http.StatusForbidden, "Owner or admin role required")
		return
	}

	var req model.AddDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	domain, err := h.domainService.AddDomain(ctx, "org", org.ID, req.Domain)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"domain":           domain,
		"dns_instructions": h.domainService.BuildDNSInstructions(ctx, domain),
	})
}

// VerifyOrgDomain triggers DNS-TXT ownership verification for an
// organization-owned custom domain. Requires owner or admin role.
func (h *DomainHandler) VerifyOrgDomain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	domainName := chi.URLParam(r, "domain")

	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	_, role, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || (role != "owner" && role != "admin") {
		respondError(w, http.StatusForbidden, "Owner or admin role required")
		return
	}

	// Resolve domain ID and confirm it belongs to this organization.
	domains, err := h.domainService.GetOrgDomains(ctx, org.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domains")
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
		respondError(w, http.StatusNotFound, "domain not found for this organization")
		return
	}

	if err := h.domainService.VerifyDomain(ctx, domainID); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("verification triggered for %s", domainName),
	})
}

// ---- API mirror handlers (/api/v1) -------------------------------------
//
// PART 14 requires every user-facing web route to have a matching versioned
// API route. These methods mirror the web handlers above but resolve the
// acting user from a Bearer user-token as well as a session cookie, and emit
// the canonical {"ok":true,"data":...} envelope via respondJSON.

// currentAPIUser resolves the acting user from an active session or, failing
// that, from a user-scoped Bearer token. Admin- and org-scoped tokens are
// rejected here so user-owned domain routes stay scoped to the token owner.
func (h *DomainHandler) currentAPIUser(r *http.Request) (*service.User, bool) {
	if user, ok := getUserFromRequest(r); ok {
		return user, true
	}
	rec, ok := getBearerFromRequest(r)
	if !ok || !strings.EqualFold(rec.OwnerType, "user") {
		return nil, false
	}
	user, err := h.authService.GetUserByID(r.Context(), rec.OwnerID)
	if err != nil {
		return nil, false
	}
	return user, true
}

// domainIDByName returns the ID of the named domain within the supplied
// owner-scoped list, or 0 when the caller does not own it.
func domainIDByName(domains []*service.CustomDomain, name string) int64 {
	for _, d := range domains {
		if d.Domain == name {
			return d.ID
		}
	}
	return 0
}

// apiOrgForMember resolves the org by slug and confirms the acting user is a
// member; when requireManage is set, owner or admin role is required. On
// failure it writes the error response and returns false.
func (h *DomainHandler) apiOrgForMember(w http.ResponseWriter, ctx context.Context, user *service.User, slug string, requireManage bool) (*service.Organization, bool) {
	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return nil, false
	}
	_, role, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || role == "" {
		respondError(w, http.StatusForbidden, "Not a member of this organization")
		return nil, false
	}
	if requireManage && role != "owner" && role != "admin" {
		respondError(w, http.StatusForbidden, "Owner or admin role required")
		return nil, false
	}
	return org, true
}

// APIListUserDomains — GET /api/v1/users/domains
func (h *DomainHandler) APIListUserDomains(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	domains, err := h.domainService.GetUserDomains(r.Context(), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load domains")
		return
	}
	if domains == nil {
		domains = []*service.CustomDomain{}
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"domains": domains})
}

// APIAddUserDomain — POST /api/v1/users/domains
func (h *DomainHandler) APIAddUserDomain(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var req model.AddDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	domain, err := h.domainService.AddDomain(r.Context(), "user", user.ID, req.Domain)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"domain":           domain,
		"dns_instructions": h.domainService.BuildDNSInstructions(r.Context(), domain),
	})
}

// APIVerifyUserDomain — POST /api/v1/users/domains/{domain}/verify
func (h *DomainHandler) APIVerifyUserDomain(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	domainName := chi.URLParam(r, "domain")
	domains, err := h.domainService.GetUserDomains(r.Context(), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domains")
		return
	}
	domainID := domainIDByName(domains, domainName)
	if domainID == 0 {
		respondError(w, http.StatusNotFound, "domain not found for this user")
		return
	}
	if err := h.domainService.VerifyDomain(r.Context(), domainID); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("verification triggered for %s", domainName),
	})
}

// APIListOrgDomains — GET /api/v1/orgs/{slug}/domains
func (h *DomainHandler) APIListOrgDomains(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	org, ok := h.apiOrgForMember(w, r.Context(), user, chi.URLParam(r, "slug"), false)
	if !ok {
		return
	}
	domains, err := h.domainService.GetOrgDomains(r.Context(), org.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load domains")
		return
	}
	if domains == nil {
		domains = []*service.CustomDomain{}
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"domains": domains})
}

// APIAddOrgDomain — POST /api/v1/orgs/{slug}/domains
func (h *DomainHandler) APIAddOrgDomain(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	org, ok := h.apiOrgForMember(w, r.Context(), user, chi.URLParam(r, "slug"), true)
	if !ok {
		return
	}
	var req model.AddDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	domain, err := h.domainService.AddDomain(r.Context(), "org", org.ID, req.Domain)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"domain":           domain,
		"dns_instructions": h.domainService.BuildDNSInstructions(r.Context(), domain),
	})
}

// APIVerifyOrgDomain — POST /api/v1/orgs/{slug}/domains/{domain}/verify
func (h *DomainHandler) APIVerifyOrgDomain(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	org, ok := h.apiOrgForMember(w, r.Context(), user, chi.URLParam(r, "slug"), true)
	if !ok {
		return
	}
	domainName := chi.URLParam(r, "domain")
	domains, err := h.domainService.GetOrgDomains(r.Context(), org.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domains")
		return
	}
	domainID := domainIDByName(domains, domainName)
	if domainID == 0 {
		respondError(w, http.StatusNotFound, "domain not found for this organization")
		return
	}
	if err := h.domainService.VerifyDomain(r.Context(), domainID); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("verification triggered for %s", domainName),
	})
}

// domainByName returns the named domain from an owner-scoped list, or nil when
// the caller does not own it.
func domainByName(domains []*service.CustomDomain, name string) *service.CustomDomain {
	for _, d := range domains {
		if d.Domain == name {
			return d
		}
	}
	return nil
}

// APIGetUserDomain — GET /api/v1/users/domains/{domain}
func (h *DomainHandler) APIGetUserDomain(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	domains, err := h.domainService.GetUserDomains(r.Context(), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domains")
		return
	}
	cd := domainByName(domains, chi.URLParam(r, "domain"))
	if cd == nil {
		respondError(w, http.StatusNotFound, "domain not found for this user")
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"domain":           cd,
		"dns_instructions": h.domainService.BuildDNSInstructions(r.Context(), cd),
	})
}

// APIUserDomainDNS — GET /api/v1/users/domains/{domain}/dns
func (h *DomainHandler) APIUserDomainDNS(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	domains, err := h.domainService.GetUserDomains(r.Context(), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domains")
		return
	}
	cd := domainByName(domains, chi.URLParam(r, "domain"))
	if cd == nil {
		respondError(w, http.StatusNotFound, "domain not found for this user")
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"dns_instructions": h.domainService.BuildDNSInstructions(r.Context(), cd),
	})
}

// APIDeleteUserDomain — DELETE /api/v1/users/domains/{domain}
func (h *DomainHandler) APIDeleteUserDomain(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	domainName := chi.URLParam(r, "domain")
	domains, err := h.domainService.GetUserDomains(r.Context(), user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domains")
		return
	}
	cd := domainByName(domains, domainName)
	if cd == nil {
		respondError(w, http.StatusNotFound, "domain not found for this user")
		return
	}
	if err := h.domainService.DeleteOwnedDomain(r.Context(), "user", user.ID, cd.ID); err != nil {
		respondError(w, domainStatusFromError(err), err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("domain %s deleted", domainName),
	})
}

// APIGetOrgDomain — GET /api/v1/orgs/{slug}/domains/{domain}
func (h *DomainHandler) APIGetOrgDomain(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	org, ok := h.apiOrgForMember(w, r.Context(), user, chi.URLParam(r, "slug"), false)
	if !ok {
		return
	}
	domains, err := h.domainService.GetOrgDomains(r.Context(), org.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domains")
		return
	}
	cd := domainByName(domains, chi.URLParam(r, "domain"))
	if cd == nil {
		respondError(w, http.StatusNotFound, "domain not found for this organization")
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"domain":           cd,
		"dns_instructions": h.domainService.BuildDNSInstructions(r.Context(), cd),
	})
}

// APIOrgDomainDNS — GET /api/v1/orgs/{slug}/domains/{domain}/dns
func (h *DomainHandler) APIOrgDomainDNS(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	org, ok := h.apiOrgForMember(w, r.Context(), user, chi.URLParam(r, "slug"), false)
	if !ok {
		return
	}
	domains, err := h.domainService.GetOrgDomains(r.Context(), org.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domains")
		return
	}
	cd := domainByName(domains, chi.URLParam(r, "domain"))
	if cd == nil {
		respondError(w, http.StatusNotFound, "domain not found for this organization")
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"dns_instructions": h.domainService.BuildDNSInstructions(r.Context(), cd),
	})
}

// APIDeleteOrgDomain — DELETE /api/v1/orgs/{slug}/domains/{domain}
// Requires owner or admin role in the organization.
func (h *DomainHandler) APIDeleteOrgDomain(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	org, ok := h.apiOrgForMember(w, r.Context(), user, chi.URLParam(r, "slug"), true)
	if !ok {
		return
	}
	domainName := chi.URLParam(r, "domain")
	domains, err := h.domainService.GetOrgDomains(r.Context(), org.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load domains")
		return
	}
	cd := domainByName(domains, domainName)
	if cd == nil {
		respondError(w, http.StatusNotFound, "domain not found for this organization")
		return
	}
	if err := h.domainService.DeleteOwnedDomain(r.Context(), "org", org.ID, cd.ID); err != nil {
		respondError(w, domainStatusFromError(err), err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("domain %s deleted", domainName),
	})
}

// adminActorID returns a pointer to the acting admin's numeric ID, resolved
// from the admin-scoped (adm_) bearer token the RequireBearerAdmin gate has
// already validated. Nil when no bearer record is present (defensive only).
func adminActorID(r *http.Request) *int64 {
	rec, ok := getBearerFromRequest(r)
	if !ok {
		return nil
	}
	id := rec.OwnerID
	return &id
}

// recordAdminAudit writes a best-effort admin audit-log entry for a domain
// action. The domain custom_domain_audit trail is written by the service
// layer; this adds the server-wide admin audit record (append-only audit.log).
func (h *DomainHandler) recordAdminAudit(r *http.Request, action, resource, details string) {
	if h.auditService == nil {
		return
	}
	_ = h.auditService.RecordEvent(r.Context(), adminActorID(r), "admin", action, resource, details, realClientIP(r), r.UserAgent())
}

// domainStatusFromError maps a domain-service error to the correct HTTP status.
func domainStatusFromError(err error) int {
	if errors.Is(err, model.ErrDomainNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

// APIAdminListDomains — GET /api/v1/server/{admin_path}/config/domains
// Lists every custom domain across all owners, paginated (default 250).
func (h *DomainHandler) APIAdminListDomains(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 250
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	domains, total, err := h.domainService.ListAllDomains(r.Context(), page, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load domains")
		return
	}
	if domains == nil {
		domains = []*service.CustomDomain{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(APIResponse{
		OK:         true,
		Data:       domains,
		Pagination: NewPagination(page, limit, total),
	})
}

// APIAdminGetDomain — GET .../config/domains/{domain}
func (h *DomainHandler) APIAdminGetDomain(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	domain, err := h.domainService.GetDomainByName(r.Context(), domainName)
	if err != nil {
		respondError(w, domainStatusFromError(err), err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"domain": domain})
}

// APIAdminDeleteDomain — DELETE .../config/domains/{domain}
// Force-deletes any domain regardless of owner (cascades its audit rows).
func (h *DomainHandler) APIAdminDeleteDomain(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	domain, err := h.domainService.GetDomainByName(r.Context(), domainName)
	if err != nil {
		respondError(w, domainStatusFromError(err), err.Error())
		return
	}
	if err := h.domainService.AdminDeleteDomain(r.Context(), domain.ID, adminActorID(r)); err != nil {
		respondError(w, domainStatusFromError(err), err.Error())
		return
	}
	h.recordAdminAudit(r, "domain.force_delete", "domain:"+domain.Domain, fmt.Sprintf("force-deleted %s (owner %s:%d)", domain.Domain, domain.OwnerType, domain.OwnerID))
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("domain %s deleted", domain.Domain),
	})
}

// APIAdminSuspendDomain — POST .../config/domains/{domain}/suspend
func (h *DomainHandler) APIAdminSuspendDomain(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	domain, err := h.domainService.GetDomainByName(r.Context(), domainName)
	if err != nil {
		respondError(w, domainStatusFromError(err), err.Error())
		return
	}
	if err := h.domainService.SuspendDomain(r.Context(), domain.ID, adminActorID(r)); err != nil {
		respondError(w, domainStatusFromError(err), err.Error())
		return
	}
	h.recordAdminAudit(r, "domain.suspend", "domain:"+domain.Domain, fmt.Sprintf("suspended %s", domain.Domain))
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("domain %s suspended", domain.Domain),
	})
}

// APIAdminUnsuspendDomain — POST .../config/domains/{domain}/unsuspend
func (h *DomainHandler) APIAdminUnsuspendDomain(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	domain, err := h.domainService.GetDomainByName(r.Context(), domainName)
	if err != nil {
		respondError(w, domainStatusFromError(err), err.Error())
		return
	}
	if err := h.domainService.UnsuspendDomain(r.Context(), domain.ID, adminActorID(r)); err != nil {
		respondError(w, domainStatusFromError(err), err.Error())
		return
	}
	h.recordAdminAudit(r, "domain.unsuspend", "domain:"+domain.Domain, fmt.Sprintf("unsuspended %s", domain.Domain))
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("domain %s unsuspended", domain.Domain),
	})
}
