package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/chat/v1"
	"google.golang.org/api/option"
)

func TestExecute_ChatEmojiList_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/customEmojis")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customEmojis": []map[string]any{
				{"name": "customEmojis/abc123", "emojiName": ":happy-face:", "uid": "uid123"},
				{"name": "customEmojis/def456", "emojiName": ":sad-face:", "uid": "uid456"},
			},
			"nextPageToken": "",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "emoji", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	emojis, ok := result["customEmojis"].([]any)
	if !ok || len(emojis) != 2 {
		t.Fatalf("expected 2 emojis, got %v", emojis)
	}
}

func TestExecute_ChatEmojiGet_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/customEmojis/")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":              "customEmojis/abc123",
			"emojiName":         ":happy-face:",
			"uid":               "uid123",
			"temporaryImageUri": "https://example.com/image.png",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "emoji", "get", "abc123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	emoji, ok := result["customEmoji"].(map[string]any)
	if !ok {
		t.Fatalf("expected customEmoji object")
	}
	if emoji["emojiName"] != ":happy-face:" {
		t.Fatalf("expected :happy-face:, got %v", emoji["emojiName"])
	}
}

func TestExecute_ChatEmojiDelete(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/customEmojis/")) {
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
			if err := Execute([]string{"--account", "a@b.com", "chat", "emoji", "delete", "abc123", "--force"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_ChatEmojiCreate_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotEmojiName string
	var gotFilename string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/customEmojis")) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotEmojiName, _ = body["emojiName"].(string)
		if payload, ok := body["payload"].(map[string]any); ok {
			gotFilename, _ = payload["filename"].(string)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":      "customEmojis/new123",
			"emojiName": ":test-emoji:",
			"uid":       "uidnew",
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

	// Create a temp file for testing
	tmpFile, err := os.CreateTemp("", "emoji-test-*.png")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	tmpFile.WriteString("test image content")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "emoji", "create", "--name", ":test-emoji:", "--file", tmpFile.Name()}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotEmojiName != ":test-emoji:" {
		t.Fatalf("expected :test-emoji:, got %q", gotEmojiName)
	}
	if !strings.HasSuffix(gotFilename, ".png") {
		t.Fatalf("expected .png filename, got %q", gotFilename)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	emoji, ok := result["customEmoji"].(map[string]any)
	if !ok {
		t.Fatalf("expected customEmoji object")
	}
	if emoji["emojiName"] != ":test-emoji:" {
		t.Fatalf("expected :test-emoji:, got %v", emoji["emojiName"])
	}
}

func TestExecute_ChatEmoji_ConsumerBlocked(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "user@gmail.com", "chat", "emoji", "list"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "Workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsValidEmojiName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid simple", ":happy:", true},
		{"valid with hyphen", ":happy-face:", true},
		{"valid with underscore", ":happy_face:", true},
		{"valid with numbers", ":emoji123:", true},
		{"missing leading colon", "happy:", false},
		{"missing trailing colon", ":happy", false},
		{"missing both colons", "happy", false},
		{"uppercase invalid", ":Happy:", false},
		{"space invalid", ":happy face:", false},
		{"special char invalid", ":happy@face:", false},
		{"too short", "::", false},
		{"single char", ":a:", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidEmojiName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidEmojiName(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}
