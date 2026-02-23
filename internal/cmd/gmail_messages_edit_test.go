package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// TestGmailMessagesImportCmd tests importing a message
func TestGmailMessagesImportCmd_RawInput(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/users/me/messages/import") {
			http.NotFound(w, r)
			return
		}

		var req gmail.Message
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Raw == "" {
			http.Error(w, "missing raw", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "msg123",
			"threadId": "thread456",
		})
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

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailMessagesImportCmd{}
		if err := runKong(t, cmd, []string{"--raw", "dGVzdCBtZXNzYWdl"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.ID != "msg123" {
		t.Fatalf("unexpected id: %q", parsed.ID)
	}
	if parsed.ThreadID != "thread456" {
		t.Fatalf("unexpected threadId: %q", parsed.ThreadID)
	}
}

func TestGmailMessagesImportCmd_MissingInput(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for validation error")
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

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailMessagesImportCmd{}
	err = runKong(t, cmd, []string{}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing input")
	}
	if !strings.Contains(err.Error(), "must provide --raw or --raw-file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestGmailMessagesInsertCmd tests inserting a message
func TestGmailMessagesInsertCmd_RawInput(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The Gmail API endpoint for insert is POST /gmail/v1/users/me/messages
		// with uploadType=multipart or media upload. But the insert method also accepts
		// a simple POST with just the message JSON.
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/users/me/messages") {
			http.NotFound(w, r)
			return
		}
		// Check that it's an insert, not import (insert has no /import in path)
		if strings.Contains(r.URL.Path, "/import") {
			http.NotFound(w, r)
			return
		}

		var req gmail.Message
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "insert123",
			"threadId": "insertthread",
		})
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

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailMessagesInsertCmd{}
		if err := runKong(t, cmd, []string{"--raw", "dGVzdA==", "--thread-id", "mythread"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.ID != "insert123" {
		t.Fatalf("unexpected id: %q", parsed.ID)
	}
}

// TestGmailMessagesModifyCmd tests modifying labels on a message
func TestGmailMessagesModifyCmd_AddLabels(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "Important", "type": "user"},
					{"id": "STARRED", "name": "Starred", "type": "system"},
				},
			})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/me/messages/msg123/modify"):
			var req gmail.ModifyMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "msg123",
				"threadId": "thread123",
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

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailMessagesModifyCmd{}
		if err := runKong(t, cmd, []string{"msg123", "--add", "Important"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		ID          string   `json:"id"`
		ThreadID    string   `json:"threadId"`
		AddedLabels []string `json:"addedLabels"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.ID != "msg123" {
		t.Fatalf("unexpected id: %q", parsed.ID)
	}
	if len(parsed.AddedLabels) != 1 || parsed.AddedLabels[0] != "Label_1" {
		t.Fatalf("unexpected addedLabels: %v", parsed.AddedLabels)
	}
}

func TestGmailMessagesModifyCmd_NoLabelsSpecified(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for validation error")
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

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailMessagesModifyCmd{}
	err = runKong(t, cmd, []string{"msg123"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for no labels")
	}
	if !strings.Contains(err.Error(), "must specify --add and/or --remove") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestGmailMessagesTrashCmd tests trashing a message
func TestGmailMessagesTrashCmd_Success(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/users/me/messages/msg123/trash") {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "msg123",
			"threadId": "thread123",
		})
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

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailMessagesTrashCmd{}
		if err := runKong(t, cmd, []string{"msg123"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
		Trashed  bool   `json:"trashed"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.ID != "msg123" {
		t.Fatalf("unexpected id: %q", parsed.ID)
	}
	if !parsed.Trashed {
		t.Fatal("expected trashed=true")
	}
}

// TestGmailMessagesUntrashCmd tests untrashing a message
func TestGmailMessagesUntrashCmd_Success(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/users/me/messages/msg123/untrash") {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "msg123",
			"threadId": "thread123",
		})
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

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailMessagesUntrashCmd{}
		if err := runKong(t, cmd, []string{"msg123"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		ID        string `json:"id"`
		ThreadID  string `json:"threadId"`
		Untrashed bool   `json:"untrashed"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.ID != "msg123" {
		t.Fatalf("unexpected id: %q", parsed.ID)
	}
	if !parsed.Untrashed {
		t.Fatal("expected untrashed=true")
	}
}

// TestResolveRawMessageInput tests the raw message input resolver
func TestResolveRawMessageInput_PreEncoded(t *testing.T) {
	raw := base64.RawURLEncoding.EncodeToString([]byte("test message"))
	result, err := resolveRawMessageInput(raw, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != raw {
		t.Fatalf("expected %q, got %q", raw, result)
	}
}

func TestResolveRawMessageInput_BothProvided(t *testing.T) {
	_, err := resolveRawMessageInput("raw", "file")
	if err == nil {
		t.Fatal("expected error when both raw and raw-file provided")
	}
}

func TestResolveRawMessageInput_NeitherProvided(t *testing.T) {
	result, err := resolveRawMessageInput("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}
}

// Test execute-level integration for new commands
func TestExecute_GmailMessagesModify_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "Work", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/test123/modify"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "test123",
				"threadId": "thread123",
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

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "messages", "modify", "test123", "--add", "Work"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"test123"`) {
			t.Fatalf("expected test123 in out=%q", out)
		}
	})
}

func TestExecute_GmailMessagesTrash_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/msg456/trash") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "msg456",
				"threadId": "thread456",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "messages", "trash", "msg456"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"msg456"`) {
			t.Fatalf("expected msg456 in out=%q", out)
		}
		if !strings.Contains(out, `"trashed"`) {
			t.Fatalf("expected trashed in out=%q", out)
		}
	})
}

func TestExecute_GmailMessagesUntrash_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/msg789/untrash") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "msg789",
				"threadId": "thread789",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "messages", "untrash", "msg789"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"msg789"`) {
			t.Fatalf("expected msg789 in out=%q", out)
		}
		if !strings.Contains(out, `"untrashed"`) {
			t.Fatalf("expected untrashed in out=%q", out)
		}
	})
}

func TestExecute_GmailMessagesImport(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/import") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "imported123",
				"threadId": "thread123",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "messages", "import", "--raw", "dGVzdA=="}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"imported123"`) {
			t.Fatalf("expected imported123 in out=%q", out)
		}
	})
}

func TestExecute_GmailMessagesInsert(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Gmail API uses POST /gmail/v1/users/me/messages for insert
		// The difference from send/import is in the query parameters or internal routing
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/me/messages") {
			// Make sure it's not the import endpoint
			if strings.Contains(r.URL.Path, "/import") {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "inserted456",
				"threadId": "thread456",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "messages", "insert", "--raw", "dGVzdA=="}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"inserted456"`) {
			t.Fatalf("expected inserted456 in out=%q", out)
		}
	})
}
