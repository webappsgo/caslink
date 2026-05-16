//go:build !windows

package paths

import "syscall"

// processAliveOS sends signal 0 to the process to check liveness without
// disturbing it. err == nil means the process exists and we have permission
// to signal it.
func processAliveOS(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
