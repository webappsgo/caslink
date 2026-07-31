//go:build linux || darwin

package metrics

import "testing"

func TestDiskCapacityRoot(t *testing.T) {
	total, free, err := diskCapacity(t.TempDir())
	if err != nil {
		t.Fatalf("diskCapacity: %v", err)
	}
	if total == 0 {
		t.Errorf("diskCapacity() total = 0, want > 0")
	}
	if free > total {
		t.Errorf("diskCapacity() free = %d, want <= total = %d", free, total)
	}
}

func TestDiskCapacityInvalidPath(t *testing.T) {
	if _, _, err := diskCapacity("/this/path/does/not/exist/at/all"); err == nil {
		t.Errorf("diskCapacity() with a nonexistent path: expected an error, got nil")
	}
}
