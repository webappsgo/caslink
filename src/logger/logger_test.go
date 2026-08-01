package logger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readFile reads the entire contents of a file, failing the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// TestNewCreatesRequiredFilesProductionMode verifies New() creates every
// required log file except debug.log when devMode is false, and that
// Close() releases the handles without error.
func TestNewCreatesRequiredFilesProductionMode(t *testing.T) {
	dir := t.TempDir()

	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New(%q, false) error = %v", dir, err)
	}
	defer l.Close()

	for _, name := range []string{"access.log", "server.log", "error.log", "audit.log", "security.log"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "debug.log")); err == nil {
		t.Error("debug.log should not be created in production mode")
	}

	if l.Debug == nil {
		t.Error("Debug logger should never be nil, even in production mode (discard writer)")
	}
}

// TestNewCreatesDebugLogInDevMode verifies debug.log is only created when
// devMode is true, and that the Debug logger is wired to it.
func TestNewCreatesDebugLogInDevMode(t *testing.T) {
	dir := t.TempDir()

	l, err := New(dir, true)
	if err != nil {
		t.Fatalf("New(%q, true) error = %v", dir, err)
	}
	defer l.Close()

	if _, err := os.Stat(filepath.Join(dir, "debug.log")); err != nil {
		t.Errorf("expected debug.log to exist in dev mode: %v", err)
	}

	l.Debug.Print("hello from debug")
	l.Close()

	got := readFile(t, filepath.Join(dir, "debug.log"))
	if !strings.Contains(got, "hello from debug") {
		t.Errorf("debug.log = %q, want it to contain the logged message", got)
	}
}

// TestNewCreatesMissingParentDirectory verifies New() creates the log
// directory (including any missing parents) rather than erroring.
func TestNewCreatesMissingParentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "log", "dir")

	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New(%q, false) error = %v", dir, err)
	}
	defer l.Close()

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected log dir to be created: %v", err)
	}
}

// TestAccessWritesApacheCommonLogFormat verifies Access() writes a single
// well-formed Apache common log format line to access.log.
func TestAccessWritesApacheCommonLogFormat(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	defer l.Close()

	l.Access("203.0.113.5", "GET", "/healthz", "HTTP/1.1", 200, 1234, 150*time.Millisecond)
	l.Close()

	got := readFile(t, filepath.Join(dir, "access.log"))
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 access line, got %d: %q", len(lines), got)
	}
	line := lines[0]

	if !strings.HasPrefix(line, "203.0.113.5 - - [") {
		t.Errorf("access line does not start with expected IP/dash prefix: %q", line)
	}
	if !strings.Contains(line, `"GET /healthz HTTP/1.1"`) {
		t.Errorf("access line missing request line: %q", line)
	}
	if !strings.Contains(line, " 200 1234 0.150") {
		t.Errorf("access line missing status/bytes/duration fields: %q", line)
	}
}

// TestAccessMultipleLinesAreAppended verifies successive Access() calls
// append rather than overwrite, and are newline-delimited.
func TestAccessMultipleLinesAreAppended(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	defer l.Close()

	l.Access("10.0.0.1", "GET", "/a", "HTTP/1.1", 200, 10, time.Millisecond)
	l.Access("10.0.0.2", "POST", "/b", "HTTP/1.1", 201, 20, 2*time.Millisecond)
	l.Close()

	got := readFile(t, filepath.Join(dir, "access.log"))
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 access lines, got %d: %q", len(lines), got)
	}
	if !strings.Contains(lines[0], "/a") || !strings.Contains(lines[1], "/b") {
		t.Errorf("access lines out of order or missing paths: %v", lines)
	}
}

// TestAuditWritesValidJSONLines verifies Audit() writes one valid JSON
// object per call, with the documented fields, and auto-fills Time when
// the caller left it blank.
func TestAuditWritesValidJSONLines(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	defer l.Close()

	l.Audit(AuditEvent{
		Action:    "login",
		Actor:     "alice",
		ActorType: "user",
		Resource:  "/server/auth/login",
		IP:        "203.0.113.9",
		Result:    "ok",
	})
	l.Close()

	got := readFile(t, filepath.Join(dir, "audit.log"))
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 audit line, got %d: %q", len(lines), got)
	}

	var ev AuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatalf("audit line is not valid JSON: %v (line=%q)", err, lines[0])
	}

	if ev.Action != "login" || ev.Actor != "alice" || ev.ActorType != "user" ||
		ev.Resource != "/server/auth/login" || ev.IP != "203.0.113.9" || ev.Result != "ok" {
		t.Errorf("audit event fields did not round-trip: %+v", ev)
	}
	if ev.Time == "" {
		t.Error("Time should be auto-filled when the caller leaves it blank")
	}
	if _, err := time.Parse(time.RFC3339, ev.Time); err != nil {
		t.Errorf("auto-filled Time is not RFC3339: %q (%v)", ev.Time, err)
	}
}

// TestAuditPreservesExplicitTime verifies Audit() does not overwrite a
// caller-supplied Time value.
func TestAuditPreservesExplicitTime(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	defer l.Close()

	explicit := "2020-01-01T00:00:00Z"
	l.Audit(AuditEvent{Time: explicit, Action: "test", Result: "ok"})
	l.Close()

	got := readFile(t, filepath.Join(dir, "audit.log"))
	var ev AuditEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &ev); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if ev.Time != explicit {
		t.Errorf("Time = %q, want unchanged %q", ev.Time, explicit)
	}
}

// TestAuditOmitsEmptyOptionalFields verifies the audit JSON leaves out
// zero-value optional fields (per the `omitempty` tags) rather than
// emitting empty strings, keeping audit lines compact.
func TestAuditOmitsEmptyOptionalFields(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	defer l.Close()

	l.Audit(AuditEvent{Action: "minimal", Result: "ok"})
	l.Close()

	got := readFile(t, filepath.Join(dir, "audit.log"))
	for _, field := range []string{"actor", "actor_type", "resource", "ip", "details"} {
		if strings.Contains(got, `"`+field+`"`) {
			t.Errorf("audit line should omit empty field %q, got %q", field, got)
		}
	}
}

// TestSecurityEventFormat verifies SecurityEvent() writes a
// fail2ban-compatible "TIMESTAMP [LEVEL] message" line.
func TestSecurityEventFormat(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	defer l.Close()

	l.SecurityEvent("WARN", "Failed login for user bob from 198.51.100.7")
	l.Close()

	got := readFile(t, filepath.Join(dir, "security.log"))
	line := strings.TrimRight(got, "\n")

	if !strings.Contains(line, "[WARN] Failed login for user bob from 198.51.100.7") {
		t.Errorf("security line missing level/message: %q", line)
	}

	// The timestamp is everything before " [WARN]"; verify it parses as RFC3339.
	idx := strings.Index(line, " [WARN]")
	if idx <= 0 {
		t.Fatalf("could not locate level marker in %q", line)
	}
	ts := line[:idx]
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("security event timestamp %q is not RFC3339: %v", ts, err)
	}
}

// TestServerAndErrorLoggersWriteToTheirFiles verifies the Server and Error
// stdlib loggers append their output to server.log and error.log
// respectively (in addition to stderr).
func TestServerAndErrorLoggersWriteToTheirFiles(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	defer l.Close()

	l.Server.Print("server started")
	l.Error.Print("something broke")
	l.Close()

	serverLog := readFile(t, filepath.Join(dir, "server.log"))
	if !strings.Contains(serverLog, "server started") {
		t.Errorf("server.log = %q, want it to contain the logged message", serverLog)
	}

	errorLog := readFile(t, filepath.Join(dir, "error.log"))
	if !strings.Contains(errorLog, "something broke") {
		t.Errorf("error.log = %q, want it to contain the logged message", errorLog)
	}
}

// TestZeroValueLoggerMethodsDoNotPanic verifies that calling the write
// methods on a Logger whose file handles were never opened (the zero
// value) is a safe no-op rather than a nil-pointer panic.
func TestZeroValueLoggerMethodsDoNotPanic(t *testing.T) {
	l := &Logger{}

	l.Access("1.2.3.4", "GET", "/", "HTTP/1.1", 200, 0, 0)
	l.Audit(AuditEvent{Action: "noop", Result: "ok"})
	l.SecurityEvent("INFO", "noop")
	l.Close()
}

// TestCloseIsIdempotentWithNilDebugFile verifies Close() tolerates the
// production-mode case where debugFile was never opened (nil).
func TestCloseIsIdempotentWithNilDebugFile(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	l.Close()
}

// TestAccessLineEndsWithNewline verifies each written access line is
// itself newline-terminated (line-oriented log format requirement).
func TestAccessLineEndsWithNewline(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir, false)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	l.Access("1.1.1.1", "GET", "/x", "HTTP/1.1", 204, 0, 0)
	l.Close()

	f, err := os.Open(filepath.Join(dir, "access.log"))
	if err != nil {
		t.Fatalf("open access.log: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	if count != 1 {
		t.Errorf("expected exactly 1 scanned line, got %d", count)
	}
}
