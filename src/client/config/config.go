// Package config handles CLI configuration loading and saving for caslink-cli.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// CLIConfig is the persistent configuration for caslink-cli.
type CLIConfig struct {
	// Server is the base URL of the caslink server (e.g. https://link.example.com).
	Server string `yaml:"server"`
	// Token is the API bearer token stored for reuse between invocations.
	// Stored as plain text in a 0600 file — keep the file mode enforced.
	Token string `yaml:"token,omitempty"`
	// Display holds display preferences.
	Display struct {
		Mode string `yaml:"mode"` // "auto", "cli", "tui"
	} `yaml:"display"`
	// Update holds auto-update preferences.
	Update struct {
		Auto    bool   `yaml:"auto"`
		Channel string `yaml:"channel"` // "stable", "beta", "daily"
	} `yaml:"update"`
	// Lang is the preferred language code (e.g. "en", "fr").
	Lang string `yaml:"lang"`
	// Color controls ANSI output: "on", "off", or "auto".
	Color string `yaml:"color"`
}

// defaultConfig returns a CLIConfig populated with sensible defaults.
func defaultConfig() CLIConfig {
	cfg := CLIConfig{}
	cfg.Display.Mode = "auto"
	cfg.Update.Auto = false
	cfg.Update.Channel = "stable"
	cfg.Lang = "en"
	cfg.Color = "auto"
	return cfg
}

// GetConfigDir returns the platform-appropriate directory for caslink-cli config.
//
//   - Unix:    ~/.config/casapps/caslink/
//   - Windows: %APPDATA%\casapps\caslink\
func GetConfigDir() (string, error) {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA environment variable not set")
		}
		return filepath.Join(appData, "casapps", "caslink"), nil
	}

	// Unix: respect XDG_CONFIG_HOME
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "casapps", "caslink"), nil
}

// configFilePath returns the path to cli.yml.
func configFilePath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cli.yml"), nil
}

// GetTokenFile returns the path to the standalone token file.
func GetTokenFile() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "token"), nil
}

// LoadCLIConfig reads cli.yml from the platform config directory.
// If the file does not exist, a default config is returned without error.
// Returns an error when the file has group- or world-readable permissions.
func LoadCLIConfig() (*CLIConfig, error) {
	path, err := configFilePath()
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		cfg := defaultConfig()
		return &cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	// Refuse to use a config file that is group- or world-readable.
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf(
			"config file %s has insecure permissions %04o — run: chmod 600 %s",
			path, info.Mode().Perm(), path,
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return &cfg, nil
}

// SaveCLIConfig writes cfg to cli.yml with 0600 permissions.
// The directory is created with 0700 if it does not exist.
func SaveCLIConfig(cfg *CLIConfig) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Write to a temp file then rename for atomicity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename config file: %w", err)
	}

	return nil
}
