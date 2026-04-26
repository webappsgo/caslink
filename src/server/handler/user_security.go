package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// UserSecurityHandler handles user security-related routes
type UserSecurityHandler struct {
	authService  *service.AuthService
	totpService  *service.TOTPService
	qrService    *service.QRService
	emailService *service.EmailService
}

// NewUserSecurityHandler creates a new user security handler
func NewUserSecurityHandler(authService *service.AuthService, totpService *service.TOTPService, qrService *service.QRService, emailService *service.EmailService) *UserSecurityHandler {
	return &UserSecurityHandler{
		authService:  authService,
		totpService:  totpService,
		qrService:    qrService,
		emailService: emailService,
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

// renderPasswordPage renders the password change form
func (h *UserSecurityHandler) renderPasswordPage(w http.ResponseWriter, r *http.Request, user *service.User) {
	// TODO: Render password change HTML page per PART 17
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `
		<h1>Change Password</h1>
		<p>User: %s</p>
		<form method="POST" action="/user/security/password">
			<div>
				<label>Current Password:</label>
				<input type="password" name="current_password" required />
			</div>
			<div>
				<label>New Password:</label>
				<input type="password" name="new_password" required />
			</div>
			<div>
				<label>Confirm Password:</label>
				<input type="password" name="confirm_password" required />
			</div>
			<button type="submit">Change Password</button>
		</form>
		<p><a href="/user/security">Back to Security</a></p>
	`, user.Username)
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
	http.Redirect(w, r, "/user/security?msg=password_changed", http.StatusSeeOther)
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

// renderSessionsPage renders the active sessions list
func (h *UserSecurityHandler) renderSessionsPage(w http.ResponseWriter, r *http.Request, user *service.User) {
	// Get active sessions from auth service
	sessions, err := h.authService.GetUserSessions(r.Context(), user.ID, "user")
	if err != nil {
		http.Error(w, "Failed to load sessions", http.StatusInternalServerError)
		return
	}
	
	// Get current session ID to identify it
	currentSessionCookie, _ := r.Cookie("user_session")
	currentSessionID := ""
	if currentSessionCookie != nil {
		currentSessionID = currentSessionCookie.Value
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	
	html := fmt.Sprintf(`
		<h1>Active Sessions</h1>
		<p>User: %s</p>
		<p>Manage your active login sessions across all devices.</p>
		<ul>
	`, user.Username)
	
	for _, session := range sessions {
		isCurrent := session.ID == currentSessionID
		if isCurrent {
			html += fmt.Sprintf(`
				<li>
					<strong>Current Session (This Device)</strong><br>
					Created: %s<br>
					Expires: %s<br>
					<em>Cannot revoke current session</em>
				</li>
			`, session.CreatedAt.Format("2006-01-02 15:04:05"), session.ExpiresAt.Format("2006-01-02 15:04:05"))
		} else {
			html += fmt.Sprintf(`
				<li>
					Session ID: %s...<br>
					Created: %s<br>
					Expires: %s<br>
					<form method="POST" action="/user/security/sessions" style="display:inline;">
						<input type="hidden" name="session_id" value="%s" />
						<button type="submit">Revoke</button>
					</form>
				</li>
			`, session.ID[:8], session.CreatedAt.Format("2006-01-02 15:04:05"), 
			   session.ExpiresAt.Format("2006-01-02 15:04:05"), session.ID)
		}
	}
	
	html += `
		</ul>
		<hr>
		<form method="POST" action="/user/security/sessions/revoke-all">
			<button type="submit" onclick="return confirm('Revoke all other sessions? You will stay logged in on this device.')">Revoke All Other Sessions</button>
		</form>
		<p><a href="/user/security">Back to Security</a></p>
	`
	
	fmt.Fprint(w, html)
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
	http.Redirect(w, r, "/user/security/sessions?msg=session_revoked", http.StatusSeeOther)
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

// renderTwoFactorPage renders the 2FA setup/management page
func (h *UserSecurityHandler) renderTwoFactorPage(w http.ResponseWriter, r *http.Request, user *service.User) {
	// Check if user has 2FA enabled
	has2FA := h.totpService.HasTOTP(user.ID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	
	if has2FA {
		fmt.Fprintf(w, `
			<h1>Two-Factor Authentication (TOTP)</h1>
			<p>User: %s</p>
			<p>Status: <strong>Enabled</strong> ✅</p>
			<div>
				<h2>Disable 2FA</h2>
				<p>This will remove two-factor authentication from your account.</p>
				<form method="POST" action="/user/security/2fa">
					<input type="hidden" name="action" value="disable" />
					<div>
						<label>Confirm your password:</label>
						<input type="password" name="password" required />
					</div>
					<button type="submit">Disable 2FA</button>
				</form>
			</div>
			<p><a href="/user/security">Back to Security</a></p>
		`, user.Username)
	} else {
		fmt.Fprintf(w, `
			<h1>Two-Factor Authentication (TOTP)</h1>
			<p>User: %s</p>
			<p>Status: Not Enabled</p>
			<div>
				<h2>Enable 2FA</h2>
				<p>Secure your account with time-based one-time passwords (TOTP)</p>
				<form method="POST" action="/user/security/2fa">
					<input type="hidden" name="action" value="enable" />
					<button type="submit">Enable 2FA</button>
				</form>
			</div>
			<p><a href="/user/security">Back to Security</a></p>
		`, user.Username)
	}
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

// renderTOTPPasswordConfirm renders password confirmation form
func (h *UserSecurityHandler) renderTOTPPasswordConfirm(w http.ResponseWriter, r *http.Request, user *service.User) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `
		<h1>Enable Two-Factor Authentication</h1>
		<h2>Step 1: Confirm Your Password</h2>
		<form method="POST" action="/user/security/2fa">
			<input type="hidden" name="action" value="enable" />
			<div>
				<label>Password:</label>
				<input type="password" name="password" required />
			</div>
			<button type="submit">Continue</button>
		</form>
		<p><a href="/user/security/2fa">Back</a></p>
	`)
}

// renderTOTPSetup renders QR code and manual key per PART 23 line 20094-20106
func (h *UserSecurityHandler) renderTOTPSetup(w http.ResponseWriter, r *http.Request, user *service.User, secret, qrURL string) {
	// Generate QR code image
	qrImage, err := h.qrService.GenerateQRCodeForText(qrURL, 200)
	if err != nil {
		http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
		return
	}
	
	// Encode as base64 for inline display
	qrBase64 := base64.StdEncoding.EncodeToString(qrImage)
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `
		<h1>Enable Two-Factor Authentication</h1>
		<h2>Step 2: Scan QR Code</h2>
		<p>Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.)</p>
		<div>
			<img src="data:image/png;base64,%s" alt="QR Code" />
		</div>
		<p><strong>Can't scan? Manual key:</strong> %s</p>
		<button onclick="navigator.clipboard.writeText('%s')">Copy Key</button>
		
		<h2>Step 3: Enter Verification Code</h2>
		<form method="POST" action="/user/security/2fa">
			<input type="hidden" name="action" value="verify" />
			<input type="hidden" name="secret" value="%s" />
			<div>
				<label>6-digit code from your app:</label>
				<input type="text" name="code" maxlength="6" pattern="[0-9]{6}" required />
			</div>
			<button type="submit">Verify and Enable</button>
		</form>
		<p><a href="/user/security/2fa">Cancel</a></p>
	`, qrBase64, secret, secret, secret)
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

// renderRecoveryKeys displays recovery keys per PART 23 line 20032-20055
func (h *UserSecurityHandler) renderRecoveryKeys(w http.ResponseWriter, r *http.Request, user *service.User, keys []string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	
	// Format keys in 2 columns per spec
	keysHTML := ""
	for i := 0; i < len(keys); i++ {
		keysHTML += fmt.Sprintf("%d. %s    ", i+1, keys[i])
		if i == 4 {
			keysHTML += "<br>"
		}
	}
	
	fmt.Fprintf(w, `
		<h1>🔑 SAVE YOUR RECOVERY KEYS</h1>
		<p><strong>⚠️ SAVE THESE NOW - THEY WILL NOT BE SHOWN AGAIN</strong></p>
		<p>These keys can be used to access your account if you lose access to your 2FA device. Each key can only be used once.</p>
		<div style="font-family: monospace; padding: 20px; background: #f5f5f5;">
			%s
		</div>
		<button onclick="downloadKeys()">Download as TXT</button>
		<button onclick="copyKeys()">Copy All</button>
		<form method="POST" action="/user/security/2fa/complete">
			<div>
				<input type="checkbox" name="confirmed" id="confirmed" required />
				<label for="confirmed">☑️ I have saved my recovery keys</label>
			</div>
			<button type="submit">Continue</button>
		</form>
		<script>
		function downloadKeys() {
			const keys = %q;
			const blob = new Blob([keys.join('\n')], {type: 'text/plain'});
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = 'caslink-recovery-keys.txt';
			a.click();
		}
		function copyKeys() {
			const keys = %q;
			navigator.clipboard.writeText(keys.join('\n'));
			alert('Recovery keys copied to clipboard');
		}
		</script>
	`, keysHTML, keys, keys)
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
	
	http.Redirect(w, r, "/user/security?msg=2fa_disabled", http.StatusSeeOther)
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

// renderPasskeysPage renders the passkey management page
func (h *UserSecurityHandler) renderPasskeysPage(w http.ResponseWriter, r *http.Request, user *service.User) {
	// Get user's registered passkeys from database
	passkeys, err := h.getPasskeysForUser(user.ID)
	if err != nil {
		http.Error(w, "Failed to load passkeys", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	
	passkeyList := ""
	if len(passkeys) == 0 {
		passkeyList = "<p>No passkeys registered yet.</p>"
	} else {
		passkeyList = "<ul>"
		for _, pk := range passkeys {
			passkeyList += fmt.Sprintf(`
				<li>
					<strong>%s</strong><br>
					Added: %s<br>
					Last used: %s<br>
					<form method="POST" action="/user/security/passkeys" style="display:inline;">
						<input type="hidden" name="action" value="delete" />
						<input type="hidden" name="passkey_id" value="%s" />
						<button type="submit" onclick="return confirm('Delete this passkey?')">Delete</button>
					</form>
				</li>
			`, pk.Name, pk.CreatedAt, pk.LastUsed, pk.ID)
		}
		passkeyList += "</ul>"
	}
	
	fmt.Fprintf(w, `
		<h1>Passkeys (WebAuthn)</h1>
		<p>User: %s</p>
		<p>Use fingerprint, Face ID, or hardware security keys to sign in.</p>
		<div>
			<h2>Registered Passkeys</h2>
			%s
			<p><strong>Note:</strong> Passkey registration requires JavaScript and browser support for WebAuthn.</p>
			<p>This feature will be fully implemented when the web frontend is complete.</p>
		</div>
		<p><a href="/user/security">Back to Security</a></p>
	`, user.Username, passkeyList)
}

// Passkey represents a WebAuthn credential
type Passkey struct {
	ID        string
	Name      string
	CreatedAt string
	LastUsed  string
}

// getPasskeysForUser retrieves passkeys from database
func (h *UserSecurityHandler) getPasskeysForUser(userID int64) ([]Passkey, error) {
	// Note: Passkeys table exists in schema per PART 23 line 7045-7058
	// For now, return empty list (actual WebAuthn implementation requires JavaScript)
	return []Passkey{}, nil
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

// renderRecoveryPage renders the recovery keys page
func (h *UserSecurityHandler) renderRecoveryPage(w http.ResponseWriter, r *http.Request, user *service.User) {
	// Check if user has MFA enabled (recovery keys only exist when MFA is enabled)
	hasMFA := h.totpService.HasTOTP(user.ID)
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	
	if hasMFA {
		remainingKeys, _ := h.totpService.GetRemainingRecoveryKeyCount(user.ID)
		fmt.Fprintf(w, `
			<h1>Recovery Keys</h1>
			<p>User: %s</p>
			<div>
				<h2>About Recovery Keys</h2>
				<p>Recovery keys allow you to access your account if you lose access to your 2FA device.</p>
				<ul>
					<li>10 keys were generated when you enabled 2FA</li>
					<li>Each key can only be used once</li>
					<li>Keys are shown only once during setup</li>
					<li>Format: a1b2c3d4-e5f6</li>
				</ul>
				<p><strong>Status:</strong> %d recovery keys remaining</p>
				<p>Recovery keys cannot be viewed again. If you lose all keys, you'll need to contact an administrator.</p>
			</div>
			<p><a href="/user/security">Back to Security</a></p>
		`, user.Username, remainingKeys)
	} else {
		fmt.Fprintf(w, `
			<h1>Recovery Keys</h1>
			<p>User: %s</p>
			<div>
				<h2>About Recovery Keys</h2>
				<p>Recovery keys allow you to access your account if you lose access to your 2FA device or passkey.</p>
				<ul>
					<li>10 keys are generated when you enable 2FA or passkeys</li>
					<li>Each key can only be used once</li>
					<li>Keys are shown only once - save them securely</li>
					<li>Format: a1b2c3d4-e5f6</li>
				</ul>
				<p><strong>Status:</strong> MFA not enabled (recovery keys not generated)</p>
				<p>Enable 2FA or passkeys to generate recovery keys.</p>
			</div>
			<p><a href="/user/security">Back to Security</a></p>
		`, user.Username)
	}
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
