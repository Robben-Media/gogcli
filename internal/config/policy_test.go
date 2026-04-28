package config

import (
	"errors"
	"testing"
)

func TestNormalizePolicy(t *testing.T) {
	policy, err := NormalizePolicy(Policy{
		Name:    " Personal-Gmail-Safe ",
		Account: "User@Example.com",
		Client:  "Personal",
		Allow:   []string{"gmail:search", "gmail:search", " gmail:labels.create "},
		Deny:    []string{"gmail:send"},
		Reason:  " keep it safe ",
	})
	if err != nil {
		t.Fatalf("NormalizePolicy: %v", err)
	}

	if policy.Name != "personal-gmail-safe" {
		t.Fatalf("unexpected name: %q", policy.Name)
	}

	if policy.Account != "user@example.com" {
		t.Fatalf("unexpected account: %q", policy.Account)
	}

	if policy.Client != "personal" {
		t.Fatalf("unexpected client: %q", policy.Client)
	}

	if len(policy.Allow) != 2 || policy.Allow[0] != "gmail:labels.create" || policy.Allow[1] != "gmail:search" {
		t.Fatalf("unexpected allow: %#v", policy.Allow)
	}

	if policy.Reason != "keep it safe" {
		t.Fatalf("unexpected reason: %q", policy.Reason)
	}
}

func TestNormalizePolicy_RequiresTarget(t *testing.T) {

	if _, err := NormalizePolicy(Policy{Name: "x", Deny: []string{"gmail:send"}}); !errors.Is(err, errPolicyMissingTarget) {
		t.Fatalf("expected missing target, got %v", err)
	}
}

func TestNormalizePolicy_RequiresRules(t *testing.T) {

	if _, err := NormalizePolicy(Policy{Name: "x", Account: "a@b.com"}); !errors.Is(err, errPolicyMissingRules) {
		t.Fatalf("expected missing rules, got %v", err)
	}
}

func TestNormalizePolicy_RejectsInvalidActions(t *testing.T) {
	if _, err := NormalizePolicy(Policy{
		Name:    "x",
		Account: "a@b.com",
		Deny:    []string{"send"},
	}); !errors.Is(err, errInvalidPolicyAction) {
		t.Fatalf("expected invalid action, got %v", err)
	}
}

func TestUpsertDeleteGetPolicy(t *testing.T) {
	var cfg File

	err := UpsertPolicy(&cfg, Policy{
		Name:    "b",
		Account: "a@b.com",
		Deny:    []string{"gmail:send"},
	}, false)
	if err != nil {
		t.Fatalf("UpsertPolicy first: %v", err)
	}

	err = UpsertPolicy(&cfg, Policy{
		Name:    "a",
		Account: "a@b.com",
		Allow:   []string{"gmail:read"},
	}, false)
	if err != nil {
		t.Fatalf("UpsertPolicy second: %v", err)
	}

	if len(cfg.Policies) != 2 || cfg.Policies[0].Name != "a" || cfg.Policies[1].Name != "b" {
		t.Fatalf("unexpected policies: %#v", cfg.Policies)
	}

	if _, ok := GetPolicy(cfg, "A"); !ok {
		t.Fatalf("expected get policy to work")
	}

	if err := UpsertPolicy(&cfg, Policy{
		Name:    "a",
		Account: "a@b.com",
		Deny:    []string{"gmail:trash"},
	}, false); !errors.Is(err, errPolicyExists) {
		t.Fatalf("expected exists error, got %v", err)
	}

	if err := DeletePolicy(&cfg, "a"); err != nil {
		t.Fatalf("DeletePolicy: %v", err)
	}

	if len(cfg.Policies) != 1 || cfg.Policies[0].Name != "b" {
		t.Fatalf("unexpected policies after delete: %#v", cfg.Policies)
	}

	if err := DeletePolicy(&cfg, "missing"); !errors.Is(err, errPolicyNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
