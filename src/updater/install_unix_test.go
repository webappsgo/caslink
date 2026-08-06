//go:build !windows

package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// withCurrentExecutable temporarily overrides the package-level
// currentExecutable seam and restores it when the test ends.
func withCurrentExecutable(t *testing.T, fn func() (string, error)) {
	t.Helper()
	orig := currentExecutable
	currentExecutable = fn
	t.Cleanup(func() { currentExecutable = orig })
}

// TestInstallUpdatedBinary_Success exercises the full post-download install
// path DoUpdateFor runs after checksum verification — chmod the downloaded
// file, resolve the running binary via the currentExecutable seam, resolve
// symlinks, and atomically replace it — without any network round trip. The
// "current" binary is a synthetic temp file, never the real test binary.
func TestInstallUpdatedBinary_Success(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "caslink")
	if err := os.WriteFile(current, []byte("old contents"), 0o755); err != nil {
		t.Fatalf("seed current binary: %v", err)
	}
	downloaded := filepath.Join(dir, "caslink-update-download")
	if err := os.WriteFile(downloaded, []byte("new binary contents"), 0o644); err != nil {
		t.Fatalf("seed downloaded binary: %v", err)
	}

	withCurrentExecutable(t, func() (string, error) { return current, nil })

	if err := installUpdatedBinary(downloaded); err != nil {
		t.Fatalf("installUpdatedBinary: %v", err)
	}

	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(got) != "new binary contents" {
		t.Errorf("installed content = %q, want %q", got, "new binary contents")
	}

	// The downloaded temp file was renamed over current, so its own path is gone.
	if _, err := os.Stat(downloaded); !os.IsNotExist(err) {
		t.Errorf("expected downloaded path to be gone after rename, stat err = %v", err)
	}

	// replaceBinary preserves the current binary's original mode (0755).
	info, err := os.Stat(current)
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("installed mode = %v, want original 0755 preserved", info.Mode().Perm())
	}
}

// TestInstallUpdatedBinary_ResolverErrorPropagates verifies that when the
// running binary's path cannot be resolved, the update aborts and the
// downloaded file is left untouched rather than installed blindly.
func TestInstallUpdatedBinary_ResolverErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	downloaded := filepath.Join(dir, "caslink-update-download")
	if err := os.WriteFile(downloaded, []byte("new binary contents"), 0o644); err != nil {
		t.Fatalf("seed downloaded binary: %v", err)
	}

	sentinel := errors.New("cannot resolve executable")
	withCurrentExecutable(t, func() (string, error) { return "", sentinel })

	err := installUpdatedBinary(downloaded)
	if err == nil {
		t.Fatal("expected error when currentExecutable fails")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want it to wrap the resolver error", err)
	}
	if _, statErr := os.Stat(downloaded); statErr != nil {
		t.Errorf("downloaded file should be untouched on resolver failure, stat err = %v", statErr)
	}
}

// TestInstallUpdatedBinary_MissingCurrentPathErrors verifies that a resolver
// pointing at a nonexistent path fails at symlink resolution rather than
// silently proceeding.
func TestInstallUpdatedBinary_MissingCurrentPathErrors(t *testing.T) {
	dir := t.TempDir()
	downloaded := filepath.Join(dir, "caslink-update-download")
	if err := os.WriteFile(downloaded, []byte("new binary contents"), 0o644); err != nil {
		t.Fatalf("seed downloaded binary: %v", err)
	}
	missing := filepath.Join(dir, "does-not-exist")
	withCurrentExecutable(t, func() (string, error) { return missing, nil })

	if err := installUpdatedBinary(downloaded); err == nil {
		t.Fatal("expected error when current binary path does not exist")
	}
}
