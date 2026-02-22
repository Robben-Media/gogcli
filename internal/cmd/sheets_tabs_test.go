package cmd

import (
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

func TestExecute_SheetsSheetAdd_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")

		if strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost {
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].AddSheet == nil {
				t.Fatalf("expected addSheet request, got %#v", req.Requests)
			}
			title := req.Requests[0].AddSheet.Properties.Title
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"replies": []map[string]any{
					{
						"addSheet": map[string]any{
							"properties": map[string]any{
								"sheetId": 123,
								"title":   title,
							},
						},
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
		cmd := &SheetsSheetAddCmd{}
		if err := runKong(t, cmd, []string{"s1", "--title", "NewTab"}, ctx, flags); err != nil {
			t.Fatalf("sheet add: %v", err)
		}
	})

	var parsed struct {
		SpreadsheetID string `json:"spreadsheetId"`
		SheetID       int64  `json:"sheetId"`
		Title         string `json:"title"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.SpreadsheetID != "s1" {
		t.Fatalf("unexpected spreadsheetId: %s", parsed.SpreadsheetID)
	}
	if parsed.SheetID != 123 {
		t.Fatalf("expected sheetId 123, got %d", parsed.SheetID)
	}
	if parsed.Title != "NewTab" {
		t.Fatalf("expected title NewTab, got %s", parsed.Title)
	}
}

func TestExecute_SheetsSheetDelete_JSON(t *testing.T) {
	origNew := newSheetsService
	t.Cleanup(func() { newSheetsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")

		if strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost {
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].DeleteSheet == nil {
				t.Fatalf("expected deleteSheet request, got %#v", req.Requests)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"replies":       []map[string]any{{}},
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
		cmd := &SheetsSheetDeleteCmd{}
		if err := runKong(t, cmd, []string{"s1", "--sheet-id", "42"}, ctx, flags); err != nil {
			t.Fatalf("sheet delete: %v", err)
		}
	})

	var parsed struct {
		SpreadsheetID  string `json:"spreadsheetId"`
		DeletedSheetID int64  `json:"deletedSheetId"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.SpreadsheetID != "s1" {
		t.Fatalf("unexpected spreadsheetId: %s", parsed.SpreadsheetID)
	}
	if parsed.DeletedSheetID != 42 {
		t.Fatalf("expected deletedSheetId 42, got %d", parsed.DeletedSheetID)
	}
}
