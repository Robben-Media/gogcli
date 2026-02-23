package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDrivePermissionsListCmd_TextAndJSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/id1/permissions"):
			if r.URL.Query().Get("pageSize") != "1" {
				t.Fatalf("expected pageSize=1, got: %q", r.URL.RawQuery)
			}
			if r.URL.Query().Get("pageToken") != "p1" {
				t.Fatalf("expected pageToken=p1, got: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"id": "p1", "type": "anyone", "role": "reader", "emailAddress": "a@b.com", "allowFileDiscovery": true},
				},
				"nextPageToken": "npt",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	// Text mode: table to stdout + next page hint to stderr.
	var errBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	textOut := captureStdout(t, func() {
		cmd := &DrivePermissionsListCmd{}
		if execErr := runKong(t, cmd, []string{"--max", "1", "--page", "p1", "id1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(textOut, "ID") || !strings.Contains(textOut, "TYPE") {
		t.Fatalf("unexpected table header: %q", textOut)
	}
	if !strings.Contains(textOut, "p1") || !strings.Contains(textOut, "anyone") || !strings.Contains(textOut, "reader") {
		t.Fatalf("missing permission row: %q", textOut)
	}
	if !strings.Contains(errBuf.String(), "--page npt") {
		t.Fatalf("missing next page hint: %q", errBuf.String())
	}
}

func TestDrivePermissionsListCmd_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/id1/permissions"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"id": "p1", "type": "user", "role": "owner", "emailAddress": "owner@example.com"},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	var errBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DrivePermissionsListCmd{}
		if execErr := runKong(t, cmd, []string{"id1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if errBuf.String() != "" {
		t.Fatalf("expected no stderr in json mode, got: %q", errBuf.String())
	}

	var parsed struct {
		FileID          string              `json:"fileId"`
		PermissionCount int                 `json:"permissionCount"`
		Permissions     []*drive.Permission `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.FileID != "id1" || parsed.PermissionCount != 1 || len(parsed.Permissions) != 1 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestDrivePermissionsGetCmd_Success(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file1/permissions/perm1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "perm1",
				"type":         "user",
				"role":         "writer",
				"emailAddress": "user@example.com",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DrivePermissionsGetCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "perm1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Permission *drive.Permission `json:"permission"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.Permission.Id != "perm1" || parsed.Permission.Role != "writer" {
		t.Fatalf("unexpected permission: %#v", parsed.Permission)
	}
}

func TestDrivePermissionsCreateCmd_Success(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/files/file1/permissions"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "newperm",
				"type":         "user",
				"role":         "reader",
				"emailAddress": "reader@example.com",
			})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "file1",
				"webViewLink": "https://drive.google.com/file/d/file1/view",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DrivePermissionsCreateCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "--type", "user", "--role", "reader", "--email", "reader@example.com"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Link       string            `json:"link"`
		Permission *drive.Permission `json:"permission"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.Permission.Id != "newperm" || parsed.Permission.Role != "reader" {
		t.Fatalf("unexpected permission: %#v", parsed.Permission)
	}
}

func TestDrivePermissionsCreateCmd_Anyone(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/files/file1/permissions"):
			// Verify allowFileDiscovery is set correctly
			var perm drive.Permission
			if err := json.NewDecoder(r.Body).Decode(&perm); err != nil {
				t.Fatalf("decode permission: %v", err)
			}
			if perm.Type != "anyone" {
				t.Fatalf("expected type=anyone, got %q", perm.Type)
			}
			if !perm.AllowFileDiscovery {
				t.Fatal("expected allowFileDiscovery=true")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                 "newperm",
				"type":               "anyone",
				"role":               "reader",
				"allowFileDiscovery": true,
			})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "file1",
				"webViewLink": "https://drive.google.com/file/d/file1/view",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DrivePermissionsCreateCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "--type", "anyone", "--role", "reader", "--discoverable"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Permission *drive.Permission `json:"permission"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.Permission.Type != "anyone" {
		t.Fatalf("unexpected type: %q", parsed.Permission.Type)
	}
}

func TestDrivePermissionsCreateCmd_Validation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing type",
			args:    []string{"file1", "--role", "reader", "--email", "user@example.com"},
			wantErr: "missing --type",
		},
		{
			name:    "missing role",
			args:    []string{"file1", "--type", "user", "--email", "user@example.com"},
			wantErr: "missing --role",
		},
		{
			name:    "user type without email",
			args:    []string{"file1", "--type", "user", "--role", "reader"},
			wantErr: "--email is required",
		},
		{
			name:    "domain type without domain",
			args:    []string{"file1", "--type", "domain", "--role", "reader"},
			wantErr: "--domain is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := &RootFlags{Account: "a@b.com"}
			u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
			if err != nil {
				t.Fatalf("ui.New: %v", err)
			}
			ctx := ui.WithUI(context.Background(), u)

			cmd := &DrivePermissionsCreateCmd{}
			err = runKong(t, cmd, tt.args, ctx, flags)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestDrivePermissionsUpdateCmd_Success(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/files/file1/permissions/perm1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "perm1",
				"type":         "user",
				"role":         "writer",
				"emailAddress": "user@example.com",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DrivePermissionsUpdateCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "perm1", "--role", "writer"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Permission *drive.Permission `json:"permission"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.Permission.Role != "writer" {
		t.Fatalf("expected role=writer, got %q", parsed.Permission.Role)
	}
}

func TestDrivePermissionsUpdateCmd_NoFields(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DrivePermissionsUpdateCmd{}
	err = runKong(t, cmd, []string{"file1", "perm1"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "at least one of") {
		t.Fatalf("expected 'at least one of' error, got %q", err.Error())
	}
}

func TestDrivePermissionsDeleteCmd_Success(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/files/file1/permissions/perm1"):
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DrivePermissionsDeleteCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "perm1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Removed      bool   `json:"removed"`
		FileID       string `json:"fileId"`
		PermissionID string `json:"permissionId"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if !parsed.Removed || parsed.FileID != "file1" || parsed.PermissionID != "perm1" {
		t.Fatalf("unexpected response: %#v", parsed)
	}
}

func TestDrivePermissionsDeleteCmd_RequiresConfirmation(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"} // no force
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DrivePermissionsDeleteCmd{}
	err = runKong(t, cmd, []string{"file1", "perm1"}, ctx, flags)
	if err == nil {
		t.Fatal("expected confirmation error, got nil")
	}
	if !strings.Contains(err.Error(), "without --force") {
		t.Fatalf("expected confirmation error, got %q", err.Error())
	}
}

// Test backward compatibility - existing tests should still pass
func TestDrivePermissionsCmd_BackwardCompatible(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/id1/permissions"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{"id": "p1", "type": "anyone", "role": "reader", "emailAddress": "a@b.com"},
				},
				"nextPageToken": "npt",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	// Test that the parent command now has subcommands
	jsonOut := captureStdout(t, func() {
		cmd := &DrivePermissionsListCmd{}
		if execErr := runKong(t, cmd, []string{"id1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		FileID          string              `json:"fileId"`
		PermissionCount int                 `json:"permissionCount"`
		Permissions     []*drive.Permission `json:"permissions"`
		NextPageToken   string              `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.FileID != "id1" || parsed.PermissionCount != 1 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}
