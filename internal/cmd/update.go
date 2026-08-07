package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/selfupdate"
	"github.com/steipete/gogcli/internal/skillpack"
)

// UpdateCmd upgrades the gog binary and/or companion skill pack.
type UpdateCmd struct {
	SkillsOnly  bool `help:"Only refresh companion skills (no binary download)"`
	BinaryOnly  bool `help:"Only update the gog binary (no skills)"`
	ForceBinary bool `name:"force-binary" help:"Allow replacing dev/dirty binaries"`
	ForceSkills bool `name:"force-skills" help:"Overwrite pack skills that have local edits"`
	Check       bool `help:"Only check for a binary update; do not install"`
}

func (c *UpdateCmd) Run(ctx context.Context) error {
	if c.SkillsOnly && c.BinaryOnly {
		return fmt.Errorf("use only one of --skills-only or --binary-only")
	}

	client := &selfupdate.Client{
		Repo:  strings.TrimSpace(os.Getenv("GOG_UPDATE_REPO")),
		Token: firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN")),
	}

	if c.Check {
		res, err := selfupdate.Check(ctx, client, version)
		if err != nil {
			return err
		}
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(os.Stdout, res)
		}
		if res.Update {
			fmt.Fprintf(os.Stdout, "gog: update available %s → %s; run: gog update\n", splitVersionBase(res.Current), res.Latest)
		} else {
			fmt.Fprintf(os.Stdout, "gog: up to date (%s)\n", res.Latest)
		}
		return nil
	}

	if !c.SkillsOnly {
		if err := c.updateBinary(ctx, client); err != nil {
			if c.BinaryOnly {
				return err
			}
			fmt.Fprintf(os.Stderr, "gog: binary update skipped: %v\n", err)
		}
	}

	if c.BinaryOnly {
		return nil
	}
	return c.updateSkills(ctx)
}

func (c *UpdateCmd) updateBinary(ctx context.Context, client *selfupdate.Client) error {
	res, err := selfupdate.Check(ctx, client, version)
	if err != nil {
		return err
	}
	if !res.Update && !c.ForceBinary {
		fmt.Fprintf(os.Stderr, "gog: binary up to date (%s)\n", res.Latest)
		return nil
	}
	applied, err := selfupdate.Apply(ctx, selfupdate.ApplyOptions{
		Client:     client,
		CurrentVer: version,
		Force:      c.ForceBinary,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gog: binary updated %s → %s\n", splitVersionBase(applied.Current), applied.Latest)
	return nil
}

func (c *UpdateCmd) updateSkills(ctx context.Context) error {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	results, err := skillpack.UpdateInstalled(skillpack.UpdateOptions{
		Discover: skillpack.DiscoverOptions{
			HomeDir:    home,
			ProjectDir: cwd,
		},
		Force:       c.ForceSkills,
		Install:     false,
		InstallRoot: filepath.Join(home, ".agents", "skills"),
		PackVersion: VersionString(),
	})
	if err != nil {
		return err
	}
	return printSkillResults(ctx, results)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func splitVersionBase(v string) string {
	v = selfupdate.NormalizeVersion(v)
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i]
	}
	return v
}
