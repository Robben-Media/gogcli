package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	keepapi "google.golang.org/api/keep/v1"
	"google.golang.org/api/option"
)

func TestKeepPermissionsBatchCreate_Plain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes/abc123/permissions:batchCreate":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			var req keepapi.BatchCreatePermissionsRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(req.Requests) != 2 {
				t.Fatalf("expected 2 requests, got %d", len(req.Requests))
			}
			if req.Requests[0].Permission.Email != "user1@example.com" {
				t.Fatalf("expected first email 'user1@example.com', got %q", req.Requests[0].Permission.Email)
			}
			if req.Requests[1].Permission.Email != "user2@example.com" {
				t.Fatalf("expected second email 'user2@example.com', got %q", req.Requests[1].Permission.Email)
			}

			resp := keepapi.BatchCreatePermissionsResponse{
				Permissions: []*keepapi.Permission{
					{Name: "notes/abc123/permissions/perm1", Email: "user1@example.com", Role: "WRITER"},
					{Name: "notes/abc123/permissions/perm2", Email: "user2@example.com", Role: "WRITER"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&resp)
			return
		default:
			http.NotFound(w, r)
		}
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "--account", account, "keep", "permissions", "batch-create", "notes/abc123", "--members", "user1@example.com", "--members", "user2@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "user1@example.com") {
		t.Fatalf("expected user1 in output, got: %q", out)
	}
	if !strings.Contains(out, "user2@example.com") {
		t.Fatalf("expected user2 in output, got: %q", out)
	}
	if !strings.Contains(out, "WRITER") {
		t.Fatalf("expected WRITER in output, got: %q", out)
	}
}

func TestKeepPermissionsBatchCreate_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes/abc123/permissions:batchCreate":
			resp := keepapi.BatchCreatePermissionsResponse{
				Permissions: []*keepapi.Permission{
					{Name: "notes/abc123/permissions/perm1", Email: "user@example.com", Role: "WRITER"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&resp)
			return
		default:
			http.NotFound(w, r)
		}
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "keep", "permissions", "batch-create", "abc123", "--members", "user@example.com", "--account", account}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var payload struct {
		Permissions []map[string]any `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(payload.Permissions) != 1 {
		t.Fatalf("expected 1 permission, got %d", len(payload.Permissions))
	}
	if payload.Permissions[0]["email"] != "user@example.com" {
		t.Fatalf("unexpected email: %v", payload.Permissions[0]["email"])
	}
}

func TestKeepPermissionsBatchCreate_NoMembers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	err := Execute([]string{"--plain", "--account", account, "keep", "permissions", "batch-create", "notes/abc"})
	if err == nil {
		t.Fatalf("expected error for no members")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestKeepPermissionsBatchCreate_InvalidRole(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	err := Execute([]string{"--plain", "--account", account, "keep", "permissions", "batch-create", "notes/abc", "--members", "user@example.com", "--role", "READER"})
	if err == nil {
		t.Fatalf("expected error for invalid role")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestKeepPermissionsBatchCreate_AutoNotePrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes/abc123/permissions:batchCreate":
			resp := keepapi.BatchCreatePermissionsResponse{
				Permissions: []*keepapi.Permission{
					{Name: "notes/abc123/permissions/perm1", Email: "user@example.com", Role: "WRITER"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&resp)
			return
		default:
			http.NotFound(w, r)
		}
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

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			// Pass "abc123" without "notes/" prefix
			if err := Execute([]string{"--plain", "--account", account, "keep", "permissions", "batch-create", "abc123", "--members", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestKeepPermissionsBatchDelete_Plain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes/abc123/permissions:batchDelete":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			var req keepapi.BatchDeletePermissionsRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(req.Names) != 2 {
				t.Fatalf("expected 2 names, got %d", len(req.Names))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
			return
		default:
			http.NotFound(w, r)
		}
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
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--force", "--plain", "--account", account, "keep", "permissions", "batch-delete", "notes/abc123", "--permission-names", "notes/abc123/permissions/perm1", "--permission-names", "notes/abc123/permissions/perm2"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "Removed 2 permission(s)") {
		t.Fatalf("expected remove message, got: %q", stderr)
	}
}

func TestKeepPermissionsBatchDelete_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes/abc123/permissions:batchDelete":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
			return
		default:
			http.NotFound(w, r)
		}
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--force", "--account", account, "keep", "permissions", "batch-delete", "abc123", "--permission-names", "perm1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if payload["deleted"] != true {
		t.Fatalf("expected deleted=true, got: %v", payload)
	}
	if payload["count"].(float64) != 1 {
		t.Fatalf("expected count=1, got: %v", payload["count"])
	}
}

func TestKeepPermissionsBatchDelete_NoPermissionNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	err := Execute([]string{"--plain", "--account", account, "keep", "permissions", "batch-delete", "notes/abc"})
	if err == nil {
		t.Fatalf("expected error for no permission names")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestKeepPermissionsBatchDelete_RequiresConfirmation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not be called
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
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

	err := Execute([]string{"--no-input", "--plain", "--account", account, "keep", "permissions", "batch-delete", "abc123", "--permission-names", "perm1"})
	if err == nil {
		t.Fatalf("expected error for confirmation required")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestKeepPermissionsBatchDelete_ShortPermissionNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes/abc123/permissions:batchDelete":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			var req keepapi.BatchDeletePermissionsRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			// Verify short permission names were expanded
			expectedName := "notes/abc123/permissions/perm1"
			if len(req.Names) != 1 || req.Names[0] != expectedName {
				t.Fatalf("expected %q, got %v", expectedName, req.Names)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
			return
		default:
			http.NotFound(w, r)
		}
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

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			// Pass short permission name "perm1" instead of full "notes/abc123/permissions/perm1"
			if err := Execute([]string{"--force", "--plain", "--account", account, "keep", "permissions", "batch-delete", "abc123", "--permission-names", "perm1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

// Test integration via Execute - verify commands are registered
func TestKeepNotesCreate_CommandRegistered(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	// Just verify the command is registered by checking help
	output := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			_ = Execute([]string{"keep", "--help"})
		})
	})

	if !strings.Contains(output, "notes") {
		t.Fatalf("expected 'notes' in keep help output, got: %q", output)
	}
}

func TestKeepPermissions_CommandRegistered(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	// Just verify the command is registered by checking help
	output := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			_ = Execute([]string{"keep", "--help"})
		})
	})

	if !strings.Contains(output, "permissions") {
		t.Fatalf("expected 'permissions' in keep help output, got: %q", output)
	}
}
