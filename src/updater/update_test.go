package updater

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestMatchesBranch covers the channel-cumulativity rules from PART 23:
// stable = non-prerelease releases only, beta = tags ending "-beta", daily =
// bare 14-digit timestamp tags (no dots, no dashes).
func TestMatchesBranch(t *testing.T) {
	tests := []struct {
		name   string
		rel    Release
		branch string
		want   bool
	}{
		{"stable release matches stable", Release{TagName: "v1.2.3", Prerelease: false}, "stable", true},
		{"prerelease does not match stable", Release{TagName: "v1.2.3-rc1", Prerelease: true}, "stable", false},
		{"unrecognized branch falls back to non-prerelease check", Release{TagName: "v1.2.3", Prerelease: false}, "unknown", true},
		{"unrecognized branch rejects prerelease", Release{TagName: "v1.2.3", Prerelease: true}, "unknown", false},

		{"beta tag matches beta branch", Release{TagName: "20260730060000-beta"}, "beta", true},
		{"stable tag does not match beta branch", Release{TagName: "v1.2.3"}, "beta", false},
		{"daily tag does not match beta branch", Release{TagName: "20260730060000"}, "beta", false},

		{"14-digit timestamp matches daily", Release{TagName: "20260730060000"}, "daily", true},
		{"beta tag does not match daily (has dash)", Release{TagName: "20260730060000-beta"}, "daily", false},
		{"semver does not match daily (has dot)", Release{TagName: "1.2.3"}, "daily", false},
		{"wrong-length timestamp does not match daily", Release{TagName: "2026073006"}, "daily", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesBranch(tt.rel, tt.branch); got != tt.want {
				t.Errorf("matchesBranch(%+v, %q) = %v, want %v", tt.rel, tt.branch, got, tt.want)
			}
		})
	}
}

// TestBinaryNameFor verifies the platform-specific asset naming convention,
// including the Windows ".exe" suffix rule.
func TestBinaryNameFor(t *testing.T) {
	got := BinaryNameFor("caslink")
	want := "caslink-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if got != want {
		t.Errorf("BinaryNameFor(%q) = %q, want %q", "caslink", got, want)
	}
}

// TestGetBinaryName_ProjectIdentity verifies the server/CLI binary names use
// the frozen internal_name "caslink" per project rules, never a generic name.
func TestGetBinaryName_ProjectIdentity(t *testing.T) {
	if got := GetBinaryName(); got != BinaryNameFor("caslink") {
		t.Errorf("GetBinaryName() = %q, want %q", got, BinaryNameFor("caslink"))
	}
	if got := GetClientBinaryName(); got != BinaryNameFor("caslink-cli") {
		t.Errorf("GetClientBinaryName() = %q, want %q", got, BinaryNameFor("caslink-cli"))
	}
	if GetBinaryName() == GetClientBinaryName() {
		t.Error("server and CLI binary names must differ")
	}
}

// writeTempFile is a small helper to create a file with known content for
// checksum verification tests.
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

// TestVerifyFileChecksum_MatchAndMismatch exercises the SHA256 verification
// path directly, including case-insensitivity (hex digests are commonly
// upper or lower case depending on the tool that produced them).
func TestVerifyFileChecksum_MatchAndMismatch(t *testing.T) {
	path := writeTempFile(t, "hello world")
	// sha256("hello world")
	const correctHash = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	if err := verifyFileChecksum(path, correctHash); err != nil {
		t.Errorf("verifyFileChecksum with correct hash: %v", err)
	}
	if err := verifyFileChecksum(path, "B94D27B9934D3E08A52E52D7DA7DABFAC484EFE37A5380EE9088F7ACE2EFCDE9"); err != nil {
		t.Errorf("verifyFileChecksum should be case-insensitive: %v", err)
	}
	if err := verifyFileChecksum(path, "0000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Error("expected mismatch error for wrong hash, got nil")
	}
}

// TestVerifyFileChecksum_MissingFileErrors verifies a nonexistent file
// surfaces an error rather than a false-positive pass.
func TestVerifyFileChecksum_MissingFileErrors(t *testing.T) {
	err := verifyFileChecksum(filepath.Join(t.TempDir(), "nope"), "deadbeef")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// TestVerifyChecksumFromURL_Success verifies the checksum file line-parsing
// (standard "hash  filename" format) matches by exact name, "./name", and
// suffix, and that a correct binary passes verification end to end.
func TestVerifyChecksumFromURL_Success(t *testing.T) {
	const content = "hello world"
	const hash = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	path := writeTempFile(t, content)

	tests := []struct {
		name      string
		assetName string
		checksums string
	}{
		{"exact name match", "caslink-linux-amd64", hash + "  caslink-linux-amd64\n"},
		{"leading ./ match", "caslink-linux-amd64", hash + "  ./caslink-linux-amd64\n"},
		{"suffix match", "caslink-linux-amd64", hash + "  release/caslink-linux-amd64\n"},
		{"other entries ignored", "caslink-linux-amd64", "deadbeef  other-file\n" + hash + "  caslink-linux-amd64\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.checksums))
			}))
			defer srv.Close()

			client := &http.Client{}
			err := verifyChecksumFromURL(t.Context(), client, path, tt.assetName, srv.URL)
			if err != nil {
				t.Errorf("verifyChecksumFromURL: %v", err)
			}
		})
	}
}

// TestVerifyChecksumFromURL_NoEntryFailsClosed verifies the fail-closed
// contract from update.go: a checksums file that doesn't mention the asset
// must refuse the update, never silently skip verification.
func TestVerifyChecksumFromURL_NoEntryFailsClosed(t *testing.T) {
	path := writeTempFile(t, "hello world")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("deadbeef  some-other-binary\n"))
	}))
	defer srv.Close()

	client := &http.Client{}
	err := verifyChecksumFromURL(t.Context(), client, path, "caslink-linux-amd64", srv.URL)
	if err == nil {
		t.Fatal("expected error when checksum file has no matching entry, got nil")
	}
}

// TestVerifyChecksumFromURL_HashMismatchErrors verifies a checksum entry
// that disagrees with the actual file content is rejected.
func TestVerifyChecksumFromURL_HashMismatchErrors(t *testing.T) {
	path := writeTempFile(t, "hello world")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000  caslink-linux-amd64\n"))
	}))
	defer srv.Close()

	client := &http.Client{}
	err := verifyChecksumFromURL(t.Context(), client, path, "caslink-linux-amd64", srv.URL)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
}

// TestVerifyChecksumFromURL_UnreachableServerErrors verifies network
// failures fetching the checksum file propagate as errors.
func TestVerifyChecksumFromURL_UnreachableServerErrors(t *testing.T) {
	path := writeTempFile(t, "hello world")
	client := &http.Client{}
	// Port 0 URL is never reachable.
	err := verifyChecksumFromURL(t.Context(), client, path, "caslink-linux-amd64", "http://127.0.0.1:0/checksums.txt")
	if err == nil {
		t.Fatal("expected error for unreachable checksum URL, got nil")
	}
}
