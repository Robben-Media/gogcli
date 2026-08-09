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

func TestPolicyEnforcement_ImplicitAllowlistDenyDoesNotBlameOnePolicy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := config.WriteConfig(config.File{
		Policies: []config.Policy{
			{
				Name:    "allow-read",
				Account: "jdjb78@gmail.com",
				Allow:   []string{"gmail:read"},
			},
			{
				Name:    "allow-labels",
				Account: "jdjb78@gmail.com",
				Allow:   []string{"gmail:labels.create"},
			},
		},
	})
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{
				"--account", "jdjb78@gmail.com",
				"gmail", "send",
				"--to", "x@y.com",
				"--subject", "hello",
				"--body", "body",
			}); err == nil {
				t.Fatalf("expected gmail send to be denied")
			}
		})
	})

	if !strings.Contains(errText, "no policy allows gmail:send for jdjb78@gmail.com") {
		t.Fatalf("missing implicit deny message: %q", errText)
	}
	if strings.Contains(errText, `policy "allow-read" denied`) || strings.Contains(errText, `policy "allow-labels" denied`) {
		t.Fatalf("unexpected blame in implicit deny: %q", errText)
	}
}

func TestPolicyEnforcement_ImplicitAllowlistDenyIncludesClientContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := config.WriteConfig(config.File{
		Policies: []config.Policy{{
			Name:    "allow-read",
			Account: "jdjb78@gmail.com",
			Client:  "personal",
			Allow:   []string{"gmail:read"},
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

	if !strings.Contains(errText, "no policy allows gmail:send for jdjb78@gmail.com (client personal)") {
		t.Fatalf("missing client context in implicit deny: %q", errText)
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

func TestPolicyEnforcement_NormalizesHyphenatedServicesAndAliases(t *testing.T) {
	tests := []struct {
		name      string
		commands  [][]string
		action    string
		otherRule string
	}{
		{
			name: "business profile",
			commands: [][]string{
				{"business-profile", "accounts", "list"},
				{"gbp", "accounts", "list"},
				{"business", "accounts", "list"},
			},
			action:    "businessprofile:accounts.list",
			otherRule: "businessprofile:locations.list",
		},
		{
			name: "search console",
			commands: [][]string{
				{"search-console", "sites", "list"},
				{"gsc", "sites", "list"},
				{"sc", "sites", "list"},
			},
			action:    "searchconsole:sites.list",
			otherRule: "searchconsole:sitemaps.list",
		},
		{
			name: "tag manager",
			commands: [][]string{
				{"tag-manager", "accounts"},
				{"gtm", "accounts"},
			},
			action:    "tagmanager:accounts",
			otherRule: "tagmanager:containers",
		},
	}

	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, command := range tt.commands {
				kctx, err := parser.Parse(command)
				if err != nil {
					t.Fatalf("Parse(%q): %v", command, err)
				}
				action := commandActionID(kctx)
				if action != tt.action {
					t.Fatalf("commandActionID(%q) = %q, want %q", command, action, tt.action)
				}

				service, _, _ := strings.Cut(action, ":")
				explicitDeny := []config.Policy{{Name: "deny", Deny: []string{tt.action}}}
				if !hasPolicyForService(explicitDeny, service) {
					t.Fatalf("explicit deny policy not discovered for %q", command)
				}
				if decision := evaluatePolicies(explicitDeny, action, "", ""); !decision.Denied || decision.ImplicitAllowlist {
					t.Fatalf("explicit deny decision for %q = %#v", command, decision)
				}

				allowlist := []config.Policy{{Name: "allow-other", Allow: []string{tt.otherRule}}}
				if !hasPolicyForService(allowlist, service) {
					t.Fatalf("allowlist policy not discovered for %q", command)
				}
				if decision := evaluatePolicies(allowlist, action, "", ""); !decision.Denied || !decision.ImplicitAllowlist {
					t.Fatalf("implicit allowlist decision for %q = %#v", command, decision)
				}
			}
		})
	}
}

func TestPolicyCommandsBypassPolicyEnforcement(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := config.WriteConfig(config.File{
		Policies: []config.Policy{{
			Name:    "lockdown",
			Account: "jdjb78@gmail.com",
			Deny:    []string{"policy:*"},
		}},
	})
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "jdjb78@gmail.com", "policy", "list"}); err != nil {
				t.Fatalf("policy list: %v", err)
			}
		})
	})
	if !strings.Contains(stdout, "lockdown") {
		t.Fatalf("expected policy list to bypass enforcement, got %q", stdout)
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
