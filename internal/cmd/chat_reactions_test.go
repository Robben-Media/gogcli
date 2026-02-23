package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/chat/v1"
	"google.golang.org/api/option"
)

func TestExecute_ChatReactionsList_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/messages/msg1/reactions")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"reactions": []map[string]any{
				{
					"name":  "spaces/abc/messages/msg1/reactions/r1",
					"emoji": map[string]any{"unicode": "\U0001f44d"},
					"user":  map[string]any{"displayName": "Ada", "name": "users/123", "type": "HUMAN"},
				},
				{
					"name":  "spaces/abc/messages/msg1/reactions/r2",
					"emoji": map[string]any{"unicode": "\u2764\ufe0f"},
					"user":  map[string]any{"displayName": "Bob", "name": "users/456", "type": "HUMAN"},
				},
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "reactions", "list", "spaces/abc/messages/msg1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	reactions, ok := result["reactions"].([]any)
	if !ok || len(reactions) != 2 {
		t.Fatalf("expected 2 reactions, got %v", reactions)
	}
	first, ok := reactions[0].(map[string]any)
	if !ok {
		t.Fatalf("expected reaction object")
	}
	if first["emoji"] != "\U0001f44d" {
		t.Fatalf("expected thumbs up emoji, got %v", first["emoji"])
	}
	if first["user"] != "Ada" {
		t.Fatalf("expected user Ada, got %v", first["user"])
	}
}

func TestExecute_ChatReactionsList_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/messages/msg1/reactions")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"reactions": []map[string]any{
				{
					"name":  "spaces/abc/messages/msg1/reactions/r1",
					"emoji": map[string]any{"unicode": "\U0001f44d"},
					"user":  map[string]any{"displayName": "Ada", "name": "users/123", "type": "HUMAN"},
				},
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
			if err := Execute([]string{"--account", "a@b.com", "chat", "reactions", "list", "spaces/abc/messages/msg1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "RESOURCE") {
		t.Fatalf("expected RESOURCE header in text output, got: %s", out)
	}
	if !strings.Contains(out, "\U0001f44d") {
		t.Fatalf("expected thumbs up emoji in text output, got: %s", out)
	}
	if !strings.Contains(out, "Ada") {
		t.Fatalf("expected user Ada in text output, got: %s", out)
	}
}

func TestExecute_ChatReactionsCreate_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotUnicode string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/messages/msg1/reactions")) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if emoji, ok := body["emoji"].(map[string]any); ok {
			gotUnicode, _ = emoji["unicode"].(string)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "spaces/abc/messages/msg1/reactions/r1",
			"emoji": map[string]any{"unicode": "\U0001f44d"},
			"user":  map[string]any{"displayName": "Ada", "name": "users/123", "type": "HUMAN"},
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "reactions", "create", "spaces/abc/messages/msg1", "--emoji", "\U0001f44d"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotUnicode != "\U0001f44d" {
		t.Fatalf("expected unicode thumbs up in request body, got %q", gotUnicode)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	reaction, ok := result["reaction"].(map[string]any)
	if !ok {
		t.Fatalf("expected reaction object")
	}
	if reaction["name"] != "spaces/abc/messages/msg1/reactions/r1" {
		t.Fatalf("expected reaction name, got %v", reaction["name"])
	}
}

func TestExecute_ChatReactionsCreate_CustomEmoji_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotCustomEmojiUID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/messages/msg1/reactions")) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if emoji, ok := body["emoji"].(map[string]any); ok {
			if custom, ok := emoji["customEmoji"].(map[string]any); ok {
				gotCustomEmojiUID, _ = custom["uid"].(string)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "spaces/abc/messages/msg1/reactions/r2",
			"emoji": map[string]any{
				"customEmoji": map[string]any{"uid": "customEmojis/myemoji"},
			},
			"user": map[string]any{"displayName": "Ada", "name": "users/123", "type": "HUMAN"},
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "reactions", "create", "spaces/abc/messages/msg1", "--emoji", "customEmojis/myemoji"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotCustomEmojiUID != "customEmojis/myemoji" {
		t.Fatalf("expected customEmojis/myemoji in request body, got %q", gotCustomEmojiUID)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	if _, ok := result["reaction"]; !ok {
		t.Fatalf("expected reaction key in response")
	}
}

func TestExecute_ChatReactionsDelete(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/reactions/")) {
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
			if err := Execute([]string{"--account", "a@b.com", "chat", "reactions", "delete", "spaces/abc/messages/msg1/reactions/r1", "--force"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_ChatReactionsDelete_NoForce(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call — delete should require confirmation")
		return nil, errUnexpectedChatServiceCall
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{"--account", "a@b.com", "--no-input", "chat", "reactions", "delete", "spaces/abc/messages/msg1/reactions/r1"})
			if err == nil {
				t.Fatalf("expected error for destructive operation without --force")
			}
		})
	})
}
