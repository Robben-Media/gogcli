package cmd

import (
	"strings"
	"testing"
)

type outputPathCmd struct {
	Output OutputPathFlag `embed:""`
}

type outputPathRequiredCmd struct {
	Output OutputPathRequiredFlag `embed:""`
}

type outputDirCmd struct {
	OutputDir OutputDirFlag `embed:""`
}

func TestOutputPathFlag_AcceptsOutputAlias(t *testing.T) {
	cmd := &outputPathCmd{}
	parseKongContext(t, cmd, []string{"--output", "file.txt"})

	if cmd.Output.Path != "file.txt" {
		t.Fatalf("expected output path, got %q", cmd.Output.Path)
	}
}

func TestOutputPathFlag_HelpDescribesDerivedDefault(t *testing.T) {
	cmd := &outputPathCmd{}
	kctx := parseKongContext(t, cmd, nil)

	for _, f := range kctx.Flags() {
		if f.Name != "out" {
			continue
		}
		if strings.Contains(f.Help, "gogcli config dir") {
			t.Fatalf("out flag help misleads about a single default dir: %q", f.Help)
		}
		if !strings.Contains(strings.ToLower(f.Help), "derived filename") {
			t.Fatalf("out flag help does not describe the derived default filename: %q", f.Help)
		}
		return
	}
	t.Fatal("out flag not found on outputPathCmd")
}

func TestOutputPathFlag_AcceptsOut(t *testing.T) {
	cmd := &outputPathCmd{}
	parseKongContext(t, cmd, []string{"--out", "file.txt"})

	if cmd.Output.Path != "file.txt" {
		t.Fatalf("expected out path, got %q", cmd.Output.Path)
	}
}

func TestOutputPathRequiredFlag_AcceptsOutputAlias(t *testing.T) {
	cmd := &outputPathRequiredCmd{}
	parseKongContext(t, cmd, []string{"--output", "file.txt"})

	if cmd.Output.Path != "file.txt" {
		t.Fatalf("expected output path, got %q", cmd.Output.Path)
	}
}

func TestOutputDirFlag_AcceptsOutputDirAlias(t *testing.T) {
	cmd := &outputDirCmd{}
	parseKongContext(t, cmd, []string{"--output-dir", "out"})

	if cmd.OutputDir.Dir != "out" {
		t.Fatalf("expected output dir, got %q", cmd.OutputDir.Dir)
	}
}

func TestOutputDirFlag_AcceptsOutDir(t *testing.T) {
	cmd := &outputDirCmd{}
	parseKongContext(t, cmd, []string{"--out-dir", "out"})

	if cmd.OutputDir.Dir != "out" {
		t.Fatalf("expected out dir, got %q", cmd.OutputDir.Dir)
	}
}
