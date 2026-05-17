// Package banner provides the startup banner for caslink binaries.
package banner

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/common/terminal"
)

// BannerConfig holds the information needed to render the startup banner.
type BannerConfig struct {
	// AppName is the display name of the binary (e.g. "caslink", "caslink-cli").
	AppName string
	// Version is the release version string.
	Version string
	// CommitID is the short git SHA embedded at build time.
	CommitID string
	// BuildDate is the ISO 8601 build timestamp.
	BuildDate string
	// Mode is the application mode ("production" or "development").
	Mode string
	// ServerURL is the public URL the server is listening on.
	ServerURL string
	// AdminURL is the admin panel URL (server binary only; empty for client).
	AdminURL string
	// SetupToken is the first-run setup token (empty after setup completes).
	SetupToken string
	// Debug indicates whether debug mode is active.
	Debug bool
}

// PrintStartupBanner writes the appropriate banner to w based on terminal width.
// It adapts from full (≥80 cols) → compact (60–79) → minimal (40–59) → micro (<40).
func PrintStartupBanner(w io.Writer, cfg BannerConfig) {
	if w == nil {
		w = os.Stdout
	}
	size := terminal.GetTerminalSize()

	switch {
	case size.IsFull():
		printFull(w, cfg)
	case size.IsCompact():
		printCompact(w, cfg)
	case size.IsMinimal():
		printMinimal(w, cfg)
	default:
		printMicro(w, cfg)
	}
}

// printFull renders the banner with full ASCII art logo (≥80 columns).
func printFull(w io.Writer, cfg BannerConfig) {
	fmt.Fprintln(w, asciiLogo)
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintf(w, "  %-20s %s\n", "Application:", cfg.AppName)
	fmt.Fprintf(w, "  %-20s %s\n", "Version:", cfg.Version)
	if cfg.CommitID != "" && cfg.CommitID != "unknown" {
		fmt.Fprintf(w, "  %-20s %s\n", "Commit:", cfg.CommitID)
	}
	if cfg.BuildDate != "" && cfg.BuildDate != "unknown" {
		fmt.Fprintf(w, "  %-20s %s\n", "Built:", cfg.BuildDate)
	}
	fmt.Fprintf(w, "  %-20s %s\n", "Mode:", cfg.Mode)
	if cfg.Debug {
		fmt.Fprintf(w, "  %-20s enabled\n", "Debug:")
	}
	if cfg.ServerURL != "" {
		fmt.Fprintf(w, "  %-20s %s\n", "Server URL:", cfg.ServerURL)
	}
	if cfg.AdminURL != "" {
		fmt.Fprintf(w, "  %-20s %s\n", "Admin URL:", cfg.AdminURL)
	}
	if cfg.SetupToken != "" {
		fmt.Fprintln(w, strings.Repeat("─", 60))
		fmt.Fprintln(w, "  FIRST-RUN SETUP TOKEN (expires in 1 hour):")
		fmt.Fprintf(w, "  %s\n", cfg.SetupToken)
	}
	fmt.Fprintln(w, strings.Repeat("─", 60))
	fmt.Fprintf(w, "  Started at %s\n\n", time.Now().Format(time.RFC1123))
}

// printCompact renders a banner without ASCII art (60–79 columns).
func printCompact(w io.Writer, cfg BannerConfig) {
	fmt.Fprintln(w, strings.Repeat("─", 50))
	fmt.Fprintf(w, "  CASLINK  %s  (%s)\n", cfg.Version, cfg.Mode)
	if cfg.ServerURL != "" {
		fmt.Fprintf(w, "  URL: %s\n", cfg.ServerURL)
	}
	if cfg.SetupToken != "" {
		fmt.Fprintf(w, "  Setup token: %s\n", cfg.SetupToken)
	}
	fmt.Fprintln(w, strings.Repeat("─", 50))
}

// printMinimal renders a single-line summary (40–59 columns).
func printMinimal(w io.Writer, cfg BannerConfig) {
	fmt.Fprintf(w, "caslink %s [%s]\n", cfg.Version, cfg.Mode)
	if cfg.SetupToken != "" {
		fmt.Fprintf(w, "token: %s\n", cfg.SetupToken)
	}
}

// printMicro renders an absolute minimum banner (<40 columns).
func printMicro(w io.Writer, cfg BannerConfig) {
	fmt.Fprintf(w, "caslink %s\n", cfg.Version)
}
