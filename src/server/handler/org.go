package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/tmpl"
	"github.com/webappsgo/caslink/src/server/validate"
)

// OrgSummary is the view model for each row in the org list.
type OrgSummary struct {
	Name        string
	Slug        string
	MemberCount int
	Role        string
}

// OrgView is the view model for a single organization.
type OrgView struct {
	Name        string
	Slug        string
	Description string
	Website     string
	Location    string
	Visibility  string
}

// OrgStats holds aggregate statistics for the org dashboard.
type OrgStats struct {
	TotalLinks  int
	TotalClicks int
	MemberCount int
}

// RecentLink is a single row in the dashboard recent-links table.
type RecentLink struct {
	Code        string
	Destination string
	Clicks      int
	CreatedAt   string
}

// OrgMemberView is a single row in the members table.
type OrgMemberView struct {
	UserID   int64
	Username string
	Role     string
	JoinedAt string
}

// OrgHandler handles organization operations.
type OrgHandler struct {
	orgService  *service.OrgService
	authService *service.AuthService
	renderer    *tmpl.Renderer
	config      *config.Config
}

// NewOrgHandler creates a new organization handler.
func NewOrgHandler(orgService *service.OrgService, authService *service.AuthService, renderer *tmpl.Renderer, cfg *config.Config) *OrgHandler {
	return &OrgHandler{
		orgService:  orgService,
		authService: authService,
		renderer:    renderer,
		config:      cfg,
	}
}

// ListOrgs renders the list of the user's organizations.
func (h *OrgHandler) ListOrgs(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	orgs, err := h.orgService.GetUserOrganizationsWithSummary(ctx, user.ID)
	if err != nil {
		http.Error(w, "Failed to load organizations", http.StatusInternalServerError)
		return
	}

	summaries := make([]OrgSummary, 0, len(orgs))
	for _, o := range orgs {
		summaries = append(summaries, OrgSummary{
			Name:        o.Name,
			Slug:        o.Slug,
			MemberCount: o.MemberCount,
			Role:        o.Role,
		})
	}

	data := struct {
		tmpl.Data
		Orgs []OrgSummary
	}{
		Data: newPageData(h.config, r, "Organizations", user),
		Orgs: summaries,
	}

	h.renderer.Render(w, "template/page/orgs/list.html", data)
}

// CreateOrgPage renders the create organization form.
func (h *OrgHandler) CreateOrgPage(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data := struct {
		tmpl.Data
		OrgName        string
		OrgSlug        string
		OrgDescription string
	}{
		Data: newPageData(h.config, r, "Create Organization", user),
	}
	h.renderer.Render(w, "template/page/orgs/new.html", data)
}

// creationBlockedReason applies the server-level org creation policy (PART 35):
// the creation mode gate (only "open" allows self-service creation) and the
// per-user ownership limit (max_per_user, 0 = unlimited). It returns a
// human-readable reason and HTTP status when creation must be refused, or an
// empty reason and 0 when the user may proceed.
func (h *OrgHandler) creationBlockedReason(ctx context.Context, userID int64) (string, int) {
	orgCfg := h.config.Server.Features.Organizations
	if !orgCfg.AuthenticatedCreationAllowed() {
		switch orgCfg.NormalizedCreationMode() {
		case "invite":
			return "Organization creation is invite-only. Use the creation invite issued by an administrator.", http.StatusForbidden
		case "admin_only":
			return "Organizations are created by an administrator. Contact your administrator to request one.", http.StatusForbidden
		default:
			return "Organization creation is currently disabled.", http.StatusForbidden
		}
	}
	if max := orgCfg.MaxPerUser; max > 0 {
		count, err := h.orgService.CountOwnedOrgs(ctx, userID)
		if err != nil {
			return "Unable to verify your organization limit. Please try again.", http.StatusInternalServerError
		}
		if count >= max {
			return fmt.Sprintf("You have reached the maximum of %d organizations.", max), http.StatusForbidden
		}
	}
	return "", 0
}

// CreateOrg handles organization creation (POST /orgs/new).
func (h *OrgHandler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	slug := r.FormValue("slug")

	if reason, status := h.creationBlockedReason(r.Context(), user.ID); reason != "" {
		w.WriteHeader(status)
		h.renderNewOrgWithError(w, r, user, name, slug, reason)
		return
	}

	if len(name) < 3 || len(name) > 40 {
		h.renderNewOrgWithError(w, r, user, name, slug, "Organization name must be between 3 and 40 characters")
		return
	}

	if err := validate.ValidateOrgSlug(slug); err != nil {
		h.renderNewOrgWithError(w, r, user, name, slug, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.CreateOrganization(ctx, user.ID, name, slug)
	if err != nil {
		h.renderNewOrgWithError(w, r, user, name, slug, err.Error())
		return
	}

	http.Redirect(w, r, "/orgs/"+org.Slug+"/", http.StatusSeeOther)
}

func (h *OrgHandler) renderNewOrgWithError(w http.ResponseWriter, r *http.Request, user *service.User, name, slug, errMsg string) {
	data := struct {
		tmpl.Data
		OrgName        string
		OrgSlug        string
		OrgDescription string
	}{
		Data:    newPageData(h.config, r, "Create Organization", user),
		OrgName: name,
		OrgSlug: slug,
	}
	data.Data.Flash = &tmpl.Flash{Type: "danger", Message: errMsg}
	h.renderer.Render(w, "template/page/orgs/new.html", data)
}

// OrgDashboard renders the organization dashboard.
func (h *OrgHandler) OrgDashboard(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	slug := chi.URLParam(r, "slug")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	members, _ := h.orgService.GetOrgMembers(ctx, org.ID)

	data := struct {
		tmpl.Data
		Org         OrgView
		Stats       OrgStats
		RecentLinks []RecentLink
	}{
		Data: newPageData(h.config, r, org.Name+" — Dashboard", user),
		Org: OrgView{
			Name: org.Name,
			Slug: org.Slug,
		},
		Stats: OrgStats{
			MemberCount: len(members),
		},
		RecentLinks: []RecentLink{},
	}

	h.renderer.Render(w, "template/page/orgs/dashboard.html", data)
}

// OrgSettings renders the organization settings page.
func (h *OrgHandler) OrgSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	slug := chi.URLParam(r, "slug")

	h.renderSettingsPage(w, r, user, slug, nil)
}

// renderSettingsPage loads the org for slug and renders the settings page,
// optionally with a one-shot flash. Shared by OrgSettings (GET) and
// OrgSettingsSave (POST). Only owners/admins reach the mutating POST path;
// the delete control in the danger zone is gated on IsOwner.
func (h *OrgHandler) renderSettingsPage(w http.ResponseWriter, r *http.Request, user *service.User, slug string, flash *tmpl.Flash) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	_, userRole, _ := h.orgService.IsMember(ctx, org.ID, user.ID)

	data := struct {
		tmpl.Data
		Org     OrgView
		IsOwner bool
	}{
		Data: newPageData(h.config, r, org.Name+" — Settings", user),
		Org: OrgView{
			Name:        org.Name,
			Slug:        org.Slug,
			Description: org.Description,
			Website:     org.Website,
			Location:    org.Location,
			Visibility:  org.Visibility,
		},
		IsOwner: userRole == "owner",
	}
	data.Data.Flash = flash

	h.renderer.Render(w, "template/page/orgs/settings.html", data)
}

// OrgSettingsSave handles POST /orgs/{slug}/settings — the General-settings
// update form and the Danger Zone delete, dispatched on the hidden "action"
// field. Update is allowed for owners and admins; delete is owner-only
// (PART 35 role permissions).
func (h *OrgHandler) OrgSettingsSave(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	slug := chi.URLParam(r, "slug")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	_, userRole, _ := h.orgService.IsMember(ctx, org.ID, user.ID)
	if userRole != "owner" && userRole != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	switch r.FormValue("action") {
	case "delete":
		if userRole != "owner" {
			http.Error(w, "Only the organization owner can delete it", http.StatusForbidden)
			return
		}
		if r.FormValue("confirm_slug") != org.Slug {
			h.renderSettingsPage(w, r, user, slug, &tmpl.Flash{Type: "danger", Message: "Confirmation text did not match the organization slug."})
			return
		}
		if err := h.orgService.DeleteOrganization(ctx, org.ID); err != nil {
			h.renderSettingsPage(w, r, user, slug, &tmpl.Flash{Type: "danger", Message: err.Error()})
			return
		}
		http.Redirect(w, r, "/orgs", http.StatusSeeOther)
		return

	case "update":
		err := h.orgService.UpdateOrganization(ctx, org.ID, service.OrgProfileUpdate{
			Name:        r.FormValue("name"),
			Description: r.FormValue("description"),
			Website:     r.FormValue("website"),
			Location:    r.FormValue("location"),
			Visibility:  r.FormValue("visibility"),
		})
		if err != nil {
			h.renderSettingsPage(w, r, user, slug, &tmpl.Flash{Type: "danger", Message: err.Error()})
			return
		}
		h.renderSettingsPage(w, r, user, slug, &tmpl.Flash{Type: "success", Message: "Organization settings saved."})
		return

	default:
		h.renderSettingsPage(w, r, user, slug, &tmpl.Flash{Type: "danger", Message: "Unknown action."})
	}
}

// OrgMembers renders the organization members page.
func (h *OrgHandler) OrgMembers(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	slug := chi.URLParam(r, "slug")
	h.renderMembersPage(w, r, user, slug, nil)
}

// renderMembersPage loads the organization + members for slug and renders
// the members page, optionally with a one-shot flash message. Shared by
// OrgMembers (GET) and OrgMembersAction (POST, after performing an action).
func (h *OrgHandler) renderMembersPage(w http.ResponseWriter, r *http.Request, user *service.User, slug string, flash *tmpl.Flash) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	members, err := h.orgService.GetMembersWithUsernames(ctx, org.ID)
	if err != nil {
		http.Error(w, "Failed to load members", http.StatusInternalServerError)
		return
	}

	_, userRole, _ := h.orgService.IsMember(ctx, org.ID, user.ID)
	isAdminOrOwner := userRole == "admin" || userRole == "owner"

	memberViews := make([]OrgMemberView, 0, len(members))
	for _, m := range members {
		memberViews = append(memberViews, OrgMemberView{
			UserID:   m.UserID,
			Username: m.Username,
			Role:     m.Role,
			JoinedAt: m.JoinedAt.Format("2006-01-02"),
		})
	}

	data := struct {
		tmpl.Data
		Org            OrgView
		Members        []OrgMemberView
		IsAdminOrOwner bool
	}{
		Data: newPageData(h.config, r, org.Name+" — Members", user),
		Org: OrgView{
			Name: org.Name,
			Slug: org.Slug,
		},
		Members:        memberViews,
		IsAdminOrOwner: isAdminOrOwner,
	}
	data.Data.Flash = flash

	h.renderer.Render(w, "template/page/orgs/members.html", data)
}

// OrgMembersAction handles POST /orgs/{slug}/members — member management
// actions (remove / change_role / invite) per AI.md PART 35. Only org
// admins/owners may perform these actions; the result is a re-render of the
// members page with a flash message (the app has no cross-redirect flash
// persistence, so we render directly rather than redirect-after-post).
func (h *OrgHandler) OrgMembersAction(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	slug := chi.URLParam(r, "slug")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	_, userRole, _ := h.orgService.IsMember(ctx, org.ID, user.ID)
	if userRole != "admin" && userRole != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	action := r.FormValue("action")
	var flash *tmpl.Flash

	switch action {
	case "remove":
		targetID, err := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
		if err != nil {
			flash = &tmpl.Flash{Type: "danger", Message: "Invalid member"}
			break
		}
		if err := h.orgService.RemoveMember(ctx, org.ID, targetID); err != nil {
			flash = &tmpl.Flash{Type: "danger", Message: err.Error()}
		} else {
			flash = &tmpl.Flash{Type: "success", Message: "Member removed"}
		}

	case "change_role":
		targetID, err := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
		if err != nil {
			flash = &tmpl.Flash{Type: "danger", Message: "Invalid member"}
			break
		}
		newRole := r.FormValue("role")
		if err := h.orgService.ChangeMemberRole(ctx, org.ID, targetID, newRole); err != nil {
			flash = &tmpl.Flash{Type: "danger", Message: err.Error()}
		} else {
			flash = &tmpl.Flash{Type: "success", Message: "Role updated"}
		}

	case "invite":
		email := r.FormValue("email")
		role := r.FormValue("role")
		if err := h.orgService.AddMemberByEmail(ctx, org.ID, email, role); err != nil {
			flash = &tmpl.Flash{Type: "danger", Message: err.Error()}
		} else {
			flash = &tmpl.Flash{Type: "success", Message: "Member added"}
		}

	default:
		flash = &tmpl.Flash{Type: "danger", Message: "Unknown action"}
	}

	h.renderMembersPage(w, r, user, slug, flash)
}

// APIListOrgs returns the user's organizations as JSON.
func (h *OrgHandler) APIListOrgs(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	orgs, err := h.orgService.GetUserOrganizations(ctx, user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load organizations")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"organizations": orgs})
}

// APICreateOrg creates an organization and returns JSON.
func (h *OrgHandler) APICreateOrg(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req model.CreateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.Name) < 3 || len(req.Name) > 40 {
		respondError(w, http.StatusBadRequest, "Organization name must be between 3 and 40 characters")
		return
	}

	if req.Slug != "" {
		if err := validate.ValidateOrgSlug(req.Slug); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if reason, status := h.creationBlockedReason(ctx, user.ID); reason != "" {
		respondError(w, status, reason)
		return
	}

	org, err := h.orgService.CreateOrganization(ctx, user.ID, req.Name, req.Slug)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "organization": org})
}

// APIGetOrg returns a single organization as JSON.
func (h *OrgHandler) APIGetOrg(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	slug := chi.URLParam(r, "slug")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	isMember, _, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || !isMember {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	respondJSON(w, http.StatusOK, org)
}

// APIGetMembers returns the members of an organization as JSON.
func (h *OrgHandler) APIGetMembers(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	slug := chi.URLParam(r, "slug")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	isMember, _, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || !isMember {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	members, err := h.orgService.GetOrgMembers(ctx, org.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load members")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"members": members})
}

// APICreateOrgToken handles POST /api/v1/orgs/{slug}/tokens
// Only org owners and admins may create tokens.
func (h *OrgHandler) APICreateOrgToken(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	slug := chi.URLParam(r, "slug")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	isMember, role, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || !isMember || (role != "owner" && role != "admin") {
		respondError(w, http.StatusForbidden, "Only org owners and admins can create tokens")
		return
	}

	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Permissions == nil {
		req.Permissions = []string{}
	}

	tok, plainToken, err := h.orgService.CreateOrgToken(ctx, org.ID, user.ID, req.Name, req.Permissions)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create token")
		return
	}

	// Pass the raw data directly — respondJSON already wraps it as
	// {"ok":true,"data":{...}}; pre-wrapping here would double-wrap it.
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"token":     plainToken,
		"org_token": tok,
	})
}

// APIListOrgTokens handles GET /api/v1/orgs/{slug}/tokens
// Returns all active tokens for the org. Only members can list tokens.
func (h *OrgHandler) APIListOrgTokens(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	slug := chi.URLParam(r, "slug")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	isMember, _, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || !isMember {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	tokens, err := h.orgService.ListOrgTokens(ctx, org.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list tokens")
		return
	}

	// Pass tokens directly — respondJSON already wraps it as
	// {"ok":true,"data":tokens}; pre-wrapping here would double-wrap it.
	respondJSON(w, http.StatusOK, tokens)
}

// APIRevokeOrgToken handles DELETE /api/v1/orgs/{slug}/tokens/{tokenID}
// Only org owners and admins may revoke tokens.
func (h *OrgHandler) APIRevokeOrgToken(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	slug := chi.URLParam(r, "slug")
	tokenIDStr := chi.URLParam(r, "tokenID")
	tokenID, err := strconv.ParseInt(tokenIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid token ID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	isMember, role, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || !isMember || (role != "owner" && role != "admin") {
		respondError(w, http.StatusForbidden, "Only org owners and admins can revoke tokens")
		return
	}

	if err := h.orgService.RevokeOrgToken(ctx, tokenID, org.ID); err != nil {
		respondError(w, http.StatusNotFound, "Token not found")
		return
	}

	respondJSON(w, http.StatusOK, nil)
}

// APITransferOrgOwnership handles POST /api/v1/orgs/{slug}/transfer
// Only the current owner may transfer ownership. The new owner must already be a member.
func (h *OrgHandler) APITransferOrgOwnership(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	slug := chi.URLParam(r, "slug")

	var req struct {
		NewOwnerID int64 `json:"new_owner_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NewOwnerID == 0 {
		respondError(w, http.StatusBadRequest, "new_owner_id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	if err := h.orgService.TransferOwnership(ctx, org.ID, user.ID, req.NewOwnerID); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Ownership transferred"})
}

// APIUpdateOrg handles PATCH /api/v1/orgs/{slug} — update General settings
// (name, description, website, location, visibility). Owners and admins only
// (PART 35 role permissions). Absent JSON fields fall back to the current
// stored value so a partial patch does not blank unspecified fields.
func (h *OrgHandler) APIUpdateOrg(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	slug := chi.URLParam(r, "slug")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	isMember, role, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || !isMember || (role != "owner" && role != "admin") {
		respondError(w, http.StatusForbidden, "Only org owners and admins can update settings")
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Website     *string `json:"website"`
		Location    *string `json:"location"`
		Visibility  *string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	in := service.OrgProfileUpdate{
		Name:        org.Name,
		Description: org.Description,
		Website:     org.Website,
		Location:    org.Location,
		Visibility:  org.Visibility,
	}
	if req.Name != nil {
		in.Name = *req.Name
	}
	if req.Description != nil {
		in.Description = *req.Description
	}
	if req.Website != nil {
		in.Website = *req.Website
	}
	if req.Location != nil {
		in.Location = *req.Location
	}
	if req.Visibility != nil {
		in.Visibility = *req.Visibility
	}

	if err := h.orgService.UpdateOrganization(ctx, org.ID, in); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to reload organization")
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

// APIDeleteOrg handles DELETE /api/v1/orgs/{slug} — permanently delete an
// organization. Owner only (PART 35). FK cascade removes members, tokens,
// and org-owned custom domains.
func (h *OrgHandler) APIDeleteOrg(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	slug := chi.URLParam(r, "slug")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	isMember, role, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || !isMember || role != "owner" {
		respondError(w, http.StatusForbidden, "Only the organization owner can delete it")
		return
	}

	if err := h.orgService.DeleteOrganization(ctx, org.ID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Organization deleted"})
}
