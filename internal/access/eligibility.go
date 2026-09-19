package access

import (
	"slices"
	"strings"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func mcpReady(identity mcpcontract.Identity) bool {
	return identity.AuthMode == authModeOAuth &&
		strings.TrimSpace(identity.Subject) != "" &&
		strings.TrimSpace(identity.Email) != "" &&
		identity.Generation > 0
}

func (a *Authorizer) identityEligible(snapshot Snapshot, principalID string, identity mcpcontract.Identity, operation string, actions []string) bool {
	if !mcpReady(identity) || identity.PrincipalID != principalID {
		return false
	}

	def, ok := mcpcontract.Lookup(operation)
	if !ok || def.Local {
		return false
	}

	if !operationAllowed(snapshot.AllowOperations, operation) {
		return false
	}

	if !a.grantMatchesActions(snapshot, principalID, identity, operation, actions) {
		return false
	}

	if !a.actionsAllowed(snapshot, identity, actions) {
		return false
	}

	if len(missingScopes(identity.Scopes, def.Scopes)) > 0 {
		return false
	}

	return true
}

// Capabilities returns MCP-advertised capabilities for one ready account.
func (a *Authorizer) Capabilities(identity mcpcontract.Identity) []string {
	snapshot := a.load()
	caps := make([]string, 0, 8)

	for _, def := range mcpcontract.Catalog() {
		if def.Local {
			continue
		}

		eligible := false

		if def.AnyAction {
			for _, action := range def.Actions {
				if a.identityEligible(snapshot, identity.PrincipalID, identity, def.Name, []string{action}) {
					eligible = true
					break
				}
			}
		} else if a.identityEligible(snapshot, identity.PrincipalID, identity, def.Name, def.Actions) {
			eligible = true
		}

		if !eligible {
			continue
		}

		name := capabilityName(def.Name)
		if name != "" && !slices.Contains(caps, name) {
			caps = append(caps, name)
		}
	}

	slices.Sort(caps)

	return caps
}

func capabilityName(operation string) string {
	switch {
	case strings.HasPrefix(operation, "gmail_"):
		return "gmail.read"
	case strings.HasPrefix(operation, "drive_"):
		return "drive.read"
	case strings.HasPrefix(operation, "docs_"):
		return "docs.read"
	case strings.HasPrefix(operation, "calendar_"):
		return "calendar.read"
	case strings.HasPrefix(operation, "analytics_"):
		return "analytics.read"
	case strings.HasPrefix(operation, "searchconsole_"):
		return "searchconsole.read"
	case strings.HasPrefix(operation, "sheets_"):
		return "sheets.read"
	default:
		return ""
	}
}
