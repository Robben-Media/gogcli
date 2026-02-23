package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	keepapi "google.golang.org/api/keep/v1"
	"google.golang.org/api/option"
)

func TestKeepPermissions_BatchCreate_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v1/notes/note1/permissions:batchCreate") {
			var req keepapi.BatchCreatePermissionsRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if len(req.Requests) != 2 {
				t.Fatalf("expected 2 requests, got %d", len(req.Requests))
			}
			if req.Requests[0].Permission.Email != "user1@example.com" {
				t.Fatalf("expected email 'user1@example.com', got %q", req.Requests[0].Permission.Email)
			}
			if req.Requests[0].Permission.Role != "WRITER" {
				t.Fatalf("expected role 'WRITER', got %q", req.Requests[0].Permission.Role)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"name": "notes/note1/permissions/perm1", "email": "user1@example.com", "role": "WRITER"},
					{"name": "notes/note1/permissions/perm2", "email": "user2@example.com", "role": "WRITER"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	orig := newKeepServiceWithSA
	t.Cleanup(func() { newKeepServiceWithSA = orig })
	newKeepServiceWithSA = func(ctx context.Context, _, _ string) (*keepapi.Service, error) {
		return keepapi.NewService(ctx,
			option.WithEndpoint(srv.URL+"/"),
			option.WithHTTPClient(srv.Client()),
			option.WithoutAuthentication(),
		)
	}

	stdout := captureStdout(t, func() {
		if err := Execute([]string{
			"keep", "permissions", "batch-create", "note1",
			"--account", account,
			"--json",
			"--members", "user1@example.com",
			"--members", "user2@example.com",
		}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	perms, ok := payload["permissions"].([]any)
	if !ok {
		t.Fatal("expected permissions array in response")
	}
	if len(perms) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(perms))
	}
}

func TestKeepPermissions_BatchCreate_ShortParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should auto-prefix "notes/" to short parent
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v1/notes/shortid/permissions:batchCreate") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"name": "notes/shortid/permissions/perm1", "email": "user@example.com", "role": "WRITER"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	orig := newKeepServiceWithSA
	t.Cleanup(func() { newKeepServiceWithSA = orig })
	newKeepServiceWithSA = func(ctx context.Context, _, _ string) (*keepapi.Service, error) {
		return keepapi.NewService(ctx,
			option.WithEndpoint(srv.URL+"/"),
			option.WithHTTPClient(srv.Client()),
			option.WithoutAuthentication(),
		)
	}

	stdout := captureStdout(t, func() {
		if err := Execute([]string{
			"keep", "permissions", "batch-create", "shortid",
			"--account", account,
			"--json",
			"--members", "user@example.com",
		}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	perms, ok := payload["permissions"].([]any)
	if !ok || len(perms) != 1 {
		t.Fatal("expected 1 permission in response")
	}
}

func TestKeepPermissions_BatchCreate_NoMembers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	if err := Execute([]string{
		"keep", "permissions", "batch-create", "note1",
		"--account", account,
	}); err == nil {
		t.Fatal("expected error")
	}
}

func TestKeepPermissions_BatchCreate_InvalidRole(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	if err := Execute([]string{
		"keep", "permissions", "batch-create", "note1",
		"--account", account,
		"--members", "user@example.com",
		"--role", "OWNER",
	}); err == nil {
		t.Fatal("expected error for non-WRITER role")
	}
}

func TestKeepPermissions_BatchDelete_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v1/notes/note1/permissions:batchDelete") {
			var req keepapi.BatchDeletePermissionsRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if len(req.Names) != 2 {
				t.Fatalf("expected 2 names, got %d", len(req.Names))
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	orig := newKeepServiceWithSA
	t.Cleanup(func() { newKeepServiceWithSA = orig })
	newKeepServiceWithSA = func(ctx context.Context, _, _ string) (*keepapi.Service, error) {
		return keepapi.NewService(ctx,
			option.WithEndpoint(srv.URL+"/"),
			option.WithHTTPClient(srv.Client()),
			option.WithoutAuthentication(),
		)
	}

	stdout := captureStdout(t, func() {
		if err := Execute([]string{
			"keep", "permissions", "batch-delete", "note1",
			"--account", account,
			"--json",
			"--force",
			"--permission-names", "notes/note1/permissions/perm1",
			"--permission-names", "notes/note1/permissions/perm2",
		}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if payload["deleted"] != true {
		t.Fatal("expected deleted=true")
	}
	if payload["count"].(float64) != 2 {
		t.Fatalf("expected count=2, got %v", payload["count"])
	}
}

func TestKeepPermissions_BatchDelete_ShortPermissionNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v1/notes/note1/permissions:batchDelete") {
			var req keepapi.BatchDeletePermissionsRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			// Short permission names should be expanded to full names
			if len(req.Names) != 1 {
				t.Fatalf("expected 1 name, got %d", len(req.Names))
			}
			if req.Names[0] != "notes/note1/permissions/perm1" {
				t.Fatalf("expected expanded name, got %q", req.Names[0])
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	orig := newKeepServiceWithSA
	t.Cleanup(func() { newKeepServiceWithSA = orig })
	newKeepServiceWithSA = func(ctx context.Context, _, _ string) (*keepapi.Service, error) {
		return keepapi.NewService(ctx,
			option.WithEndpoint(srv.URL+"/"),
			option.WithHTTPClient(srv.Client()),
			option.WithoutAuthentication(),
		)
	}

	stderr := captureStderr(t, func() {
		if err := Execute([]string{
			"keep", "permissions", "batch-delete", "note1",
			"--account", account,
			"--force",
			"--permission-names", "perm1", // short form
		}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})
	if !strings.Contains(stderr, "Removed 1 permission(s)") {
		t.Fatalf("expected remove message, got %q", stderr)
	}
}

func TestKeepPermissions_BatchDelete_RequiresConfirmation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	orig := newKeepServiceWithSA
	t.Cleanup(func() { newKeepServiceWithSA = orig })
	newKeepServiceWithSA = func(ctx context.Context, _, _ string) (*keepapi.Service, error) {
		return keepapi.NewService(ctx,
			option.WithEndpoint(srv.URL+"/"),
			option.WithHTTPClient(srv.Client()),
			option.WithoutAuthentication(),
		)
	}

	if err := Execute([]string{
		"keep", "permissions", "batch-delete", "note1",
		"--account", account,
		"--no-input",
		"--permission-names", "perm1",
	}); err == nil {
		t.Fatal("expected confirmation error")
	}
}

func TestKeepPermissions_BatchDelete_NoPermissionNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	if err := Execute([]string{
		"keep", "permissions", "batch-delete", "note1",
		"--account", account,
		"--force",
	}); err == nil {
		t.Fatal("expected error")
	}
}

func TestKeepPermissions_BatchCreate_TextOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v1/notes/note1/permissions:batchCreate") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"name": "notes/note1/permissions/perm1", "email": "user@example.com", "role": "WRITER"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	orig := newKeepServiceWithSA
	t.Cleanup(func() { newKeepServiceWithSA = orig })
	newKeepServiceWithSA = func(ctx context.Context, _, _ string) (*keepapi.Service, error) {
		return keepapi.NewService(ctx,
			option.WithEndpoint(srv.URL+"/"),
			option.WithHTTPClient(srv.Client()),
			option.WithoutAuthentication(),
		)
	}

	stdout := captureStdout(t, func() {
		if err := Execute([]string{
			"keep", "permissions", "batch-create", "note1",
			"--account", account,
			"--members", "user@example.com",
		}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	if !strings.Contains(stdout, "user@example.com") || !strings.Contains(stdout, "WRITER") {
		t.Fatalf("expected table output with email and role, got %q", stdout)
	}
}
