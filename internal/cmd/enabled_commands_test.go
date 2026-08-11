package cmd

import (
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func TestParseEnabledCommands(t *testing.T) {
	allow := parseEnabledCommands("calendar, tasks ,Gmail")
	if !allow["calendar"] || !allow["tasks"] || !allow["gmail"] {
		t.Fatalf("unexpected allow map: %#v", allow)
	}
}

func TestCommandPath_ExcludesPositionalsAndUsesCanonicalNames(t *testing.T) {
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "search drops query positional",
			args: []string{"gmail", "search", "is:unread", "--max", "5"},
			want: []string{"gmail", "search"},
		},
		{
			name: "service alias canonicalizes",
			args: []string{"mail", "search", "is:unread"},
			want: []string{"gmail", "search"},
		},
		{
			name: "nested default and alias resolve to full leaf path",
			args: []string{"gmail", "read", "thread-1"},
			want: []string{"gmail", "thread", "get"},
		},
		{
			name: "explicit nested path",
			args: []string{"gmail", "thread", "get", "thread-1"},
			want: []string{"gmail", "thread", "get"},
		},
		{
			name: "flags only command",
			args: []string{"gmail", "send", "--to", "a@b.com", "--subject", "s", "--body", "b"},
			want: []string{"gmail", "send"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kctx, err := parser.Parse(tt.args)
			if err != nil {
				t.Fatalf("Parse(%v): %v", tt.args, err)
			}
			got := commandPath(kctx)
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Fatalf("commandPath(%v)=%v want %v (command=%q)", tt.args, got, tt.want, kctx.Command())
			}
		})
	}
}

func TestEnforceEnabledCommands_ExactPathAllowAndSiblingDenial(t *testing.T) {
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	search, err := parser.Parse([]string{"gmail", "search", "is:unread"})
	if err != nil {
		t.Fatalf("Parse search: %v", err)
	}
	if enforceErr := enforceEnabledCommands(search, "", "gmail search"); enforceErr != nil {
		t.Fatalf("expected gmail search allow: %v", enforceErr)
	}

	send, err := parser.Parse([]string{"gmail", "send", "--to", "a@b.com", "--subject", "s", "--body", "b"})
	if err != nil {
		t.Fatalf("Parse send: %v", err)
	}
	if err := enforceEnabledCommands(send, "", "gmail search"); err == nil {
		t.Fatalf("expected sibling gmail send to be denied")
	} else if !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("unexpected deny error: %v", err)
	}
}

func TestEnforceEnabledCommands_NestedPathsExactOnly(t *testing.T) {
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	get, err := parser.Parse([]string{"gmail", "thread", "get", "thread-1"})
	if err != nil {
		t.Fatalf("Parse get: %v", err)
	}
	if enforceErr := enforceEnabledCommands(get, "", "gmail thread get"); enforceErr != nil {
		t.Fatalf("expected nested path allow: %v", enforceErr)
	}

	// Parent path must not implicitly allow the leaf.
	if enforceErr := enforceEnabledCommands(get, "", "gmail thread"); enforceErr == nil {
		t.Fatalf("expected parent path not to allow nested leaf")
	}

	modify, err := parser.Parse([]string{"gmail", "thread", "modify", "thread-1", "--add", "INBOX"})
	if err != nil {
		t.Fatalf("Parse modify: %v", err)
	}
	if err := enforceEnabledCommands(modify, "", "gmail thread get"); err == nil {
		t.Fatalf("expected sibling nested path denial")
	}
}

func TestEnforceEnabledCommands_AliasAllowlistEntries(t *testing.T) {
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	// Allowlist uses aliases; invocation uses canonical names (and the reverse).
	kctx, err := parser.Parse([]string{"gmail", "search", "is:unread"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if enforceErr := enforceEnabledCommands(kctx, "", "mail search"); enforceErr != nil {
		t.Fatalf("expected alias allowlist entry to match canonical path: %v", enforceErr)
	}

	aliasInvoke, err := parser.Parse([]string{"mail", "search", "is:unread"})
	if err != nil {
		t.Fatalf("Parse alias invoke: %v", err)
	}
	if enforceErr := enforceEnabledCommands(aliasInvoke, "", "gmail search"); enforceErr != nil {
		t.Fatalf("expected alias invocation to match canonical allowlist: %v", enforceErr)
	}

	// Nested command alias must not bypass a leaf restriction.
	// `read` aliases the `thread` group; with a thread id Kong selects default `get`,
	// so the canonical leaf identity is still "gmail thread get".
	read, err := parser.Parse([]string{"gmail", "read", "thread-1"})
	if err != nil {
		t.Fatalf("Parse read: %v", err)
	}
	if err := enforceEnabledCommands(read, "", "gmail search"); err == nil {
		t.Fatalf("expected alias invocation to remain restricted")
	}
	if err := enforceEnabledCommands(read, "", "gmail thread get"); err != nil {
		t.Fatalf("expected nested alias invocation to share canonical allow identity: %v", err)
	}
	// Allowlisting the group alias alone is still a parent path, not the leaf.
	if err := enforceEnabledCommands(read, "", "gmail read"); err == nil {
		t.Fatalf("expected group alias allowlist entry not to allow the default leaf")
	}
}

func TestEnforceEnabledCommands_LegacyTopLevelMatchesHistoricalExactNames(t *testing.T) {
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	tests := []struct {
		name        string
		args        []string
		enabled     string
		wantAllowed bool
	}{
		{name: "canonical calendar", args: []string{"calendar", "colors"}, enabled: "calendar", wantAllowed: true},
		{name: "canonical gmail", args: []string{"gmail", "search", "is:unread"}, enabled: "gmail", wantAllowed: true},
		{name: "mail alias entry remains inert", args: []string{"mail", "search", "is:unread"}, enabled: "mail", wantAllowed: false},
		{name: "email alias entry remains inert", args: []string{"email", "search", "is:unread"}, enabled: "email", wantAllowed: false},
		{name: "bq alias entry remains inert", args: []string{"bq", "datasets", "--project", "test-project"}, enabled: "bq", wantAllowed: false},
		{name: "gsc alias entry remains inert", args: []string{"gsc", "sites", "list"}, enabled: "gsc", wantAllowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kctx, parseErr := parser.Parse(tt.args)
			if parseErr != nil {
				t.Fatalf("Parse(%v): %v", tt.args, parseErr)
			}
			gotAllowed := enforceEnabledCommands(kctx, tt.enabled, "") == nil
			if gotAllowed != tt.wantAllowed {
				t.Fatalf("legacy --enable-commands=%q with %v allowed=%t, want %t", tt.enabled, tt.args, gotAllowed, tt.wantAllowed)
			}
		})
	}
}

func TestEnforceEnabledCommands_ORCompositionBetweenAllowlists(t *testing.T) {
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	cal, err := parser.Parse([]string{"calendar", "colors"})
	if err != nil {
		t.Fatalf("Parse calendar: %v", err)
	}
	search, err := parser.Parse([]string{"gmail", "search", "is:unread"})
	if err != nil {
		t.Fatalf("Parse search: %v", err)
	}
	send, err := parser.Parse([]string{"gmail", "send", "--to", "a@b.com", "--subject", "s", "--body", "b"})
	if err != nil {
		t.Fatalf("Parse send: %v", err)
	}

	if err := enforceEnabledCommands(cal, "calendar", "gmail search"); err != nil {
		t.Fatalf("top-level side of OR failed: %v", err)
	}
	if err := enforceEnabledCommands(search, "calendar", "gmail search"); err != nil {
		t.Fatalf("exact-path side of OR failed: %v", err)
	}
	if err := enforceEnabledCommands(send, "calendar", "gmail search"); err == nil {
		t.Fatalf("expected command matching neither allowlist to be denied")
	}
}

func TestEnforceEnabledCommands_UnrestrictedWhenNeitherSet(t *testing.T) {
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}
	kctx, err := parser.Parse([]string{"gmail", "send", "--to", "a@b.com", "--subject", "s", "--body", "b"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := enforceEnabledCommands(kctx, "", ""); err != nil {
		t.Fatalf("expected unrestricted when neither allowlist is set: %v", err)
	}
	if err := enforceEnabledCommands(kctx, "  ", " , "); err != nil {
		t.Fatalf("expected unrestricted for empty-ish allowlists: %v", err)
	}
	if err := enforceEnabledCommands(kctx, "*", ""); err != nil {
		t.Fatalf("expected * top-level to remain unrestricted: %v", err)
	}
	if err := enforceEnabledCommands(kctx, "all", ""); err != nil {
		t.Fatalf("expected all top-level to remain unrestricted: %v", err)
	}
}

func TestEnforceEnabledCommands_AndCompositionWithPersistedPolicy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := config.WriteConfig(config.File{
		Policies: []config.Policy{{
			Name:    "no-send",
			Account: "user@example.com",
			Deny:    []string{"gmail:send"},
			Reason:  "send blocked by policy",
		}},
	})
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	// Path allowlist permits send, but persisted policy must still deny.
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{
				"--enable-command-paths", "gmail send",
				"--account", "user@example.com",
				"gmail", "send",
				"--to", "x@y.com",
				"--subject", "hello",
				"--body", "body",
			}); err == nil {
				t.Fatalf("expected policy denial after path allow")
			}
		})
	})
	if !strings.Contains(errText, `policy "no-send" denied gmail:send`) {
		t.Fatalf("missing policy denial: %q", errText)
	}
	if strings.Contains(errText, "not enabled") {
		t.Fatalf("path allowlist should have passed before policy: %q", errText)
	}

	// Sibling still fails at the enablement gate before policy.
	errText = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{
				"--enable-command-paths", "gmail search",
				"--account", "user@example.com",
				"gmail", "send",
				"--to", "x@y.com",
				"--subject", "hello",
				"--body", "body",
			}); err == nil {
				t.Fatalf("expected enablement denial for sibling")
			}
		})
	})
	if !strings.Contains(errText, "not enabled") {
		t.Fatalf("expected enablement denial, got: %q", errText)
	}
}

func TestEnableCommandPaths_ExecuteSurface(t *testing.T) {
	err := Execute([]string{"--enable-command-paths", "gmail search", "tasks", "lists"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}
