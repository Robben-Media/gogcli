package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestExecute_GmailSearch_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/threads") && !strings.Contains(path, "/users/me/threads/"):
			// threads.list
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"threads":       []map[string]any{{"id": "t1"}},
				"nextPageToken": "npt",
			})
			return
		case strings.Contains(path, "/users/me/threads/t1"):
			// threads.get (metadata)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":       "m1",
						"labelIds": []string{"INBOX"},
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "From", "value": "Me <me@example.com>"},
								{"name": "Subject", "value": "Hello"},
								{"name": "Date", "value": "Mon, 02 Jan 2006 15:04:05 -0700"},
							},
						},
					},
				},
			})
			return
		case strings.Contains(path, "/users/me/labels"):
			// labels.list (used for id->name mapping)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "search", "newer_than:7d", "--max", "1", "--timezone", "UTC"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Threads []struct {
			ID           string   `json:"id"`
			Date         string   `json:"date"`
			From         string   `json:"from"`
			Subject      string   `json:"subject"`
			Labels       []string `json:"labels"`
			MessageCount int      `json:"messageCount"`
		} `json:"threads"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.NextPageToken != "npt" || len(parsed.Threads) != 1 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if parsed.Threads[0].ID != "t1" || parsed.Threads[0].Subject != "Hello" {
		t.Fatalf("unexpected thread: %#v", parsed.Threads[0])
	}
	if parsed.Threads[0].MessageCount != 1 {
		t.Fatalf("unexpected messageCount: %d", parsed.Threads[0].MessageCount)
	}
	if parsed.Threads[0].Date != "2006-01-02 22:04" {
		t.Fatalf("unexpected date: %q", parsed.Threads[0].Date)
	}
	if len(parsed.Threads[0].Labels) != 1 || parsed.Threads[0].Labels[0] != "INBOX" {
		t.Fatalf("unexpected labels: %#v", parsed.Threads[0].Labels)
	}

	resultsOnly := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--results-only", "--select", "id,subject", "--account", "a@b.com", "gmail", "search", "newer_than:7d", "--max", "1", "--timezone", "UTC"}); err != nil {
				t.Fatalf("Execute results-only: %v", err)
			}
		})
	})
	var threads []map[string]any
	if err := json.Unmarshal([]byte(resultsOnly), &threads); err != nil {
		t.Fatalf("results-only JSON parse: %v\nout=%q", err, resultsOnly)
	}
	if len(threads) != 1 || len(threads[0]) != 2 || threads[0]["id"] != "t1" || threads[0]["subject"] != "Hello" {
		t.Fatalf("unexpected results-only threads: %#v", threads)
	}
}

func TestExecute_GmailURL_JSON(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "url", "t1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	var parsed struct {
		URLs []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"urls"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.URLs) != 1 || parsed.URLs[0].ID != "t1" || !strings.Contains(parsed.URLs[0].URL, "#all/t1") {
		t.Fatalf("unexpected urls: %#v", parsed.URLs)
	}
}
