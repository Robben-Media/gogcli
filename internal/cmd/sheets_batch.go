package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsBatchGetCmd struct {
	SpreadsheetID string   `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Ranges        []string `arg:"" name:"ranges" help:"Ranges to read (e.g., Sheet1!A1:D10)"`
}

func writeSheetsValueRangesPlain(ctx context.Context, valueRanges []*sheets.ValueRange) {
	writeTableRow(ctx, os.Stdout, []string{"RANGE", "ROW_INDEX", "COLUMN_INDEX", "VALUE"})
	for _, valueRange := range valueRanges {
		if valueRange == nil {
			continue
		}
		for majorIndex, majorValues := range valueRange.Values {
			for minorIndex, cell := range majorValues {
				row, column := majorIndex, minorIndex
				if strings.EqualFold(valueRange.MajorDimension, "COLUMNS") {
					row, column = minorIndex, majorIndex
				}
				value := ""
				if cell != nil {
					value = fmt.Sprintf("%v", cell)
				}
				writeTableRow(ctx, os.Stdout, []string{
					valueRange.Range,
					fmt.Sprintf("%d", row),
					fmt.Sprintf("%d", column),
					value,
				})
			}
		}
	}
}

func (c *SheetsBatchGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(c.SpreadsheetID)
	if id == "" {
		return usage("empty spreadsheetId")
	}
	if len(c.Ranges) == 0 {
		return usage("at least one range required")
	}

	cleaned := make([]string, len(c.Ranges))
	for i, r := range c.Ranges {
		cleaned[i] = cleanRange(r)
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Values.BatchGet(id).Ranges(cleaned...).Do()
	if err != nil {
		return fmt.Errorf("batch get: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": resp.SpreadsheetId,
			"valueRanges":   resp.ValueRanges,
		})
	}
	if outfmt.IsPlain(ctx) {
		writeSheetsValueRangesPlain(ctx, resp.ValueRanges)
		return nil
	}

	u := ui.FromContext(ctx)
	for _, vr := range resp.ValueRanges {
		u.Out().Printf("\n--- %s ---", vr.Range)
		for _, row := range vr.Values {
			cells := make([]string, len(row))
			for i, cell := range row {
				cells[i] = fmt.Sprintf("%v", cell)
			}
			u.Out().Println(strings.Join(plainTableFields(ctx, cells), "\t"))
		}
	}
	return nil
}

type SheetsBatchUpdateCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	ValuesJSON    string `name:"values-json" required:"" help:"JSON array of {range, values} objects"`
	InputOption   string `name:"input-option" help:"Value input option: RAW|USER_ENTERED" default:"USER_ENTERED"`
}

func (c *SheetsBatchUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(c.SpreadsheetID)
	if id == "" {
		return usage("empty spreadsheetId")
	}

	var entries []struct {
		Range  string          `json:"range"`
		Values [][]interface{} `json:"values"`
	}
	if unmarshalErr := json.Unmarshal([]byte(c.ValuesJSON), &entries); unmarshalErr != nil {
		return fmt.Errorf("invalid values-json: %w", unmarshalErr)
	}
	if len(entries) == 0 {
		return usage("values-json must contain at least one entry")
	}

	data := make([]*sheets.ValueRange, len(entries))
	for i, e := range entries {
		data[i] = &sheets.ValueRange{
			Range:  e.Range,
			Values: e.Values,
		}
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	inputOption := strings.TrimSpace(c.InputOption)
	if inputOption == "" {
		inputOption = "USER_ENTERED"
	}

	resp, err := svc.Spreadsheets.Values.BatchUpdate(id, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: inputOption,
		Data:             data,
	}).Do()
	if err != nil {
		return fmt.Errorf("batch update: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId":       resp.SpreadsheetId,
			"totalUpdatedRows":    resp.TotalUpdatedRows,
			"totalUpdatedColumns": resp.TotalUpdatedColumns,
			"totalUpdatedCells":   resp.TotalUpdatedCells,
			"totalUpdatedSheets":  resp.TotalUpdatedSheets,
		})
	}
	if outfmt.IsPlain(ctx) {
		spreadsheetIDOut := sheetsMutationSpreadsheetID(id, resp.SpreadsheetId)
		rows := make([]sheetsValueMutationPlainRow, 0, len(resp.Responses)+1)
		for _, response := range resp.Responses {
			if response != nil {
				rows = append(rows, newSheetsValueMutationUpdateRow("batch-update", spreadsheetIDOut, response.UpdatedRange, response.UpdatedRows, response.UpdatedColumns, response.UpdatedCells))
			}
		}
		writeSheetsValueMutationPlain(ctx, sheetsValueMutationRowsWithTotals(
			"batch-update",
			spreadsheetIDOut,
			rows,
			resp.TotalUpdatedRows,
			resp.TotalUpdatedColumns,
			resp.TotalUpdatedCells,
			resp.TotalUpdatedSheets,
		))
		return nil
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Updated %d cells across %d sheets", resp.TotalUpdatedCells, resp.TotalUpdatedSheets)
	return nil
}
