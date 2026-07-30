package backup

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Retention controls how many backups are kept per AI.md PART 22
// "Backup Retention". It mirrors config.BackupRetentionConfig field-for-field
// so callers can convert with a plain struct literal without an import cycle.
type Retention struct {
	MaxBackups   int    // daily full backups to keep (>=1)
	KeepWeekly   int    // Sunday backups to keep (0 = disabled)
	KeepMonthly  int    // 1st-of-month backups to keep (0 = disabled)
	KeepYearly   int    // Jan-1st backups to keep (0 = disabled)
	MaxTotalSize string // percent ("10%"), absolute ("50G"), or falsey to disable
}

// falseyRetentionValues are the spec's "all disabled" tokens for MaxTotalSize.
var falseyRetentionValues = map[string]bool{
	"0": true, "false": true, "no": true, "none": true,
	"disable": true, "disabled": true, "off": true, "": true,
}

// datedBackupRe matches the dated full-backup filenames created by
// createArchive: caslink_backup_YYYY-MM-DD.tar.gz[.enc]. The fixed-name daily
// incremental (caslink-daily.tar.gz[.enc]) and its manifest are never subject
// to retention — only dated full backups are candidates for deletion.
var datedBackupRe = regexp.MustCompile(`^caslink_backup_(\d{4})-(\d{2})-(\d{2})\.tar\.gz(\.enc)?$`)

type datedBackup struct {
	path string
	date time.Time
	size int64
}

// ApplyRetention deletes dated full backups in backupDir that fall outside
// the retention policy, per AI.md PART 22's "Backup Creation Flow" step 7
// ("Apply retention policy") — called only after a backup run's
// verifications all pass. The fixed-name daily incremental is left alone.
func ApplyRetention(backupDir string, r Retention) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return err
	}

	var backups []datedBackup
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := datedBackupRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		date, err := time.Parse("2006-01-02", m[1]+"-"+m[2]+"-"+m[3])
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		backups = append(backups, datedBackup{
			path: filepath.Join(backupDir, e.Name()),
			date: date,
			size: info.Size(),
		})
	}
	if len(backups) == 0 {
		return nil
	}

	// Newest first.
	sort.Slice(backups, func(i, j int) bool { return backups[i].date.After(backups[j].date) })

	keep := make(map[string]bool, len(backups))

	// Daily: the N most recent dated backups.
	maxDaily := r.MaxBackups
	if maxDaily < 1 {
		maxDaily = 1
	}
	for i := 0; i < maxDaily && i < len(backups); i++ {
		keep[backups[i].path] = true
	}

	// Weekly: most recent Sunday backup per week, for KeepWeekly weeks.
	keepPeriodic(backups, keep, r.KeepWeekly, func(t time.Time) string {
		if t.Weekday() != time.Sunday {
			return ""
		}
		y, w := t.ISOWeek()
		return strconv.Itoa(y) + "-W" + strconv.Itoa(w)
	})

	// Monthly: 1st-of-month backup, for KeepMonthly months.
	keepPeriodic(backups, keep, r.KeepMonthly, func(t time.Time) string {
		if t.Day() != 1 {
			return ""
		}
		return t.Format("2006-01")
	})

	// Yearly: Jan-1st backup, for KeepYearly years.
	keepPeriodic(backups, keep, r.KeepYearly, func(t time.Time) string {
		if t.Month() != time.January || t.Day() != 1 {
			return ""
		}
		return t.Format("2006")
	})

	// Delete everything not marked for keeping.
	for _, b := range backups {
		if keep[b.path] {
			continue
		}
		if err := deleteBackup(b.path); err != nil {
			return err
		}
	}

	// Re-read the surviving set and enforce the hard size cap, oldest first.
	if capBytes, ok := parseMaxTotalSize(r.MaxTotalSize, backupDir); ok {
		var kept []datedBackup
		for _, b := range backups {
			if keep[b.path] {
				kept = append(kept, b)
			}
		}
		sort.Slice(kept, func(i, j int) bool { return kept[i].date.Before(kept[j].date) })

		var total int64
		for _, b := range kept {
			total += b.size
		}
		for len(kept) > 1 && total > capBytes {
			oldest := kept[0]
			if err := deleteBackup(oldest.path); err != nil {
				return err
			}
			total -= oldest.size
			kept = kept[1:]
		}
	}

	return nil
}

// keepPeriodic marks up to n backups for keeping, one per distinct bucket
// value returned by bucketOf (empty string = not a candidate for this
// period), newest bucket first. backups must already be sorted newest-first.
func keepPeriodic(backups []datedBackup, keep map[string]bool, n int, bucketOf func(time.Time) string) {
	if n <= 0 {
		return
	}
	seen := make(map[string]bool, n)
	for _, b := range backups {
		if len(seen) >= n {
			return
		}
		bucket := bucketOf(b.date)
		if bucket == "" || seen[bucket] {
			continue
		}
		seen[bucket] = true
		keep[b.path] = true
	}
}

// deleteBackup removes an archive and its manifest sidecar.
func deleteBackup(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(manifestPath(path)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// parseMaxTotalSize interprets the retention max_total_size setting: a
// percentage of the backup volume's disk capacity ("10%"), an absolute size
// ("50G", "500M", "1T"), or a falsey value (disabled). ok is false when the
// cap is disabled or cannot be determined.
func parseMaxTotalSize(raw, backupDir string) (capBytes int64, ok bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if falseyRetentionValues[v] {
		return 0, false
	}

	if strings.HasSuffix(v, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
		if err != nil || pct <= 0 {
			return 0, false
		}
		total, _, err := diskCapacity(backupDir)
		if err != nil || total == 0 {
			return 0, false
		}
		return int64(float64(total) * pct / 100), true
	}

	units := map[byte]int64{'k': 1 << 10, 'm': 1 << 20, 'g': 1 << 30, 't': 1 << 40}
	if len(v) > 1 {
		if mult, isUnit := units[v[len(v)-1]]; isUnit {
			n, err := strconv.ParseFloat(strings.TrimSpace(v[:len(v)-1]), 64)
			if err != nil || n <= 0 {
				return 0, false
			}
			return int64(n * float64(mult)), true
		}
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
