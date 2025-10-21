package bulk

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// Importer handles CSV/JSON import functionality
type Importer struct {
	db     *db.DB
	config *config.BulkConfig
	logger *logrus.Logger
}

// NewImporter creates a new importer instance
func NewImporter(database *db.DB, cfg *config.BulkConfig, logger *logrus.Logger) (*Importer, error) {
	return &Importer{
		db:     database,
		config: cfg,
		logger: logger,
	}, nil
}

// ProcessImport processes an import request
func (i *Importer) ProcessImport(ctx context.Context, req *ImportRequest) (*ImportResult, error) {
	startTime := time.Now()

	i.logger.WithFields(logrus.Fields{
		"user_id":  req.UserID,
		"format":   req.Format,
		"filename": req.Filename,
		"dry_run":  req.DryRun,
	}).Info("Starting import processing")

	var records []URLRecord
	var err error

	// Parse the data based on format
	switch strings.ToLower(req.Format) {
	case FormatCSV:
		records, err = i.parseCSV(req.Data)
	case FormatJSON:
		records, err = i.parseJSON(req.Data)
	case FormatTXT:
		records, err = i.parseTXT(req.Data)
	default:
		return nil, fmt.Errorf("unsupported format: %s", req.Format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse import data: %w", err)
	}

	result := &ImportResult{
		TotalItems: int64(len(records)),
		Errors:     []ImportError{},
	}

	// Validate records if requested
	if req.ValidateURLs {
		records, result.Errors = i.validateRecords(records)
	}

	// Set user ID for all records
	for i := range records {
		records[i].UserID = req.UserID
	}

	// Process records
	if req.DryRun {
		// For dry run, just return validation results
		result.SuccessItems = int64(len(records))
		result.FailedItems = int64(len(result.Errors))
	} else {
		// Actually import the records
		successCount, importErrors := i.importRecords(ctx, records, req.Overwrite)
		result.SuccessItems = successCount
		result.FailedItems = result.TotalItems - successCount
		result.Errors = append(result.Errors, importErrors...)
	}

	result.Duration = time.Since(startTime)

	i.logger.WithFields(logrus.Fields{
		"total_items":   result.TotalItems,
		"success_items": result.SuccessItems,
		"failed_items":  result.FailedItems,
		"duration":      result.Duration,
		"dry_run":       req.DryRun,
	}).Info("Import processing completed")

	return result, nil
}

// parseCSV parses CSV data into URLRecord slice
func (i *Importer) parseCSV(data []byte) ([]URLRecord, error) {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.FieldsPerRecord = -1 // Allow variable number of fields

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Create field mapping
	fieldMap := make(map[string]int)
	for i, field := range header {
		fieldMap[strings.ToLower(strings.TrimSpace(field))] = i
	}

	var records []URLRecord
	rowNum := 1 // Start from 1 since header is row 0

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			i.logger.WithError(err).WithField("row", rowNum).Warn("Failed to read CSV row")
			rowNum++
			continue
		}

		record, err := i.parseCSVRow(row, fieldMap, rowNum)
		if err != nil {
			i.logger.WithError(err).WithField("row", rowNum).Warn("Failed to parse CSV row")
			rowNum++
			continue
		}

		records = append(records, *record)
		rowNum++
	}

	return records, nil
}

// parseCSVRow parses a single CSV row into URLRecord
func (i *Importer) parseCSVRow(row []string, fieldMap map[string]int, rowNum int) (*URLRecord, error) {
	record := &URLRecord{
		IsActive:  true, // Default to active
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Original URL (required)
	if idx, exists := fieldMap["original_url"]; exists && idx < len(row) {
		record.OriginalURL = strings.TrimSpace(row[idx])
	} else if idx, exists := fieldMap["url"]; exists && idx < len(row) {
		record.OriginalURL = strings.TrimSpace(row[idx])
	}

	if record.OriginalURL == "" {
		return nil, fmt.Errorf("row %d: original_url is required", rowNum)
	}

	// Short code (optional)
	if idx, exists := fieldMap["short_code"]; exists && idx < len(row) {
		record.ShortCode = strings.TrimSpace(row[idx])
	} else if idx, exists := fieldMap["code"]; exists && idx < len(row) {
		record.ShortCode = strings.TrimSpace(row[idx])
	}

	// Title (optional)
	if idx, exists := fieldMap["title"]; exists && idx < len(row) {
		record.Title = strings.TrimSpace(row[idx])
	}

	// Description (optional)
	if idx, exists := fieldMap["description"]; exists && idx < len(row) {
		record.Description = strings.TrimSpace(row[idx])
	}

	// Tags (optional)
	if idx, exists := fieldMap["tags"]; exists && idx < len(row) {
		tagsStr := strings.TrimSpace(row[idx])
		if tagsStr != "" {
			tags := strings.Split(tagsStr, ",")
			for j := range tags {
				tags[j] = strings.TrimSpace(tags[j])
			}
			record.Tags = tags
		}
	}

	// Expires at (optional)
	if idx, exists := fieldMap["expires_at"]; exists && idx < len(row) {
		expiresStr := strings.TrimSpace(row[idx])
		if expiresStr != "" {
			expiresAt, err := time.Parse(time.RFC3339, expiresStr)
			if err != nil {
				// Try other common formats
				formats := []string{
					"2006-01-02 15:04:05",
					"2006-01-02",
					"01/02/2006",
				}
				for _, format := range formats {
					if expiresAt, err = time.Parse(format, expiresStr); err == nil {
						break
					}
				}
				if err != nil {
					return nil, fmt.Errorf("row %d: invalid expires_at format: %s", rowNum, expiresStr)
				}
			}
			record.ExpiresAt = &expiresAt
		}
	}

	// Password (optional)
	if idx, exists := fieldMap["password"]; exists && idx < len(row) {
		record.Password = strings.TrimSpace(row[idx])
	}

	// Active status (optional)
	if idx, exists := fieldMap["is_active"]; exists && idx < len(row) {
		activeStr := strings.ToLower(strings.TrimSpace(row[idx]))
		record.IsActive = activeStr == "true" || activeStr == "1" || activeStr == "yes"
	} else if idx, exists := fieldMap["active"]; exists && idx < len(row) {
		activeStr := strings.ToLower(strings.TrimSpace(row[idx]))
		record.IsActive = activeStr == "true" || activeStr == "1" || activeStr == "yes"
	}

	return record, nil
}

// parseJSON parses JSON data into URLRecord slice
func (i *Importer) parseJSON(data []byte) ([]URLRecord, error) {
	var records []URLRecord

	// Try to parse as array first
	err := json.Unmarshal(data, &records)
	if err != nil {
		// Try to parse as single object
		var record URLRecord
		err = json.Unmarshal(data, &record)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		records = []URLRecord{record}
	}

	// Set defaults
	for i := range records {
		if records[i].CreatedAt.IsZero() {
			records[i].CreatedAt = time.Now()
		}
		if records[i].UpdatedAt.IsZero() {
			records[i].UpdatedAt = time.Now()
		}
	}

	return records, nil
}

// parseTXT parses plain text data (one URL per line) into URLRecord slice
func (i *Importer) parseTXT(data []byte) ([]URLRecord, error) {
	lines := strings.Split(string(data), "\n")
	var records []URLRecord

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		record := URLRecord{
			OriginalURL: line,
			IsActive:    true,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Basic URL validation
		if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
			i.logger.WithFields(logrus.Fields{
				"line":    lineNum + 1,
				"content": line,
			}).Warn("Line does not appear to be a valid URL")
		}

		records = append(records, record)
	}

	return records, nil
}

// validateRecords validates URL records and returns valid records with errors
func (i *Importer) validateRecords(records []URLRecord) ([]URLRecord, []ImportError) {
	var validRecords []URLRecord
	var errors []ImportError

	for idx, record := range records {
		rowErrors := i.validateRecord(record, idx+1)
		if len(rowErrors) == 0 {
			validRecords = append(validRecords, record)
		} else {
			errors = append(errors, rowErrors...)
		}
	}

	return validRecords, errors
}

// validateRecord validates a single URL record
func (i *Importer) validateRecord(record URLRecord, row int) []ImportError {
	var errors []ImportError

	// Validate original URL
	if record.OriginalURL == "" {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "original_url",
			Value:       record.OriginalURL,
			Error:       "required",
			Description: "Original URL is required",
		})
	} else if !i.isValidURL(record.OriginalURL) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "original_url",
			Value:       record.OriginalURL,
			Error:       "invalid_url",
			Description: "URL format is invalid",
		})
	}

	// Validate short code if provided
	if record.ShortCode != "" {
		if len(record.ShortCode) < 3 || len(record.ShortCode) > 50 {
			errors = append(errors, ImportError{
				Row:         row,
				Field:       "short_code",
				Value:       record.ShortCode,
				Error:       "invalid_length",
				Description: "Short code must be between 3-50 characters",
			})
		} else if !i.isValidShortCode(record.ShortCode) {
			errors = append(errors, ImportError{
				Row:         row,
				Field:       "short_code",
				Value:       record.ShortCode,
				Error:       "invalid_characters",
				Description: "Short code contains invalid characters",
			})
		}
	}

	// Validate title length
	if len(record.Title) > 255 {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "title",
			Value:       record.Title,
			Error:       "too_long",
			Description: "Title must be 255 characters or less",
		})
	}

	// Validate description length
	if len(record.Description) > 500 {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "description",
			Value:       record.Description,
			Error:       "too_long",
			Description: "Description must be 500 characters or less",
		})
	}

	// Validate expiration date
	if record.ExpiresAt != nil && record.ExpiresAt.Before(time.Now()) {
		errors = append(errors, ImportError{
			Row:         row,
			Field:       "expires_at",
			Value:       record.ExpiresAt.Format(time.RFC3339),
			Error:       "past_date",
			Description: "Expiration date cannot be in the past",
		})
	}

	return errors
}

// importRecords imports validated records into the database
func (i *Importer) importRecords(ctx context.Context, records []URLRecord, overwrite bool) (int64, []ImportError) {
	var successCount int64
	var errors []ImportError

	for idx, record := range records {
		err := i.importSingleRecord(ctx, record, overwrite)
		if err != nil {
			errors = append(errors, ImportError{
				Row:         idx + 1,
				Field:       "import",
				Value:       record.OriginalURL,
				Error:       "import_failed",
				Description: err.Error(),
			})
		} else {
			successCount++
		}
	}

	return successCount, errors
}

// importSingleRecord imports a single record into the database
func (i *Importer) importSingleRecord(ctx context.Context, record URLRecord, overwrite bool) error {
	// Generate short code if not provided
	if record.ShortCode == "" {
		record.ShortCode = i.generateShortCode()
	}

	// Check if short code already exists
	exists, err := i.shortCodeExists(ctx, record.ShortCode)
	if err != nil {
		return fmt.Errorf("failed to check short code existence: %w", err)
	}

	if exists {
		if !overwrite {
			return fmt.Errorf("short code '%s' already exists", record.ShortCode)
		}
		// Update existing record
		return i.updateURLRecord(ctx, record)
	}

	// Insert new record
	return i.insertURLRecord(ctx, record)
}

// Helper methods

func (i *Importer) isValidURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func (i *Importer) isValidShortCode(code string) bool {
	// Allow alphanumeric characters, hyphens, and underscores
	for _, char := range code {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

func (i *Importer) generateShortCode() string {
	// Simple short code generation - in production, use a proper algorithm
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	length := 6

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func (i *Importer) shortCodeExists(ctx context.Context, shortCode string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM urls WHERE id = ?)`
	if i.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$1")
	}

	var exists bool
	err := i.db.QueryRow(ctx, query, shortCode).Scan(&exists)
	return exists, err
}

func (i *Importer) insertURLRecord(ctx context.Context, record URLRecord) error {
	query := `
		INSERT INTO urls (id, original_url, title, description, expires_at, password,
		                  created_at, updated_at, user_id, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if i.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		for j := 1; j <= 10; j++ {
			query = strings.Replace(query, "$", fmt.Sprintf("$%d", j), 1)
		}
	}

	_, err := i.db.Exec(ctx, query,
		record.ShortCode, record.OriginalURL, record.Title, record.Description,
		record.ExpiresAt, record.Password, record.CreatedAt, record.UpdatedAt,
		record.UserID, record.IsActive)

	return err
}

func (i *Importer) updateURLRecord(ctx context.Context, record URLRecord) error {
	query := `
		UPDATE urls
		SET original_url = ?, title = ?, description = ?, expires_at = ?,
		    password = ?, updated_at = ?, active = ?
		WHERE id = ? AND user_id = ?`

	if i.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		for j := 1; j <= 9; j++ {
			query = strings.Replace(query, "$", fmt.Sprintf("$%d", j), 1)
		}
	}

	result, err := i.db.Exec(ctx, query,
		record.OriginalURL, record.Title, record.Description, record.ExpiresAt,
		record.Password, time.Now(), record.IsActive, record.ShortCode, record.UserID)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no rows updated - URL may not exist or belong to different user")
	}

	return nil
}