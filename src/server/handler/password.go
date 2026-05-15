package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/tmpl"
)

// PasswordHandler handles password reset operations
type PasswordHandler struct {
	authService  *service.AuthService
	emailService *service.EmailService
	renderer     *tmpl.Renderer
	cfg          *config.Config
}

// NewPasswordHandler creates a new password handler
func NewPasswordHandler(
	authService *service.AuthService,
	emailService *service.EmailService,
	renderer *tmpl.Renderer,
	cfg *config.Config,
) *PasswordHandler {
	return &PasswordHandler{
		authService:  authService,
		emailService: emailService,
		renderer:     renderer,
		cfg:          cfg,
	}
}

// ForgotPasswordPage renders the password reset request page
func (h *PasswordHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	if !h.emailService.SMTPConfigured() {
		data := struct {
			tmpl.Data
			Error string
		}{
			Data:  newPageData(h.cfg, r, "Reset Password", nil),
			Error: "Password reset requires email configuration. Please contact your administrator.",
		}
		h.renderer.Render(w, "template/page/auth/forgot.html", data)
		return
	}

	data := struct {
		tmpl.Data
		Error   string
		Email   string
		Success bool
	}{
		Data: newPageData(h.cfg, r, "Reset Password", nil),
	}
	h.renderer.Render(w, "template/page/auth/forgot.html", data)
}

// ForgotPassword handles password reset request (JSON and form)
func (h *PasswordHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	isForm := strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")

	if !h.emailService.SMTPConfigured() {
		if isForm {
			http.Redirect(w, r, "/server/auth/password/forgot", http.StatusSeeOther)
			return
		}
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Email features not available",
		})
		return
	}

	var email string
	if isForm {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		email = r.PostFormValue("email")
	} else {
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Invalid request body",
			})
			return
		}
		email = req.Email
	}

	token, _ := h.authService.CreatePasswordResetToken(ctx, email, "user")
	if token != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		resetLink := fmt.Sprintf("%s://%s/server/auth/password/reset/%s", scheme, r.Host, token)
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}
		_ = h.emailService.SendPasswordReset(email, resetLink, clientIP)
	}

	if isForm {
		data := struct {
			tmpl.Data
			Error   string
			Email   string
			Success bool
		}{
			Data:    newPageData(h.cfg, r, "Reset Password", nil),
			Email:   email,
			Success: true,
		}
		h.renderer.Render(w, "template/page/auth/forgot.html", data)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{
		"message": "If an account exists, instructions have been sent.",
	})
}

// ResetPasswordPage renders the password reset form
func (h *PasswordHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	_, _, tokenErr := h.authService.ValidatePasswordResetToken(r.Context(), token)

	data := struct {
		tmpl.Data
		Error        string
		Token        string
		InvalidToken bool
	}{
		Data:         newPageData(h.cfg, r, "Set New Password", nil),
		Token:        token,
		InvalidToken: tokenErr != nil,
	}
	h.renderer.Render(w, "template/page/auth/reset.html", data)
}

// ResetPassword handles password reset with token (JSON and form)
func (h *PasswordHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := chi.URLParam(r, "token")
	isForm := strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")

	var password, passwordConfirm string
	if isForm {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		password = r.PostFormValue("password")
		passwordConfirm = r.PostFormValue("confirm_password")
	} else {
		var req struct {
			Password        string `json:"password"`
			PasswordConfirm string `json:"password_confirm"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		password = req.Password
		passwordConfirm = req.PasswordConfirm
	}

	renderErr := func(msg string, invalidToken bool) {
		data := struct {
			tmpl.Data
			Error        string
			Token        string
			InvalidToken bool
		}{
			Data:         newPageData(h.cfg, r, "Set New Password", nil),
			Error:        msg,
			Token:        token,
			InvalidToken: invalidToken,
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderer.Render(w, "template/page/auth/reset.html", data)
	}

	if password != passwordConfirm {
		if isForm {
			renderErr("Passwords do not match", false)
			return
		}
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Passwords do not match"})
		return
	}
	if len(password) < 8 {
		if isForm {
			renderErr("Password must be at least 8 characters", false)
			return
		}
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Password must be at least 8 characters"})
		return
	}

	if err := h.authService.ResetPassword(ctx, token, password); err != nil {
		if isForm {
			renderErr("Invalid or expired reset link. Please request a new one.", true)
			return
		}
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid or expired reset token"})
		return
	}

	if isForm {
		http.Redirect(w, r, "/server/auth/login?reset=1", http.StatusSeeOther)
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Password has been reset. Please log in with your new password.",
	})
}
