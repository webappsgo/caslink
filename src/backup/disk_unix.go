//go:build linux || darwin || freebsd

package backup

import "syscall"

// diskCapacity returns the total and available bytes of the filesystem
// containing path, used to resolve a percentage max_total_size retention cap.
func diskCapacity(path string) (total, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	bsize := uint64(stat.Bsize)
	// Bavail is int64 on freebsd (it can go negative for reserved blocks)
	// but uint64 on linux/darwin; converting through int64 compiles on all
	// three and lets us clamp a negative freebsd value to zero instead of
	// wrapping into a huge uint64.
	bavail := int64(stat.Bavail)
	if bavail < 0 {
		bavail = 0
	}
	return stat.Blocks * bsize, uint64(bavail) * bsize, nil
}
