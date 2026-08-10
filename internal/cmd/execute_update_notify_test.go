package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/selfupdate"
)

func TestExecute_SkipsUpdateNotifyForCompletionCommands(t *testing.T) {
	// Serial: overrides package-level maybeNotifyUpdate seam.
	cases := []struct {
		name string
		args []string
	}{
		{name: "completion bash", args: []string{"completion", "bash"}},
		{name: "completion zsh", args: []string{"completion", "zsh"}},
		{name: "completion fish", args: []string{"completion", "fish"}},
		{name: "completion powershell", args: []string{"completion", "powershell"}},
		{name: "__complete", args: []string{"__complete", "--cword", "1", "--", "gog", ""}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			orig := maybeNotifyUpdate
			maybeNotifyUpdate = func(context.Context, *selfupdate.Client, string, time.Duration) string {
				calls++
				return "gog: update available 1.0.0 → 9.9.9; run: gog update"
			}
			t.Cleanup(func() { maybeNotifyUpdate = orig })

			errText := captureStderr(t, func() {
				_ = captureStdout(t, func() {
					if err := Execute(tc.args); err != nil {
						t.Fatalf("Execute(%v): %v", tc.args, err)
					}
				})
			})
			if calls != 0 {
				t.Fatalf("update notifier calls = %d, want 0 for %v", calls, tc.args)
			}
			if strings.Contains(errText, "update available") {
				t.Fatalf("stderr should not contain update notice, got %q", errText)
			}
		})
	}
}

func TestExecute_InvokesUpdateNotifyForNormalCommand(t *testing.T) {
	calls := 0
	orig := maybeNotifyUpdate
	maybeNotifyUpdate = func(context.Context, *selfupdate.Client, string, time.Duration) string {
		calls++
		return ""
	}
	t.Cleanup(func() { maybeNotifyUpdate = orig })

	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"version"}); err != nil {
				t.Fatalf("Execute(version): %v", err)
			}
		})
	})
	if calls != 1 {
		t.Fatalf("update notifier calls = %d, want 1 for normal command", calls)
	}
}
