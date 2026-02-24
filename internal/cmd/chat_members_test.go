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

func TestExecute_ChatMembersList_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/members")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"memberships": []map[string]any{
				{
					"name":  "spaces/abc/members/user1",
					"role":  "ROLE_MEMBER",
					"state": "JOINED",
					"member": map[string]any{
						"name":        "users/user1",
						"displayName": "Alice",
						"type":        "HUMAN",
					},
				},
				{
					"name":  "spaces/abc/members/user2",
					"role":  "ROLE_MANAGER",
					"state": "JOINED",
					"member": map[string]any{
						"name":        "users/user2",
						"displayName": "Bob",
						"type":        "HUMAN",
					},
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "members", "list", "spaces/abc"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	members, ok := result["memberships"].([]any)
	if !ok || len(members) != 2 {
		t.Fatalf("expected 2 memberships, got %v", members)
	}
}

func TestExecute_ChatMembersList_Text(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"memberships": []map[string]any{
				{
					"name":  "spaces/abc/members/user1",
					"role":  "ROLE_MEMBER",
					"state": "JOINED",
					"member": map[string]any{
						"name":        "users/user1",
						"displayName": "Alice",
						"type":        "HUMAN",
					},
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
			if err := Execute([]string{"--account", "a@b.com", "chat", "members", "list", "spaces/abc"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "RESOURCE") {
		t.Fatalf("expected output to contain 'RESOURCE' header, got %q", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Fatalf("expected output to contain 'Alice', got %q", out)
	}
}

func TestExecute_ChatMembersGet_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/members/")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "spaces/abc/members/user1",
			"role":  "ROLE_MEMBER",
			"state": "JOINED",
			"member": map[string]any{
				"name":        "users/user1",
				"displayName": "Alice",
				"type":        "HUMAN",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "chat", "members", "get", "spaces/abc/members/user1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	membership, ok := result["membership"].(map[string]any)
	if !ok {
		t.Fatalf("expected membership object")
	}
	if membership["name"] != "spaces/abc/members/user1" {
		t.Fatalf("expected spaces/abc/members/user1, got %v", membership["name"])
	}
}

func TestExecute_ChatMembersCreate_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/members")) {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "spaces/abc/members/newuser",
			"role":  "ROLE_MEMBER",
			"state": "JOINED",
			"member": map[string]any{
				"name":        "users/newuser",
				"displayName": "New User",
				"type":        "HUMAN",
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
				"--json", "--account", "a@b.com",
				"chat", "members", "create", "spaces/abc",
				"--user", "users/newuser",
				"--role", "ROLE_MEMBER",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	// Verify the request body contains member.name (user)
	member, ok := gotBody["member"].(map[string]any)
	if !ok {
		t.Fatalf("expected member object in request body, got %v", gotBody)
	}
	if member["name"] != "users/newuser" {
		t.Fatalf("expected member.name 'users/newuser', got %v", member["name"])
	}
	if gotBody["role"] != "ROLE_MEMBER" {
		t.Fatalf("expected role 'ROLE_MEMBER', got %v", gotBody["role"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	membership, ok := result["membership"].(map[string]any)
	if !ok {
		t.Fatalf("expected membership object")
	}
	if membership["name"] != "spaces/abc/members/newuser" {
		t.Fatalf("expected spaces/abc/members/newuser, got %v", membership["name"])
	}
}

func TestExecute_ChatMembersDelete(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/members/")) {
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
			if err := Execute([]string{"--account", "a@b.com", "chat", "members", "delete", "spaces/abc/members/user1", "--force"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_ChatMembersDelete_NoForce(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })
	newChatService = func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("unexpected chat service call — delete should require confirmation")
		return nil, errUnexpectedChatServiceCall
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{"--account", "a@b.com", "--no-input", "chat", "members", "delete", "spaces/abc/members/user1"})
			if err == nil {
				t.Fatalf("expected error for destructive operation without --force")
			}
		})
	})
}

func TestExecute_ChatMembersPatch_JSON(t *testing.T) {
	origNew := newChatService
	t.Cleanup(func() { newChatService = origNew })

	var gotUpdateMask string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/members/") {
			http.NotFound(w, r)
			return
		}
		gotUpdateMask = r.URL.Query().Get("updateMask")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":  "spaces/abc/members/user1",
			"role":  "ROLE_MANAGER",
			"state": "JOINED",
			"member": map[string]any{
				"name":        "users/user1",
				"displayName": "Alice",
				"type":        "HUMAN",
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
				"--json", "--account", "a@b.com",
				"chat", "members", "patch", "spaces/abc/members/user1",
				"--role", "ROLE_MANAGER",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(gotUpdateMask, "role") {
		t.Fatalf("expected updateMask to contain 'role', got %q", gotUpdateMask)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parsing JSON: %v", err)
	}
	membership, ok := result["membership"].(map[string]any)
	if !ok {
		t.Fatalf("expected membership object")
	}
	if membership["role"] != "ROLE_MANAGER" {
		t.Fatalf("expected 'ROLE_MANAGER', got %v", membership["role"])
	}
}
