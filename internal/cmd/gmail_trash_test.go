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

func TestExecute_GmailTrash_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/msg123/trash") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "msg123",
				"threadId": "thread1",
			})
			return
		}
		http.NotFound(w, r)
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

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "trash", "msg123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"id"`) || !strings.Contains(out, `"msg123"`) {
			t.Fatalf("unexpected out=%q", out)
		}
		if !strings.Contains(out, `"trashed"`) || !strings.Contains(out, "true") {
			t.Fatalf("expected trashed=true in out=%q", out)
		}
		if !strings.Contains(out, `"threadId"`) || !strings.Contains(out, `"thread1"`) {
			t.Fatalf("expected threadId in out=%q", out)
		}
	})
}

func TestExecute_GmailTrash_EmptyID(t *testing.T) {
	// Kong will reject missing positional arg before Run is called,
	// so we just verify the command fails with a non-nil error.
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "trash"})
			if err == nil {
				t.Fatal("expected error for missing messageId")
			}
		})
	})
}
