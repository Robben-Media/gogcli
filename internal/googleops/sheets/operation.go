// Package sheets provides typed, bounded Google Sheets MCP operations.
package sheets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	defaultMaxCells = 10000
	hardMaxCells    = 10000
	maxA1Column     = 18278
	maxUpstreamBits = 8 * 1024 * 1024
)

var errResponseLimit = errors.New("response exceeds bounded read limit")

func invalid(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: message}
}

type GetMetadataRequest struct {
	mcpcontract.Selection
	SpreadsheetID string `json:"spreadsheet_id" jsonschema:"Google Sheets spreadsheet ID; required"`
}

type SheetMetadata struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Index   int64  `json:"index"`
	Rows    int64  `json:"rows"`
	Columns int64  `json:"columns"`
	Hidden  bool   `json:"hidden,omitempty"`
}

type GetMetadataData struct {
	SpreadsheetID string          `json:"spreadsheet_id"`
	Title         string          `json:"title,omitempty"`
	URL           string          `json:"url,omitempty"`
	Locale        string          `json:"locale,omitempty"`
	TimeZone      string          `json:"time_zone,omitempty"`
	Sheets        []SheetMetadata `json:"sheets"`
}

type ReadRangeRequest struct {
	mcpcontract.Selection
	SpreadsheetID     string `json:"spreadsheet_id" jsonschema:"Google Sheets spreadsheet ID; required"`
	Range             string `json:"range" jsonschema:"Explicit bounded A1 range, such as Sheet1!A1:C20; required"`
	MajorDimension    string `json:"major_dimension" jsonschema:"ROWS or COLUMNS; required"`
	ValueRenderOption string `json:"value_render_option" jsonschema:"FORMATTED_VALUE, UNFORMATTED_VALUE, or FORMULA; required"`
	MaxCells          int    `json:"max_cells,omitempty" jsonschema:"Maximum cells to return; default and hard maximum 10000"`
}

type ReadRangeData struct {
	SpreadsheetID     string  `json:"spreadsheet_id"`
	RequestedRange    string  `json:"requested_range"`
	UpstreamRange     string  `json:"upstream_range,omitempty"`
	MajorDimension    string  `json:"major_dimension"`
	ValueRenderOption string  `json:"value_render_option"`
	Rows              [][]any `json:"rows"`
	CellCount         int     `json:"cell_count"`
	MaxCells          int     `json:"max_cells"`
	URL               string  `json:"url,omitempty"`
	truncated         bool
}

func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return []mcpcontract.Operation{
		mcpcontract.NewOperation("sheets_get_metadata", validateGetMetadata, func(ctx context.Context, id mcpcontract.Identity, in GetMetadataRequest) (mcpcontract.Result[GetMetadataData], error) {
			data, err := getMetadata(ctx, provider, id, in)
			if err != nil {
				return mcpcontract.Result[GetMetadataData]{}, err
			}

			return mcpcontract.NewResult(id, data), nil
		}),
		mcpcontract.NewOperation("sheets_read_range", validateReadRange, func(ctx context.Context, id mcpcontract.Identity, in ReadRangeRequest) (mcpcontract.Result[ReadRangeData], error) {
			data, err := readRange(ctx, provider, id, in)
			if err != nil {
				return mcpcontract.Result[ReadRangeData]{}, err
			}
			out := mcpcontract.NewResult(id, data)
			out.Truncated = data.truncated

			return out, nil
		}),
	}
}

func validateGetMetadata(in GetMetadataRequest) error {
	if strings.TrimSpace(in.SpreadsheetID) == "" {
		return invalid("spreadsheet_id is required")
	}

	return nil
}

func getMetadata(ctx context.Context, provider mcpcontract.ClientProvider, identity mcpcontract.Identity, in GetMetadataRequest) (GetMetadataData, error) {
	spreadsheetID := strings.TrimSpace(in.SpreadsheetID)

	client, err := provider.HTTPClient(ctx, identity, mcpcontract.CallOptions{Operation: "sheets_get_metadata", Retry: mcpcontract.SafeRead})
	if err != nil {
		return GetMetadataData{}, setupError("sheets_get_metadata", err)
	}

	svc, err := sheets.NewService(ctx, option.WithHTTPClient(boundedClient(client, maxUpstreamBits)))
	if err != nil {
		return GetMetadataData{}, setupError("sheets_get_metadata", err)
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields(googleapi.Field("spreadsheetId,spreadsheetUrl,properties(title,locale,timeZone),sheets(properties(sheetId,title,index,hidden,gridProperties(rowCount,columnCount)))")).
		Context(ctx).
		Do()
	if err != nil {
		return GetMetadataData{}, mapGoogleError("sheets_get_metadata", err)
	}

	if resp == nil {
		return GetMetadataData{}, &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "sheets_get_metadata returned no spreadsheet"}
	}

	data := GetMetadataData{
		SpreadsheetID: resp.SpreadsheetId,
		URL:           spreadsheetURL(resp.SpreadsheetId),
		Sheets:        make([]SheetMetadata, 0, len(resp.Sheets)),
	}
	if resp.Properties != nil {
		data.Title = resp.Properties.Title
		data.Locale = resp.Properties.Locale
		data.TimeZone = resp.Properties.TimeZone
	}

	if resp.SpreadsheetUrl != "" {
		data.URL = resp.SpreadsheetUrl
	}

	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}

		item := SheetMetadata{
			ID:     sheet.Properties.SheetId,
			Title:  sheet.Properties.Title,
			Index:  sheet.Properties.Index,
			Hidden: sheet.Properties.Hidden,
		}
		if sheet.Properties.GridProperties != nil {
			item.Rows = sheet.Properties.GridProperties.RowCount
			item.Columns = sheet.Properties.GridProperties.ColumnCount
		}

		data.Sheets = append(data.Sheets, item)
	}

	return data, nil
}

func validateReadRange(in ReadRangeRequest) error {
	if strings.TrimSpace(in.SpreadsheetID) == "" {
		return invalid("spreadsheet_id is required")
	}

	switch in.MajorDimension {
	case "ROWS", "COLUMNS":
	default:
		return invalid("major_dimension must be ROWS or COLUMNS")
	}

	switch in.ValueRenderOption {
	case "FORMATTED_VALUE", "UNFORMATTED_VALUE", "FORMULA":
	default:
		return invalid("value_render_option must be FORMATTED_VALUE, UNFORMATTED_VALUE, or FORMULA")
	}

	maxCells := in.MaxCells
	if maxCells == 0 {
		maxCells = defaultMaxCells
	}

	if maxCells < 0 || maxCells > hardMaxCells {
		return invalid(fmt.Sprintf("max_cells must be between 1 and %d", hardMaxCells))
	}

	rangeSize, err := boundedA1Cells(in.Range)
	if err != nil {
		return err
	}

	if rangeSize > hardMaxCells {
		return invalid(fmt.Sprintf("range contains %d cells, which exceeds the %d-cell hard limit", rangeSize, hardMaxCells))
	}

	return nil
}

func readRange(ctx context.Context, provider mcpcontract.ClientProvider, identity mcpcontract.Identity, in ReadRangeRequest) (ReadRangeData, error) {
	maxCells := in.MaxCells
	if maxCells == 0 {
		maxCells = defaultMaxCells
	}
	spreadsheetID := strings.TrimSpace(in.SpreadsheetID)

	client, err := provider.HTTPClient(ctx, identity, mcpcontract.CallOptions{Operation: "sheets_read_range", Retry: mcpcontract.SafeRead})
	if err != nil {
		return ReadRangeData{}, setupError("sheets_read_range", err)
	}

	svc, err := sheets.NewService(ctx, option.WithHTTPClient(boundedClient(client, maxUpstreamBits)))
	if err != nil {
		return ReadRangeData{}, setupError("sheets_read_range", err)
	}

	resp, err := svc.Spreadsheets.Values.Get(spreadsheetID, cleanRange(in.Range)).
		MajorDimension(in.MajorDimension).
		ValueRenderOption(in.ValueRenderOption).
		Context(ctx).
		Do()
	if err != nil {
		return ReadRangeData{}, mapGoogleError("sheets_read_range", err)
	}

	data := ReadRangeData{
		SpreadsheetID:     spreadsheetID,
		RequestedRange:    strings.TrimSpace(in.Range),
		UpstreamRange:     resp.Range,
		MajorDimension:    in.MajorDimension,
		ValueRenderOption: in.ValueRenderOption,
		Rows:              make([][]any, 0, len(resp.Values)),
		MaxCells:          maxCells,
		URL:               spreadsheetURL(spreadsheetID),
	}

	remaining := maxCells
	for _, row := range resp.Values {
		if remaining == 0 {
			data.truncated = true
			break
		}

		if len(row) > remaining {
			row = row[:remaining]
			data.truncated = true
		}
		copied := append([]any(nil), row...)
		data.Rows = append(data.Rows, copied)
		remaining -= len(copied)

		data.CellCount += len(copied)
		if data.truncated {
			break
		}
	}

	return data, nil
}

func boundedA1Cells(raw string) (int, error) {
	value := cleanRange(strings.TrimSpace(raw))
	if value == "" {
		return 0, invalid("range is required")
	}

	sheet, rangePart, err := splitSheet(value)
	if err != nil {
		return 0, err
	}
	_ = sheet

	parts := strings.Split(rangePart, ":")
	if len(parts) != 2 {
		if len(parts) == 1 {
			return 1, nil
		}

		return 0, invalid("range must contain at most one colon")
	}

	startCol, startRow, err := parseA1Cell(parts[0])
	if err != nil {
		return 0, err
	}

	endCol, endRow, err := parseA1Cell(parts[1])
	if err != nil {
		return 0, err
	}

	if endRow < startRow || endCol < startCol {
		return 0, invalid("range endpoints must be in ascending row and column order")
	}
	rows := int64(endRow - startRow + 1)

	columns := int64(endCol - startCol + 1)
	if rows > hardMaxCells || columns > hardMaxCells || rows*columns > hardMaxCells {
		return 0, invalid("range exceeds the 10000-cell hard limit")
	}

	return int(rows * columns), nil
}

func splitSheet(value string) (string, string, error) {
	index := strings.LastIndex(value, "!")
	if index <= 0 || index == len(value)-1 {
		return "", "", invalid("range must include a sheet name and an explicit bounded A1 range")
	}

	sheet, err := unquoteSheet(value[:index])
	if err != nil {
		return "", "", err
	}

	return sheet, strings.TrimSpace(value[index+1:]), nil
}

func unquoteSheet(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalid("range sheet name is required")
	}

	if !strings.HasPrefix(value, "'") {
		return value, nil
	}

	if len(value) < 2 || !strings.HasSuffix(value, "'") {
		return "", invalid("range sheet name has an unterminated quote")
	}
	var out strings.Builder

	for i := 1; i < len(value)-1; i++ {
		if value[i] != '\'' {
			out.WriteByte(value[i])
			continue
		}

		if i+1 >= len(value)-1 || value[i+1] != '\'' {
			return "", invalid("range sheet name has an unterminated quote")
		}

		out.WriteByte('\'')

		i++
	}

	result := out.String()
	if result == "" {
		return "", invalid("range sheet name is required")
	}

	return result, nil
}

func parseA1Cell(value string) (int, int, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "$", ""))

	letters := 0
	for letters < len(value) {
		ch := value[letters]
		if (ch < 'A' || ch > 'Z') && (ch < 'a' || ch > 'z') {
			break
		}

		letters++
	}

	if letters == 0 || letters == len(value) {
		return 0, 0, invalid("range contains an invalid A1 cell")
	}

	row, err := strconv.Atoi(value[letters:])
	if err != nil || row <= 0 {
		return 0, 0, invalid("range contains an invalid A1 row")
	}

	column := 0

	for i := 0; i < letters; i++ {
		ch := value[i]
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}

		next := column*26 + int(ch-'A'+1)
		if next < column || next > maxA1Column {
			return 0, 0, invalid("range contains an invalid A1 column")
		}
		column = next
	}

	return column, row, nil
}

func cleanRange(value string) string {
	return strings.ReplaceAll(value, `\!`, "!")
}

func spreadsheetURL(id string) string {
	if id == "" {
		return ""
	}

	return "https://docs.google.com/spreadsheets/d/" + url.PathEscape(id) + "/edit"
}

type limitedTransport struct {
	base http.RoundTripper
	max  int64
}

func (t limitedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.base == nil {
		t.base = http.DefaultTransport
	}

	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}
	response.Body = &limitedBody{ReadCloser: response.Body, remaining: t.max}

	return response, nil
}

type limitedBody struct {
	io.ReadCloser
	remaining int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		probe := make([]byte, 1)

		n, err := b.ReadCloser.Read(probe)
		if n > 0 {
			return 0, errResponseLimit
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0, io.EOF
			}

			return 0, fmt.Errorf("%w", err)
		}

		return 0, nil
	}

	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)

	if err != nil {
		if errors.Is(err, io.EOF) {
			return n, io.EOF
		}

		return n, fmt.Errorf("%w", err)
	}

	return n, nil
}

func boundedClient(client *http.Client, maxBytes int64) *http.Client {
	out := *client
	out.Transport = limitedTransport{base: client.Transport, max: maxBytes}

	return &out
}

func mapGoogleError(operation string, err error) error {
	if err == nil {
		return nil
	}

	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		out := &mcpcontract.Error{Message: fmt.Sprintf("%s failed with Google status %d", operation, apiErr.Code), Retryable: apiErr.Code == http.StatusTooManyRequests || apiErr.Code >= 500}
		switch apiErr.Code {
		case http.StatusBadRequest:
			out.Category = mcpcontract.InvalidInput
		case http.StatusUnauthorized:
			out.Category = mcpcontract.AuthRequired
		case http.StatusForbidden:
			out.Category = mcpcontract.Forbidden
		case http.StatusNotFound:
			out.Category = mcpcontract.NotFound
		case http.StatusTooManyRequests:
			out.Category = mcpcontract.QuotaExhausted
		default:
			out.Category = mcpcontract.UpstreamFailure
		}

		return out
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: operation + " exceeded its deadline"}
	}

	message := operation + " failed"
	if errors.Is(err, errResponseLimit) {
		message = operation + " response exceeded the bounded read limit"
	}

	return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: message}
}

func setupError(operation string, err error) error {
	var public *mcpcontract.Error
	if errors.As(err, &public) {
		return public
	}

	return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: operation + " client setup failed"}
}
