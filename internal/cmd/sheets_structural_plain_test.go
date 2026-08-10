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

const sheetsStructuralPlainHeader = "ACTION\tSPREADSHEET_ID\tSHEET_ID\tTITLE\tRANGE\tSTATUS\n"

func TestSheetsStructuralMutations_PlainReceipts(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/v4/spreadsheets"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "ss-create",
				"spreadsheetUrl": "http://example.com/ss-create",
				"properties":     map[string]any{"title": "Budget 2026"},
			})
			return

		case r.Method == http.MethodGet && strings.Contains(path, "/v4/spreadsheets/ss-format"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "ss-format",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 42, "title": "Sheet1"}},
				},
			})
			return

		case r.Method == http.MethodPost && strings.Contains(path, "/v4/spreadsheets/ss-format:batchUpdate"):
			_ = json.NewEncoder(w).Encode(map[string]any{"spreadsheetId": "ss-format", "replies": []any{map[string]any{}}})
			return

		case r.Method == http.MethodPost && strings.Contains(path, "/sheets/7:copyTo"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sheetId": 99,
				"title":   "Copied Tab",
				"index":   1,
			})
			return

		case r.Method == http.MethodPost && strings.Contains(path, "/v4/spreadsheets/ss-tabs:batchUpdate"):
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode batchUpdate: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if len(req.Requests) != 1 {
				t.Errorf("expected 1 request, got %d", len(req.Requests))
			}
			switch {
			case req.Requests[0].AddSheet != nil:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"spreadsheetId": "ss-tabs",
					"replies": []map[string]any{{
						"addSheet": map[string]any{
							"properties": map[string]any{
								"sheetId": 123,
								"title":   req.Requests[0].AddSheet.Properties.Title,
							},
						},
					}},
				})
			case req.Requests[0].DeleteSheet != nil:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"spreadsheetId": "ss-tabs",
					"replies":       []map[string]any{{}},
				})
			case req.Requests[0].UpdateSheetProperties != nil:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"spreadsheetId": "ss-tabs",
					"replies":       []map[string]any{{}},
				})
			default:
				http.NotFound(w, r)
			}
			return

		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := sheets.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSheetsService = func(context.Context, string) (*sheets.Service, error) { return svc, nil }

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "create",
			args: []string{"--plain", "--account", "a@b.com", "sheets", "create", "Budget 2026"},
			want: sheetsStructuralPlainHeader + "create\tss-create\t\tBudget 2026\t\tok\n",
		},
		{
			name: "format",
			args: []string{
				"--plain", "--account", "a@b.com",
				"sheets", "format", "ss-format", "Sheet1!B2:C3",
				"--format-json", `{"textFormat":{"bold":true}}`,
				"--format-fields", "textFormat.bold",
			},
			want: sheetsStructuralPlainHeader + "format\tss-format\t42\t\tSheet1!B2:C3\tok\n",
		},
		{
			name: "copy-to",
			args: []string{
				"--plain", "--account", "a@b.com",
				"sheets", "copy-to", "ss-source", "7",
				"--destination-spreadsheet-id", "ss-dest",
			},
			want: sheetsStructuralPlainHeader + "copy-to\tss-dest\t99\tCopied Tab\t\tok\n",
		},
		{
			name: "sheet add",
			args: []string{"--plain", "--account", "a@b.com", "sheets", "sheet", "add", "ss-tabs", "--title", "NewTab"},
			want: sheetsStructuralPlainHeader + "add\tss-tabs\t123\tNewTab\t\tok\n",
		},
		{
			name: "sheet update",
			args: []string{
				"--plain", "--account", "a@b.com",
				"sheets", "sheet", "update", "ss-tabs",
				"--sheet-id", "55",
				"--title", "Renamed",
			},
			want: sheetsStructuralPlainHeader + "update\tss-tabs\t55\tRenamed\t\tok\n",
		},
		{
			name: "sheet delete",
			args: []string{
				"--plain", "--account", "a@b.com",
				"sheets", "sheet", "delete", "ss-tabs",
				"--sheet-id", "55",
			},
			want: sheetsStructuralPlainHeader + "delete\tss-tabs\t55\t\t\tok\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				_ = captureStderr(t, func() {
					if err := Execute(tt.args); err != nil {
						t.Fatalf("Execute: %v", err)
					}
				})
			})
			if out != tt.want {
				t.Fatalf("plain output mismatch:\nwant %q\ngot  %q", tt.want, out)
			}
			if strings.Contains(out, "Created ") ||
				strings.Contains(out, "Formatted ") ||
				strings.Contains(out, "Added sheet") ||
				strings.Contains(out, "Updated sheet") ||
				strings.Contains(out, "Deleted sheet") {
				t.Fatalf("plain stdout contains prose: %q", out)
			}
		})
	}
}
