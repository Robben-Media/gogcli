package sheets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type fakeProvider struct {
	identity mcpcontract.Identity
	options  []mcpcontract.CallOptions
	client   *http.Client
}

func (p *fakeProvider) HTTPClient(ctx context.Context, identity mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	p.options = append(p.options, options)
	return p.client, nil
}

func jsonResponse(value string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(value))}
}

func TestReadRangePreservesSparseValuesAndQuotedSheets(t *testing.T) {
	var request *http.Request
	provider := &fakeProvider{
		identity: mcpcontract.Identity{AccountID: "opaque-a", Label: "Work"},
		client: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			request = req

			return jsonResponse(`{
				"range": "'Bob''s Sheet'!A1:C2",
				"majorDimension": "ROWS",
				"values": [["Name", "Amount"], ["Invoice", 42]]
			}`), nil
		})},
	}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{
		"account_id":"opaque-a", "spreadsheet_id":"sheet-1", "range":"'Bob''s Sheet'!A1:C2",
		"major_dimension":"ROWS", "value_render_option":"UNFORMATTED_VALUE"
	}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), provider.identity)
	if err != nil {
		t.Fatal(err)
	}
	result := resultAny.(mcpcontract.Result[ReadRangeData])

	data := result.Data
	if len(data.Rows) != 2 || len(data.Rows[0]) != 3 || len(data.Rows[1]) != 3 ||
		data.Rows[0][0] != "Name" || data.Rows[0][2] != nil ||
		data.Rows[1][1] != float64(42) || data.Rows[1][2] != nil || data.CellCount != 6 {
		t.Fatalf("unexpected values %#v", data.Rows)
	}

	if result.Truncated || data.MaxCells != defaultMaxCells {
		t.Fatalf("unexpected bounds %#v", data)
	}

	if got, expected := request.URL.Path, `/v4/spreadsheets/sheet-1/values/'Bob''s Sheet'!A1:C2`; got != expected {
		t.Fatalf("unexpected path %s", request.URL.Path)
	}

	query := request.URL.Query()
	if query.Get("majorDimension") != "ROWS" || query.Get("valueRenderOption") != "UNFORMATTED_VALUE" {
		t.Fatalf("unexpected query %s", request.URL.RawQuery)
	}

	if len(provider.options) != 1 || provider.options[0].Operation != "sheets_read_range" || provider.options[0].Retry != mcpcontract.SafeRead {
		t.Fatalf("unexpected provider options %#v", provider.options)
	}
}

func TestReadRangeRejectsUnsafeInputsBeforeProvider(t *testing.T) {
	provider := &fakeProvider{}

	readOp := Operations(provider)[1]
	for _, raw := range []string{
		`{"account_id":"a","spreadsheet_id":"s","range":"Sheet1","major_dimension":"ROWS","value_render_option":"FORMULA"}`,
		`{"account_id":"a","spreadsheet_id":"s","range":"Sheet1!A:B","major_dimension":"ROWS","value_render_option":"FORMULA"}`,
		`{"account_id":"a","spreadsheet_id":"s","range":"Sheet1!1:3","major_dimension":"ROWS","value_render_option":"FORMULA"}`,
		`{"account_id":"a","spreadsheet_id":"s","range":"Sheet1!A1:C100000","major_dimension":"ROWS","value_render_option":"FORMULA"}`,
		`{"account_id":"a","spreadsheet_id":"s","range":"Sheet1!C2:A1","major_dimension":"ROWS","value_render_option":"FORMULA"}`,
		`{"account_id":"a","spreadsheet_id":"s","range":"Sheet1!A1:C2","major_dimension":"BAD","value_render_option":"FORMULA"}`,
		`{"account_id":"a","spreadsheet_id":"s","range":"Sheet1!A1:C2","major_dimension":"ROWS","value_render_option":"BAD"}`,
		`{"account_id":"a","spreadsheet_id":"s","range":"Sheet1!not-a-cell","major_dimension":"ROWS","value_render_option":"FORMULA"}`,
		`{"account_id":"a","spreadsheet_id":"s","range":"Sheet1!A1:B2","major_dimension":"ROWS","value_render_option":"FORMULA","max_cells":3}`,
	} {
		if _, err := readOp.Decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}

	if len(provider.options) != 0 {
		t.Fatalf("invalid input acquired a provider: %#v", provider.options)
	}
}

func TestReadRangeNormalizesColumnsIntoRows(t *testing.T) {
	provider := &fakeProvider{
		identity: mcpcontract.Identity{AccountID: "opaque-a"},
		client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(`{
				"range": "Sheet1!A1:B3",
				"majorDimension": "COLUMNS",
				"values": [["Name"], ["Invoice", 42]]
			}`), nil
		})},
	}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{
		"account_id":"opaque-a", "spreadsheet_id":"sheet-1", "range":"Sheet1!A1:B3",
		"major_dimension":"COLUMNS", "value_render_option":"UNFORMATTED_VALUE"
	}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), provider.identity)
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[ReadRangeData])
	data := result.Data
	expected := [][]any{{"Name", "Invoice"}, {nil, float64(42)}, {nil, nil}}

	if len(data.Rows) != 3 || len(data.Rows[0]) != 2 || data.CellCount != 6 || result.Truncated {
		t.Fatalf("unexpected shape %#v", data)
	}

	for row := range expected {
		for column := range expected[row] {
			if data.Rows[row][column] != expected[row][column] {
				t.Fatalf("values = %#v, want %#v", data.Rows, expected)
			}
		}
	}
}

func TestParseBoundedA1AcceptsLowercaseReferences(t *testing.T) {
	t.Parallel()

	if dimensions, err := parseBoundedA1("'Monthly Budget'!a1:b2"); err != nil || dimensions.Rows*dimensions.Columns != 4 {
		t.Fatalf("parseBoundedA1 lower-case: %#v %v", dimensions, err)
	}

	if _, err := parseBoundedA1("Sheet1!a1:b10001"); err == nil {
		t.Fatal("accepted a range over the hard cell limit")
	}
}

func TestReadRangeRejectsRangeLargerThanEffectiveMaxCells(t *testing.T) {
	provider := &fakeProvider{}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{
		"account_id":"opaque-a", "spreadsheet_id":"sheet-1", "range":"Sheet1!A1:B2",
		"major_dimension":"ROWS", "value_render_option":"FORMATTED_VALUE", "max_cells":3
	}`))
	_ = call

	if err == nil {
		t.Fatal("accepted a range larger than the effective cell limit")
	}

	var public *mcpcontract.Error
	if !errors.As(err, &public) || public.Category != mcpcontract.InvalidInput {
		t.Fatalf("unexpected error %#v", err)
	}

	if len(provider.options) != 0 {
		t.Fatalf("oversized range acquired a provider: %#v", provider.options)
	}
}

func TestReadRangeTruncatesUnexpectedUpstreamExcess(t *testing.T) {
	provider := &fakeProvider{
		identity: mcpcontract.Identity{AccountID: "opaque-a"},
		client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(`{"range":"Sheet1!A1:A3","values":[["a","b"],["c","d"],["e","f"]]}`), nil
		})},
	}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{
		"account_id":"opaque-a","spreadsheet_id":"s","range":"Sheet1!A1:A3",
		"major_dimension":"ROWS","value_render_option":"FORMATTED_VALUE","max_cells":3
	}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), provider.identity)
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[ReadRangeData])
	if len(result.Data.Rows) != 3 || len(result.Data.Rows[0]) != 1 || result.Data.CellCount != 3 ||
		result.Data.Rows[0][0] != "a" || !result.Truncated {
		t.Fatalf("unexpected result %#v", result.Data)
	}
}

func TestGetMetadataProjection(t *testing.T) {
	provider := &fakeProvider{
		identity: mcpcontract.Identity{AccountID: "opaque-a", Label: "Work"},
		client: &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Path != "/v4/spreadsheets/sheet-1" {
				t.Fatalf("unexpected path %s", request.URL.Path)
			}

			return jsonResponse(`{
				"spreadsheetId":"sheet-1",
				"spreadsheetUrl":"https://example.invalid/sheet",
				"properties":{"title":"Budget","locale":"en_US","timeZone":"America/Chicago"},
				"sheets":[{"properties":{"sheetId":7,"title":"Q3","index":0,"gridProperties":{"rowCount":20,"columnCount":5}}}]
			}`), nil
		})},
	}

	call, err := Operations(provider)[0].Decode(json.RawMessage(`{"account_id":"opaque-a","spreadsheet_id":"sheet-1"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), provider.identity)
	if err != nil {
		t.Fatal(err)
	}

	data := resultAny.(mcpcontract.Result[GetMetadataData]).Data
	if data.Title != "Budget" || data.TimeZone != "America/Chicago" || len(data.Sheets) != 1 {
		t.Fatalf("unexpected metadata %#v", data)
	}

	sheet := data.Sheets[0]
	if sheet.ID != 7 || sheet.Rows != 20 || sheet.Columns != 5 {
		t.Fatalf("unexpected sheet %#v", sheet)
	}

	if data.URL != "https://example.invalid/sheet" {
		t.Fatalf("unexpected URL %q", data.URL)
	}
}

func TestReadRangeMapsTypedGoogleErrors(t *testing.T) {
	tests := []struct {
		reason   string
		category mcpcontract.ErrorCategory
	}{
		{"insufficientAuthenticationScopes", mcpcontract.InsufficientScope},
		{"insufficientPermissions", mcpcontract.InsufficientScope},
		{"quotaExceeded", mcpcontract.QuotaExhausted},
	}

	for _, test := range tests {
		provider := &fakeProvider{
			identity: mcpcontract.Identity{AccountID: "opaque-a"},
			client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				body := `{"error":{"code":403,"message":"denied","errors":[{"reason":"` + test.reason + `"}]}}`
				return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})},
		}

		call, err := Operations(provider)[1].Decode(json.RawMessage(`{
			"account_id":"opaque-a","spreadsheet_id":"s","range":"Sheet1!A1:B2",
			"major_dimension":"ROWS","value_render_option":"FORMULA"
		}`))
		if err != nil {
			t.Fatal(err)
		}
		_, err = call.Run(context.Background(), provider.identity)

		var public *mcpcontract.Error
		if !errors.As(err, &public) || public.Category != test.category || strings.Contains(public.Error(), "denied") {
			t.Fatalf("%s: unexpected error %#v", test.reason, err)
		}
	}
}

func TestReadRangeMapsQuotaErrorSafely(t *testing.T) {
	provider := &fakeProvider{
		identity: mcpcontract.Identity{AccountID: "opaque-a"},
		client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(`{"error":"user secret"}`))}, nil
		})},
	}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{"account_id":"opaque-a","spreadsheet_id":"s","range":"Sheet1!A1:B2","major_dimension":"ROWS","value_render_option":"FORMULA"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), provider.identity)

	var public *mcpcontract.Error
	if !errors.As(err, &public) || public.Category != mcpcontract.QuotaExhausted || !public.Retryable || strings.Contains(public.Error(), "user secret") {
		t.Fatalf("unexpected error %#v", err)
	}
}

func TestInputSchemas(t *testing.T) {
	t.Parallel()

	ops := Operations(&fakeProvider{})

	tests := []struct {
		index      int
		properties []string
		required   []string
	}{
		{0, []string{"account_id", "spreadsheet_id"}, []string{"account_id", "spreadsheet_id"}},
		{1, []string{"account_id", "spreadsheet_id", "range", "major_dimension", "value_render_option", "max_cells"}, []string{"account_id", "spreadsheet_id", "range", "major_dimension", "value_render_option"}},
	}
	for _, test := range tests {
		raw, err := json.Marshal(ops[test.index].InputSchema)
		if err != nil {
			t.Fatal(err)
		}

		var schema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}

		if len(schema.Properties) != len(test.properties) {
			t.Fatalf("operation %d properties: %#v", test.index, schema.Properties)
		}

		for _, name := range test.properties {
			if _, ok := schema.Properties[name]; !ok {
				t.Fatalf("operation %d missing property %s", test.index, name)
			}
		}

		sort.Strings(schema.Required)
		sort.Strings(test.required)

		if strings.Join(schema.Required, ",") != strings.Join(test.required, ",") {
			t.Fatalf("operation %d required: %#v", test.index, schema.Required)
		}
	}
}
