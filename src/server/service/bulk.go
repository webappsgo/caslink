package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/model"
	"github.com/casjaysdevdocker/caslink/src/server/store"
)

const (
	bulkMaxBytes = 5 * 1024 * 1024 // 5 MB
	bulkMaxRows  = 10_000
)

// BulkRow is a single row for CSV/JSON import.
type BulkRow struct {
	URL        string `json:"url"`
	CustomCode string `json:"custom_code,omitempty"`
	Title      string `json:"title,omitempty"`
}

// BulkService handles bulk CSV/JSON import and export.
type BulkService struct {
	store      *store.Store
	urlService *URLService
}

// NewBulkService creates a new BulkService.
func NewBulkService(st *store.Store, urlService *URLService) *BulkService {
	return &BulkService{store: st, urlService: urlService}
}

// exportRow is the shape written to CSV and JSON exports.
type exportRow struct {
	ShortCode      string `json:"short_code"`
	DestinationURL string `json:"destination_url"`
	Title          string `json:"title"`
	Clicks         int64  `json:"clicks"`
	CreatedAt      string `json:"created_at"`
	ExpiresAt      string `json:"expires_at"`
}

// fetchExportRows queries all URLs owned by userID with their click counts.
func (s *BulkService) fetchExportRows(ctx context.Context, userID int64) ([]exportRow, error) {
	rows, err := s.store.ServerDB.QueryContext(ctx,
		`SELECT u.short_code, u.long_url, COALESCE(u.title,''), u.created_at, u.expires_at,
		        (SELECT COUNT(*) FROM clicks c WHERE c.url_id = u.id) AS clicks
		 FROM urls u
		 WHERE u.user_id = ?
		 ORDER BY u.created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query URLs: %w", err)
	}
	defer rows.Close()

	var result []exportRow
	for rows.Next() {
		var r exportRow
		var createdAt time.Time
		var expiresAt *time.Time

		if err := rows.Scan(&r.ShortCode, &r.DestinationURL, &r.Title, &createdAt, &expiresAt, &r.Clicks); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		r.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		if expiresAt != nil {
			r.ExpiresAt = expiresAt.UTC().Format(time.RFC3339)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ExportCSV returns a CSV of all URLs owned by userID.
// Columns: short_code, destination_url, title, clicks, created_at, expires_at
func (s *BulkService) ExportCSV(ctx context.Context, userID int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	exportRows, err := s.fetchExportRows(ctx, userID)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"short_code", "destination_url", "title", "clicks", "created_at", "expires_at"}); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}
	for _, r := range exportRows {
		if err := w.Write([]string{
			r.ShortCode,
			r.DestinationURL,
			r.Title,
			fmt.Sprintf("%d", r.Clicks),
			r.CreatedAt,
			r.ExpiresAt,
		}); err != nil {
			return nil, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("CSV flush error: %w", err)
	}
	return buf.Bytes(), nil
}

// ExportJSON returns a JSON array of URL objects for userID.
func (s *BulkService) ExportJSON(ctx context.Context, userID int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	exportRows, err := s.fetchExportRows(ctx, userID)
	if err != nil {
		return nil, err
	}

	out, err := json.Marshal(exportRows)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return out, nil
}

// ImportCSV parses a CSV and creates URLs for userID.
// Returns (successCount, errorRows, error).
// Validates each row; skips duplicates (returned in errorRows).
func (s *BulkService) ImportCSV(ctx context.Context, userID int64, csvData []byte) (int, []string, error) {
	if len(csvData) > bulkMaxBytes {
		return 0, nil, fmt.Errorf("import data exceeds 5 MB limit")
	}

	r := csv.NewReader(bytes.NewReader(csvData))
	r.TrimLeadingSpace = true

	records, err := r.ReadAll()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	// Strip optional header row
	if len(records) > 0 {
		first := strings.ToLower(records[0][0])
		if first == "url" || first == "destination_url" || first == "long_url" {
			records = records[1:]
		}
	}

	if len(records) > bulkMaxRows {
		return 0, nil, fmt.Errorf("import exceeds %d row limit", bulkMaxRows)
	}

	var rows []BulkRow
	for i, rec := range records {
		if len(rec) == 0 {
			continue
		}
		row := BulkRow{URL: strings.TrimSpace(rec[0])}
		if len(rec) > 1 {
			row.CustomCode = strings.TrimSpace(rec[1])
		}
		if len(rec) > 2 {
			row.Title = strings.TrimSpace(rec[2])
		}
		if row.URL == "" {
			continue
		}
		_ = i
		rows = append(rows, row)
	}

	return s.importRows(ctx, userID, rows)
}

// ImportJSON parses a JSON array and creates URLs for userID.
func (s *BulkService) ImportJSON(ctx context.Context, userID int64, jsonData []byte) (int, []string, error) {
	if len(jsonData) > bulkMaxBytes {
		return 0, nil, fmt.Errorf("import data exceeds 5 MB limit")
	}

	var rows []BulkRow
	if err := json.Unmarshal(jsonData, &rows); err != nil {
		return 0, nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if len(rows) > bulkMaxRows {
		return 0, nil, fmt.Errorf("import exceeds %d row limit", bulkMaxRows)
	}

	return s.importRows(ctx, userID, rows)
}

// importRows creates URLs for each BulkRow, returning success count and error rows.
func (s *BulkService) importRows(ctx context.Context, userID int64, rows []BulkRow) (int, []string, error) {
	var errorRows []string
	success := 0

	for _, row := range rows {
		if strings.TrimSpace(row.URL) == "" {
			continue
		}

		title := (*string)(nil)
		if row.Title != "" {
			t := row.Title
			title = &t
		}

		req := &model.CreateURLRequest{
			LongURL:    row.URL,
			CustomCode: row.CustomCode,
			Title:      title,
		}

		rowCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := s.urlService.CreateURLForUser(rowCtx, userID, req)
		cancel()

		if err != nil {
			errorRows = append(errorRows, fmt.Sprintf("%s: %v", row.URL, err))
			continue
		}
		success++
	}

	return success, errorRows, nil
}
