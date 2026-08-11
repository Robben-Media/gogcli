package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/skillpack"
)

// SkillsCmd manages the companion skill pack for this CLI.
type SkillsCmd struct {
	Status  SkillsStatusCmd  `cmd:"" help:"Show pack skill install status (pack skills only)"`
	Update  SkillsUpdateCmd  `cmd:"" help:"Refresh installed pack skills (skip local edits unless --force)"`
	Install SkillsInstallCmd `cmd:"" help:"Install pack skills into ~/.agents/skills and link other agent roots"`
}

// SkillsStatusCmd lists status for each pack skill.
type SkillsStatusCmd struct{}

func (c *SkillsStatusCmd) Run(ctx context.Context) error {
	cwd, _ := os.Getwd()
	rows, err := skillpack.EvaluateAll(VersionString(), skillpack.DiscoverOptions{ProjectDir: cwd})
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		type row struct {
			Skill    string `json:"skill"`
			Status   string `json:"status"`
			Path     string `json:"path,omitempty"`
			RealPath string `json:"real_path,omitempty"`
		}
		out := make([]row, 0, len(rows))
		for _, r := range rows {
			out = append(out, row{
				Skill:    r.Skill,
				Status:   string(r.Kind),
				Path:     r.Path,
				RealPath: r.RealPath,
			})
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"pack_version": VersionString(),
			"skills":       out,
		})
	}
	if outfmt.IsPlain(ctx) {
		for _, r := range rows {
			path := r.RealPath
			if path == "" {
				path = "-"
			}
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n", r.Skill, r.Kind, path)
		}
		return nil
	}
	fmt.Fprintf(os.Stdout, "gog skill pack (version %s)\n", VersionString())
	for _, r := range rows {
		switch r.Kind {
		case skillpack.StatusNotInstalled:
			fmt.Fprintf(os.Stdout, "  %-28s  %s\n", r.Skill, r.Kind)
		default:
			fmt.Fprintf(os.Stdout, "  %-28s  %-16s  %s\n", r.Skill, r.Kind, r.RealPath)
		}
	}
	return nil
}

// SkillsUpdateCmd refreshes installed pack skills.
type SkillsUpdateCmd struct {
	OverwriteLocal bool `name:"overwrite-local" help:"Overwrite pack skills that have local edits"`
}

func (c *SkillsUpdateCmd) Run(ctx context.Context) error {
	return runSkillUpdate(ctx, c.OverwriteLocal, false)
}

// SkillsInstallCmd installs missing pack skills.
type SkillsInstallCmd struct {
	OverwriteLocal bool `name:"overwrite-local" help:"Overwrite pack skills that have local edits"`
}

func (c *SkillsInstallCmd) Run(ctx context.Context) error {
	return runSkillUpdate(ctx, c.OverwriteLocal, true)
}

func runSkillUpdate(ctx context.Context, force, install bool) error {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	results, err := skillpack.UpdateInstalled(skillpack.UpdateOptions{
		Discover: skillpack.DiscoverOptions{
			HomeDir:    home,
			ProjectDir: cwd,
		},
		Force:       force,
		Install:     install,
		InstallRoot: filepath.Join(home, ".agents", "skills"),
		PackVersion: VersionString(),
	})
	if err != nil {
		return err
	}
	return printSkillResults(ctx, results)
}

func printSkillResults(ctx context.Context, results []skillpack.ApplyResult) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"results": results})
	}

	var updated, installed, dirty, current, missing []string
	for _, r := range results {
		switch r.Action {
		case "updated":
			updated = append(updated, r.Skill)
			fmt.Fprintf(os.Stderr, "gog: skills updated: %s @ %s\n", r.Skill, r.RealPath)
		case "installed":
			installed = append(installed, r.Skill)
			fmt.Fprintf(os.Stderr, "gog: skills installed: %s @ %s\n", r.Skill, r.RealPath)
		case "skipped_dirty":
			dirty = append(dirty, r.Skill)
			fmt.Fprintf(os.Stderr, "gog: skills skipped (local edits): %s @ %s\n", r.Skill, r.RealPath)
			fmt.Fprintf(os.Stderr, "     re-run with: gog skills update --overwrite-local\n")
		case "skipped_current":
			current = append(current, r.Skill)
		case "skipped_not_installed":
			missing = append(missing, r.Skill)
		}
	}

	if outfmt.IsPlain(ctx) {
		for _, r := range results {
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n", r.Skill, r.Action, r.RealPath)
		}
		return nil
	}

	// Human summary on stdout
	if len(updated) > 0 {
		fmt.Fprintf(os.Stdout, "updated: %s\n", strings.Join(unique(updated), ", "))
	}
	if len(installed) > 0 {
		fmt.Fprintf(os.Stdout, "installed: %s\n", strings.Join(unique(installed), ", "))
	}
	if len(dirty) > 0 {
		fmt.Fprintf(os.Stdout, "skipped (local edits): %s\n", strings.Join(unique(dirty), ", "))
		fmt.Fprintln(os.Stderr, "Tell the user which skills were skipped and their paths; do not force-overwrite unless they ask.")
	}
	if len(missing) > 0 {
		missingList := strings.Join(unique(missing), ", ")
		fmt.Fprintf(os.Stdout, "not installed: %s\n", missingList)
		fmt.Fprintf(os.Stderr, "gog: skills not installed: %s — run: gog skills install\n", missingList)
	}
	if len(updated) == 0 && len(installed) == 0 && len(dirty) == 0 && len(missing) == 0 {
		fmt.Fprintln(os.Stdout, "all installed pack skills are current")
	}
	_ = current
	return nil
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
