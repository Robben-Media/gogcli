package skillpack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

// State tracks last-written pack hashes so we can detect local edits.
type State struct {
	Version int                  `json:"version"`
	Paths   map[string]PathState `json:"paths"`
	Updated time.Time            `json:"updated_at"`
}

// PathState is the last install record for one real skill directory.
type PathState struct {
	Skill       string    `json:"skill"`
	ContentHash string    `json:"content_hash"`
	PackVersion string    `json:"pack_version,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func statePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}

	return filepath.Join(dir, "skill-pack-state.json"), nil
}

// LoadState reads skill pack state (empty if missing).
func LoadState() (State, error) {
	path, err := statePath()
	if err != nil {
		return State{}, err
	}

	//nolint:gosec // path is under user config dir
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{Version: 1, Paths: map[string]PathState{}}, nil
		}

		return State{}, fmt.Errorf("read skill pack state: %w", err)
	}

	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, fmt.Errorf("parse skill pack state: %w", err)
	}

	if s.Paths == nil {
		s.Paths = map[string]PathState{}
	}

	if s.Version == 0 {
		s.Version = 1
	}

	return s, nil
}

// SaveState writes skill pack state atomically.
func SaveState(s State) error {
	if _, err := config.EnsureDir(); err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	path, err := statePath()
	if err != nil {
		return err
	}

	s.Version = 1
	s.Updated = time.Now().UTC()

	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode skill pack state: %w", err)
	}

	b = append(b, '\n')
	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write skill pack state: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit skill pack state: %w", err)
	}

	return nil
}

// RecordWrite updates state after a successful skill write.
func RecordWrite(s *State, realPath, skillName, contentHash, packVersion string) {
	if s.Paths == nil {
		s.Paths = map[string]PathState{}
	}

	s.Paths[realPath] = PathState{
		Skill:       skillName,
		ContentHash: contentHash,
		PackVersion: packVersion,
		UpdatedAt:   time.Now().UTC(),
	}
}
