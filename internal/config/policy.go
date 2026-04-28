package config

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type Policy struct {
	Name    string   `json:"name"`
	Account string   `json:"account,omitempty"`
	Client  string   `json:"client,omitempty"`
	Allow   []string `json:"allow,omitempty"`
	Deny    []string `json:"deny,omitempty"`
	Reason  string   `json:"reason,omitempty"`
}

var (
	errInvalidPolicyName   = errors.New("invalid policy name")
	errInvalidPolicyAction = errors.New("invalid policy action")
	errPolicyMissingTarget = errors.New("policy requires --account and/or --client")
	errPolicyMissingRules  = errors.New("policy requires --allow and/or --deny")
	errPolicyExists        = errors.New("policy already exists")
	errPolicyNotFound      = errors.New("policy not found")
	errNilConfig           = errors.New("nil config")
)

var policyNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func NormalizePolicyName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))

	if !policyNamePattern.MatchString(name) {
		return "", fmt.Errorf("%w: %q", errInvalidPolicyName, raw)
	}

	return name, nil
}

func NormalizePolicy(cfg Policy) (Policy, error) {
	name, err := NormalizePolicyName(cfg.Name)
	if err != nil {
		return Policy{}, err
	}

	cfg.Name = name

	cfg.Account = strings.ToLower(strings.TrimSpace(cfg.Account))

	if strings.TrimSpace(cfg.Client) != "" {
		normalizedClient, err := NormalizeClientNameOrDefault(cfg.Client)
		if err != nil {
			return Policy{}, err
		}

		cfg.Client = normalizedClient
	}

	cfg.Reason = strings.TrimSpace(cfg.Reason)

	cfg.Allow = normalizePolicyActions(cfg.Allow)

	cfg.Deny = normalizePolicyActions(cfg.Deny)

	if err := validatePolicyActions(cfg.Allow); err != nil {
		return Policy{}, err
	}

	if err := validatePolicyActions(cfg.Deny); err != nil {
		return Policy{}, err
	}

	if cfg.Account == "" && cfg.Client == "" {
		return Policy{}, errPolicyMissingTarget
	}

	if len(cfg.Allow) == 0 && len(cfg.Deny) == 0 {
		return Policy{}, errPolicyMissingRules
	}

	return cfg, nil
}

func normalizePolicyActions(actions []string) []string {
	if len(actions) == 0 {
		return nil
	}

	seen := map[string]struct{}{}

	out := make([]string, 0, len(actions))

	for _, action := range actions {
		action = strings.ToLower(strings.TrimSpace(action))

		if action == "" {
			continue
		}

		if _, ok := seen[action]; ok {
			continue
		}

		seen[action] = struct{}{}
		out = append(out, action)
	}

	slices.Sort(out)

	return out
}

func validatePolicyActions(actions []string) error {
	for _, action := range actions {

		service, rest, ok := strings.Cut(action, ":")

		if !ok || strings.TrimSpace(service) == "" || strings.TrimSpace(rest) == "" {
			return fmt.Errorf("%w: %q (use service:command form)", errInvalidPolicyAction, action)
		}
	}

	return nil
}

func UpsertPolicy(cfg *File, policy Policy, replace bool) error {
	if cfg == nil {
		return fmt.Errorf("%w", errNilConfig)
		return errNilConfig
	}

	normalized, err := NormalizePolicy(policy)
	if err != nil {
		return err
	}

	for i := range cfg.Policies {
		if cfg.Policies[i].Name != normalized.Name {
			continue
		}

		if !replace {
			return fmt.Errorf("%w: %s", errPolicyExists, normalized.Name)
		}

		cfg.Policies[i] = normalized

		sortPolicies(cfg)

		return nil
	}

	cfg.Policies = append(cfg.Policies, normalized)

	sortPolicies(cfg)

	return nil
}

func GetPolicy(cfg File, name string) (Policy, bool) {
	normalized, err := NormalizePolicyName(name)
	if err != nil {
		return Policy{}, false
	}

	for _, policy := range cfg.Policies {
		if policy.Name == normalized {
			return policy, true
		}
	}

	return Policy{}, false
}

func DeletePolicy(cfg *File, name string) error {
	if cfg == nil {
		return fmt.Errorf("%w", errNilConfig)
		return errNilConfig
	}

	normalized, err := NormalizePolicyName(name)
	if err != nil {
		return err
	}

	for i := range cfg.Policies {
		if cfg.Policies[i].Name != normalized {
			continue
		}

		cfg.Policies = append(cfg.Policies[:i], cfg.Policies[i+1:]...)

		return nil
	}

	return fmt.Errorf("%w: %s", errPolicyNotFound, normalized)
}

func sortPolicies(cfg *File) {
	slices.SortFunc(cfg.Policies, func(a, b Policy) int {
		return strings.Compare(a.Name, b.Name)
	})
}
