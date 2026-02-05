package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsSheetCmd struct {
	Add    SheetsSheetAddCmd    `cmd:"" name:"add" help:"Add a new sheet tab"`
	Delete SheetsSheetDeleteCmd `cmd:"" name:"delete" help:"Delete a sheet tab"`
	Update SheetsSheetUpdateCmd `cmd:"" name:"update" help:"Update sheet tab properties"`
}

type SheetsSheetAddCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Title         string `name:"title" required:"" help:"Title for the new sheet tab"`
}

func (c *SheetsSheetAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(c.SpreadsheetID)
	if id == "" {
		return usage("empty spreadsheetId")
	}
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{
						Title: title,
					},
				},
			},
		},
	}).Do()
	if err != nil {
		return fmt.Errorf("add sheet: %w", err)
	}

	var addedProps *sheets.SheetProperties
	for _, reply := range resp.Replies {
		if reply.AddSheet != nil && reply.AddSheet.Properties != nil {
			addedProps = reply.AddSheet.Properties
			break
		}
	}

	if outfmt.IsJSON(ctx) {
		out := map[string]any{
			"spreadsheetId": resp.SpreadsheetId,
		}
		if addedProps != nil {
			out["sheetId"] = addedProps.SheetId
			out["title"] = addedProps.Title
		}
		return outfmt.WriteJSON(os.Stdout, out)
	}

	u := ui.FromContext(ctx)
	if addedProps != nil {
		u.Out().Printf("Added sheet %q (id=%d)", addedProps.Title, addedProps.SheetId)
	} else {
		u.Out().Println("Sheet added")
	}
	return nil
}

type SheetsSheetDeleteCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	SheetID       int64  `name:"sheet-id" required:"" help:"Sheet tab ID to delete"`
}

func (c *SheetsSheetDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(c.SpreadsheetID)
	if id == "" {
		return usage("empty spreadsheetId")
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				DeleteSheet: &sheets.DeleteSheetRequest{
					SheetId: c.SheetID,
				},
			},
		},
	}).Do()
	if err != nil {
		return fmt.Errorf("delete sheet: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"spreadsheetId":  resp.SpreadsheetId,
			"deletedSheetId": c.SheetID,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Deleted sheet tab %d", c.SheetID)
	return nil
}

type SheetsSheetUpdateCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	SheetID       int64  `name:"sheet-id" required:"" help:"Sheet tab ID to update"`
	Title         string `name:"title" help:"New title for the sheet tab"`
	FrozenRows    *int64 `name:"frozen-rows" help:"Number of frozen rows"`
	FrozenCols    *int64 `name:"frozen-cols" help:"Number of frozen columns"`
}

func (c *SheetsSheetUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(c.SpreadsheetID)
	if id == "" {
		return usage("empty spreadsheetId")
	}

	props := &sheets.SheetProperties{
		SheetId: c.SheetID,
	}

	var fields []string
	if strings.TrimSpace(c.Title) != "" {
		props.Title = c.Title
		fields = append(fields, "title")
	}
	if c.FrozenRows != nil {
		if props.GridProperties == nil {
			props.GridProperties = &sheets.GridProperties{}
		}
		props.GridProperties.FrozenRowCount = *c.FrozenRows
		fields = append(fields, "gridProperties.frozenRowCount")
	}
	if c.FrozenCols != nil {
		if props.GridProperties == nil {
			props.GridProperties = &sheets.GridProperties{}
		}
		props.GridProperties.FrozenColumnCount = *c.FrozenCols
		fields = append(fields, "gridProperties.frozenColumnCount")
	}

	if len(fields) == 0 {
		return usage("at least one of --title, --frozen-rows, or --frozen-cols required")
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
					Properties: props,
					Fields:     strings.Join(fields, ","),
				},
			},
		},
	}).Do()
	if err != nil {
		return fmt.Errorf("update sheet: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"spreadsheetId": resp.SpreadsheetId,
			"sheetId":       c.SheetID,
			"updatedFields": fields,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Updated sheet tab %d (%s)", c.SheetID, strings.Join(fields, ", "))
	return nil
}
