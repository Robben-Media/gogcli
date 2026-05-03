package cmd

import (
	"fmt"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/config"
)

var commandServiceAliases = map[string]string{
	"bq":       "bigquery",
	"business": "businessprofile",
	"email":    serviceGmail,
	"ga":       "analytics",
	"ga4":      "analytics",
	"gbp":      "businessprofile",
	"gsc":      "searchconsole",
	"gtm":      "tagmanager",
	"mail":     serviceGmail,
	"sc":       "searchconsole",
	"yt":       "youtube",
}

type policyDecision struct {
	Denied            bool
	DeniedBy          config.Policy
	ImplicitAllowlist bool
}

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
	for _, policy := range policies {
		for _, action := range append(append([]string{}, policy.Allow...), policy.Deny...) {
			policyService, _, _ := strings.Cut(action, ":")
			if normalizeCommandService(policyService) == service {
				return true
			}
		}
	}
	return false
}

func evaluatePolicies(policies []config.Policy, action string, account string, client string) policyDecision {
	var candidates []config.Policy
	bestSpecificity := -1
	for _, policy := range policies {
		if !policyApplies(policy, account, client) {
			continue
		}
		specificity := policySpecificity(policy)
		if specificity > bestSpecificity {
			bestSpecificity = specificity
			candidates = []config.Policy{policy}
			continue
		}
		if specificity == bestSpecificity {
			candidates = append(candidates, policy)
		}
	}
	if len(candidates) == 0 {
		return policyDecision{}
	}

	for _, policy := range candidates {
		if matchesAnyAction(policy.Deny, action) {
			return policyDecision{Denied: true, DeniedBy: policy}
		}
	}

	hasAllowlist := false
	for _, policy := range candidates {
		if len(policy.Allow) == 0 {
			continue
		}
		hasAllowlist = true
		if matchesAnyAction(policy.Allow, action) {
			return policyDecision{}
		}
	}

	if hasAllowlist {
		return policyDecision{Denied: true, ImplicitAllowlist: true}
	}

	return policyDecision{}
}

func policyApplies(policy config.Policy, account string, client string) bool {
	if policy.Account != "" && !strings.EqualFold(strings.TrimSpace(policy.Account), strings.TrimSpace(account)) {
		return false
	}
	if policy.Client != "" && !strings.EqualFold(strings.TrimSpace(policy.Client), strings.TrimSpace(client)) {
		return false
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

	service := normalizeCommandService(parts[0])
	segments := parts[1:]
	if service == serviceGmail && len(segments) > 1 && segments[0] == "settings" {
		segments = segments[1:]
	}
	return service + ":" + strings.Join(segments, ".")
}

func normalizeCommandService(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if canonical, ok := commandServiceAliases[raw]; ok {
		return canonical
	}
	return raw
}

func matchesAnyAction(patterns []string, action string) bool {
	for _, pattern := range patterns {
		if policyActionMatches(pattern, action) {
			return true
		}
	}
	return false
}

// policyActionMatches supports exact action IDs plus a small set of shorthands.
// The `read` and `reply` expansions are Gmail-only convenience aliases:
// `gmail:read` expands to common read-only Gmail actions, and `gmail:reply`
// expands to send actions used for replies.
func policyActionMatches(pattern string, action string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	action = strings.ToLower(strings.TrimSpace(action))
	if pattern == "" || action == "" {
		return false
	}
	if pattern == action {
		return true
	}

	patternService, patternRest, ok := strings.Cut(pattern, ":")
	if !ok {
		return false
	}
	actionService, actionRest, ok := strings.Cut(action, ":")
	if !ok {
		return false
	}
	if normalizeCommandService(patternService) != normalizeCommandService(actionService) {
		return false
	}
	if patternRest == "*" || patternRest == "all" {
		return true
	}
	if strings.HasSuffix(patternRest, ".*") {
		prefix := strings.TrimSuffix(patternRest, ".*")
		return actionRest == prefix || strings.HasPrefix(actionRest, prefix+".")
	}
	if patternRest == "read" {
		return isReadLikeAction(normalizeCommandService(actionService), actionRest)
	}
	if patternRest == "reply" {
		return actionRest == "send" || strings.HasSuffix(actionRest, ".send")
	}
	if !strings.Contains(patternRest, ".") {
		last := actionRest
		if idx := strings.LastIndex(last, "."); idx >= 0 {
			last = last[idx+1:]
		}
		return last == patternRest
	}
	return false
}

func isReadLikeAction(service string, actionRest string) bool {
	if service != serviceGmail {
		return false
	}
	last := actionRest
	if idx := strings.LastIndex(last, "."); idx >= 0 {
		last = last[idx+1:]
	}
	switch last {
	case "attachment", "attachments", "get", "history", "list", "opens", "search", "status", "url":
		return true
	default:
		return false
	}
}

func normalizePolicyInputs(actions []string) []string {
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		if normalized := normalizePolicyAction(action); normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
}

func normalizePolicyAction(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	service, rest, ok := strings.Cut(raw, ":")
	if !ok {
		return raw
	}
	service = normalizeCommandService(service)
	rest = strings.TrimSpace(rest)
	rest = strings.ReplaceAll(rest, "-", ".")
	rest = strings.ReplaceAll(rest, " ", ".")
	for strings.Contains(rest, "..") {
		rest = strings.ReplaceAll(rest, "..", ".")
	}
	rest = strings.Trim(rest, ".")
	if service == serviceGmail {
		if rest == "settings" {
			rest = "*"
		}
		rest = strings.TrimPrefix(rest, "settings.")
	}
	if rest == "" {
		return service + ":*"
	}
	return service + ":" + rest
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
