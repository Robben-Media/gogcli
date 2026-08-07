package skillpack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steipete/gogcli/skills"
)

// ApplyResult is the outcome of one skill path update attempt.
type ApplyResult struct {
	Skill    string
	Path     string
	RealPath string
	Action   string // updated|skipped_current|skipped_dirty|installed|skipped_not_installed
	Detail   string
}

// UpdateOptions controls skill pack application.
type UpdateOptions struct {
	Discover    DiscoverOptions
	Force       bool   // overwrite dirty
	Install     bool   // create missing under InstallRoot
	InstallRoot string // default ~/.agents/skills
	PackVersion string
	// OnlySkills restricts to these names (empty = all pack skills).
	OnlySkills []string
}

// UpdateInstalled refreshes pack skills that already exist on disk (and optionally installs missing).
func UpdateInstalled(opts UpdateOptions) ([]ApplyResult, error) {
	if err := VerifyPackPresent(); err != nil {
		return nil, err
	}
	manifest, err := skills.LoadManifest()
	if err != nil {
		return nil, err
	}
	managed := manifest.ManagedFiles
	skillFilter := map[string]struct{}{}
	if len(opts.OnlySkills) > 0 {
		for _, s := range opts.OnlySkills {
			skillFilter[s] = struct{}{}
		}
	} else {
		for _, s := range manifest.Skills {
			skillFilter[s] = struct{}{}
		}
	}

	state, err := LoadState()
	if err != nil {
		return nil, err
	}

	var names []string
	for _, s := range manifest.Skills {
		if _, ok := skillFilter[s]; ok {
			names = append(names, s)
		}
	}

	installs, err := Discover(names, opts.Discover)
	if err != nil {
		return nil, err
	}
	bySkill := map[string][]InstallRef{}
	for _, inst := range installs {
		bySkill[inst.Skill] = append(bySkill[inst.Skill], inst)
	}

	var results []ApplyResult
	stateDirty := false

	for _, name := range names {
		packHash, err := HashManagedFromFS(skills.FS(), "pack/"+name, managed)
		if err != nil {
			return nil, err
		}
		refs := bySkill[name]
		if len(refs) == 0 {
			if opts.Install {
				root := opts.InstallRoot
				if root == "" {
					home := opts.Discover.HomeDir
					if home == "" {
						home, err = os.UserHomeDir()
						if err != nil {
							return nil, err
						}
					}
					root = filepath.Join(home, ".agents", "skills")
				}
				dest := filepath.Join(root, name)
				if err := writeSkill(dest, name, managed); err != nil {
					return results, err
				}
				if err := linkIntoOtherRoots(name, dest, opts.Discover); err != nil {
					return results, err
				}
				RecordWrite(&state, filepath.Clean(dest), name, packHash, opts.PackVersion)
				stateDirty = true
				results = append(results, ApplyResult{
					Skill:    name,
					Path:     dest,
					RealPath: dest,
					Action:   "installed",
				})
			} else {
				results = append(results, ApplyResult{
					Skill:  name,
					Action: "skipped_not_installed",
					Detail: "run: gog skills install",
				})
			}
			continue
		}

		for _, ref := range refs {
			diskHash, err := HashManagedFiles(ref.RealPath, managed)
			if err != nil {
				return results, err
			}
			stateHash := ""
			if ps, ok := state.Paths[ref.RealPath]; ok {
				stateHash = ps.ContentHash
			}
			kind := classify(diskHash, packHash, stateHash)
			switch kind {
			case StatusCurrent:
				results = append(results, ApplyResult{
					Skill:    name,
					Path:     ref.Path,
					RealPath: ref.RealPath,
					Action:   "skipped_current",
				})
			case StatusDirty:
				if !opts.Force {
					results = append(results, ApplyResult{
						Skill:    name,
						Path:     ref.Path,
						RealPath: ref.RealPath,
						Action:   "skipped_dirty",
						Detail:   "local edits; re-run with --overwrite-local or --force-skills",
					})
					continue
				}
				fallthrough
			case StatusOutdated:
				if err := writeSkill(ref.RealPath, name, managed); err != nil {
					return results, err
				}
				RecordWrite(&state, ref.RealPath, name, packHash, opts.PackVersion)
				stateDirty = true
				results = append(results, ApplyResult{
					Skill:    name,
					Path:     ref.Path,
					RealPath: ref.RealPath,
					Action:   "updated",
				})
			}
		}
	}

	if stateDirty {
		if err := SaveState(state); err != nil {
			return results, err
		}
	}
	return results, nil
}

func writeSkill(destDir, skill string, managed []string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", destDir, err)
	}
	// If destDir itself is a symlink, write through to the real path's files
	// without replacing the symlink.
	for _, rel := range managed {
		b, err := skills.ReadManagedFile(skill, rel)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		tmp := target + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		if err := os.Rename(tmp, target); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("commit %s: %w", target, err)
		}
	}
	return nil
}

// linkIntoOtherRoots creates symlinks from other existing agent skill roots to the primary install.
func linkIntoOtherRoots(skill, primary string, opts DiscoverOptions) error {
	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	primaryReal, err := filepath.EvalSymlinks(primary)
	if err != nil {
		primaryReal = primary
	}
	primaryReal = filepath.Clean(primaryReal)

	for _, rel := range DefaultHomeSkillRoots {
		root := filepath.Join(home, rel)
		// Only link into roots that already exist (user uses that agent).
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			continue
		}
		linkPath := filepath.Join(root, skill)
		if linkPath == primary || filepath.Clean(linkPath) == primaryReal {
			continue
		}
		if _, err := os.Lstat(linkPath); err == nil {
			continue // already present
		} else if !os.IsNotExist(err) {
			return err
		}
		// Prefer relative symlink when possible.
		relLink, err := filepath.Rel(root, primaryReal)
		if err != nil {
			relLink = primaryReal
		}
		if err := os.Symlink(relLink, linkPath); err != nil {
			// Non-fatal if symlink unsupported; primary install still valid.
			if !strings.Contains(err.Error(), "not supported") {
				return fmt.Errorf("symlink %s -> %s: %w", linkPath, relLink, err)
			}
		}
	}
	return nil
}
