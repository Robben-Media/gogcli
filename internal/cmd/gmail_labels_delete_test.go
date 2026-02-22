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

func TestExecute_GmailLabelsDelete_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/labels/"):
			// List labels for name-to-ID resolution
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "MyLabel", "type": "user"},
					{"id": "INBOX", "name": "INBOX", "type": "system"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels/Label_1") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
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

	// Test delete by name (should resolve to ID)
	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "labels", "delete", "MyLabel"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"id"`) || !strings.Contains(out, `"Label_1"`) {
			t.Fatalf("expected id=Label_1 in out=%q", out)
		}
		if !strings.Contains(out, `"deleted"`) || !strings.Contains(out, "true") {
			t.Fatalf("expected deleted=true in out=%q", out)
		}
	})
}

func TestExecute_GmailLabelsDelete_ByID_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/labels/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "MyLabel", "type": "user"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels/Label_1") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
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

	// Test delete by ID directly
	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "labels", "delete", "Label_1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"Label_1"`) {
			t.Fatalf("expected Label_1 in out=%q", out)
		}
	})
}

func TestExecute_GmailLabelsDelete_EmptyLabel(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "labels", "delete"})
			if err == nil {
				t.Fatal("expected error for missing label arg")
			}
		})
	})
}
