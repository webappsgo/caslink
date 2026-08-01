package main

import (
	"os"
	"reflect"
	"testing"
)

// TestExtractPasswordFlagSpaceForm verifies "--password <value>" is
// extracted and removed from the returned positional args.
func TestExtractPasswordFlagSpaceForm(t *testing.T) {
	pw, rest := extractPasswordFlag([]string{"backup.tar.gz.enc", "--password", "hunter2"})
	if pw != "hunter2" {
		t.Errorf("password = %q, want hunter2", pw)
	}
	if !reflect.DeepEqual(rest, []string{"backup.tar.gz.enc"}) {
		t.Errorf("rest = %v, want [backup.tar.gz.enc]", rest)
	}
}

// TestExtractPasswordFlagEqualsForm verifies "--password=<value>" is
// extracted and removed from the returned positional args.
func TestExtractPasswordFlagEqualsForm(t *testing.T) {
	pw, rest := extractPasswordFlag([]string{"--password=hunter2", "backup.tar.gz.enc"})
	if pw != "hunter2" {
		t.Errorf("password = %q, want hunter2", pw)
	}
	if !reflect.DeepEqual(rest, []string{"backup.tar.gz.enc"}) {
		t.Errorf("rest = %v, want [backup.tar.gz.enc]", rest)
	}
}

// TestExtractPasswordFlagAbsent verifies no --password flag leaves the
// password empty and all args passed through as rest, in order.
func TestExtractPasswordFlagAbsent(t *testing.T) {
	pw, rest := extractPasswordFlag([]string{"backup.tar.gz.enc", "extra"})
	if pw != "" {
		t.Errorf("password = %q, want empty", pw)
	}
	if !reflect.DeepEqual(rest, []string{"backup.tar.gz.enc", "extra"}) {
		t.Errorf("rest = %v, want [backup.tar.gz.enc extra]", rest)
	}
}

// TestExtractPasswordFlagTrailingWithoutValue verifies a trailing
// "--password" with no following value does not panic and yields an empty
// password with no positional entry for the flag itself.
func TestExtractPasswordFlagTrailingWithoutValue(t *testing.T) {
	pw, rest := extractPasswordFlag([]string{"backup.tar.gz.enc", "--password"})
	if pw != "" {
		t.Errorf("password = %q, want empty", pw)
	}
	if !reflect.DeepEqual(rest, []string{"backup.tar.gz.enc"}) {
		t.Errorf("rest = %v, want [backup.tar.gz.enc]", rest)
	}
}

// TestExtractPasswordFlagEmpty verifies an empty args slice returns empty
// password and nil rest.
func TestExtractPasswordFlagEmpty(t *testing.T) {
	pw, rest := extractPasswordFlag(nil)
	if pw != "" {
		t.Errorf("password = %q, want empty", pw)
	}
	if rest != nil {
		t.Errorf("rest = %v, want nil", rest)
	}
}

// TestPromptBackupPasswordNonTerminalReadsFullLine verifies that when stdin
// is not a terminal (e.g. a redirected pipe/file, as in this test), the
// fallback path reads a full line — including embedded spaces — rather than
// truncating at the first whitespace, and strips the trailing newline.
func TestPromptBackupPasswordNonTerminalReadsFullLine(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	defer r.Close()

	if _, err := w.WriteString("correct horse battery staple\n"); err != nil {
		t.Fatalf("write to pipe failed: %v", err)
	}
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	got := promptBackupPassword("Password: ")
	if got != "correct horse battery staple" {
		t.Errorf("promptBackupPassword() = %q, want %q", got, "correct horse battery staple")
	}
}
