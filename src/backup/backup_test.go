package backup

import (
	"os"
	"path/filepath"
	"testing"
)

// setupDirs creates config/data/backup dirs with a couple of files so
// createArchive has something real to pack.
func setupDirs(t *testing.T) (configDir, dataDir, backupDir string) {
	t.Helper()
	root := t.TempDir()
	configDir = filepath.Join(root, "config")
	dataDir = filepath.Join(root, "data")
	backupDir = filepath.Join(root, "backups")

	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server: {}\n"), 0o640); err != nil {
		t.Fatalf("write server.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "server.db"), []byte("fake-sqlite-data"), 0o640); err != nil {
		t.Fatalf("write server.db: %v", err)
	}
	return configDir, dataDir, backupDir
}

func TestRunBackupUnencrypted(t *testing.T) {
	configDir, dataDir, backupDir := setupDirs(t)

	if err := RunBackup(configDir, dataDir, backupDir, "", Options{}); err != nil {
		t.Fatalf("RunBackup failed: %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	var archives, manifests int
	for _, e := range entries {
		switch {
		case filepath.Ext(e.Name()) == ".json":
			manifests++
		case filepath.Ext(e.Name()) == ".enc":
			t.Fatalf("expected unencrypted output, found %q", e.Name())
		default:
			archives++
		}
	}
	if archives != 1 || manifests != 1 {
		t.Fatalf("expected 1 archive + 1 manifest, got %d archives, %d manifests", archives, manifests)
	}
}

func TestRunBackupEncryptedRoundTrip(t *testing.T) {
	configDir, dataDir, backupDir := setupDirs(t)
	dst := filepath.Join(backupDir, "test_backup.tar.gz")

	opts := Options{Password: "correct horse battery staple"}
	if err := RunBackup(configDir, dataDir, backupDir, dst, opts); err != nil {
		t.Fatalf("RunBackup failed: %v", err)
	}

	encPath := dst + ".enc"
	if _, err := os.Stat(encPath); err != nil {
		t.Fatalf("expected encrypted archive at %q: %v", encPath, err)
	}

	// Wrong password must fail verification.
	if err := Verify(encPath, "wrong password"); err == nil {
		t.Fatal("expected Verify to fail with wrong password")
	}

	// Right password must pass.
	if err := Verify(encPath, opts.Password); err != nil {
		t.Fatalf("Verify with correct password failed: %v", err)
	}

	// Restore must reproduce the original files.
	restoreConfig := filepath.Join(t.TempDir(), "config")
	restoreData := filepath.Join(t.TempDir(), "data")
	if err := RunRestore(encPath, restoreConfig, restoreData, opts.Password); err != nil {
		t.Fatalf("RunRestore failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(restoreConfig, "server.yml"))
	if err != nil {
		t.Fatalf("read restored server.yml: %v", err)
	}
	if string(got) != "server: {}\n" {
		t.Fatalf("restored server.yml mismatch: %q", got)
	}

	// Restore without a password on an encrypted file must fail, not silently
	// produce garbage output.
	if err := RunRestore(encPath, filepath.Join(t.TempDir(), "c2"), filepath.Join(t.TempDir(), "d2"), ""); err == nil {
		t.Fatal("expected RunRestore to fail without a password for an encrypted backup")
	}
}

func TestRunBackupComplianceBlocksWithoutPassword(t *testing.T) {
	configDir, dataDir, backupDir := setupDirs(t)

	err := RunBackup(configDir, dataDir, backupDir, "", Options{ComplianceRequired: true})
	if err == nil {
		t.Fatal("expected compliance mode to block an unencrypted backup")
	}
	if err != ErrCompliancePasswordRequired {
		t.Fatalf("expected ErrCompliancePasswordRequired, got: %v", err)
	}

	entries, _ := os.ReadDir(backupDir)
	if len(entries) != 0 {
		t.Fatalf("expected no files written when compliance blocks the backup, found %d", len(entries))
	}
}

func TestRunDailyBackupEncrypted(t *testing.T) {
	configDir, dataDir, backupDir := setupDirs(t)
	opts := Options{Password: "daily-secret"}

	if err := RunDailyBackup(configDir, dataDir, backupDir, opts); err != nil {
		t.Fatalf("RunDailyBackup failed: %v", err)
	}

	dailyPath := filepath.Join(backupDir, "caslink-daily.tar.gz.enc")
	if err := Verify(dailyPath, opts.Password); err != nil {
		t.Fatalf("Verify daily incremental failed: %v", err)
	}
}

// TestVerifyMissingFile covers the "file exists" check: a path that was
// never written must fail with a not-found error, not panic or pass.
func TestVerifyMissingFile(t *testing.T) {
	dir := t.TempDir()
	if err := Verify(filepath.Join(dir, "nope.tar.gz"), ""); err == nil {
		t.Fatal("expected Verify to fail for a missing file")
	}
}

// TestVerifyEmptyFile covers the "size > 0" check.
func TestVerifyEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.tar.gz")
	if err := os.WriteFile(path, nil, 0o640); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	if err := os.WriteFile(manifestPath(path), []byte("{}"), 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := Verify(path, ""); err == nil {
		t.Fatal("expected Verify to fail for a zero-size archive")
	}
}

// TestVerifyManifestMissing covers the "manifest readable" check: a valid
// archive with no sidecar manifest must fail Verify rather than skip the
// checksum/content checks silently.
func TestVerifyManifestMissing(t *testing.T) {
	configDir, dataDir, backupDir := setupDirs(t)
	if err := RunBackup(configDir, dataDir, backupDir, "", Options{}); err != nil {
		t.Fatalf("RunBackup failed: %v", err)
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	var archive string
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			archive = filepath.Join(backupDir, e.Name())
		}
	}
	if archive == "" {
		t.Fatal("no archive found")
	}
	if err := os.Remove(manifestPath(archive)); err != nil {
		t.Fatalf("remove manifest: %v", err)
	}
	if err := Verify(archive, ""); err == nil {
		t.Fatal("expected Verify to fail when the manifest sidecar is missing")
	}
}

// TestVerifyChecksumMismatch covers the checksum check: corrupting the
// archive bytes without updating the manifest must be detected and must
// fail closed before any content-extraction attempt.
func TestVerifyChecksumMismatch(t *testing.T) {
	configDir, dataDir, backupDir := setupDirs(t)
	dst := filepath.Join(backupDir, "checksum_test.tar.gz")
	if err := RunBackup(configDir, dataDir, backupDir, dst, Options{}); err != nil {
		t.Fatalf("RunBackup failed: %v", err)
	}

	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	// Flip a single byte so the file no longer matches its recorded checksum.
	raw[len(raw)-1] ^= 0xFF
	if err := os.WriteFile(dst, raw, 0o640); err != nil {
		t.Fatalf("rewrite archive: %v", err)
	}

	err = Verify(dst, "")
	if err == nil {
		t.Fatal("expected Verify to fail on checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected a checksum mismatch error, got: %v", err)
	}
}

// TestVerifyCorruptContentPassesChecksumFailsExtraction covers the content
// extraction check independently of the checksum check: when the manifest
// checksum is (re)computed to match corrupted bytes, Verify must still fail
// because the corrupted bytes are not a readable gzip/tar stream.
func TestVerifyCorruptContentPassesChecksumFailsExtraction(t *testing.T) {
	configDir, dataDir, backupDir := setupDirs(t)
	dst := filepath.Join(backupDir, "corrupt_content.tar.gz")
	if err := RunBackup(configDir, dataDir, backupDir, dst, Options{}); err != nil {
		t.Fatalf("RunBackup failed: %v", err)
	}

	garbage := []byte("this is not a valid gzip archive at all")
	if err := os.WriteFile(dst, garbage, 0o640); err != nil {
		t.Fatalf("rewrite archive with garbage: %v", err)
	}
	m, err := readManifest(dst)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	m.Checksum = sha256Hex(garbage)
	if err := writeManifest(dst, m); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	err = Verify(dst, "")
	if err == nil {
		t.Fatal("expected Verify to fail on unreadable archive content")
	}
	if strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected the checksum check to pass and extraction to fail, got: %v", err)
	}
}

// TestVerifyEncryptedRequiresPassword covers the decrypt-test check when no
// password is supplied at all (distinct from a wrong password).
func TestVerifyEncryptedRequiresPassword(t *testing.T) {
	configDir, dataDir, backupDir := setupDirs(t)
	opts := Options{Password: "some-password"}
	if err := RunBackup(configDir, dataDir, backupDir, "", opts); err != nil {
		t.Fatalf("RunBackup failed: %v", err)
	}
	entries, _ := os.ReadDir(backupDir)
	var encPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".enc") {
			encPath = filepath.Join(backupDir, e.Name())
		}
	}
	if encPath == "" {
		t.Fatal("no encrypted archive found")
	}
	if err := Verify(encPath, ""); err == nil {
		t.Fatal("expected Verify to fail for an encrypted archive with no password")
	}
}

// TestRunBackupEmptyContentsFailsVerification covers the "archive contains
// no entries" check: if both source directories are absent, createArchive
// still produces a non-empty gzip stream (framing only), and Verify must
// reject it as content-free rather than accept an empty backup.
func TestRunBackupEmptyContentsFailsVerification(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "no-such-config")
	dataDir := filepath.Join(root, "no-such-data")
	backupDir := filepath.Join(root, "backups")

	err := RunBackup(configDir, dataDir, backupDir, "", Options{})
	if err == nil {
		t.Fatal("expected RunBackup to fail when there is no content to archive")
	}

	entries, readErr := os.ReadDir(backupDir)
	if readErr != nil {
		t.Fatalf("read backup dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected the failed backup and its manifest to be cleaned up, found %d entries", len(entries))
	}
}

// TestRunBackupFailureDoesNotAffectExistingBackups is the regression test for
// AI.md PART 22's "never delete existing valid backups if the new backup
// fails ANY verification check". A prior valid backup must survive untouched
// when a later, unrelated backup attempt fails verification.
func TestRunBackupFailureDoesNotAffectExistingBackups(t *testing.T) {
	configDir, dataDir, backupDir := setupDirs(t)

	goodDst := filepath.Join(backupDir, "good_backup.tar.gz")
	if err := RunBackup(configDir, dataDir, backupDir, goodDst, Options{}); err != nil {
		t.Fatalf("RunBackup (good) failed: %v", err)
	}
	if err := Verify(goodDst, ""); err != nil {
		t.Fatalf("expected the good backup to verify before the failing attempt: %v", err)
	}

	// A second, distinctly-named attempt with no source content to archive
	// must fail its own verification without touching the earlier backup.
	badDst := filepath.Join(backupDir, "bad_backup.tar.gz")
	emptyConfig := filepath.Join(t.TempDir(), "missing-config")
	emptyData := filepath.Join(t.TempDir(), "missing-data")
	if err := RunBackup(emptyConfig, emptyData, backupDir, badDst, Options{}); err == nil {
		t.Fatal("expected the second RunBackup call to fail verification")
	}

	if _, err := os.Stat(badDst); err == nil {
		t.Fatal("expected the failed backup file to be removed")
	}
	if err := Verify(goodDst, ""); err != nil {
		t.Fatalf("expected the earlier good backup to remain valid, got: %v", err)
	}
}

// buildMaliciousTarGz constructs an in-memory tar.gz whose single entry name
// attempts a path-traversal escape, for RunRestore's traversal-rejection test.
func buildMaliciousTarGz(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	content := []byte("pwned")
	hdr := &tar.Header{
		Name: "config/../../../etc/pwned",
		Mode: 0o640,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buf.Bytes()
}

// TestRunRestorePathTraversalRejected covers RunRestore's traversal guard:
// an archive entry containing ".." must be rejected outright.
func TestRunRestorePathTraversalRejected(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "malicious.tar.gz")
	if err := os.WriteFile(src, buildMaliciousTarGz(t), 0o640); err != nil {
		t.Fatalf("write malicious archive: %v", err)
	}

	configDir := filepath.Join(root, "config")
	dataDir := filepath.Join(root, "data")
	err := RunRestore(src, configDir, dataDir, "")
	if err == nil {
		t.Fatal("expected RunRestore to reject a path-traversal entry")
	}
	if !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("expected a traversal-specific error, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "etc", "pwned")); statErr == nil {
		t.Fatal("traversal entry must not have been written outside the target dirs")
	}
}

// TestRunRestoreWrongPasswordFails covers restore-time decryption failure:
// a wrong password must fail cleanly (AEAD auth failure) with no output
// files written, rather than silently extracting garbage.
func TestRunRestoreWrongPasswordFails(t *testing.T) {
	configDir, dataDir, backupDir := setupDirs(t)
	opts := Options{Password: "correct-password"}
	dst := filepath.Join(backupDir, "restore_wrong_pw.tar.gz")
	if err := RunBackup(configDir, dataDir, backupDir, dst, opts); err != nil {
		t.Fatalf("RunBackup failed: %v", err)
	}
	encPath := dst + ".enc"

	restoreConfig := filepath.Join(t.TempDir(), "config")
	restoreData := filepath.Join(t.TempDir(), "data")
	if err := RunRestore(encPath, restoreConfig, restoreData, "wrong-password"); err == nil {
		t.Fatal("expected RunRestore to fail with a wrong password")
	}
	if _, err := os.Stat(restoreConfig); err == nil {
		t.Fatal("expected no files to be written to configDir on decrypt failure")
	}
}
