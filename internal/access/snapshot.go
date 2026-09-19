package access

import (
	"slices"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

// Snapshot is an immutable grants/policy/allowlist cut. Replace installs a new
// copy; in-flight Authorize calls keep the pointer they loaded.
type Snapshot struct {
	Grants          []mcpcontract.Grant
	Policies        []config.Policy
	AllowOperations []string
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	return Snapshot{
		Grants:          cloneGrants(snapshot.Grants),
		Policies:        clonePolicies(snapshot.Policies),
		AllowOperations: cloneStrings(snapshot.AllowOperations),
	}
}

func cloneGrants(grants []mcpcontract.Grant) []mcpcontract.Grant {
	out := make([]mcpcontract.Grant, 0, len(grants))
	for _, grant := range grants {
		out = append(out, mcpcontract.Grant{
			PrincipalID: strings.TrimSpace(grant.PrincipalID),
			AccountIDs:  cloneStrings(grant.AccountIDs),
			ClientNames: cloneStrings(grant.ClientNames),
			Operations:  cloneStrings(grant.Operations),
		})
	}

	return out
}

func clonePolicies(policies []config.Policy) []config.Policy {
	out := make([]config.Policy, 0, len(policies))
	for _, policy := range policies {
		out = append(out, clonePolicy(policy))
	}

	return out
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}

	return slices.Clone(values)
}

func operationAllowed(allow []string, operation string) bool {
	if operation == "" || len(allow) == 0 {
		return false
	}

	for _, name := range allow {
		if strings.TrimSpace(name) == operation {
			return true
		}
	}

	return false
}

func (s Snapshot) Validate() error {
	return validateSnapshot(s)
}

func validateSnapshot(snapshot Snapshot) error {
	for _, name := range snapshot.AllowOperations {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		if _, ok := mcpcontract.Lookup(name); !ok {
			return invalid("unknown operation " + name)
		}
	}

	for _, grant := range snapshot.Grants {
		for _, operation := range grant.Operations {
			operation = strings.TrimSpace(operation)
			if operation == "" {
				continue
			}

			if _, ok := mcpcontract.Lookup(operation); ok {
				continue
			}

			if matchesCatalogAction(operation) {
				continue
			}

			return invalid("unknown grant operation " + operation)
		}
	}

	return nil
}

func matchesCatalogAction(pattern string) bool {
	for _, definition := range mcpcontract.Catalog() {
		for _, action := range definition.Actions {
			if MatchAction(pattern, action) {
				return true
			}
		}
	}

	return false
}
