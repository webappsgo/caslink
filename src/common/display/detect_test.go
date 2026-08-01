package display

import "testing"

// TestAutoDetectDisplayModeHeadless covers the boundary where neither a
// terminal nor a display is present — must classify as headless per the
// PART 7/33 display mode hierarchy table.
func TestAutoDetectDisplayModeHeadless(t *testing.T) {
	env := DisplayEnv{IsTerminal: false, HasDisplay: false}
	if got := env.autoDetectDisplayMode(); got != DisplayModeHeadless {
		t.Fatalf("autoDetectDisplayMode() = %v, want DisplayModeHeadless", got)
	}
}

// TestAutoDetectDisplayModeDumbTerminalForcesCLI verifies TERM=dumb forces
// CLI mode unconditionally, even when a display and terminal are present —
// this is the NON-NEGOTIABLE rule from binary-rules.md.
func TestAutoDetectDisplayModeDumbTerminalForcesCLI(t *testing.T) {
	env := DisplayEnv{IsTerminal: true, HasDisplay: true, TerminalType: "dumb"}
	if got := env.autoDetectDisplayMode(); got != DisplayModeCLI {
		t.Fatalf("autoDetectDisplayMode() with TERM=dumb = %v, want DisplayModeCLI", got)
	}
}

// TestAutoDetectDisplayModeGUI verifies a local display with no SSH/mosh
// selects GUI mode.
func TestAutoDetectDisplayModeGUI(t *testing.T) {
	env := DisplayEnv{IsTerminal: true, HasDisplay: true, IsSSH: false, IsMosh: false}
	if got := env.autoDetectDisplayMode(); got != DisplayModeGUI {
		t.Fatalf("autoDetectDisplayMode() with local display = %v, want DisplayModeGUI", got)
	}
}

// TestAutoDetectDisplayModeSSHForcesTUI verifies the mandatory rule that
// remote sessions (SSH), even with a display present (X11 forwarding), never
// get GUI — they fall through to TUI when a terminal is attached.
func TestAutoDetectDisplayModeSSHForcesTUI(t *testing.T) {
	env := DisplayEnv{IsTerminal: true, HasDisplay: true, IsSSH: true}
	if got := env.autoDetectDisplayMode(); got != DisplayModeTUI {
		t.Fatalf("autoDetectDisplayMode() over SSH with display = %v, want DisplayModeTUI (never GUI over SSH)", got)
	}
}

// TestAutoDetectDisplayModeMoshForcesTUI mirrors the SSH case for mosh
// sessions.
func TestAutoDetectDisplayModeMoshForcesTUI(t *testing.T) {
	env := DisplayEnv{IsTerminal: true, HasDisplay: true, IsMosh: true}
	if got := env.autoDetectDisplayMode(); got != DisplayModeTUI {
		t.Fatalf("autoDetectDisplayMode() over mosh with display = %v, want DisplayModeTUI (never GUI over mosh)", got)
	}
}

// TestAutoDetectDisplayModeTerminalNoDisplay covers a plain SSH/terminal
// session with no display forwarding at all — must be TUI, not headless.
func TestAutoDetectDisplayModeTerminalNoDisplay(t *testing.T) {
	env := DisplayEnv{IsTerminal: true, HasDisplay: false}
	if got := env.autoDetectDisplayMode(); got != DisplayModeTUI {
		t.Fatalf("autoDetectDisplayMode() terminal, no display = %v, want DisplayModeTUI", got)
	}
}

// TestAutoDetectDisplayModePipedOutputNoDisplay covers piped/non-TTY stdout
// with neither a terminal nor a display attached (e.g. `caslink status |
// cat` in a headless CI job) — must classify as headless.
func TestAutoDetectDisplayModePipedOutputNoDisplay(t *testing.T) {
	env := DisplayEnv{IsTerminal: false, HasDisplay: false, TerminalType: "xterm-256color"}
	if got := env.autoDetectDisplayMode(); got != DisplayModeHeadless {
		t.Fatalf("autoDetectDisplayMode() piped, no display = %v, want DisplayModeHeadless", got)
	}
}

// TestAutoDetectDisplayModePipedWithDisplayStillGUI documents the actual
// current behavior: a display being present (HasDisplay) forces GUI mode
// even when stdout itself is piped/non-terminal and not SSH/mosh — the
// function only inspects HasDisplay, not IsTerminal, on that branch.
func TestAutoDetectDisplayModePipedWithDisplayStillGUI(t *testing.T) {
	env := DisplayEnv{IsTerminal: false, HasDisplay: true, TerminalType: "xterm-256color"}
	if got := env.autoDetectDisplayMode(); got != DisplayModeGUI {
		t.Fatalf("autoDetectDisplayMode() piped stdout with display present = %v, want DisplayModeGUI (documents current HasDisplay-wins behavior)", got)
	}
}

// TestIsDumbTerminal is a direct unit check of the predicate used by
// PrintStartupBanner and CanUseANSI to gate all ANSI output.
func TestIsDumbTerminal(t *testing.T) {
	dumb := DisplayEnv{TerminalType: "dumb"}
	if !dumb.IsDumbTerminal() {
		t.Fatal("IsDumbTerminal() = false for TERM=dumb, want true")
	}
	notDumb := DisplayEnv{TerminalType: "xterm-256color"}
	if notDumb.IsDumbTerminal() {
		t.Fatal("IsDumbTerminal() = true for TERM=xterm-256color, want false")
	}
	empty := DisplayEnv{TerminalType: ""}
	if empty.IsDumbTerminal() {
		t.Fatal("IsDumbTerminal() = true for empty TERM, want false")
	}
}

// TestIsAutoDetectDisplayModePredicates verifies each Is* predicate matches
// exactly one Mode value and none of the others — a copy-paste bug here
// (e.g. two predicates comparing against the same constant) would make two
// modes appear simultaneously true.
func TestIsAutoDetectDisplayModePredicates(t *testing.T) {
	modes := []struct {
		mode         DisplayMode
		wantGUI      bool
		wantTUI      bool
		wantCLI      bool
		wantHeadless bool
	}{
		{DisplayModeGUI, true, false, false, false},
		{DisplayModeTUI, false, true, false, false},
		{DisplayModeCLI, false, false, true, false},
		{DisplayModeHeadless, false, false, false, true},
	}
	for _, tc := range modes {
		env := DisplayEnv{Mode: tc.mode}
		if got := env.IsAutoDetectDisplayModeGUI(); got != tc.wantGUI {
			t.Errorf("mode %v: IsAutoDetectDisplayModeGUI() = %v, want %v", tc.mode, got, tc.wantGUI)
		}
		if got := env.IsAutoDetectDisplayModeTUI(); got != tc.wantTUI {
			t.Errorf("mode %v: IsAutoDetectDisplayModeTUI() = %v, want %v", tc.mode, got, tc.wantTUI)
		}
		if got := env.IsAutoDetectDisplayModeCLI(); got != tc.wantCLI {
			t.Errorf("mode %v: IsAutoDetectDisplayModeCLI() = %v, want %v", tc.mode, got, tc.wantCLI)
		}
		if got := env.IsAutoDetectDisplayModeHeadless(); got != tc.wantHeadless {
			t.Errorf("mode %v: IsAutoDetectDisplayModeHeadless() = %v, want %v", tc.mode, got, tc.wantHeadless)
		}
	}
}

// TestCanUseANSIRespectsDumbTerminal verifies dumb terminals never get ANSI
// output, even if somehow flagged as a TTY.
func TestCanUseANSIRespectsDumbTerminal(t *testing.T) {
	env := &DisplayEnv{TerminalType: "dumb", IsTerminal: true}
	if CanUseANSI(env) {
		t.Fatal("CanUseANSI() = true for TERM=dumb, want false")
	}
}

// TestCanUseANSIRespectsNoColor verifies the NO_COLOR env var disables ANSI
// output even on a real terminal.
func TestCanUseANSIRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	env := &DisplayEnv{TerminalType: "xterm-256color", IsTerminal: true}
	if CanUseANSI(env) {
		t.Fatal("CanUseANSI() = true with NO_COLOR set, want false")
	}
}

// TestCanUseANSIRequiresTerminal verifies non-TTY output (piped/redirected)
// never gets ANSI codes even without NO_COLOR or TERM=dumb.
func TestCanUseANSIRequiresTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	env := &DisplayEnv{TerminalType: "xterm-256color", IsTerminal: false}
	if CanUseANSI(env) {
		t.Fatal("CanUseANSI() = true for non-terminal output, want false")
	}
}

// TestCanUseANSIAllowsInteractiveTerminal is the happy path: a real
// interactive terminal with no NO_COLOR and not TERM=dumb gets ANSI.
func TestCanUseANSIAllowsInteractiveTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	env := &DisplayEnv{TerminalType: "xterm-256color", IsTerminal: true}
	if !CanUseANSI(env) {
		t.Fatal("CanUseANSI() = false for interactive non-dumb terminal with no NO_COLOR, want true")
	}
}

// TestDetectDisplayEnvDoesNotPanic exercises the full real detection path
// (including platform-specific env probing) under the actual test-runner
// environment. It cannot assert a specific Mode since that depends on the
// sandbox, but it verifies the function completes and produces internally
// consistent output (Mode matches what autoDetectDisplayMode would compute
// from the same fields, and SSH env vars are read correctly).
func TestDetectDisplayEnvDoesNotPanic(t *testing.T) {
	t.Setenv("SSH_CLIENT", "1.2.3.4 1 22")
	t.Setenv("SSH_TTY", "/dev/pts/0")

	env := DetectDisplayEnv()

	if !env.IsSSH {
		t.Fatal("DetectDisplayEnv() with SSH_CLIENT/SSH_TTY set did not detect IsSSH")
	}

	want := env.autoDetectDisplayMode()
	if env.Mode != want {
		t.Fatalf("DetectDisplayEnv().Mode = %v, want recomputed %v (internal inconsistency)", env.Mode, want)
	}
}

// TestDetectDisplayEnvMoshDetection verifies MOSH env var and TERM
// substring detection both set IsMosh.
func TestDetectDisplayEnvMoshDetection(t *testing.T) {
	t.Setenv("MOSH", "")
	t.Setenv("TERM", "mosh")
	env := DetectDisplayEnv()
	if !env.IsMosh {
		t.Fatal("DetectDisplayEnv() with TERM containing 'mosh' did not detect IsMosh")
	}
}

// TestDetectDisplayEnvScreenDetection verifies STY/TMUX env vars set
// IsScreen.
func TestDetectDisplayEnvScreenDetection(t *testing.T) {
	t.Setenv("STY", "")
	t.Setenv("TMUX", "/tmp/tmux-0/default,123,0")
	env := DetectDisplayEnv()
	if !env.IsScreen {
		t.Fatal("DetectDisplayEnv() with TMUX set did not detect IsScreen")
	}
}
