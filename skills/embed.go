// Package skills embeds the gog companion skill pack shipped with this CLI.
package skills

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
)

//go:embed manifest.json pack/*/SKILL.md
var packFS embed.FS

var (
	errNoSkills      = errors.New("skill manifest has no skills")
	errReadManifest  = errors.New("read skill manifest")
	errParseManifest = errors.New("parse skill manifest")
	errReadPackFile  = errors.New("read pack file")
)

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
		return Manifest{}, fmt.Errorf("%w: %w", errReadManifest, err)
	}

	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", errParseManifest, err)
	}

	if len(m.Skills) == 0 {
		return Manifest{}, errNoSkills
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
		return nil, fmt.Errorf("%w %s: %w", errReadPackFile, path, err)
	}

	return b, nil
}
