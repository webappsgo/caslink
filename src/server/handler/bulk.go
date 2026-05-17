package handler

import (
	"io"
	"net/http"

	"github.com/casjaysdevdocker/caslink/src/server/service"
)

// BulkHandler handles bulk import/export endpoints.
type BulkHandler struct {
	bulkService *service.BulkService
}

// NewBulkHandler creates a new BulkHandler.
func NewBulkHandler(bulkService *service.BulkService) *BulkHandler {
	return &BulkHandler{bulkService: bulkService}
}

// Export handles GET /api/v1/users/urls/export?format=csv|json
func (h *BulkHandler) Export(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok || user == nil {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID := user.ID

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	switch format {
	case "csv":
		data, err := h.bulkService.ExportCSV(r.Context(), userID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Export failed")
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="urls.csv"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)

	case "json":
		data, err := h.bulkService.ExportJSON(r.Context(), userID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Export failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)

	default:
		respondError(w, http.StatusBadRequest, "Invalid format: use csv or json")
	}
}

// Import handles POST /api/v1/users/urls/import
// Accepts multipart/form-data (field "file") or raw CSV/JSON body.
func (h *BulkHandler) Import(w http.ResponseWriter, r *http.Request) {
	user, ok := getUserFromRequest(r)
	if !ok || user == nil {
		respondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID := user.ID

	const maxBodyBytes = 5 * 1024 * 1024

	contentType := r.Header.Get("Content-Type")
	var rawData []byte
	var format string

	if len(contentType) >= 19 && contentType[:19] == "multipart/form-data" {
		if err := r.ParseMultipartForm(maxBodyBytes); err != nil {
			respondError(w, http.StatusBadRequest, "Failed to parse multipart form")
			return
		}
		file, fh, err := r.FormFile("file")
		if err != nil {
			respondError(w, http.StatusBadRequest, "Missing file field")
			return
		}
		defer file.Close()

		limited := io.LimitReader(file, maxBodyBytes+1)
		rawData, err = io.ReadAll(limited)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Failed to read file")
			return
		}
		if len(rawData) > maxBodyBytes {
			respondError(w, http.StatusRequestEntityTooLarge, "File exceeds 5 MB limit")
			return
		}

		// Determine format from filename
		if len(fh.Filename) >= 5 && fh.Filename[len(fh.Filename)-4:] == ".csv" {
			format = "csv"
		} else {
			format = "json"
		}
	} else {
		limited := io.LimitReader(r.Body, maxBodyBytes+1)
		var err error
		rawData, err = io.ReadAll(limited)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Failed to read body")
			return
		}
		if len(rawData) > maxBodyBytes {
			respondError(w, http.StatusRequestEntityTooLarge, "Body exceeds 5 MB limit")
			return
		}
		if contentType == "text/csv" || contentType == "application/csv" {
			format = "csv"
		} else {
			format = "json"
		}
	}

	// Allow explicit format override via query param
	if qf := r.URL.Query().Get("format"); qf == "csv" || qf == "json" {
		format = qf
	}

	var successCount int
	var errorRows []string
	var err error

	switch format {
	case "csv":
		successCount, errorRows, err = h.bulkService.ImportCSV(r.Context(), userID, rawData)
	default:
		successCount, errorRows, err = h.bulkService.ImportJSON(r.Context(), userID, rawData)
	}

	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": successCount,
		"errors":  errorRows,
	})
}

