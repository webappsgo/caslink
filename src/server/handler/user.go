package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/tmpl"
)

// UserHandler handles user profile and settings pages
type UserHandler struct {
	authService  *service.AuthService
	tokenService *service.TokenService
	urlService   *service.URLService
	renderer     *tmpl.Renderer
	cfg          *config.Config
}

// NewUserHandler creates a new user handler
func NewUserHandler(
	authService *service.AuthService,
	tokenService *service.TokenService,
	urlService *service.URLService,
	renderer *tmpl.Renderer,
	cfg *config.Config,
) *UserHandler {
	return &UserHandler{
		authService:  authService,
		tokenService: tokenService,
		urlService:   urlService,
		renderer:     renderer,
		cfg:          cfg,
	}
}

// Profile renders the user profile page
func (h *UserHandler) Profile(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data := struct {
		tmpl.Data
	}{
		Data: newPageData(h.cfg, r, "Profile", user),
	}
	h.renderer.Render(w, "template/page/users/profile.html", data)
}

// Settings renders the user settings page
func (h *UserHandler) Settings(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data := struct {
		tmpl.Data
		DisplayName string
		Bio         string
	}{
		Data: newPageData(h.cfg, r, "Settings", user),
	}
	h.renderer.Render(w, "template/page/users/settings.html", data)
}

// Tokens renders the API tokens management page (GET) and handles create/revoke (POST)
func (h *UserHandler) Tokens(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()

	type pageData struct {
		tmpl.Data
		Tokens   []*service.TokenRecord
		NewToken string
		Flash    *tmpl.Flash
	}

	renderPage := func(newToken string, flash *tmpl.Flash) {
		tokens, _ := h.tokenService.ListTokens(ctx, user.ID)
		base := newPageData(h.cfg, r, "API Tokens", user)
		base.Flash = flash
		d := pageData{
			Data:     base,
			Tokens:   tokens,
			NewToken: newToken,
		}
		h.renderer.Render(w, "template/page/users/tokens.html", d)
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		action := r.PostFormValue("action")
		switch action {
		case "create":
			name := strings.TrimSpace(r.PostFormValue("token_name"))
			if name == "" {
				renderPage("", &tmpl.Flash{Type: "danger", Message: "Token name is required."})
				return
			}
			var expiresAt *time.Time
			if days := r.PostFormValue("expires_in"); days != "" && days != "0" {
				if n, err := strconv.Atoi(days); err == nil && n > 0 {
					t := time.Now().AddDate(0, 0, n)
					expiresAt = &t
				}
			}
			plain, err := h.tokenService.CreateToken(ctx, user.ID, "user", name, nil, expiresAt)
			if err != nil {
				renderPage("", &tmpl.Flash{Type: "danger", Message: "Failed to create token."})
				return
			}
			renderPage(plain, &tmpl.Flash{Type: "success", Message: "Token created. Copy it now — it will not be shown again."})
			return

		case "revoke":
			idStr := r.PostFormValue("token_id")
			tokenID, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				renderPage("", &tmpl.Flash{Type: "danger", Message: "Invalid token ID."})
				return
			}
			if err := h.tokenService.RevokeToken(ctx, tokenID, user.ID); err != nil {
				renderPage("", &tmpl.Flash{Type: "danger", Message: "Failed to revoke token."})
				return
			}
			http.Redirect(w, r, "/users/tokens", http.StatusSeeOther)
			return
		}
	}

	renderPage("", nil)
}

// Security renders the security settings page
func (h *UserHandler) Security(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data := struct {
		tmpl.Data
		TOTPEnabled      bool
		RecoveryKeyCount int
	}{
		Data:        newPageData(h.cfg, r, "Security Settings", user),
		TOTPEnabled: user.TOTPEnabled,
	}
	h.renderer.Render(w, "template/page/users/security.html", data)
}

// currentAPIUser resolves the caller of a /api/v1/users/* request. These
// routes only run BearerAuthMiddleware (see server.go), which attaches a
// *service.TokenRecord, not a session *service.User — so falling back to
// getUserFromRequest alone would 401 every Bearer-authenticated call.
// Session-authenticated callers (if any reach these routes) are still
// honored first; Bearer "user" tokens are resolved to their full User via
// authService.GetUserByID. Admin/org tokens are rejected — PART 14 scopes
// /users/* to the current user's own resources.
func (h *UserHandler) currentAPIUser(r *http.Request) (*service.User, bool) {
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

// APIProfile returns the current user's profile as JSON for GET /api/v1/users.
func (h *UserHandler) APIProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, 401, "authentication required")
		return
	}
	type profileData struct {
		ID          int64   `json:"id"`
		Username    string  `json:"username"`
		Email       string  `json:"email"`
		DisplayName *string `json:"display_name"`
	}
	respondJSON(w, 200, profileData{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
	})
}

// updateProfileRequest is the partial-update body for PATCH /api/v1/users.
// Pointer fields distinguish "not sent" (nil) from "sent empty" so callers
// can clear display_name/bio without also touching email.
type updateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	Bio         *string `json:"bio"`
	Email       *string `json:"email"`
}

// APIUpdateProfile handles PATCH /api/v1/users — partial update of the
// current Bearer-authenticated user's profile per AI.md PART 14.
func (h *UserHandler) APIUpdateProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Email == nil && req.DisplayName == nil && req.Bio == nil {
		respondError(w, http.StatusBadRequest, "No fields to update")
		return
	}

	updated, err := h.authService.UpdateUserProfile(r.Context(), user.ID, req.DisplayName, req.Bio, req.Email)
	if err != nil {
		if err == service.ErrEmailAlreadyInUse {
			respondError(w, http.StatusConflict, "Email already in use")
			return
		}
		respondError(w, http.StatusInternalServerError, "Failed to update profile")
		return
	}

	type profileData struct {
		ID          int64   `json:"id"`
		Username    string  `json:"username"`
		Email       string  `json:"email"`
		DisplayName *string `json:"display_name"`
		Bio         *string `json:"bio"`
	}
	respondJSON(w, http.StatusOK, profileData{
		ID:          updated.ID,
		Username:    updated.Username,
		Email:       updated.Email,
		DisplayName: updated.DisplayName,
		Bio:         updated.Bio,
	})
}

// createTokenRequest is the body for POST /api/v1/users/tokens.
type createTokenRequest struct {
	Name      string `json:"name"`
	ExpiresIn int    `json:"expires_in_days"`
}

// APICreateToken handles POST /api/v1/users/tokens — creates a new API
// token for the current Bearer-authenticated user. The plaintext token is
// only ever returned in this response; it is never stored or shown again.
func (h *UserHandler) APICreateToken(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		respondError(w, http.StatusBadRequest, "Token name is required")
		return
	}
	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		t := time.Now().AddDate(0, 0, req.ExpiresIn)
		expiresAt = &t
	}

	plain, err := h.tokenService.CreateToken(r.Context(), user.ID, "user", name, nil, expiresAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create token")
		return
	}

	type tokenCreatedData struct {
		Token string `json:"token"`
	}
	respondJSON(w, http.StatusCreated, tokenCreatedData{Token: plain})
}

// APIRevokeToken handles DELETE /api/v1/users/tokens/{id} — revokes one of
// the current user's own API tokens. RevokeToken itself scopes the delete
// to ownerID so a caller cannot revoke another user's token.
func (h *UserHandler) APIRevokeToken(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	idStr := chi.URLParam(r, "id")
	tokenID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid token ID")
		return
	}
	if err := h.tokenService.RevokeToken(r.Context(), tokenID, user.ID); err != nil {
		respondError(w, http.StatusNotFound, "Token not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// APITokens returns the current user's API tokens as JSON for GET /api/v1/users/tokens.
func (h *UserHandler) APITokens(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, 401, "authentication required")
		return
	}
	tokens, err := h.tokenService.ListTokens(r.Context(), user.ID)
	if err != nil {
		respondError(w, 500, "failed to list tokens")
		return
	}
	respondJSON(w, 200, tokens)
}

// APISettings returns the current user's settings as JSON for GET /api/v1/users/settings.
func (h *UserHandler) APISettings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, 401, "authentication required")
		return
	}
	type settingsData struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	respondJSON(w, 200, settingsData{
		Username: user.Username,
		Email:    user.Email,
	})
}

// APISecurity returns the current user's security settings as JSON for GET /api/v1/users/security.
func (h *UserHandler) APISecurity(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentAPIUser(r)
	if !ok {
		respondError(w, 401, "authentication required")
		return
	}
	type securityData struct {
		TOTPEnabled bool `json:"totp_enabled"`
	}
	respondJSON(w, 200, securityData{
		TOTPEnabled: user.TOTPEnabled,
	})
}

// Dashboard renders the user dashboard with their URLs
func (h *UserHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusFound)
		return
	}
	ctx := r.Context()

	urls, _ := h.urlService.ListByUser(ctx, user.ID, 50)

	base := newPageData(h.cfg, r, "Dashboard", user)
	// Surface feedback from the WebCreateURL/WebURLManage PRG redirects
	// (works without JavaScript per AI.md PART 16).
	if code := r.URL.Query().Get("created"); code != "" {
		base.Flash = &tmpl.Flash{Type: "success", Message: "Short link created: /" + code}
	} else if code := r.URL.Query().Get("deleted"); code != "" {
		base.Flash = &tmpl.Flash{Type: "success", Message: "Link deleted: " + code}
	}

	data := struct {
		tmpl.Data
		URLs interface{}
	}{
		Data: base,
		URLs: urls,
	}
	h.renderer.Render(w, "template/page/dashboard.html", data)
}

// Bulk renders the bulk import/export page — links to CSV/JSON export and a
// file-upload import form. Reuses the existing BulkHandler.Export/Import
// (session-authenticated, registered alongside this route in server.go).
func (h *UserHandler) Bulk(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/server/auth/login", http.StatusFound)
		return
	}
	base := newPageData(h.cfg, r, "Bulk Import/Export", user)
	q := r.URL.Query()
	switch {
	case q.Get("import_error") != "":
		base.Flash = &tmpl.Flash{Type: "danger", Message: q.Get("import_error")}
	case q.Get("imported") != "":
		errCount := q.Get("errors")
		msg := "Imported " + q.Get("success") + " link(s)."
		if errCount != "" && errCount != "0" {
			msg += " " + errCount + " row(s) failed."
		}
		base.Flash = &tmpl.Flash{Type: "success", Message: msg}
	}
	data := struct {
		tmpl.Data
	}{
		Data: base,
	}
	h.renderer.Render(w, "template/page/url_bulk.html", data)
}
