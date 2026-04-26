package mode

import (
	"os"
	"strings"

	"github.com/casjaysdevdocker/caslink/src/config"
)

// Mode represents the application mode
type Mode string

const (
	// Production mode - optimized for production use
	Production Mode = "production"

	// Development mode - enables debug features, verbose logging
	Development Mode = "development"
)

// String returns the string representation of the mode
func (m Mode) String() string {
	return string(m)
}

// IsProduction returns true if running in production mode
func (m Mode) IsProduction() bool {
	return m == Production
}

// IsDevelopment returns true if running in development mode
func (m Mode) IsDevelopment() bool {
	return m == Development
}

// Detect determines the application mode from various sources.
//
// Priority order:
// 1. CLI flag (--mode)
// 2. Environment variable (MODE, CASLINK_MODE, APP_ENV, ENV, ENVIRONMENT)
// 3. Config file (server.mode)
// 4. Default: production
//
// Development mode is enabled if ANY of:
//   - Mode explicitly set to "development" or "dev"
//   - DEBUG environment variable is truthy
//   - Running in container with DEBUG=true
//
// Production mode is the default for safety.
func Detect(cliMode string, configMode string) Mode {
	// Priority 1: CLI flag
	if cliMode != "" {
		return parseMode(cliMode)
	}

	// Priority 2: Environment variables (check multiple common names)
	envVars := []string{
		"MODE",
		"CASLINK_MODE",
		"APP_ENV",
		"ENV",
		"ENVIRONMENT",
	}

	for _, envVar := range envVars {
		if value := os.Getenv(envVar); value != "" {
			return parseMode(value)
		}
	}

	// Check DEBUG flag (common in containers)
	if config.ParseBool(os.Getenv("DEBUG")) {
		return Development
	}

	// Priority 3: Config file
	if configMode != "" {
		return parseMode(configMode)
	}

	// Default: production (safe default)
	return Production
}

// parseMode parses a mode string into a Mode type
func parseMode(s string) Mode {
	normalized := strings.ToLower(strings.TrimSpace(s))

	switch normalized {
	case "development", "dev", "devel", "debug":
		return Development
	case "production", "prod", "release":
		return Production
	default:
		// Unknown mode defaults to production for safety
		return Production
	}
}

// IsContainer detects if running inside a container
func IsContainer() bool {
	// Check for common container indicators
	indicators := []string{
		"/.dockerenv",                    // Docker
		"/.containerenv",                 // Podman
		"/run/.containerenv",             // Podman alternative
		"/proc/1/cgroup",                 // cgroup check (read separately)
	}

	for _, path := range indicators {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	// Check environment variables
	containerEnvVars := []string{
		"KUBERNETES_SERVICE_HOST", // Kubernetes
		"CONTAINER",                // Generic container flag
		"DOCKER_CONTAINER",         // Docker
	}

	for _, envVar := range containerEnvVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}

	// Check if running under container init systems
	if isContainerInit() {
		return true
	}

	return false
}

// isContainerInit checks if the parent process is a container init system
func isContainerInit() bool {
	// Read /proc/1/comm to see what PID 1 is
	data, err := os.ReadFile("/proc/1/comm")
	if err != nil {
		return false
	}

	pid1 := strings.TrimSpace(string(data))

	// Common container init systems
	containerInits := []string{
		"tini",
		"dumb-init",
		"s6-svscan",
		"runsv",
		"runsvdir",
		"docker-init",
	}

	for _, init := range containerInits {
		if pid1 == init {
			return true
		}
	}

	return false
}

// GetModeInfo returns detailed information about the current mode
func GetModeInfo(m Mode) map[string]interface{} {
	return map[string]interface{}{
		"mode":         m.String(),
		"production":   m.IsProduction(),
		"development":  m.IsDevelopment(),
		"in_container": IsContainer(),
		"debug_mode":   m.IsDevelopment(),
	}
}
