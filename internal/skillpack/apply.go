package skillpack

import (
	"fmt"
	"os"
	"path/filepath"

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
	Force       bool // overwrite dirty
	Install     bool // create missing under InstallRoot
	InstallRoot string
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
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	managed := manifest.ManagedFiles
	skillFilter := buildSkillFilter(manifest.Skills, opts.OnlySkills)

	state, err := LoadState()
	if err != nil {
		return nil, err
	}

	names := filteredNames(manifest.Skills, skillFilter)

	installs, err := Discover(names, opts.Discover)
	if err != nil {
		return nil, err
	}

	bySkill := groupInstalls(installs)

	var results []ApplyResult
	stateDirty := false

	for _, name := range names {
		packHash, err := HashManagedFromFS(skills.FS(), "pack/"+name, managed)
		if err != nil {
			return nil, err
		}

		refs := bySkill[name]

		var installResults []ApplyResult
		var installStateDirty bool

		refs, installResults, installStateDirty, err = maybeInstallCanonical(name, refs, packHash, managed, opts, &state)
		if err != nil {
			return results, err
		}

		results = append(results, installResults...)
		if installStateDirty {
			stateDirty = true
		}

		if len(refs) == 0 {
			results = append(results, ApplyResult{
				Skill:  name,
				Action: "skipped_not_installed",
				Detail: "run: gog skills install",
			})

			continue
		}

		pathResults, pathStateDirty, err := applyRefs(name, refs, packHash, managed, opts, &state)
		if err != nil {
			return results, err
		}

		results = append(results, pathResults...)
		if pathStateDirty {
			stateDirty = true
		}
	}

	if stateDirty {
		if err := SaveState(state); err != nil {
			return results, err
		}
	}

	return results, nil
}

func buildSkillFilter(all, only []string) map[string]struct{} {
	skillFilter := map[string]struct{}{}
	if len(only) > 0 {
		for _, s := range only {
			skillFilter[s] = struct{}{}
		}

		return skillFilter
	}

	for _, s := range all {
		skillFilter[s] = struct{}{}
	}

	return skillFilter
}

func filteredNames(all []string, filter map[string]struct{}) []string {
	var names []string
	for _, s := range all {
		if _, ok := filter[s]; ok {
			names = append(names, s)
		}
	}

	return names
}

func groupInstalls(installs []InstallRef) map[string][]InstallRef {
	bySkill := map[string][]InstallRef{}
	for _, inst := range installs {
		bySkill[inst.Skill] = append(bySkill[inst.Skill], inst)
	}

	return bySkill
}

func maybeInstallCanonical(
	name string,
	refs []InstallRef,
	packHash string,
	managed []string,
	opts UpdateOptions,
	state *State,
) ([]InstallRef, []ApplyResult, bool, error) {
	if !opts.Install {
		return refs, nil, false, nil
	}

	root := opts.InstallRoot
	if root == "" {
		home := opts.Discover.HomeDir
		if home == "" {
			h, err := os.UserHomeDir()
			if err != nil {
				return refs, nil, false, fmt.Errorf("resolve home dir: %w", err)
			}

			home = h
		}

		root = filepath.Join(home, ".agents", "skills")
	}

	dest := filepath.Join(root, name)
	if hasCanonical(refs, dest) {
		return refs, nil, false, nil
	}

	if err := writeSkill(dest, name, managed); err != nil {
		return refs, nil, false, err
	}

	_ = linkIntoOtherRoots(name, dest, opts.Discover)

	destReal := dest
	if resolved, err := filepath.EvalSymlinks(dest); err == nil {
		destReal = filepath.Clean(resolved)
	}

	RecordWrite(state, destReal, name, packHash, opts.PackVersion)

	result := ApplyResult{
		Skill:    name,
		Path:     dest,
		RealPath: destReal,
		Action:   "installed",
	}

	refs = append(refs, InstallRef{Skill: name, Path: dest, RealPath: destReal})

	return refs, []ApplyResult{result}, true, nil
}

func hasCanonical(refs []InstallRef, dest string) bool {
	destClean := filepath.Clean(dest)
	for _, ref := range refs {
		if filepath.Clean(ref.RealPath) == destClean {
			return true
		}

		if resolved, err := filepath.EvalSymlinks(dest); err == nil && filepath.Clean(resolved) == filepath.Clean(ref.RealPath) {
			return true
		}
	}

	return false
}

func applyRefs(
	name string,
	refs []InstallRef,
	packHash string,
	managed []string,
	opts UpdateOptions,
	state *State,
) ([]ApplyResult, bool, error) {
	var results []ApplyResult
	stateDirty := false

	for _, ref := range refs {
		diskHash, err := HashManagedFiles(ref.RealPath, managed)
		if err != nil {
			return results, stateDirty, err
		}

		stateHash := ""
		if ps, ok := state.Paths[ref.RealPath]; ok {
			stateHash = ps.ContentHash
		}

		kind := classify(diskHash, packHash, stateHash)

		switch kind {
		case StatusCurrent:
			if stateHash == "" {
				RecordWrite(state, ref.RealPath, name, packHash, opts.PackVersion)
				stateDirty = true
			}

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
				return results, stateDirty, err
			}

			RecordWrite(state, ref.RealPath, name, packHash, opts.PackVersion)
			stateDirty = true

			results = append(results, ApplyResult{
				Skill:    name,
				Path:     ref.Path,
				RealPath: ref.RealPath,
				Action:   "updated",
			})
		}
	}

	return results, stateDirty, nil
}

func writeSkill(destDir, skill string, managed []string) error {
	//nolint:gosec // skill dirs are user agent skill roots
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("mkdir %s: %w", destDir, err)
	}

	for _, rel := range managed {
		b, err := skills.ReadManagedFile(skill, rel)
		if err != nil {
			return fmt.Errorf("read pack file: %w", err)
		}

		target := filepath.Join(destDir, rel)

		//nolint:gosec // parent of managed skill file
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("mkdir parent: %w", err)
		}

		tmp := target + ".tmp"
		if err := os.WriteFile(tmp, b, 0o600); err != nil {
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
		h, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}

		home = h
	}

	primaryReal := primary
	if resolved, err := filepath.EvalSymlinks(primary); err == nil {
		primaryReal = filepath.Clean(resolved)
	}

	for _, rel := range DefaultHomeSkillRoots {
		root := filepath.Join(home, rel)

		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			continue
		}

		linkPath := filepath.Join(root, skill)
		if linkPath == primary || filepath.Clean(linkPath) == primaryReal {
			continue
		}

		if _, err := os.Lstat(linkPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			// Best-effort: ignore unexpected lstat errors for optional roots.
			continue
		}

		relLink, err := filepath.Rel(root, primaryReal)
		if err != nil {
			relLink = primaryReal
		}

		// Symlinks are best-effort (Windows privilege errors, etc.).
		_ = os.Symlink(relLink, linkPath)
	}

	return nil
}
