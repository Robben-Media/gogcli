package cmd

import (
	"context"
	"fmt"
)

// writeSheetsStructuralPlain emits one TSV receipt row for Sheets structural mutations.
// Header: ACTION SPREADSHEET_ID SHEET_ID TITLE RANGE STATUS
func writeSheetsStructuralPlain(ctx context.Context, action, spreadsheetID, sheetID, title, rangeSpec, status string) {
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ACTION\tSPREADSHEET_ID\tSHEET_ID\tTITLE\tRANGE\tSTATUS")
	writeTableRow(ctx, w, []string{action, spreadsheetID, sheetID, title, rangeSpec, status})
}
