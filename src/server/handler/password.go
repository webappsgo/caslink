package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// PasswordHandler handles password reset operations
type PasswordHandler struct {
	authService  *service.AuthService
	emailService *service.EmailService
}

// NewPasswordHandler creates a new password handler
func NewPasswordHandler(authService *service.AuthService, emailService *service.EmailService) *PasswordHandler {
	return &PasswordHandler{
		authService:  authService,
		emailService: emailService,
	}
}

// ForgotPasswordPage renders the password reset request page
func (h *PasswordHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	// Check if SMTP is configured per PART 26 line 22666
	if !h.emailService.SMTPConfigured() {
		// Show contact administrator message per PART 26
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<h1>Password Reset Unavailable</h1>
			<p>Email features require SMTP configuration.</p>
			<p>Please contact your administrator for assistance.</p>`)
		return
	}

	// TODO: Render forgot password HTML page per PART 17
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "<h1>Forgot Password</h1><p>Password reset form will be implemented per PART 17</p>")
}

// ForgotPassword handles password reset request
func (h *PasswordHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if SMTP is configured per PART 26
	if !h.emailService.SMTPConfigured() {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Email features not available",
		})
		return
	}

	// Parse request
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	// Create reset token
	// Note: Returns success even if email doesn't exist per PART 23 line 19998
	token, err := h.authService.CreatePasswordResetToken(ctx, req.Email, "user")
	if err != nil {
		// Log error but return generic success message
	}

	// Send email if token was created
	if token != "" {
		resetLink := fmt.Sprintf("https://%s/auth/password/reset/%s", r.Host, token)
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		err = h.emailService.SendPasswordReset(req.Email, resetLink, clientIP)
		if err != nil {
			// Log error but don't fail request
		}
	}

	// Always return generic success per PART 23 line 19998
	respondJSON(w, http.StatusOK, map[string]string{
		"message": "If an account exists, instructions have been sent.",
	})
}

// ResetPasswordPage renders the password reset form (with token)
func (h *PasswordHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	// TODO: Render reset password HTML page per PART 17
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "<h1>Reset Password</h1><p>Token: %s</p><p>Reset password form will be implemented per PART 17</p>", token)
}

// ResetPassword handles password reset with token
func (h *PasswordHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := chi.URLParam(r, "token")

	// Parse request
	var req struct {
		Password        string `json:"password"`
		PasswordConfirm string `json:"password_confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	// Validate passwords match
	if req.Password != req.PasswordConfirm {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Passwords do not match",
		})
		return
	}

	// Validate password strength (minimum 8 characters per PART 23)
	if len(req.Password) < 8 {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Password must be at least 8 characters",
		})
		return
	}

	// Reset password (invalidates all sessions per PART 23 line 20534)
	err := h.authService.ResetPassword(ctx, token, req.Password)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid or expired reset token",
		})
		return
	}

	// Return success
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Password has been reset. Please log in with your new password.",
	})
}
