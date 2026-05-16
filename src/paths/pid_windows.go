//go:build windows

package paths

import (
	"os"
)

// processAliveOS checks whether a process with the given PID is running.
// On Windows we attempt to open the process; FindProcess always succeeds,
// but we can confirm existence by trying to signal it.
func processAliveOS(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, Signal with os.Interrupt simply calls TerminateProcess which
	// is destructive. Instead we use a nil signal equivalent by checking if the
	// HANDLE is valid via a zero-byte wait — not available without cgo.
	// Fall back to assuming alive if FindProcess succeeded (conservative).
	_ = proc
	return true
}
