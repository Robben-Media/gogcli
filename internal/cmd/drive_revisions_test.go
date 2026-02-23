package cmd

import (
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

func TestDriveRevisionsListCmd_TextAndJSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodGet && path == "/files/file1/revisions":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"revisions": []map[string]any{
					{
						"id":           "rev1",
						"modifiedTime": "2024-01-01T12:00:00Z",
						"size":         "1024",
						"keepForever":  false,
						"published":    false,
					},
					{
						"id":           "rev2",
						"modifiedTime": "2024-01-02T14:30:00Z",
						"size":         "2048",
						"keepForever":  true,
						"published":    true,
					},
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	textOut := captureStdout(t, func() {
		cmd := &DriveRevisionsListCmd{}
		if execErr := runKong(t, cmd, []string{"file1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(textOut, "rev1") || !strings.Contains(textOut, "rev2") {
		t.Fatalf("unexpected output: %q", textOut)
	}

	// Test JSON output
	u2, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx2 := ui.WithUI(context.Background(), u2)
	ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveRevisionsListCmd{}
		if execErr := runKong(t, cmd, []string{"file1"}, ctx2, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(jsonOut, "revisions") || !strings.Contains(jsonOut, "file1") {
		t.Fatalf("unexpected JSON output: %q", jsonOut)
	}
}

func TestDriveRevisionsGetCmd_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodGet && path == "/files/file1/revisions/rev1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "rev1",
				"modifiedTime": "2024-01-01T12:00:00Z",
				"size":         "1024",
				"mimeType":     "text/plain",
				"keepForever":  true,
				"published":    true,
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
		cmd := &DriveRevisionsGetCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "rev1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(jsonOut, "rev1") || !strings.Contains(jsonOut, "keepForever") {
		t.Fatalf("unexpected JSON output: %q", jsonOut)
	}
}

func TestDriveRevisionsDeleteCmd_WithForce(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodDelete && path == "/files/file1/revisions/rev1":
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
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx = ui.WithUI(ctx, u)

	out := captureStdout(t, func() {
		cmd := &DriveRevisionsDeleteCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "rev1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Deleted    bool   `json:"deleted"`
		FileID     string `json:"fileId"`
		RevisionID string `json:"revisionId"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if !parsed.Deleted || parsed.FileID != "file1" || parsed.RevisionID != "rev1" {
		t.Fatalf("unexpected delete result: %#v", parsed)
	}
}

func TestDriveRevisionsUpdateCmd_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodPatch && path == "/files/file1/revisions/rev1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "rev1",
				"modifiedTime": "2024-01-01T12:00:00Z",
				"keepForever":  true,
				"published":    true,
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
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx = ui.WithUI(ctx, u)

	out := captureStdout(t, func() {
		cmd := &DriveRevisionsUpdateCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "rev1", "--keep-forever"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Revision *drive.Revision `json:"revision"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if parsed.Revision == nil || !parsed.Revision.KeepForever {
		t.Fatalf("unexpected revision: %#v", parsed.Revision)
	}
}

func TestDriveRevisionsValidation(t *testing.T) {
	tests := []struct {
		name    string
		cmd     any
		args    []string
		wantErr string
	}{
		{
			name:    "list missing fileId",
			cmd:     &DriveRevisionsListCmd{},
			args:    []string{""},
			wantErr: "empty fileId",
		},
		{
			name:    "get missing fileId",
			cmd:     &DriveRevisionsGetCmd{},
			args:    []string{"", "rev1"},
			wantErr: "empty fileId",
		},
		{
			name:    "get missing revisionId",
			cmd:     &DriveRevisionsGetCmd{},
			args:    []string{"file1", ""},
			wantErr: "empty revisionId",
		},
		{
			name:    "delete missing fileId",
			cmd:     &DriveRevisionsDeleteCmd{},
			args:    []string{"", "rev1"},
			wantErr: "empty fileId",
		},
		{
			name:    "update no flags",
			cmd:     &DriveRevisionsUpdateCmd{},
			args:    []string{"file1", "rev1"},
			wantErr: "at least one of --keep-forever or --publish",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origNew := newDriveService
			t.Cleanup(func() { newDriveService = origNew })

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
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
			ctx = outfmt.WithMode(ctx, outfmt.Mode{})

			err = runKong(t, tt.cmd, tt.args, ctx, flags)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
