package graphql

// Theme constants for GraphiQL
const (
	ThemeDark  = "dark"
	ThemeLight = "light"
	ThemeAuto  = "auto"
)

// GetTheme returns the appropriate theme based on request or config
func GetTheme(requestedTheme string, configTheme string) string {
	// Priority: request > config > default (dark)
	if requestedTheme != "" {
		switch requestedTheme {
		case ThemeDark, ThemeLight, ThemeAuto:
			return requestedTheme
		}
	}

	if configTheme != "" {
		switch configTheme {
		case ThemeDark, ThemeLight, ThemeAuto:
			return configTheme
		}
	}

	// Default to dark theme
	return ThemeDark
}
