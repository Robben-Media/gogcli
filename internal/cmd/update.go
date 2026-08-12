package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

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
	// Internal: set when re-execing the new binary for the skills phase after binary replace.
	SkillsAfterBinary bool `name:"skills-after-binary" hidden:"" help:"Internal: run skills refresh after binary update"`
}

var newSelfUpdateClient = func() *selfupdate.Client {
	return &selfupdate.Client{
		Repo:  strings.TrimSpace(os.Getenv("GOG_UPDATE_REPO")),
		Token: firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN")),
	}
}

func (c *UpdateCmd) Run(ctx context.Context) error {
	if c.SkillsOnly && c.BinaryOnly {
		return fmt.Errorf("use only one of --skills-only or --binary-only")
	}

	client := newSelfUpdateClient()

	if c.Check {
		res, err := selfupdate.Check(ctx, client, version)
		if err != nil {
			return err
		}

		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, outfmt.DirectResult(res))
		}
		if outfmt.IsPlain(ctx) {
			fmt.Fprintln(os.Stdout, "CURRENT\tLATEST\tUPDATE\tASSET")
			fmt.Fprintf(os.Stdout, "%s\t%s\t%t\t%s\n", res.Current, res.Latest, res.Update, res.Asset)

			return nil
		}

		if res.Update {
			fmt.Fprintf(os.Stdout, "gog: update available %s → %s; run: gog update\n", splitVersionBase(res.Current), res.Latest)
		} else {
			fmt.Fprintf(os.Stdout, "gog: up to date (%s)\n", res.Latest)
		}

		return nil
	}

	// Skills phase after re-exec of the newly installed binary.
	if c.SkillsAfterBinary || c.SkillsOnly {
		return c.updateSkills(ctx)
	}

	binaryUpdated := false

	if !c.SkillsOnly {
		updated, err := c.updateBinary(ctx, client)
		if err != nil {
			if c.BinaryOnly {
				return err
			}

			fmt.Fprintf(os.Stderr, "gog: binary update skipped: %v\n", err)
		} else {
			binaryUpdated = updated
		}
	}

	if c.BinaryOnly {
		return nil
	}

	// After replacing the binary, re-exec so skills come from the new embed FS.
	if binaryUpdated {
		return reexecSkillsPhase(ctx, c.ForceSkills)
	}

	return c.updateSkills(ctx)
}

func (c *UpdateCmd) updateBinary(ctx context.Context, client *selfupdate.Client) (updated bool, err error) {
	res, err := selfupdate.Check(ctx, client, version)
	if err != nil {
		return false, err
	}

	if !res.Update && !c.ForceBinary {
		fmt.Fprintf(os.Stderr, "gog: binary up to date (%s)\n", res.Latest)

		return false, nil
	}

	applied, err := selfupdate.Apply(ctx, selfupdate.ApplyOptions{
		Client:     client,
		CurrentVer: version,
		Force:      c.ForceBinary,
	})
	if err != nil {
		return false, err
	}

	fmt.Fprintf(os.Stderr, "gog: binary updated %s → %s\n", splitVersionBase(applied.Current), applied.Latest)

	return true, nil
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

func reexecSkillsPhase(ctx context.Context, forceSkills bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable for skills re-exec: %w", err)
	}

	if resolved, linkErr := filepath.EvalSymlinks(exe); linkErr == nil {
		exe = resolved
	}

	args := []string{"update", "--skills-after-binary"}
	if forceSkills {
		args = append(args, "--force-skills")
	}

	// Preserve output mode flags if present in original argv (best-effort).
	for _, a := range os.Args[1:] {
		switch a {
		case "--json", "--plain":
			args = append(args, a)
		}
	}

	//nolint:gosec // re-exec of the just-installed gog binary path
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if status, ok := ee.Sys().(syscall.WaitStatus); ok {
				return &ExitError{Code: status.ExitStatus(), Err: err}
			}
		}

		return err
	}

	return nil
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
