package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/chat/v1"
	"google.golang.org/api/option"
)

func TestExecute_ChatMessagesGet_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/messages/")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "spaces/aaa/messages/msg1",
			"sender": map[string]any{
				"displayName": "Ada",
				"name":        "users/123",
				"type":        "HUMAN",
			},
			"text":       "hello world",
			"createTime": "2025-06-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "messages", "get", "spaces/aaa/messages/msg1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	msg, ok := result["message"].(map[string]any)
	if !ok {
		t.Fatalf("expected message object")
	}
	if msg["name"] != "spaces/aaa/messages/msg1" {
		t.Fatalf("expected name spaces/aaa/messages/msg1, got %v", msg["name"])
	}
	sender, ok := msg["sender"].(map[string]any)
	if !ok {
		t.Fatalf("expected sender object")
	}
	if sender["displayName"] != "Ada" {
		t.Fatalf("expected sender Ada, got %v", sender["displayName"])
	}
	if msg["text"] != "hello world" {
		t.Fatalf("expected text 'hello world', got %v", msg["text"])
	}
}

func TestExecute_ChatMessagesGet_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "spaces/aaa/messages/msg1",
			"sender": map[string]any{
				"displayName": "Ada",
				"name":        "users/123",
				"type":        "HUMAN",
			},
			"text":       "hello world",
			"createTime": "2025-06-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "chat", "messages", "get", "spaces/aaa/messages/msg1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "resource") || !strings.Contains(out, "spaces/aaa/messages/msg1") {
		t.Fatalf("expected resource in output, got %q", out)
	}
	if !strings.Contains(out, "Ada") {
		t.Fatalf("expected sender Ada in output, got %q", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Fatalf("expected text in output, got %q", out)
	}
}

func TestExecute_ChatMessagesDelete(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/messages/")) {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "chat", "messages", "delete", "spaces/aaa/messages/msg1", "--force"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_ChatMessagesDelete_NoForce(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call — delete should require confirmation")
		return nil, errUnexpectedChatServiceCall
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{"--account", "a@b.com", "--no-input", "chat", "messages", "delete", "spaces/aaa/messages/msg1"})
			if err == nil {
				t.Fatalf("expected error for destructive operation without --force")
			}
		})
	})
}

func TestExecute_ChatMessagesPatch_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var mu sync.Mutex
	var gotUpdateMask string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/messages/") {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		gotUpdateMask = r.URL.Query().Get("updateMask")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "spaces/aaa/messages/msg1",
			"sender": map[string]any{
				"displayName": "Ada",
				"name":        "users/123",
				"type":        "HUMAN",
			},
			"text": "updated text",
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"chat", "messages", "patch", "spaces/aaa/messages/msg1",
				"--text", "updated text",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	if !strings.Contains(gotUpdateMask, "text") {
		t.Fatalf("expected updateMask to contain 'text', got %q", gotUpdateMask)
	}
	mu.Unlock()

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	msg, ok := result["message"].(map[string]any)
	if !ok {
		t.Fatalf("expected message object")
	}
	if msg["text"] != "updated text" {
		t.Fatalf("expected 'updated text', got %v", msg["text"])
	}
}

func TestExecute_ChatMessagesPatch_NoFields(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call — no fields provided should fail before API call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "a@b.com", "chat", "messages", "patch", "spaces/aaa/messages/msg1"})
	if err == nil {
		t.Fatalf("expected error when no fields provided")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_ChatMessagesUpdate_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var mu sync.Mutex
	var gotUpdateMask string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/messages/") {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		gotUpdateMask = r.URL.Query().Get("updateMask")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "spaces/aaa/messages/msg1",
			"sender": map[string]any{
				"displayName": "Ada",
				"name":        "users/123",
				"type":        "HUMAN",
			},
			"text": "fully replaced",
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"chat", "messages", "update", "spaces/aaa/messages/msg1",
				"--text", "fully replaced",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	mu.Lock()
	if !strings.Contains(gotUpdateMask, "text") {
		t.Fatalf("expected updateMask to contain 'text', got %q", gotUpdateMask)
	}
	mu.Unlock()

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	msg, ok := result["message"].(map[string]any)
	if !ok {
		t.Fatalf("expected message object")
	}
	if msg["text"] != "fully replaced" {
		t.Fatalf("expected 'fully replaced', got %v", msg["text"])
	}
}

func TestExecute_ChatAttachmentsGet_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/attachments/")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/aaa/messages/msg1/attachments/att1",
			"contentName": "report.pdf",
			"contentType": "application/pdf",
			"downloadUri": "https://example.com/download/att1",
			"source":      "DRIVE_FILE",
		})
	}))
	defer srv.Close()

	svc, err := chat.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newChatService = func(context.Context, string) (*chat.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"chat", "messages", "attachments", "spaces/aaa/messages/msg1/attachments/att1",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	att, ok := result["attachment"].(map[string]any)
	if !ok {
		t.Fatalf("expected attachment object")
	}
	if att["name"] != "spaces/aaa/messages/msg1/attachments/att1" {
		t.Fatalf("expected attachment name, got %v", att["name"])
	}
	if att["contentName"] != "report.pdf" {
		t.Fatalf("expected contentName 'report.pdf', got %v", att["contentName"])
	}
	if att["contentType"] != "application/pdf" {
		t.Fatalf("expected contentType 'application/pdf', got %v", att["contentType"])
	}
}
