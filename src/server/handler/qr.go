package handler

import (
	"net/http"
	"strconv"

	"fmt"

	"github.com/go-chi/chi/v5"

	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// QRHandler handles QR code endpoints
type QRHandler struct {
	qrService  *service.QRService
	urlService *service.URLService
}

// NewQRHandler creates a new QR handler
func NewQRHandler(qrService *service.QRService, urlService *service.URLService) *QRHandler {
	return &QRHandler{
		qrService:  qrService,
		urlService: urlService,
	}
}

// GenerateQR handles GET /api/v1/qr/{code}
func (h *QRHandler) GenerateQR(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "Short code required")
		return
	}

	// Get URL by code
	url, err := h.urlService.GetURLByCode(r.Context(), code)
	if err != nil {
		respondError(w, http.StatusNotFound, "URL not found")
		return
	}

	// Parse query parameters for QR options
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "png"
	}

	sizeStr := r.URL.Query().Get("size")
	size := 200
	if sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil {
			size = s
		}
	}

	style := r.URL.Query().Get("style")
	if style == "" {
		style = "square"
	}

	// Normalise format: svg is served as-is; pdf and other formats fall back to png.
	if format != "svg" && format != "png" {
		format = "png"
	}

	opts := &service.QRCodeOptions{
		Format: format,
		Size:   size,
		Style:  style,
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	fullURL := fmt.Sprintf("%s://%s/%s", scheme, host, code)

	data, contentType, err := h.qrService.GenerateQRCode(r.Context(), url.ID, fullURL, opts)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to generate QR code")
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
