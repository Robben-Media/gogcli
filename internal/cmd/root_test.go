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
	if !strings.Contains(out, "--wrap-untrusted") {
		t.Fatalf("expected --wrap-untrusted in root help, got: %q", out)
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
