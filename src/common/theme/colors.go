// Package theme provides color palettes for caslink TUI and CLI output.
package theme

import "os"

// ThemePalette holds the ANSI color codes for a single theme variant.
type ThemePalette struct {
	// Primary colors
	Primary   string
	Secondary string
	Accent    string

	// Status colors
	Success string
	Warning string
	Error   string
	Info    string

	// Text colors
	Text      string
	TextMuted string
	TextBold  string

	// Background / surface colors
	Background string
	Surface    string
	Border     string

	// Interactive element colors
	Highlight  string
	Selected   string
	Cursor     string

	// Reset sequence
	Reset string
}

// ThemePaletteDark is the default dark-mode palette.
var ThemePaletteDark = ThemePalette{
	Primary:    "\033[38;5;75m",
	Secondary:  "\033[38;5;111m",
	Accent:     "\033[38;5;213m",
	Success:    "\033[38;5;82m",
	Warning:    "\033[38;5;214m",
	Error:      "\033[38;5;196m",
	Info:       "\033[38;5;45m",
	Text:       "\033[38;5;252m",
	TextMuted:  "\033[38;5;244m",
	TextBold:   "\033[1;38;5;255m",
	Background: "\033[48;5;235m",
	Surface:    "\033[48;5;237m",
	Border:     "\033[38;5;240m",
	Highlight:  "\033[48;5;24m",
	Selected:   "\033[48;5;25m\033[38;5;255m",
	Cursor:     "\033[48;5;75m\033[38;5;235m",
	Reset:      "\033[0m",
}

// ThemePaletteLight is the light-mode palette.
var ThemePaletteLight = ThemePalette{
	Primary:    "\033[38;5;26m",
	Secondary:  "\033[38;5;33m",
	Accent:     "\033[38;5;165m",
	Success:    "\033[38;5;28m",
	Warning:    "\033[38;5;130m",
	Error:      "\033[38;5;160m",
	Info:       "\033[38;5;31m",
	Text:       "\033[38;5;235m",
	TextMuted:  "\033[38;5;244m",
	TextBold:   "\033[1;38;5;232m",
	Background: "\033[48;5;255m",
	Surface:    "\033[48;5;253m",
	Border:     "\033[38;5;248m",
	Highlight:  "\033[48;5;153m",
	Selected:   "\033[48;5;111m\033[38;5;232m",
	Cursor:     "\033[48;5;26m\033[38;5;255m",
	Reset:      "\033[0m",
}

// ThemePaletteNone is a no-color palette for dumb terminals / NO_COLOR.
var ThemePaletteNone = ThemePalette{
	Reset: "",
}

// GetThemePalette returns the appropriate palette based on the environment.
// When NO_COLOR is set or the terminal is dumb, ThemePaletteNone is returned.
// Otherwise dark or light is chosen by IsSystemDarkTheme.
func GetThemePalette(color string) *ThemePalette {
	// Respect NO_COLOR env
	if os.Getenv("NO_COLOR") != "" {
		return &ThemePaletteNone
	}

	switch color {
	case "off":
		return &ThemePaletteNone
	case "light":
		return &ThemePaletteLight
	case "dark":
		return &ThemePaletteDark
	default:
		// "on" or "auto" — detect from system
		if IsSystemDarkTheme() {
			return &ThemePaletteDark
		}
		return &ThemePaletteLight
	}
}
