package paths

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestCheckPIDFileMissingFile verifies a missing PID file is treated as
// "safe to start" — (0, nil), not an error.
func TestCheckPIDFileMissingFile(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "does-not-exist.pid")

	pid, err := CheckPIDFile(pidPath, "caslink")
	if err != nil {
		t.Fatalf("CheckPIDFile() error = %v, want nil", err)
	}
	if pid != 0 {
		t.Errorf("CheckPIDFile() pid = %d, want 0", pid)
	}
}

// TestCheckPIDFileCorruptContentIsRemoved verifies a PID file with
// non-numeric content is treated as stale: removed, and (0, nil) returned so
// startup can proceed.
func TestCheckPIDFileCorruptContentIsRemoved(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "caslink.pid")
	if err := os.WriteFile(pidPath, []byte("not-a-pid\n"), 0644); err != nil {
		t.Fatalf("seed pid file: %v", err)
	}

	pid, err := CheckPIDFile(pidPath, "caslink")
	if err != nil {
		t.Fatalf("CheckPIDFile() error = %v, want nil", err)
	}
	if pid != 0 {
		t.Errorf("CheckPIDFile() pid = %d, want 0", pid)
	}
	if _, statErr := os.Stat(pidPath); !os.IsNotExist(statErr) {
		t.Errorf("corrupt pid file was not removed")
	}
}

// TestCheckPIDFileZeroOrNegativePIDIsRemoved verifies a syntactically
// numeric but semantically invalid PID (<=0) is also treated as corrupt.
func TestCheckPIDFileZeroOrNegativePIDIsRemoved(t *testing.T) {
	for _, val := range []string{"0", "-1"} {
		t.Run(val, func(t *testing.T) {
			pidPath := filepath.Join(t.TempDir(), "caslink.pid")
			if err := os.WriteFile(pidPath, []byte(val), 0644); err != nil {
				t.Fatalf("seed pid file: %v", err)
			}

			pid, err := CheckPIDFile(pidPath, "caslink")
			if err != nil {
				t.Fatalf("CheckPIDFile() error = %v, want nil", err)
			}
			if pid != 0 {
				t.Errorf("CheckPIDFile() pid = %d, want 0", pid)
			}
		})
	}
}

// TestCheckPIDFileStalePIDIsRemoved verifies a PID file naming a process
// that is not running is treated as stale: removed, (0, nil) returned.
// math.MaxInt32 is used as a PID virtually guaranteed not to be alive.
func TestCheckPIDFileStalePIDIsRemoved(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "caslink.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(math.MaxInt32)), 0644); err != nil {
		t.Fatalf("seed pid file: %v", err)
	}

	pid, err := CheckPIDFile(pidPath, "caslink")
	if err != nil {
		t.Fatalf("CheckPIDFile() error = %v, want nil", err)
	}
	if pid != 0 {
		t.Errorf("CheckPIDFile() pid = %d, want 0", pid)
	}
	if _, statErr := os.Stat(pidPath); !os.IsNotExist(statErr) {
		t.Errorf("stale pid file was not removed")
	}
}

// TestCheckPIDFileLiveProcessSameBinaryReturnsAlreadyRunning verifies the
// core "prevent double start" behavior: writing the current test process's
// own PID and checking with no binary-name filter (empty string skips the
// name check) must report ErrAlreadyRunning, since the process genuinely is
// alive.
func TestCheckPIDFileLiveProcessSameBinaryReturnsAlreadyRunning(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "caslink.pid")
	selfPID := os.Getpid()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(selfPID)), 0644); err != nil {
		t.Fatalf("seed pid file: %v", err)
	}

	pid, err := CheckPIDFile(pidPath, "")
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("CheckPIDFile() error = %v, want ErrAlreadyRunning", err)
	}
	if pid != selfPID {
		t.Errorf("CheckPIDFile() pid = %d, want %d", pid, selfPID)
	}
	// The live-process case must not delete the PID file.
	if _, statErr := os.Stat(pidPath); statErr != nil {
		t.Errorf("pid file for a live process was unexpectedly removed: %v", statErr)
	}
}

// TestCheckPIDFileLiveProcessDifferentBinaryIsTreatedAsStale verifies the
// PID-reuse protection on Linux: even though the PID is alive, if
// /proc/{pid}/exe's basename does not contain the expected binaryName the
// process is treated as a different program, so the file is removed and
// startup is allowed to proceed.
func TestCheckPIDFileLiveProcessDifferentBinaryIsTreatedAsStale(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "caslink.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatalf("seed pid file: %v", err)
	}

	pid, err := CheckPIDFile(pidPath, "definitely-not-this-test-binary-name")
	if err != nil {
		t.Fatalf("CheckPIDFile() error = %v, want nil (treated as stale)", err)
	}
	if pid != 0 {
		t.Errorf("CheckPIDFile() pid = %d, want 0", pid)
	}
	if _, statErr := os.Stat(pidPath); !os.IsNotExist(statErr) {
		t.Errorf("pid file for a binary-name mismatch was not removed")
	}
}

// TestWritePIDFileWritesCurrentPID verifies WritePIDFile writes the calling
// process's own PID with a trailing newline, and creates parent directories
// as needed.
func TestWritePIDFileWritesCurrentPID(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "nested", "sub", "caslink.pid")

	if err := WritePIDFile(pidPath); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("reading pid file: %v", err)
	}

	want := strconv.Itoa(os.Getpid()) + "\n"
	if string(data) != want {
		t.Errorf("pid file content = %q, want %q", data, want)
	}
}

// TestRemovePIDFileIgnoresMissingFile verifies RemovePIDFile does not panic
// or otherwise surface an error when the file is already gone — it is
// called from signal handlers where the process is exiting regardless.
func TestRemovePIDFileIgnoresMissingFile(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "never-created.pid")
	// Must not panic.
	RemovePIDFile(pidPath)
}

// TestRemovePIDFileRemovesExistingFile verifies the happy path actually
// deletes the file.
func TestRemovePIDFileRemovesExistingFile(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "caslink.pid")
	if err := os.WriteFile(pidPath, []byte("123"), 0644); err != nil {
		t.Fatalf("seed pid file: %v", err)
	}

	RemovePIDFile(pidPath)

	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Errorf("RemovePIDFile did not remove the file")
	}
}

// TestProcessAliveSelf verifies processAlive reports true for the current,
// definitely-running process.
func TestProcessAliveSelf(t *testing.T) {
	if !processAlive(os.Getpid(), "") {
		t.Errorf("processAlive(self) = false, want true")
	}
}

// TestProcessAliveNonexistentPID verifies processAlive reports false for a
// PID that (virtually certainly) does not correspond to a running process.
func TestProcessAliveNonexistentPID(t *testing.T) {
	if processAlive(math.MaxInt32, "") {
		t.Errorf("processAlive(MaxInt32) = true, want false")
	}
}
