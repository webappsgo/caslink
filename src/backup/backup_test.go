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
