package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/server/model"
	"github.com/casjaysdevdocker/caslink/src/server/service"
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
}

// NewURLHandler creates a new URL handler
func NewURLHandler(urlService *service.URLService, analyticsService *service.AnalyticsService) *URLHandler {
	return &URLHandler{
		urlService:       urlService,
		analyticsService: analyticsService,
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
		req.LongURL = r.FormValue("url")
		req.CustomCode = r.FormValue("custom_code")
		req.Password = r.FormValue("password")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
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

	// Record click (async - don't block redirect). Cap concurrency with a
	// semaphore so a flood of redirects cannot spawn unbounded goroutines,
	// and use a detached context with a deadline so the database write is
	// not cancelled the moment the response completes.
	ipAddress := r.RemoteAddr
	userAgent := r.UserAgent()
	referrer := r.Referer()
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

	// Redirect to long URL
	http.Redirect(w, r, url.LongURL, http.StatusFound)
}

// Stats handles GET /api/v1/urls/{code}/stats
func (h *URLHandler) Stats(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "Short code required")
		return
	}

	stats, err := h.analyticsService.GetURLStats(r.Context(), code)
	if err != nil {
		respondError(w, http.StatusNotFound, "URL not found or no stats available")
		return
	}

	respondJSON(w, http.StatusOK, stats)
}

// respondJSON and respondError are defined in helpers.go and shared across
// every handler. They emit the canonical APIResponse envelope per
// AI.md PART 9 ("Response Format").
