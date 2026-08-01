package backup

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestManifestPath covers the sidecar naming convention.
func TestManifestPath(t *testing.T) {
	got := manifestPath("/backups/caslink_backup_2026-01-01.tar.gz")
	want := "/backups/caslink_backup_2026-01-01.tar.gz.manifest.json"
	if got != want {
		t.Fatalf("manifestPath() = %q, want %q", got, want)
	}
}

// TestWriteReadManifestRoundTrip covers the write/read sidecar pair: every
// field written must come back unchanged.
func TestWriteReadManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive.tar.gz")

	want := Manifest{
		Version:          "1.0.0",
		CreatedAt:        "2026-01-01T00:00:00Z",
		CreatedBy:        "administrator",
		AppVersion:       "1.2.3",
		Contents:         []string{"config/", "data/"},
		Encrypted:        true,
		EncryptionMethod: EncryptionMethod,
		Checksum:         "deadbeef",
	}
	if err := writeManifest(archive, want); err != nil {
		t.Fatalf("writeManifest failed: %v", err)
	}

	got, err := readManifest(archive)
	if err != nil {
		t.Fatalf("readManifest failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readManifest() = %+v, want %+v", got, want)
	}

	// The sidecar must live next to the archive with the documented suffix.
	if _, err := os.Stat(archive + ".manifest.json"); err != nil {
		t.Fatalf("expected manifest sidecar on disk: %v", err)
	}
}

// TestReadManifestMissing covers reading a manifest that was never written.
func TestReadManifestMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := readManifest(filepath.Join(dir, "no-archive.tar.gz")); err == nil {
		t.Fatal("expected readManifest to fail for a missing sidecar")
	}
}

// TestReadManifestCorruptJSON covers reading a sidecar that exists but is
// not valid JSON.
func TestReadManifestCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(manifestPath(archive), []byte("{not json"), 0o640); err != nil {
		t.Fatalf("write corrupt manifest: %v", err)
	}
	if _, err := readManifest(archive); err == nil {
		t.Fatal("expected readManifest to fail on corrupt JSON")
	}
}

// TestManifestEncryptionMethodOmittedWhenUnencrypted covers the
// `omitempty` field: an unencrypted manifest must round-trip with an empty
// EncryptionMethod, never a stale/leftover value.
func TestManifestEncryptionMethodOmittedWhenUnencrypted(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive.tar.gz")

	m := Manifest{
		Version:   "1.0.0",
		Encrypted: false,
		Checksum:  "abc123",
	}
	if err := writeManifest(archive, m); err != nil {
		t.Fatalf("writeManifest failed: %v", err)
	}
	raw, err := os.ReadFile(manifestPath(archive))
	if err != nil {
		t.Fatalf("read manifest file: %v", err)
	}
	if got := string(raw); strings.Contains(got, `"encryption_method"`) {
		t.Fatalf("expected encryption_method to be omitted when unencrypted, got: %s", got)
	}
}
