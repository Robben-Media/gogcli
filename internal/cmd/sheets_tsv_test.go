package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	sheetsSafeTSV      = "tab value\tline feed\tcarriage return\tcrlf value\n"
	sheetsBatchHeader  = "RANGE\tROW_INDEX\tCOLUMN_INDEX\tVALUE\n"
	sheetsBatchSafeTSV = sheetsBatchHeader +
		"Sheet1!A1:D1\t0\t0\ttab value\n" +
		"Sheet1!A1:D1\t0\t1\tline feed\n" +
		"Sheet1!A1:D1\t0\t2\tcarriage return\n" +
		"Sheet1!A1:D1\t0\t3\tcrlf value\n" +
		"Sheet2!B2:D4\t0\t0\tsecond\n" +
		"Sheet2!B2:D4\t0\t1\t\n" +
		"Sheet2!B2:D4\t0\t2\tthird\n" +
		"Sheet2!B2:D4\t2\t0\tlast\n"
	sheetsColumnMajorSafeTSV = sheetsBatchHeader +
		"Sheet3!A1:B2\t0\t0\ta\n" +
		"Sheet3!A1:B2\t1\t0\tb\n" +
		"Sheet3!A1:B2\t0\t1\tc\n" +
		"Sheet3!A1:B2\t1\t1\td\n"
)

func installSheetsTestService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	original := newSheetsService
	t.Cleanup(func() { newSheetsService = original })
	service, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) {
		return service, nil
	}
}

func TestSheetsValueReads_PlainTSVPreservesRows(t *testing.T) {
	values := [][]any{{"tab\tvalue", "line\nfeed", "carriage\rreturn", "crlf\r\nvalue"}}
	valueRanges := []map[string]any{
		{
			"range":  "Sheet1!A1:D1",
			"values": values,
		},
		{
			"range": "Sheet2!B2:D4",
			"values": [][]any{
				{"second", nil, "third"},
				{},
				{"last"},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var response map[string]any
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/values:batchGet"):
			response = map[string]any{
				"spreadsheetId": "s1",
				"valueRanges":   valueRanges,
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/values:batchGetByDataFilter"):
			var request struct {
				MajorDimension string `json:"majorDimension"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
				return
			}
			if request.MajorDimension == "COLUMNS" {
				response = map[string]any{"valueRanges": []map[string]any{{
					"valueRange": map[string]any{
						"range":          "Sheet3!A1:B2",
						"majorDimension": "COLUMNS",
						"values":         [][]any{{"a", "b"}, {"c", "d"}},
					},
				}}}
				break
			}
			matchedValueRanges := make([]map[string]any, len(valueRanges))
			for i, valueRange := range valueRanges {
				matchedValueRanges[i] = map[string]any{"valueRange": valueRange}
			}
			response = map[string]any{"valueRanges": matchedValueRanges}
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/values/"):
			response = map[string]any{
				"range":  "Sheet1!A1:D1",
				"values": values,
			}
		default:
			http.NotFound(w, r)
			return
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	installSheetsTestService(t, srv)

	tests := []struct {
		name    string
		command []string
		want    string
	}{
		{
			name:    "single range",
			command: []string{"sheets", "get", "s1", "Sheet1!A1:D1"},
			want:    sheetsSafeTSV,
		},
		{
			name:    "batch ranges",
			command: []string{"sheets", "batch-get", "s1", "Sheet1!A1:D1", "Sheet2!B2:D4"},
			want:    sheetsBatchSafeTSV,
		},
		{
			name:    "filter ranges",
			command: []string{"sheets", "batch-get-by-filter", "s1", "--filters-json", `[{"a1Range":"Sheet1!A1:D1"},{"a1Range":"Sheet2!B2:D4"}]`},
			want:    sheetsBatchSafeTSV,
		},
		{
			name:    "column-major filter range",
			command: []string{"sheets", "batch-get-by-filter", "s1", "--filters-json", `[{"a1Range":"Sheet3!A1:B2"}]`, "--major-dimension", "COLUMNS"},
			want:    sheetsColumnMajorSafeTSV,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				_ = captureStderr(t, func() {
					args := append([]string{"--plain", "--account", "a@b.com"}, tt.command...)
					if err := Execute(args); err != nil {
						t.Fatalf("Execute: %v", err)
					}
				})
			})
			if out != tt.want {
				t.Fatalf("plain output = %q, want %q", out, tt.want)
			}
		})
	}
}

func TestSheetsBatchValueReads_PlainEmptyResultsPrintHeaderOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"valueRanges": []any{}}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	installSheetsTestService(t, srv)

	commands := [][]string{
		{"sheets", "batch-get", "s1", "Sheet1!A1"},
		{"sheets", "batch-get-by-filter", "s1", "--filters-json", `[{"a1Range":"Sheet1!A1"}]`},
	}
	for _, command := range commands {
		name := command[1]
		t.Run(name, func(t *testing.T) {
			var stderr string
			stdout := captureStdout(t, func() {
				stderr = captureStderr(t, func() {
					args := append([]string{"--plain", "--account", "a@b.com"}, command...)
					if err := Execute(args); err != nil {
						t.Fatalf("Execute: %v", err)
					}
				})
			})
			if stdout != sheetsBatchHeader {
				t.Fatalf("plain output = %q, want %q", stdout, sheetsBatchHeader)
			}
			if stderr != "" {
				t.Fatalf("plain stderr = %q, want empty", stderr)
			}
		})
	}
}
