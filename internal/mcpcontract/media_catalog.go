package mcpcontract

// Media operations preserve the equivalent CLI policy actions. Resource-limited
// drive.file is an explicit method alternative, never a global read equivalent.
func mediaDefinitions() []Definition {
	const driveFile = "https://www.googleapis.com/auth/drive.file"
	const drive = "https://www.googleapis.com/auth/drive"

	return []Definition{
		{Name: "gmail_get_attachment", Description: "Read one Gmail attachment by exact message and attachment IDs. Returns a temporary account-bound artifact reference by default, or explicit bounded inline bytes; one API read before retries.", Actions: []string{"gmail:attachment"}, Scopes: []string{GmailReadScope}, Retry: SafeRead},
		{Name: "drive_download_file", Description: "Download bounded binary Drive file content by exact ID. One API read unless metadata is explicitly requested. Native Google documents require drive_export_file.", Actions: []string{"drive:download"}, Scopes: []string{DriveReadScope, driveFile}, AnyScope: true, Retry: SafeRead},
		{Name: "drive_export_file", Description: "Export a Google document, spreadsheet or presentation to an explicit supported MIME type. Returns a temporary account-bound artifact reference by default in one API read; no local file or sharing changes.", Actions: []string{"drive:download"}, Scopes: []string{DriveReadScope, driveFile}, AnyScope: true, Retry: SafeRead},
		{Name: "drive_create_file", Description: "Upload bounded caller-supplied bytes as a new Drive file in one multipart request, optionally using a persisted pre-generated file ID. Requires an explicit write grant; do not replay after an unknown outcome.", Actions: []string{"drive:upload"}, Scopes: []string{drive, driveFile}, AnyScope: true, Retry: NonReplayableWrite},
		{Name: "drive_update_file", Description: "Replace content of the exact Drive file ID with bounded caller-supplied bytes. Optional expected_version adds a metadata read and atomic If-Match to reject concurrent edits. Requires an explicit write grant; reconcile unknown outcomes before repeating.", Actions: []string{"drive:upload"}, Scopes: []string{drive, driveFile}, AnyScope: true, Retry: NonReplayableWrite},
	}
}
