//go:build !windows

// Package theme provides color palettes for caslink TUI and CLI output.
package theme

import (
	"os/exec"
	"runtime"
	"strings"
)

// IsSystemDarkTheme returns true when the OS reports a dark color scheme.
// Falls back to true (dark) when the scheme cannot be determined.
func IsSystemDarkTheme() bool {
	switch runtime.GOOS {
	case "linux":
		return linuxIsDark()
	case "darwin":
		return darwinIsDark()
	default:
		return true
	}
}

// linuxIsDark queries gsettings for the GNOME color scheme.
func linuxIsDark() bool {
	cmd := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme")
	out, err := cmd.Output()
	if err != nil {
		return true
	}
	return strings.Contains(strings.ToLower(string(out)), "dark")
}

// darwinIsDark queries the macOS defaults system for the AppleInterfaceStyle.
func darwinIsDark() bool {
	cmd := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle")
	out, err := cmd.Output()
	if err != nil {
		// When the key is absent, Light mode is active.
		return false
	}
	return strings.TrimSpace(strings.ToLower(string(out))) == "dark"
}
