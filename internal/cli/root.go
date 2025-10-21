package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile      string
	serverURL    string
	apiToken     string
	outputFormat string
	verbose      bool
	quiet        bool
	noColor      bool
	version      string
	commit       string
	date         string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "caslink",
	Short: "Caslink URL Shortener CLI",
	Long: `Caslink is a comprehensive URL shortener with analytics, QR codes,
bulk operations, and more. This CLI tool allows you to manage URLs,
view analytics, and perform administrative tasks.

Examples:
  caslink webui                     # Start web server
  caslink url create https://example.com
  caslink url list
  caslink analytics summary
  caslink bulk import urls.csv`,
	Version: getVersionString(),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initLogging()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

// ExecuteStandalone executes the CLI in standalone mode
func ExecuteStandalone(ctx context.Context) error {
	// Set CLI-specific behavior for standalone mode
	rootCmd.Use = "caslink-cli"
	return rootCmd.ExecuteContext(ctx)
}

// SetVersionInfo sets the version information
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d
	rootCmd.Version = getVersionString()
}

func getVersionString() string {
	if version == "" {
		version = "dev"
	}
	return fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.caslink.yaml)")
	rootCmd.PersistentFlags().StringVarP(&serverURL, "server", "s", "", "server URL (default: auto-detect)")
	rootCmd.PersistentFlags().StringVarP(&apiToken, "token", "t", "", "API authentication token")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "output format (table, json, yaml, csv)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "quiet mode (errors only)")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")

	// Bind flags to viper
	viper.BindPFlag("server.url", rootCmd.PersistentFlags().Lookup("server"))
	viper.BindPFlag("api.token", rootCmd.PersistentFlags().Lookup("token"))
	viper.BindPFlag("output.format", rootCmd.PersistentFlags().Lookup("output"))
	viper.BindPFlag("logging.verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("logging.quiet", rootCmd.PersistentFlags().Lookup("quiet"))
	viper.BindPFlag("output.no_color", rootCmd.PersistentFlags().Lookup("no-color"))

	// Add subcommands
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(urlCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(analyticsCmd)
	rootCmd.AddCommand(bulkCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(completionCmd)
}

// initConfig reads in config file and ENV variables
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".caslink" (without extension)
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/caslink")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".caslink")
	}

	// Environment variable prefix
	viper.SetEnvPrefix("CASLINK")
	viper.AutomaticEnv()

	// Set defaults
	setDefaults()

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil && verbose {
		fmt.Fprintf(os.Stderr, "Using config file: %s\n", viper.ConfigFileUsed())
	}
}

// setDefaults sets default configuration values
func setDefaults() {
	viper.SetDefault("server.url", "http://localhost:64000")
	viper.SetDefault("output.format", "table")
	viper.SetDefault("output.no_color", false)
	viper.SetDefault("logging.verbose", false)
	viper.SetDefault("logging.quiet", false)
	viper.SetDefault("api.timeout", "30s")
	viper.SetDefault("api.retry_attempts", 3)
}

// initLogging initializes logging configuration
func initLogging() {
	logger := logrus.New()

	// Set log level
	if quiet {
		logger.SetLevel(logrus.ErrorLevel)
	} else if verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	// Set formatter
	if noColor || os.Getenv("NO_COLOR") != "" {
		logger.SetFormatter(&logrus.TextFormatter{
			DisableColors: true,
			FullTimestamp: true,
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			DisableColors: false,
			FullTimestamp: true,
			ForceColors:   true,
		})
	}

	// Set as global logger
	logrus.SetOutput(logger.Out)
	logrus.SetLevel(logger.Level)
	logrus.SetFormatter(logger.Formatter)
}

// Helper functions for CLI operations

// getServerURL returns the configured server URL
func getServerURL() string {
	if serverURL != "" {
		return serverURL
	}
	return viper.GetString("server.url")
}

// getAPIToken returns the configured API token
func getAPIToken() string {
	if apiToken != "" {
		return apiToken
	}
	return viper.GetString("api.token")
}

// getOutputFormat returns the configured output format
func getOutputFormat() string {
	if outputFormat != "" {
		return outputFormat
	}
	return viper.GetString("output.format")
}

// CLIError represents a CLI error with exit code
type CLIError struct {
	Message  string
	ExitCode int
}

func (e *CLIError) Error() string {
	return e.Message
}

// NewCLIError creates a new CLI error
func NewCLIError(message string, exitCode int) *CLIError {
	return &CLIError{
		Message:  message,
		ExitCode: exitCode,
	}
}

// PrintSuccess prints a success message
func PrintSuccess(message string) {
	if !quiet {
		if noColor {
			fmt.Printf("✓ %s\n", message)
		} else {
			fmt.Printf("\033[32m✓\033[0m %s\n", message)
		}
	}
}

// PrintWarning prints a warning message
func PrintWarning(message string) {
	if !quiet {
		if noColor {
			fmt.Printf("⚠ %s\n", message)
		} else {
			fmt.Printf("\033[33m⚠\033[0m %s\n", message)
		}
	}
}

// PrintError prints an error message
func PrintError(message string) {
	if noColor {
		fmt.Fprintf(os.Stderr, "✗ %s\n", message)
	} else {
		fmt.Fprintf(os.Stderr, "\033[31m✗\033[0m %s\n", message)
	}
}

// RequireAuth ensures authentication is configured
func RequireAuth() error {
	if getAPIToken() == "" {
		return NewCLIError("API token required. Set with --token or configure with 'caslink config auth'", 1)
	}
	return nil
}

// IsConfigured checks if the CLI is properly configured
func IsConfigured() bool {
	return getServerURL() != "" && (getAPIToken() != "" || viper.GetBool("server.allow_anonymous"))
}

// Confirm prompts for user confirmation
func Confirm(message string) bool {
	if quiet {
		return false
	}

	fmt.Printf("%s [y/N]: ", message)
	var response string
	fmt.Scanln(&response)

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// makeAPIRequest makes an HTTP request to the API
func makeAPIRequest(method, path string, body interface{}) (*http.Response, error) {
	serverURL := getServerURL()
	token := getAPIToken()

	// Create request URL
	url := strings.TrimSuffix(serverURL, "/") + path

	// Prepare request body
	var bodyReader io.Reader
	if body != nil {
		if reader, ok := body.(io.Reader); ok {
			bodyReader = reader
		} else {
			jsonData, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			bodyReader = bytes.NewReader(jsonData)
		}
	}

	// Create HTTP request
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("caslink-cli/%s", version))

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Make the request
	client := &http.Client{}
	return client.Do(req)
}

// handleAPIError handles API error responses
func handleAPIError(resp *http.Response) error {
	var errorResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Code    int    `json:"code"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
		return fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	message := errorResp.Message
	if message == "" {
		message = errorResp.Error
	}
	if message == "" {
		message = fmt.Sprintf("Request failed with status %d", resp.StatusCode)
	}

	return &CLIError{
		Message:  message,
		ExitCode: 1,
	}
}

// outputJSON outputs data as JSON
func outputJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// outputYAML outputs data as YAML (basic implementation)
func outputYAML(data interface{}) error {
	// For now, convert to JSON and then format as YAML-like structure
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Simple YAML-like conversion
	yamlData := strings.ReplaceAll(string(jsonData), "\":", ":")
	yamlData = strings.ReplaceAll(yamlData, "\",", ",")
	yamlData = strings.ReplaceAll(yamlData, "\"", "")
	yamlData = strings.ReplaceAll(yamlData, "{", "")
	yamlData = strings.ReplaceAll(yamlData, "}", "")
	yamlData = strings.ReplaceAll(yamlData, "[", "")
	yamlData = strings.ReplaceAll(yamlData, "]", "")

	fmt.Println(yamlData)
	return nil
}