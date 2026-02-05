package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var errUnexpectedCall = errors.New("unexpected call")

func TestExecute_SheetsBatchGet_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")

		if strings.Contains(path, "/spreadsheets/s1/values:batchGet") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"valueRanges": []map[string]any{
					{
						"range":  "Sheet1!A1:B2",
						"values": [][]any{{"a", "b"}, {"c", "d"}},
					},
					{
						"range":  "Sheet2!A1:A3",
						"values": [][]any{{"x"}, {"y"}, {"z"}},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
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

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &SheetsBatchGetCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B2", "Sheet2!A1:A3"}, ctx, flags); err != nil {
			t.Fatalf("batch-get: %v", err)
		}
	})

	var parsed struct {
		SpreadsheetID string `json:"spreadsheetId"`
		ValueRanges   []struct {
			Range  string  `json:"range"`
			Values [][]any `json:"values"`
		} `json:"valueRanges"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.SpreadsheetID != "s1" {
		t.Fatalf("unexpected spreadsheetId: %s", parsed.SpreadsheetID)
	}
	if len(parsed.ValueRanges) != 2 {
		t.Fatalf("expected 2 value ranges, got %d", len(parsed.ValueRanges))
	}
	if parsed.ValueRanges[0].Range != "Sheet1!A1:B2" {
		t.Fatalf("unexpected range[0]: %s", parsed.ValueRanges[0].Range)
	}
	if parsed.ValueRanges[1].Range != "Sheet2!A1:A3" {
		t.Fatalf("unexpected range[1]: %s", parsed.ValueRanges[1].Range)
	}
}

func TestExecute_SheetsBatchGet_NoRanges(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	// No server needed -- validation should fail before any API call.
	newSheetsService = func(context.Context, string) (*sheets.Service, error) {
		t.Fatal("should not reach API")
		return nil, errUnexpectedCall
	}

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &SheetsBatchGetCmd{}
	err := runKong(t, cmd, []string{"s1"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing ranges")
	}
}

func TestExecute_SheetsBatchUpdate_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")

		if strings.Contains(path, "/spreadsheets/s1/values:batchUpdate") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":       "s1",
				"totalUpdatedRows":    2,
				"totalUpdatedColumns": 2,
				"totalUpdatedCells":   4,
				"totalUpdatedSheets":  1,
			})
			return
		}
		http.NotFound(w, r)
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

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	valuesJSON := `[{"range":"Sheet1!A1:B2","values":[["a","b"],["c","d"]]}]`

	jsonOut := captureStdout(t, func() {
		cmd := &SheetsBatchUpdateCmd{}
		if err := runKong(t, cmd, []string{"s1", "--values-json", valuesJSON}, ctx, flags); err != nil {
			t.Fatalf("batch-update: %v", err)
		}
	})

	var parsed struct {
		SpreadsheetID      string `json:"spreadsheetId"`
		TotalUpdatedCells  int    `json:"totalUpdatedCells"`
		TotalUpdatedSheets int    `json:"totalUpdatedSheets"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.SpreadsheetID != "s1" {
		t.Fatalf("unexpected spreadsheetId: %s", parsed.SpreadsheetID)
	}
	if parsed.TotalUpdatedCells != 4 {
		t.Fatalf("expected 4 updated cells, got %d", parsed.TotalUpdatedCells)
	}
	if parsed.TotalUpdatedSheets != 1 {
		t.Fatalf("expected 1 updated sheet, got %d", parsed.TotalUpdatedSheets)
	}
}
