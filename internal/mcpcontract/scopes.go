package mcpcontract

import "slices"

// ScopeGranted reports whether a granted scope covers the required read
// capability. Only explicit Google scope alternatives are accepted; URI prefixes
// are not permission hierarchies. Resource-limited drive.file is not equivalent
// to unrestricted Drive, Docs, or Sheets read access.
func ScopeGranted(granted []string, required string) bool {
	if slices.Contains(granted, required) {
		return true
	}

	for _, scope := range readScopeAlternatives[required] {
		if slices.Contains(granted, scope) {
			return true
		}
	}

	return false
}

// Docs documents.get and Sheets spreadsheets.get/values.get explicitly accept
// drive and drive.readonly in their published Google discovery documents.
var readScopeAlternatives = map[string][]string{
	GmailReadScope:         {GmailFullScope, GmailModifyScope},
	DriveReadScope:         {"https://www.googleapis.com/auth/drive"},
	DocsReadScope:          {"https://www.googleapis.com/auth/documents", "https://www.googleapis.com/auth/drive", DriveReadScope},
	SheetsReadScope:        {"https://www.googleapis.com/auth/spreadsheets", "https://www.googleapis.com/auth/drive", DriveReadScope},
	CalendarReadScope:      {"https://www.googleapis.com/auth/calendar"},
	AnalyticsReadScope:     {"https://www.googleapis.com/auth/analytics"},
	SearchConsoleReadScope: {"https://www.googleapis.com/auth/webmasters"},
}

// ScopesSatisfied checks all required scopes for curated workflows, or one
// accepted alternative for a Google API method. An empty alternatives list
// never authorizes an API method.
func ScopesSatisfied(granted []string, def Definition) bool {
	if def.AnyScope {
		for _, required := range def.Scopes {
			if required != "" && ScopeGranted(granted, required) {
				return true
			}
		}

		return false
	}

	for _, required := range def.Scopes {
		if required == "" || !ScopeGranted(granted, required) {
			return false
		}
	}

	return true
}
