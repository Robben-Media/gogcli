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
	GmailReadScope:         {"https://mail.google.com/", "https://www.googleapis.com/auth/gmail.modify"},
	DriveReadScope:         {"https://www.googleapis.com/auth/drive"},
	DocsReadScope:          {"https://www.googleapis.com/auth/documents", "https://www.googleapis.com/auth/drive", DriveReadScope},
	SheetsReadScope:        {"https://www.googleapis.com/auth/spreadsheets", "https://www.googleapis.com/auth/drive", DriveReadScope},
	CalendarReadScope:      {"https://www.googleapis.com/auth/calendar"},
	AnalyticsReadScope:     {"https://www.googleapis.com/auth/analytics"},
	SearchConsoleReadScope: {"https://www.googleapis.com/auth/webmasters"},
}
