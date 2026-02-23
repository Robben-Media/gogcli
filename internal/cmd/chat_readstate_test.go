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

func TestExecute_ChatNotificationSettingsGet_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/spaceNotificationSetting")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "users/me/spaces/abc/spaceNotificationSetting",
			"muteSetting": "MUTE",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "notification-settings", "get", "users/me/spaces/abc/spaceNotificationSetting"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	ns, ok := result["notificationSetting"].(map[string]any)
	if !ok {
		t.Fatalf("expected notificationSetting object")
	}
	if ns["name"] != "users/me/spaces/abc/spaceNotificationSetting" {
		t.Fatalf("expected name, got %v", ns["name"])
	}
	if ns["muteSetting"] != "MUTE" {
		t.Fatalf("expected MUTE, got %v", ns["muteSetting"])
	}
}

func TestExecute_ChatNotificationSettingsGet_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "users/me/spaces/abc/spaceNotificationSetting",
			"muteSetting": "MUTE",
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
			if err := Execute([]string{"--account", "a@b.com", "chat", "notification-settings", "get", "users/me/spaces/abc/spaceNotificationSetting"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "users/me/spaces/abc/spaceNotificationSetting") {
		t.Fatalf("expected output to contain name, got %q", out)
	}
	if !strings.Contains(out, "MUTE") {
		t.Fatalf("expected output to contain MUTE, got %q", out)
	}
}

func TestExecute_ChatNotificationSettingsPatch_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotUpdateMask string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/spaceNotificationSetting")) {
			http.NotFound(w, r)
			return
		}
		gotUpdateMask = r.URL.Query().Get("updateMask")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "users/me/spaces/abc/spaceNotificationSetting",
			"muteSetting": "MUTE",
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
				"chat", "notification-settings", "patch", "users/me/spaces/abc/spaceNotificationSetting",
				"--mute-setting", "MUTE",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(gotUpdateMask, "muteSetting") {
		t.Fatalf("expected updateMask to contain muteSetting, got %q", gotUpdateMask)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	ns, ok := result["notificationSetting"].(map[string]any)
	if !ok {
		t.Fatalf("expected notificationSetting object")
	}
	if ns["muteSetting"] != "MUTE" {
		t.Fatalf("expected MUTE, got %v", ns["muteSetting"])
	}
}

func TestExecute_ChatNotificationSettingsPatch_NoFields(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call — no fields provided should fail before API call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "a@b.com", "chat", "notification-settings", "patch", "users/me/spaces/abc/spaceNotificationSetting"})
	if err == nil {
		t.Fatalf("expected error when no fields provided")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_ChatThreadReadStateGet_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/threadReadState")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":         "users/me/spaces/abc/threads/t1/threadReadState",
			"lastReadTime": "2025-01-01T00:00:00Z",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "thread-read-state", "get", "users/me/spaces/abc/threads/t1/threadReadState"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	trs, ok := result["threadReadState"].(map[string]any)
	if !ok {
		t.Fatalf("expected threadReadState object")
	}
	if trs["name"] != "users/me/spaces/abc/threads/t1/threadReadState" {
		t.Fatalf("expected name, got %v", trs["name"])
	}
	if trs["lastReadTime"] != "2025-01-01T00:00:00Z" {
		t.Fatalf("expected lastReadTime, got %v", trs["lastReadTime"])
	}
}

func TestExecute_ChatSpaceReadStateUpdate_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotUpdateMask string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/spaceReadState")) {
			http.NotFound(w, r)
			return
		}
		gotUpdateMask = r.URL.Query().Get("updateMask")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":         "users/me/spaces/abc/spaceReadState",
			"lastReadTime": "2025-01-01T00:00:00Z",
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
				"chat", "space-read-state", "update", "users/me/spaces/abc/spaceReadState",
				"--last-read-time", "2025-01-01T00:00:00Z",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(gotUpdateMask, "lastReadTime") {
		t.Fatalf("expected updateMask to contain lastReadTime, got %q", gotUpdateMask)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	srs, ok := result["spaceReadState"].(map[string]any)
	if !ok {
		t.Fatalf("expected spaceReadState object")
	}
	if srs["name"] != "users/me/spaces/abc/spaceReadState" {
		t.Fatalf("expected name, got %v", srs["name"])
	}
	if srs["lastReadTime"] != "2025-01-01T00:00:00Z" {
		t.Fatalf("expected lastReadTime, got %v", srs["lastReadTime"])
	}
}

func TestExecute_ChatSpaceReadStateUpdate_NoFields(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call — no fields provided should fail before API call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "a@b.com", "chat", "space-read-state", "update", "users/me/spaces/abc/spaceReadState"})
	if err == nil {
		t.Fatalf("expected error when no fields provided")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("unexpected error: %v", err)
	}
}
