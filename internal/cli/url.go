package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type URLResponse struct {
	ID          string    `json:"id"`
	OriginalURL string    `json:"original_url"`
	ShortURL    string    `json:"short_url"`
	Title       string    `json:"title"`
	Clicks      int64     `json:"clicks"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
}

type URLListResponse struct {
	URLs  []URLResponse `json:"urls"`
	Total int           `json:"total"`
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
}

type URLCreateRequest struct {
	OriginalURL string     `json:"original_url"`
	CustomCode  string     `json:"custom_code,omitempty"`
	Title       string     `json:"title,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Password    string     `json:"password,omitempty"`
}

var urlCmd = &cobra.Command{
	Use:   "url",
	Short: "URL management",
	Long:  `Manage short URLs - create, list, update, delete, and view analytics.`,
}

var urlCreateCmd = &cobra.Command{
	Use:   "create <url>",
	Short: "Create a short URL",
	Long: `Create a new short URL with optional customization.

Examples:
  caslink url create https://example.com
  caslink url create https://example.com --custom my-link
  caslink url create https://example.com --title "My Website" --expires "2024-12-31"`,
	Args: cobra.ExactArgs(1),
	RunE: createURL,
}

var urlListCmd = &cobra.Command{
	Use:   "list",
	Short: "List URLs",
	Long: `List all URLs owned by the authenticated user.

Examples:
  caslink url list
  caslink url list --limit 50
  caslink url list --page 2`,
	RunE: listURLs,
}

var urlGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get URL details",
	Long: `Get detailed information about a specific URL.

Examples:
  caslink url get abc123
  caslink url get my-custom-link`,
	Args: cobra.ExactArgs(1),
	RunE: getURL,
}

var urlUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update URL",
	Long: `Update properties of an existing URL.

Examples:
  caslink url update abc123 --title "New Title"
  caslink url update my-link --expires "2025-01-01"`,
	Args: cobra.ExactArgs(1),
	RunE: updateURL,
}

var urlDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete URL",
	Long: `Delete a URL permanently. This action cannot be undone.

Examples:
  caslink url delete abc123
  caslink url delete my-custom-link`,
	Args: cobra.ExactArgs(1),
	RunE: deleteURL,
}

var urlAnalyticsCmd = &cobra.Command{
	Use:   "analytics <id>",
	Short: "View URL analytics",
	Long: `View detailed analytics for a specific URL.

Examples:
  caslink url analytics abc123
  caslink url analytics my-link --days 30`,
	Args: cobra.ExactArgs(1),
	RunE: getURLAnalytics,
}

// createURL creates a new short URL
func createURL(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	originalURL := args[0]
	customCode, _ := cmd.Flags().GetString("custom")
	title, _ := cmd.Flags().GetString("title")
	expiresStr, _ := cmd.Flags().GetString("expires")
	password, _ := cmd.Flags().GetString("password")

	req := URLCreateRequest{
		OriginalURL: originalURL,
		CustomCode:  customCode,
		Title:       title,
		Password:    password,
	}

	if expiresStr != "" {
		expiresAt, err := time.Parse("2006-01-02", expiresStr)
		if err != nil {
			return fmt.Errorf("invalid expiration date format. Use YYYY-MM-DD: %w", err)
		}
		req.ExpiresAt = &expiresAt
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	resp, err := makeAPIRequest("POST", "/api/v1/urls", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return handleAPIError(resp)
	}

	var urlResp URLResponse
	if err := json.NewDecoder(resp.Body).Decode(&urlResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(urlResp)
	}

	PrintSuccess(fmt.Sprintf("Short URL created: %s", urlResp.ShortURL))
	fmt.Printf("ID: %s\n", urlResp.ID)
	fmt.Printf("Original URL: %s\n", urlResp.OriginalURL)
	if urlResp.Title != "" {
		fmt.Printf("Title: %s\n", urlResp.Title)
	}
	if urlResp.ExpiresAt != nil {
		fmt.Printf("Expires: %s\n", urlResp.ExpiresAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}

// listURLs lists all URLs for the authenticated user
func listURLs(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	page, _ := cmd.Flags().GetInt("page")
	limit, _ := cmd.Flags().GetInt("limit")

	url := "/api/v1/urls"
	if page > 0 || limit > 0 {
		url += "?"
		if page > 0 {
			url += fmt.Sprintf("page=%d&", page)
		}
		if limit > 0 {
			url += fmt.Sprintf("limit=%d&", limit)
		}
		url = strings.TrimSuffix(url, "&")
	}

	resp, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	var listResp URLListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(listResp)
	}

	if len(listResp.URLs) == 0 {
		fmt.Println("No URLs found")
		return nil
	}

	// Table output
	fmt.Printf("%-12s %-50s %-8s %-20s\n", "ID", "Original URL", "Clicks", "Created")
	fmt.Printf("%-12s %-50s %-8s %-20s\n", "──", "──────────", "──────", "───────")

	for _, url := range listResp.URLs {
		originalURL := url.OriginalURL
		if len(originalURL) > 47 {
			originalURL = originalURL[:44] + "..."
		}
		fmt.Printf("%-12s %-50s %-8d %-20s\n",
			url.ID,
			originalURL,
			url.Clicks,
			url.CreatedAt.Format("2006-01-02 15:04"))
	}

	fmt.Printf("\nShowing %d of %d URLs", len(listResp.URLs), listResp.Total)
	if listResp.Page > 1 {
		fmt.Printf(" (page %d)", listResp.Page)
	}
	fmt.Println()

	return nil
}

// getURL gets details for a specific URL
func getURL(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	urlID := args[0]
	resp, err := makeAPIRequest("GET", "/api/v1/urls/"+urlID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	var urlResp URLResponse
	if err := json.NewDecoder(resp.Body).Decode(&urlResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(urlResp)
	}

	fmt.Printf("ID: %s\n", urlResp.ID)
	fmt.Printf("Short URL: %s\n", urlResp.ShortURL)
	fmt.Printf("Original URL: %s\n", urlResp.OriginalURL)
	if urlResp.Title != "" {
		fmt.Printf("Title: %s\n", urlResp.Title)
	}
	fmt.Printf("Clicks: %d\n", urlResp.Clicks)
	fmt.Printf("Created: %s\n", urlResp.CreatedAt.Format("2006-01-02 15:04:05"))
	if urlResp.ExpiresAt != nil {
		fmt.Printf("Expires: %s\n", urlResp.ExpiresAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}

// updateURL updates an existing URL
func updateURL(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	urlID := args[0]
	title, _ := cmd.Flags().GetString("title")
	expiresStr, _ := cmd.Flags().GetString("expires")

	req := make(map[string]interface{})

	if title != "" {
		req["title"] = title
	}

	if expiresStr != "" {
		if expiresStr == "never" {
			req["expires_at"] = nil
		} else {
			expiresAt, err := time.Parse("2006-01-02", expiresStr)
			if err != nil {
				return fmt.Errorf("invalid expiration date format. Use YYYY-MM-DD or 'never': %w", err)
			}
			req["expires_at"] = expiresAt
		}
	}

	if len(req) == 0 {
		return fmt.Errorf("no updates specified")
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	resp, err := makeAPIRequest("PUT", "/api/v1/urls/"+urlID, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	var urlResp URLResponse
	if err := json.NewDecoder(resp.Body).Decode(&urlResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(urlResp)
	}

	PrintSuccess(fmt.Sprintf("URL %s updated successfully", urlID))
	return nil
}

// deleteURL deletes a URL
func deleteURL(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	urlID := args[0]

	// Confirm deletion unless in quiet mode
	if !Confirm(fmt.Sprintf("Are you sure you want to delete URL '%s'?", urlID)) {
		fmt.Println("Deletion cancelled")
		return nil
	}

	resp, err := makeAPIRequest("DELETE", "/api/v1/urls/"+urlID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return handleAPIError(resp)
	}

	PrintSuccess(fmt.Sprintf("URL %s deleted successfully", urlID))
	return nil
}

// getURLAnalytics gets analytics for a specific URL
func getURLAnalytics(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	urlID := args[0]
	days, _ := cmd.Flags().GetInt("days")

	url := "/api/v1/urls/" + urlID + "/analytics"
	if days > 0 {
		url += "?days=" + strconv.Itoa(days)
	}

	resp, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	var analytics map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&analytics); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(analytics)
	}

	fmt.Printf("Analytics for URL: %s\n\n", urlID)

	if totalClicks, ok := analytics["total_clicks"].(float64); ok {
		fmt.Printf("Total Clicks: %.0f\n", totalClicks)
	}

	if uniqueClicks, ok := analytics["unique_clicks"].(float64); ok {
		fmt.Printf("Unique Clicks: %.0f\n", uniqueClicks)
	}

	if topCountries, ok := analytics["top_countries"].([]interface{}); ok && len(topCountries) > 0 {
		fmt.Println("\nTop Countries:")
		for i, country := range topCountries {
			if i >= 5 { break }
			if countryMap, ok := country.(map[string]interface{}); ok {
				name := countryMap["name"]
				count := countryMap["count"]
				fmt.Printf("  %s: %.0f\n", name, count)
			}
		}
	}

	if topReferrers, ok := analytics["top_referrers"].([]interface{}); ok && len(topReferrers) > 0 {
		fmt.Println("\nTop Referrers:")
		for i, referrer := range topReferrers {
			if i >= 5 { break }
			if referrerMap, ok := referrer.(map[string]interface{}); ok {
				name := referrerMap["name"]
				count := referrerMap["count"]
				fmt.Printf("  %s: %.0f\n", name, count)
			}
		}
	}

	return nil
}

func init() {
	urlCmd.AddCommand(urlCreateCmd)
	urlCmd.AddCommand(urlListCmd)
	urlCmd.AddCommand(urlGetCmd)
	urlCmd.AddCommand(urlUpdateCmd)
	urlCmd.AddCommand(urlDeleteCmd)
	urlCmd.AddCommand(urlAnalyticsCmd)

	// Add flags for URL creation
	urlCreateCmd.Flags().String("custom", "", "custom short code")
	urlCreateCmd.Flags().String("title", "", "URL title")
	urlCreateCmd.Flags().String("expires", "", "expiration date (YYYY-MM-DD)")
	urlCreateCmd.Flags().String("password", "", "password protection")

	// Add flags for URL listing
	urlListCmd.Flags().Int("page", 1, "page number")
	urlListCmd.Flags().Int("limit", 25, "items per page")

	// Add flags for URL update
	urlUpdateCmd.Flags().String("title", "", "update URL title")
	urlUpdateCmd.Flags().String("expires", "", "update expiration date (YYYY-MM-DD or 'never')")

	// Add flags for analytics
	urlAnalyticsCmd.Flags().Int("days", 30, "number of days to include in analytics")
}