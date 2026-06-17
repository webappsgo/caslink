//go:build windows

package svcmgr

// DropPrivilegesIfRoot is a no-op on Windows.
// Windows services run under a Virtual Service Account (VSA) which already
// has minimal-privilege; no privilege drop step is required per AI.md PART 24.
func DropPrivilegesIfRoot(targetUser string) error {
	return nil
}
