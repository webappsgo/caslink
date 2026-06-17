//go:build !windows

package svcmgr

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// DropPrivilegesIfRoot drops from root to the caslink system user after
// binding a privileged port (< 1024) per AI.md PART 24.
// Returns nil immediately when:
//   - not running as root (os.Getuid() != 0)
//   - running on Windows (build-tagged out)
//   - the caslink user does not exist (non-privileged port scenario)
//
// Sets supplementary groups, GID, then UID in that order per
// the POSIX-correct privilege-drop sequence.
func DropPrivilegesIfRoot(targetUser string) error {
	if os.Getuid() != 0 {
		return nil
	}
	if targetUser == "" {
		targetUser = "caslink"
	}
	u, err := user.Lookup(targetUser)
	if err != nil {
		// Service user doesn't exist — skip drop silently.
		// It will be created by --service --install.
		return nil
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("invalid UID for %s: %w", targetUser, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return fmt.Errorf("invalid GID for %s: %w", targetUser, err)
	}

	// Set supplementary groups first (while still root).
	if err := syscall.Setgroups([]int{gid}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	// Drop GID before UID (setgid after setuid would fail).
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("setgid %d: %w", gid, err)
	}
	// Drop UID last.
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("setuid %d: %w", uid, err)
	}

	return nil
}
