//go:build !windows

package paths

import (
	"math"
	"os"
	"testing"
)

// TestProcessAliveOSSelf verifies signal-0 liveness check succeeds for the
// current process, which we always have permission to signal.
func TestProcessAliveOSSelf(t *testing.T) {
	if !processAliveOS(os.Getpid()) {
		t.Errorf("processAliveOS(self) = false, want true")
	}
}

// TestProcessAliveOSNonexistentPID verifies signal-0 against a PID that does
// not exist returns false (ESRCH), rather than a false positive.
func TestProcessAliveOSNonexistentPID(t *testing.T) {
	if processAliveOS(math.MaxInt32) {
		t.Errorf("processAliveOS(MaxInt32) = true, want false")
	}
}
