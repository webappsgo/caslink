package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// TestCheckForUpdate_ContextCanceledPropagatesError verifies that a
// request-level failure surfaces as an error rather than being silently
// swallowed.
func TestCheckForUpdate_ContextCanceledPropagatesError(t *testing.T) {
	for _, branch := range []string{"stable", "", "beta", "daily", "unknown"} {
		t.Run(branch, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			rel, err := CheckForUpdate(ctx, "1.0.0", branch)
			if err == nil {
				t.Fatalf("expected error for canceled context, got nil (release=%+v)", rel)
			}
			if rel != nil {
				t.Errorf("expected nil release alongside an error, got %+v", rel)
			}
		})
	}
}

// withMockGitHubAPI points CheckForUpdate at an httptest.Server for the
// duration of the test, via the apiBaseURL injection seam, restoring the
// real GitHub host on cleanup.
func withMockGitHubAPI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	prev := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = prev })
}

// TestCheckForUpdate_NotFoundReturnsNilRelease verifies the "no releases
// yet" 404 branch reports no update available, not an error, per PART 23
// ("ALWAYS use HTTP 404 from GitHub API as 'no update available'").
func TestCheckForUpdate_NotFoundReturnsNilRelease(t *testing.T) {
	withMockGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	rel, err := CheckForUpdate(t.Context(), "1.0.0", "stable")
	if err != nil {
		t.Fatalf("CheckForUpdate() error = %v, want nil", err)
	}
	if rel != nil {
		t.Errorf("CheckForUpdate() release = %+v, want nil", rel)
	}
}

// TestCheckForUpdate_NonOKNonNotFoundStatusErrors verifies a GitHub API
// error status other than 404 surfaces as an error.
func TestCheckForUpdate_NonOKNonNotFoundStatusErrors(t *testing.T) {
	withMockGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	rel, err := CheckForUpdate(t.Context(), "1.0.0", "stable")
	if err == nil {
		t.Fatal("CheckForUpdate() expected error for 500 status, got nil")
	}
	if rel != nil {
		t.Errorf("CheckForUpdate() release = %+v, want nil alongside an error", rel)
	}
}

// TestCheckForUpdate_StableAlreadyUpToDate verifies the tag-comparison
// short-circuit: when the latest release's tag matches the current version
// (with or without a "v" prefix), CheckForUpdate reports no update.
func TestCheckForUpdate_StableAlreadyUpToDate(t *testing.T) {
	for _, tag := range []string{"1.0.0", "v1.0.0"} {
		t.Run(tag, func(t *testing.T) {
			withMockGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(Release{TagName: tag})
			})
			rel, err := CheckForUpdate(t.Context(), "1.0.0", "stable")
			if err != nil {
				t.Fatalf("CheckForUpdate() error = %v, want nil", err)
			}
			if rel != nil {
				t.Errorf("CheckForUpdate() release = %+v, want nil (already up to date)", rel)
			}
		})
	}
}

// TestCheckForUpdate_StableNewerReleaseReturned verifies the success path:
// a differently-tagged latest release is decoded and returned.
func TestCheckForUpdate_StableNewerReleaseReturned(t *testing.T) {
	withMockGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{TagName: "v2.0.0"})
	})
	rel, err := CheckForUpdate(t.Context(), "1.0.0", "stable")
	if err != nil {
		t.Fatalf("CheckForUpdate() error = %v, want nil", err)
	}
	if rel == nil || rel.TagName != "v2.0.0" {
		t.Errorf("CheckForUpdate() release = %+v, want TagName v2.0.0", rel)
	}
}

// TestCheckForUpdate_StableMalformedJSONErrors verifies the stable-branch
// decode-failure path.
func TestCheckForUpdate_StableMalformedJSONErrors(t *testing.T) {
	withMockGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	if _, err := CheckForUpdate(t.Context(), "1.0.0", "stable"); err == nil {
		t.Fatal("CheckForUpdate() expected decode error, got nil")
	}
}

// TestCheckForUpdate_NonStableMalformedJSONErrors verifies the non-stable
// (releases-list) branch's decode-failure path.
func TestCheckForUpdate_NonStableMalformedJSONErrors(t *testing.T) {
	withMockGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	if _, err := CheckForUpdate(t.Context(), "1.0.0", "beta"); err == nil {
		t.Fatal("CheckForUpdate() expected decode error, got nil")
	}
}

// TestCheckForUpdate_NonStableMatchesFirstQualifyingRelease exercises the
// per-branch release-matching loop over the /releases list: it must skip
// releases that don't match the requested branch or that equal the current
// version, and return the first one that does qualify.
func TestCheckForUpdate_NonStableMatchesFirstQualifyingRelease(t *testing.T) {
	withMockGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Release{
			{TagName: "v1.0.0"},                    // matches branch but equals current version
			{TagName: "20260730060000-beta"},        // qualifies
			{TagName: "20260601060000-beta"},        // would also qualify, but not first
		})
	})
	rel, err := CheckForUpdate(t.Context(), "1.0.0", "beta")
	if err != nil {
		t.Fatalf("CheckForUpdate() error = %v, want nil", err)
	}
	if rel == nil || rel.TagName != "20260730060000-beta" {
		t.Errorf("CheckForUpdate() release = %+v, want TagName 20260730060000-beta", rel)
	}
}

// TestCheckForUpdate_NonStableNoQualifyingReleaseReturnsNil verifies that
// when nothing in the releases list matches the branch, CheckForUpdate
// reports no update rather than an error.
func TestCheckForUpdate_NonStableNoQualifyingReleaseReturnsNil(t *testing.T) {
	withMockGitHubAPI(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Release{
			{TagName: "v1.0.0", Prerelease: false},
		})
	})
	rel, err := CheckForUpdate(t.Context(), "1.0.0", "beta")
	if err != nil {
		t.Fatalf("CheckForUpdate() error = %v, want nil", err)
	}
	if rel != nil {
		t.Errorf("CheckForUpdate() release = %+v, want nil (no qualifying release)", rel)
	}
}

// TestDoUpdateFor_NoMatchingAsset verifies the release-has-no-asset error
// path: if the release's asset list doesn't contain assetName, DoUpdateFor
// must fail before attempting any network I/O.
func TestDoUpdateFor_NoMatchingAsset(t *testing.T) {
	release := &Release{
		TagName: "v1.2.3",
		Assets: []Asset{
			{Name: "some-other-binary", BrowserDownloadURL: "http://example.invalid/x"},
		},
	}
	err := DoUpdateFor(t.Context(), release, "caslink-linux-amd64")
	if err == nil {
		t.Fatal("expected error when no asset matches assetName")
	}
	if !strings.Contains(err.Error(), "no binary found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDoUpdate_NoMatchingAsset exercises the DoUpdate wrapper (which
// delegates to DoUpdateFor with GetBinaryName()); an empty asset list must
// fail the same way as DoUpdateFor with a non-matching asset.
func TestDoUpdate_NoMatchingAsset(t *testing.T) {
	release := &Release{TagName: "v1.2.3"}
	err := DoUpdate(t.Context(), release)
	if err == nil {
		t.Fatal("expected error when release has no assets")
	}
	if !strings.Contains(err.Error(), "no binary found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDoUpdateFor_DownloadRequestCreationError verifies a malformed
// download URL (invalid percent-encoding) is rejected by
// http.NewRequestWithContext and surfaced as an error.
func TestDoUpdateFor_DownloadRequestCreationError(t *testing.T) {
	release := &Release{
		Assets: []Asset{
			{Name: "caslink-linux-amd64", BrowserDownloadURL: "http://example.com/%zz"},
		},
	}
	err := DoUpdateFor(t.Context(), release, "caslink-linux-amd64")
	if err == nil {
		t.Fatal("expected error for malformed download URL")
	}
}

// TestDoUpdateFor_DownloadNetworkError verifies a connection-level failure
// while downloading the binary asset (as opposed to a checksum fetch
// failure, covered elsewhere) is propagated as an error.
func TestDoUpdateFor_DownloadNetworkError(t *testing.T) {
	release := &Release{
		Assets: []Asset{
			// Port 0 is never reachable.
			{Name: "caslink-linux-amd64", BrowserDownloadURL: "http://127.0.0.1:0/binary"},
			// A checksum asset must be present or DoUpdateFor refuses before
			// attempting the download; the download error is what we test here.
			{Name: "checksums.txt", BrowserDownloadURL: "http://127.0.0.1:0/checksums.txt"},
		},
	}
	err := DoUpdateFor(t.Context(), release, "caslink-linux-amd64")
	if err == nil {
		t.Fatal("expected network error for unreachable download URL")
	}
}

// TestDoUpdateFor_NoChecksumAssetRefuses verifies the mandatory-checksum
// contract (PART 23): a release that ships a binary but no checksums asset is
// refused before any download, never installed unverified.
func TestDoUpdateFor_NoChecksumAssetRefuses(t *testing.T) {
	release := &Release{
		Assets: []Asset{
			{Name: "caslink-linux-amd64", BrowserDownloadURL: "http://127.0.0.1:0/binary"},
		},
	}
	err := DoUpdateFor(t.Context(), release, "caslink-linux-amd64")
	if err == nil {
		t.Fatal("expected refusal when no checksum asset is present")
	}
	if !strings.Contains(err.Error(), "refusing unverified update") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDoUpdateFor_DownloadNon200Status verifies a non-200 HTTP status while
// fetching the binary asset is treated as a download failure, not silently
// accepted.
func TestDoUpdateFor_DownloadNon200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	release := &Release{
		Assets: []Asset{
			{Name: "caslink-linux-amd64", BrowserDownloadURL: srv.URL},
			// Present so DoUpdateFor proceeds to the download step; the binary
			// fetch returns 404 before the checksum is ever fetched.
			{Name: "checksums.txt", BrowserDownloadURL: srv.URL},
		},
	}
	err := DoUpdateFor(t.Context(), release, "caslink-linux-amd64")
	if err == nil {
		t.Fatal("expected error for non-200 download status")
	}
	if !strings.Contains(err.Error(), "download failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDoUpdateFor_ChecksumMismatchStopsBeforeReplace exercises DoUpdateFor's
// full download-then-verify pipeline against mock HTTP servers for both the
// binary asset and the checksums.txt asset. A checksum mismatch must return
// an error and MUST NOT reach replaceBinary/os.Executable() — which would
// attempt to overwrite the currently-running test binary — so this is the
// deepest point in DoUpdateFor safely reachable from a unit test.
func TestDoUpdateFor_ChecksumMismatchStopsBeforeReplace(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake binary contents"))
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000  caslink-linux-amd64\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	release := &Release{
		Assets: []Asset{
			{Name: "caslink-linux-amd64", BrowserDownloadURL: srv.URL + "/binary"},
			{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/checksums.txt"},
		},
	}
	err := DoUpdateFor(t.Context(), release, "caslink-linux-amd64")
	if err == nil {
		t.Fatal("expected checksum verification failure")
	}
	if !strings.Contains(err.Error(), "checksum verification failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDoUpdateFor_ChecksumAssetRecognizedBySHA256SUMSName verifies the
// alternate checksum-asset filename ("SHA256SUMS", used by some release
// tooling instead of "checksums.txt") is also picked up.
func TestDoUpdateFor_ChecksumAssetRecognizedBySHA256SUMSName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake binary contents"))
	})
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000  caslink-linux-amd64\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	release := &Release{
		Assets: []Asset{
			{Name: "caslink-linux-amd64", BrowserDownloadURL: srv.URL + "/binary"},
			{Name: "SHA256SUMS", BrowserDownloadURL: srv.URL + "/SHA256SUMS"},
		},
	}
	err := DoUpdateFor(t.Context(), release, "caslink-linux-amd64")
	if err == nil {
		t.Fatal("expected checksum verification failure via SHA256SUMS asset")
	}
	if !strings.Contains(err.Error(), "checksum verification failed") {
		t.Errorf("unexpected error (want checksum path to have been used): %v", err)
	}
}
