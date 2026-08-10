package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"
)

func TestExecute_SearchConsoleQuery_StartRow_HelpDocumentsFlag(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"search-console", "query", "--help"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "--start-row") {
		t.Fatalf("expected help to document --start-row, got: %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "zero-based") {
		t.Fatalf("expected help to mention zero-based start row meaning, got: %q", out)
	}
}

func TestExecute_SearchConsoleQuery_StartRow_OmittedAndExplicit(t *testing.T) {
	cases := []struct {
		name         string
		extraArgs    []string
		wantStartRow int64
	}{
		{
			name:         "omitted defaults to first window",
			extraArgs:    nil,
			wantStartRow: 0,
		},
		{
			name:         "explicit zero",
			extraArgs:    []string{"--start-row", "0"},
			wantStartRow: 0,
		},
		{
			name:         "positive offset",
			extraArgs:    []string{"--start-row", "25"},
			wantStartRow: 25,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			origNew := newSearchConsoleService
			t.Cleanup(func() { newSearchConsoleService = origNew })

			var got searchconsole.SearchAnalyticsQueryRequest
			var sawBody bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "searchAnalytics/query") && r.Method == http.MethodPost {
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
						t.Errorf("decode body: %v", err)
					}
					sawBody = true
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"rows": []map[string]any{
							{
								"keys":        []string{"keyword"},
								"clicks":      1.0,
								"impressions": 10.0,
								"ctr":         0.1,
								"position":    2.0,
							},
						},
					})
					return
				}
				http.NotFound(w, r)
			}))
			t.Cleanup(srv.Close)

			svc, err := searchconsole.NewService(context.Background(),
				option.WithoutAuthentication(),
				option.WithHTTPClient(srv.Client()),
				option.WithEndpoint(srv.URL+"/"),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			newSearchConsoleService = func(context.Context, string) (*searchconsole.Service, error) { return svc, nil }

			args := []string{
				"--json", "--account", "a@b.com",
				"search-console", "query",
				"--site-url", "https://example.com/",
				"--start-date", "2026-01-01",
				"--end-date", "2026-01-31",
				"--dimensions", "query",
			}
			args = append(args, tc.extraArgs...)

			out := captureStdout(t, func() {
				_ = captureStderr(t, func() {
					if err := Execute(args); err != nil {
						t.Fatalf("Execute: %v", err)
					}
				})
			})

			if !sawBody {
				t.Fatal("expected Search Analytics query request body")
			}
			if got.StartRow != tc.wantStartRow {
				t.Fatalf("startRow=%d, want %d", got.StartRow, tc.wantStartRow)
			}

			var parsed struct {
				Rows []struct {
					Keys        []string `json:"keys"`
					Clicks      float64  `json:"clicks"`
					Impressions float64  `json:"impressions"`
					Ctr         float64  `json:"ctr"`
					Position    float64  `json:"position"`
				} `json:"rows"`
			}
			if err := json.Unmarshal([]byte(out), &parsed); err != nil {
				t.Fatalf("json parse: %v\nout=%q", err, out)
			}
			if len(parsed.Rows) != 1 || parsed.Rows[0].Keys[0] != "keyword" {
				t.Fatalf("unexpected JSON shape: %q", out)
			}
		})
	}
}

func TestExecute_SearchConsoleQuery_StartRow_NegativeFailsBeforeService(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	serviceCalls := 0
	newSearchConsoleService = func(context.Context, string) (*searchconsole.Service, error) {
		serviceCalls++
		t.Fatal("service must not be constructed for negative --start-row")
		return nil, nil
	}

	err := Execute([]string{
		"--json", "--account", "a@b.com",
		"search-console", "query",
		"--site-url", "https://example.com/",
		"--start-date", "2026-01-01",
		"--end-date", "2026-01-31",
		// Equals form avoids Kong treating a bare "-1" as another short flag.
		"--start-row=-1",
	})
	if err == nil {
		t.Fatal("expected error for negative --start-row")
	}
	if serviceCalls != 0 {
		t.Fatalf("expected validation before service construction, got %d call(s)", serviceCalls)
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "start-row") {
		t.Fatalf("expected error to mention start-row, got: %v", err)
	}
	if !strings.Contains(msg, ">=") && !strings.Contains(msg, "negative") && !strings.Contains(msg, "must be") {
		t.Fatalf("expected non-negative validation message, got: %v", err)
	}
	code := ExitCode(err)
	if code != 2 {
		t.Fatalf("expected usage exit code 2, got %v", code)
	}
}

func TestExecute_SearchConsoleQuery_StartRow_PlainShapeUnchanged(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	var got searchconsole.SearchAnalyticsQueryRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "searchAnalytics/query") && r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("decode body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rows": []map[string]any{
					{
						"keys":        []string{"keyword a", "keyword b"},
						"clicks":      42.0,
						"impressions": 1000.0,
						"ctr":         0.042,
						"position":    3.5,
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	svc, err := searchconsole.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newSearchConsoleService = func(context.Context, string) (*searchconsole.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"search-console", "query",
				"--site-url", "https://example.com/",
				"--start-date", "2026-01-01",
				"--end-date", "2026-01-31",
				"--start-row", "50",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if got.StartRow != 50 {
		t.Fatalf("startRow=%d, want 50", got.StartRow)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + one data row, got %d lines: %q", len(lines), out)
	}
	if lines[0] != "KEYS\tCLICKS\tIMPRESSIONS\tCTR\tPOSITION" {
		t.Fatalf("unexpected header: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "keyword a, keyword b\t") {
		t.Fatalf("unexpected data row: %q", lines[1])
	}
}
