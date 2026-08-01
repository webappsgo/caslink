//go:build linux || darwin || freebsd

package metrics

import "syscall"

// diskCapacity returns total and free bytes for the filesystem containing path.
func diskCapacity(path string) (total, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	bsize := uint64(stat.Bsize)
	return stat.Blocks * bsize, stat.Bavail * bsize, nil
}
