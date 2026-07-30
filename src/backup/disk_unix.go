//go:build linux || darwin

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
	return stat.Blocks * bsize, stat.Bavail * bsize, nil
}
