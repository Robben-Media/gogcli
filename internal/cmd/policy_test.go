package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func TestPolicyCreateListGetDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	_ = captureStderr(t, func() {
		if err := Execute([]string{
			"--account", "jdjb78@gmail.com",
			"--client", "personal",
			"policy", "create", "personal-gmail-safe",
			"--allow", "gmail:read,gmail:labels.create,gmail:batch-modify",
			"--deny", "gmail:send,gmail:trash,gmail:batch-delete",
			"--reason", "Jeremy allows triage and labeling, but not sending or deleting.",
		}); err != nil {
			t.Fatalf("policy create: %v", err)
		}
	})

	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	policy, ok := config.GetPolicy(cfg, "personal-gmail-safe")
	if !ok {
		t.Fatalf("expected saved policy")
	}
	if policy.Account != "jdjb78@gmail.com" || policy.Client != "personal" {
		t.Fatalf("unexpected selectors: %#v", policy)
	}
	if !containsStringSlice(policy.Allow, "gmail:batch.modify") || !containsStringSlice(policy.Deny, "gmail:batch.delete") {
		t.Fatalf("unexpected actions: %#v", policy)
	}

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "policy", "list"}); err != nil {
				t.Fatalf("policy list: %v", err)
			}
		})
	})
	var list struct {
		Policies []config.Policy `json:"policies"`
	}
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("list json: %v", err)
	}
	if len(list.Policies) != 1 || list.Policies[0].Name != "personal-gmail-safe" {
		t.Fatalf("unexpected list output: %#v", list.Policies)
	}

	getOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"policy", "get", "personal-gmail-safe"}); err != nil {
				t.Fatalf("policy get: %v", err)
			}
		})
	})
	if !strings.Contains(getOut, "reason\tJeremy allows triage and labeling, but not sending or deleting.") {
		t.Fatalf("unexpected get output: %q", getOut)
	}

	_ = captureStderr(t, func() {
		if err := Execute([]string{"policy", "delete", "personal-gmail-safe"}); err != nil {
			t.Fatalf("policy delete: %v", err)
		}
	})
}

func TestPolicyCreate_OverwriteRequiresForce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	args := []string{
		"--account", "jdjb78@gmail.com",
		"policy", "create", "safe",
		"--deny", "gmail:send",
	}
	_ = captureStderr(t, func() {
		if err := Execute(args); err != nil {
			t.Fatalf("first create: %v", err)
		}
	})
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute(args); err == nil {
				t.Fatalf("expected duplicate create to fail")
			}
		})
	})
	if !strings.Contains(errText, "policy already exists") {
		t.Fatalf("unexpected duplicate error: %q", errText)
	}

	_ = captureStderr(t, func() {
		if err := Execute([]string{
			"--force",
			"--account", "jdjb78@gmail.com",
			"policy", "create", "safe",
			"--allow", "gmail:read",
		}); err != nil {
			t.Fatalf("overwrite create: %v", err)
		}
	})
}

func containsStringSlice(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
