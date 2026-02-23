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

func TestDriveChangesGetStartPageTokenCmd_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodGet && path == "/changes/startPageToken":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"startPageToken": "test-page-abc",
				"kind":           "drive#startPageToken",
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
		cmd := &DriveChangesGetStartPageTokCmd{}
		if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		StartPageToken string `json:"startPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if parsed.StartPageToken != "test-page-abc" {
		t.Fatalf("unexpected startPageToken: %q", parsed.StartPageToken)
	}
}

func TestDriveChangesListCmd(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodGet && path == "/changes":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"changes": []map[string]any{
					{
						"type":    "file",
						"fileId":  "file123",
						"removed": false,
						"time":    "2024-01-01T12:00:00Z",
						"file": map[string]any{
							"id":           "file123",
							"name":         "Test File",
							"mimeType":     "text/plain",
							"modifiedTime": "2024-01-01T12:00:00Z",
						},
					},
					{
						"type":    "file",
						"fileId":  "file456",
						"removed": true,
						"time":    "2024-01-02T14:00:00Z",
					},
				},
				"newStartPageToken": "new-page-xyz",
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
		cmd := &DriveChangesListCmd{}
		if execErr := runKong(t, cmd, []string{"test-page-abc"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(textOut, "file123") || !strings.Contains(textOut, "Test File") {
		t.Fatalf("unexpected output: %q", textOut)
	}
}

func TestDriveChangesWatchCmd_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodPost && path == "/changes/watch":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "channel123",
				"resourceId": "resource123",
				"address":    "https://example.com/webhook",
				"expiration": "1704067200000", // String for int64
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
		cmd := &DriveChangesWatchCmd{}
		if execErr := runKong(t, cmd, []string{"test-page-abc", "--address", "https://example.com/webhook"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Channel  *drive.Channel `json:"channel"`
		Resource string         `json:"resource"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if parsed.Channel == nil || parsed.Channel.Id != "channel123" {
		t.Fatalf("unexpected channel: %#v", parsed.Channel)
	}
}

func TestDriveChangesValidation(t *testing.T) {
	tests := []struct {
		name    string
		cmd     any
		args    []string
		wantErr string
	}{
		{
			name:    "watch missing address",
			cmd:     &DriveChangesWatchCmd{},
			args:    []string{"test-page-abc"},
			wantErr: "--address is required",
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
