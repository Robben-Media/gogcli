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

func TestExecute_SearchConsoleSites_JSON(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sites") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"siteEntry": []map[string]any{
					{
						"siteUrl":         "https://example.com/",
						"permissionLevel": "SITE_OWNER",
					},
					{
						"siteUrl":         "sc-domain:example.org",
						"permissionLevel": "SITE_FULL_USER",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

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
			if err := Execute([]string{"--json", "--account", "a@b.com", "search-console", "sites", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Sites []struct {
			SiteUrl         string `json:"siteUrl"`
			PermissionLevel string `json:"permissionLevel"`
		} `json:"sites"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(parsed.Sites))
	}
	if parsed.Sites[0].SiteUrl != "https://example.com/" {
		t.Fatalf("unexpected first site URL: %q", parsed.Sites[0].SiteUrl)
	}
	if parsed.Sites[0].PermissionLevel != "SITE_OWNER" {
		t.Fatalf("unexpected first site permission: %q", parsed.Sites[0].PermissionLevel)
	}
	if parsed.Sites[1].SiteUrl != "sc-domain:example.org" {
		t.Fatalf("unexpected second site URL: %q", parsed.Sites[1].SiteUrl)
	}
}

func TestExecute_SearchConsoleQuery_JSON(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "searchAnalytics/query") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rows": []map[string]any{
					{
						"keys":        []string{"test keyword"},
						"clicks":      42.0,
						"impressions": 1000.0,
						"ctr":         0.042,
						"position":    3.5,
					},
					{
						"keys":        []string{"another query"},
						"clicks":      10.0,
						"impressions": 500.0,
						"ctr":         0.02,
						"position":    7.2,
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

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
				"--json", "--account", "a@b.com",
				"search-console", "query",
				"--site-url", "https://example.com/",
				"--start-date", "2026-01-01",
				"--end-date", "2026-01-31",
				"--dimensions", "query",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

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
	if len(parsed.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(parsed.Rows))
	}
	if parsed.Rows[0].Keys[0] != "test keyword" {
		t.Fatalf("unexpected first row key: %q", parsed.Rows[0].Keys[0])
	}
	if parsed.Rows[0].Clicks != 42 {
		t.Fatalf("unexpected first row clicks: %f", parsed.Rows[0].Clicks)
	}
	if parsed.Rows[0].Position != 3.5 {
		t.Fatalf("unexpected first row position: %f", parsed.Rows[0].Position)
	}
}

func TestExecute_SearchConsoleSitemaps_JSON(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sitemaps") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sitemap": []map[string]any{
					{
						"path":          "https://example.com/sitemap.xml",
						"lastSubmitted": "2026-01-15T00:00:00Z",
						"isPending":     false,
						"warnings":      "0",
						"errors":        "0",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

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
				"--json", "--account", "a@b.com",
				"search-console", "sitemaps", "list",
				"--site-url", "https://example.com/",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Sitemaps []struct {
			Path          string `json:"path"`
			LastSubmitted string `json:"lastSubmitted"`
			IsPending     bool   `json:"isPending"`
			Warnings      int64  `json:"warnings"`
			Errors        int64  `json:"errors"`
		} `json:"sitemaps"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Sitemaps) != 1 {
		t.Fatalf("expected 1 sitemap, got %d", len(parsed.Sitemaps))
	}
	if parsed.Sitemaps[0].Path != "https://example.com/sitemap.xml" {
		t.Fatalf("unexpected sitemap path: %q", parsed.Sitemaps[0].Path)
	}
	if parsed.Sitemaps[0].LastSubmitted != "2026-01-15T00:00:00Z" {
		t.Fatalf("unexpected lastSubmitted: %q", parsed.Sitemaps[0].LastSubmitted)
	}
}

func TestExecute_SearchConsoleQuery_MissingSiteUrl(t *testing.T) {
	err := Execute([]string{
		"--json", "--account", "a@b.com",
		"search-console", "query",
		"--start-date", "2026-01-01",
		"--end-date", "2026-01-31",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	// Kong enforces required flags, so it returns exit code 1 via parse error
	code := ExitCode(err)
	if code != 1 && code != 2 {
		t.Fatalf("expected exit code 1 or 2, got %v", code)
	}
}
