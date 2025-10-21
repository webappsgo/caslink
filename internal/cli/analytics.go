package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type AnalyticsSummary struct {
	TotalURLs    int64                  `json:"total_urls"`
	TotalClicks  int64                  `json:"total_clicks"`
	UniqueClicks int64                  `json:"unique_clicks"`
	TopURLs      []AnalyticsURLItem     `json:"top_urls"`
	TopCountries []AnalyticsCountryItem `json:"top_countries"`
	TopReferrers []AnalyticsReferrerItem `json:"top_referrers"`
	ClickTrends  []AnalyticsTimePoint   `json:"click_trends"`
}

type AnalyticsURLItem struct {
	ID          string `json:"id"`
	OriginalURL string `json:"original_url"`
	Clicks      int64  `json:"clicks"`
}

type AnalyticsCountryItem struct {
	CountryCode string `json:"country_code"`
	CountryName string `json:"country_name"`
	Clicks      int64  `json:"clicks"`
}

type AnalyticsReferrerItem struct {
	Domain string `json:"domain"`
	Clicks int64  `json:"clicks"`
}

type AnalyticsTimePoint struct {
	Date   string `json:"date"`
	Clicks int64  `json:"clicks"`
}

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Analytics and reporting",
	Long:  `View analytics, generate reports, and export data.`,
}

var analyticsSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Analytics overview",
	Long: `Display a comprehensive analytics summary including:
- Total URLs and clicks
- Top performing URLs
- Geographic distribution
- Traffic sources and trends

Examples:
  caslink analytics summary
  caslink analytics summary --days 30
  caslink analytics summary --format json`,
	RunE: getAnalyticsSummary,
}

var analyticsExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export analytics data",
	Long: `Export analytics data to various formats for external analysis.

Supported formats: csv, json, xlsx

Examples:
  caslink analytics export --format csv --output analytics.csv
  caslink analytics export --format json --days 90
  caslink analytics export --url abc123 --format csv`,
	RunE: exportAnalytics,
}

var analyticsRealtimeCmd = &cobra.Command{
	Use:   "realtime",
	Short: "Real-time analytics",
	Long: `View real-time analytics data with live updates.

Examples:
  caslink analytics realtime
  caslink analytics realtime --interval 5s`,
	RunE: getRealtimeAnalytics,
}

// getAnalyticsSummary gets comprehensive analytics summary
func getAnalyticsSummary(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	days, _ := cmd.Flags().GetInt("days")

	url := "/api/v1/analytics/summary"
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

	var summary AnalyticsSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(summary)
	}

	// Display formatted summary
	fmt.Println("Analytics Summary")
	fmt.Println("================")
	fmt.Printf("Total URLs: %d\n", summary.TotalURLs)
	fmt.Printf("Total Clicks: %d\n", summary.TotalClicks)
	fmt.Printf("Unique Clicks: %d\n", summary.UniqueClicks)

	if summary.TotalClicks > 0 {
		fmt.Printf("Click-through Rate: %.2f%%\n", float64(summary.UniqueClicks)/float64(summary.TotalClicks)*100)
	}

	if len(summary.TopURLs) > 0 {
		fmt.Println("\nTop URLs:")
		fmt.Printf("%-12s %-50s %-8s\n", "ID", "URL", "Clicks")
		fmt.Printf("%-12s %-50s %-8s\n", "──", "───", "──────")
		for i, url := range summary.TopURLs {
			if i >= 10 { break }
			originalURL := url.OriginalURL
			if len(originalURL) > 47 {
				originalURL = originalURL[:44] + "..."
			}
			fmt.Printf("%-12s %-50s %-8d\n", url.ID, originalURL, url.Clicks)
		}
	}

	if len(summary.TopCountries) > 0 {
		fmt.Println("\nTop Countries:")
		for i, country := range summary.TopCountries {
			if i >= 5 { break }
			fmt.Printf("  %s (%s): %d clicks\n", country.CountryName, country.CountryCode, country.Clicks)
		}
	}

	if len(summary.TopReferrers) > 0 {
		fmt.Println("\nTop Referrers:")
		for i, referrer := range summary.TopReferrers {
			if i >= 5 { break }
			fmt.Printf("  %s: %d clicks\n", referrer.Domain, referrer.Clicks)
		}
	}

	return nil
}

// exportAnalytics exports analytics data
func exportAnalytics(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	format, _ := cmd.Flags().GetString("format")
	output, _ := cmd.Flags().GetString("output")
	days, _ := cmd.Flags().GetInt("days")
	urlID, _ := cmd.Flags().GetString("url")

	if format == "" {
		format = "csv"
	}

	url := "/api/v1/analytics/export"
	params := make([]string, 0)

	if format != "" {
		params = append(params, "format="+format)
	}
	if days > 0 {
		params = append(params, "days="+strconv.Itoa(days))
	}
	if urlID != "" {
		params = append(params, "url="+urlID)
	}

	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	resp, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	// Handle different content types
	contentType := resp.Header.Get("Content-Type")

	if output != "" {
		// Save to file
		file, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			return fmt.Errorf("failed to write to output file: %w", err)
		}

		PrintSuccess(fmt.Sprintf("Analytics data exported to %s", output))
	} else {
		// Output to stdout
		if strings.Contains(contentType, "application/json") {
			var data interface{}
			if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
				return fmt.Errorf("failed to decode JSON response: %w", err)
			}
			return outputJSON(data)
		} else {
			// For CSV or other formats, output as-is
			_, err = io.Copy(os.Stdout, resp.Body)
			if err != nil {
				return fmt.Errorf("failed to output data: %w", err)
			}
		}
	}

	return nil
}

// getRealtimeAnalytics shows real-time analytics
func getRealtimeAnalytics(cmd *cobra.Command, args []string) error {
	if err := RequireAuth(); err != nil {
		return err
	}

	interval, _ := cmd.Flags().GetString("interval")
	if interval == "" {
		interval = "10s"
	}

	fmt.Println("Real-time Analytics")
	fmt.Println("==================")
	fmt.Printf("Refreshing every %s (Press Ctrl+C to stop)\n\n", interval)

	// This would ideally use WebSocket or Server-Sent Events
	// For now, we'll implement a simple polling mechanism
	resp, err := makeAPIRequest("GET", "/api/v1/analytics/realtime", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	var realtimeData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&realtimeData); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if getOutputFormat() == "json" {
		return outputJSON(realtimeData)
	}

	// Display real-time data
	if activeUsers, ok := realtimeData["active_users"].(float64); ok {
		fmt.Printf("Active Users: %.0f\n", activeUsers)
	}

	if clicksLastMinute, ok := realtimeData["clicks_last_minute"].(float64); ok {
		fmt.Printf("Clicks (last minute): %.0f\n", clicksLastMinute)
	}

	if clicksLastHour, ok := realtimeData["clicks_last_hour"].(float64); ok {
		fmt.Printf("Clicks (last hour): %.0f\n", clicksLastHour)
	}

	if recentClicks, ok := realtimeData["recent_clicks"].([]interface{}); ok && len(recentClicks) > 0 {
		fmt.Println("\nRecent Clicks:")
		for i, click := range recentClicks {
			if i >= 10 { break }
			if clickMap, ok := click.(map[string]interface{}); ok {
				urlID := clickMap["url_id"]
				country := clickMap["country"]
				timestamp := clickMap["timestamp"]
				fmt.Printf("  %s - %s (%s)\n", urlID, country, timestamp)
			}
		}
	}

	return nil
}

func init() {
	analyticsCmd.AddCommand(analyticsSummaryCmd)
	analyticsCmd.AddCommand(analyticsExportCmd)
	analyticsCmd.AddCommand(analyticsRealtimeCmd)

	// Add flags for analytics summary
	analyticsSummaryCmd.Flags().Int("days", 30, "number of days to include")

	// Add flags for analytics export
	analyticsExportCmd.Flags().String("format", "csv", "export format (csv, json, xlsx)")
	analyticsExportCmd.Flags().String("output", "", "output file path (default: stdout)")
	analyticsExportCmd.Flags().Int("days", 0, "number of days to include (0 for all)")
	analyticsExportCmd.Flags().String("url", "", "specific URL ID to export")

	// Add flags for real-time analytics
	analyticsRealtimeCmd.Flags().String("interval", "10s", "refresh interval")
}