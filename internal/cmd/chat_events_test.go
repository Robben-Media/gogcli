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

func TestExecute_ChatEventsGet_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/spaceEvents/")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":      "spaces/abc/spaceEvents/evt1",
			"eventType": "MESSAGE_CREATED",
			"eventTime": "2025-01-01T00:00:00Z",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "events", "get", "spaces/abc/spaceEvents/evt1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	evt, ok := result["event"].(map[string]any)
	if !ok {
		t.Fatalf("expected event object")
	}
	if evt["name"] != "spaces/abc/spaceEvents/evt1" {
		t.Fatalf("expected spaces/abc/spaceEvents/evt1, got %v", evt["name"])
	}
	if evt["eventType"] != "MESSAGE_CREATED" {
		t.Fatalf("expected MESSAGE_CREATED, got %v", evt["eventType"])
	}
	if evt["eventTime"] != "2025-01-01T00:00:00Z" {
		t.Fatalf("expected 2025-01-01T00:00:00Z, got %v", evt["eventTime"])
	}
}

func TestExecute_ChatEventsGet_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/spaceEvents/")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":      "spaces/abc/spaceEvents/evt1",
			"eventType": "MESSAGE_CREATED",
			"eventTime": "2025-01-01T00:00:00Z",
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
			if err := Execute([]string{"--account", "a@b.com", "chat", "events", "get", "spaces/abc/spaceEvents/evt1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "spaces/abc/spaceEvents/evt1") {
		t.Fatalf("expected name in output, got %q", out)
	}
	if !strings.Contains(out, "MESSAGE_CREATED") {
		t.Fatalf("expected eventType in output, got %q", out)
	}
	if !strings.Contains(out, "2025-01-01T00:00:00Z") {
		t.Fatalf("expected eventTime in output, got %q", out)
	}
}

func TestExecute_ChatEventsList_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/spaceEvents")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spaceEvents": []map[string]any{
				{
					"name":      "spaces/abc/spaceEvents/evt1",
					"eventType": "MESSAGE_CREATED",
					"eventTime": "2025-01-01T00:00:00Z",
				},
			},
			"nextPageToken": "token123",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "events", "list", "spaces/abc", "--filter", "eventTypes = \"MESSAGE_CREATED\""}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	events, ok := result["spaceEvents"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected 1 spaceEvent, got %v", events)
	}
	if result["nextPageToken"] != "token123" {
		t.Fatalf("expected nextPageToken token123, got %v", result["nextPageToken"])
	}
}

func TestExecute_ChatEventsList_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/spaceEvents")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spaceEvents": []map[string]any{
				{
					"name":      "spaces/abc/spaceEvents/evt1",
					"eventType": "MESSAGE_CREATED",
					"eventTime": "2025-01-01T00:00:00Z",
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
			if err := Execute([]string{"--account", "a@b.com", "chat", "events", "list", "spaces/abc", "--filter", "eventTypes = \"MESSAGE_CREATED\""}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "RESOURCE") {
		t.Fatalf("expected RESOURCE header, got %q", out)
	}
	if !strings.Contains(out, "EVENT_TYPE") {
		t.Fatalf("expected EVENT_TYPE header, got %q", out)
	}
	if !strings.Contains(out, "TIME") {
		t.Fatalf("expected TIME header, got %q", out)
	}
	if !strings.Contains(out, "spaces/abc/spaceEvents/evt1") {
		t.Fatalf("expected event name in output, got %q", out)
	}
}

func TestExecute_ChatEventsList_Empty(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/spaceEvents")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spaceEvents":   []map[string]any{},
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

	var stderr string
	_ = captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "chat", "events", "list", "spaces/abc", "--filter", "eventTypes = \"MESSAGE_CREATED\""}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "No space events") {
		t.Fatalf("expected 'No space events' in stderr, got %q", stderr)
	}
}
