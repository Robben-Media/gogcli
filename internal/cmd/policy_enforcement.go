package cmd

import (
	"fmt"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/config"
)

func enforceCommandPolicies(kctx *kong.Context, flags *RootFlags) error {
	if isSchemaCommand(kctx.Command()) {
		return nil
	}

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

	decision := access.Evaluate(cfg.Policies, action, account, client)
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

func commandActionID(kctx *kong.Context) string {
	parts := commandPath(kctx)
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
