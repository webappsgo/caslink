package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/server/model"
	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// URLHandler handles URL shortening endpoints
type URLHandler struct {
	urlService *service.URLService
}

// NewURLHandler creates a new URL handler
func NewURLHandler(urlService *service.URLService) *URLHandler {
	return &URLHandler{
		urlService: urlService,
	}
}

// CreateURL handles POST /api/v1/urls
func (h *URLHandler) CreateURL(w http.ResponseWriter, r *http.Request) {
	var req model.CreateURLRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	url, err := h.urlService.CreateURL(r.Context(), &req)
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
		respondError(w, http.StatusInternalServerError, "Failed to create URL")
		return
	}

	respondJSON(w, http.StatusCreated, url)
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

	respondJSON(w, http.StatusOK, url)
}

// RedirectURL handles GET /{code} - redirects to the long URL
func (h *URLHandler) RedirectURL(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		http.NotFound(w, r)
		return
	}

	url, err := h.urlService.GetURLByCode(r.Context(), code)
	if err != nil {
		if err == model.ErrURLNotFound || err == model.ErrURLExpired {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Record click (async - don't block redirect)
	go func() {
		ipAddress := r.RemoteAddr
		userAgent := r.UserAgent()
		referrer := r.Referer()
		h.urlService.RecordClick(r.Context(), url.ID, ipAddress, userAgent, referrer)
	}()

	// Redirect to long URL
	http.Redirect(w, r, url.LongURL, http.StatusFound)
}

// respondJSON sends a JSON response
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError sends an error response
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]interface{}{
		"error":  message,
		"status": status,
	})
}
