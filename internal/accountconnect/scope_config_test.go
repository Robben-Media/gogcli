package accountconnect

import (
	"slices"
	"testing"
)

func TestConfiguredScopeConsentIsExplicitAndControllerLocal(t *testing.T) {
	const writeScope = "https://www.googleapis.com/auth/documents"
	provider := &fakeProvider{}
	ctrl := testController(t, provider)

	choices, err := configuredScopeChoices([]ScopeChoice{{Capability: "docs.write", Scope: writeScope}})
	if err != nil {
		t.Fatal(err)
	}
	ctrl.scopeChoices = choices

	for _, tc := range []struct {
		name   string
		scopes []string
		want   bool
	}{
		{name: "default unchanged"},
		{name: "explicit write", scopes: []string{writeScope}, want: true},
		{name: "empty", scopes: []string{}},
		{name: "unknown scope", scopes: []string{"https://www.googleapis.com/auth/unconfigured"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ctrl.StartConnect(t.Context(), ConnectRequest{PrincipalID: "fixture", Scopes: tc.scopes}); err != nil {
				t.Fatal(err)
			}

			if slices.Contains(provider.last.Scopes, writeScope) != tc.want {
				t.Fatalf("consent scopes=%v", provider.last.Scopes)
			}
		})
	}

	other := testController(t, &fakeProvider{})
	if _, ok := other.scopeForCapability("docs.write"); ok {
		t.Fatal("configured capability leaked to another controller")
	}
	copyChoices := ctrl.ScopeChoices()

	copyChoices[0].Scope = "changed"
	if ctrl.ScopeChoices()[0].Scope == "changed" {
		t.Fatal("caller mutated controller choices")
	}
}

func TestConfiguredScopesRejectMandatoryOrConflictingChoices(t *testing.T) {
	for _, extra := range [][]ScopeChoice{
		{{Capability: "docs.write", Scope: "https://www.googleapis.com/auth/documents", Required: true}},
		{{Capability: "gmail.read", Scope: "https://www.googleapis.com/auth/documents"}},
		{{Capability: "evil", Scope: "https://example.test/auth/x"}},
	} {
		if _, err := configuredScopeChoices(extra); err == nil {
			t.Fatalf("accepted invalid choices: %+v", extra)
		}
	}
}
