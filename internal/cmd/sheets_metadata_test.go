package cmd

import (
	"bytes"
	"context"
	"encoding/json"
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

const (
	sheetsMetadataPlainHeader = "SPREADSHEET_ID\tSPREADSHEET_TITLE\tLOCALE\tTIMEZONE\tURL\tSHEET_ID\tSHEET_TITLE\tROWS\tCOLUMNS"
)

func TestSheetsMetadataCmd_TextAndJSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v4/spreadsheets/id1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "id1",
				"spreadsheetUrl": "https://docs.google.com/spreadsheets/d/id1",
				"properties": map[string]any{
					"title":    "Budget",
					"locale":   "en_US",
					"timeZone": "UTC",
				},
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 1, "title": "Sheet1", "gridProperties": map[string]any{"rowCount": 10, "columnCount": 5}}},
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

	var outBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	cmd := &SheetsMetadataCmd{}
	if err := runKong(t, cmd, []string{"id1"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
	text := outBuf.String()
	if !strings.Contains(text, "ID\tid1") || !strings.Contains(text, "Sheets:") {
		t.Fatalf("unexpected text: %q", text)
	}

	jsonOut := captureStdout(t, func() {
		u2, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx2 := ui.WithUI(context.Background(), u2)
		ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

		cmd2 := &SheetsMetadataCmd{}
		if err := runKong(t, cmd2, []string{"id1"}, ctx2, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		SpreadsheetID string `json:"spreadsheetId"`
		Title         string `json:"title"`
		Sheets        []any  `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.SpreadsheetID != "id1" || parsed.Title != "Budget" || len(parsed.Sheets) != 1 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestExecute_SheetsMetadata_PlainTSV(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	const (
		spreadsheetTitle = "Budget\tQ1\n2026"
		sheetTitle1      = "Income\tMain"
		sheetTitle2      = "Expenses\r\nOps"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v4/spreadsheets/id1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "id1",
				"spreadsheetUrl": "https://docs.google.com/spreadsheets/d/id1",
				"properties": map[string]any{
					"title":    spreadsheetTitle,
					"locale":   "en_US",
					"timeZone": "America/Chicago",
				},
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 1, "title": sheetTitle1, "gridProperties": map[string]any{"rowCount": 10, "columnCount": 5}}},
					{"properties": map[string]any{"sheetId": 2, "title": sheetTitle2, "gridProperties": map[string]any{"rowCount": 20, "columnCount": 8}}},
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

	plain := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "--account", "a@b.com", "sheets", "metadata", "id1"}); err != nil {
				t.Fatalf("Execute plain: %v", err)
			}
		})
	})
	wantPlain := strings.Join([]string{
		sheetsMetadataPlainHeader,
		"id1\tBudget Q1 2026\ten_US\tAmerica/Chicago\thttps://docs.google.com/spreadsheets/d/id1\t1\tIncome Main\t10\t5",
		"id1\tBudget Q1 2026\ten_US\tAmerica/Chicago\thttps://docs.google.com/spreadsheets/d/id1\t2\tExpenses Ops\t20\t8",
		"",
	}, "\n")
	if plain != wantPlain {
		t.Fatalf("plain output =\n%q\nwant\n%q", plain, wantPlain)
	}
	if strings.Contains(plain, "Sheets:") || strings.Contains(plain, "ID\tid1") {
		t.Fatalf("plain output mixed human headings: %q", plain)
	}

	jsonOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "sheets", "metadata", "id1"}); err != nil {
				t.Fatalf("Execute json: %v", err)
			}
		})
	})
	var parsed struct {
		SpreadsheetID string `json:"spreadsheetId"`
		Title         string `json:"title"`
		Locale        string `json:"locale"`
		TimeZone      string `json:"timeZone"`
		Sheets        []struct {
			Properties struct {
				Title string `json:"title"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.SpreadsheetID != "id1" || parsed.Title != spreadsheetTitle || parsed.Locale != "en_US" || parsed.TimeZone != "America/Chicago" {
		t.Fatalf("unexpected json identity: %#v", parsed)
	}
	if len(parsed.Sheets) != 2 || parsed.Sheets[0].Properties.Title != sheetTitle1 || parsed.Sheets[1].Properties.Title != sheetTitle2 {
		t.Fatalf("JSON must preserve unsanitized sheet titles: %#v", parsed.Sheets)
	}

	human := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "sheets", "metadata", "id1"}); err != nil {
				t.Fatalf("Execute human: %v", err)
			}
		})
	})
	if !strings.Contains(human, "ID\tid1") || !strings.Contains(human, "Sheets:") {
		t.Fatalf("human output lost metadata layout: %q", human)
	}
	if strings.Contains(human, sheetsMetadataPlainHeader) {
		t.Fatalf("human output used plain header: %q", human)
	}
}

func TestExecute_SheetsMetadata_PlainTSV_NoSheets(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v4/spreadsheets/empty1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "empty1",
				"spreadsheetUrl": "https://docs.google.com/spreadsheets/d/empty1",
				"properties": map[string]any{
					"title":    "Empty",
					"locale":   "en_GB",
					"timeZone": "UTC",
				},
				"sheets": []map[string]any{},
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

	plain := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "--account", "a@b.com", "sheets", "metadata", "empty1"}); err != nil {
				t.Fatalf("Execute plain: %v", err)
			}
		})
	})
	wantPlain := strings.Join([]string{
		sheetsMetadataPlainHeader,
		"empty1\tEmpty\ten_GB\tUTC\thttps://docs.google.com/spreadsheets/d/empty1\t\t\t\t",
		"",
	}, "\n")
	if plain != wantPlain {
		t.Fatalf("plain empty-sheets output =\n%q\nwant\n%q", plain, wantPlain)
	}
}
