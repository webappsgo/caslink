package bulk

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Service handles bulk operations coordination
type Service struct {
	db        *db.DB
	config    *config.BulkConfig
	logger    *logrus.Logger
	importer  *Importer
	exporter  *Exporter
	validator *Validator
	processor *Processor
}

// NewService creates a new bulk operations service
func NewService(database *db.DB, cfg *config.BulkConfig, logger *logrus.Logger) (*Service, error) {
	importer, err := NewImporter(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create importer: %w", err)
	}

	exporter, err := NewExporter(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	validator, err := NewValidator(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create validator: %w", err)
	}

	processor, err := NewProcessor(database, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create processor: %w", err)
	}

	return &Service{
		db:        database,
		config:    cfg,
		logger:    logger,
		importer:  importer,
		exporter:  exporter,
		validator: validator,
		processor: processor,
	}, nil
}

// ImportURLs imports URLs from uploaded data
func (s *Service) ImportURLs(ctx context.Context, req *ImportRequest) (*ImportResponse, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("bulk operations are disabled")
	}

	// Validate request
	if err := s.validateImportRequest(req); err != nil {
		return nil, fmt.Errorf("invalid import request: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":      req.UserID,
		"format":       req.Format,
		"data_size":    len(req.Data),
		"filename":     req.Filename,
		"overwrite":    req.Overwrite,
		"validate_urls": req.ValidateURLs,
	}).Info("Starting bulk import")

	// Generate import job ID
	jobID := uuid.New().String()

	// Create import job record
	job := &ImportJob{
		ID:          jobID,
		UserID:      req.UserID,
		Format:      req.Format,
		Filename:    req.Filename,
		Status:      StatusPending,
		TotalItems:  0,
		ProcessedItems: 0,
		SuccessItems: 0,
		FailedItems: 0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Store job record
	if err := s.createImportJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create import job: %w", err)
	}

	// Start background processing
	go s.processImportAsync(ctx, jobID, req)

	return &ImportResponse{
		JobID:     jobID,
		Status:    StatusPending,
		Message:   "Import job queued for processing",
		CreatedAt: job.CreatedAt,
	}, nil
}

// ExportURLs exports URLs to specified format
func (s *Service) ExportURLs(ctx context.Context, req *ExportRequest) (*ExportResponse, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("bulk operations are disabled")
	}

	// Validate request
	if err := s.validateExportRequest(req); err != nil {
		return nil, fmt.Errorf("invalid export request: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":    req.UserID,
		"format":     req.Format,
		"filters":    req.Filters,
		"start_date": req.StartDate,
		"end_date":   req.EndDate,
	}).Info("Starting bulk export")

	// Generate export job ID
	jobID := uuid.New().String()

	// Create export job record
	job := &ExportJob{
		ID:         jobID,
		UserID:     req.UserID,
		Format:     req.Format,
		Status:     StatusPending,
		TotalItems: 0,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Store job record
	if err := s.createExportJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create export job: %w", err)
	}

	// Start background processing
	go s.processExportAsync(ctx, jobID, req)

	return &ExportResponse{
		JobID:     jobID,
		Status:    StatusPending,
		Message:   "Export job queued for processing",
		CreatedAt: job.CreatedAt,
	}, nil
}

// GetImportJob retrieves import job status
func (s *Service) GetImportJob(ctx context.Context, jobID string, userID string) (*ImportJob, error) {
	job, err := s.getImportJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get import job: %w", err)
	}

	// Check authorization
	if job.UserID != userID {
		return nil, fmt.Errorf("access denied")
	}

	return job, nil
}

// GetExportJob retrieves export job status
func (s *Service) GetExportJob(ctx context.Context, jobID string, userID string) (*ExportJob, error) {
	job, err := s.getExportJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get export job: %w", err)
	}

	// Check authorization
	if job.UserID != userID {
		return nil, fmt.Errorf("access denied")
	}

	return job, nil
}

// ListImportJobs lists import jobs for a user
func (s *Service) ListImportJobs(ctx context.Context, userID string, limit, offset int) ([]*ImportJob, int64, error) {
	return s.listImportJobs(ctx, userID, limit, offset)
}

// ListExportJobs lists export jobs for a user
func (s *Service) ListExportJobs(ctx context.Context, userID string, limit, offset int) ([]*ExportJob, int64, error) {
	return s.listExportJobs(ctx, userID, limit, offset)
}

// CancelJob cancels a running job
func (s *Service) CancelJob(ctx context.Context, jobID string, userID string) error {
	// Try import job first
	if job, err := s.getImportJob(ctx, jobID); err == nil {
		if job.UserID != userID {
			return fmt.Errorf("access denied")
		}
		if job.Status == StatusRunning || job.Status == StatusPending {
			return s.updateImportJobStatus(ctx, jobID, StatusCancelled, "Cancelled by user")
		}
		return fmt.Errorf("job cannot be cancelled (status: %s)", job.Status)
	}

	// Try export job
	if job, err := s.getExportJob(ctx, jobID); err == nil {
		if job.UserID != userID {
			return fmt.Errorf("access denied")
		}
		if job.Status == StatusRunning || job.Status == StatusPending {
			return s.updateExportJobStatus(ctx, jobID, StatusCancelled, "Cancelled by user")
		}
		return fmt.Errorf("job cannot be cancelled (status: %s)", job.Status)
	}

	return fmt.Errorf("job not found")
}

// DownloadExport downloads the export file
func (s *Service) DownloadExport(ctx context.Context, jobID string, userID string) (*DownloadResponse, error) {
	job, err := s.getExportJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get export job: %w", err)
	}

	// Check authorization
	if job.UserID != userID {
		return nil, fmt.Errorf("access denied")
	}

	if job.Status != StatusCompleted {
		return nil, fmt.Errorf("export not ready (status: %s)", job.Status)
	}

	if job.OutputPath == "" {
		return nil, fmt.Errorf("export file not found")
	}

	// Read the export file
	content, err := s.exporter.ReadExportFile(job.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read export file: %w", err)
	}

	return &DownloadResponse{
		Content:     content,
		ContentType: s.getContentType(job.Format),
		Filename:    s.generateExportFilename(job),
	}, nil
}

// GetBulkStats returns bulk operation statistics
func (s *Service) GetBulkStats(ctx context.Context, userID string) (*BulkStats, error) {
	stats := &BulkStats{}

	// Get import stats
	importStats, err := s.getImportStats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get import stats: %w", err)
	}
	stats.ImportStats = importStats

	// Get export stats
	exportStats, err := s.getExportStats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get export stats: %w", err)
	}
	stats.ExportStats = exportStats

	return stats, nil
}

// CleanupOldJobs removes old completed/failed jobs
func (s *Service) CleanupOldJobs(ctx context.Context) error {
	if s.config.RetentionDays <= 0 {
		return nil // Unlimited retention
	}

	cutoffDate := time.Now().AddDate(0, 0, -s.config.RetentionDays)

	// Cleanup import jobs
	deletedImports, err := s.cleanupOldImportJobs(ctx, cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to cleanup import jobs: %w", err)
	}

	// Cleanup export jobs
	deletedExports, err := s.cleanupOldExportJobs(ctx, cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to cleanup export jobs: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"cutoff_date":     cutoffDate,
		"deleted_imports": deletedImports,
		"deleted_exports": deletedExports,
	}).Info("Bulk jobs cleanup completed")

	return nil
}

// Background processing methods

func (s *Service) processImportAsync(ctx context.Context, jobID string, req *ImportRequest) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.WithFields(logrus.Fields{
				"job_id": jobID,
				"error":  r,
			}).Error("Import job panicked")
			s.updateImportJobStatus(ctx, jobID, StatusFailed, fmt.Sprintf("Job panicked: %v", r))
		}
	}()

	// Update status to running
	if err := s.updateImportJobStatus(ctx, jobID, StatusRunning, "Processing import"); err != nil {
		s.logger.WithError(err).Error("Failed to update job status to running")
		return
	}

	// Process the import
	result, err := s.importer.ProcessImport(ctx, req)
	if err != nil {
		s.logger.WithError(err).WithField("job_id", jobID).Error("Import processing failed")
		s.updateImportJobStatus(ctx, jobID, StatusFailed, err.Error())
		return
	}

	// Update job with results
	if err := s.updateImportJobResults(ctx, jobID, result); err != nil {
		s.logger.WithError(err).WithField("job_id", jobID).Error("Failed to update job results")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"job_id":         jobID,
		"total_items":    result.TotalItems,
		"success_items":  result.SuccessItems,
		"failed_items":   result.FailedItems,
	}).Info("Import job completed successfully")
}

func (s *Service) processExportAsync(ctx context.Context, jobID string, req *ExportRequest) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.WithFields(logrus.Fields{
				"job_id": jobID,
				"error":  r,
			}).Error("Export job panicked")
			s.updateExportJobStatus(ctx, jobID, StatusFailed, fmt.Sprintf("Job panicked: %v", r))
		}
	}()

	// Update status to running
	if err := s.updateExportJobStatus(ctx, jobID, StatusRunning, "Processing export"); err != nil {
		s.logger.WithError(err).Error("Failed to update job status to running")
		return
	}

	// Process the export
	result, err := s.exporter.ProcessExport(ctx, req)
	if err != nil {
		s.logger.WithError(err).WithField("job_id", jobID).Error("Export processing failed")
		s.updateExportJobStatus(ctx, jobID, StatusFailed, err.Error())
		return
	}

	// Update job with results
	if err := s.updateExportJobResults(ctx, jobID, result); err != nil {
		s.logger.WithError(err).WithField("job_id", jobID).Error("Failed to update job results")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"job_id":      jobID,
		"total_items": result.TotalItems,
		"output_path": result.OutputPath,
	}).Info("Export job completed successfully")
}

// Validation methods

func (s *Service) validateImportRequest(req *ImportRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	if len(req.Data) == 0 {
		return fmt.Errorf("import data is required")
	}

	if len(req.Data) > s.config.MaxFileSize {
		return fmt.Errorf("file size %d exceeds maximum %d", len(req.Data), s.config.MaxFileSize)
	}

	if !s.isValidFormat(req.Format) {
		return fmt.Errorf("unsupported format: %s", req.Format)
	}

	return nil
}

func (s *Service) validateExportRequest(req *ExportRequest) error {
	if req.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	if !s.isValidFormat(req.Format) {
		return fmt.Errorf("unsupported format: %s", req.Format)
	}

	if !req.StartDate.IsZero() && !req.EndDate.IsZero() && req.StartDate.After(req.EndDate) {
		return fmt.Errorf("start date cannot be after end date")
	}

	return nil
}

func (s *Service) isValidFormat(format string) bool {
	validFormats := map[string]bool{
		"csv":  true,
		"json": true,
		"xlsx": true,
		"txt":  true,
	}
	return validFormats[strings.ToLower(format)]
}

func (s *Service) getContentType(format string) string {
	switch strings.ToLower(format) {
	case "csv":
		return "text/csv"
	case "json":
		return "application/json"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func (s *Service) generateExportFilename(job *ExportJob) string {
	timestamp := job.CreatedAt.Format("2006-01-02_15-04-05")
	extension := strings.ToLower(job.Format)
	return fmt.Sprintf("urls_export_%s.%s", timestamp, extension)
}

// Database operations (these would be implemented with actual SQL queries)

func (s *Service) createImportJob(ctx context.Context, job *ImportJob) error {
	query := `
		INSERT INTO import_jobs
		(id, user_id, format, filename, status, total_items, processed_items,
		 success_items, failed_items, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		for i := 1; i <= 11; i++ {
			query = strings.Replace(query, "$", fmt.Sprintf("$%d", i), 1)
		}
	}

	_, err := s.db.Exec(ctx, query,
		job.ID, job.UserID, job.Format, job.Filename, job.Status,
		job.TotalItems, job.ProcessedItems, job.SuccessItems, job.FailedItems,
		job.CreatedAt, job.UpdatedAt)

	return err
}

func (s *Service) createExportJob(ctx context.Context, job *ExportJob) error {
	query := `
		INSERT INTO export_jobs
		(id, user_id, format, status, total_items, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$")
		for i := 1; i <= 7; i++ {
			query = strings.Replace(query, "$", fmt.Sprintf("$%d", i), 1)
		}
	}

	_, err := s.db.Exec(ctx, query,
		job.ID, job.UserID, job.Format, job.Status, job.TotalItems,
		job.CreatedAt, job.UpdatedAt)

	return err
}

func (s *Service) getImportJob(ctx context.Context, jobID string) (*ImportJob, error) {
	query := `
		SELECT id, user_id, format, filename, status, total_items, processed_items,
		       success_items, failed_items, error_message, created_at, updated_at,
		       completed_at
		FROM import_jobs
		WHERE id = ?`

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$1")
	}

	var job ImportJob
	var completedAt *time.Time

	err := s.db.QueryRow(ctx, query, jobID).Scan(
		&job.ID, &job.UserID, &job.Format, &job.Filename, &job.Status,
		&job.TotalItems, &job.ProcessedItems, &job.SuccessItems, &job.FailedItems,
		&job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt, &completedAt)

	if err != nil {
		return nil, err
	}

	if completedAt != nil {
		job.CompletedAt = completedAt
	}

	return &job, nil
}

func (s *Service) getExportJob(ctx context.Context, jobID string) (*ExportJob, error) {
	query := `
		SELECT id, user_id, format, status, total_items, output_path,
		       error_message, created_at, updated_at, completed_at
		FROM export_jobs
		WHERE id = ?`

	if s.db.Type() == "postgres" {
		query = strings.ReplaceAll(query, "?", "$1")
	}

	var job ExportJob
	var completedAt *time.Time

	err := s.db.QueryRow(ctx, query, jobID).Scan(
		&job.ID, &job.UserID, &job.Format, &job.Status, &job.TotalItems,
		&job.OutputPath, &job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt, &completedAt)

	if err != nil {
		return nil, err
	}

	if completedAt != nil {
		job.CompletedAt = completedAt
	}

	return &job, nil
}

// Additional helper methods would be implemented here for:
// - updateImportJobStatus
// - updateExportJobStatus
// - updateImportJobResults
// - updateExportJobResults
// - listImportJobs
// - listExportJobs
// - getImportStats
// - getExportStats
// - cleanupOldImportJobs
// - cleanupOldExportJobs