package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/tmpl"
)

// clickRecordWorkers caps the number of concurrent goroutines used to
// record analytics so an attacker cannot exhaust memory by replaying
// redirects. The semaphore is buffered to absorb short bursts but bounded
// at startup so a flood drops events instead of spawning unbounded
// goroutines (CLAUDE.md memory-safety rule).
var clickRecordWorkers = make(chan struct{}, 64)

// URLHandler handles URL shortening endpoints
type URLHandler struct {
	urlService       *service.URLService
	analyticsService *service.AnalyticsService
	renderer         *tmpl.Renderer
	cfg              *config.Config
}

// NewURLHandler creates a new URL handler
func NewURLHandler(urlService *service.URLService, analyticsService *service.AnalyticsService, renderer *tmpl.Renderer, cfg *config.Config) *URLHandler {
	return &URLHandler{
		urlService:       urlService,
		analyticsService: analyticsService,
		renderer:         renderer,
		cfg:              cfg,
	}
}

// CreateURL handles POST /api/v1/urls.
// Accepts both application/json (API clients) and
// application/x-www-form-urlencoded (HTML forms, progressive enhancement
// per AI.md PART 16).
func (h *URLHandler) CreateURL(w http.ResponseWriter, r *http.Request) {
	var req model.CreateURLRequest

	ct := r.Header.Get("Content-Type")
	if ct == "application/x-www-form-urlencoded" || ct == "multipart/form-data" ||
		(len(ct) > 33 && ct[:33] == "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid form data")
			return
		}
		// Accept both "url" (short) and "long_url" (form field name in dashboard).
		req.LongURL = r.FormValue("long_url")
		if req.LongURL == "" {
			req.LongURL = r.FormValue("url")
		}
		req.CustomCode = r.FormValue("custom_code")
		req.Password = r.FormValue("password")
		req.Visibility = r.FormValue("visibility")
		req.Tags = splitFormList(r.FormValue("tags"))
		req.UTMSource = r.FormValue("utm_source")
		req.UTMMedium = r.FormValue("utm_medium")
		req.UTMCampaign = r.FormValue("utm_campaign")
		req.UTMTerm = r.FormValue("utm_term")
		req.UTMContent = r.FormValue("utm_content")
		req.GeoMode = r.FormValue("geo_mode")
		req.GeoCountries = splitFormList(r.FormValue("geo_countries"))
		req.MobileURL = r.FormValue("mobile_url")
		req.DesktopURL = r.FormValue("desktop_url")
		req.TabletURL = r.FormValue("tablet_url")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	// Bearer-authenticated user/org tokens own the links they create, so the
	// caller can later edit/delete them (see checkURLOwnership below). Admin
	// tokens keep the prior anonymous (unowned) behavior.
	var url *model.URL
	var err error
	switch rec, ok := getBearerFromRequest(r); {
	case ok && strings.EqualFold(rec.OwnerType, "user"):
		url, err = h.urlService.CreateURLForUser(r.Context(), rec.OwnerID, &req)
	case ok && strings.EqualFold(rec.OwnerType, "org"):
		url, err = h.urlService.CreateURLForOrg(r.Context(), rec.OwnerID, &req)
	default:
		url, err = h.urlService.CreateURL(r.Context(), &req)
	}
	if err != nil {
		if err == model.ErrCodeAlreadyExists {
			respondError(w, http.StatusConflict, "Short code already exists")
			return
		}
		if err == model.ErrReservedWord {
			respondError(w, http.StatusBadRequest, "Short code is a reserved word")
			return
		}
		if err == model.ErrInvalidCustomCode {
			respondError(w, http.StatusBadRequest, "Invalid custom code")
			return
		}
		if errors.Is(err, model.ErrInvalidURL) {
			respondError(w, http.StatusBadRequest, "Invalid URL")
			return
		}
		respondError(w, http.StatusInternalServerError, "Failed to create URL")
		return
	}

	respondJSON(w, http.StatusCreated, url)
}

// WebCreateURL handles POST /urls — the HTML-form version of CreateURL.
// Uses the Post/Redirect/Get (PRG) pattern so the browser follows a 303
// redirect after success instead of re-submitting on back/reload.
// This makes core URL creation work without JavaScript (PART 16 PRE).
func (h *URLHandler) WebCreateURL(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	longURL := r.FormValue("long_url")
	if longURL == "" {
		longURL = r.FormValue("url")
	}
	if longURL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}
	req := &model.CreateURLRequest{
		LongURL:    longURL,
		CustomCode: r.FormValue("custom_code"),
		Password:   r.FormValue("password"),
	}
	// Associate the link with the signed-in user so it shows up on their
	// dashboard (ListByUser) and so they can edit/delete it later.
	var url *model.URL
	var err error
	if user, ok := getUserFromRequest(r); ok {
		url, err = h.urlService.CreateURLForUser(r.Context(), user.ID, req)
	} else {
		url, err = h.urlService.CreateURL(r.Context(), req)
	}
	if err != nil {
		// Redirect back to root with an error query param.
		http.Redirect(w, r, "/?error="+http.StatusText(http.StatusBadRequest), http.StatusSeeOther)
		return
	}
	// PRG: redirect to the dashboard with the new code highlighted.
	http.Redirect(w, r, "/users/dashboard?created="+url.ShortCode, http.StatusSeeOther)
}

// urlListResponse is the paginated envelope for GET /api/v1/urls.
type urlListResponse struct {
	URLs  []*model.URL `json:"urls"`
	Page  int          `json:"page"`
	Limit int          `json:"limit"`
	Total int          `json:"total"`
}

// ListURLs handles GET /api/v1/urls per AI.md PART 14 (current Bearer-
// authenticated caller's own links, paginated via ?page&limit). User tokens
// list their own links; org tokens list the org's links. Admin tokens are
// rejected since PART 14 scopes /users/* to the current caller's resources —
// admins list all links via the admin config API instead.
func (h *URLHandler) ListURLs(w http.ResponseWriter, r *http.Request) {
	rec, ok := getBearerFromRequest(r)
	if !ok || !(strings.EqualFold(rec.OwnerType, "user") || strings.EqualFold(rec.OwnerType, "org")) {
		respondError(w, http.StatusUnauthorized, "Bearer user or org token required")
		return
	}

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	limit := 250
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 250 {
			limit = n
		}
	}
	offset := (page - 1) * limit

	var urls []*model.URL
	var total int
	var err error
	if strings.EqualFold(rec.OwnerType, "org") {
		urls, err = h.urlService.ListByOrgPage(r.Context(), rec.OwnerID, limit, offset)
		if err == nil {
			total, err = h.urlService.CountByOrg(r.Context(), rec.OwnerID)
		}
	} else {
		urls, err = h.urlService.ListByUserPage(r.Context(), rec.OwnerID, limit, offset)
		if err == nil {
			total, err = h.urlService.CountByUser(r.Context(), rec.OwnerID)
		}
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list URLs")
		return
	}

	respondJSON(w, http.StatusOK, urlListResponse{
		URLs:  urls,
		Page:  page,
		Limit: limit,
		Total: total,
	})
}

// GetURL handles GET /api/v1/urls/{code}
func (h *URLHandler) GetURL(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "Short code required")
		return
	}

	url, err := h.urlService.GetURLByCode(r.Context(), code)
	if err != nil {
		if err == model.ErrURLNotFound {
			respondError(w, http.StatusNotFound, "URL not found")
			return
		}
		if err == model.ErrURLExpired {
			respondError(w, http.StatusGone, "URL has expired")
			return
		}
		respondError(w, http.StatusInternalServerError, "Failed to get URL")
		return
	}
	if !h.canViewURL(r, url) {
		respondError(w, http.StatusNotFound, "URL not found")
		return
	}

	respondJSON(w, http.StatusOK, url)
}

// canViewURL reports whether the caller may read a "private" link's details/
// stats (GetURL, Stats): admins and the owning user/org may; anyone may read
// a "public" link (the default), matching the prior open-by-code behavior
// (IDEA.md line 25/37 visibility).
func (h *URLHandler) canViewURL(r *http.Request, url *model.URL) bool {
	rec, ok := getBearerFromRequest(r)
	if !ok {
		return service.CanView(url, false, "", 0)
	}
	return service.CanView(url, strings.EqualFold(rec.OwnerType, "admin"), rec.OwnerType, rec.OwnerID)
}

// RedirectURL handles GET /{code} - redirects to the long URL. Applies the
// link's options (IDEA.md line 25/37): geo-restriction may block the
// redirect entirely, device targeting picks the destination, and UTM
// passthrough is appended to whichever destination is chosen.
func (h *URLHandler) RedirectURL(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		http.NotFound(w, r)
		return
	}

	// On a verified custom domain, resolve the code only within that domain
	// owner's links (PART 36): a custom domain must serve its owner's short
	// links and never another account's. On the main host, resolve globally.
	var url *model.URL
	var err error
	if cd, ok := getCustomDomainFromRequest(r); ok {
		url, err = h.urlService.GetURLByCodeForOwner(r.Context(), code, cd.OwnerType, cd.OwnerID)
	} else {
		url, err = h.urlService.GetURLByCode(r.Context(), code)
	}
	if err != nil {
		if err == model.ErrURLNotFound || err == model.ErrURLExpired {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Password-protected links must be unlocked before any redirect. This
	// enforces the advertised "password protection" link option (IDEA.md);
	// without it the stored Argon2id hash would be decorative and anyone
	// with the short code could follow the link.
	if service.URLRequiresPassword(url) {
		if !h.unlockURLOrPrompt(w, r, url) {
			return
		}
	}

	ipAddress := realClientIP(r)
	userAgent := r.UserAgent()
	referrer := r.Referer()

	// Geo-restriction: only enforced when the caller's country is positively
	// known; an unresolved lookup (GeoIP disabled, private/loopback IP) never
	// blocks, per IDEA.md's "GeoIP is a risk signal, not identity" caution.
	if url.GeoMode != "" && url.GeoMode != "none" {
		country := h.urlService.LookupCountry(ipAddress)
		if !service.GeoAllowed(url, country) {
			http.NotFound(w, r)
			return
		}
	}

	dest := service.SelectDestination(url, service.DetectDeviceType(userAgent))
	dest = service.ApplyUTM(dest, url)

	// Record click (async - don't block redirect). Cap concurrency with a
	// semaphore so a flood of redirects cannot spawn unbounded goroutines,
	// and use a detached context with a deadline so the database write is
	// not cancelled the moment the response completes.
	urlID := url.ID
	select {
	case clickRecordWorkers <- struct{}{}:
		go func() {
			defer func() { <-clickRecordWorkers }()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.urlService.RecordClick(ctx, urlID, ipAddress, userAgent, referrer)
		}()
	default:
		// Worker pool saturated — drop the click rather than block the
		// redirect or queue indefinitely. Analytics are best-effort.
	}

	// Redirect to the (possibly device-targeted, UTM-tagged) destination
	http.Redirect(w, r, dest, http.StatusFound)
}

// unlockURLOrPrompt gates a password-protected redirect. It returns true only
// when the caller supplied the correct password (via a POSTed form field or a
// ?password= query param). Otherwise it responds — a JSON 401 for API clients,
// an HTML unlock prompt for browsers (progressive enhancement / no-JS per
// AI.md PART 16) — and returns false so the caller must stop.
func (h *URLHandler) unlockURLOrPrompt(w http.ResponseWriter, r *http.Request, u *model.URL) bool {
	var provided string
	var submitted bool
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err == nil {
			provided = r.PostFormValue("password")
			submitted = true
		}
	} else if q := r.URL.Query().Get("password"); q != "" {
		provided = q
		submitted = true
	}

	if submitted && service.VerifyURLPassword(u, provided) {
		return true
	}

	if r.Header.Get("Accept") == "application/json" {
		respondError(w, http.StatusUnauthorized, "This link is password protected")
		return false
	}

	data := struct {
		tmpl.Data
		Code  string
		Error string
	}{
		Data: newPageData(h.cfg, r, "Password Required", nil),
		Code: u.ShortCode,
	}
	if submitted {
		data.Error = "Incorrect password. Please try again."
	}
	h.renderer.Render(w, "template/page/link/password.html", data)
	return false
}

// checkURLOwnership verifies the Bearer-authenticated caller is allowed to
// mutate the given URL: admin tokens always pass; user tokens must match the
// URL's owning user_id; org tokens must match the URL's owning org_id (the
// org token's OwnerID is the org's ID — see service/token.go). Links with no
// owner (created before ownership was tracked) can only be mutated by an
// admin token. Returns false (after writing the response) when the caller
// must not proceed.
func (h *URLHandler) checkURLOwnership(w http.ResponseWriter, r *http.Request, code string) bool {
	rec, ok := getBearerFromRequest(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Bearer token required")
		return false
	}
	if strings.EqualFold(rec.OwnerType, "admin") {
		return true
	}

	existing, err := h.urlService.GetURLByCodeAny(r.Context(), code)
	if err != nil {
		if err == model.ErrURLNotFound {
			respondError(w, http.StatusNotFound, "URL not found")
			return false
		}
		respondError(w, http.StatusInternalServerError, "Failed to load URL")
		return false
	}
	if strings.EqualFold(rec.OwnerType, "org") {
		if existing.OrgID == nil || *existing.OrgID != rec.OwnerID {
			respondError(w, http.StatusNotFound, "URL not found")
			return false
		}
		return true
	}
	if existing.UserID == nil || *existing.UserID != rec.OwnerID {
		respondError(w, http.StatusNotFound, "URL not found")
		return false
	}
	return true
}

// UpdateURL handles PATCH /api/v1/urls/{code}. Uses PATCH (not PUT) per
// AI.md PART 14's partial-update convention (e.g. `PATCH /api/{api_version}/
// users`) — callers only send the fields they want changed. Bearer callers
// may only mutate links they own (see checkURLOwnership).
func (h *URLHandler) UpdateURL(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "Short code required")
		return
	}
	if !h.checkURLOwnership(w, r, code) {
		return
	}

	var req model.UpdateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	url, err := h.urlService.UpdateURL(r.Context(), code, &req)
	if err != nil {
		if err == model.ErrURLNotFound {
			respondError(w, http.StatusNotFound, "URL not found")
			return
		}
		respondError(w, http.StatusBadRequest, "Failed to update URL")
		return
	}

	respondJSON(w, http.StatusOK, url)
}

// DeleteURL handles DELETE /api/v1/urls/{code} per AI.md PART 16
// (`DELETE /api/{api_version}/links/{code}`). Bearer callers may only
// delete links they own (see checkURLOwnership).
func (h *URLHandler) DeleteURL(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "Short code required")
		return
	}
	if !h.checkURLOwnership(w, r, code) {
		return
	}

	if err := h.urlService.DeleteURL(r.Context(), code); err != nil {
		if err == model.ErrURLNotFound {
			respondError(w, http.StatusNotFound, "URL not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "Failed to delete URL")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Stats handles GET /api/v1/urls/{code}/stats
func (h *URLHandler) Stats(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "Short code required")
		return
	}

	url, err := h.urlService.GetURLByCodeAny(r.Context(), code)
	if err != nil {
		respondError(w, http.StatusNotFound, "URL not found")
		return
	}
	if !h.canViewURL(r, url) {
		respondError(w, http.StatusNotFound, "URL not found")
		return
	}

	stats, err := h.analyticsService.GetURLStats(r.Context(), code)
	if err != nil {
		respondError(w, http.StatusNotFound, "URL not found or no stats available")
		return
	}

	respondJSON(w, http.StatusOK, stats)
}

// ownedURLForUser fetches a URL by code and verifies the given user owns it
// (mirrors checkURLOwnership's rules but for session-authenticated web
// requests instead of Bearer tokens). Returns (nil, false) after writing a
// 404 when the code does not exist or belongs to someone else.
func (h *URLHandler) ownedURLForUser(w http.ResponseWriter, r *http.Request, code string, userID int64) (*model.URL, bool) {
	existing, err := h.urlService.GetURLByCodeAny(r.Context(), code)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	if existing.UserID == nil || *existing.UserID != userID {
		http.NotFound(w, r)
		return nil, false
	}
	return existing, true
}

// WebURLManage handles GET/POST /users/urls/{code} — the per-link
// management page: stats, QR code display, an edit form, and a delete
// form. Everything works without JavaScript (PART 16 progressive
// enhancement); GET renders the page, POST re-renders it in place with a
// Flash after applying the requested action (update or delete).
func (h *URLHandler) WebURLManage(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusFound)
		return
	}
	code := chi.URLParam(r, "code")
	if code == "" {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if _, ok := h.ownedURLForUser(w, r, code, user.ID); !ok {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}
		switch r.PostFormValue("action") {
		case "delete":
			if err := h.urlService.DeleteURL(r.Context(), code); err != nil {
				h.renderURLManage(w, r, user, code, &tmpl.Flash{Type: "danger", Message: "Failed to delete link."})
				return
			}
			http.Redirect(w, r, "/users/dashboard?deleted="+code, http.StatusSeeOther)
			return

		default: // "update"
			req := &model.UpdateURLRequest{}
			if v := strings.TrimSpace(r.PostFormValue("long_url")); v != "" {
				req.LongURL = &v
			}
			title := strings.TrimSpace(r.PostFormValue("title"))
			req.Title = &title
			desc := strings.TrimSpace(r.PostFormValue("description"))
			req.Description = &desc
			password := r.PostFormValue("password")
			if r.PostFormValue("clear_password") == "1" {
				empty := ""
				req.Password = &empty
			} else if password != "" {
				req.Password = &password
			}
			if r.PostFormValue("clear_expiry") == "1" {
				var zero time.Time
				req.ExpiresAt = &zero
			} else if v := r.PostFormValue("expires_at"); v != "" {
				if t, err := time.Parse("2006-01-02T15:04", v); err == nil {
					req.ExpiresAt = &t
				}
			}

			if v := r.PostFormValue("visibility"); v != "" {
				req.Visibility = &v
			}
			tags := splitFormList(r.PostFormValue("tags"))
			req.Tags = &tags
			if v := strings.TrimSpace(r.PostFormValue("utm_source")); v != "" || r.Form.Has("utm_source") {
				req.UTMSource = &v
			}
			if v := strings.TrimSpace(r.PostFormValue("utm_medium")); v != "" || r.Form.Has("utm_medium") {
				req.UTMMedium = &v
			}
			if v := strings.TrimSpace(r.PostFormValue("utm_campaign")); v != "" || r.Form.Has("utm_campaign") {
				req.UTMCampaign = &v
			}
			if v := strings.TrimSpace(r.PostFormValue("utm_term")); v != "" || r.Form.Has("utm_term") {
				req.UTMTerm = &v
			}
			if v := strings.TrimSpace(r.PostFormValue("utm_content")); v != "" || r.Form.Has("utm_content") {
				req.UTMContent = &v
			}
			if v := r.PostFormValue("geo_mode"); v != "" {
				req.GeoMode = &v
			}
			geoCountries := splitFormList(r.PostFormValue("geo_countries"))
			req.GeoCountries = &geoCountries
			if v := strings.TrimSpace(r.PostFormValue("mobile_url")); v != "" || r.Form.Has("mobile_url") {
				req.MobileURL = &v
			}
			if v := strings.TrimSpace(r.PostFormValue("desktop_url")); v != "" || r.Form.Has("desktop_url") {
				req.DesktopURL = &v
			}
			if v := strings.TrimSpace(r.PostFormValue("tablet_url")); v != "" || r.Form.Has("tablet_url") {
				req.TabletURL = &v
			}

			if _, err := h.urlService.UpdateURL(r.Context(), code, req); err != nil {
				log.Printf("[url] update link %q failed: %v", code, err)
				h.renderURLManage(w, r, user, code, &tmpl.Flash{Type: "danger", Message: "Failed to update link"})
				return
			}
			h.renderURLManage(w, r, user, code, &tmpl.Flash{Type: "success", Message: "Link updated."})
			return
		}
	}

	h.renderURLManage(w, r, user, code, nil)
}

// renderURLManage loads the URL (with an ownership check), its stats, and
// renders the manage page. Writes 404 itself when the link does not exist
// or is not owned by user.
func (h *URLHandler) renderURLManage(w http.ResponseWriter, r *http.Request, user *service.User, code string, flash *tmpl.Flash) {
	url, ok := h.ownedURLForUser(w, r, code, user.ID)
	if !ok {
		return
	}
	stats, _ := h.analyticsService.GetURLStats(r.Context(), code)

	base := newPageData(h.cfg, r, "Manage "+code, user)
	base.Flash = flash
	data := struct {
		tmpl.Data
		URL   *model.URL
		Stats *service.URLStats
	}{
		Data:  base,
		URL:   url,
		Stats: stats,
	}
	h.renderer.Render(w, "template/page/url_manage.html", data)
}

// respondJSON and respondError are defined in helpers.go and shared across
// every handler. They emit the canonical APIResponse envelope per
// AI.md PART 9 ("Response Format").
