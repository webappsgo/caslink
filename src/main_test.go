package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBranchFile verifies the update-channel marker file path is
// config-dir-relative.
func TestBranchFile(t *testing.T) {
	got := branchFile("/etc/webappsgo/caslink")
	want := filepath.Join("/etc/webappsgo/caslink", "update_branch")
	if got != want {
		t.Errorf("branchFile() = %q, want %q", got, want)
	}
}

// TestCurrentBranchMissingFileDefaultsStable verifies a config dir with no
// persisted branch selection falls back to "stable".
func TestCurrentBranchMissingFileDefaultsStable(t *testing.T) {
	dir := t.TempDir()
	if got := currentBranch(dir); got != "stable" {
		t.Errorf("currentBranch() = %q, want stable", got)
	}
}

// TestCurrentBranchValidValues verifies each of the three valid channel
// names round-trips through saveBranch/currentBranch.
func TestCurrentBranchValidValues(t *testing.T) {
	for _, branch := range []string{"stable", "beta", "daily"} {
		t.Run(branch, func(t *testing.T) {
			dir := t.TempDir()
			if err := saveBranch(dir, branch); err != nil {
				t.Fatalf("saveBranch() error: %v", err)
			}
			if got := currentBranch(dir); got != branch {
				t.Errorf("currentBranch() = %q, want %q", got, branch)
			}
		})
	}
}

// TestCurrentBranchInvalidValueFallsBackToStable verifies a corrupted or
// hand-edited marker file with an unrecognized value is not trusted and
// falls back to "stable" rather than propagating garbage.
func TestCurrentBranchInvalidValueFallsBackToStable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(branchFile(dir), []byte("nonsense\n"), 0o644); err != nil {
		t.Fatalf("failed to seed branch file: %v", err)
	}
	if got := currentBranch(dir); got != "stable" {
		t.Errorf("currentBranch() = %q, want stable for invalid persisted value", got)
	}
}

// TestCurrentBranchTrimsWhitespace verifies surrounding whitespace/newlines
// in the persisted file do not prevent a match.
func TestCurrentBranchTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(branchFile(dir), []byte("  beta  \n"), 0o644); err != nil {
		t.Fatalf("failed to seed branch file: %v", err)
	}
	if got := currentBranch(dir); got != "beta" {
		t.Errorf("currentBranch() = %q, want beta", got)
	}
}

// TestSaveBranchCreatesConfigDir verifies saveBranch creates a missing
// config directory rather than failing.
func TestSaveBranchCreatesConfigDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "config")
	if err := saveBranch(dir, "daily"); err != nil {
		t.Fatalf("saveBranch() error: %v", err)
	}
	data, err := os.ReadFile(branchFile(dir))
	if err != nil {
		t.Fatalf("failed to read persisted branch file: %v", err)
	}
	if got := string(data); got != "daily\n" {
		t.Errorf("persisted content = %q, want %q", got, "daily\n")
	}
}
