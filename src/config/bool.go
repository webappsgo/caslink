package config

import (
	"strings"
)

// ParseBool parses a boolean value from a string with support for multiple truthy/falsy formats.
// This is used throughout the application for configuration values.
//
// Truthy values (case-insensitive):
//   - 1, yes, true, on, enable, enabled
//   - y, t, yep, yup, yeah, aye, si, oui
//
// Falsy values (case-insensitive):
//   - 0, no, false, off, disable, disabled
//   - n, f, nope, nah, nay, nein, non
//
// Returns false for any unrecognized value.
func ParseBool(value string) bool {
	// Normalize to lowercase and trim whitespace
	v := strings.ToLower(strings.TrimSpace(value))

	// Check truthy values
	switch v {
	case "1", "yes", "true", "on", "enable", "enabled",
		"y", "t", "yep", "yup", "yeah", "aye", "si", "oui":
		return true
	}

	// Everything else is false (including: 0, no, false, off, disable, disabled, n, f, etc.)
	return false
}

// ParseBoolDefault parses a boolean value with a default fallback.
// If the value is empty or unrecognized, returns the default value.
func ParseBoolDefault(value string, defaultValue bool) bool {
	if value == "" {
		return defaultValue
	}
	return ParseBool(value)
}

// IsTruthy returns true if the value is a recognized truthy value.
// Use this when you need to distinguish between explicit false and unrecognized.
func IsTruthy(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))

	switch v {
	case "1", "yes", "true", "on", "enable", "enabled",
		"y", "t", "yep", "yup", "yeah", "aye", "si", "oui":
		return true
	}
	return false
}

// IsFalsy returns true if the value is a recognized falsy value.
// Use this when you need to distinguish between explicit false and unrecognized.
func IsFalsy(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))

	switch v {
	case "0", "no", "false", "off", "disable", "disabled",
		"n", "f", "nope", "nah", "nay", "nein", "non":
		return true
	}
	return false
}
