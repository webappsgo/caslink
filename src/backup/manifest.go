package backup

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest describes a backup archive per AI.md PART 22. It is written as a
// sidecar file ({archive}.manifest.json) rather than embedded in the tar
// itself: the checksum covers the archive's own bytes, so an embedded
// self-referencing manifest would create a circular checksum dependency.
type Manifest struct {
	Version          string   `json:"version"`
	CreatedAt        string   `json:"created_at"`
	CreatedBy        string   `json:"created_by"`
	AppVersion       string   `json:"app_version"`
	Contents         []string `json:"contents"`
	Encrypted        bool     `json:"encrypted"`
	EncryptionMethod string   `json:"encryption_method,omitempty"`
	Checksum         string   `json:"checksum"`
}

// manifestPath returns the sidecar manifest path for an archive path.
func manifestPath(archivePath string) string {
	return archivePath + ".manifest.json"
}

// writeManifest marshals m as indented JSON and writes it next to archivePath.
func writeManifest(archivePath string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(manifestPath(archivePath), data, 0o640); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// readManifest reads and parses the sidecar manifest for archivePath.
func readManifest(archivePath string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(manifestPath(archivePath))
	if err != nil {
		return m, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse manifest: %w", err)
	}
	return m, nil
}
