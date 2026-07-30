package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// touchBackup creates an empty dated backup file (+ manifest sidecar) named
// as createArchive would, so ApplyRetention can be tested without going
// through a full RunBackup.
func touchBackup(t *testing.T, dir string, date time.Time, size int) string {
	t.Helper()
	name := "caslink_backup_" + date.Format("2006-01-02") + ".tar.gz"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0o640); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	if err := os.WriteFile(manifestPath(path), []byte("{}"), 0o640); err != nil {
		t.Fatalf("write manifest for %q: %v", path, err)
	}
	return path
}

func TestApplyRetentionMaxBackups(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	var paths []string
	for i := 0; i < 5; i++ {
		paths = append(paths, touchBackup(t, dir, base.AddDate(0, 0, i), 10))
	}

	if err := ApplyRetention(dir, Retention{MaxBackups: 2}); err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}

	// Only the two most recent (days 3, 4) should survive.
	for i, p := range paths {
		_, err := os.Stat(p)
		shouldExist := i >= 3
		exists := err == nil
		if exists != shouldExist {
			t.Errorf("backup %d (%q): exists=%v, want %v", i, p, exists, shouldExist)
		}
		_, mErr := os.Stat(manifestPath(p))
		if (mErr == nil) != shouldExist {
			t.Errorf("manifest for backup %d: exists=%v, want %v", i, mErr == nil, shouldExist)
		}
	}
}

func TestApplyRetentionKeepsAtLeastOne(t *testing.T) {
	dir := t.TempDir()
	only := touchBackup(t, dir, time.Now(), 10)

	if err := ApplyRetention(dir, Retention{MaxBackups: 0}); err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}
	if _, err := os.Stat(only); err != nil {
		t.Fatal("expected MaxBackups<1 to still keep at least one backup")
	}
}

func TestApplyRetentionMaxTotalSizeAbsolute(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	oldest := touchBackup(t, dir, base, 100)
	middle := touchBackup(t, dir, base.AddDate(0, 0, 1), 100)
	newest := touchBackup(t, dir, base.AddDate(0, 0, 2), 100)

	// Keep all 3 by count, but cap total size to 150 bytes — each file is
	// 100 bytes, so only one fits under the cap; eviction proceeds oldest
	// first and never removes the last survivor.
	if err := ApplyRetention(dir, Retention{MaxBackups: 3, MaxTotalSize: "150"}); err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}

	if _, err := os.Stat(oldest); err == nil {
		t.Error("expected oldest backup to be evicted by the size cap")
	}
	if _, err := os.Stat(middle); err == nil {
		t.Error("expected middle backup to be evicted by the size cap")
	}
	if _, err := os.Stat(newest); err != nil {
		t.Error("expected newest backup to survive")
	}
}

func TestApplyRetentionIgnoresDailyIncremental(t *testing.T) {
	dir := t.TempDir()
	dated := touchBackup(t, dir, time.Now(), 10)
	daily := filepath.Join(dir, "caslink-daily.tar.gz")
	if err := os.WriteFile(daily, []byte("x"), 0o640); err != nil {
		t.Fatalf("write daily: %v", err)
	}

	if err := ApplyRetention(dir, Retention{MaxBackups: 1}); err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}
	if _, err := os.Stat(dated); err != nil {
		t.Error("expected dated backup to survive")
	}
	if _, err := os.Stat(daily); err != nil {
		t.Error("expected daily incremental to be untouched by retention")
	}
}
