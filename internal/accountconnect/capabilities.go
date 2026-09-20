package accountconnect

import (
	"slices"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func capabilityPairs() []struct {
	scope string
	name  string
} {
	return []struct {
		scope string
		name  string
	}{
		{mcpcontract.GmailReadScope, "gmail.read"},
		{mcpcontract.DriveReadScope, "drive.read"},
		{mcpcontract.DocsReadScope, "docs.read"},
		{mcpcontract.CalendarReadScope, "calendar.read"},
		{mcpcontract.AnalyticsReadScope, "analytics.read"},
		{mcpcontract.SearchConsoleReadScope, "searchconsole.read"},
		{mcpcontract.SheetsReadScope, "sheets.read"},
	}
}

func capabilitiesForScopes(scopes []string) []string {
	out := make([]string, 0, len(capabilityPairs()))
	for _, pair := range capabilityPairs() {
		if mcpcontract.ScopeGranted(scopes, pair.scope) {
			out = append(out, pair.name)
		}
	}

	slices.Sort(out)

	return out
}

func accountView(rec Record) AccountView {
	return AccountView{
		AccountID:      rec.AccountID,
		Email:          rec.Email,
		Label:          rec.Label,
		ClientName:     rec.ClientName,
		AuthMode:       rec.AuthMode,
		Scopes:         append([]string(nil), rec.Scopes...),
		Capabilities:   capabilitiesForScopes(rec.Scopes),
		State:          rec.State,
		CleanupPending: rec.Cleanup != nil,
		UpdatedAt:      rec.UpdatedAt,
	}
}

// DefaultConnectScopes is the consent set when the UI submits no capabilities:
// OpenID identity plus Gmail readonly. An explicit capability list requests
// only those scopes plus OpenID identity.
func DefaultConnectScopes() []string {
	return []string{
		scopeOpenID,
		scopeEmail,
		mcpcontract.GmailReadScope,
	}
}

// AllowedConnectScopes is the full read-only pilot set the handler may accept.
func AllowedConnectScopes() []string {
	return []string{
		scopeOpenID,
		scopeEmail,
		mcpcontract.GmailReadScope,
		mcpcontract.DriveReadScope,
		mcpcontract.DocsReadScope,
		mcpcontract.CalendarReadScope,
		mcpcontract.AnalyticsReadScope,
		mcpcontract.SearchConsoleReadScope,
		mcpcontract.SheetsReadScope,
	}
}

// ScopeChoices is the server-provided readonly capability list for the UI.
// None are required; Gmail is a default checked selection, not a forced grant.
func ScopeChoices() []ScopeChoice {
	pairs := capabilityPairs()

	out := make([]ScopeChoice, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, ScopeChoice{Capability: pair.name, Scope: pair.scope})
	}

	return out
}
