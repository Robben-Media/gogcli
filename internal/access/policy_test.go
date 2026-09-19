package access

import (
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func TestCanonicalServiceSearchConsole(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"search-console", "searchconsole", "gsc", "sc"} {
		if got := CanonicalService(raw); got != "searchconsole" {
			t.Fatalf("%q: got %q", raw, got)
		}
	}
}

func TestCanonicalActionSearchConsoleAliases(t *testing.T) {
	t.Parallel()

	want := "searchconsole:query"
	for _, raw := range []string{"search-console:query", "searchconsole:query", "gsc:query", "search-console:query"} {
		if got := CanonicalAction(raw); got != want {
			t.Fatalf("%q: got %q want %q", raw, got, want)
		}
	}
}

func TestMatchActionPreservesGmailShorthands(t *testing.T) {
	t.Parallel()

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
		{pattern: "search-console:query", action: "searchconsole:query", match: true},
		{pattern: "searchconsole:*", action: "search-console:sites.list", match: true},
		{pattern: "gsc:sites.list", action: "search-console:sites.list", match: true},
	}
	for _, tt := range tests {
		if got := MatchAction(tt.pattern, tt.action); got != tt.match {
			t.Fatalf("pattern=%q action=%q got=%v want=%v", tt.pattern, tt.action, got, tt.match)
		}
	}
}

func TestEvaluateSpecificityAndDeny(t *testing.T) {
	t.Parallel()

	policies := []config.Policy{
		{
			Name:    "broad-allow",
			Account: "work@example.com",
			Allow:   []string{"gmail:*"},
		},
		{
			Name:    "client-deny",
			Account: "work@example.com",
			Client:  "workspace",
			Deny:    []string{"gmail:send"},
			Reason:  "no send from workspace client",
		},
	}

	denied := Evaluate(policies, "gmail:send", "work@example.com", "workspace")
	if !denied.Denied || denied.DeniedBy.Name != "client-deny" {
		t.Fatalf("expected client deny, got %#v", denied)
	}

	allowed := Evaluate(policies, "gmail:send", "work@example.com", "personal")
	if allowed.Denied {
		t.Fatalf("broader allow should apply without the client deny: %#v", allowed)
	}
}

func TestEvaluateImplicitAllowlist(t *testing.T) {
	t.Parallel()

	policies := []config.Policy{{
		Name:    "read-only",
		Account: "jdjb78@gmail.com",
		Allow:   []string{"gmail:read"},
	}}

	got := Evaluate(policies, "gmail:send", "jdjb78@gmail.com", "")
	if !got.Denied || !got.ImplicitAllowlist {
		t.Fatalf("expected implicit allowlist deny, got %#v", got)
	}
}

func TestEvaluateDashedStoredPolicy(t *testing.T) {
	t.Parallel()

	policies := []config.Policy{{
		Name:    "gsc-read",
		Account: "work@example.com",
		Allow:   []string{"search-console:query"},
		Deny:    []string{"searchconsole:sites.add"},
	}}
	if got := Evaluate(policies, "searchconsole:query", "work@example.com", ""); got.Denied {
		t.Fatalf("canonical query should match dashed stored allow: %#v", got)
	}

	if got := Evaluate(policies, "search-console:sites.add", "work@example.com", ""); !got.Denied {
		t.Fatalf("dashed invoke should match canonical deny")
	}
}

func TestEvaluateOpaqueAccountIDIsExact(t *testing.T) {
	t.Parallel()

	policies := []config.Policy{
		{
			Name:   "client-deny",
			Client: "native",
			Deny:   []string{"gmail:get"},
		},
		{
			Name:    "upper-allow",
			Account: "A",
			Client:  "native",
			Allow:   []string{"gmail:get"},
		},
	}

	if got := Evaluate(policies, "gmail:get", "lower@example.test", "native"); !got.Denied {
		t.Fatalf("email without matching ID should keep client deny: %#v", got)
	}

	got := evaluate(policies, "gmail:get", "lower@example.test", "a", "native")
	if !got.Denied {
		t.Fatalf("opaque id a must not inherit policy for A: %#v", got)
	}

	got = evaluate(policies, "gmail:get", "upper@example.test", "A", "native")
	if got.Denied {
		t.Fatalf("exact A should use the more specific allow: %#v", got)
	}
}
