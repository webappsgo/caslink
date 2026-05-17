package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/tmpl"
)

// UserSecurityHandler handles user security-related routes.
type UserSecurityHandler struct {
	authService  *service.AuthService
	totpService  *service.TOTPService
	qrService    *service.QRService
	emailService *service.EmailService
	renderer     *tmpl.Renderer
	config       *config.Config
}

// NewUserSecurityHandler creates a new user security handler.
func NewUserSecurityHandler(authService *service.AuthService, totpService *service.TOTPService, qrService *service.QRService, emailService *service.EmailService, renderer *tmpl.Renderer, cfg *config.Config) *UserSecurityHandler {
	return &UserSecurityHandler{
		authService:  authService,
		totpService:  totpService,
		qrService:    qrService,
		emailService: emailService,
		renderer:     renderer,
		config:       cfg,
	}
}

// Password renders the password change page and handles password updates
func (h *UserSecurityHandler) Password(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.renderPasswordPage(w, r, user)
	case http.MethodPost:
		h.handlePasswordChange(w, r, user)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// renderPasswordPage renders the password change form.
func (h *UserSecurityHandler) renderPasswordPage(w http.ResponseWriter, r *http.Request, user *service.User) {
	data := newPageData(h.config, r, "Change Password", user)
	h.renderer.Render(w, "template/page/users/security/password.html", data)
}

// handlePasswordChange processes password change requests
func (h *UserSecurityHandler) handlePasswordChange(w http.ResponseWriter, r *http.Request, user *service.User) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	currentPassword := strings.TrimSpace(r.FormValue("current_password"))
	newPassword := strings.TrimSpace(r.FormValue("new_password"))
	confirmPassword := strings.TrimSpace(r.FormValue("confirm_password"))

	// Validate input
	if currentPassword == "" || newPassword == "" || confirmPassword == "" {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	if newPassword != confirmPassword {
		http.Error(w, "New passwords do not match", http.StatusBadRequest)
		return
	}

	if len(newPassword) < 8 {
		http.Error(w, "Password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	// Verify current password
	if err := h.authService.VerifyPassword(user.ID, currentPassword); err != nil {
		http.Error(w, "Current password is incorrect", http.StatusUnauthorized)
		return
	}

	// Change password
	if err := h.authService.ChangePassword(user.ID, newPassword); err != nil {
		http.Error(w, "Failed to change password: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Success - redirect back to security page
	http.Redirect(w, r, "/users/security?msg=password_changed", http.StatusSeeOther)
}

// Sessions renders the active sessions management page
func (h *UserSecurityHandler) Sessions(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.renderSessionsPage(w, r, user)
	case http.MethodPost:
		h.handleSessionRevocation(w, r, user)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// sessionView is the view model for a single session row.
type sessionView struct {
	ID         string
	ShortID    string
	IPAddress  string
	UserAgent  string
	CreatedAt  string
	LastActive string
	IsCurrent  bool
}

// renderSessionsPage renders the active sessions list.
func (h *UserSecurityHandler) renderSessionsPage(w http.ResponseWriter, r *http.Request, user *service.User) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sessions, err := h.authService.GetUserSessions(ctx, user.ID, "user")
	if err != nil {
		http.Error(w, "Failed to load sessions", http.StatusInternalServerError)
		return
	}

	currentSessionCookie, _ := r.Cookie("user_session")
	currentSessionID := ""
	if currentSessionCookie != nil {
		currentSessionID = currentSessionCookie.Value
	}

	views := make([]sessionView, 0, len(sessions))
	for _, s := range sessions {
		shortID := s.ID
		if len(shortID) > 8 {
			shortID = shortID[len(shortID)-8:]
		}
		lastActive := s.LastActive.Format("2006-01-02 15:04")
		if s.LastActive.IsZero() {
			lastActive = s.CreatedAt.Format("2006-01-02 15:04")
		}
		views = append(views, sessionView{
			ID:         s.ID,
			ShortID:    shortID,
			IPAddress:  s.IPAddress,
			UserAgent:  s.UserAgent,
			CreatedAt:  s.CreatedAt.Format("2006-01-02 15:04"),
			LastActive: lastActive,
			IsCurrent:  s.ID == currentSessionID,
		})
	}

	data := struct {
		tmpl.Data
		Sessions []sessionView
	}{
		Data:     newPageData(h.config, r, "Active Sessions", user),
		Sessions: views,
	}
	h.renderer.Render(w, "template/page/users/security/sessions.html", data)
}

// handleSessionRevocation revokes a specific session
func (h *UserSecurityHandler) handleSessionRevocation(w http.ResponseWriter, r *http.Request, user *service.User) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	sessionID := strings.TrimSpace(r.FormValue("session_id"))
	if sessionID == "" {
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	// Revoke the session
	err := h.authService.RevokeSession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "Failed to revoke session", http.StatusInternalServerError)
		return
	}

	// Success - redirect back
	http.Redirect(w, r, "/users/security/sessions?msg=session_revoked", http.StatusSeeOther)
}

// TwoFactor renders the 2FA (TOTP) management page
func (h *UserSecurityHandler) TwoFactor(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.renderTwoFactorPage(w, r, user)
	case http.MethodPost:
		h.handleTwoFactorAction(w, r, user)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// renderTwoFactorPage renders the 2FA setup/management page.
func (h *UserSecurityHandler) renderTwoFactorPage(w http.ResponseWriter, r *http.Request, user *service.User) {
	has2FA := h.totpService.HasTOTP(user.ID)
	recoveryKeysRemaining := 0
	if has2FA {
		recoveryKeysRemaining, _ = h.totpService.GetRemainingRecoveryKeyCount(user.ID)
	}

	data := struct {
		tmpl.Data
		TOTPEnabled           bool
		QRDataURL             string
		TOTPSecret            string
		RecoveryKeysRemaining int
	}{
		Data:                  newPageData(h.config, r, "Two-Factor Authentication", user),
		TOTPEnabled:           has2FA,
		RecoveryKeysRemaining: recoveryKeysRemaining,
	}
	h.renderer.Render(w, "template/page/users/security/2fa.html", data)
}

// handleTwoFactorAction handles 2FA enable/disable/verify actions
func (h *UserSecurityHandler) handleTwoFactorAction(w http.ResponseWriter, r *http.Request, user *service.User) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	action := strings.TrimSpace(r.FormValue("action"))

	switch action {
	case "enable":
		h.handleTOTPEnable(w, r, user)
	case "disable":
		h.handleTOTPDisable(w, r, user)
	case "verify":
		h.handleTOTPVerify(w, r, user)
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
	}
}

// handleTOTPEnable generates TOTP secret and shows QR code per PART 23 line 20082-20106
func (h *UserSecurityHandler) handleTOTPEnable(w http.ResponseWriter, r *http.Request, user *service.User) {
	// Step 1: Verify password (per PART 23 line 20141-20142)
	password := strings.TrimSpace(r.FormValue("password"))
	if password == "" {
		// Show password confirmation form
		h.renderTOTPPasswordConfirm(w, r, user)
		return
	}
	
	// Verify password
	if err := h.authService.VerifyPassword(user.ID, password); err != nil {
		http.Error(w, "Incorrect password", http.StatusUnauthorized)
		return
	}
	
	// Step 2: Generate TOTP secret per PART 23 line 20094-20106
	secret, err := h.totpService.GenerateTOTPSecret()
	if err != nil {
		http.Error(w, "Failed to generate TOTP secret", http.StatusInternalServerError)
		return
	}
	
	// Generate QR code URL
	issuer := "Caslink"
	accountName := user.Username
	qrURL := h.totpService.GenerateQRCodeURL(secret, issuer, accountName)
	
	// Show QR code and manual entry key
	h.renderTOTPSetup(w, r, user, secret, qrURL)
}

// renderTOTPPasswordConfirm renders the password-confirm step before generating a TOTP secret.
func (h *UserSecurityHandler) renderTOTPPasswordConfirm(w http.ResponseWriter, r *http.Request, user *service.User) {
	data := struct {
		tmpl.Data
		TOTPEnabled           bool
		QRDataURL             string
		TOTPSecret            string
		RecoveryKeysRemaining int
	}{
		Data: newPageData(h.config, r, "Enable Two-Factor Authentication", user),
	}
	h.renderer.Render(w, "template/page/users/security/2fa.html", data)
}

// renderTOTPSetup renders the QR code and code-entry step.
func (h *UserSecurityHandler) renderTOTPSetup(w http.ResponseWriter, r *http.Request, user *service.User, secret, qrURL string) {
	qrImage, err := h.qrService.GenerateQRCodeForText(qrURL, 200)
	if err != nil {
		http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
		return
	}

	qrDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrImage)

	data := struct {
		tmpl.Data
		TOTPEnabled           bool
		QRDataURL             string
		TOTPSecret            string
		RecoveryKeysRemaining int
	}{
		Data:      newPageData(h.config, r, "Enable Two-Factor Authentication", user),
		QRDataURL: qrDataURL,
		TOTPSecret: secret,
	}
	h.renderer.Render(w, "template/page/users/security/2fa.html", data)
}

// handleTOTPVerify verifies TOTP code and enables 2FA per PART 23 line 20140-20147
func (h *UserSecurityHandler) handleTOTPVerify(w http.ResponseWriter, r *http.Request, user *service.User) {
	secret := strings.TrimSpace(r.FormValue("secret"))
	code := strings.TrimSpace(r.FormValue("code"))
	
	if secret == "" || code == "" {
		http.Error(w, "Missing secret or code", http.StatusBadRequest)
		return
	}
	
	// Verify TOTP code
	if !h.totpService.VerifyTOTPCode(secret, code) {
		http.Error(w, "Invalid verification code. Please try again.", http.StatusBadRequest)
		return
	}
	
	// Enable TOTP and generate recovery keys per PART 23 line 20145-20147
	recoveryKeys, err := h.totpService.EnableTOTP(user.ID, secret)
	if err != nil {
		http.Error(w, "Failed to enable 2FA: "+err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Show recovery keys per PART 23 line 20032-20055
	h.renderRecoveryKeys(w, r, user, recoveryKeys)
}

// renderRecoveryKeys displays recovery keys per PART 23 line 20032-20055.
// Keys are shown exactly once; the page must not be re-rendered from a bookmark.
func (h *UserSecurityHandler) renderRecoveryKeys(w http.ResponseWriter, r *http.Request, user *service.User, keys []string) {
	keysJSON, err := json.Marshal(keys)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	type recoveryKeysData struct {
		tmpl.Data
		Keys    []string
		KeysJSON template.JS
	}
	data := recoveryKeysData{
		Data:    newPageData(h.config, r, "Save Recovery Keys", user),
		Keys:    keys,
		KeysJSON: template.JS(keysJSON),
	}
	h.renderer.Render(w, "template/page/users/security/recovery-keys.html", data)
}

// handleTOTPDisable disables 2FA (requires password confirmation)
func (h *UserSecurityHandler) handleTOTPDisable(w http.ResponseWriter, r *http.Request, user *service.User) {
	password := strings.TrimSpace(r.FormValue("password"))
	if password == "" {
		http.Error(w, "Password required to disable 2FA", http.StatusBadRequest)
		return
	}
	
	// Verify password
	if err := h.authService.VerifyPassword(user.ID, password); err != nil {
		http.Error(w, "Incorrect password", http.StatusUnauthorized)
		return
	}
	
	// Disable TOTP
	if err := h.totpService.DisableTOTP(user.ID); err != nil {
		http.Error(w, "Failed to disable 2FA", http.StatusInternalServerError)
		return
	}
	
	// Send 2FA disabled email notification per PART 26
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		clientIP = strings.Split(forwarded, ",")[0]
	}
	
	// Note: Password changed template used for 2FA changes per PART 26
	// In production, would use a dedicated 2fa_disabled template
	_ = h.emailService.SendPasswordChanged(user.Email, user.Username, clientIP, "2FA disabled")
	
	http.Redirect(w, r, "/users/security?msg=2fa_disabled", http.StatusSeeOther)
}

// Passkeys renders the passkey/WebAuthn management page
func (h *UserSecurityHandler) Passkeys(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.renderPasskeysPage(w, r, user)
	case http.MethodPost:
		h.handlePasskeyAction(w, r, user)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// renderPasskeysPage renders the passkey management page.
func (h *UserSecurityHandler) renderPasskeysPage(w http.ResponseWriter, r *http.Request, user *service.User) {
	data := newPageData(h.config, r, "Passkeys", user)
	h.renderer.Render(w, "template/page/users/security/passkeys.html", data)
}

// Passkey represents a WebAuthn credential
type Passkey struct {
	ID        string
	Name      string
	CreatedAt string
	LastUsed  string
}

// handlePasskeyAction handles passkey registration/deletion
func (h *UserSecurityHandler) handlePasskeyAction(w http.ResponseWriter, r *http.Request, user *service.User) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	action := strings.TrimSpace(r.FormValue("action"))

	switch action {
	case "register":
		// WebAuthn registration requires browser API and JavaScript
		// Cannot be implemented server-side only
		// Placeholder response
		http.Error(w, "Passkey registration requires JavaScript. This will be implemented when the frontend is complete per PART 17.", http.StatusNotImplemented)
		
	case "delete":
		passkeyID := strings.TrimSpace(r.FormValue("passkey_id"))
		if passkeyID == "" {
			http.Error(w, "Passkey ID required", http.StatusBadRequest)
			return
		}
		
		// Delete passkey from database
		// Note: Table exists per PART 23 line 7045-7058
		// For now, placeholder response
		http.Error(w, "Passkey deletion will be implemented when WebAuthn registration is complete.", http.StatusNotImplemented)
		
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
	}
}

// Recovery renders the recovery keys management page
func (h *UserSecurityHandler) Recovery(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.renderRecoveryPage(w, r, user)
	case http.MethodPost:
		h.handleRecoveryAction(w, r, user)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// renderRecoveryPage renders the recovery keys status page.
func (h *UserSecurityHandler) renderRecoveryPage(w http.ResponseWriter, r *http.Request, user *service.User) {
	hasMFA := h.totpService.HasTOTP(user.ID)

	type recoveryPageData struct {
		tmpl.Data
		HasMFA                bool
		RecoveryKeysRemaining int
	}

	var remaining int
	if hasMFA {
		remaining, _ = h.totpService.GetRemainingRecoveryKeyCount(user.ID)
	}

	data := recoveryPageData{
		Data:                  newPageData(h.config, r, "Recovery Keys", user),
		HasMFA:                hasMFA,
		RecoveryKeysRemaining: remaining,
	}
	h.renderer.Render(w, "template/page/users/security/recovery.html", data)
}

// handleRecoveryAction handles recovery key regeneration
func (h *UserSecurityHandler) handleRecoveryAction(w http.ResponseWriter, r *http.Request, user *service.User) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	action := strings.TrimSpace(r.FormValue("action"))

	switch action {
	case "regenerate":
		// Regenerate recovery keys (requires password confirmation)
		password := strings.TrimSpace(r.FormValue("password"))
		if password == "" {
			http.Error(w, "Password required to regenerate recovery keys", http.StatusBadRequest)
			return
		}
		
		// Verify password
		if err := h.authService.VerifyPassword(user.ID, password); err != nil {
			http.Error(w, "Incorrect password", http.StatusUnauthorized)
			return
		}
		
		// Check if user has 2FA enabled
		if !h.totpService.HasTOTP(user.ID) {
			http.Error(w, "2FA must be enabled to regenerate recovery keys", http.StatusBadRequest)
			return
		}
		
		// Get current TOTP secret
		secret, err := h.totpService.GetTOTPSecret(user.ID)
		if err != nil {
			http.Error(w, "Failed to get TOTP secret", http.StatusInternalServerError)
			return
		}
		
		// Regenerate recovery keys (this replaces old ones)
		newKeys, err := h.totpService.EnableTOTP(user.ID, secret)
		if err != nil {
			http.Error(w, "Failed to regenerate recovery keys", http.StatusInternalServerError)
			return
		}
		
		// Show new recovery keys
		h.renderRecoveryKeys(w, r, user, newKeys)
		
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
	}
}
