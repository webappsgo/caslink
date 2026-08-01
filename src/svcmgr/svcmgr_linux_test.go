//go:build linux

package svcmgr

import (
	"os"
	"strings"
	"testing"
)

// TestDetectInitSystem_ReturnsKnownValue asserts detectInitSystem() always
// resolves to one of the four recognized init systems or "unknown" — the
// actual value depends on the host, which we do not control in CI/Docker,
// so we only assert the return stays within the documented enum.
func TestDetectInitSystem_ReturnsKnownValue(t *testing.T) {
	got := detectInitSystem()
	valid := map[string]bool{"systemd": true, "openrc": true, "runit": true, "sysvinit": true, "unknown": true}
	if !valid[got] {
		t.Errorf("detectInitSystem() = %q, want one of systemd/openrc/runit/sysvinit/unknown", got)
	}
}

// TestCheckStatus_NonexistentServiceIsUnknown proves checkStatus never
// fabricates a status for a service that cannot exist: on every init-system
// branch, querying a bogus unit either errors (falls through to "unknown")
// or the branch itself isn't taken (falls through to "unknown"). This holds
// regardless of what init system the CI/Docker container actually has, so
// it's safe to assert unconditionally without mutating any real service.
func TestCheckStatus_NonexistentServiceIsUnknown(t *testing.T) {
	got := checkStatus("caslink-test-definitely-nonexistent-svc-zzz")
	if got != "unknown" {
		t.Errorf("checkStatus(bogus) = %q, want %q", got, "unknown")
	}
}

// TestStartStopRestartDisableReload_NonexistentServiceErrors exercises the
// unexported per-verb helpers directly (white-box, same package) using a
// service name that cannot exist on any init system. Whether the host has
// systemd/openrc/runit/sysvinit or none at all, this always ends in an
// error: either "unsupported init system" (no init system found) or the
// real service manager rejecting an unknown unit. No real service is
// touched either way.
func TestStartStopRestartDisableReload_NonexistentServiceErrors(t *testing.T) {
	const bogus = "caslink-test-definitely-nonexistent-svc-zzz"

	if err := startSvc(bogus); err == nil {
		t.Error("startSvc(bogus) = nil error, want error")
	}
	if err := stopSvc(bogus); err == nil {
		t.Error("stopSvc(bogus) = nil error, want error")
	}
	if err := restartSvc(bogus); err == nil {
		t.Error("restartSvc(bogus) = nil error, want error")
	}
	if err := disable(bogus); err == nil {
		t.Error("disable(bogus) = nil error, want error")
	}
	if err := reloadSvc(bogus); err == nil {
		t.Error("reloadSvc(bogus) = nil error, want error")
	}
}

// TestUninstall_AbortsOnDeclinedConfirmation feeds "n" to the interactive
// [y/N] prompt via a redirected stdin pipe and verifies uninstall() returns
// immediately without touching any directory, PID file, or service state —
// the only uninstall() path safe to exercise without root/a real init
// system. The "y" (destructive) path is intentionally never tested here: it
// would delete real directories, remove a systemd unit, and require root.
func TestUninstall_AbortsOnDeclinedConfirmation(t *testing.T) {
	dir := t.TempDir()
	marker := dir + "/should-not-be-removed"
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed marker file: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.Write([]byte("n\n"))
		_ = w.Close()
	}()

	// Also capture and discard stdout so the prompt text doesn't pollute
	// test output.
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = outW
	defer func() { os.Stdout = origStdout }()

	m := &Manager{ServiceName: "caslink-test-definitely-nonexistent-svc-zzz"}
	uerr := m.Uninstall(dir, "", "", "", "", "")
	_ = outW.Close()
	buf := make([]byte, 4096)
	n, _ := outR.Read(buf)
	out := string(buf[:n])

	if uerr != nil {
		t.Errorf("Uninstall() with declined confirmation = %v, want nil", uerr)
	}
	if !strings.Contains(out, "Aborted") {
		t.Errorf("Uninstall() output = %q, want it to contain %q", out, "Aborted")
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("marker file was removed despite declined confirmation: %v", statErr)
	}
}
