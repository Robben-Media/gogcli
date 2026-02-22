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

func TestExecute_SearchConsoleSitesList_JSON(t *testing.T) {
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
	if len(parsed.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(parsed.Sites))
	}
	if parsed.Sites[0].SiteUrl != "https://example.com/" {
		t.Fatalf("unexpected site URL: %q", parsed.Sites[0].SiteUrl)
	}
}

func TestExecute_SearchConsoleSitesGet_JSON(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sites.Get uses path like /sites/{siteUrl}
		if strings.Contains(r.URL.Path, "/sites/") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/sitemaps") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"siteUrl":         "https://example.com/",
				"permissionLevel": "SITE_OWNER",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "search-console", "sites", "get", "https://example.com/"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Site struct {
			SiteUrl         string `json:"siteUrl"`
			PermissionLevel string `json:"permissionLevel"`
		} `json:"site"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Site.SiteUrl != "https://example.com/" {
		t.Fatalf("unexpected site URL: %q", parsed.Site.SiteUrl)
	}
	if parsed.Site.PermissionLevel != "SITE_OWNER" {
		t.Fatalf("unexpected permission level: %q", parsed.Site.PermissionLevel)
	}
}

func TestExecute_SearchConsoleSitesAdd(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sites.Add uses PUT to /sites/{siteUrl}
		if strings.Contains(r.URL.Path, "/sites/") && r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
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

	_ = captureStdout(t, func() {
		stderr := captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "search-console", "sites", "add", "https://example.com/"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(stderr, "Site added") {
			t.Fatalf("expected success message in stderr, got: %q", stderr)
		}
	})
}

func TestExecute_SearchConsoleSitesDelete(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sites.Delete uses DELETE to /sites/{siteUrl}
		if strings.Contains(r.URL.Path, "/sites/") && r.Method == http.MethodDelete && !strings.Contains(r.URL.Path, "/sitemaps") {
			w.WriteHeader(http.StatusNoContent)
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

	_ = captureStdout(t, func() {
		stderr := captureStderr(t, func() {
			if err := Execute([]string{"--force", "--account", "a@b.com", "search-console", "sites", "delete", "https://example.com/"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(stderr, "Site removed") {
			t.Fatalf("expected success message in stderr, got: %q", stderr)
		}
	})
}

func TestExecute_SearchConsoleSitemapsList_JSON(t *testing.T) {
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "search-console", "sitemaps", "list", "--site-url", "https://example.com/"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Sitemaps []struct {
			Path          string `json:"path"`
			LastSubmitted string `json:"lastSubmitted"`
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
}

func TestExecute_SearchConsoleSitemapsGet_JSON(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sitemaps.Get uses path like /sites/{siteUrl}/sitemaps/{feedpath}
		if strings.Contains(r.URL.Path, "/sitemaps/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"path":           "https://example.com/sitemap.xml",
				"type":           "sitemap",
				"lastSubmitted":  "2026-01-15T00:00:00Z",
				"lastDownloaded": "2026-01-16T00:00:00Z",
				"warnings":       "0",
				"errors":         "0",
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
				"search-console", "sitemaps", "get",
				"https://example.com/", "https://example.com/sitemap.xml",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Sitemap struct {
			Path          string `json:"path"`
			Type          string `json:"type"`
			LastSubmitted string `json:"lastSubmitted"`
		} `json:"sitemap"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Sitemap.Path != "https://example.com/sitemap.xml" {
		t.Fatalf("unexpected sitemap path: %q", parsed.Sitemap.Path)
	}
}

func TestExecute_SearchConsoleSitemapsDelete(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sitemaps.Delete uses DELETE to /sites/{siteUrl}/sitemaps/{feedpath}
		if strings.Contains(r.URL.Path, "/sitemaps/") && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
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

	_ = captureStdout(t, func() {
		stderr := captureStderr(t, func() {
			if err := Execute([]string{
				"--force", "--account", "a@b.com",
				"search-console", "sitemaps", "delete",
				"https://example.com/", "https://example.com/sitemap.xml",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(stderr, "Sitemap deleted") {
			t.Fatalf("expected success message in stderr, got: %q", stderr)
		}
	})
}

func TestExecute_SearchConsoleMobileFriendlyTest_JSON(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// MobileFriendlyTest.Run uses POST to /v1/urlTestingTools/mobileFriendlyTest:run
		if strings.Contains(r.URL.Path, "mobileFriendlyTest") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"testStatus": map[string]any{
					"status": "COMPLETE",
				},
				"mobileFriendliness":   "MOBILE_FRIENDLY",
				"mobileFriendlyIssues": []any{},
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
				"search-console", "mobile-friendly-test",
				"--url", "https://example.com/",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		TestResult struct {
			TestStatus         struct{ Status string } `json:"testStatus"`
			MobileFriendliness string                  `json:"mobileFriendliness"`
		} `json:"testResult"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.TestResult.TestStatus.Status != "COMPLETE" {
		t.Fatalf("unexpected test status: %q", parsed.TestResult.TestStatus.Status)
	}
	if parsed.TestResult.MobileFriendliness != "MOBILE_FRIENDLY" {
		t.Fatalf("unexpected mobile friendliness: %q", parsed.TestResult.MobileFriendliness)
	}
}

func TestExecute_SearchConsoleMobileFriendlyTest_WithIssues(t *testing.T) {
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "mobileFriendlyTest") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"testStatus": map[string]any{
					"status": "COMPLETE",
				},
				"mobileFriendliness": "NOT_MOBILE_FRIENDLY",
				"mobileFriendlyIssues": []map[string]any{
					{"rule": "TEXT_TOO_SMALL"},
					{"rule": "VIEWPORT_NOT_CONFIGURED"},
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
				"search-console", "mobile-friendly-test",
				"--url", "https://example.com/notmobile/",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		TestResult struct {
			MobileFriendliness   string `json:"mobileFriendliness"`
			MobileFriendlyIssues []struct {
				Rule string `json:"rule"`
			} `json:"mobileFriendlyIssues"`
		} `json:"testResult"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.TestResult.MobileFriendliness != "NOT_MOBILE_FRIENDLY" {
		t.Fatalf("unexpected mobile friendliness: %q", parsed.TestResult.MobileFriendliness)
	}
	if len(parsed.TestResult.MobileFriendlyIssues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(parsed.TestResult.MobileFriendlyIssues))
	}
}
