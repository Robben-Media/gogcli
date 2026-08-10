package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"
)

type searchConsoleMutationCase struct {
	name       string
	method     string
	pathPart   string
	args       []string
	action     string
	siteURL    string
	sitemapURL string
	stderr     string
}

var searchConsoleMutationCases = []searchConsoleMutationCase{
	{
		name:     "sites add",
		method:   http.MethodPut,
		pathPart: "/sites/",
		args: []string{
			"search-console", "sites", "add", "https://example.com/",
		},
		action:  "sites.add",
		siteURL: "https://example.com/",
		stderr:  "Site added: https://example.com/. Verify ownership to access data.",
	},
	{
		name:     "sites delete",
		method:   http.MethodDelete,
		pathPart: "/sites/",
		args: []string{
			"--force", "search-console", "sites", "delete", "sc-domain:example.org",
		},
		action:  "sites.delete",
		siteURL: "sc-domain:example.org",
		stderr:  "Site removed",
	},
	{
		name:     "sitemaps submit",
		method:   http.MethodPut,
		pathPart: "/sitemaps/",
		args: []string{
			"search-console", "submit-sitemap",
			"--site-url", "https://example.com/",
			"--sitemap-url", "https://example.com/sitemap.xml",
		},
		action:     "sitemaps.submit",
		siteURL:    "https://example.com/",
		sitemapURL: "https://example.com/sitemap.xml",
		stderr:     "Sitemap submitted successfully",
	},
	{
		name:     "sitemaps delete",
		method:   http.MethodDelete,
		pathPart: "/sitemaps/",
		args: []string{
			"--force", "search-console", "sitemaps", "delete",
			"https://example.com/", "https://example.com/sitemap.xml",
		},
		action:     "sitemaps.delete",
		siteURL:    "https://example.com/",
		sitemapURL: "https://example.com/sitemap.xml",
		stderr:     "Sitemap deleted",
	},
}

func setupSearchConsoleMutationService(t *testing.T, tc searchConsoleMutationCase) {
	t.Helper()
	origNew := newSearchConsoleService
	t.Cleanup(func() { newSearchConsoleService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != tc.method || !strings.Contains(r.URL.Path, tc.pathPart) {
			t.Errorf("request = %s %s, want %s path containing %q", r.Method, r.URL.Path, tc.method, tc.pathPart)
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
}

func executeSearchConsoleMutation(t *testing.T, tc searchConsoleMutationCase, outputFlag string) (string, string) {
	t.Helper()
	setupSearchConsoleMutationService(t, tc)

	args := make([]string, 0, 3+len(tc.args))
	args = append(args, outputFlag, "--account", "a@b.com")
	args = append(args, tc.args...)
	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if err := Execute(args); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	return stdout, stderr
}

func TestExecute_SearchConsoleMutationReceiptJSON(t *testing.T) {
	for _, tc := range searchConsoleMutationCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := executeSearchConsoleMutation(t, tc, "--json")

			want := map[string]any{
				"action":  tc.action,
				"siteUrl": tc.siteURL,
				"success": true,
			}
			if tc.sitemapURL != "" {
				want["sitemapUrl"] = tc.sitemapURL
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("json parse: %v\nout=%q", err, stdout)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("JSON receipt mismatch:\nwant %#v\ngot  %#v", want, got)
			}
			if !strings.Contains(stderr, tc.stderr) {
				t.Fatalf("stderr = %q, want confirmation containing %q", stderr, tc.stderr)
			}
		})
	}
}

func TestExecute_SearchConsoleMutationReceiptPlain(t *testing.T) {
	for _, tc := range searchConsoleMutationCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := executeSearchConsoleMutation(t, tc, "--plain")

			want := "ACTION\tSITE_URL\tSITEMAP_URL\tSUCCESS\n" +
				tc.action + "\t" + tc.siteURL + "\t" + tc.sitemapURL + "\ttrue\n"
			if stdout != want {
				t.Fatalf("plain output mismatch:\nwant %q\ngot  %q", want, stdout)
			}
			if !strings.Contains(stderr, tc.stderr) {
				t.Fatalf("stderr = %q, want confirmation containing %q", stderr, tc.stderr)
			}
		})
	}
}
