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

const sheetsSafeTSV = "tab value\tline feed\tcarriage return\tcrlf value\n"

func TestSheetsValueReads_PlainTSVPreservesRows(t *testing.T) {
	values := [][]any{{"tab\tvalue", "line\nfeed", "carriage\rreturn", "crlf\r\nvalue"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var response map[string]any
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/values:batchGet"):
			response = map[string]any{
				"spreadsheetId": "s1",
				"valueRanges": []map[string]any{{
					"range":  "Sheet1!A1:D1",
					"values": values,
				}},
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/values:batchGetByDataFilter"):
			response = map[string]any{
				"valueRanges": []map[string]any{{
					"valueRange": map[string]any{
						"range":  "Sheet1!A1:D1",
						"values": values,
					},
				}},
			}
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

	tests := []struct {
		name    string
		command []string
	}{
		{
			name:    "single range",
			command: []string{"sheets", "get", "s1", "Sheet1!A1:D1"},
		},
		{
			name:    "batch ranges",
			command: []string{"sheets", "batch-get", "s1", "Sheet1!A1:D1"},
		},
		{
			name:    "filter ranges",
			command: []string{"sheets", "batch-get-by-filter", "s1", "--filters-json", `[{"a1Range":"Sheet1!A1:D1"}]`},
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
			if !strings.HasSuffix(out, sheetsSafeTSV) {
				t.Fatalf("plain output = %q, want sanitized row suffix %q", out, sheetsSafeTSV)
			}
		})
	}
}
