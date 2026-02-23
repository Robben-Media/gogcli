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

func TestDriveRepliesListCmd_TextAndJSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodGet && path == "/files/file1/comments/comment1/replies":
			if r.URL.Query().Get("pageSize") != "10" {
				t.Fatalf("expected pageSize=10, got: %q", r.URL.RawQuery)
			}
			if r.URL.Query().Get("pageToken") != "p1" {
				t.Fatalf("expected pageToken=p1, got: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"replies": []map[string]any{
					{
						"id":          "r1",
						"author":      map[string]any{"displayName": "Bob"},
						"content":     "Reply content",
						"createdTime": "2025-01-01T00:00:00Z",
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

	var errBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	textOut := captureStdout(t, func() {
		cmd := &DriveRepliesListCmd{}
		if execErr := runKong(t, cmd, []string{"--max", "10", "--page", "p1", "file1", "comment1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(textOut, "Bob") || !strings.Contains(textOut, "Reply content") {
		t.Fatalf("unexpected output: %q", textOut)
	}
	if !strings.Contains(errBuf.String(), "--page npt") {
		t.Fatalf("missing next page hint: %q", errBuf.String())
	}

	// Test JSON output
	var errBuf2 bytes.Buffer
	u2, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf2, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx2 := ui.WithUI(context.Background(), u2)
	ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveRepliesListCmd{}
		if execErr := runKong(t, cmd, []string{"--max", "10", "--page", "p1", "file1", "comment1"}, ctx2, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		FileID        string         `json:"fileId"`
		CommentID     string         `json:"commentId"`
		Replies       []*drive.Reply `json:"replies"`
		NextPageToken string         `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.FileID != "file1" || parsed.CommentID != "comment1" || parsed.NextPageToken != "npt" || len(parsed.Replies) != 1 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
	if parsed.Replies[0].Content != "Reply content" {
		t.Fatalf("unexpected reply content: %s", parsed.Replies[0].Content)
	}
}

func TestDriveRepliesGetCmd_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodGet && path == "/files/file1/comments/comment1/replies/reply1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "reply1",
				"author":       map[string]any{"displayName": "Bob"},
				"content":      "Reply text",
				"createdTime":  "2025-01-01T00:00:00Z",
				"modifiedTime": "2025-01-02T00:00:00Z",
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
		cmd := &DriveRepliesGetCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "comment1", "reply1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Reply *drive.Reply `json:"reply"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if parsed.Reply == nil || parsed.Reply.Id != "reply1" || parsed.Reply.Content != "Reply text" {
		t.Fatalf("unexpected reply: %#v", parsed.Reply)
	}
}

func TestDriveRepliesCreateCmd_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodPost && path == "/files/file1/comments/comment1/replies":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if payload["content"] != "New reply" {
				t.Fatalf("expected content 'New reply', got: %#v", payload["content"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "r1",
				"author":      map[string]any{"displayName": "Alice"},
				"content":     "New reply",
				"createdTime": "2025-01-01T00:00:00Z",
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
		cmd := &DriveRepliesCreateCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "comment1", "New reply"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Reply *drive.Reply `json:"reply"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if parsed.Reply == nil || parsed.Reply.Id != "r1" || parsed.Reply.Content != "New reply" {
		t.Fatalf("unexpected reply: %#v", parsed.Reply)
	}
}

func TestDriveRepliesUpdateCmd_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodPatch && path == "/files/file1/comments/comment1/replies/reply1":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if payload["content"] != "Updated reply" {
				t.Fatalf("expected content 'Updated reply', got: %#v", payload["content"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "reply1",
				"author":       map[string]any{"displayName": "Alice"},
				"content":      "Updated reply",
				"modifiedTime": "2025-01-02T00:00:00Z",
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
		cmd := &DriveRepliesUpdateCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "comment1", "reply1", "Updated reply"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Reply *drive.Reply `json:"reply"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if parsed.Reply == nil || parsed.Reply.Id != "reply1" || parsed.Reply.Content != "Updated reply" {
		t.Fatalf("unexpected reply: %#v", parsed.Reply)
	}
}

func TestDriveRepliesDeleteCmd_WithConfirm(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodDelete && path == "/files/file1/comments/comment1/replies/reply1":
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

	flags := &RootFlags{Account: "a@b.com", Force: true} // --force to skip confirmation
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx = ui.WithUI(ctx, u)

	out := captureStdout(t, func() {
		cmd := &DriveRepliesDeleteCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "comment1", "reply1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Deleted   bool   `json:"deleted"`
		FileID    string `json:"fileId"`
		CommentID string `json:"commentId"`
		ReplyID   string `json:"replyId"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if !parsed.Deleted || parsed.FileID != "file1" || parsed.CommentID != "comment1" || parsed.ReplyID != "reply1" {
		t.Fatalf("unexpected delete result: %#v", parsed)
	}
}

func TestDriveRepliesListCmd_Empty(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodGet && path == "/files/file1/comments/comment1/replies":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"replies": []map[string]any{},
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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	_ = captureStdout(t, func() {
		cmd := &DriveRepliesListCmd{}
		if execErr := runKong(t, cmd, []string{"file1", "comment1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	if !strings.Contains(errBuf.String(), "No replies") {
		t.Fatalf("expected 'No replies' message, got: %q", errBuf.String())
	}
}

func TestDriveRepliesValidation(t *testing.T) {
	tests := []struct {
		name    string
		cmd     any
		args    []string
		wantErr string
	}{
		{
			name:    "list missing commentId",
			cmd:     &DriveRepliesListCmd{},
			args:    []string{"file1", ""},
			wantErr: "empty commentId",
		},
		{
			name:    "list missing fileId",
			cmd:     &DriveRepliesListCmd{},
			args:    []string{"", "comment1"},
			wantErr: "empty fileId",
		},
		{
			name:    "get missing replyId",
			cmd:     &DriveRepliesGetCmd{},
			args:    []string{"file1", "comment1", ""},
			wantErr: "empty replyId",
		},
		{
			name:    "create empty content",
			cmd:     &DriveRepliesCreateCmd{},
			args:    []string{"file1", "comment1", "  "},
			wantErr: "empty content",
		},
		{
			name:    "update empty content",
			cmd:     &DriveRepliesUpdateCmd{},
			args:    []string{"file1", "comment1", "reply1", ""},
			wantErr: "empty content",
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
