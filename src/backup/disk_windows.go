//go:build windows

package backup

import "golang.org/x/sys/windows"

// diskCapacity returns the total and available bytes of the volume
// containing path, used to resolve a percentage max_total_size retention cap.
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
