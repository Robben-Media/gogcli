package cmd

import (
	"fmt"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/config"
)

type policyDecision = access.Decision

func enforceCommandPolicies(kctx *kong.Context, flags *RootFlags) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if len(cfg.Policies) == 0 {
		return nil
	}

	action := commandActionID(kctx)
	if action == "" {
		return nil
	}
	service, _, _ := strings.Cut(action, ":")
	if service == "policy" {
		return nil
	}
	if !hasPolicyForService(cfg.Policies, service) {
		return nil
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := resolveClientForEmail(account, flags)
	if err != nil {
		return err
	}

	decision := evaluatePolicies(cfg.Policies, action, account, client)
	if !decision.Denied {
		return nil
	}

	target := account
	if client != "" {
		target = fmt.Sprintf("%s (client %s)", target, client)
	}
	if decision.ImplicitAllowlist {
		return usagef("no policy allows %s for %s", action, target)
	}
	if decision.DeniedBy.Reason != "" {
		return usagef("policy %q denied %s for %s: %s", decision.DeniedBy.Name, action, target, decision.DeniedBy.Reason)
	}
	return usagef("policy %q denied %s for %s", decision.DeniedBy.Name, action, target)
}

func hasPolicyForService(policies []config.Policy, service string) bool {
	service = access.CanonicalService(service)
	for _, policy := range policies {
		for _, action := range append(append([]string{}, policy.Allow...), policy.Deny...) {
			policyService, _, _ := strings.Cut(access.CanonicalAction(action), ":")
			if policyService == service {
				return true
			}
		}
	}
	return false
}

func evaluatePolicies(policies []config.Policy, action string, account string, client string) policyDecision {
	return access.Evaluate(policies, action, account, client)
}

func commandActionID(kctx *kong.Context) string {
	if kctx == nil {
		return ""
	}
	rawParts := strings.Fields(strings.ToLower(strings.TrimSpace(kctx.Command())))
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) < 2 {
		return ""
	}

	service := access.CanonicalService(parts[0])
	segments := parts[1:]
	if service == serviceGmail && len(segments) > 1 && segments[0] == "settings" {
		segments = segments[1:]
	}
	return service + ":" + strings.Join(segments, ".")
}

func policyActionMatches(pattern string, action string) bool {
	return access.MatchAction(pattern, action)
}

func normalizePolicyInputs(actions []string) []string {
	return access.CanonicalActions(actions)
}

func normalizePolicyAction(raw string) string {
	return access.CanonicalAction(raw)
}

func validatePolicyActions(policy config.Policy) error {
	for _, action := range append(append([]string{}, policy.Allow...), policy.Deny...) {
		service, _, ok := strings.Cut(action, ":")
		if !ok || service == "" {
			return usagef("invalid policy action %q (use service:command form like gmail:send or businessprofile:accounts.list)", action)
		}
	}
	return nil
}

func joinCSV(values []string) string {
	return strings.Join(values, ",")
}
