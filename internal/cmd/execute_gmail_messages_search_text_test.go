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

func TestExecute_GmailMessagesSearch_PlainKeepsOneRowPerMessage(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/messages") && !strings.Contains(path, "/users/me/messages/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{"id": "m1", "threadId": "t1"},
					{"id": "m2", "threadId": "t1"},
				},
			})
			return
		case strings.Contains(path, "/users/me/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"mimeType": "text/plain",
					"headers": []map[string]any{
						{"name": "From", "value": "Example\tSender\nTeam"},
						{"name": "Subject", "value": "Receipt\rArchive\r\nCopy"},
						{"name": "Date", "value": "Mon, 02 Jan 2006 15:04:05 -0700"},
					},
				},
			})
			return
		case strings.Contains(path, "/users/me/messages/m2"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m2",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "Example <no-reply@example.com>"},
						{"name": "Subject", "value": "Receipt"},
						{"name": "Date", "value": "Mon, 02 Jan 2006 15:05:05 -0700"},
					},
				},
			})
			return
		case strings.Contains(path, "/users/me/labels"):
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
			if err := Execute([]string{"--plain", "--account", "a@b.com", "gmail", "messages", "search", "from:example.com", "--max", "2"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if got, want := len(lines), 3; got != want {
		t.Fatalf("physical rows = %d, want %d; output=%q", got, want, out)
	}
	for i, line := range lines {
		if got, want := len(strings.Split(line, "\t")), 6; got != want {
			t.Fatalf("row %d columns = %d, want %d; row=%q", i, got, want, line)
		}
		if strings.ContainsRune(line, '\r') {
			t.Fatalf("row %d contains carriage return: %q", i, line)
		}
	}
	if !strings.Contains(out, "Example Sender Team") || !strings.Contains(out, "Receipt Archive Copy") {
		t.Fatalf("sanitized fields missing from output: %q", out)
	}
}

func TestExecute_GmailMessagesSearch_JSON_IncludeBody(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/messages") && !strings.Contains(path, "/users/me/messages/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{"id": "m1", "threadId": "t1"},
				},
			})
			return
		case strings.Contains(path, "/users/me/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"mimeType": "multipart/alternative",
					"headers": []map[string]any{
						{"name": "From", "value": "Example <no-reply@example.com>"},
						{"name": "Subject", "value": "Receipt"},
						{"name": "Date", "value": "Mon, 02 Jan 2006 15:04:05 -0700"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"headers": []map[string]any{
								{"name": "Content-Transfer-Encoding", "value": "quoted-printable"},
								{"name": "Content-Type", "value": "text/plain; charset=utf-8"},
							},
							"body": map[string]any{
								"data": encodeBase64URL("Total =E2=82=AC99.99"),
							},
						},
					},
				},
			})
			return
		case strings.Contains(path, "/users/me/labels"):
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "messages", "search", "from:example.com", "--include-body"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Total €99.99") {
		t.Fatalf("expected decoded body, got: %q", out)
	}
}
