package access

import (
	"strings"

	"github.com/steipete/gogcli/internal/config"
)

// Decision is the pure specificity/deny result. Grants are applied separately
// and cannot override a deny.
type Decision struct {
	Denied            bool
	DeniedBy          config.Policy
	ImplicitAllowlist bool
}

// Evaluate preserves CLI policy semantics: most specific matching policies win,
// explicit deny beats allow, and an allowlist that does not mention the action
// is an implicit deny.
func Evaluate(policies []config.Policy, action string, account string, client string) Decision {
	return evaluate(policies, action, account, "", client)
}

func evaluate(policies []config.Policy, action string, account string, accountID string, client string) Decision {
	action = CanonicalAction(action)
	if action == "" {
		return Decision{}
	}

	var candidates []config.Policy
	bestSpecificity := -1

	for _, policy := range policies {
		if !policyApplies(policy, account, accountID, client) {
			continue
		}

		specificity := policySpecificity(policy)
		if specificity > bestSpecificity {
			bestSpecificity = specificity
			candidates = []config.Policy{clonePolicy(policy)}

			continue
		}

		if specificity == bestSpecificity {
			candidates = append(candidates, clonePolicy(policy))
		}
	}

	if len(candidates) == 0 {
		return Decision{}
	}

	for _, policy := range candidates {
		if matchesAnyAction(policy.Deny, action) {
			return Decision{Denied: true, DeniedBy: policy}
		}
	}

	hasAllowlist := false

	for _, policy := range candidates {
		if len(policy.Allow) == 0 {
			continue
		}

		hasAllowlist = true

		if matchesAnyAction(policy.Allow, action) {
			return Decision{}
		}
	}

	if hasAllowlist {
		return Decision{Denied: true, ImplicitAllowlist: true}
	}

	return Decision{}
}

func policyApplies(policy config.Policy, account string, accountID string, client string) bool {
	if target := strings.TrimSpace(policy.Account); target != "" {
		emailMatch := equalFold(target, account)

		idMatch := target == strings.TrimSpace(accountID)
		if !emailMatch && !idMatch {
			return false
		}
	}

	if target := strings.TrimSpace(policy.Client); target != "" {
		if !equalFold(target, client) {
			return false
		}
	}

	return true
}

func policySpecificity(policy config.Policy) int {
	score := 0
	if strings.TrimSpace(policy.Account) != "" {
		score++
	}

	if strings.TrimSpace(policy.Client) != "" {
		score++
	}

	return score
}

func clonePolicy(policy config.Policy) config.Policy {
	policy.Allow = append([]string(nil), policy.Allow...)
	policy.Deny = append([]string(nil), policy.Deny...)

	return policy
}

func equalFold(a string, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
