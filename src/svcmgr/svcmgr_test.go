package svcmgr

import (
	"os"
	"strings"
	"testing"
)

// TestNew_PopulatesExpectedFields verifies the fixed identity fields caslink
// relies on (service name, display name) and that BinaryPath resolves to a
// real, absolute path from os.Executable().
func TestNew_PopulatesExpectedFields(t *testing.T) {
	m := New()

	if m.ServiceName != "caslink" {
		t.Errorf("ServiceName = %q, want %q", m.ServiceName, "caslink")
	}
	if m.DisplayName != "Caslink URL Shortener" {
		t.Errorf("DisplayName = %q, want %q", m.DisplayName, "Caslink URL Shortener")
	}
	if m.Description == "" {
		t.Error("Description is empty")
	}
	if m.BinaryPath == "" {
		t.Error("BinaryPath is empty")
	}
	exe, err := os.Executable()
	if err == nil && m.BinaryPath != exe {
		t.Errorf("BinaryPath = %q, want %q (from os.Executable())", m.BinaryPath, exe)
	}
}

// TestStatus_DelegatesToCheckStatus proves Status() returns whatever the
// platform's checkStatus reports for the Manager's own ServiceName, without
// mutating any real service state.
func TestStatus_DelegatesToCheckStatus(t *testing.T) {
	m := New()
	got := m.Status()
	want := checkStatus(m.ServiceName)
	if got != want {
		t.Errorf("Status() = %q, want %q (checkStatus(%q))", got, want, m.ServiceName)
	}
}

// TestPrintHelp_OutputContainsExpectedSections captures stdout and verifies
// PrintHelp emits the documented subcommands and current-status block,
// including the actual (possibly renamed) binary path per AI.md binary-rules.
func TestPrintHelp_OutputContainsExpectedSections(t *testing.T) {
	m := &Manager{
		BinaryPath:  "/usr/local/bin/caslink",
		ServiceName: "caslink",
		DisplayName: "Caslink URL Shortener",
		Description: "Self-hosted URL shortener service",
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	m.PrintHelp()
	_ = w.Close()
	os.Stdout = origStdout

	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	for _, want := range []string{
		"start", "stop", "restart", "reload",
		"--install", "--disable", "--uninstall", "--help",
		"Current status:",
		"/usr/local/bin/caslink",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("PrintHelp() output missing %q; got:\n%s", want, out)
		}
	}
}

// TestManagerMethods_UnsupportedInitReturnError exercises Start/Stop/Restart/
// Disable/Reload against a service name that can never exist on any init
// system, so the calls are guaranteed side-effect-free — the underlying
// commands (systemctl/rc-service/sv) either aren't present (falls through to
// "unsupported init system") or reject the nonexistent unit outright, in
// either case returning a non-nil error regardless of the host's actual init
// system. Install/Uninstall/ensureServiceUser/escalateIfNeeded are
// intentionally NOT exercised here (see svcmgr_linux_test.go) because they
// require root and can mutate real system state (create users, write unit
// files, invoke sudo/pkexec).
func TestManagerMethods_UnsupportedInitReturnError(t *testing.T) {
	m := &Manager{ServiceName: "caslink-test-definitely-nonexistent-svc-zzz"}

	if err := m.Start(); err == nil {
		t.Error("Start() on a nonexistent service = nil error, want error")
	}
	if err := m.Stop(); err == nil {
		t.Error("Stop() on a nonexistent service = nil error, want error")
	}
	if err := m.Restart(); err == nil {
		t.Error("Restart() on a nonexistent service = nil error, want error")
	}
	if err := m.Disable(); err == nil {
		t.Error("Disable() on a nonexistent service = nil error, want error")
	}
	if err := m.Reload(); err == nil {
		t.Error("Reload() on a nonexistent service = nil error, want error")
	}
}
