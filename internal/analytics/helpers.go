package analytics

import (
	"encoding/json"
	"fmt"
	"time"
)

// JSON parsing helpers for aggregated statistics

// parseJSONToCountryStats parses JSON string to country statistics
func parseJSONToCountryStats(jsonStr string, stats *[]CountryStat) error {
	if jsonStr == "" {
		return nil
	}
	return json.Unmarshal([]byte(jsonStr), stats)
}

// parseJSONToReferrerStats parses JSON string to referrer statistics
func parseJSONToReferrerStats(jsonStr string, stats *[]ReferrerStat) error {
	if jsonStr == "" {
		return nil
	}
	return json.Unmarshal([]byte(jsonStr), stats)
}

// parseJSONToBrowserStats parses JSON string to browser statistics
func parseJSONToBrowserStats(jsonStr string, stats *[]BrowserStat) error {
	if jsonStr == "" {
		return nil
	}
	return json.Unmarshal([]byte(jsonStr), stats)
}

// parseJSONToDeviceStats parses JSON string to device statistics
func parseJSONToDeviceStats(jsonStr string, stats *[]DeviceStat) error {
	if jsonStr == "" {
		return nil
	}
	return json.Unmarshal([]byte(jsonStr), stats)
}

// convertCountryStatsToJSON converts country statistics to JSON string
func convertCountryStatsToJSON(stats []CountryStat) (string, error) {
	if len(stats) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return "[]", fmt.Errorf("failed to marshal country stats: %w", err)
	}
	return string(data), nil
}

// convertReferrerStatsToJSON converts referrer statistics to JSON string
func convertReferrerStatsToJSON(stats []ReferrerStat) (string, error) {
	if len(stats) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return "[]", fmt.Errorf("failed to marshal referrer stats: %w", err)
	}
	return string(data), nil
}

// convertBrowserStatsToJSON converts browser statistics to JSON string
func convertBrowserStatsToJSON(stats []BrowserStat) (string, error) {
	if len(stats) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return "[]", fmt.Errorf("failed to marshal browser stats: %w", err)
	}
	return string(data), nil
}

// convertDeviceStatsToJSON converts device statistics to JSON string
func convertDeviceStatsToJSON(stats []DeviceStat) (string, error) {
	if len(stats) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return "[]", fmt.Errorf("failed to marshal device stats: %w", err)
	}
	return string(data), nil
}

// Helper functions for CSV and PDF export

// generateCSV generates CSV content from analytics data
func (s *Service) generateCSV(data interface{}) ([]byte, error) {
	// This is a simplified CSV generation
	// In production, you might want to use a proper CSV library
	csv := "Date,URL,Clicks,Unique Clicks,Top Country,Top Referrer,Top Browser\n"

	switch v := data.(type) {
	case []Click:
		for _, click := range v {
			country := "Unknown"
			if click.Location != nil {
				country = click.Location.CountryName
			}
			browser := "Unknown"
			if click.DeviceInfo != nil {
				browser = click.DeviceInfo.Browser
			}
			csv += fmt.Sprintf("%s,%s,1,1,%s,%s,%s\n",
				click.ClickedAt.Format("2006-01-02 15:04:05"),
				click.URLID,
				country,
				click.ReferrerDomain,
				browser,
			)
		}
	case []DailyStat:
		for _, stat := range v {
			topCountry := "Unknown"
			if len(stat.TopCountries) > 0 {
				topCountry = stat.TopCountries[0].CountryName
			}
			topReferrer := "Direct"
			if len(stat.TopReferrers) > 0 {
				topReferrer = stat.TopReferrers[0].Domain
			}
			topBrowser := "Unknown"
			if len(stat.TopBrowsers) > 0 {
				topBrowser = stat.TopBrowsers[0].Browser
			}
			csv += fmt.Sprintf("%s,%s,%d,%d,%s,%s,%s\n",
				stat.Date.Format("2006-01-02"),
				stat.URLID,
				stat.Clicks,
				stat.UniqueClicks,
				topCountry,
				topReferrer,
				topBrowser,
			)
		}
	default:
		return nil, fmt.Errorf("unsupported data type for CSV export")
	}

	return []byte(csv), nil
}

// generatePDF generates PDF content from analytics data
func (s *Service) generatePDF(data interface{}) ([]byte, error) {
	// This is a placeholder for PDF generation
	// In production, you would use a proper PDF library like gofpdf

	pdfContent := fmt.Sprintf("Analytics Report\nGenerated: %s\n\nData: %+v",
		time.Now().Format("2006-01-02 15:04:05"), data)

	return []byte(pdfContent), nil
}

// Utility functions for data processing

// mergeLocationData merges location data with click records
func mergeLocationData(clicks []Click, locationMap map[string]*LocationData) []Click {
	for i := range clicks {
		if location, exists := locationMap[clicks[i].IPAddress]; exists {
			clicks[i].Location = location
		}
	}
	return clicks
}

// filterBotTraffic filters out bot traffic from clicks
func filterBotTraffic(clicks []Click) []Click {
	var filtered []Click
	for _, click := range clicks {
		if !click.IsBot {
			filtered = append(filtered, click)
		}
	}
	return filtered
}

// calculateClickPercentages calculates percentages for statistics
func calculateClickPercentages(stats interface{}, total int64) {
	switch v := stats.(type) {
	case []CountryStat:
		for i := range v {
			if total > 0 {
				v[i].Percentage = float64(v[i].Clicks) / float64(total) * 100
			}
		}
	case []ReferrerStat:
		for i := range v {
			if total > 0 {
				v[i].Percentage = float64(v[i].Clicks) / float64(total) * 100
			}
		}
	case []BrowserStat:
		for i := range v {
			if total > 0 {
				v[i].Percentage = float64(v[i].Clicks) / float64(total) * 100
			}
		}
	case []DeviceStat:
		for i := range v {
			if total > 0 {
				v[i].Percentage = float64(v[i].Clicks) / float64(total) * 100
			}
		}
	}
}

// groupClicksByTimeInterval groups clicks by time interval
func groupClicksByTimeInterval(clicks []Click, interval TimeInterval) map[string][]Click {
	groups := make(map[string][]Click)

	for _, click := range clicks {
		var key string
		switch interval {
		case IntervalHour:
			key = click.ClickedAt.Format("2006-01-02 15")
		case IntervalDay:
			key = click.ClickedAt.Format("2006-01-02")
		case IntervalWeek:
			year, week := click.ClickedAt.ISOWeek()
			key = fmt.Sprintf("%d-W%02d", year, week)
		case IntervalMonth:
			key = click.ClickedAt.Format("2006-01")
		case IntervalYear:
			key = click.ClickedAt.Format("2006")
		default:
			key = click.ClickedAt.Format("2006-01-02")
		}

		groups[key] = append(groups[key], click)
	}

	return groups
}

// validateExportRequest validates export request parameters
func validateExportRequest(req *ExportRequest) error {
	if req.StartDate.After(req.EndDate) {
		return fmt.Errorf("start date cannot be after end date")
	}

	if req.EndDate.After(time.Now()) {
		return fmt.Errorf("end date cannot be in the future")
	}

	// Limit to 1 year of data for performance
	if req.EndDate.Sub(req.StartDate) > 365*24*time.Hour {
		return fmt.Errorf("date range cannot exceed 1 year")
	}

	validFormats := map[string]bool{
		"csv":  true,
		"json": true,
		"pdf":  true,
	}

	if !validFormats[req.Format] {
		return fmt.Errorf("unsupported export format: %s", req.Format)
	}

	return nil
}