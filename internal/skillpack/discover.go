package skillpack

import (
	"os"
	"path/filepath"
	"sort"
)

// DefaultHomeSkillRoots are relative to the user home directory.
var DefaultHomeSkillRoots = []string{
	".agents/skills",
	".claude/skills",
	".codex/skills",
	".cursor/skills",
	".grok/skills",
	".factory/skills",
	".config/agents/skills",
	".config/opencode/skills",
	".gemini/antigravity/skills",
}

// DefaultProjectSkillRoots are relative to a project working directory.
var DefaultProjectSkillRoots = []string{
	".agents/skills",
	".claude/skills",
	".codex/skills",
	".cursor/skills",
	".grok/skills",
	".factory/skills",
}

// InstallRef is one discovered install location for a pack skill.
type InstallRef struct {
	Skill    string
	Path     string // path as found (may be symlink)
	RealPath string // resolved path
	RootKind string // home|project
	Root     string // skills parent root
}

// DiscoverOptions controls where we look for pack skills.
type DiscoverOptions struct {
	HomeDir      string
	ProjectDir   string
	HomeRoots    []string
	ProjectRoots []string
}

// Discover finds existing install directories for the given skill names.
// Results are deduped by RealPath (first wins).
func Discover(skillNames []string, opts DiscoverOptions) ([]InstallRef, error) {
	home := opts.HomeDir
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		home = h
	}
	homeRoots := opts.HomeRoots
	if len(homeRoots) == 0 {
		homeRoots = DefaultHomeSkillRoots
	}
	projectRoots := opts.ProjectRoots
	if len(projectRoots) == 0 {
		projectRoots = DefaultProjectSkillRoots
	}

	seen := map[string]struct{}{}
	var out []InstallRef

	add := func(skill, path, root, kind string) {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			real = path
		}
		real = filepath.Clean(real)
		if _, ok := seen[real]; ok {
			return
		}
		// Must look like a skill dir (directory exists).
		st, err := os.Stat(path)
		if err != nil || !st.IsDir() {
			return
		}
		seen[real] = struct{}{}
		out = append(out, InstallRef{
			Skill:    skill,
			Path:     path,
			RealPath: real,
			RootKind: kind,
			Root:     root,
		})
	}

	for _, skill := range skillNames {
		for _, rel := range homeRoots {
			root := filepath.Join(home, rel)
			add(skill, filepath.Join(root, skill), root, "home")
		}
		if opts.ProjectDir != "" {
			for _, rel := range projectRoots {
				root := filepath.Join(opts.ProjectDir, rel)
				add(skill, filepath.Join(root, skill), root, "project")
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Skill != out[j].Skill {
			return out[i].Skill < out[j].Skill
		}
		return out[i].RealPath < out[j].RealPath
	})
	return out, nil
}
