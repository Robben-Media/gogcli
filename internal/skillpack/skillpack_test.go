package skillpack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steipete/gogcli/skills"
)

func TestPackManifestAndEmbed(t *testing.T) {
	if err := VerifyPackPresent(); err != nil {
		t.Fatal(err)
	}

	m, err := skills.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Skills) != 10 {
		t.Fatalf("expected 10 skills, got %d", len(m.Skills))
	}

	for _, name := range m.Skills {
		b, readErr := skills.ReadManagedFile(name, "SKILL.md")
		if readErr != nil {
			t.Fatalf("%s: %v", name, readErr)
		}

		if len(b) < 20 {
			t.Fatalf("%s: SKILL.md too short", name)
		}
	}
}

func TestDiscoverDedupeSymlink(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents", "skills", "google-calendar")
	claudeRoot := filepath.Join(home, ".claude", "skills")

	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(agents, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(claudeRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	// relative symlink like real machine
	if err := os.Symlink(filepath.Join("..", "..", ".agents", "skills", "google-calendar"), filepath.Join(claudeRoot, "google-calendar")); err != nil {
		t.Fatal(err)
	}

	refs, err := Discover([]string{"google-calendar"}, DiscoverOptions{HomeDir: home})
	if err != nil {
		t.Fatal(err)
	}

	if len(refs) != 1 {
		t.Fatalf("expected 1 deduped ref, got %d: %+v", len(refs), refs)
	}
}

func TestDirtySkipAndForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	skillDir := filepath.Join(home, ".agents", "skills", "google-docs")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	packBytes, err := skills.ReadManagedFile("google-docs", "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), packBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := UpdateInstalled(UpdateOptions{
		Discover:    DiscoverOptions{HomeDir: home},
		PackVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := actionFor(res, "google-docs"); got != "skipped_current" {
		t.Fatalf("expected skipped_current, got %s (%+v)", got, res)
	}

	packHash, err := PackHashFor("google-docs")
	if err != nil {
		t.Fatal(err)
	}

	st, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}

	resolved, _ := filepath.EvalSymlinks(skillDir)
	RecordWrite(&st, filepath.Clean(resolved), "google-docs", packHash, "test")

	if err := SaveState(st); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: google-docs\ndescription: local edit\n---\n# edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err = UpdateInstalled(UpdateOptions{
		Discover:    DiscoverOptions{HomeDir: home},
		PackVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := actionFor(res, "google-docs"); got != "skipped_dirty" {
		t.Fatalf("expected skipped_dirty, got %s", got)
	}

	res, err = UpdateInstalled(UpdateOptions{
		Discover:    DiscoverOptions{HomeDir: home},
		Force:       true,
		PackVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := actionFor(res, "google-docs"); got != "updated" {
		t.Fatalf("expected updated, got %s", got)
	}

	got, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != string(packBytes) {
		t.Fatalf("skill not restored to pack content")
	}

	if err := os.MkdirAll(filepath.Join(skillDir, "learnings"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(skillDir, "learnings", "LEARNINGS.md"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = UpdateInstalled(UpdateOptions{
		Discover:    DiscoverOptions{HomeDir: home},
		Force:       true,
		PackVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(skillDir, "learnings", "LEARNINGS.md"))
	if err != nil || string(b) != "keep me" {
		t.Fatalf("learnings clobbered: %v %q", err, b)
	}
}

func TestOutdatedUpdatesWithoutForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	skillDir := filepath.Join(home, ".agents", "skills", "google-sheets")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	old := []byte("---\nname: google-sheets\ndescription: old\n---\n# old\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), old, 0o644); err != nil {
		t.Fatal(err)
	}

	realDir, err := filepath.EvalSymlinks(skillDir)
	if err != nil {
		realDir = skillDir
	}

	realDir = filepath.Clean(realDir)

	oldHash, err := HashManagedFiles(realDir, []string{"SKILL.md"})
	if err != nil {
		t.Fatal(err)
	}

	st, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}

	RecordWrite(&st, realDir, "google-sheets", oldHash, "old")

	if err := SaveState(st); err != nil {
		t.Fatal(err)
	}

	res, err := UpdateInstalled(UpdateOptions{
		Discover:    DiscoverOptions{HomeDir: home},
		PackVersion: "new",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := actionFor(res, "google-sheets"); got != "updated" {
		t.Fatalf("expected updated for outdated, got %s (%+v)", got, res)
	}
}

func TestInstallMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := UpdateInstalled(UpdateOptions{
		Discover:    DiscoverOptions{HomeDir: home},
		Install:     true,
		InstallRoot: filepath.Join(home, ".agents", "skills"),
		OnlySkills:  []string{"google-calendar"},
		PackVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := actionFor(res, "google-calendar"); got != "installed" {
		t.Fatalf("expected installed, got %s", got)
	}

	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "google-calendar", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func actionFor(results []ApplyResult, skill string) string {
	for _, r := range results {
		if r.Skill == skill {
			return r.Action
		}
	}

	return ""
}
