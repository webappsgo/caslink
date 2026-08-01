package terminal

import "testing"

// TestSizeModeBoundaries exercises sizeMode at every documented boundary
// (<40 micro, 40-59 minimal, 60-79 compact, >=80 full) including the exact
// edge columns where classification flips.
func TestSizeModeBoundaries(t *testing.T) {
	tests := []struct {
		name string
		cols int
		want SizeMode
	}{
		{"zero columns", 0, SizeModeMicro},
		{"one column", 1, SizeModeMicro},
		{"39 cols still micro", 39, SizeModeMicro},
		{"40 cols starts minimal", 40, SizeModeMinimal},
		{"59 cols still minimal", 59, SizeModeMinimal},
		{"60 cols starts compact", 60, SizeModeCompact},
		{"79 cols still compact", 79, SizeModeCompact},
		{"80 cols starts full", 80, SizeModeFull},
		{"very wide terminal", 400, SizeModeFull},
		{"negative columns treated as micro", -1, SizeModeMicro},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sizeMode(tc.cols); got != tc.want {
				t.Errorf("sizeMode(%d) = %v, want %v", tc.cols, got, tc.want)
			}
		})
	}
}

// TestTerminalSizePredicates verifies the four Is* predicates are mutually
// exclusive and match the Mode they were constructed with — a bug here would
// make a caller's width branching silently wrong.
func TestTerminalSizePredicates(t *testing.T) {
	tests := []struct {
		mode        SizeMode
		wantFull    bool
		wantCompact bool
		wantMinimal bool
		wantMicro   bool
	}{
		{SizeModeFull, true, false, false, false},
		{SizeModeCompact, false, true, false, false},
		{SizeModeMinimal, false, false, true, false},
		{SizeModeMicro, false, false, false, true},
	}
	for _, tc := range tests {
		ts := TerminalSize{Mode: tc.mode}
		if got := ts.IsFull(); got != tc.wantFull {
			t.Errorf("mode %v: IsFull() = %v, want %v", tc.mode, got, tc.wantFull)
		}
		if got := ts.IsCompact(); got != tc.wantCompact {
			t.Errorf("mode %v: IsCompact() = %v, want %v", tc.mode, got, tc.wantCompact)
		}
		if got := ts.IsMinimal(); got != tc.wantMinimal {
			t.Errorf("mode %v: IsMinimal() = %v, want %v", tc.mode, got, tc.wantMinimal)
		}
		if got := ts.IsMicro(); got != tc.wantMicro {
			t.Errorf("mode %v: IsMicro() = %v, want %v", tc.mode, got, tc.wantMicro)
		}
	}
}

// TestGetTerminalSizeFallsBackWhenNotATerminal verifies the documented
// fallback (80x24, SizeModeFull) applies when stdout is not a TTY, which is
// always true under `go test`.
func TestGetTerminalSizeFallsBackWhenNotATerminal(t *testing.T) {
	size := GetTerminalSize()
	if size.Cols != 80 || size.Rows != 24 {
		t.Fatalf("GetTerminalSize() under non-TTY = %+v, want 80x24 fallback", size)
	}
	if !size.IsFull() {
		t.Fatalf("GetTerminalSize() fallback mode = %v, want full (80 cols)", size.Mode)
	}
}
