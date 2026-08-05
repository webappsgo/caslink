package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubOrg  = "webappsgo"
	githubRepo = "caslink"
)

// apiBaseURL is the GitHub API host. It is an unexported var (not a const)
// so tests can point CheckForUpdate at an httptest.Server instead of the
// real network, the same way DoUpdateFor is already mockable via its
// caller-supplied asset URLs.
var apiBaseURL = "https://api.github.com"

// httpClientTimeout is the CheckForUpdate request timeout, overridable by
// tests that need a shorter bound than the production default.
var httpClientTimeout = 30 * time.Second

// Release represents a GitHub release.
type Release struct {
	TagName    string  `json:"tag_name"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Asset is a release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckForUpdate checks GitHub releases for a newer version.
// Returns nil, nil when already up to date.
func CheckForUpdate(ctx context.Context, currentVersion, branch string) (*Release, error) {
	var apiURL string
	switch branch {
	case "stable", "":
		apiURL = fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBaseURL, githubOrg, githubRepo)
	default:
		apiURL = fmt.Sprintf("%s/repos/%s/%s/releases", apiBaseURL, githubOrg, githubRepo)
	}

	client := &http.Client{Timeout: httpClientTimeout}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil // No updates available
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	if branch == "stable" || branch == "" {
		var release Release
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return nil, err
		}
		if release.TagName == currentVersion || "v"+currentVersion == release.TagName {
			return nil, nil // Already up to date
		}
		return &release, nil
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	for _, r := range releases {
		if matchesBranch(r, branch) && r.TagName != currentVersion && "v"+currentVersion != r.TagName {
			return &r, nil
		}
	}
	return nil, nil
}

// DoUpdate downloads and installs the update for the server binary.
func DoUpdate(ctx context.Context, release *Release) error {
	return DoUpdateFor(ctx, release, GetBinaryName())
}

// DoUpdateFor downloads and installs the update for the named binary asset.
func DoUpdateFor(ctx context.Context, release *Release, assetName string) error {
	var downloadURL string
	var checksumURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
		}
		if asset.Name == "checksums.txt" || asset.Name == "SHA256SUMS" {
			checksumURL = asset.BrowserDownloadURL
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}
	// Checksum verification is MANDATORY (PART 23): refuse to replace the running
	// binary when the release ships no checksum asset — an update we cannot verify
	// is never installed.
	if checksumURL == "" {
		return fmt.Errorf("no checksum asset found for %s - refusing unverified update", assetName)
	}

	tmpFile, err := os.CreateTemp(os.TempDir(), "caslink-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: unexpected status %s", resp.Status)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to download: %w", err)
	}
	tmpFile.Close()

	// Checksum verification is mandatory — checksumURL is guaranteed non-empty above.
	if err := verifyChecksumFromURL(ctx, client, tmpPath, assetName, checksumURL); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			return fmt.Errorf("failed to set permissions: %w", err)
		}
	}

	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	currentPath, err = filepath.EvalSymlinks(currentPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	return replaceBinary(currentPath, tmpPath)
}

// GetBinaryName returns the expected release asset name for the server binary.
func GetBinaryName() string {
	return BinaryNameFor("caslink")
}

// GetClientBinaryName returns the expected release asset name for the CLI binary.
func GetClientBinaryName() string {
	return BinaryNameFor("caslink-cli")
}

// BinaryNameFor returns the platform-specific asset name for a given binary prefix.
func BinaryNameFor(prefix string) string {
	name := fmt.Sprintf("%s-%s-%s", prefix, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func matchesBranch(r Release, branch string) bool {
	switch branch {
	case "beta":
		return strings.HasSuffix(r.TagName, "-beta")
	case "daily":
		return len(r.TagName) == 14 && !strings.Contains(r.TagName, ".") && !strings.Contains(r.TagName, "-")
	default:
		return !r.Prerelease
	}
}

func verifyChecksumFromURL(ctx context.Context, client *http.Client, filePath, assetName, checksumURL string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", checksumURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var expectedHash string
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && (parts[1] == assetName || parts[1] == "./"+assetName || strings.HasSuffix(parts[1], assetName)) {
			expectedHash = parts[0]
			break
		}
	}
	// Fail closed: a missing checksum entry means we cannot verify the
	// downloaded binary, so refuse the update rather than replacing the
	// running executable with unverified content.
	if expectedHash == "" {
		return fmt.Errorf("no checksum entry found for %q; refusing unverified update", assetName)
	}

	return verifyFileChecksum(filePath, expectedHash)
}

func verifyFileChecksum(filePath, expectedHash string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", strings.ToLower(expectedHash), actualHash)
	}
	return nil
}
