package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// getEnvString gets a string environment variable with fallback
func getEnvString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// getEnvInt gets an integer environment variable with fallback
func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return fallback
}

// getEnvBool gets a boolean environment variable with fallback
func getEnvBool(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		boolVal, err := strconv.ParseBool(value)
		if err == nil {
			return boolVal
		}
	}
	return fallback
}

// getEnvDuration gets a duration environment variable with fallback
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return fallback
}

// getEnvStringSlice gets a comma-separated string slice environment variable with fallback
func getEnvStringSlice(key string, fallback []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return fallback
}

// DetectEnvironment detects the deployment environment
func DetectEnvironment() string {
	// Check for explicit environment setting
	if env := os.Getenv("CASLINK_ENVIRONMENT"); env != "" {
		return env
	}

	// Check for container environments
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "kubernetes"
	}
	if os.Getenv("DOCKER_CONTAINER") != "" || os.Getenv("container") != "" {
		return "docker"
	}

	// Check for cloud environments
	if os.Getenv("AWS_EXECUTION_ENV") != "" || os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		return "aws"
	}
	if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" || os.Getenv("GAE_APPLICATION") != "" {
		return "gcp"
	}
	if os.Getenv("WEBSITE_SITE_NAME") != "" {
		return "azure"
	}

	// Check for development environment
	if os.Getenv("NODE_ENV") == "development" || os.Getenv("GO_ENV") == "development" {
		return "development"
	}

	// Default to production
	return "production"
}

// IsProduction returns true if running in production environment
func IsProduction() bool {
	env := DetectEnvironment()
	return env == "production" || env == "kubernetes" || env == "aws" || env == "gcp" || env == "azure"
}

// IsDevelopment returns true if running in development environment
func IsDevelopment() bool {
	return DetectEnvironment() == "development"
}

// DetectCloudProvider detects the cloud provider
func DetectCloudProvider() string {
	// AWS
	if os.Getenv("AWS_EXECUTION_ENV") != "" ||
	   os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" ||
	   os.Getenv("AWS_REGION") != "" {
		return "aws"
	}

	// Google Cloud
	if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" ||
	   os.Getenv("GAE_APPLICATION") != "" ||
	   os.Getenv("GCP_PROJECT") != "" {
		return "gcp"
	}

	// Azure
	if os.Getenv("WEBSITE_SITE_NAME") != "" ||
	   os.Getenv("AZURE_CLIENT_ID") != "" {
		return "azure"
	}

	// DigitalOcean
	if os.Getenv("DO_REGION") != "" {
		return "digitalocean"
	}

	return "unknown"
}