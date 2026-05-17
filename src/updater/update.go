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
	githubOrg  = "casapps"
	githubRepo = "caslink"
)

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
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", githubOrg, githubRepo)
	default:
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", githubOrg, githubRepo)
	}

	client := &http.Client{Timeout: 30 * time.Second}
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

// DoUpdate downloads and installs the update binary.
func DoUpdate(ctx context.Context, release *Release) error {
	assetName := GetBinaryName()
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

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to download: %w", err)
	}
	tmpFile.Close()

	// Verify checksum if available
	if checksumURL != "" {
		if err := verifyChecksumFromURL(ctx, client, tmpPath, assetName, checksumURL); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
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

// GetBinaryName returns the expected release asset name for this platform.
func GetBinaryName() string {
	name := fmt.Sprintf("caslink-%s-%s", runtime.GOOS, runtime.GOARCH)
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
	if expectedHash == "" {
		return nil // No checksum entry found; skip verification
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
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}
	return nil
}
