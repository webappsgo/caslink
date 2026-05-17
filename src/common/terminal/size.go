// Package terminal provides terminal size detection and related helpers.
package terminal

import (
	"os"

	"golang.org/x/term"
)

// SizeMode classifies the terminal by its width.
type SizeMode int

const (
	// SizeModeMicro is less than 40 columns.
	SizeModeMicro SizeMode = iota
	// SizeModeMinimal is 40–59 columns.
	SizeModeMinimal
	// SizeModeCompact is 60–79 columns.
	SizeModeCompact
	// SizeModeFull is 80 or more columns.
	SizeModeFull
)

// TerminalSize holds the current terminal dimensions.
type TerminalSize struct {
	Cols int
	Rows int
	Mode SizeMode
}

// GetTerminalSize returns the current terminal size.
// When stdout is not a terminal, sensible defaults (80×24) are returned.
func GetTerminalSize() TerminalSize {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols == 0 {
		cols = 80
		rows = 24
	}
	return TerminalSize{
		Cols: cols,
		Rows: rows,
		Mode: sizeMode(cols),
	}
}

func sizeMode(cols int) SizeMode {
	switch {
	case cols < 40:
		return SizeModeMicro
	case cols < 60:
		return SizeModeMinimal
	case cols < 80:
		return SizeModeCompact
	default:
		return SizeModeFull
	}
}

// IsFull returns true when the terminal is 80 columns or wider.
func (t TerminalSize) IsFull() bool { return t.Mode == SizeModeFull }

// IsCompact returns true when the terminal is 60–79 columns wide.
func (t TerminalSize) IsCompact() bool { return t.Mode == SizeModeCompact }

// IsMinimal returns true when the terminal is 40–59 columns wide.
func (t TerminalSize) IsMinimal() bool { return t.Mode == SizeModeMinimal }

// IsMicro returns true when the terminal is less than 40 columns wide.
func (t TerminalSize) IsMicro() bool { return t.Mode == SizeModeMicro }
