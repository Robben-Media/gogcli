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

// SheetsDeveloperMetadataCmd contains subcommands for developer metadata.
type SheetsDeveloperMetadataCmd struct {
	Get    SheetsDeveloperMetadataGetCmd    `cmd:"" name:"get" help:"Get a developer metadata entry by ID"`
	Search SheetsDeveloperMetadataSearchCmd `cmd:"" name:"search" help:"Search developer metadata"`
}

// SheetsDeveloperMetadataGetCmd retrieves a single developer metadata entry by ID.
type SheetsDeveloperMetadataGetCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	MetadataID    int64  `arg:"" name:"metadataId" help:"Developer metadata ID"`
}

func (c *SheetsDeveloperMetadataGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.DeveloperMetadata.Get(spreadsheetID, c.MetadataID).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"metadata": resp})
	}

	u.Out().Printf("ID\t%d", resp.MetadataId)
	u.Out().Printf("Key\t%s", resp.MetadataKey)
	u.Out().Printf("Value\t%s", resp.MetadataValue)
	if resp.Location != nil {
		loc := formatDevMetadataLocation(resp.Location)
		u.Out().Printf("Location\t%s", loc)
	}
	u.Out().Printf("Visibility\t%s", resp.Visibility)
	return nil
}

func formatDevMetadataLocation(loc *sheets.DeveloperMetadataLocation) string {
	switch loc.LocationType {
	case "SPREADSHEET":
		return "SPREADSHEET"
	case "SHEET":
		return fmt.Sprintf("SHEET:%d", loc.SheetId)
	case "ROW", "COLUMN":
		if loc.DimensionRange != nil {
			return fmt.Sprintf("%s:sheet=%d,start=%d,end=%d",
				loc.LocationType,
				loc.DimensionRange.SheetId,
				loc.DimensionRange.StartIndex,
				loc.DimensionRange.EndIndex)
		}
		return loc.LocationType
	default:
		return loc.LocationType
	}
}

// SheetsDeveloperMetadataSearchCmd searches developer metadata.
type SheetsDeveloperMetadataSearchCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Key           string `name:"key" help:"Metadata key to search for"`
	Value         string `name:"value" help:"Metadata value to match"`
	LocationType  string `name:"location-type" help:"Location type: ROW, COLUMN, SHEET, or SPREADSHEET"`
	Visibility    string `name:"visibility" help:"Visibility: DOCUMENT or PROJECT"`
}

func (c *SheetsDeveloperMetadataSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	// Build the data filter
	df := &sheets.DataFilter{}

	key := strings.TrimSpace(c.Key)
	value := strings.TrimSpace(c.Value)
	locType := strings.TrimSpace(c.LocationType)
	visibility := strings.TrimSpace(c.Visibility)

	if key == "" && value == "" && locType == "" && visibility == "" {
		return usage("at least one filter criterion (--key, --value, --location-type, or --visibility) is required")
	}

	if key != "" || value != "" {
		df.DeveloperMetadataLookup = &sheets.DeveloperMetadataLookup{}
		if key != "" {
			df.DeveloperMetadataLookup.MetadataKey = key
		}
		if value != "" {
			df.DeveloperMetadataLookup.MetadataValue = value
		}
		if visibility != "" {
			df.DeveloperMetadataLookup.Visibility = visibility
		}
		if locType != "" {
			df.DeveloperMetadataLookup.LocationType = locType
		}
	}

	dataFilters := []*sheets.DataFilter{df}

	req := &sheets.SearchDeveloperMetadataRequest{
		DataFilters: dataFilters,
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.DeveloperMetadata.Search(spreadsheetID, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"matchedDeveloperMetadata": resp.MatchedDeveloperMetadata,
		})
	}

	if len(resp.MatchedDeveloperMetadata) == 0 {
		u.Err().Println("No matching metadata found")
		return nil
	}

	for _, m := range resp.MatchedDeveloperMetadata {
		md := m.DeveloperMetadata
		u.Out().Printf("ID\t%d", md.MetadataId)
		u.Out().Printf("Key\t%s", md.MetadataKey)
		u.Out().Printf("Value\t%s", md.MetadataValue)
		if md.Location != nil {
			u.Out().Printf("Location\t%s", formatDevMetadataLocation(md.Location))
		}
		u.Out().Println("")
	}
	return nil
}

// SheetsGetByFilterCmd retrieves a spreadsheet filtered by DataFilter.
type SheetsGetByFilterCmd struct {
	SpreadsheetID   string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	FiltersJSON     string `name:"filters-json" help:"JSON array of DataFilter objects" required:""`
	IncludeGridData bool   `name:"include-grid-data" help:"Include cell data in response"`
}

func (c *SheetsGetByFilterCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	filtersJSON, err := readJSONFromFlag(c.FiltersJSON, "filters")
	if err != nil {
		return fmt.Errorf("read filters: %w", err)
	}

	var dataFilters []*sheets.DataFilter
	if unmarshalErr := json.Unmarshal([]byte(filtersJSON), &dataFilters); unmarshalErr != nil {
		return fmt.Errorf("invalid filters JSON: %w", unmarshalErr)
	}

	req := &sheets.GetSpreadsheetByDataFilterRequest{
		DataFilters:     dataFilters,
		IncludeGridData: c.IncludeGridData,
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.GetByDataFilter(spreadsheetID, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"spreadsheetId": resp.SpreadsheetId,
			"title":         resp.Properties.Title,
			"sheets":        resp.Sheets,
		})
	}

	u.Out().Printf("ID\t%s", resp.SpreadsheetId)
	u.Out().Printf("Title\t%s", resp.Properties.Title)
	u.Out().Printf("Sheets\t%d", len(resp.Sheets))
	return nil
}

// SheetsCopyToCmd copies a sheet to another spreadsheet.
type SheetsCopyToCmd struct {
	SpreadsheetID            string `arg:"" name:"spreadsheetId" help:"Source spreadsheet ID"`
	SheetID                  int64  `arg:"" name:"sheetId" help:"Source sheet ID (tab ID)"`
	DestinationSpreadsheetID string `name:"destination-spreadsheet-id" help:"Target spreadsheet ID" required:""`
}

func (c *SheetsCopyToCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
	destID := strings.TrimSpace(c.DestinationSpreadsheetID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if destID == "" {
		return usage("empty --destination-spreadsheet-id")
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	req := &sheets.CopySheetToAnotherSpreadsheetRequest{
		DestinationSpreadsheetId: destID,
	}

	resp, err := svc.Spreadsheets.Sheets.CopyTo(spreadsheetID, c.SheetID, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"sheetId":   resp.SheetId,
			"title":     resp.Title,
			"index":     resp.Index,
			"sheetType": resp.SheetType,
		})
	}

	u.Out().Printf("Sheet ID\t%d", resp.SheetId)
	u.Out().Printf("Title\t%s", resp.Title)
	u.Out().Printf("Index\t%d", resp.Index)
	return nil
}

// SheetsValuesBatchClearCmd clears values from multiple ranges.
type SheetsValuesBatchClearCmd struct {
	SpreadsheetID string   `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Ranges        []string `name:"ranges" help:"A1 notation ranges to clear" required:""`
}

func (c *SheetsValuesBatchClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	if len(c.Ranges) == 0 {
		return usage("at least one --ranges is required")
	}

	// Clean the ranges
	ranges := make([]string, len(c.Ranges))
	for i, r := range c.Ranges {
		ranges[i] = cleanRange(r)
	}

	if confErr := confirmDestructive(ctx, flags, fmt.Sprintf("clear %d range(s) from spreadsheet %s", len(ranges), spreadsheetID)); confErr != nil {
		return confErr
	}

	req := &sheets.BatchClearValuesRequest{
		Ranges: ranges,
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Values.BatchClear(spreadsheetID, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"spreadsheetId": resp.SpreadsheetId,
			"clearedRanges": resp.ClearedRanges,
		})
	}

	u.Out().Printf("Cleared %d range(s)", len(resp.ClearedRanges))
	return nil
}

// SheetsValuesBatchClearByFilterCmd clears values using DataFilters.
type SheetsValuesBatchClearByFilterCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	FiltersJSON   string `name:"filters-json" help:"JSON array of DataFilter objects" required:""`
}

func (c *SheetsValuesBatchClearByFilterCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	filtersJSON, err := readJSONFromFlag(c.FiltersJSON, "filters")
	if err != nil {
		return fmt.Errorf("read filters: %w", err)
	}

	var dataFilters []*sheets.DataFilter
	if unmarshalErr := json.Unmarshal([]byte(filtersJSON), &dataFilters); unmarshalErr != nil {
		return fmt.Errorf("invalid filters JSON: %w", unmarshalErr)
	}

	if confErr := confirmDestructive(ctx, flags, fmt.Sprintf("clear %d data filter(s) from spreadsheet %s", len(dataFilters), spreadsheetID)); confErr != nil {
		return confErr
	}

	req := &sheets.BatchClearValuesByDataFilterRequest{
		DataFilters: dataFilters,
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Values.BatchClearByDataFilter(spreadsheetID, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"spreadsheetId": resp.SpreadsheetId,
			"clearedRanges": resp.ClearedRanges,
		})
	}

	u.Out().Printf("Cleared %d range(s)", len(resp.ClearedRanges))
	return nil
}

// SheetsValuesBatchGetByFilterCmd retrieves values using DataFilters.
type SheetsValuesBatchGetByFilterCmd struct {
	SpreadsheetID  string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	FiltersJSON    string `name:"filters-json" help:"JSON array of DataFilter objects" required:""`
	MajorDimension string `name:"major-dimension" help:"ROWS or COLUMNS (default: ROWS)"`
	ValueRender    string `name:"value-render" help:"FORMATTED_VALUE, UNFORMATTED_VALUE, or FORMULA"`
	DateTimeRender string `name:"date-time-render" help:"SERIAL_NUMBER or FORMATTED_STRING"`
}

func (c *SheetsValuesBatchGetByFilterCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	filtersJSON, err := readJSONFromFlag(c.FiltersJSON, "filters")
	if err != nil {
		return fmt.Errorf("read filters: %w", err)
	}

	var dataFilters []*sheets.DataFilter
	if unmarshalErr := json.Unmarshal([]byte(filtersJSON), &dataFilters); unmarshalErr != nil {
		return fmt.Errorf("invalid filters JSON: %w", unmarshalErr)
	}

	req := &sheets.BatchGetValuesByDataFilterRequest{
		DataFilters: dataFilters,
	}

	if strings.TrimSpace(c.MajorDimension) != "" {
		req.MajorDimension = c.MajorDimension
	}
	if strings.TrimSpace(c.ValueRender) != "" {
		req.ValueRenderOption = c.ValueRender
	}
	if strings.TrimSpace(c.DateTimeRender) != "" {
		req.DateTimeRenderOption = c.DateTimeRender
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Values.BatchGetByDataFilter(spreadsheetID, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"valueRanges": resp.ValueRanges,
		})
	}

	if len(resp.ValueRanges) == 0 {
		u.Err().Println("No data found")
		return nil
	}

	for i, mvr := range resp.ValueRanges {
		u.Out().Printf("Range %d:", i+1)
		if mvr.ValueRange != nil {
			for _, row := range mvr.ValueRange.Values {
				cells := make([]string, len(row))
				for j, cell := range row {
					cells[j] = fmt.Sprintf("%v", cell)
				}
				u.Out().Println(strings.Join(cells, "\t"))
			}
		}
	}
	return nil
}

// SheetsValuesBatchUpdateByFilterCmd updates values using DataFilters.
type SheetsValuesBatchUpdateByFilterCmd struct {
	SpreadsheetID           string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	DataJSON                string `name:"data-json" help:"JSON array of DataFilterValueRange objects" required:""`
	ValueInput              string `name:"value-input" help:"RAW or USER_ENTERED (default: USER_ENTERED)"`
	IncludeValuesInResponse bool   `name:"include-values-in-response" help:"Return updated values"`
	ResponseValueRender     string `name:"response-value-render" help:"FORMATTED_VALUE, UNFORMATTED_VALUE, or FORMULA"`
	ResponseDateTimeRender  string `name:"response-date-time-render" help:"SERIAL_NUMBER or FORMATTED_STRING"`
}

func (c *SheetsValuesBatchUpdateByFilterCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	dataJSON, err := readJSONFromFlag(c.DataJSON, "data")
	if err != nil {
		return fmt.Errorf("read data: %w", err)
	}

	var data []*sheets.DataFilterValueRange
	if unmarshalErr := json.Unmarshal([]byte(dataJSON), &data); unmarshalErr != nil {
		return fmt.Errorf("invalid data JSON: %w", unmarshalErr)
	}

	req := &sheets.BatchUpdateValuesByDataFilterRequest{
		Data: data,
	}

	valueInput := strings.TrimSpace(c.ValueInput)
	if valueInput == "" {
		valueInput = "USER_ENTERED"
	}
	req.ValueInputOption = valueInput

	if c.IncludeValuesInResponse {
		req.IncludeValuesInResponse = true
	}
	if strings.TrimSpace(c.ResponseValueRender) != "" {
		req.ResponseValueRenderOption = c.ResponseValueRender
	}
	if strings.TrimSpace(c.ResponseDateTimeRender) != "" {
		req.ResponseDateTimeRenderOption = c.ResponseDateTimeRender
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Values.BatchUpdateByDataFilter(spreadsheetID, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"spreadsheetId":       resp.SpreadsheetId,
			"totalUpdatedRows":    resp.TotalUpdatedRows,
			"totalUpdatedColumns": resp.TotalUpdatedColumns,
			"totalUpdatedCells":   resp.TotalUpdatedCells,
			"totalUpdatedSheets":  resp.TotalUpdatedSheets,
			"responses":           resp.Responses,
		})
	}

	u.Out().Printf("Updated %d cells in %d row(s)", resp.TotalUpdatedCells, resp.TotalUpdatedRows)
	return nil
}
