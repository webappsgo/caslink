// Package backup provides tar.gz backup and restore helpers used by both the
// offline maintenance commands (package main) and the scheduled backup task
// (package scheduler). SQLite databases are stored inside dataDir and are
// included automatically; for external databases the operator must capture a
// DB dump separately.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunBackup packs configDir + dataDir into a dated full backup per AI.md PART 22.
//
// When explicitDst is non-empty it is used verbatim (absolute or CWD-relative).
// Otherwise the file is named caslink_backup_YYYY-MM-DD.tar.gz under backupDir.
// Returns the path of the created file.
func RunBackup(configDir, dataDir, backupDir, explicitDst string) error {
	dst, err := createArchive(configDir, dataDir, backupDir, explicitDst)
	if err != nil {
		return err
	}
	if err := Verify(dst); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("backup verification failed — deleted corrupt file %s: %w", dst, err)
	}
	fmt.Printf("Backup written and verified: %s\n", dst)
	return nil
}

// RunDailyBackup creates the dated full backup AND the fixed-name daily
// incremental file (caslink-daily.tar.gz) per AI.md PART 22.
// Both files are verified after creation; a verification failure deletes
// only the failed file and does not touch the other.
func RunDailyBackup(configDir, dataDir, backupDir string) error {
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	// Create dated full backup.
	fullPath, err := createArchive(configDir, dataDir, backupDir, "")
	if err != nil {
		return fmt.Errorf("create full backup: %w", err)
	}
	if err := Verify(fullPath); err != nil {
		_ = os.Remove(fullPath)
		return fmt.Errorf("full backup verification failed — deleted %s: %w", fullPath, err)
	}

	// Create/overwrite the fixed-name daily incremental.
	dailyPath := filepath.Join(backupDir, "caslink-daily.tar.gz")
	dailyTmp := dailyPath + ".tmp"
	if _, err := createArchive(configDir, dataDir, backupDir, dailyTmp); err != nil {
		return fmt.Errorf("create daily incremental: %w", err)
	}
	if err := Verify(dailyTmp); err != nil {
		_ = os.Remove(dailyTmp)
		return fmt.Errorf("daily incremental verification failed — deleted tmp: %w", err)
	}
	if err := os.Rename(dailyTmp, dailyPath); err != nil {
		_ = os.Remove(dailyTmp)
		return fmt.Errorf("rename daily incremental: %w", err)
	}

	fmt.Printf("Backup complete: %s (full) + %s (daily)\n", fullPath, dailyPath)
	return nil
}

// Verify checks that dst is a non-empty, readable tar.gz file.
// Returns nil when all checks pass; a descriptive error otherwise.
func Verify(dst string) error {
	info, err := os.Stat(dst)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("file is empty")
	}

	// Verify the archive can be fully read and decompressed.
	f, err := os.Open(dst)
	if err != nil {
		return fmt.Errorf("cannot open: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	tr, err := func() (*tar.Reader, error) {
		gz, err := gzip.NewReader(io.TeeReader(f, h))
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		return tar.NewReader(gz), nil
	}()
	if err != nil {
		return err
	}

	var entries int
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("corrupt tar entry after %d files: %w", entries, err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return fmt.Errorf("read entry %q: %w", hdr.Name, err)
			}
		}
		entries++
	}
	if entries == 0 {
		return fmt.Errorf("archive contains no entries")
	}

	_ = hex.EncodeToString(h.Sum(nil)) // checksum available for future manifest
	return nil
}

// RunRestore extracts src into configDir/dataDir, mirroring the directory map
// written by RunBackup. Path traversal attempts inside the archive are rejected.
func RunRestore(src, configDir, dataDir string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer in.Close()

	gz, err := gzip.NewReader(in)
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

// createArchive packs configDir + dataDir into a single tar.gz.
// dst is the output file path; when empty, a dated name is auto-generated.
// Returns the final output path.
func createArchive(configDir, dataDir, backupDir, dst string) (string, error) {
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	if dst == "" {
		// Format per AI.md PART 22: caslink_backup_YYYY-MM-DD.tar.gz
		date := time.Now().UTC().Format("2006-01-02")
		dst = filepath.Join(backupDir, fmt.Sprintf("caslink_backup_%s.tar.gz", date))
	}

	out, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("create %q: %w", dst, err)
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	// Map each source dir to a stable archive prefix so restore can reverse it.
	sources := map[string]string{
		"config": configDir,
		"data":   dataDir,
	}
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
