package bulk

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Exporter handles data export in multiple formats
type Exporter struct {
	db     *db.DB
	config *config.BulkConfig
	logger *logrus.Logger
}

// NewExporter creates a new exporter instance
func NewExporter(database *db.DB, cfg *config.BulkConfig, logger *logrus.Logger) (*Exporter, error) {
	return &Exporter{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// ProcessExport processes an export request
func (e *Exporter) ProcessExport(ctx context.Context, req *ExportRequest) (*ExportResult, error) {
	startTime := time.Now()

	e.logger.WithFields(logrus.Fields{
		"user_id":    req.UserID,
		"format":     req.Format,
		"start_date": req.StartDate,
		"end_date":   req.EndDate,
		"url_count":  len(req.URLIDs),
	}).Info("Starting export processing")

	// Query URLs based on request filters
	records, err := e.queryURLs(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to query URLs: %w", err)
	}

	if len(records) == 0 {
		return &ExportResult{
			TotalItems: 0,
			Duration:   time.Since(startTime),
		}, nil
	}

	// Generate output file path
	outputPath := e.generateOutputPath(req.Format, req.UserID)

	// Export data in the requested format
	switch strings.ToLower(req.Format) {
	case FormatCSV:
		err = e.exportCSV(records, outputPath, req.Fields)
	case FormatJSON:
		err = e.exportJSON(records, outputPath, req.Fields)
	case FormatTXT:
		err = e.exportTXT(records, outputPath)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", req.Format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to export data: %w", err)
	}

	result := &ExportResult{
		TotalItems: int64(len(records)),
		OutputPath: outputPath,
		Duration:   time.Since(startTime),
	}

	e.logger.WithFields(logrus.Fields{
		"total_items": result.TotalItems,
		"output_path": result.OutputPath,
		"duration":    result.Duration,
	}).Info("Export processing completed")

	return result, nil
}

// queryURLs queries URLs based on export request filters
func (e *Exporter) queryURLs(ctx context.Context, req *ExportRequest) ([]URLRecord, error) {
	var whereConditions []string
	var args []interface{}
	argIndex := 1

	// Base query
	query := `
		SELECT id, original_url, title, description, expires_at, password,
		       created_at, updated_at, clicks, unique_clicks, active, user_id
		FROM urls
		WHERE user_id = ?`

	whereConditions = append(whereConditions, "user_id = ?")
	args = append(args, req.UserID)
	argIndex++

	// Apply filters
	if !req.StartDate.IsZero() {
		placeholder := "?"
		if e.db.Type() == "postgres" {
			placeholder = fmt.Sprintf("$%d", argIndex)
		}
		whereConditions = append(whereConditions, fmt.Sprintf("created_at >= %s", placeholder))
		args = append(args, req.StartDate)
		argIndex++
	}

	if !req.EndDate.IsZero() {
		placeholder := "?"
		if e.db.Type() == "postgres" {
			placeholder = fmt.Sprintf("$%d", argIndex)
		}
		whereConditions = append(whereConditions, fmt.Sprintf("created_at <= %s", placeholder))
		args = append(args, req.EndDate)
		argIndex++
	}

	if len(req.URLIDs) > 0 {
		placeholders := make([]string, len(req.URLIDs))
		for i, urlID := range req.URLIDs {
			if e.db.Type() == "postgres" {
				placeholders[i] = fmt.Sprintf("$%d", argIndex)
			} else {
				placeholders[i] = "?"
			}
			args = append(args, urlID)
			argIndex++
		}
		whereConditions = append(whereConditions, fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")))
	}

	// Apply custom filters
	for field, value := range req.Filters {
		switch field {
		case "active":
			placeholder := "?"
			if e.db.Type() == "postgres" {
				placeholder = fmt.Sprintf("$%d", argIndex)
			}
			whereConditions = append(whereConditions, fmt.Sprintf("active = %s", placeholder))
			args = append(args, value == "true")
			argIndex++
		case "has_password":
			if value == "true" {
				whereConditions = append(whereConditions, "password IS NOT NULL AND password != ''")
			} else {
				whereConditions = append(whereConditions, "(password IS NULL OR password = '')")
			}
		case "expired":
			if value == "true" {
				whereConditions = append(whereConditions, "expires_at IS NOT NULL AND expires_at < NOW()")
			} else {
				whereConditions = append(whereConditions, "(expires_at IS NULL OR expires_at >= NOW())")
			}
		}
	}

	// Build final query
	if len(whereConditions) > 1 {
		query += " AND " + strings.Join(whereConditions[1:], " AND ")
	}
	query += " ORDER BY created_at DESC"

	// Convert placeholders for PostgreSQL
	if e.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "user_id = ?", "user_id = $1")
	}

	e.logger.WithFields(logrus.Fields{
		"query":     query,
		"arg_count": len(args),
	}).Debug("Executing export query")

	rows, err := e.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var records []URLRecord
	for rows.Next() {
		var record URLRecord
		var expiresAt *time.Time
		var password *string

		err := rows.Scan(
			&record.ID, &record.OriginalURL, &record.Title, &record.Description,
			&expiresAt, &password, &record.CreatedAt, &record.UpdatedAt,
			&record.Clicks, &record.UniqueClicks, &record.IsActive, &record.UserID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		record.ShortCode = record.ID
		record.ExpiresAt = expiresAt
		if password != nil {
			record.Password = *password
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return records, nil
}

// exportCSV exports data to CSV format
func (e *Exporter) exportCSV(records []URLRecord, outputPath string, fields []string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Determine fields to export
	if len(fields) == 0 {
		fields = []string{
			"short_code", "original_url", "title", "description", "tags",
			"expires_at", "password", "is_active", "clicks", "unique_clicks",
			"created_at", "updated_at",
		}
	}

	// Write header
	if err := writer.Write(fields); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write records
	for _, record := range records {
		row := make([]string, len(fields))
		for i, field := range fields {
			row[i] = e.getFieldValue(record, field)
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

// exportJSON exports data to JSON format
func (e *Exporter) exportJSON(records []URLRecord, outputPath string, fields []string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Filter records by fields if specified
	var exportData interface{}
	if len(fields) == 0 {
		exportData = records
	} else {
		// Create filtered data
		filteredRecords := make([]map[string]interface{}, len(records))
		for i, record := range records {
			filteredRecord := make(map[string]interface{})
			for _, field := range fields {
				filteredRecord[field] = e.getFieldValue(record, field)
			}
			filteredRecords[i] = filteredRecord
		}
		exportData = filteredRecords
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(exportData); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// exportTXT exports data to plain text format (one URL per line)
func (e *Exporter) exportTXT(records []URLRecord, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Write header comment
	if _, err := file.WriteString(fmt.Sprintf("# URL Export - %s\n", time.Now().Format(time.RFC3339))); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	if _, err := file.WriteString(fmt.Sprintf("# Total URLs: %d\n\n", len(records))); err != nil {
		return fmt.Errorf("failed to write count: %w", err)
	}

	// Write URLs
	for _, record := range records {
		line := record.OriginalURL
		if record.Title != "" {
			line += fmt.Sprintf(" # %s", record.Title)
		}
		line += "\n"

		if _, err := file.WriteString(line); err != nil {
			return fmt.Errorf("failed to write URL: %w", err)
		}
	}

	return nil
}

// getFieldValue extracts a field value from a URLRecord
func (e *Exporter) getFieldValue(record URLRecord, field string) string {
	switch field {
	case "id", "short_code":
		return record.ShortCode
	case "original_url":
		return record.OriginalURL
	case "title":
		return record.Title
	case "description":
		return record.Description
	case "tags":
		return strings.Join(record.Tags, ",")
	case "expires_at":
		if record.ExpiresAt != nil {
			return record.ExpiresAt.Format(time.RFC3339)
		}
		return ""
	case "password":
		return record.Password
	case "is_active":
		if record.IsActive {
			return "true"
		}
		return "false"
	case "clicks":
		return fmt.Sprintf("%d", record.Clicks)
	case "unique_clicks":
		return fmt.Sprintf("%d", record.UniqueClicks)
	case "created_at":
		return record.CreatedAt.Format(time.RFC3339)
	case "updated_at":
		return record.UpdatedAt.Format(time.RFC3339)
	case "user_id":
		return record.UserID
	default:
		return ""
	}
}

// generateOutputPath generates a unique output file path
func (e *Exporter) generateOutputPath(format, userID string) string {
	// Create exports directory if it doesn't exist
	exportDir := filepath.Join(e.config.ExportDirectory, userID)
	os.MkdirAll(exportDir, 0755)

	// Generate unique filename
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("export_%s_%s.%s", timestamp, uuid.New().String()[:8], format)

	return filepath.Join(exportDir, filename)
}

// ReadExportFile reads an export file and returns its content
func (e *Exporter) ReadExportFile(filePath string) ([]byte, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read export file: %w", err)
	}
	return content, nil
}

// CleanupExportFile removes an export file
func (e *Exporter) CleanupExportFile(filePath string) error {
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to cleanup export file: %w", err)
	}
	return nil
}

// GetExportSize returns the size of an export file
func (e *Exporter) GetExportSize(filePath string) (int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to get file info: %w", err)
	}
	return info.Size(), nil
}

// ValidateExportRequest validates an export request
func (e *Exporter) ValidateExportRequest(req *ExportRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	validFormats := map[string]bool{
		FormatCSV:  true,
		FormatJSON: true,
		FormatTXT:  true,
	}

	if !validFormats[req.Format] {
		return fmt.Errorf("unsupported format: %s", req.Format)
	}

	if !req.StartDate.IsZero() && !req.EndDate.IsZero() {
		if req.StartDate.After(req.EndDate) {
			return fmt.Errorf("start date cannot be after end date")
		}

		// Limit date range to prevent excessive data export
		maxRange := 365 * 24 * time.Hour // 1 year
		if req.EndDate.Sub(req.StartDate) > maxRange {
			return fmt.Errorf("date range cannot exceed 1 year")
		}
	}

	// Validate field names if specified
	if len(req.Fields) > 0 {
		validFields := map[string]bool{
			"id": true, "short_code": true, "original_url": true, "title": true,
			"description": true, "tags": true, "expires_at": true, "password": true,
			"is_active": true, "clicks": true, "unique_clicks": true,
			"created_at": true, "updated_at": true, "user_id": true,
		}

		for _, field := range req.Fields {
			if !validFields[field] {
				return fmt.Errorf("invalid field: %s", field)
			}
		}
	}

	return nil
}

// EstimateExportSize estimates the size of an export based on record count
func (e *Exporter) EstimateExportSize(ctx context.Context, req *ExportRequest) (int64, error) {
	// Get approximate record count
	count, err := e.getRecordCount(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("failed to get record count: %w", err)
	}

	// Estimate size based on format and record count
	var estimatedSizePerRecord int64
	switch strings.ToLower(req.Format) {
	case FormatCSV:
		estimatedSizePerRecord = 200 // Average CSV row size
	case FormatJSON:
		estimatedSizePerRecord = 400 // Average JSON record size
	case FormatTXT:
		estimatedSizePerRecord = 100 // Average text line size
	default:
		estimatedSizePerRecord = 300 // Default estimate
	}

	return count * estimatedSizePerRecord, nil
}

// getRecordCount gets the count of records that would be exported
func (e *Exporter) getRecordCount(ctx context.Context, req *ExportRequest) (int64, error) {
	var whereConditions []string
	var args []interface{}
	argIndex := 1

	query := "SELECT COUNT(*) FROM urls WHERE user_id = ?"
	whereConditions = append(whereConditions, "user_id = ?")
	args = append(args, req.UserID)
	argIndex++

	// Apply the same filters as in queryURLs
	if !req.StartDate.IsZero() {
		placeholder := "?"
		if e.db.Type() == "postgres" {
			placeholder = fmt.Sprintf("$%d", argIndex)
		}
		whereConditions = append(whereConditions, fmt.Sprintf("created_at >= %s", placeholder))
		args = append(args, req.StartDate)
		argIndex++
	}

	if !req.EndDate.IsZero() {
		placeholder := "?"
		if e.db.Type() == "postgres" {
			placeholder = fmt.Sprintf("$%d", argIndex)
		}
		whereConditions = append(whereConditions, fmt.Sprintf("created_at <= %s", placeholder))
		args = append(args, req.EndDate)
		argIndex++
	}

	if len(req.URLIDs) > 0 {
		placeholders := make([]string, len(req.URLIDs))
		for i, urlID := range req.URLIDs {
			if e.db.Type() == "postgres" {
				placeholders[i] = fmt.Sprintf("$%d", argIndex)
			} else {
				placeholders[i] = "?"
			}
			args = append(args, urlID)
			argIndex++
		}
		whereConditions = append(whereConditions, fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")))
	}

	// Build final query
	if len(whereConditions) > 1 {
		query += " AND " + strings.Join(whereConditions[1:], " AND ")
	}

	// Convert placeholders for PostgreSQL
	if e.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "user_id = ?", "user_id = $1")
	}

	var count int64
	err := e.db.QueryRow(ctx, query, args...).Scan(&count)
	return count, err
}