package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/server/model"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/tmpl"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

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

	orgs, err := h.orgService.GetUserOrganizations(ctx, user.ID)
	if err != nil {
		http.Error(w, "Failed to load organizations", http.StatusInternalServerError)
		return
	}

	summaries := make([]OrgSummary, 0, len(orgs))
	for _, o := range orgs {
		members, _ := h.orgService.GetOrgMembers(ctx, o.ID)
		_, role, _ := h.orgService.IsMember(ctx, o.ID, user.ID)
		summaries = append(summaries, OrgSummary{
			Name:        o.Name,
			Slug:        o.Slug,
			MemberCount: len(members),
			Role:        role,
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

	data := newPageData(h.config, r, "Create Organization", user)
	h.renderer.Render(w, "template/page/orgs/new.html", data)
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

	if len(name) < 3 || len(name) > 40 {
		h.renderNewOrgWithError(w, r, user, name, slug, "Organization name must be between 3 and 40 characters")
		return
	}

	if !slugRegex.MatchString(slug) {
		h.renderNewOrgWithError(w, r, user, name, slug, "Slug must be 3–40 lowercase letters, digits, and hyphens, and cannot start or end with a hyphen")
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

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	data := struct {
		tmpl.Data
		Org OrgView
	}{
		Data: newPageData(h.config, r, org.Name+" — Settings", user),
		Org: OrgView{
			Name: org.Name,
			Slug: org.Slug,
		},
	}

	h.renderer.Render(w, "template/page/orgs/settings.html", data)
}

// OrgMembers renders the organization members page.
func (h *OrgHandler) OrgMembers(w http.ResponseWriter, r *http.Request) {
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

	h.renderer.Render(w, "template/page/orgs/members.html", data)
}

// APIListOrgs returns the user's organizations as JSON.
func (h *OrgHandler) APIListOrgs(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	orgs, err := h.orgService.GetUserOrganizations(ctx, user.ID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load organizations"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"organizations": orgs})
}

// APICreateOrg creates an organization and returns JSON.
func (h *OrgHandler) APICreateOrg(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}

	var req model.CreateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if len(req.Name) < 3 || len(req.Name) > 40 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Organization name must be between 3 and 40 characters"})
		return
	}

	if req.Slug != "" && !slugRegex.MatchString(req.Slug) {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid slug format"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.CreateOrganization(ctx, user.ID, req.Name, req.Slug)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "organization": org})
}

// APIGetOrg returns a single organization as JSON.
func (h *OrgHandler) APIGetOrg(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}

	slug := chi.URLParam(r, "slug")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Organization not found"})
		return
	}

	isMember, _, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || !isMember {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Access denied"})
		return
	}

	respondJSON(w, http.StatusOK, org)
}

// APIGetMembers returns the members of an organization as JSON.
func (h *OrgHandler) APIGetMembers(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}

	slug := chi.URLParam(r, "slug")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	org, err := h.orgService.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Organization not found"})
		return
	}

	isMember, _, err := h.orgService.IsMember(ctx, org.ID, user.ID)
	if err != nil || !isMember {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Access denied"})
		return
	}

	members, err := h.orgService.GetOrgMembers(ctx, org.ID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load members"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"members": members})
}
