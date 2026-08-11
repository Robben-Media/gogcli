package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestEnvOr(t *testing.T) {
	t.Setenv("X_TEST", "")
	if got := envOr("X_TEST", "fallback"); got != "fallback" {
		t.Fatalf("unexpected: %q", got)
	}
	t.Setenv("X_TEST", "value")
	if got := envOr("X_TEST", "fallback"); got != "value" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestExecute_Help(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--help"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Google CLI") && !strings.Contains(out, "Usage:") {
		t.Fatalf("unexpected help output: %q", out)
	}
	if !strings.Contains(out, "config.json") || !strings.Contains(out, "keyring backend") {
		t.Fatalf("expected config info in help output: %q", out)
	}
	if strings.Contains(out, "gmail (mail,email) thread get") {
		t.Fatalf("expected collapsed help (no expanded subcommands), got: %q", out)
	}
}

func TestExecute_Help_GmailHasGroupsAndRelativeCommands(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"gmail", "--help"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "\nRead\n") || !strings.Contains(out, "\nWrite\n") || !strings.Contains(out, "\nAdmin\n") {
		t.Fatalf("expected command groups in gmail help, got: %q", out)
	}
	if !strings.Contains(out, "\n  search <query>") {
		t.Fatalf("expected relative command summaries in gmail help, got: %q", out)
	}
	if strings.Contains(out, "\n  gmail (mail,email) search <query>") {
		t.Fatalf("unexpected full command prefix in gmail help, got: %q", out)
	}
	if strings.Contains(out, "\n  watch <command>") {
		t.Fatalf("expected watch to be under gmail settings (not top-level gmail help), got: %q", out)
	}
	if !strings.Contains(out, "\n  settings <command>") {
		t.Fatalf("expected settings subgroup in gmail help, got: %q", out)
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"no_such_cmd"}); err == nil {
				t.Fatalf("expected error")
			}
		})
	})
	if errText == "" {
		t.Fatalf("expected stderr output")
	}
}

func TestExecute_UnknownFlag(t *testing.T) {
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--definitely-nope"}); err == nil {
				t.Fatalf("expected error")
			}
		})
	})
	if errText == "" {
		t.Fatalf("expected stderr output")
	}
}

func TestExecute_JSONTransformationsRequireJSON(t *testing.T) {
	for _, args := range [][]string{
		{"--results-only", "version"},
		{"--select", "version,commit", "version"},
		{"--plain", "--results-only", "version"},
		{"--version", "--select", "version"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var execErr error
			stdout := captureStdout(t, func() {
				stderr := captureStderr(t, func() {
					execErr = Execute(args)
				})
				if !strings.Contains(stderr, "require --json") {
					t.Fatalf("expected require --json diagnostic, got %q", stderr)
				}
			})
			if execErr == nil || ExitCode(execErr) != 2 {
				t.Fatalf("expected usage error, got %v", execErr)
			}
			if stdout != "" {
				t.Fatalf("unexpected partial output: %q", stdout)
			}
		})
	}
}

func TestExecute_InvalidJSONSelectionReportsUsageError(t *testing.T) {
	var execErr error
	stdout := captureStdout(t, func() {
		stderr := captureStderr(t, func() {
			execErr = Execute([]string{"--json", "--select", "version,,commit", "version"})
		})
		if !strings.Contains(stderr, "invalid JSON selection path") {
			t.Fatalf("unexpected stderr: %q", stderr)
		}
	})
	if execErr == nil || ExitCode(execErr) != 2 {
		t.Fatalf("expected usage error, got %v", execErr)
	}
	if stdout != "" {
		t.Fatalf("unexpected partial output: %q", stdout)
	}
}

func TestExecute_ConflictingOutputModesReportsStderr(t *testing.T) {
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			err := Execute([]string{"--json", "--plain", "version"})
			if err == nil {
				t.Fatalf("expected error")
			}
			if ExitCode(err) != 2 {
				t.Fatalf("expected exit code 2, got %d (err=%v)", ExitCode(err), err)
			}
		})
	})
	if !strings.Contains(errText, "invalid output mode") {
		t.Fatalf("expected stderr diagnostic, got %q", errText)
	}
}

func TestExecute_VersionSelectsJSONFields(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--version", "--json", "--select", "version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "\"version\"") || strings.Contains(out, "\"commit\"") {
		t.Fatalf("unexpected selected version output: %q", out)
	}
}

func TestExecute_VersionConflictingOutputModesReportsStderr(t *testing.T) {
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			err := Execute([]string{"--version", "--json", "--plain"})
			if err == nil {
				t.Fatalf("expected error")
			}
			if ExitCode(err) != 2 {
				t.Fatalf("expected exit code 2, got %d (err=%v)", ExitCode(err), err)
			}
		})
	})
	if !strings.Contains(errText, "invalid output mode") {
		t.Fatalf("expected stderr diagnostic, got %q", errText)
	}
}

func TestExecute_InvalidColorReportsStderr(t *testing.T) {
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--color", "nope", "version"}); err == nil {
				t.Fatalf("expected error")
			}
		})
	})
	if !strings.Contains(errText, "invalid --color") {
		t.Fatalf("expected stderr diagnostic, got %q", errText)
	}
}

func TestNewUsageError(t *testing.T) {
	if newUsageError(nil) != nil {
		t.Fatalf("expected nil for nil error")
	}

	err := errors.New("bad")
	wrapped := newUsageError(err)
	if wrapped == nil {
		t.Fatalf("expected wrapped error")
	}
	var exitErr *ExitError
	if !errors.As(wrapped, &exitErr) || exitErr.Code != 2 || !errors.Is(exitErr.Err, err) {
		t.Fatalf("unexpected wrapped error: %#v", wrapped)
	}
}
