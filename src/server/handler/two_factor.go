package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// TwoFactorHandler handles 2FA verification during login per PART 23
type TwoFactorHandler struct {
	authService *service.AuthService
	totpService *service.TOTPService
}

// NewTwoFactorHandler creates a new 2FA handler
func NewTwoFactorHandler(authService *service.AuthService, totpService *service.TOTPService) *TwoFactorHandler {
	return &TwoFactorHandler{
		authService: authService,
		totpService: totpService,
	}
}

// VerifyPage renders the 2FA verification page per PART 23 line 20217-20229
func (h *TwoFactorHandler) VerifyPage(w http.ResponseWriter, r *http.Request) {
	// Check if user has pending 2FA session
	cookie, err := r.Cookie("2fa_pending")
	if err != nil || cookie.Value == "" {
		http.Error(w, "No pending 2FA session. Please log in first.", http.StatusUnauthorized)
		return
	}
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `
		<h1>Two-Factor Authentication</h1>
		<p>Enter the 6-digit code from your authenticator app:</p>
		<form method="POST" action="/server/auth/2fa">
			<div>
				<input type="text" name="code" maxlength="6" pattern="[0-9]{6}" required autofocus />
			</div>
			<button type="submit">Verify</button>
		</form>
		<hr>
		<p>Lost access to your authenticator?</p>
		<a href="/server/server/auth/2fa/recovery">Use Recovery Key</a>
	`)
}

// Verify handles 2FA code verification per PART 23 line 20217-20229
func (h *TwoFactorHandler) Verify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	ctx := r.Context()
	
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	
	code := strings.TrimSpace(r.FormValue("code"))
	if code == "" || len(code) != 6 {
		http.Error(w, "Invalid code format. Must be 6 digits.", http.StatusBadRequest)
		return
	}
	
	// Get pending 2FA session from cookie
	cookie, err := r.Cookie("2fa_pending")
	if err != nil || cookie.Value == "" {
		http.Error(w, "No pending 2FA session. Please log in again.", http.StatusUnauthorized)
		return
	}
	
	// Validate temporary session and get user
	user, err := h.authService.ValidateUserSession(ctx, cookie.Value)
	if err != nil {
		http.Error(w, "Invalid or expired 2FA session. Please log in again.", http.StatusUnauthorized)
		return
	}
	
	// Get TOTP secret
	secret, err := h.totpService.GetTOTPSecret(user.ID)
	if err != nil {
		http.Error(w, "Failed to verify 2FA code", http.StatusInternalServerError)
		return
	}
	
	// Verify TOTP code
	if !h.totpService.VerifyTOTPCode(secret, code) {
		http.Error(w, "Invalid verification code. Please try again.", http.StatusUnauthorized)
		return
	}
	
	// Code is valid! Delete temp session and create full session
	_ = h.authService.DeleteSession(ctx, cookie.Value)
	
	// Create full session (7 days default)
	sessionID, err := h.authService.CreateUserSession(ctx, user.ID, false)
	if err != nil {
		http.Error(w, "2FA verification succeeded but session creation failed", http.StatusInternalServerError)
		return
	}
	
	// Clear 2FA pending cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "2fa_pending",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	
	// Set full session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "user_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60, // 7 days
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	
	// Redirect to user dashboard
	http.Redirect(w, r, "/users/profile", http.StatusSeeOther)
}

// RecoveryPage renders the recovery key entry page per PART 23 line 20233-20243
func (h *TwoFactorHandler) RecoveryPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `
		<h1>Use Recovery Key</h1>
		<p>Enter one of your recovery keys:</p>
		<form method="POST" action="/server/server/auth/2fa/recovery">
			<div>
				<input type="text" name="recovery_key" placeholder="a1b2c3d4-e5f6" required autofocus />
			</div>
			<p><strong>⚠️ This key will be invalidated after use.</strong></p>
			<button type="submit">Submit</button>
		</form>
		<p><a href="/server/auth/2fa">Back to 2FA</a></p>
	`)
}

// Recovery handles recovery key verification per PART 23 line 20244-20259
func (h *TwoFactorHandler) Recovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	ctx := r.Context()
	
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	
	recoveryKey := strings.TrimSpace(r.FormValue("recovery_key"))
	if recoveryKey == "" {
		http.Error(w, "Recovery key is required", http.StatusBadRequest)
		return
	}
	
	// Get pending 2FA session from cookie
	cookie, err := r.Cookie("2fa_pending")
	if err != nil || cookie.Value == "" {
		http.Error(w, "No pending 2FA session. Please log in again.", http.StatusUnauthorized)
		return
	}
	
	// Validate temporary session and get user
	user, err := h.authService.ValidateUserSession(ctx, cookie.Value)
	if err != nil {
		http.Error(w, "Invalid or expired 2FA session. Please log in again.", http.StatusUnauthorized)
		return
	}
	
	// Verify and use recovery key (single-use per PART 23 line 20029)
	err = h.totpService.UseRecoveryKey(user.ID, recoveryKey)
	if err != nil {
		http.Error(w, "Invalid or already used recovery key.", http.StatusUnauthorized)
		return
	}
	
	// Get remaining key count
	remainingKeys, _ := h.totpService.GetRemainingRecoveryKeyCount(user.ID)
	
	// Recovery key accepted! Show options per PART 23 line 20247-20259
	h.renderRecoveryOptionsPageWithKeys(w, r, user, remainingKeys)
}

// renderRecoveryOptionsPageWithKeys shows options with actual key count
func (h *TwoFactorHandler) renderRecoveryOptionsPageWithKeys(w http.ResponseWriter, r *http.Request, user *service.User, remainingKeys int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `
		<h1>✅ Recovery Key Accepted</h1>
		<p>Recovery key accepted and invalidated.</p>
		<p>You have %d recovery keys remaining.</p>
		<h2>What would you like to do?</h2>
		<form method="POST" action="/server/server/server/auth/2fa/recovery/action">
			<div>
				<input type="radio" name="action" value="disable" id="disable" />
				<label for="disable">Disable 2FA temporarily (login with password only)</label>
			</div>
			<div>
				<input type="radio" name="action" value="setup" id="setup" checked />
				<label for="setup">Set up new 2FA device now</label>
			</div>
			<div>
				<input type="radio" name="action" value="continue" id="continue" />
				<label for="continue">Continue to dashboard (keep 2FA enabled)</label>
			</div>
			<button type="submit">Continue</button>
		</form>
	`, remainingKeys)
}

// RecoveryOptionsPage shows options after recovery key is validated per PART 23 line 20247-20259
func (h *TwoFactorHandler) RecoveryOptionsPage(w http.ResponseWriter, r *http.Request) {
	// Get user from pending 2FA session
	cookie, err := r.Cookie("2fa_pending")
	if err != nil || cookie.Value == "" {
		http.Error(w, "No pending session", http.StatusUnauthorized)
		return
	}
	
	user, err := h.authService.ValidateUserSession(r.Context(), cookie.Value)
	if err != nil {
		http.Error(w, "Invalid session", http.StatusUnauthorized)
		return
	}
	
	remainingKeys, _ := h.totpService.GetRemainingRecoveryKeyCount(user.ID)
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `
		<h1>✅ Recovery Key Accepted</h1>
		<p>Recovery key accepted and invalidated.</p>
		<p>You have %d recovery keys remaining.</p>
		<h2>What would you like to do?</h2>
		<form method="POST" action="/server/server/server/auth/2fa/recovery/action">
			<div>
				<input type="radio" name="action" value="disable" id="disable" />
				<label for="disable">Disable 2FA temporarily (login with password only)</label>
			</div>
			<div>
				<input type="radio" name="action" value="setup" id="setup" checked />
				<label for="setup">Set up new 2FA device now</label>
			</div>
			<button type="submit">Continue</button>
		</form>
	`, remainingKeys)
}
