package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// admin domain management web pages (AI.md PART 36 — Admin Domain Management).
// These are the server-rendered, no-JS counterparts to the admin domain JSON
// API (APIAdminListDomains / APIAdminGetDomain / APIAdminSuspendDomain /
// APIAdminUnsuspendDomain / APIAdminDeleteDomain) and reuse the same
// DomainService methods, so the two surfaces never drift.

// csrfInputField renders the hidden CSRF field for no-JS admin forms, matching
// the double-submit cookie validated by CSRFMiddleware.
func csrfInputField(r *http.Request) template.HTML {
	return template.HTML(`<input type="hidden" name="_csrf" value="` + template.HTMLEscapeString(csrfToken(r)) + `">`)
}

// domainStatusBadge renders a coloured status badge for a domain status/state.
func domainStatusBadge(status string) template.HTML {
	label := template.HTMLEscapeString(status)
	class := "badge-blue"
	switch strings.ToLower(status) {
	case "active", "verified":
		class = "badge-green"
	case "suspended", "failed":
		class = "badge-red"
	case "pending", "pending_verification", "verifying":
		class = "badge-yellow"
	}
	return template.HTML(fmt.Sprintf(`<span class="badge %s">%s</span>`, class, label))
}

// domainFlashMessage maps a ?done= query value to a human flash string.
func domainFlashMessage(done string) string {
	switch done {
	case "suspend":
		return "Domain suspended."
	case "unsuspend":
		return "Domain unsuspended."
	case "delete":
		return "Domain deleted."
	default:
		return ""
	}
}

// ConfigDomains renders the admin list of every custom domain (GET
// /server/{admin_path}/config/domains).
func (h *AdminHandler) ConfigDomains(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const limit = 250
	domains, total, err := h.domainService.ListAllDomains(r.Context(), page, limit)
	if err != nil {
		h.adminLayout(w, r, "Custom Domains", "/config/domains", "", "", "Failed to load domains: "+err.Error())
		return
	}

	base := h.basePath()
	var b strings.Builder
	b.WriteString(`<h1>Custom Domains</h1>`)
	b.WriteString(fmt.Sprintf(`<div class="card"><p class="help-text">%d domain(s) total.</p>`, total))
	if len(domains) == 0 {
		b.WriteString(`<p class="help-text">No custom domains have been registered yet.</p></div>`)
		h.adminLayout(w, r, "Custom Domains", "/config/domains", template.HTML(b.String()), domainFlashMessage(r.URL.Query().Get("done")), "")
		return
	}
	b.WriteString(`<table><thead><tr><th>Domain</th><th>Owner</th><th>Verification</th><th>SSL</th><th>Status</th><th></th></tr></thead><tbody>`)
	for _, d := range domains {
		detailURL := base + "/config/domains/" + url.PathEscape(d.Domain)
		b.WriteString(fmt.Sprintf(
			`<tr><td><a href="%s">%s</a></td><td>%s:%d</td><td>%s</td><td>%s</td><td>%s</td><td><a class="btn btn-secondary" href="%s">Manage</a></td></tr>`,
			detailURL,
			template.HTMLEscapeString(d.Domain),
			template.HTMLEscapeString(d.OwnerType), d.OwnerID,
			domainStatusBadge(d.VerificationStatus),
			domainStatusBadge(d.SSLStatus),
			domainStatusBadge(d.Status),
			detailURL,
		))
	}
	b.WriteString(`</tbody></table>`)

	// Pagination controls (no-JS).
	pages := (total + limit - 1) / limit
	if pages > 1 {
		b.WriteString(`<div style="margin-top:16px">`)
		if page > 1 {
			b.WriteString(fmt.Sprintf(`<a class="btn btn-secondary" href="%s/config/domains?page=%d">Previous</a> `, base, page-1))
		}
		b.WriteString(fmt.Sprintf(`<span class="help-text">Page %d of %d</span>`, page, pages))
		if page < pages {
			b.WriteString(fmt.Sprintf(` <a class="btn btn-secondary" href="%s/config/domains?page=%d">Next</a>`, base, page+1))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	h.adminLayout(w, r, "Custom Domains", "/config/domains", template.HTML(b.String()), domainFlashMessage(r.URL.Query().Get("done")), "")
}

// ConfigDomainDetail renders the view/manage page for a single domain (GET
// /server/{admin_path}/config/domains/{domain}).
func (h *AdminHandler) ConfigDomainDetail(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain")
	d, err := h.domainService.GetDomainByName(r.Context(), domainName)
	if err != nil {
		h.adminLayout(w, r, "Custom Domains", "/config/domains", "", "", "Domain not found: "+template.HTMLEscapeString(domainName))
		return
	}

	base := h.basePath()
	esc := template.HTMLEscapeString(d.Domain)
	pathDomain := url.PathEscape(d.Domain)
	csrf := csrfInputField(r)

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<div class="breadcrumb"><a href="%s/config/domains">Custom Domains</a> / %s</div>`, base, esc))
	b.WriteString(fmt.Sprintf(`<h1>%s</h1>`, esc))

	b.WriteString(`<div class="card"><h2>Details</h2>`)
	verified := "no"
	if d.VerifiedAt != nil {
		verified = d.VerifiedAt.UTC().Format("2006-01-02 15:04:05 MST")
	}
	rows := [][2]string{
		{"Owner", fmt.Sprintf("%s:%d", template.HTMLEscapeString(d.OwnerType), d.OwnerID)},
		{"Apex", strconv.FormatBool(d.IsApex)},
		{"Wildcard", strconv.FormatBool(d.IsWildcard)},
		{"Verified at", template.HTMLEscapeString(verified)},
		{"Check count", strconv.Itoa(d.CheckCount)},
		{"Created", d.CreatedAt.UTC().Format("2006-01-02 15:04:05 MST")},
	}
	for _, row := range rows {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">%s</span><span class="info-value">%s</span></div>`, row[0], row[1]))
	}
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Verification</span><span class="info-value">%s</span></div>`, domainStatusBadge(d.VerificationStatus)))
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">SSL</span><span class="info-value">%s</span></div>`, domainStatusBadge(d.SSLStatus)))
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Status</span><span class="info-value">%s</span></div>`, domainStatusBadge(d.Status)))
	b.WriteString(`</div>`)

	b.WriteString(`<div class="card"><h2>Actions</h2>`)
	if strings.EqualFold(d.Status, "suspended") {
		b.WriteString(fmt.Sprintf(
			`<form method="POST" action="%s/config/domains/%s/unsuspend" style="display:inline">%s<button type="submit" class="btn btn-primary">Unsuspend</button></form> `,
			base, pathDomain, csrf))
	} else {
		b.WriteString(fmt.Sprintf(
			`<form method="POST" action="%s/config/domains/%s/suspend" style="display:inline">%s<button type="submit" class="btn btn-secondary">Suspend</button></form> `,
			base, pathDomain, csrf))
	}
	b.WriteString(fmt.Sprintf(
		`<form method="POST" action="%s/config/domains/%s/delete" style="display:inline" onsubmit="return confirm('Force-delete this domain? This cannot be undone.')">%s<button type="submit" class="btn btn-danger">Force Delete</button></form>`,
		base, pathDomain, csrf))
	b.WriteString(`</div>`)

	h.adminLayout(w, r, "Custom Domains", "/config/domains", template.HTML(b.String()), domainFlashMessage(r.URL.Query().Get("done")), "")
}

// ConfigDomainSuspend suspends a domain then redirects back to its detail page
// (POST /server/{admin_path}/config/domains/{domain}/suspend).
func (h *AdminHandler) ConfigDomainSuspend(w http.ResponseWriter, r *http.Request) {
	h.domainWebAction(w, r, "suspend")
}

// ConfigDomainUnsuspend unsuspends a domain (POST .../{domain}/unsuspend).
func (h *AdminHandler) ConfigDomainUnsuspend(w http.ResponseWriter, r *http.Request) {
	h.domainWebAction(w, r, "unsuspend")
}

// ConfigDomainDelete force-deletes a domain then redirects to the list
// (POST .../{domain}/delete).
func (h *AdminHandler) ConfigDomainDelete(w http.ResponseWriter, r *http.Request) {
	h.domainWebAction(w, r, "delete")
}

// domainWebAction is the shared body for the three mutating domain web actions.
func (h *AdminHandler) domainWebAction(w http.ResponseWriter, r *http.Request, action string) {
	admin := h.getAdminFromSession(r)
	if admin == nil {
		http.Redirect(w, r, h.basePath(), http.StatusFound)
		return
	}
	domainName := chi.URLParam(r, "domain")
	d, err := h.domainService.GetDomainByName(r.Context(), domainName)
	if err != nil {
		h.adminLayout(w, r, "Custom Domains", "/config/domains", "", "", "Domain not found: "+template.HTMLEscapeString(domainName))
		return
	}

	base := h.basePath()
	detailURL := base + "/config/domains/" + url.PathEscape(d.Domain)
	actorID := &admin.ID

	switch action {
	case "suspend":
		if err := h.domainService.SuspendDomain(r.Context(), d.ID, actorID); err != nil {
			h.adminLayout(w, r, "Custom Domains", "/config/domains", "", "", "Failed to suspend: "+err.Error())
			return
		}
		h.recordAudit(r, actorID, "domain.suspend", "domain:"+d.Domain, "suspended "+d.Domain)
		http.Redirect(w, r, detailURL+"?done=suspend", http.StatusSeeOther)
	case "unsuspend":
		if err := h.domainService.UnsuspendDomain(r.Context(), d.ID, actorID); err != nil {
			h.adminLayout(w, r, "Custom Domains", "/config/domains", "", "", "Failed to unsuspend: "+err.Error())
			return
		}
		h.recordAudit(r, actorID, "domain.unsuspend", "domain:"+d.Domain, "unsuspended "+d.Domain)
		http.Redirect(w, r, detailURL+"?done=unsuspend", http.StatusSeeOther)
	case "delete":
		if err := h.domainService.AdminDeleteDomain(r.Context(), d.ID, actorID); err != nil {
			h.adminLayout(w, r, "Custom Domains", "/config/domains", "", "", "Failed to delete: "+err.Error())
			return
		}
		h.recordAudit(r, actorID, "domain.force_delete", "domain:"+d.Domain, fmt.Sprintf("force-deleted %s (owner %s:%d)", d.Domain, d.OwnerType, d.OwnerID))
		http.Redirect(w, r, base+"/config/domains?done=delete", http.StatusSeeOther)
	}
}
