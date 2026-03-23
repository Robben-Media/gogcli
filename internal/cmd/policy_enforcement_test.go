package cmd

import (
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func TestPolicyEnforcement_DeniesBlockedGmailAction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := config.WriteConfig(config.File{
		Policies: []config.Policy{{
			Name:    "personal-gmail-safe",
			Account: "jdjb78@gmail.com",
			Client:  "personal",
			Allow:   []string{"gmail:read", "gmail:labels.create", "gmail:messages.modify"},
			Deny:    []string{"gmail:send", "gmail:trash", "gmail:delete", "gmail:batch-delete"},
			Reason:  "Jeremy allows triage and labeling, but not sending or deleting.",
		}},
	})
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{
				"--account", "jdjb78@gmail.com",
				"--client", "personal",
				"gmail", "send",
				"--to", "x@y.com",
				"--subject", "hello",
				"--body", "body",
			}); err == nil {
				t.Fatalf("expected gmail send to be denied")
			}
		})
	})

	if !strings.Contains(errText, `policy "personal-gmail-safe" denied gmail:send`) {
		t.Fatalf("missing denial detail: %q", errText)
	}
	if !strings.Contains(errText, "Jeremy allows triage and labeling, but not sending or deleting.") {
		t.Fatalf("missing reason: %q", errText)
	}
}

func TestPolicyEnforcement_AllowsReadLikeAction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := config.WriteConfig(config.File{
		Policies: []config.Policy{{
			Name:    "personal-gmail-safe",
			Account: "jdjb78@gmail.com",
			Allow:   []string{"gmail:read"},
		}},
	})
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "jdjb78@gmail.com",
				"gmail", "url", "thread-1",
			}); err != nil {
				t.Fatalf("gmail url: %v", err)
			}
		})
	})
	if !strings.Contains(stdout, "thread-1") || !strings.Contains(stdout, "mail.google.com") {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
}

func TestCommandActionID_FlattensGmailSettingsAndAliases(t *testing.T) {
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}
	kctx, err := parser.Parse([]string{"mail", "settings", "filters", "delete", "abc"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := commandActionID(kctx); got != "gmail:filters.delete" {
		t.Fatalf("unexpected action id: %q", got)
	}
}

func TestPolicyActionMatches(t *testing.T) {
	tests := []struct {
		pattern string
		action  string
		match   bool
	}{
		{pattern: "gmail:batch-delete", action: "gmail:batch.delete", match: true},
		{pattern: "gmail:delete", action: "gmail:thread.delete", match: true},
		{pattern: "gmail:trash", action: "gmail:messages.trash", match: true},
		{pattern: "gmail:read", action: "gmail:url", match: true},
		{pattern: "gmail:read", action: "gmail:send", match: false},
		{pattern: "gmail:settings.*", action: "gmail:settings.watch.stop", match: true},
	}
	for _, tt := range tests {
		got := policyActionMatches(normalizePolicyAction(tt.pattern), normalizePolicyAction(tt.action))
		if got != tt.match {
			t.Fatalf("pattern=%q action=%q got=%v want=%v", tt.pattern, tt.action, got, tt.match)
		}
	}
}
