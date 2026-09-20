package accountconnect

import (
	"fmt"
	"slices"
	"strings"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const maxAdditionalScopeChoices = 64

func configuredScopeChoices(extra []ScopeChoice) ([]ScopeChoice, error) {
	if len(extra) > maxAdditionalScopeChoices {
		return nil, fmt.Errorf("%w: too many additional scope choices", ErrMissingScopes)
	}
	choices := ScopeChoices()

	names := make(map[string]string, len(choices)+len(extra))
	for _, choice := range choices {
		names[choice.Capability] = choice.Scope
	}

	for _, choice := range extra {
		if choice.Required || strings.TrimSpace(choice.Capability) != choice.Capability || choice.Capability == "" || len(choice.Capability) > 160 || !(strings.HasPrefix(choice.Scope, "https://www.googleapis.com/auth/") || choice.Scope == "https://mail.google.com/") || strings.ContainsAny(choice.Scope, " \t\r\n?#") {
			return nil, fmt.Errorf("%w: invalid additional scope choice", ErrMissingScopes)
		}

		if previous, ok := names[choice.Capability]; ok {
			if previous != choice.Scope {
				return nil, fmt.Errorf("%w: conflicting scope capability", ErrMissingScopes)
			}

			continue
		}
		names[choice.Capability] = choice.Scope
		choices = append(choices, choice)
	}

	return choices, nil
}

// ScopeChoices returns the server-configured capabilities for this controller.
// Added write/admin scopes are optional and never join the default consent set.
func (c *Controller) ScopeChoices() []ScopeChoice {
	return append([]ScopeChoice(nil), c.scopeChoices...)
}

func (c *Controller) scopeForCapability(name string) (string, bool) {
	for _, choice := range c.scopeChoices {
		if choice.Capability == strings.TrimSpace(name) {
			return choice.Scope, true
		}
	}

	return "", false
}

func (c *Controller) unionScopes(sets ...[]string) []string {
	allowed := map[string]bool{scopeOpenID: true, scopeEmail: true}
	for _, choice := range c.scopeChoices {
		allowed[choice.Scope] = true
	}
	seen := make(map[string]bool)
	var out []string

	for _, set := range sets {
		for _, scope := range set {
			scope = strings.TrimSpace(scope)
			if allowed[scope] && !seen[scope] {
				out = append(out, scope)
				seen[scope] = true
			}
		}
	}

	return out
}

func (c *Controller) accountView(rec Record) AccountView {
	view := accountView(rec)
	for _, choice := range c.scopeChoices {
		if mcpcontract.ScopeGranted(rec.Scopes, choice.Scope) && !slices.Contains(view.Capabilities, choice.Capability) {
			view.Capabilities = append(view.Capabilities, choice.Capability)
		}
	}

	slices.Sort(view.Capabilities)

	return view
}
