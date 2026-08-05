// Package backup provides tar.gz backup and restore helpers used by both the
// offline maintenance commands (package main) and the scheduled backup task
// (package scheduler). SQLite databases are stored inside dataDir and are
// included automatically; for external databases the operator must capture a
// DB dump separately.
package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrCompliancePasswordRequired is returned when compliance mode is enabled
// but no backup encryption password is available, per AI.md PART 22
// "Compliance Mode Enforcement": backups must not run unencrypted.
var ErrCompliancePasswordRequired = errors.New("backup encryption password required: compliance mode is enabled and no backup is allowed unencrypted")

// ErrSkippedDiskFull is returned when a backup is aborted before creation
// because the target filesystem is too full, per AI.md PART 22: free space
// must be at least twice the most recent backup's size, and overall disk
// usage must stay at or below disk_threshold (default 90%).
var ErrSkippedDiskFull = errors.New("backup skipped: insufficient disk space")

// Options controls how a backup is created per AI.md PART 22.
type Options struct {
	// Password encrypts the archive with AES-256-GCM using an Argon2id-derived
	// key when non-empty. It is never persisted anywhere.
	Password string
	// ComplianceRequired blocks the backup entirely when Password is empty.
	ComplianceRequired bool
	// CreatedBy is recorded in manifest.json ("administrator", a username, etc.).
	CreatedBy string
	// AppVersion is recorded in manifest.json.
	AppVersion string
	// DiskThreshold is the maximum disk-usage percentage (0-100) allowed before
	// a backup is skipped. Zero (or out of range) falls back to the PART 22
	// default of 90.
	DiskThreshold int
}

func (o Options) validate() error {
	if o.ComplianceRequired && o.Password == "" {
		return ErrCompliancePasswordRequired
	}
	return nil
}

func (o Options) diskThreshold() int {
	if o.DiskThreshold <= 0 || o.DiskThreshold > 100 {
		return 90
	}
	return o.DiskThreshold
}

// precheckDiskSpace aborts a backup before any bytes are written when the
// target filesystem cannot safely hold a new backup (AI.md PART 22): overall
// disk usage must stay at or below the threshold, and free space must be at
// least twice the most recent backup's size. It fails open — an inability to
// stat the filesystem never blocks the backup.
func precheckDiskSpace(backupDir string, threshold int) error {
	if backupDir == "" {
		return nil
	}
	total, free, err := diskCapacity(backupDir)
	if err != nil || total == 0 {
		return nil
	}
	usedPct := float64(total-free) / float64(total) * 100
	if usedPct > float64(threshold) {
		return fmt.Errorf("%w: disk %.1f%% used exceeds %d%% threshold", ErrSkippedDiskFull, usedPct, threshold)
	}
	recent := mostRecentBackupSize(backupDir)
	if recent > 0 && free < 2*recent {
		return fmt.Errorf("%w: free %d bytes < 2x most recent backup (%d bytes)", ErrSkippedDiskFull, free, recent)
	}
	return nil
}

// mostRecentBackupSize returns the byte size of the newest backup archive in
// backupDir, or 0 when none exist or the directory cannot be read.
func mostRecentBackupSize(backupDir string) uint64 {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return 0
	}
	var newest time.Time
	var size uint64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".tar.gz") && !strings.HasSuffix(n, ".tar.gz.enc") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
			size = uint64(info.Size())
		}
	}
	return size
}

// RunBackup packs configDir + dataDir into a dated full backup per AI.md PART 22.
//
// When explicitDst is non-empty it is used verbatim (absolute or CWD-relative);
// a ".enc" suffix is appended automatically when opts.Password is set. Otherwise
// the file is named caslink_backup_YYYY-MM-DD.tar.gz[.enc] under backupDir.
func RunBackup(configDir, dataDir, backupDir, explicitDst string, opts Options) error {
	if err := opts.validate(); err != nil {
		return err
	}
	checkDir := backupDir
	if checkDir == "" && explicitDst != "" {
		checkDir = filepath.Dir(explicitDst)
	}
	if err := precheckDiskSpace(checkDir, opts.diskThreshold()); err != nil {
		return err
	}
	dst, err := createArchive(configDir, dataDir, backupDir, explicitDst, opts)
	if err != nil {
		return err
	}
	if err := Verify(dst, opts.Password); err != nil {
		_ = os.Remove(dst)
		_ = os.Remove(manifestPath(dst))
		return fmt.Errorf("backup verification failed — deleted corrupt file %s: %w", dst, err)
	}
	fmt.Printf("Backup written and verified: %s\n", dst)
	return nil
}

// RunDailyBackup creates the dated full backup AND the fixed-name daily
// incremental file (caslink-daily.tar.gz[.enc]) per AI.md PART 22.
// Both files are verified after creation; a verification failure deletes
// only the failed file and does not touch the other.
func RunDailyBackup(configDir, dataDir, backupDir string, opts Options) error {
	if err := opts.validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	if err := precheckDiskSpace(backupDir, opts.diskThreshold()); err != nil {
		return err
	}

	// Create dated full backup.
	fullPath, err := createArchive(configDir, dataDir, backupDir, "", opts)
	if err != nil {
		return fmt.Errorf("create full backup: %w", err)
	}
	if err := Verify(fullPath, opts.Password); err != nil {
		_ = os.Remove(fullPath)
		_ = os.Remove(manifestPath(fullPath))
		return fmt.Errorf("full backup verification failed — deleted %s: %w", fullPath, err)
	}

	// Create/overwrite the fixed-name daily incremental. createArchive appends
	// ".enc" itself when opts.Password is set, so the base name stays bare here.
	dailyBase := filepath.Join(backupDir, "caslink-daily.tar.gz")
	dailyTmp := dailyBase + ".tmp"
	dailyPath := dailyBase
	if opts.Password != "" {
		dailyPath += ".enc"
	}
	createdTmp, err := createArchive(configDir, dataDir, backupDir, dailyTmp, opts)
	if err != nil {
		return fmt.Errorf("create daily incremental: %w", err)
	}
	if err := Verify(createdTmp, opts.Password); err != nil {
		_ = os.Remove(createdTmp)
		_ = os.Remove(manifestPath(createdTmp))
		return fmt.Errorf("daily incremental verification failed — deleted tmp: %w", err)
	}
	if err := os.Rename(createdTmp, dailyPath); err != nil {
		_ = os.Remove(createdTmp)
		_ = os.Remove(manifestPath(createdTmp))
		return fmt.Errorf("rename daily incremental: %w", err)
	}
	if err := os.Rename(manifestPath(createdTmp), manifestPath(dailyPath)); err != nil {
		return fmt.Errorf("rename daily incremental manifest: %w", err)
	}

	fmt.Printf("Backup complete: %s (full) + %s (daily)\n", fullPath, dailyPath)
	return nil
}

// Verify checks that dst is a non-empty, readable backup, per AI.md PART 22's
// verification checklist: file exists, size > 0, checksum matches manifest,
// decrypt test (if encrypted), manifest readable, content extraction test.
// Password is required to decrypt a ".enc" file; ignored otherwise.
func Verify(dst, password string) error {
	info, err := os.Stat(dst)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("file is empty")
	}

	m, err := readManifest(dst)
	if err != nil {
		return fmt.Errorf("manifest not readable: %w", err)
	}

	raw, err := os.ReadFile(dst)
	if err != nil {
		return fmt.Errorf("cannot read: %w", err)
	}

	plain := raw
	if m.Encrypted {
		if password == "" {
			return fmt.Errorf("decrypt test failed: file is encrypted but no password supplied")
		}
		plain, err = decryptArchive(raw, password)
		if err != nil {
			return fmt.Errorf("decrypt test failed: %w", err)
		}
	}

	if got := sha256Hex(plain); got != m.Checksum {
		return fmt.Errorf("checksum mismatch: manifest says %q, archive is %q", m.Checksum, got)
	}

	// Content extraction test: fully decompress and walk every tar entry.
	entries, err := countTarEntries(plain)
	if err != nil {
		return err
	}
	if entries == 0 {
		return fmt.Errorf("archive contains no entries")
	}

	return nil
}

// countTarEntries decompresses and walks every entry of a tar.gz byte slice,
// discarding file contents, to confirm the archive is fully readable.
func countTarEntries(gzData []byte) (int, error) {
	gz, err := gzip.NewReader(bytes.NewReader(gzData))
	if err != nil {
		return 0, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	var entries int
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return entries, fmt.Errorf("corrupt tar entry after %d files: %w", entries, err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return entries, fmt.Errorf("read entry %q: %w", hdr.Name, err)
			}
		}
		entries++
	}
	return entries, nil
}

// RunRestore extracts src into configDir/dataDir, mirroring the directory map
// written by RunBackup. Path traversal attempts inside the archive are
// rejected. password decrypts a ".tar.gz.enc" source; ignored otherwise.
func RunRestore(src, configDir, dataDir, password string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}

	if strings.HasSuffix(src, ".enc") {
		if password == "" {
			return fmt.Errorf("%q is encrypted: a password is required to restore", src)
		}
		raw, err = decryptArchive(raw, password)
		if err != nil {
			return fmt.Errorf("decrypt %q: %w", src, err)
		}
	}

	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	prefixDirs := map[string]string{
		"config": configDir,
		"data":   dataDir,
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		// Reject path traversal.
		if strings.Contains(hdr.Name, "..") {
			return fmt.Errorf("refusing entry with traversal: %q", hdr.Name)
		}
		parts := strings.SplitN(hdr.Name, "/", 2)
		if len(parts) < 2 {
			continue
		}
		base, ok := prefixDirs[parts[0]]
		if !ok || base == "" {
			continue
		}
		dst := filepath.Join(base, filepath.FromSlash(parts[1]))
		// Ensure dst remains inside base (belt + braces on top of the .. check).
		absBase, _ := filepath.Abs(base)
		absDst, _ := filepath.Abs(dst)
		if !strings.HasPrefix(absDst, absBase+string(os.PathSeparator)) && absDst != absBase {
			return fmt.Errorf("refusing entry that escapes target: %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, os.FileMode(hdr.Mode)&0o7777); err != nil {
				return fmt.Errorf("mkdir %q: %w", dst, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
				return fmt.Errorf("mkdir parent %q: %w", dst, err)
			}
			outf, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o7777)
			if err != nil {
				return fmt.Errorf("create %q: %w", dst, err)
			}
			const maxBytes = 1 << 30 // 1 GiB per file
			if _, err := io.Copy(outf, io.LimitReader(tr, maxBytes)); err != nil {
				_ = outf.Close()
				return fmt.Errorf("write %q: %w", dst, err)
			}
			if err := outf.Close(); err != nil {
				return err
			}
		default:
			// skip symlinks, devices, etc.
		}
	}

	fmt.Printf("Restore complete: %s → %s, %s\n", src, configDir, dataDir)
	return nil
}

// createArchive packs configDir + dataDir into a tar.gz built entirely in
// memory, optionally encrypts it (AES-256-GCM, Argon2id-derived key) per
// AI.md PART 22 ("archive is created in memory... unencrypted archive never
// touches disk"), then writes the final file plus its manifest.json sidecar.
//
// dst is the output file path; when empty, a dated name is auto-generated.
// A ".enc" suffix is appended automatically when opts.Password is set.
// Returns the final output path (including any ".enc" suffix applied).
func createArchive(configDir, dataDir, backupDir, dst string, opts Options) (string, error) {
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	if dst == "" {
		// Format per AI.md PART 22: caslink_backup_YYYY-MM-DD.tar.gz
		date := time.Now().UTC().Format("2006-01-02")
		dst = filepath.Join(backupDir, fmt.Sprintf("caslink_backup_%s.tar.gz", date))
	}
	if opts.Password != "" && !strings.HasSuffix(dst, ".enc") {
		dst += ".enc"
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	// Map each source dir to a stable archive prefix so restore can reverse it.
	sources := map[string]string{
		"config": configDir,
		"data":   dataDir,
	}
	var contents []string
	for prefix, root := range sources {
		if root == "" {
			continue
		}
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		if err := addTreeToTar(tw, root, prefix); err != nil {
			return "", fmt.Errorf("archive %s: %w", prefix, err)
		}
		contents = append(contents, prefix+"/")
	}
	if err := tw.Close(); err != nil {
		return "", fmt.Errorf("close tar writer: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("close gzip writer: %w", err)
	}

	plain := buf.Bytes()
	checksum := sha256Hex(plain)

	final := plain
	encrypted := opts.Password != ""
	if encrypted {
		enc, err := encryptArchive(plain, opts.Password)
		if err != nil {
			return "", err
		}
		final = enc
	}

	if err := os.WriteFile(dst, final, 0o640); err != nil {
		return "", fmt.Errorf("write %q: %w", dst, err)
	}

	createdBy := opts.CreatedBy
	if createdBy == "" {
		createdBy = "administrator"
	}
	appVersion := opts.AppVersion
	if appVersion == "" {
		appVersion = "dev"
	}
	method := ""
	if encrypted {
		method = EncryptionMethod
	}
	m := Manifest{
		Version:          "1.0.0",
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		CreatedBy:        createdBy,
		AppVersion:       appVersion,
		Contents:         contents,
		Encrypted:        encrypted,
		EncryptionMethod: method,
		Checksum:         checksum,
	}
	if err := writeManifest(dst, m); err != nil {
		_ = os.Remove(dst)
		return "", err
	}

	return dst, nil
}

// addTreeToTar walks root and writes every regular file beneath it into tw,
// rooted at the archive prefix. Symlinks and special files are skipped to
// avoid escaping the archive root during restore.
func addTreeToTar(tw *tar.Writer, root, prefix string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		archiveName := filepath.ToSlash(filepath.Join(prefix, rel))
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = archiveName
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, f)
			_ = f.Close()
			if err != nil {
				return err
			}
		}
		return nil
	})
}
