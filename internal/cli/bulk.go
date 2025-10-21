package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type BulkJobResponse struct {
	ID             string                 `json:"id"`
	Type           string                 `json:"type"`
	Status         string                 `json:"status"`
	Progress       *BulkProgress          `json:"progress,omitempty"`
	Result         *BulkResult            `json:"result,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
}

type BulkProgress struct {
	ProcessedItems int64 `json:"processed_items"`
	TotalItems     int64 `json:"total_items"`
	SuccessItems   int64 `json:"success_items"`
	FailedItems    int64 `json:"failed_items"`
	Percentage     int   `json:"percentage"`
}

type BulkResult struct {
	ProcessedItems int64         `json:"processed_items"`
	SuccessItems   int64         `json:"success_items"`
	FailedItems    int64         `json:"failed_items"`
	OutputPath     string        `json:"output_path,omitempty"`
	Errors         []BulkError   `json:"errors,omitempty"`
	Duration       time.Duration `json:"duration"`
}

type BulkError struct {
	Row         int    `json:"row"`
	Field       string `json:"field"`
	Value       string `json:"value"`
	Error       string `json:"error"`
	Description string `json:"description"`
}

var bulkCmd = &cobra.Command{
	Use:   "bulk",
	Short: "Bulk operations",
	Long:  `Import and export URLs in bulk, check operation status.`,
}

var bulkImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import URLs from file",
	Long: `Import URLs from a CSV or JSON file.

Supported formats:
- CSV: columns should be "original_url", "custom_code", "title", "expires_at", "password"
- JSON: array of URL objects

Examples:
  caslink bulk import urls.csv
  caslink bulk import data.json --format json
  caslink bulk import urls.csv --overwrite --batch-size 100`,
	Args: cobra.ExactArgs(1),
	RunE: importURLs,
}

var bulkExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export URLs to file",
	Long: `Export URLs to a CSV, JSON, or Excel file.

Examples:
  caslink bulk export --format csv --output urls.csv
  caslink bulk export --format json --output data.json
  caslink bulk export --format xlsx --output report.xlsx --include-analytics`,
	RunE: exportURLs,
}

var bulkStatusCmd = &cobra.Command{
	Use:   "status [job-id]",
	Short: "Check bulk operation status",
	Long: `Check the status of a bulk operation.

Examples:
  caslink bulk status
  caslink bulk status job123
  caslink bulk status --watch job123`,
	Args: cobra.MaximumNArgs(1),
	RunE: getBulkStatus,
}

// importURLs handles bulk URL import
func importURLs(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	filePath := args[0]
	format, _ := cmd.Flags().GetString("format")
	overwrite, _ := cmd.Flags().GetBool("overwrite")
	batchSize, _ := cmd.Flags().GetInt("batch-size")

	// Auto-detect format from file extension if not specified
	if format == "" {
		switch filepath.Ext(filePath) {
		case ".csv":
			format = "csv"
		case ".json":
			format = "json"
		default:
			format = "csv"
		}
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", filePath)
	}

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add file
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Add form fields
	writer.WriteField("format", format)
	writer.WriteField("overwrite", strconv.FormatBool(overwrite))
	if batchSize > 0 {
		writer.WriteField("batch_size", strconv.Itoa(batchSize))
	}

	writer.Close()

	// Create request
	req, err := http.NewRequest("POST", getServerURL()+"/api/v1/bulk/import", &body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+getAPIToken())

	// Make request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return handleAPIError(resp)
	}

	var job BulkJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(job)
	}

	PrintSuccess(fmt.Sprintf("Import job started: %s", job.ID))
	fmt.Printf("Status: %s\n", job.Status)
	fmt.Printf("File: %s\n", filePath)
	fmt.Printf("Format: %s\n", format)

	if job.Progress != nil {
		fmt.Printf("Progress: %d/%d items processed\n", job.Progress.ProcessedItems, job.Progress.TotalItems)
	}

	fmt.Printf("\nUse 'caslink bulk status %s' to check progress\n", job.ID)

	return nil
}

// exportURLs handles bulk URL export
func exportURLs(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	format, _ := cmd.Flags().GetString("format")
	output, _ := cmd.Flags().GetString("output")
	includeAnalytics, _ := cmd.Flags().GetBool("include-analytics")
	dateFrom, _ := cmd.Flags().GetString("date-from")
	dateTo, _ := cmd.Flags().GetString("date-to")

	if format == "" {
		format = "csv"
	}

	// Build request
	reqData := map[string]interface{}{
		"format":            format,
		"include_analytics": includeAnalytics,
	}

	if dateFrom != "" {
		reqData["date_from"] = dateFrom
	}
	if dateTo != "" {
		reqData["date_to"] = dateTo
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	resp, err := makeAPIRequest("POST", "/api/v1/bulk/export", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return handleAPIError(resp)
	}

	var job BulkJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(job)
	}

	PrintSuccess(fmt.Sprintf("Export job started: %s", job.ID))
	fmt.Printf("Status: %s\n", job.Status)
	fmt.Printf("Format: %s\n", format)

	if output != "" {
		fmt.Printf("Output will be saved to: %s\n", output)
	}

	fmt.Printf("\nUse 'caslink bulk status %s' to check progress\n", job.ID)

	return nil
}

// getBulkStatus checks the status of bulk operations
func getBulkStatus(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	watch, _ := cmd.Flags().GetBool("watch")

	if len(args) == 0 {
		// List all jobs
		return listBulkJobs(cmd)
	}

	jobID := args[0]

	if watch {
		return watchBulkJob(jobID)
	}

	return getBulkJobStatus(jobID)
}

// listBulkJobs lists all bulk jobs
func listBulkJobs(cmd *cobra.Command) error {
	resp, err := makeAPIRequest("GET", "/api/v1/bulk/jobs", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	var jobs struct {
		Jobs  []BulkJobResponse `json:"jobs"`
		Total int               `json:"total"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(jobs)
	}

	if len(jobs.Jobs) == 0 {
		fmt.Println("No bulk jobs found")
		return nil
	}

	fmt.Printf("%-12s %-8s %-10s %-20s %-20s\n", "ID", "Type", "Status", "Created", "Progress")
	fmt.Printf("%-12s %-8s %-10s %-20s %-20s\n", "──", "────", "──────", "───────", "────────")

	for _, job := range jobs.Jobs {
		progress := "N/A"
		if job.Progress != nil {
			progress = fmt.Sprintf("%d%% (%d/%d)", job.Progress.Percentage, job.Progress.ProcessedItems, job.Progress.TotalItems)
		}

		fmt.Printf("%-12s %-8s %-10s %-20s %-20s\n",
			job.ID,
			job.Type,
			job.Status,
			job.CreatedAt.Format("2006-01-02 15:04"),
			progress)
	}

	fmt.Printf("\nTotal: %d jobs\n", jobs.Total)

	return nil
}

// getBulkJobStatus gets status for a specific job
func getBulkJobStatus(jobID string) error {
	resp, err := makeAPIRequest("GET", "/api/v1/bulk/jobs/"+jobID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	var job BulkJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(job)
	}

	fmt.Printf("Job ID: %s\n", job.ID)
	fmt.Printf("Type: %s\n", job.Type)
	fmt.Printf("Status: %s\n", job.Status)
	fmt.Printf("Created: %s\n", job.CreatedAt.Format("2006-01-02 15:04:05"))

	if job.CompletedAt != nil {
		fmt.Printf("Completed: %s\n", job.CompletedAt.Format("2006-01-02 15:04:05"))
	}

	if job.Progress != nil {
		fmt.Printf("Progress: %d%% (%d/%d items)\n", job.Progress.Percentage, job.Progress.ProcessedItems, job.Progress.TotalItems)
		fmt.Printf("Success: %d items\n", job.Progress.SuccessItems)
		fmt.Printf("Failed: %d items\n", job.Progress.FailedItems)
	}

	if job.Result != nil {
		fmt.Printf("\nResults:\n")
		fmt.Printf("  Processed: %d items\n", job.Result.ProcessedItems)
		fmt.Printf("  Success: %d items\n", job.Result.SuccessItems)
		fmt.Printf("  Failed: %d items\n", job.Result.FailedItems)
		fmt.Printf("  Duration: %s\n", job.Result.Duration)

		if job.Result.OutputPath != "" {
			fmt.Printf("  Output: %s\n", job.Result.OutputPath)
		}

		if len(job.Result.Errors) > 0 {
			fmt.Printf("\nErrors (%d):\n", len(job.Result.Errors))
			for i, err := range job.Result.Errors {
				if i >= 10 {
					fmt.Printf("  ... and %d more errors\n", len(job.Result.Errors)-10)
					break
				}
				fmt.Printf("  Row %d: %s - %s\n", err.Row, err.Error, err.Description)
			}
		}
	}

	if job.ErrorMessage != "" {
		fmt.Printf("\nError: %s\n", job.ErrorMessage)
	}

	return nil
}

// watchBulkJob watches a job until completion
func watchBulkJob(jobID string) error {
	fmt.Printf("Watching job %s (Press Ctrl+C to stop)\n\n", jobID)

	for {
		if err := getBulkJobStatus(jobID); err != nil {
			return err
		}

		// Check if job is completed
		resp, err := makeAPIRequest("GET", "/api/v1/bulk/jobs/"+jobID, nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var job BulkJobResponse
		if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		if job.Status == "completed" || job.Status == "failed" {
			fmt.Printf("\nJob %s\n", job.Status)
			break
		}

		time.Sleep(2 * time.Second)
		fmt.Println("\n" + strings.Repeat("─", 50))
	}

	return nil
}

func init() {
	bulkCmd.AddCommand(bulkImportCmd)
	bulkCmd.AddCommand(bulkExportCmd)
	bulkCmd.AddCommand(bulkStatusCmd)

	// Import flags
	bulkImportCmd.Flags().String("format", "", "file format (csv, json) - auto-detected if not specified")
	bulkImportCmd.Flags().Bool("overwrite", false, "overwrite existing URLs with same short code")
	bulkImportCmd.Flags().Int("batch-size", 100, "batch size for processing")

	// Export flags
	bulkExportCmd.Flags().String("format", "csv", "export format (csv, json, xlsx)")
	bulkExportCmd.Flags().String("output", "", "output file path")
	bulkExportCmd.Flags().Bool("include-analytics", false, "include analytics data in export")
	bulkExportCmd.Flags().String("date-from", "", "start date for export (YYYY-MM-DD)")
	bulkExportCmd.Flags().String("date-to", "", "end date for export (YYYY-MM-DD)")

	// Status flags
	bulkStatusCmd.Flags().Bool("watch", false, "watch job progress until completion")
}