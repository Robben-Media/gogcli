package skillpack

import (
	"fmt"
	"io/fs"
	"sort"

	"github.com/steipete/gogcli/skills"
)

// StatusKind classifies a pack skill install.
type StatusKind string

const (
	StatusNotInstalled StatusKind = "not_installed"
	StatusCurrent      StatusKind = "current"
	StatusOutdated     StatusKind = "outdated"
	StatusDirty        StatusKind = "dirty"
)

// SkillStatus is the evaluation of one install (or absence) for a pack skill.
type SkillStatus struct {
	Skill       string
	Kind        StatusKind
	Path        string
	RealPath    string
	PackHash    string
	DiskHash    string
	StateHash   string
	PackVersion string
}

// EvaluateAll returns status rows for every pack skill (installs + not_installed).
func EvaluateAll(packVersion string, opts DiscoverOptions) ([]SkillStatus, error) {
	manifest, err := skills.LoadManifest()
	if err != nil {
		return nil, err
	}
	state, err := LoadState()
	if err != nil {
		return nil, err
	}
	installs, err := Discover(manifest.Skills, opts)
	if err != nil {
		return nil, err
	}

	bySkill := map[string][]InstallRef{}
	for _, inst := range installs {
		bySkill[inst.Skill] = append(bySkill[inst.Skill], inst)
	}

	var out []SkillStatus
	for _, name := range manifest.Skills {
		packHash, err := HashManagedFromFS(skills.FS(), "pack/"+name, manifest.ManagedFiles)
		if err != nil {
			return nil, err
		}
		refs := bySkill[name]
		if len(refs) == 0 {
			out = append(out, SkillStatus{
				Skill:       name,
				Kind:        StatusNotInstalled,
				PackHash:    packHash,
				PackVersion: packVersion,
			})
			continue
		}
		for _, ref := range refs {
			st, err := evaluateInstall(name, ref, packHash, packVersion, state, manifest.ManagedFiles)
			if err != nil {
				return nil, err
			}
			out = append(out, st)
		}
	}
	return out, nil
}

func evaluateInstall(skill string, ref InstallRef, packHash, packVersion string, state State, managed []string) (SkillStatus, error) {
	diskHash, err := HashManagedFiles(ref.RealPath, managed)
	if err != nil {
		return SkillStatus{}, err
	}
	stateHash := ""
	if ps, ok := state.Paths[ref.RealPath]; ok {
		stateHash = ps.ContentHash
	}
	kind := classify(diskHash, packHash, stateHash)
	return SkillStatus{
		Skill:       skill,
		Kind:        kind,
		Path:        ref.Path,
		RealPath:    ref.RealPath,
		PackHash:    packHash,
		DiskHash:    diskHash,
		StateHash:   stateHash,
		PackVersion: packVersion,
	}, nil
}

func classify(diskHash, packHash, stateHash string) StatusKind {
	if diskHash == packHash {
		return StatusCurrent
	}
	// Last write matches disk but pack moved on → safe auto-update.
	if stateHash != "" && diskHash == stateHash {
		return StatusOutdated
	}
	// No state, or disk diverged from both pack and last write → treat as local edits.
	return StatusDirty
}

// PackSkillNames returns the embedded pack skill names.
func PackSkillNames() ([]string, error) {
	m, err := skills.LoadManifest()
	if err != nil {
		return nil, err
	}
	out := append([]string(nil), m.Skills...)
	sort.Strings(out)
	return out, nil
}

// PackHashFor returns the pack content hash for a skill.
func PackHashFor(skill string) (string, error) {
	m, err := skills.LoadManifest()
	if err != nil {
		return "", err
	}
	return HashManagedFromFS(skills.FS(), "pack/"+skill, m.ManagedFiles)
}

// Ensure skill name is in pack.
func IsPackSkill(name string) (bool, error) {
	m, err := skills.LoadManifest()
	if err != nil {
		return false, err
	}
	for _, s := range m.Skills {
		if s == name {
			return true, nil
		}
	}
	return false, nil
}

// ReadPackFile reads a managed file from the embed pack.
func ReadPackFile(skill, file string) ([]byte, error) {
	return skills.ReadManagedFile(skill, file)
}

// ManagedFiles returns managed relative paths from the manifest.
func ManagedFiles() ([]string, error) {
	m, err := skills.LoadManifest()
	if err != nil {
		return nil, err
	}
	return append([]string(nil), m.ManagedFiles...), nil
}

// VerifyPackPresent ensures every skill has SKILL.md in the embed FS.
func VerifyPackPresent() error {
	m, err := skills.LoadManifest()
	if err != nil {
		return err
	}
	for _, skill := range m.Skills {
		for _, f := range m.ManagedFiles {
			if _, err := fs.Stat(skills.FS(), pathJoin("pack/"+skill, f)); err != nil {
				return fmt.Errorf("pack missing %s/%s: %w", skill, f, err)
			}
		}
	}
	return nil
}
