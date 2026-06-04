package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/server/tmpl"
	apktor "github.com/casjaysdevdocker/caslink/src/tor"
)

// PagesHandler handles the public server information pages: About, Help,
// Privacy, Contact, and Terms. Content is sourced from IDEA.md / spec baked
// into the binary — never generic placeholder text.
type PagesHandler struct {
	cfg           *config.Config
	renderer      *tmpl.Renderer
	version       string
	buildDate     string
	getTorManager func() *apktor.TorManager
}

// NewPagesHandler creates a PagesHandler. getTorManager is called on each
// request so the handler sees the TorManager even though it is initialised
// after setupRoutes() runs (in Start()). Pass nil to disable Tor sections.
func NewPagesHandler(cfg *config.Config, renderer *tmpl.Renderer, version, buildDate string, getTorManager func() *apktor.TorManager) *PagesHandler {
	return &PagesHandler{
		cfg:           cfg,
		renderer:      renderer,
		version:       version,
		buildDate:     buildDate,
		getTorManager: getTorManager,
	}
}

// caslink* constants hold the baked-in IDEA.md content shown on public pages.
const (
	caslinkTagline     = "Self-hosted URL shortening, done right."
	caslinkDescription = "Caslink is a secure, mobile-first, fully self-hosted URL shortener written in Go that ships as a single static binary with zero external dependencies. It targets individuals and teams who want the control of self-hosting without the operational complexity of multi-service stacks. Any user can shorten links, track clicks, generate QR codes, and manage custom branded domains — all features ship to all users, no tier gating."
	caslinkGitHub      = "https://github.com/casjaysdevdocker/caslink"
	caslinkSite        = "https://caslink.casapps.us"
)

// caslinkFeatures lists the canonical feature set sourced from IDEA.md.
var caslinkFeatures = []string{
	"URL shortening with custom codes, expiry, and password protection",
	"Click analytics with GeoIP country/city tracking",
	"QR code generation (PNG, SVG, PDF) with logo overlay and custom colors",
	"Custom branded domains with automatic Let's Encrypt SSL",
	"Organizations with owner/admin/member RBAC",
	"Bulk import/export (CSV, JSON)",
	"Federation: link sharing between instances",
	"Optional billing module for monetization",
	"REST API + GraphQL + OpenAPI/Swagger",
	"Single static binary — zero external dependencies, runs anywhere",
}

// aboutData is the template context for the About page.
type aboutData struct {
	tmpl.Data
	Tagline      string
	Description  string
	Features     []string
	GitHubURL    string
	OfficialSite string
	BuildDate    string
	AppVersion   string
}

// helpSection groups a help topic with its content items.
type helpSection struct {
	Title string
	Items []helpItem
}

// helpItem holds a single help entry (question/answer or example).
type helpItem struct {
	Label   string
	Content string
	IsCode  bool
}

// helpData is the template context for the Help page.
type helpData struct {
	tmpl.Data
	Sections   []helpSection
	TorEnabled bool
	TorAddress string
	APIBase    string
}

// privacyData is the template context for the Privacy page.
type privacyData struct {
	tmpl.Data
	ContactURL string
}

// contactData is the template context for the Contact page.
type contactData struct {
	tmpl.Data
	Sent    bool
	Name    string
	Email   string
	Subject string
	Message string
	Errors  []string
}

// contactSubmission holds a validated contact form payload.
type contactSubmission struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

// termsData is the template context for the Terms page.
type termsData struct {
	tmpl.Data
}

// ---- HTML handlers ----

// About renders the /server/about page.
func (h *PagesHandler) About(w http.ResponseWriter, r *http.Request) {
	data := aboutData{
		Data:         newPageData(h.cfg, r, "About", nil),
		Tagline:      caslinkTagline,
		Description:  caslinkDescription,
		Features:     caslinkFeatures,
		GitHubURL:    caslinkGitHub,
		OfficialSite: caslinkSite,
		BuildDate:    h.buildDate,
		AppVersion:   h.version,
	}
	h.renderer.Render(w, "template/page/server/about.html", data)
}

// Help renders the /server/help page.
func (h *PagesHandler) Help(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if !h.cfg.Server.SSL.Enabled {
		scheme = "http"
	}
	fqdn := h.cfg.Server.FQDN
	if fqdn == "" {
		fqdn = "caslink.casapps.us"
	}
	apiBase := scheme + "://" + fqdn

	torEnabled := false
	torAddress := ""
	if h.getTorManager != nil {
		if tm := h.getTorManager(); tm != nil && tm.IsRunning() {
			torEnabled = true
			torAddress = tm.OnionAddress()
		}
	}

	data := helpData{
		Data:       newPageData(h.cfg, r, "Help", nil),
		TorEnabled: torEnabled,
		TorAddress: torAddress,
		APIBase:    apiBase,
		Sections:   buildHelpSections(apiBase),
	}
	h.renderer.Render(w, "template/page/server/help.html", data)
}

// buildHelpSections returns the structured help content using the real API base.
func buildHelpSections(apiBase string) []helpSection {
	return []helpSection{
		{
			Title: "Getting Started",
			Items: []helpItem{
				{Label: "Create a short link", Content: `curl -s -X POST ` + apiBase + `/api/v1/urls \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"long_url":"https://example.com/very/long/path"}'`, IsCode: true},
				{Label: "Create with a custom code", Content: `curl -s -X POST ` + apiBase + `/api/v1/urls \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"long_url":"https://example.com","custom_code":"mylink"}'`, IsCode: true},
				{Label: "Look up a short link", Content: `curl -s ` + apiBase + `/api/v1/urls/mylink`, IsCode: true},
			},
		},
		{
			Title: "Authentication",
			Items: []helpItem{
				{Label: "Log in and get a session token", Content: `curl -s -X POST ` + apiBase + `/api/v1/server/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"secret"}'`, IsCode: true},
				{Label: "Generate an API token", Content: "Visit " + apiBase + "/users/tokens to create long-lived API tokens. Pass them as `Authorization: Bearer <token>` on every API request.", IsCode: false},
			},
		},
		{
			Title: "Analytics",
			Items: []helpItem{
				{Label: "Get click stats for a link", Content: `curl -s ` + apiBase + `/api/v1/urls/mylink/stats`, IsCode: true},
				{Label: "What is tracked?", Content: "Each redirect records the timestamp, country (GeoIP), browser family, OS, device type, referrer domain, and a salted IP hash for unique-visitor counting. Raw IPs are never stored when anonymize_ips is enabled.", IsCode: false},
			},
		},
		{
			Title: "QR Codes",
			Items: []helpItem{
				{Label: "Download a QR code (PNG)", Content: `curl -s ` + apiBase + `/api/v1/qr/mylink -o qr.png`, IsCode: true},
				{Label: "Request SVG format", Content: `curl -s "` + apiBase + `/api/v1/qr/mylink?format=svg" -o qr.svg`, IsCode: true},
			},
		},
		{
			Title: "Bulk Operations",
			Items: []helpItem{
				{Label: "Import URLs from CSV", Content: `curl -s -X POST ` + apiBase + `/api/v1/users/urls/import \
  -H "Authorization: Bearer <token>" \
  -F "file=@links.csv"`, IsCode: true},
				{Label: "Export all your URLs", Content: `curl -s ` + apiBase + `/api/v1/users/urls/export \
  -H "Authorization: Bearer <token>" -o export.json`, IsCode: true},
			},
		},
		{
			Title: "API Documentation",
			Items: []helpItem{
				{Label: "Interactive Swagger UI", Content: apiBase + "/server/docs/swagger", IsCode: false},
				{Label: "OpenAPI JSON spec", Content: apiBase + "/api/v1/server/swagger", IsCode: false},
				{Label: "GraphiQL explorer", Content: apiBase + "/graphiql", IsCode: false},
			},
		},
		{
			Title: "Troubleshooting",
			Items: []helpItem{
				{Label: "Health check", Content: `curl -s ` + apiBase + `/server/healthz`, IsCode: true},
				{Label: "401 Unauthorized", Content: "Your API token is missing, expired, or revoked. Regenerate it at " + apiBase + "/users/tokens.", IsCode: false},
				{Label: "409 Conflict on custom code", Content: "That code is already taken. Choose a different custom code or omit it to have one generated automatically.", IsCode: false},
				{Label: "429 Too Many Requests", Content: "You have hit the rate limit. Wait 60 seconds and retry. Authenticated users have higher limits than anonymous visitors.", IsCode: false},
			},
		},
	}
}

// Privacy renders the /server/privacy page.
func (h *PagesHandler) Privacy(w http.ResponseWriter, r *http.Request) {
	data := privacyData{
		Data:       newPageData(h.cfg, r, "Privacy Policy", nil),
		ContactURL: "/server/contact",
	}
	h.renderer.Render(w, "template/page/server/privacy.html", data)
}

// Contact renders the /server/contact GET page.
func (h *PagesHandler) Contact(w http.ResponseWriter, r *http.Request) {
	sent := r.URL.Query().Get("sent") == "1"
	data := contactData{
		Data: newPageData(h.cfg, r, "Contact", nil),
		Sent: sent,
	}
	h.renderer.Render(w, "template/page/server/contact.html", data)
}

// ContactSubmit handles POST /server/contact.
func (h *PagesHandler) ContactSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	message := strings.TrimSpace(r.FormValue("message"))

	var errs []string
	if name == "" {
		errs = append(errs, "Name is required.")
	}
	if email == "" {
		errs = append(errs, "Email is required.")
	}
	if subject == "" {
		errs = append(errs, "Subject is required.")
	}
	if message == "" {
		errs = append(errs, "Message is required.")
	}

	if len(errs) > 0 {
		data := contactData{
			Data:    newPageData(h.cfg, r, "Contact", nil),
			Name:    name,
			Email:   email,
			Subject: subject,
			Message: message,
			Errors:  errs,
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderer.Render(w, "template/page/server/contact.html", data)
		return
	}

	recipient := h.cfg.Server.Contact.General.Email
	if recipient == "" {
		recipient = h.cfg.Server.Admin.Email
	}
	log.Printf("[contact] from=%s <%s> to=%s subject=%q len=%d", name, email, recipient, subject, len(message))

	http.Redirect(w, r, "/server/contact?sent=1", http.StatusSeeOther)
}

// Terms renders the /server/terms page.
func (h *PagesHandler) Terms(w http.ResponseWriter, r *http.Request) {
	data := termsData{
		Data: newPageData(h.cfg, r, "Terms of Service", nil),
	}
	h.renderer.Render(w, "template/page/server/terms.html", data)
}

// ---- JSON API handlers ----

// APIAbout returns About data as JSON.
func (h *PagesHandler) APIAbout(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"name":          "Caslink",
		"tagline":       caslinkTagline,
		"description":   caslinkDescription,
		"version":       h.version,
		"build_date":    h.buildDate,
		"features":      caslinkFeatures,
		"github_url":    caslinkGitHub,
		"official_site": caslinkSite,
	})
}

// APIHelp returns Help content as JSON.
func (h *PagesHandler) APIHelp(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if !h.cfg.Server.SSL.Enabled {
		scheme = "http"
	}
	fqdn := h.cfg.Server.FQDN
	if fqdn == "" {
		fqdn = "caslink.casapps.us"
	}
	apiBase := scheme + "://" + fqdn

	torEnabled := false
	torAddress := ""
	if h.getTorManager != nil {
		if tm := h.getTorManager(); tm != nil && tm.IsRunning() {
			torEnabled = true
			torAddress = tm.OnionAddress()
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"api_base":    apiBase,
		"swagger_ui":  apiBase + "/server/docs/swagger",
		"graphiql":    apiBase + "/graphiql",
		"healthz":     apiBase + "/server/healthz",
		"sections":    buildHelpSections(apiBase),
		"tor_enabled": torEnabled,
		"tor_address": torAddress,
	})
}

// APIPrivacy returns Privacy policy as JSON.
func (h *PagesHandler) APIPrivacy(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"data_sold":        false,
		"contact_url":      "/server/contact",
		"retention_days":   30,
		"cookies":          []string{"essential", "preferences", "analytics"},
		"export_available": true,
		"deletion_available": true,
	})
}

// APITerms returns Terms of Service as JSON.
func (h *PagesHandler) APITerms(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"service":     "Caslink",
		"contact_url": "/server/contact",
		"sections": []string{
			"Acceptance",
			"Account Terms",
			"Acceptable Use",
			"Content",
			"Termination",
			"Liability",
			"Changes",
			"Governing Law",
		},
	})
}

// APIContact handles POST /api/v1/server/contact with a JSON body.
func (h *PagesHandler) APIContact(w http.ResponseWriter, r *http.Request) {
	var sub contactSubmission
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	sub.Name = strings.TrimSpace(sub.Name)
	sub.Email = strings.TrimSpace(sub.Email)
	sub.Subject = strings.TrimSpace(sub.Subject)
	sub.Message = strings.TrimSpace(sub.Message)

	var missing []string
	if sub.Name == "" {
		missing = append(missing, "name")
	}
	if sub.Email == "" {
		missing = append(missing, "email")
	}
	if sub.Subject == "" {
		missing = append(missing, "subject")
	}
	if sub.Message == "" {
		missing = append(missing, "message")
	}
	if len(missing) > 0 {
		respondError(w, http.StatusUnprocessableEntity, "required fields missing: "+strings.Join(missing, ", "))
		return
	}

	recipient := h.cfg.Server.Contact.General.Email
	if recipient == "" {
		recipient = h.cfg.Server.Admin.Email
	}
	log.Printf("[contact/api] from=%s <%s> to=%s subject=%q len=%d", sub.Name, sub.Email, recipient, sub.Subject, len(sub.Message))

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"sent": true,
	})
}
