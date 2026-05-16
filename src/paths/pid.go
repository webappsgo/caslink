package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ErrAlreadyRunning is returned by CheckPIDFile when the process named in the
// PID file is still alive and appears to be the same binary.
var ErrAlreadyRunning = errors.New("already running")

// CheckPIDFile reads the PID file at pidPath and verifies whether the process
// is still alive.
//
// Returns:
//   - (pid, nil)             — file exists; process is alive → ErrAlreadyRunning
//   - (0, ErrAlreadyRunning) — process is running per pidPath
//   - (0, nil)               — file missing or stale (safe to start)
//   - (0, err)               — unexpected I/O error
//
// On Unix, it additionally checks /proc/{pid}/exe to confirm the process is our
// own binary (prevents false positives from PID reuse).
func CheckPIDFile(pidPath, binaryName string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // file absent — safe to start
		}
		return 0, fmt.Errorf("reading pid file: %w", err)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		// Corrupt PID file — remove it and allow startup.
		_ = os.Remove(pidPath)
		return 0, nil
	}

	if !processAlive(pid, binaryName) {
		// Stale file — remove it and allow startup.
		_ = os.Remove(pidPath)
		return 0, nil
	}

	return pid, ErrAlreadyRunning
}

// WritePIDFile writes the current process PID to pidPath, creating parent
// directories as needed.
func WritePIDFile(pidPath string) error {
	dir := filepath.Dir(pidPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating pid directory: %w", err)
	}
	pid := os.Getpid()
	data := []byte(strconv.Itoa(pid) + "\n")
	if err := os.WriteFile(pidPath, data, 0644); err != nil {
		return fmt.Errorf("writing pid file: %w", err)
	}
	return nil
}

// RemovePIDFile removes the PID file. Errors are silently ignored (called from
// signal handlers and deferred cleanup where the process is exiting anyway).
func RemovePIDFile(pidPath string) {
	_ = os.Remove(pidPath)
}

// processAlive returns true if pid refers to a running process that appears to
// be the same binary.
//
// On Linux it inspects /proc/{pid}/exe (reliable, no signal needed).
// On other platforms it delegates to processAliveOS (platform-specific file).
func processAlive(pid int, binaryName string) bool {
	if runtime.GOOS == "linux" {
		exePath := fmt.Sprintf("/proc/%d/exe", pid)
		target, err := os.Readlink(exePath)
		if err != nil {
			// Can't read link — process likely dead.
			return false
		}
		// Confirm the binary name matches to prevent PID-reuse false positives.
		if binaryName != "" && !strings.Contains(filepath.Base(target), binaryName) {
			return false
		}
		return true
	}

	return processAliveOS(pid)
}
