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

func setupSearchConsoleMutationService(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(handler)
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
}

func searchConsoleMutationOK(w http.ResponseWriter, _ *http.Request) {
	// Mutation endpoints return empty success bodies (204 / empty JSON).
	w.WriteHeader(http.StatusNoContent)
}

func TestExecute_SearchConsoleSitesAdd_MutationReceiptJSON(t *testing.T) {
	setupSearchConsoleMutationService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/sites/") {
			searchConsoleMutationOK(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"search-console", "sites", "add",
				"https://example.com/",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Action     string `json:"action"`
		SiteURL    string `json:"siteUrl"`
		SitemapURL string `json:"sitemapUrl"`
		Success    bool   `json:"success"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Action != "sites.add" {
		t.Fatalf("action = %q, want sites.add", parsed.Action)
	}
	if parsed.SiteURL != "https://example.com/" {
		t.Fatalf("siteUrl = %q, want https://example.com/", parsed.SiteURL)
	}
	if parsed.SitemapURL != "" {
		t.Fatalf("sitemapUrl should be omitted/empty for site ops, got %q", parsed.SitemapURL)
	}
	if !parsed.Success {
		t.Fatal("expected success=true")
	}
	if strings.Contains(out, "Site added") {
		t.Fatalf("JSON stdout must not contain human prose: %q", out)
	}
}

func TestExecute_SearchConsoleSitesAdd_MutationReceiptPlain(t *testing.T) {
	setupSearchConsoleMutationService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/sites/") {
			searchConsoleMutationOK(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"search-console", "sites", "add",
				"https://example.com/",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	const want = "ACTION\tSITE_URL\tSITEMAP_URL\tSUCCESS\nsites.add\thttps://example.com/\t\ttrue\n"
	if out != want {
		t.Fatalf("plain output mismatch:\nwant %q\ngot  %q", want, out)
	}
}

func TestExecute_SearchConsoleSitesDelete_MutationReceiptJSON(t *testing.T) {
	setupSearchConsoleMutationService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/sites/") && !strings.Contains(r.URL.Path, "/sitemaps") {
			searchConsoleMutationOK(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--force", "--account", "a@b.com",
				"search-console", "sites", "delete",
				"sc-domain:example.org",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Action     string `json:"action"`
		SiteURL    string `json:"siteUrl"`
		SitemapURL string `json:"sitemapUrl"`
		Success    bool   `json:"success"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Action != "sites.delete" {
		t.Fatalf("action = %q, want sites.delete", parsed.Action)
	}
	if parsed.SiteURL != "sc-domain:example.org" {
		t.Fatalf("siteUrl = %q, want sc-domain:example.org", parsed.SiteURL)
	}
	if parsed.SitemapURL != "" {
		t.Fatalf("sitemapUrl should be empty for site ops, got %q", parsed.SitemapURL)
	}
	if !parsed.Success {
		t.Fatal("expected success=true")
	}
}

func TestExecute_SearchConsoleSitesDelete_MutationReceiptPlain(t *testing.T) {
	setupSearchConsoleMutationService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/sites/") && !strings.Contains(r.URL.Path, "/sitemaps") {
			searchConsoleMutationOK(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--force", "--account", "a@b.com",
				"search-console", "sites", "delete",
				"sc-domain:example.org",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	const want = "ACTION\tSITE_URL\tSITEMAP_URL\tSUCCESS\nsites.delete\tsc-domain:example.org\t\ttrue\n"
	if out != want {
		t.Fatalf("plain output mismatch:\nwant %q\ngot  %q", want, out)
	}
}

func TestExecute_SearchConsoleSubmitSitemap_MutationReceiptJSON(t *testing.T) {
	setupSearchConsoleMutationService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/sitemaps/") {
			searchConsoleMutationOK(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"search-console", "submit-sitemap",
				"--site-url", "https://example.com/",
				"--sitemap-url", "https://example.com/sitemap.xml",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Action     string `json:"action"`
		SiteURL    string `json:"siteUrl"`
		SitemapURL string `json:"sitemapUrl"`
		Success    bool   `json:"success"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Action != "sitemaps.submit" {
		t.Fatalf("action = %q, want sitemaps.submit", parsed.Action)
	}
	if parsed.SiteURL != "https://example.com/" {
		t.Fatalf("siteUrl = %q, want https://example.com/", parsed.SiteURL)
	}
	if parsed.SitemapURL != "https://example.com/sitemap.xml" {
		t.Fatalf("sitemapUrl = %q, want https://example.com/sitemap.xml", parsed.SitemapURL)
	}
	if !parsed.Success {
		t.Fatal("expected success=true")
	}
	if strings.Contains(out, "submitted") {
		t.Fatalf("JSON stdout must not contain human prose: %q", out)
	}
}

func TestExecute_SearchConsoleSubmitSitemap_MutationReceiptPlain(t *testing.T) {
	setupSearchConsoleMutationService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/sitemaps/") {
			searchConsoleMutationOK(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--account", "a@b.com",
				"search-console", "submit-sitemap",
				"--site-url", "https://example.com/",
				"--sitemap-url", "https://example.com/sitemap.xml",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	const want = "ACTION\tSITE_URL\tSITEMAP_URL\tSUCCESS\nsitemaps.submit\thttps://example.com/\thttps://example.com/sitemap.xml\ttrue\n"
	if out != want {
		t.Fatalf("plain output mismatch:\nwant %q\ngot  %q", want, out)
	}
}

func TestExecute_SearchConsoleSitemapsDelete_MutationReceiptJSON(t *testing.T) {
	setupSearchConsoleMutationService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/sitemaps/") {
			searchConsoleMutationOK(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--force", "--account", "a@b.com",
				"search-console", "sitemaps", "delete",
				"https://example.com/",
				"https://example.com/sitemap.xml",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Action     string `json:"action"`
		SiteURL    string `json:"siteUrl"`
		SitemapURL string `json:"sitemapUrl"`
		Success    bool   `json:"success"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Action != "sitemaps.delete" {
		t.Fatalf("action = %q, want sitemaps.delete", parsed.Action)
	}
	if parsed.SiteURL != "https://example.com/" {
		t.Fatalf("siteUrl = %q, want https://example.com/", parsed.SiteURL)
	}
	if parsed.SitemapURL != "https://example.com/sitemap.xml" {
		t.Fatalf("sitemapUrl = %q, want https://example.com/sitemap.xml", parsed.SitemapURL)
	}
	if !parsed.Success {
		t.Fatal("expected success=true")
	}
}

func TestExecute_SearchConsoleSitemapsDelete_MutationReceiptPlain(t *testing.T) {
	setupSearchConsoleMutationService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/sitemaps/") {
			searchConsoleMutationOK(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain", "--force", "--account", "a@b.com",
				"search-console", "sitemaps", "delete",
				"https://example.com/",
				"https://example.com/sitemap.xml",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	const want = "ACTION\tSITE_URL\tSITEMAP_URL\tSUCCESS\nsitemaps.delete\thttps://example.com/\thttps://example.com/sitemap.xml\ttrue\n"
	if out != want {
		t.Fatalf("plain output mismatch:\nwant %q\ngot  %q", want, out)
	}
}
