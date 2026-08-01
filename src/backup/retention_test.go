package backup

import (
	"os"
	"path/filepath"
	"strconv"
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

// TestApplyRetentionEmptyDir covers the zero-backups boundary: an empty
// backup directory must be a no-op, not an error.
func TestApplyRetentionEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := ApplyRetention(dir, Retention{MaxBackups: 5}); err != nil {
		t.Fatalf("expected no error for an empty backup dir, got: %v", err)
	}
}

// TestApplyRetentionNonexistentDir covers the error path when the backup
// directory itself does not exist — ReadDir's error must propagate rather
// than being swallowed.
func TestApplyRetentionNonexistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	if err := ApplyRetention(dir, Retention{MaxBackups: 5}); err == nil {
		t.Fatal("expected ApplyRetention to fail for a nonexistent backup dir")
	}
}

// TestApplyRetentionExactlyAtMaxBackups covers the boundary where the
// backup count exactly equals MaxBackups: nothing should be deleted.
func TestApplyRetentionExactlyAtMaxBackups(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	var paths []string
	for i := 0; i < 3; i++ {
		paths = append(paths, touchBackup(t, dir, base.AddDate(0, 0, i), 10))
	}

	if err := ApplyRetention(dir, Retention{MaxBackups: 3}); err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}
	for i, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("backup %d (%q) should survive when count == MaxBackups: %v", i, p, err)
		}
	}
}

// nextSunday returns the first day on or after t that falls on a Sunday, so
// tests can build weekly-tier fixtures without hand-computing weekdays.
func nextSunday(t time.Time) time.Time {
	for t.Weekday() != time.Sunday {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// TestApplyRetentionTierPriority covers the yearly/monthly/weekly/daily
// bucket logic together: a backup on a "special" date (Jan 1, 1st-of-month,
// Sunday) survives via its own tier even when it falls outside the recent
// MaxBackups window, while a backup on an ordinary old date is deleted, and
// each periodic tier keeps only its N most recent distinct buckets.
func TestApplyRetentionTierPriority(t *testing.T) {
	dir := t.TempDir()

	yearly2024 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	monthly2024 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC) // superseded: KeepMonthly=1
	weekly2024 := nextSunday(monthly2024)                      // superseded: KeepWeekly=1
	strayOld := time.Date(2024, 11, 15, 0, 0, 0, 0, time.UTC)  // ordinary date, no bucket
	yearly2025 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	monthly2025 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	weekly2026 := nextSunday(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC))
	dailyOlder := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC) // outside top-2 daily, no bucket
	dailyMid := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	dailyNewest := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	if strayOld.Weekday() == time.Sunday || strayOld.Day() == 1 {
		t.Fatal("fixture bug: strayOld must not land on a Sunday or the 1st")
	}
	if dailyOlder.Weekday() == time.Sunday || dailyOlder.Day() == 1 {
		t.Fatal("fixture bug: dailyOlder must not land on a Sunday or the 1st")
	}

	pYearly2024 := touchBackup(t, dir, yearly2024, 10)
	pMonthly2024 := touchBackup(t, dir, monthly2024, 10)
	pWeekly2024 := touchBackup(t, dir, weekly2024, 10)
	pStray := touchBackup(t, dir, strayOld, 10)
	pYearly2025 := touchBackup(t, dir, yearly2025, 10)
	pMonthly2025 := touchBackup(t, dir, monthly2025, 10)
	pWeekly2026 := touchBackup(t, dir, weekly2026, 10)
	pDailyOlder := touchBackup(t, dir, dailyOlder, 10)
	pDailyMid := touchBackup(t, dir, dailyMid, 10)
	pDailyNewest := touchBackup(t, dir, dailyNewest, 10)

	err := ApplyRetention(dir, Retention{
		MaxBackups:  2,
		KeepWeekly:  1,
		KeepMonthly: 1,
		KeepYearly:  2,
	})
	if err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}

	want := map[string]bool{
		pYearly2024:  true,  // kept: yearly bucket 2024 (of 2 kept years)
		pMonthly2024: false, // superseded by the more recent 2025-03 monthly bucket
		pWeekly2024:  false, // superseded by the more recent 2026 weekly bucket
		pStray:       false, // no tier applies, outside MaxBackups window
		pYearly2025:  true,  // kept: yearly bucket 2025
		pMonthly2025: true,  // kept: most recent monthly bucket
		pWeekly2026:  true,  // kept: most recent weekly bucket
		pDailyOlder:  false, // outside the 2 most recent daily backups, no other tier
		pDailyMid:    true,  // kept: within MaxBackups=2
		pDailyNewest: true,  // kept: within MaxBackups=2
	}
	for path, shouldExist := range want {
		_, statErr := os.Stat(path)
		exists := statErr == nil
		if exists != shouldExist {
			t.Errorf("%s: exists=%v, want %v", filepath.Base(path), exists, shouldExist)
		}
	}
}

// TestApplyRetentionMaxTotalSizePercentageNoEviction covers a percentage cap
// large enough that it never triggers eviction beyond the count-based tiers.
func TestApplyRetentionMaxTotalSizePercentageNoEviction(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	a := touchBackup(t, dir, base, 100)
	b := touchBackup(t, dir, base.AddDate(0, 0, 1), 100)

	// 100% of the real filesystem's capacity is always far larger than the
	// couple hundred bytes these fixtures use, regardless of the test host.
	if err := ApplyRetention(dir, Retention{MaxBackups: 2, MaxTotalSize: "100%"}); err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}
	if _, err := os.Stat(a); err != nil {
		t.Error("expected backup a to survive an oversized percentage cap")
	}
	if _, err := os.Stat(b); err != nil {
		t.Error("expected backup b to survive an oversized percentage cap")
	}
}

// TestApplyRetentionMaxTotalSizePercentageEvictsToOne covers a percentage
// cap small enough to force eviction, while the "never delete the last
// survivor" guarantee still holds. The percentage is derived from the real
// filesystem's reported capacity so the test forces eviction regardless of
// how large the underlying disk actually is.
func TestApplyRetentionMaxTotalSizePercentageEvictsToOne(t *testing.T) {
	dir := t.TempDir()

	total, _, err := diskCapacity(dir)
	if err != nil || total == 0 {
		t.Skipf("diskCapacity unavailable in this environment: %v", err)
	}

	const fixtureSize = 100
	// Cap set to 1.5x a single fixture's size (as a percentage of real disk
	// capacity), so two 100-byte backups exceed it but one alone does not.
	pct := (1.5 * fixtureSize) / float64(total) * 100

	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	oldest := touchBackup(t, dir, base, fixtureSize)
	middle := touchBackup(t, dir, base.AddDate(0, 0, 1), fixtureSize)
	newest := touchBackup(t, dir, base.AddDate(0, 0, 2), fixtureSize)

	r := Retention{MaxBackups: 3, MaxTotalSize: strconv.FormatFloat(pct, 'f', -1, 64) + "%"}
	if err := ApplyRetention(dir, r); err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}
	if _, err := os.Stat(oldest); err == nil {
		t.Error("expected oldest backup to be evicted by a near-zero percentage cap")
	}
	if _, err := os.Stat(middle); err == nil {
		t.Error("expected middle backup to be evicted by a near-zero percentage cap")
	}
	if _, err := os.Stat(newest); err != nil {
		t.Error("expected the newest (last survivor) backup to remain")
	}
}

// TestApplyRetentionMaxTotalSizeFalseyDisablesCap covers every documented
// falsey token for max_total_size: none of them should trigger size-based
// eviction beyond what MaxBackups already allows.
func TestApplyRetentionMaxTotalSizeFalseyDisablesCap(t *testing.T) {
	for _, falsey := range []string{"0", "false", "no", "none", "disable", "disabled", "off", ""} {
		t.Run(falsey, func(t *testing.T) {
			dir := t.TempDir()
			base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
			a := touchBackup(t, dir, base, 1<<20)
			b := touchBackup(t, dir, base.AddDate(0, 0, 1), 1<<20)

			if err := ApplyRetention(dir, Retention{MaxBackups: 2, MaxTotalSize: falsey}); err != nil {
				t.Fatalf("ApplyRetention failed: %v", err)
			}
			if _, err := os.Stat(a); err != nil {
				t.Errorf("MaxTotalSize=%q must not evict backup a", falsey)
			}
			if _, err := os.Stat(b); err != nil {
				t.Errorf("MaxTotalSize=%q must not evict backup b", falsey)
			}
		})
	}
}

// TestParseMaxTotalSizeUnits covers parseMaxTotalSize's absolute-unit
// parsing (k/m/g/t suffixes) and its rejection of invalid input.
func TestParseMaxTotalSizeUnits(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		raw      string
		wantOK   bool
		wantSize int64
	}{
		{"50G", true, 50 << 30},
		{"500M", true, 500 << 20},
		{"1T", true, 1 << 40},
		{"10K", true, 10 << 10},
		{"1024", true, 1024},
		{"0", false, 0},
		{"false", false, 0},
		{"", false, 0},
		{"-5G", false, 0},
		{"not-a-size", false, 0},
		{"0%", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			gotSize, gotOK := parseMaxTotalSize(tc.raw, dir)
			if gotOK != tc.wantOK {
				t.Fatalf("parseMaxTotalSize(%q) ok = %v, want %v", tc.raw, gotOK, tc.wantOK)
			}
			if gotOK && gotSize != tc.wantSize {
				t.Fatalf("parseMaxTotalSize(%q) = %d, want %d", tc.raw, gotSize, tc.wantSize)
			}
		})
	}
}
