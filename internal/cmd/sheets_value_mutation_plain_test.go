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

const sheetsValueMutationPlainHeader = "ACTION\tSPREADSHEET_ID\tRANGE\tUPDATED_ROWS\tUPDATED_COLUMNS\tUPDATED_CELLS\tUPDATED_SHEETS\tSTATUS\n"

func TestSheetsValueMutations_PlainReceipts(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPut && strings.Contains(path, "/v4/spreadsheets/ss-update/values/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "ss-update",
				"updatedRange":   "Sheet1!A1:B2",
				"updatedRows":    2,
				"updatedColumns": 2,
				"updatedCells":   4,
			})
			return

		case r.Method == http.MethodPost && strings.Contains(path, "/v4/spreadsheets/ss-append/values/") && strings.Contains(path, ":append"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "ss-append",
				"updates": map[string]any{
					"spreadsheetId":  "ss-append",
					"updatedRange":   "Sheet1!A3:B3",
					"updatedRows":    1,
					"updatedColumns": 2,
					"updatedCells":   2,
				},
			})
			return

		case r.Method == http.MethodPost && strings.Contains(path, "/v4/spreadsheets/ss-clear/values/") && strings.Contains(path, ":clear"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "ss-clear",
				"clearedRange":  "Sheet1!A1:B2",
			})
			return

		case r.Method == http.MethodPost && strings.Contains(path, "/v4/spreadsheets/ss-batch-update/values:batchUpdate"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":       "ss-batch-update",
				"totalUpdatedRows":    3,
				"totalUpdatedColumns": 2,
				"totalUpdatedCells":   5,
				"totalUpdatedSheets":  1,
				"responses": []map[string]any{
					{
						"spreadsheetId":  "ss-batch-update",
						"updatedRange":   "Sheet1!A1:B2",
						"updatedRows":    2,
						"updatedColumns": 2,
						"updatedCells":   4,
					},
					{
						"spreadsheetId":  "ss-batch-update",
						"updatedRange":   "Sheet1!C1",
						"updatedRows":    1,
						"updatedColumns": 1,
						"updatedCells":   1,
					},
				},
			})
			return

		case r.Method == http.MethodPost && strings.Contains(path, "/v4/spreadsheets/ss-batch-clear/values:batchClear"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "ss-batch-clear",
				"clearedRanges": []string{"Sheet1!A1:B2", "Sheet1!D1:E2"},
			})
			return

		case r.Method == http.MethodPost && strings.Contains(path, "/v4/spreadsheets/ss-filter-update/values:batchUpdateByDataFilter"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":       "ss-filter-update",
				"totalUpdatedRows":    2,
				"totalUpdatedColumns": 2,
				"totalUpdatedCells":   4,
				"totalUpdatedSheets":  1,
				"responses": []map[string]any{
					{
						"updatedRange":   "Sheet1!A1:B2",
						"updatedRows":    2,
						"updatedColumns": 2,
						"updatedCells":   4,
					},
				},
			})
			return

		case r.Method == http.MethodPost && strings.Contains(path, "/v4/spreadsheets/ss-filter-clear/values:batchClearByDataFilter"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "ss-filter-clear",
				"clearedRanges": []string{"Sheet1!A1:B2"},
			})
			return

		case r.Method == http.MethodPost && strings.Contains(path, "/v4/spreadsheets/ss-batch-update-totals/values:batchUpdate"):
			// Multi-range API response without per-range details: emit totals once.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":       "ss-batch-update-totals",
				"totalUpdatedRows":    2,
				"totalUpdatedColumns": 2,
				"totalUpdatedCells":   4,
				"totalUpdatedSheets":  1,
			})
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
			name: "update",
			args: []string{
				"--plain", "--account", "a@b.com",
				"sheets", "update", "ss-update", "Sheet1!A1:B2",
				"--values-json", `[["a","b"],["c","d"]]`,
			},
			want: sheetsValueMutationPlainHeader + "update\tss-update\tSheet1!A1:B2\t2\t2\t4\t\tok\n",
		},
		{
			name: "append",
			args: []string{
				"--plain", "--account", "a@b.com",
				"sheets", "append", "ss-append", "Sheet1!A:B",
				"--values-json", `[["e","f"]]`,
			},
			want: sheetsValueMutationPlainHeader + "append\tss-append\tSheet1!A3:B3\t1\t2\t2\t\tok\n",
		},
		{
			name: "clear",
			args: []string{
				"--plain", "--account", "a@b.com", "--force",
				"sheets", "clear", "ss-clear", "Sheet1!A1:B2",
			},
			want: sheetsValueMutationPlainHeader + "clear\tss-clear\tSheet1!A1:B2\t\t\t\t\tok\n",
		},
		{
			name: "batch-update multi-range",
			args: []string{
				"--plain", "--account", "a@b.com",
				"sheets", "batch-update", "ss-batch-update",
				"--values-json", `[{"range":"Sheet1!A1:B2","values":[["a","b"],["c","d"]]},{"range":"Sheet1!C1","values":[["x"]]}]`,
			},
			want: sheetsValueMutationPlainHeader +
				"batch-update\tss-batch-update\tSheet1!A1:B2\t2\t2\t4\t\tok\n" +
				"batch-update\tss-batch-update\tSheet1!C1\t1\t1\t1\t\tok\n",
		},
		{
			name: "batch-update totals only",
			args: []string{
				"--plain", "--account", "a@b.com",
				"sheets", "batch-update", "ss-batch-update-totals",
				"--values-json", `[{"range":"Sheet1!A1:B2","values":[["a","b"],["c","d"]]}]`,
			},
			want: sheetsValueMutationPlainHeader + "batch-update\tss-batch-update-totals\t\t2\t2\t4\t1\tok\n",
		},
		{
			name: "batch-clear",
			args: []string{
				"--plain", "--account", "a@b.com", "--force",
				"sheets", "batch-clear", "ss-batch-clear",
				"--ranges", "Sheet1!A1:B2",
				"--ranges", "Sheet1!D1:E2",
			},
			want: sheetsValueMutationPlainHeader +
				"batch-clear\tss-batch-clear\tSheet1!A1:B2\t\t\t\t\tok\n" +
				"batch-clear\tss-batch-clear\tSheet1!D1:E2\t\t\t\t\tok\n",
		},
		{
			name: "batch-update-by-filter",
			args: []string{
				"--plain", "--account", "a@b.com",
				"sheets", "batch-update-by-filter", "ss-filter-update",
				"--data-json", `[{"dataFilter":{"a1Range":"Sheet1!A1:B2"},"values":[["a","b"],["c","d"]]}]`,
			},
			want: sheetsValueMutationPlainHeader + "batch-update-by-filter\tss-filter-update\tSheet1!A1:B2\t2\t2\t4\t\tok\n",
		},
		{
			name: "batch-clear-by-filter",
			args: []string{
				"--plain", "--account", "a@b.com", "--force",
				"sheets", "batch-clear-by-filter", "ss-filter-clear",
				"--filters-json", `[{"a1Range":"Sheet1!A1:B2"}]`,
			},
			want: sheetsValueMutationPlainHeader + "batch-clear-by-filter\tss-filter-clear\tSheet1!A1:B2\t\t\t\t\tok\n",
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
			for _, prose := range []string{"Updated ", "Appended ", "Cleared "} {
				if strings.Contains(out, prose) {
					t.Fatalf("plain stdout contains prose %q: %q", prose, out)
				}
			}
		})
	}
}
