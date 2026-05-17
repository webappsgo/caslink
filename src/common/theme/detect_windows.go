//go:build windows

package theme

import (
	"golang.org/x/sys/windows/registry"
)

// IsSystemDarkTheme returns true when the OS reports a dark color scheme.
func IsSystemDarkTheme() bool {
	return windowsIsDark()
}

// windowsIsDark reads the Windows registry to determine the current color scheme.
func windowsIsDark() bool {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return true
	}
	defer key.Close()

	val, _, err := key.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return true
	}
	// 0 = dark, 1 = light
	return val == 0
}
