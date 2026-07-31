//go:build windows

package metrics

import "golang.org/x/sys/windows"

// diskCapacity returns total and free bytes for the filesystem containing path.
func diskCapacity(path string) (total, free uint64, err error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeBytes, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &freeBytes, &totalBytes, &totalFreeBytes); err != nil {
		return 0, 0, err
	}
	return totalBytes, freeBytes, nil
}
