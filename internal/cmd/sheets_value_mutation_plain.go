package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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

func sheetsMutationSpreadsheetID(requestID, responseID string) string {
	if id := strings.TrimSpace(responseID); id != "" {
		return id
	}
	return requestID
}

func formatSheetsMutationCount(v int64) string {
	return strconv.FormatInt(v, 10)
}

func newSheetsValueMutationUpdateRow(action, spreadsheetID, updatedRange string, updatedRows, updatedColumns, updatedCells int64) sheetsValueMutationPlainRow {
	return sheetsValueMutationPlainRow{
		Action:         action,
		SpreadsheetID:  spreadsheetID,
		Range:          updatedRange,
		UpdatedRows:    formatSheetsMutationCount(updatedRows),
		UpdatedColumns: formatSheetsMutationCount(updatedColumns),
		UpdatedCells:   formatSheetsMutationCount(updatedCells),
		Status:         "ok",
	}
}

func sheetsValueMutationRowsWithTotals(action, spreadsheetID string, rows []sheetsValueMutationPlainRow, totalRows, totalColumns, totalCells, totalSheets int64) []sheetsValueMutationPlainRow {
	rows = append(rows, sheetsValueMutationPlainRow{
		Action:         action,
		SpreadsheetID:  spreadsheetID,
		UpdatedRows:    formatSheetsMutationCount(totalRows),
		UpdatedColumns: formatSheetsMutationCount(totalColumns),
		UpdatedCells:   formatSheetsMutationCount(totalCells),
		UpdatedSheets:  formatSheetsMutationCount(totalSheets),
		Status:         "ok",
	})
	return rows
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

func writeSheetsValueMutationPlainSingle(ctx context.Context, action, spreadsheetID, rangeSpec string, updatedRows, updatedColumns, updatedCells int64) {
	writeSheetsValueMutationPlain(ctx, []sheetsValueMutationPlainRow{{
		Action:         action,
		SpreadsheetID:  spreadsheetID,
		Range:          rangeSpec,
		UpdatedRows:    formatSheetsMutationCount(updatedRows),
		UpdatedColumns: formatSheetsMutationCount(updatedColumns),
		UpdatedCells:   formatSheetsMutationCount(updatedCells),
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
