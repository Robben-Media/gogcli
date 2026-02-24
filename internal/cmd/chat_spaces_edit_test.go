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

func TestExecute_ChatSpacesGet_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/spaces/")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":                "spaces/abc123",
			"displayName":         "Test Space",
			"spaceType":           "SPACE",
			"spaceThreadingState": "THREADED_MESSAGES",
			"spaceUri":            "https://chat.google.com/room/abc123",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "spaces", "get", "abc123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	space, ok := result["space"].(map[string]any)
	if !ok {
		t.Fatalf("expected space object")
	}
	if space["displayName"] != "Test Space" {
		t.Fatalf("expected 'Test Space', got %v", space["displayName"])
	}
}

func TestExecute_ChatSpacesGet_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/abc123",
			"displayName": "Test Space",
			"spaceType":   "SPACE",
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
			if err := Execute([]string{"--account", "a@b.com", "chat", "spaces", "get", "abc123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Test Space") {
		t.Fatalf("expected output to contain 'Test Space', got %q", out)
	}
}

func TestExecute_ChatSpacesDelete(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/spaces/")) {
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
			if err := Execute([]string{"--account", "a@b.com", "chat", "spaces", "delete", "abc123", "--force"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_ChatSpacesDelete_NoForce(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call — delete should require confirmation")
		return nil, errUnexpectedChatServiceCall
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{"--account", "a@b.com", "--no-input", "chat", "spaces", "delete", "abc123"})
			if err == nil {
				t.Fatalf("expected error for destructive operation without --force")
			}
		})
	})
}

func TestExecute_ChatSpacesFindDm_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "findDirectMessage") {
			http.NotFound(w, r)
			return
		}
		// Verify the name query parameter
		nameParam := r.URL.Query().Get("name")
		if nameParam == "" {
			http.Error(w, "missing name param", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/dm123",
			"displayName": "",
			"spaceType":   "DIRECT_MESSAGE",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "spaces", "find-dm", "--user", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	space, ok := result["space"].(map[string]any)
	if !ok {
		t.Fatalf("expected space object")
	}
	if space["name"] != "spaces/dm123" {
		t.Fatalf("expected spaces/dm123, got %v", space["name"])
	}
}

func TestExecute_ChatSpacesPatch_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotUpdateMask string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/spaces/") {
			http.NotFound(w, r)
			return
		}
		gotUpdateMask = r.URL.Query().Get("updateMask")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/abc123",
			"displayName": "Updated Space",
			"spaceType":   "SPACE",
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
				"chat", "spaces", "patch", "abc123",
				"--display-name", "Updated Space",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(gotUpdateMask, "displayName") {
		t.Fatalf("expected updateMask to contain displayName, got %q", gotUpdateMask)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	space, ok := result["space"].(map[string]any)
	if !ok {
		t.Fatalf("expected space object")
	}
	if space["displayName"] != "Updated Space" {
		t.Fatalf("expected 'Updated Space', got %v", space["displayName"])
	}
}

func TestExecute_ChatSpacesSearch_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "search") {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query().Get("query")
		if query == "" {
			http.Error(w, "missing query", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spaces": []map[string]any{
				{
					"name":        "spaces/space1",
					"displayName": "Space One",
					"spaceType":   "SPACE",
				},
				{
					"name":        "spaces/space2",
					"displayName": "Space Two",
					"spaceType":   "SPACE",
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
			if err := Execute([]string{
				"--json", "--account", "a@b.com",
				"chat", "spaces", "search",
				"--query", `spaceType = "SPACE"`,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	spaces, ok := result["spaces"].([]any)
	if !ok || len(spaces) != 2 {
		t.Fatalf("expected 2 spaces, got %v", spaces)
	}
	if result["nextPageToken"] != "token123" {
		t.Fatalf("expected nextPageToken 'token123', got %v", result["nextPageToken"])
	}
}

func TestExecute_ChatSpacesSearch_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spaces": []map[string]any{
				{
					"name":                "spaces/space1",
					"displayName":         "Space One",
					"spaceType":           "SPACE",
					"spaceThreadingState": "THREADED_MESSAGES",
				},
			},
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
				"--account", "a@b.com",
				"chat", "spaces", "search",
				"--query", `spaceType = "SPACE"`,
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Space One") {
		t.Fatalf("expected output to contain 'Space One', got %q", out)
	}
	if !strings.Contains(out, "THREADED_MESSAGES") {
		t.Fatalf("expected output to contain 'THREADED_MESSAGES', got %q", out)
	}
}

func TestExecute_ChatSpacesCompleteImport_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, ":completeImport") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"space": map[string]any{
				"name":        "spaces/import123",
				"displayName": "Imported Space",
				"spaceType":   "SPACE",
			},
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "spaces", "complete-import", "import123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	space, ok := result["space"].(map[string]any)
	if !ok {
		t.Fatalf("expected space object")
	}
	if space["displayName"] != "Imported Space" {
		t.Fatalf("expected 'Imported Space', got %v", space["displayName"])
	}
}

func TestExecute_ChatSpacesCreateDirect_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/spaces") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "spaces/new123",
			"displayName": "New Space",
			"spaceType":   "SPACE",
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
				"chat", "spaces", "create-direct",
				"--display-name", "New Space",
				"--description", "A test space",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotBody["displayName"] != "New Space" {
		t.Fatalf("expected displayName 'New Space', got %v", gotBody["displayName"])
	}
	// Description is nested under spaceDetails
	spaceDetails, ok := gotBody["spaceDetails"].(map[string]any)
	if !ok {
		t.Fatalf("expected spaceDetails object, got %v", gotBody["spaceDetails"])
	}
	if spaceDetails["description"] != "A test space" {
		t.Fatalf("expected description 'A test space', got %v", spaceDetails["description"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	space, ok := result["space"].(map[string]any)
	if !ok {
		t.Fatalf("expected space object")
	}
	if space["name"] != "spaces/new123" {
		t.Fatalf("expected spaces/new123, got %v", space["name"])
	}
}

func TestExecute_ChatSpaces_ConsumerBlocked(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call")
		return nil, errUnexpectedChatServiceCall
	}

	tests := []struct {
		name string
		args []string
	}{
		{"get", []string{"--account", "user@gmail.com", "chat", "spaces", "get", "abc123"}},
		{"delete", []string{"--account", "user@gmail.com", "chat", "spaces", "delete", "abc123", "--force"}},
		{"find-dm", []string{"--account", "user@gmail.com", "chat", "spaces", "find-dm", "--user", "test@example.com"}},
		{"search", []string{"--account", "user@gmail.com", "chat", "spaces", "search", "--query", "test"}},
		{"patch", []string{"--account", "user@gmail.com", "chat", "spaces", "patch", "abc123", "--display-name", "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Execute(tt.args)
			if err == nil {
				t.Fatalf("expected error for consumer account")
			}
			if !strings.Contains(err.Error(), "Workspace") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestExecute_ChatSpacesPatch_NoFields(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call — no fields provided should fail before API call")
		return nil, errUnexpectedChatServiceCall
	}

	err := Execute([]string{"--account", "a@b.com", "chat", "spaces", "patch", "abc123"})
	if err == nil {
		t.Fatalf("expected error when no fields provided")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("unexpected error: %v", err)
	}
}
