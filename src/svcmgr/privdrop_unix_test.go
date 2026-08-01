//go:build !windows

package svcmgr

import (
	"os"
	"testing"
)

// TestDropPrivilegesIfRoot_NonexistentUserIsNoop calls DropPrivilegesIfRoot
// with a target username that is guaranteed not to exist on any test host.
// This is deliberate: it forces the function down the safe "service user
// doesn't exist — skip drop silently" branch regardless of whether the test
// process itself is running as root, so the real Setgroups/Setgid/Setuid
// syscalls are never reached. Calling this with a real, existing user would
// irreversibly drop the test binary's privileges for the rest of the run,
// so that path is intentionally never exercised here.
func TestDropPrivilegesIfRoot_NonexistentUserIsNoop(t *testing.T) {
	const bogusUser = "nonexistent-user-devnull-caslink-test"

	if err := DropPrivilegesIfRoot(bogusUser); err != nil {
		t.Errorf("DropPrivilegesIfRoot(%q) = %v, want nil (user lookup should fail silently)", bogusUser, err)
	}
}

// TestDropPrivilegesIfRoot_NotRootIsNoop verifies the very first guard: when
// the calling process is not root, the function must return nil immediately
// without attempting any user lookup or syscall, even for an empty
// (default-to-"caslink") target user. This is only meaningful when the test
// process is actually non-root; when running as root (common in Docker
// build images) we skip rather than risk resolving a real "caslink" user
// that might legitimately exist on the host and dropping this process's
// privileges for real.
func TestDropPrivilegesIfRoot_NotRootIsNoop(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test process is running as root; skipping to avoid risking a real privilege drop " +
			"if a \"caslink\" system user happens to exist on this host")
	}

	if err := DropPrivilegesIfRoot(""); err != nil {
		t.Errorf("DropPrivilegesIfRoot(\"\") as non-root = %v, want nil", err)
	}
}
