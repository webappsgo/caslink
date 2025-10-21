package bulk

import (
	"fmt"
	"strings"
	"time"
)

// Job statuses
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// Supported formats
const (
	FormatCSV  = "csv"
	FormatJSON = "json"
	FormatXLSX = "xlsx"
	FormatTXT  = "txt"
)

// ImportRequest represents a request to import URLs
type ImportRequest struct {
	UserID       string `json:"user_id"`
	Format       string `json:"format"`       // csv, json, xlsx
	Data         []byte `json:"data"`         // File content
	Filename     string `json:"filename"`     // Original filename
	Overwrite    bool   `json:"overwrite"`    // Overwrite existing URLs
	ValidateURLs bool   `json:"validate_urls"` // Validate URLs before import
	DryRun       bool   `json:"dry_run"`      // Preview import without saving
}

// ImportResponse represents a response to an import request
type ImportResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// ExportRequest represents a request to export URLs
type ExportRequest struct {
	UserID    string            `json:"user_id"`
	Format    string            `json:"format"`    // csv, json, xlsx
	Filters   map[string]string `json:"filters"`   // URL filters
	Fields    []string          `json:"fields"`    // Fields to include
	StartDate time.Time         `json:"start_date,omitempty"`
	EndDate   time.Time         `json:"end_date,omitempty"`
	URLIDs    []string          `json:"url_ids,omitempty"` // Specific URLs to export
}

// ExportResponse represents a response to an export request
type ExportResponse struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// ImportJob represents an import job
type ImportJob struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	Format         string     `json:"format"`
	Filename       string     `json:"filename"`
	Status         string     `json:"status"`
	TotalItems     int64      `json:"total_items"`
	ProcessedItems int64      `json:"processed_items"`
	SuccessItems   int64      `json:"success_items"`
	FailedItems    int64      `json:"failed_items"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Errors         []ImportError `json:"errors,omitempty"`
}

// ExportJob represents an export job
type ExportJob struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Format       string     `json:"format"`
	Status       string     `json:"status"`
	TotalItems   int64      `json:"total_items"`
	OutputPath   string     `json:"output_path,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// ImportError represents an error during import
type ImportError struct {
	Row         int    `json:"row"`
	Field       string `json:"field"`
	Value       string `json:"value"`
	Error       string `json:"error"`
	Description string `json:"description"`
}

// URLRecord represents a URL record for import/export
type URLRecord struct {
	ID          string    `json:"id,omitempty"`
	OriginalURL string    `json:"original_url"`
	ShortCode   string    `json:"short_code,omitempty"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Password    string    `json:"password,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	Clicks      int64     `json:"clicks,omitempty"`
	UniqueClicks int64    `json:"unique_clicks,omitempty"`
	IsActive    bool      `json:"is_active"`
	UserID      string    `json:"user_id,omitempty"`
}

// ImportResult represents the result of an import operation
type ImportResult struct {
	TotalItems   int64         `json:"total_items"`
	SuccessItems int64         `json:"success_items"`
	FailedItems  int64         `json:"failed_items"`
	Errors       []ImportError `json:"errors"`
	Duration     time.Duration `json:"duration"`
}

// ExportResult represents the result of an export operation
type ExportResult struct {
	TotalItems int64         `json:"total_items"`
	OutputPath string        `json:"output_path"`
	Duration   time.Duration `json:"duration"`
}

// ValidationResult represents the result of URL validation
type ValidationResult struct {
	Valid   bool     `json:"valid"`
	Errors  []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	URL     string   `json:"url"`
}

// BulkStats represents bulk operation statistics
type BulkStats struct {
	ImportStats *ImportStats `json:"import_stats"`
	ExportStats *ExportStats `json:"export_stats"`
}

// ImportStats represents import operation statistics
type ImportStats struct {
	TotalJobs       int64 `json:"total_jobs"`
	CompletedJobs   int64 `json:"completed_jobs"`
	FailedJobs      int64 `json:"failed_jobs"`
	PendingJobs     int64 `json:"pending_jobs"`
	TotalURLs       int64 `json:"total_urls"`
	SuccessfulURLs  int64 `json:"successful_urls"`
	FailedURLs      int64 `json:"failed_urls"`
	AverageJobTime  float64 `json:"average_job_time_seconds"`
	LastImportAt    *time.Time `json:"last_import_at,omitempty"`
}

// ExportStats represents export operation statistics
type ExportStats struct {
	TotalJobs      int64 `json:"total_jobs"`
	CompletedJobs  int64 `json:"completed_jobs"`
	FailedJobs     int64 `json:"failed_jobs"`
	PendingJobs    int64 `json:"pending_jobs"`
	TotalURLs      int64 `json:"total_urls"`
	AverageJobTime float64 `json:"average_job_time_seconds"`
	LastExportAt   *time.Time `json:"last_export_at,omitempty"`
}

// DownloadResponse represents a file download response
type DownloadResponse struct {
	Content     []byte `json:"-"`
	ContentType string `json:"content_type"`
	Filename    string `json:"filename"`
}

// ProcessingOptions represents options for bulk processing
type ProcessingOptions struct {
	BatchSize      int           `json:"batch_size"`
	MaxConcurrency int           `json:"max_concurrency"`
	Timeout        time.Duration `json:"timeout"`
	RetryAttempts  int           `json:"retry_attempts"`
	ValidateURLs   bool          `json:"validate_urls"`
	SkipDuplicates bool          `json:"skip_duplicates"`
}

// CSVRecord represents a CSV record structure
type CSVRecord struct {
	OriginalURL string `csv:"original_url"`
	ShortCode   string `csv:"short_code"`
	Title       string `csv:"title"`
	Description string `csv:"description"`
	Tags        string `csv:"tags"`
	ExpiresAt   string `csv:"expires_at"`
	Password    string `csv:"password"`
	IsActive    string `csv:"is_active"`
}

// JobProgress represents the progress of a bulk job
type JobProgress struct {
	JobID          string  `json:"job_id"`
	Status         string  `json:"status"`
	TotalItems     int64   `json:"total_items"`
	ProcessedItems int64   `json:"processed_items"`
	SuccessItems   int64   `json:"success_items"`
	FailedItems    int64   `json:"failed_items"`
	PercentComplete float64 `json:"percent_complete"`
	EstimatedTimeRemaining time.Duration `json:"estimated_time_remaining,omitempty"`
	CurrentOperation string `json:"current_operation,omitempty"`
}

// Template represents a bulk operation template
type Template struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        string            `json:"type"` // import, export
	Format      string            `json:"format"`
	Fields      []string          `json:"fields"`
	Mapping     map[string]string `json:"mapping"` // Field mapping
	Options     ProcessingOptions `json:"options"`
	UserID      string            `json:"user_id"`
	IsPublic    bool              `json:"is_public"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	UsageCount  int64             `json:"usage_count"`
}

// TemplateRequest represents a request to create/update a template
type TemplateRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        string            `json:"type"`
	Format      string            `json:"format"`
	Fields      []string          `json:"fields"`
	Mapping     map[string]string `json:"mapping"`
	Options     ProcessingOptions `json:"options"`
	IsPublic    bool              `json:"is_public"`
}

// QueueStatus represents the status of the bulk processing queue
type QueueStatus struct {
	PendingJobs  int64 `json:"pending_jobs"`
	RunningJobs  int64 `json:"running_jobs"`
	QueueLength  int   `json:"queue_length"`
	WorkerCount  int   `json:"worker_count"`
	ActiveWorkers int  `json:"active_workers"`
}

// Default values and limits
const (
	DefaultBatchSize      = 1000
	DefaultMaxConcurrency = 5
	DefaultTimeout        = 300 // 5 minutes
	DefaultRetryAttempts  = 3
	MaxBatchSize          = 10000
	MaxFileSize           = 100 * 1024 * 1024 // 100MB
	MaxConcurrency        = 10
)

// GetProgress calculates the progress percentage
func (job *ImportJob) GetProgress() *JobProgress {
	progress := &JobProgress{
		JobID:          job.ID,
		Status:         job.Status,
		TotalItems:     job.TotalItems,
		ProcessedItems: job.ProcessedItems,
		SuccessItems:   job.SuccessItems,
		FailedItems:    job.FailedItems,
	}

	if job.TotalItems > 0 {
		progress.PercentComplete = float64(job.ProcessedItems) / float64(job.TotalItems) * 100
	}

	return progress
}

// GetProgress calculates the progress percentage for export jobs
func (job *ExportJob) GetProgress() *JobProgress {
	progress := &JobProgress{
		JobID:      job.ID,
		Status:     job.Status,
		TotalItems: job.TotalItems,
	}

	// For export jobs, we don't track processed items the same way
	// Progress is mainly based on status
	switch job.Status {
	case StatusPending:
		progress.PercentComplete = 0
	case StatusRunning:
		progress.PercentComplete = 50 // Rough estimate
	case StatusCompleted:
		progress.PercentComplete = 100
	case StatusFailed, StatusCancelled:
		progress.PercentComplete = 0
	}

	return progress
}

// IsCompleted checks if the job is in a terminal state
func (job *ImportJob) IsCompleted() bool {
	return job.Status == StatusCompleted || job.Status == StatusFailed || job.Status == StatusCancelled
}

// IsCompleted checks if the job is in a terminal state
func (job *ExportJob) IsCompleted() bool {
	return job.Status == StatusCompleted || job.Status == StatusFailed || job.Status == StatusCancelled
}

// Validate validates an import request
func (req *ImportRequest) Validate() error {
	if req.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	if len(req.Data) == 0 {
		return fmt.Errorf("data is required")
	}

	if len(req.Data) > MaxFileSize {
		return fmt.Errorf("file size exceeds maximum limit of %d bytes", MaxFileSize)
	}

	validFormats := map[string]bool{
		FormatCSV:  true,
		FormatJSON: true,
		FormatXLSX: true,
		FormatTXT:  true,
	}

	if !validFormats[req.Format] {
		return fmt.Errorf("unsupported format: %s", req.Format)
	}

	return nil
}

// Validate validates an export request
func (req *ExportRequest) Validate() error {
	if req.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	validFormats := map[string]bool{
		FormatCSV:  true,
		FormatJSON: true,
		FormatXLSX: true,
		FormatTXT:  true,
	}

	if !validFormats[req.Format] {
		return fmt.Errorf("unsupported format: %s", req.Format)
	}

	if !req.StartDate.IsZero() && !req.EndDate.IsZero() {
		if req.StartDate.After(req.EndDate) {
			return fmt.Errorf("start date cannot be after end date")
		}
	}

	return nil
}

// SetDefaults sets default values for processing options
func (opts *ProcessingOptions) SetDefaults() {
	if opts.BatchSize <= 0 {
		opts.BatchSize = DefaultBatchSize
	}
	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = DefaultMaxConcurrency
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout * time.Second
	}
	if opts.RetryAttempts < 0 {
		opts.RetryAttempts = DefaultRetryAttempts
	}

	// Apply limits
	if opts.BatchSize > MaxBatchSize {
		opts.BatchSize = MaxBatchSize
	}
	if opts.MaxConcurrency > MaxConcurrency {
		opts.MaxConcurrency = MaxConcurrency
	}
}

// ToCSVRecord converts a URLRecord to CSVRecord
func (record *URLRecord) ToCSVRecord() *CSVRecord {
	csvRecord := &CSVRecord{
		OriginalURL: record.OriginalURL,
		ShortCode:   record.ShortCode,
		Title:       record.Title,
		Description: record.Description,
		Password:    record.Password,
	}

	if len(record.Tags) > 0 {
		csvRecord.Tags = strings.Join(record.Tags, ",")
	}

	if record.ExpiresAt != nil {
		csvRecord.ExpiresAt = record.ExpiresAt.Format(time.RFC3339)
	}

	if record.IsActive {
		csvRecord.IsActive = "true"
	} else {
		csvRecord.IsActive = "false"
	}

	return csvRecord
}

// FromCSVRecord creates a URLRecord from CSVRecord
func (record *URLRecord) FromCSVRecord(csvRecord *CSVRecord) error {
	record.OriginalURL = csvRecord.OriginalURL
	record.ShortCode = csvRecord.ShortCode
	record.Title = csvRecord.Title
	record.Description = csvRecord.Description
	record.Password = csvRecord.Password

	if csvRecord.Tags != "" {
		record.Tags = strings.Split(csvRecord.Tags, ",")
		// Trim whitespace from tags
		for i := range record.Tags {
			record.Tags[i] = strings.TrimSpace(record.Tags[i])
		}
	}

	if csvRecord.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, csvRecord.ExpiresAt)
		if err != nil {
			return fmt.Errorf("invalid expires_at format: %w", err)
		}
		record.ExpiresAt = &expiresAt
	}

	record.IsActive = strings.ToLower(csvRecord.IsActive) == "true"

	return nil
}