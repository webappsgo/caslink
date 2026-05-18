// Offline maintenance helpers for `caslink --maintenance backup` and
// `caslink --maintenance restore` (AI.md PART 22). These run against the
// filesystem only — for external databases the operator must still capture
// a DB dump separately. SQLite databases are part of the data directory and
// are included automatically.
package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runOfflineBackup packs configDir + dataDir into a single tar.gz under
// backupDir. When explicitDst is non-empty it is used verbatim (treated as
// an absolute or working-dir-relative file path). Otherwise the file is
// named caslink-YYYYMMDD-HHMMSS.tar.gz under backupDir.
func runOfflineBackup(configDir, dataDir, backupDir, explicitDst string) error {
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	dst := explicitDst
	if dst == "" {
		stamp := time.Now().UTC().Format("20060102-150405")
		dst = filepath.Join(backupDir, fmt.Sprintf("caslink-%s.tar.gz", stamp))
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %q: %w", dst, err)
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	// Map each source dir to a stable archive prefix so restore can reverse it
	// deterministically.
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
			return fmt.Errorf("archive %s: %w", prefix, err)
		}
	}

	fmt.Printf("Backup written to %s\n", dst)
	return nil
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

// runOfflineRestore extracts src into configDir/dataDir, mirroring the
// directory map written by runOfflineBackup. Path traversal attempts inside
// the archive are rejected.
func runOfflineRestore(src, configDir, dataDir string) error {
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
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
				return fmt.Errorf("mkdir parent %q: %w", dst, err)
			}
			out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o7777)
			if err != nil {
				return fmt.Errorf("create %q: %w", dst, err)
			}
			const maxBytes = 1 << 30 // 1 GiB per file
			if _, err := io.Copy(out, io.LimitReader(tr, maxBytes)); err != nil {
				_ = out.Close()
				return fmt.Errorf("write %q: %w", dst, err)
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			// skip symlinks, devices, etc.
		}
	}

	fmt.Printf("Restore complete: %s → %s, %s\n", src, configDir, dataDir)
	return nil
}
