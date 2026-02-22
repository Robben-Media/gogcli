package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDocsCreateCmd_JSON(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/v1/documents" && r.Method == http.MethodPost:
			// Verify request body contains title
			var req docs.Document
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.Title != "Test Document" {
				t.Fatalf("expected title 'Test Document', got %q", req.Title)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "newdoc123",
				"title":      "Test Document",
				"revisionId": "rev1",
				"body":       map[string]any{"content": []any{}},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) { return nil, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		cmd := &DocsCreateCmd{Title: "Test Document"}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	file, ok := payload["file"].(map[string]any)
	if !ok {
		t.Fatal("expected file object in output")
	}
	if file["id"] != "newdoc123" {
		t.Fatalf("expected id 'newdoc123', got %v", file["id"])
	}
	if file["name"] != "Test Document" {
		t.Fatalf("expected name 'Test Document', got %v", file["name"])
	}
	doc, ok := payload["document"].(map[string]any)
	if !ok {
		t.Fatal("expected document object in output")
	}
	if doc["documentId"] != "newdoc123" {
		t.Fatalf("expected documentId 'newdoc123', got %v", doc["documentId"])
	}
	if doc["revisionId"] != "rev1" {
		t.Fatalf("expected revisionId 'rev1', got %v", doc["revisionId"])
	}
}

func TestDocsCreateCmd_WithParent(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

	driveUpdateCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/v1/documents" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "newdoc456",
				"title":      "Doc in Folder",
				"revisionId": "rev2",
				"body":       map[string]any{"content": []any{}},
			})
			return
		case strings.Contains(path, "newdoc456") && r.Method == http.MethodPatch:
			// Drive API files.update to move to parent folder
			if r.URL.Query().Get("addParents") != "folder123" {
				t.Fatalf("expected addParents=folder123, got %q", r.URL.Query().Get("addParents"))
			}
			driveUpdateCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "newdoc456"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/drive/v3/"),
	)
	if err != nil {
		t.Fatalf("NewDriveService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return driveSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	_ = captureStdout(t, func() {
		cmd := &DocsCreateCmd{Title: "Doc in Folder", Parent: "folder123"}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !driveUpdateCalled {
		t.Fatal("expected Drive API files.update to be called for parent folder")
	}
}

func TestDocsCreateCmd_EmptyTitle(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

	newDocsService = func(context.Context, string) (*docs.Service, error) { return nil, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) { return nil, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsCreateCmd{Title: "   "}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty title")
	}
	if !strings.Contains(err.Error(), "empty title") {
		t.Fatalf("expected 'empty title' error, got: %v", err)
	}
}

func TestDocsCreateCmd_Text(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/v1/documents" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "textdoc789",
				"title":      "Text Output Doc",
				"revisionId": "rev3",
				"body":       map[string]any{"content": []any{}},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) { return nil, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		// Text mode (not JSON - don't set outfmt.Mode)

		cmd := &DocsCreateCmd{Title: "Text Output Doc"}
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "id\ttextdoc789") {
		t.Fatalf("expected id line in output, got: %q", out)
	}
	if !strings.Contains(out, "name\tText Output Doc") {
		t.Fatalf("expected name line in output, got: %q", out)
	}
	if !strings.Contains(out, "revision\trev3") {
		t.Fatalf("expected revision line in output, got: %q", out)
	}
}
