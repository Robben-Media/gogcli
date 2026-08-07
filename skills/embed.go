// Package skills embeds the gog companion skill pack shipped with this CLI.
package skills

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
)

//go:embed manifest.json pack/*/SKILL.md
var packFS embed.FS

// Manifest describes which skills this CLI owns and which files are managed.
type Manifest struct {
	Name         string   `json:"name"`
	Skills       []string `json:"skills"`
	ManagedFiles []string `json:"managed_files"`
}

// LoadManifest reads the embedded pack manifest.
func LoadManifest() (Manifest, error) {
	b, err := packFS.ReadFile("manifest.json")
	if err != nil {
		return Manifest{}, fmt.Errorf("read skill manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse skill manifest: %w", err)
	}
	if len(m.Skills) == 0 {
		return Manifest{}, fmt.Errorf("skill manifest has no skills")
	}
	if len(m.ManagedFiles) == 0 {
		m.ManagedFiles = []string{"SKILL.md"}
	}
	return m, nil
}

// FS returns the embedded skill pack filesystem.
func FS() fs.FS {
	return packFS
}

// ReadManagedFile returns a managed file for a pack skill (e.g. "google-calendar", "SKILL.md").
func ReadManagedFile(skillName, fileName string) ([]byte, error) {
	path := fmt.Sprintf("pack/%s/%s", skillName, fileName)
	b, err := packFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pack file %s: %w", path, err)
	}
	return b, nil
}
