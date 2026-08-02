//go:build !windows

package updater

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReplaceBinary_Success verifies the happy path: the new binary
// content ends up at currentPath, with currentPath's original file mode
// preserved (per PART 7's "restore permissions after replace" rule) and
// the temp file's own path no longer usable as the new binary (it was
// renamed away). Both paths are ordinary temp files, never the real
// running test binary, so this is safe to exercise directly.
func TestReplaceBinary_Success(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "current-binary")
	newBinaryPath := filepath.Join(dir, "new-binary")

	if err := os.WriteFile(currentPath, []byte("old contents"), 0o750); err != nil {
		t.Fatalf("seed current binary: %v", err)
	}
	if err := os.WriteFile(newBinaryPath, []byte("new contents"), 0o640); err != nil {
		t.Fatalf("seed new binary: %v", err)
	}

	if err := replaceBinary(currentPath, newBinaryPath); err != nil {
		t.Fatalf("replaceBinary: %v", err)
	}

	got, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if string(got) != "new contents" {
		t.Errorf("currentPath content = %q, want %q", got, "new contents")
	}

	info, err := os.Stat(currentPath)
	if err != nil {
		t.Fatalf("stat replaced binary: %v", err)
	}
	if info.Mode().Perm() != 0o750 {
		t.Errorf("currentPath mode = %v, want the original 0750 preserved", info.Mode().Perm())
	}

	if _, err := os.Stat(newBinaryPath); !os.IsNotExist(err) {
		t.Errorf("expected newBinaryPath to be gone after rename, stat err = %v", err)
	}
}

// TestReplaceBinary_MissingCurrentPathErrors verifies that replaceBinary
// fails (rather than silently installing the update with no permission
// reference) when the "current" binary it needs to Stat for its mode
// doesn't exist.
func TestReplaceBinary_MissingCurrentPathErrors(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "does-not-exist")
	newBinaryPath := filepath.Join(dir, "new-binary")

	if err := os.WriteFile(newBinaryPath, []byte("new contents"), 0o755); err != nil {
		t.Fatalf("seed new binary: %v", err)
	}

	err := replaceBinary(currentPath, newBinaryPath)
	if err == nil {
		t.Fatal("expected error when current binary path does not exist")
	}

	// The new binary must be left untouched since the operation never
	// reached the rename step.
	if _, statErr := os.Stat(newBinaryPath); statErr != nil {
		t.Errorf("expected newBinaryPath to remain in place after failure, stat err = %v", statErr)
	}
}

// TestReplaceBinary_MissingNewBinaryErrors verifies that replaceBinary
// fails when the downloaded replacement file is missing, instead of
// leaving currentPath in a half-updated state.
func TestReplaceBinary_MissingNewBinaryErrors(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "current-binary")
	newBinaryPath := filepath.Join(dir, "does-not-exist")

	if err := os.WriteFile(currentPath, []byte("old contents"), 0o755); err != nil {
		t.Fatalf("seed current binary: %v", err)
	}

	err := replaceBinary(currentPath, newBinaryPath)
	if err == nil {
		t.Fatal("expected error when new binary path does not exist")
	}

	got, readErr := os.ReadFile(currentPath)
	if readErr != nil {
		t.Fatalf("read current binary after failed replace: %v", readErr)
	}
	if string(got) != "old contents" {
		t.Errorf("currentPath content changed despite failed rename: %q", got)
	}
}
