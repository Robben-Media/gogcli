package mcpcontract

const (
	GmailFullScope   = "https://mail.google.com/"
	GmailModifyScope = "https://www.googleapis.com/auth/gmail.modify"
	DocsWriteScope   = "https://www.googleapis.com/auth/documents"
	SheetsWriteScope = "https://www.googleapis.com/auth/spreadsheets"
	SlidesWriteScope = "https://www.googleapis.com/auth/presentations"
)

// WorkflowCatalog is opt-in authoring functionality, separate from the original
// curated read-only catalog. Each workflow has its own explicit write grant.
func WorkflowCatalog() []Definition {
	definitions := make([]Definition, 0, 11)
	definitions = append(definitions, []Definition{
		{Name: "mail_compose_prepare", Description: "Prepare a formatted email preview using a verified Gmail sending identity and optional signature and reply context. Does not save or send. Uses at most two API reads before retries.", Actions: []string{"gmail:workflow.prepare"}, Scopes: []string{GmailModifyScope, GmailFullScope}, AnyScope: true, Retry: SafeRead},
		{Name: "mail_compose_draft", Description: "Create a formatted Gmail draft after verifying the sending identity, signature and optional reply context. Requires explicit write grant. At most three API calls; never repeat blindly after unknown outcome.", Actions: []string{"gmail:workflow.draft"}, Scopes: []string{GmailModifyScope, GmailFullScope}, AnyScope: true, Retry: NonReplayableWrite},
		{Name: "mail_compose_send", Description: "Send a formatted email with verified Gmail sender, optional signature and derived reply headers. Requires explicit send authorization and write grant. At most three API calls; unknown outcome requires reconciliation, never blind replay.", Actions: []string{"gmail:workflow.send"}, Scopes: []string{GmailModifyScope, GmailFullScope}, AnyScope: true, Retry: NonReplayableWrite},
		{Name: "docs_create_document", Description: "Create a Google Doc with structured paragraphs and heading styles in at most two API calls. Does not share it. Returns the created ID even if population fails; never repeat creation blindly.", Actions: []string{"docs:workflow.create"}, Scopes: []string{DocsWriteScope}, Retry: NonReplayableWrite},
		{Name: "slides_create_presentation", Description: "Create a presentation with structured title/body slides in at most two API calls. Uses batched layout and text formatting; does not share or render thumbnails. Returns created ID on partial failure.", Actions: []string{"slides:workflow.create"}, Scopes: []string{SlidesWriteScope}, Retry: NonReplayableWrite},
		{Name: "sheets_create_spreadsheet", Description: "Create a spreadsheet with explicitly typed cells and optional header formatting in one API call. Strings are not interpreted as formulas; formulas require an explicit formula field. Does not share it.", Actions: []string{"sheets:workflow.create"}, Scopes: []string{SheetsWriteScope}, Retry: NonReplayableWrite},
	}...)

	return append(definitions, mediaDefinitions()...)
}
