package cmd

import "context"

// writeSheetsStructuralPlain emits one TSV receipt row for Sheets structural mutations.
// Header: ACTION SPREADSHEET_ID SHEET_ID TITLE RANGE STATUS.
// Actions are the lowercase machine values create, format, copy-to, add, update, and delete.
func writeSheetsStructuralPlain(ctx context.Context, action, spreadsheetID, sheetID, title, rangeSpec string) {
	writePlainReceipt(ctx,
		[]string{"ACTION", "SPREADSHEET_ID", "SHEET_ID", "TITLE", "RANGE", "STATUS"},
		[]string{action, spreadsheetID, sheetID, title, rangeSpec, "ok"},
	)
}
