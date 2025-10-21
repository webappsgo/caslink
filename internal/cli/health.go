package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check application health",
	Long: `Check the health status of the Caslink application.

This command is used for health checks in Docker containers and monitoring systems.

Examples:
  caslink health
  caslink health --timeout 5s`,
	RunE: checkHealth,
}

type HealthResponse struct {
	Status    string                 `json:"status"`
	Version   string                 `json:"version"`
	Timestamp string                 `json:"timestamp"`
	Checks    map[string]interface{} `json:"checks"`
	Database  map[string]interface{} `json:"database"`
}

type APIHealthResponse struct {
	Success bool            `json:"success"`
	Data    *HealthResponse `json:"data"`
	Error   string          `json:"error"`
}

func checkHealth(cmd *cobra.Command, args []string) error {
	timeout, _ := cmd.Flags().GetDuration("timeout")
	serverURL := getServerURL()

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Make health check request
	resp, err := client.Get(serverURL + "/health")
	if err != nil {
		if !quiet {
			PrintError(fmt.Sprintf("Health check failed: %v", err))
		}
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		if !quiet {
			PrintError(fmt.Sprintf("Health check returned status %d", resp.StatusCode))
		}
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	// Parse response
	var apiResponse APIHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		if !quiet {
			PrintError(fmt.Sprintf("Failed to parse health response: %v", err))
		}
		return fmt.Errorf("failed to parse health response: %w", err)
	}

	// Check API response success
	if !apiResponse.Success {
		if !quiet {
			PrintError(fmt.Sprintf("Health check API error: %s", apiResponse.Error))
		}
		return fmt.Errorf("health check API error: %s", apiResponse.Error)
	}

	health := apiResponse.Data
	if health == nil {
		if !quiet {
			PrintError("No health data in response")
		}
		return fmt.Errorf("no health data in response")
	}

	// Output health status
	if getOutputFormat() == "json" {
		return outputJSON(health)
	}

	// Check if healthy
	if health.Status != "ok" && health.Status != "healthy" {
		if !quiet {
			PrintError(fmt.Sprintf("Application is unhealthy: %s", health.Status))
		}
		return fmt.Errorf("application is unhealthy: %s", health.Status)
	}

	if !quiet {
		PrintSuccess(fmt.Sprintf("Application is healthy (status: %s)", health.Status))
		if health.Version != "" {
			fmt.Printf("Version: %s\n", health.Version)
		}
		if health.Timestamp != "" {
			fmt.Printf("Timestamp: %s\n", health.Timestamp)
		}

		// Show detailed checks if available
		if len(health.Checks) > 0 {
			fmt.Println("\nHealth Checks:")
			for name, status := range health.Checks {
				statusStr := fmt.Sprintf("%v", status)
				if statusStr == "true" || statusStr == "ok" || statusStr == "healthy" {
					fmt.Printf("  ✓ %s: %s\n", name, statusStr)
				} else {
					fmt.Printf("  ✗ %s: %s\n", name, statusStr)
				}
			}
		}

		// Show database status if available
		if len(health.Database) > 0 {
			fmt.Println("\nDatabase Status:")
			if status, ok := health.Database["status"]; ok {
				statusStr := fmt.Sprintf("%v", status)
				if statusStr == "healthy" {
					fmt.Printf("  ✓ Status: %s\n", statusStr)
				} else {
					fmt.Printf("  ✗ Status: %s\n", statusStr)
				}
			}
			if dbType, ok := health.Database["type"]; ok {
				fmt.Printf("  • Type: %v\n", dbType)
			}
			if responseTime, ok := health.Database["response_time"]; ok {
				fmt.Printf("  • Response Time: %v\n", responseTime)
			}
		}
	}

	return nil
}

func init() {
	healthCmd.Flags().Duration("timeout", 10*time.Second, "health check timeout")
}