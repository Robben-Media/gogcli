package cmd

import (
	"context"
	"fmt"
	"strconv"
)

// sheetsValueMutationPlainRow is one TSV receipt for a Sheets value mutation.
// Header: ACTION SPREADSHEET_ID RANGE UPDATED_ROWS UPDATED_COLUMNS UPDATED_CELLS UPDATED_SHEETS STATUS
type sheetsValueMutationPlainRow struct {
	Action         string
	SpreadsheetID  string
	Range          string
	UpdatedRows    string
	UpdatedColumns string
	UpdatedCells   string
	UpdatedSheets  string
	Status         string
}

func formatInt64Field(v int64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}

func writeSheetsValueMutationPlain(ctx context.Context, rows []sheetsValueMutationPlainRow) {
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ACTION\tSPREADSHEET_ID\tRANGE\tUPDATED_ROWS\tUPDATED_COLUMNS\tUPDATED_CELLS\tUPDATED_SHEETS\tSTATUS")
	for _, row := range rows {
		status := row.Status
		if status == "" {
			status = "ok"
		}
		writeTableRow(ctx, w, []string{
			row.Action,
			row.SpreadsheetID,
			row.Range,
			row.UpdatedRows,
			row.UpdatedColumns,
			row.UpdatedCells,
			row.UpdatedSheets,
			status,
		})
	}
}

func writeSheetsValueMutationPlainSingle(ctx context.Context, action, spreadsheetID, rangeSpec string, updatedRows, updatedColumns, updatedCells, updatedSheets int64) {
	writeSheetsValueMutationPlain(ctx, []sheetsValueMutationPlainRow{{
		Action:         action,
		SpreadsheetID:  spreadsheetID,
		Range:          rangeSpec,
		UpdatedRows:    formatInt64Field(updatedRows),
		UpdatedColumns: formatInt64Field(updatedColumns),
		UpdatedCells:   formatInt64Field(updatedCells),
		UpdatedSheets:  formatInt64Field(updatedSheets),
		Status:         "ok",
	}})
}

func writeSheetsValueMutationPlainClears(ctx context.Context, action, spreadsheetID string, ranges []string) {
	if len(ranges) == 0 {
		writeSheetsValueMutationPlain(ctx, []sheetsValueMutationPlainRow{{
			Action:        action,
			SpreadsheetID: spreadsheetID,
			Status:        "ok",
		}})
		return
	}
	rows := make([]sheetsValueMutationPlainRow, 0, len(ranges))
	for _, r := range ranges {
		rows = append(rows, sheetsValueMutationPlainRow{
			Action:        action,
			SpreadsheetID: spreadsheetID,
			Range:         r,
			Status:        "ok",
		})
	}
	writeSheetsValueMutationPlain(ctx, rows)
}
