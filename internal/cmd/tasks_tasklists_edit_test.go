package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestTasksLists_Get_JSON(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/tasks/v1/users/@me/lists/tl1") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "tl1",
				"title":   "Test List",
				"updated": "2025-01-15T10:00:00.000Z",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := tasks.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newTasksService = func(context.Context, string) (*tasks.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &TasksListsGetCmd{}, []string{"tl1"}, ctx, flags); err != nil {
			t.Fatalf("get: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	tl, ok := payload["tasklist"].(map[string]any)
	if !ok {
		t.Fatalf("expected tasklist in response")
	}
	if tl["id"] != "tl1" {
		t.Fatalf("expected id tl1, got %v", tl["id"])
	}
	if tl["title"] != "Test List" {
		t.Fatalf("expected title 'Test List', got %v", tl["title"])
	}
}

func TestTasksLists_Delete_JSON(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/tasks/v1/users/@me/lists/tl1") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := tasks.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newTasksService = func(context.Context, string) (*tasks.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &TasksListsDeleteCmd{}, []string{"tl1"}, ctx, flags); err != nil {
			t.Fatalf("delete: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if payload["deleted"] != true {
		t.Fatalf("expected deleted=true")
	}
	if payload["tasklistId"] != "tl1" {
		t.Fatalf("expected tasklistId=tl1, got %v", payload["tasklistId"])
	}
}

func TestTasksLists_Patch_JSON(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/tasks/v1/users/@me/lists/tl1") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "tl1",
				"title":   "Patched Title",
				"updated": "2025-01-15T11:00:00.000Z",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := tasks.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newTasksService = func(context.Context, string) (*tasks.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &TasksListsPatchCmd{}, []string{"tl1", "--title", "Patched Title"}, ctx, flags); err != nil {
			t.Fatalf("patch: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	tl, ok := payload["tasklist"].(map[string]any)
	if !ok {
		t.Fatalf("expected tasklist in response")
	}
	if tl["title"] != "Patched Title" {
		t.Fatalf("expected title 'Patched Title', got %v", tl["title"])
	}
}

func TestTasksLists_Patch_NoFields(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := tasks.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newTasksService = func(context.Context, string) (*tasks.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &TasksListsPatchCmd{}, []string{"tl1"}, ctx, flags); err == nil || !strings.Contains(err.Error(), "no updates") {
		t.Fatalf("expected no updates error, got %v", err)
	}
}

func TestTasksLists_Update_JSON(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/tasks/v1/users/@me/lists/tl1") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "tl1",
				"title":   "Updated Title",
				"updated": "2025-01-15T12:00:00.000Z",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := tasks.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newTasksService = func(context.Context, string) (*tasks.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &TasksListsUpdateCmd{}, []string{"tl1", "--title", "Updated Title"}, ctx, flags); err != nil {
			t.Fatalf("update: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	tl, ok := payload["tasklist"].(map[string]any)
	if !ok {
		t.Fatalf("expected tasklist in response")
	}
	if tl["title"] != "Updated Title" {
		t.Fatalf("expected title 'Updated Title', got %v", tl["title"])
	}
}

func TestTasksLists_TextPaths(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/tasks/v1/users/@me/lists/tl1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "tl1",
				"title":   "Test List",
				"updated": "2025-01-15T10:00:00.000Z",
			})
			return
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/tasks/v1/users/@me/lists/tl1"):
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/tasks/v1/users/@me/lists/tl2"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "tl2",
				"title":   "Patched",
				"updated": "2025-01-15T11:00:00.000Z",
			})
			return
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/tasks/v1/users/@me/lists/tl3"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "tl3",
				"title":   "Updated",
				"updated": "2025-01-15T12:00:00.000Z",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := tasks.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newTasksService = func(context.Context, string) (*tasks.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &TasksListsGetCmd{}, []string{"tl1"}, ctx, flags); err != nil {
		t.Fatalf("get: %v", err)
	}

	if err := runKong(t, &TasksListsDeleteCmd{}, []string{"tl1"}, ctx, flags); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if err := runKong(t, &TasksListsPatchCmd{}, []string{"tl2", "--title", "Patched"}, ctx, flags); err != nil {
		t.Fatalf("patch: %v", err)
	}

	if err := runKong(t, &TasksListsUpdateCmd{}, []string{"tl3", "--title", "Updated"}, ctx, flags); err != nil {
		t.Fatalf("update: %v", err)
	}
}

func TestTasksLists_Validation(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := tasks.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newTasksService = func(context.Context, string) (*tasks.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	// Get with empty ID (direct call, bypassing kong)
	cmd := &TasksListsGetCmd{TasklistID: ""}
	if err := cmd.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty tasklistId") {
		t.Fatalf("expected empty tasklistId error, got %v", err)
	}

	// Delete with empty ID (direct call, bypassing kong)
	cmdDel := &TasksListsDeleteCmd{TasklistID: ""}
	if err := cmdDel.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty tasklistId") {
		t.Fatalf("expected empty tasklistId error, got %v", err)
	}

	// Patch with whitespace ID (gets trimmed to empty) - using kong to test flagProvided
	cmdPatch := &TasksListsPatchCmd{}
	kctx := parseTasksKong(t, cmdPatch, []string{"   ", "--title", "test"})
	if err := cmdPatch.Run(ctx, kctx, flags); err == nil || !strings.Contains(err.Error(), "empty tasklistId") {
		t.Fatalf("expected empty tasklistId error, got %v", err)
	}

	// Update with whitespace ID (gets trimmed to empty)
	cmdUpdate := &TasksListsUpdateCmd{TasklistID: "   ", Title: "test"}
	if err := cmdUpdate.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty tasklistId") {
		t.Fatalf("expected empty tasklistId error, got %v", err)
	}

	// Update with empty title
	cmdUpdate2 := &TasksListsUpdateCmd{TasklistID: "tl1", Title: ""}
	if err := cmdUpdate2.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty title") {
		t.Fatalf("expected empty title error, got %v", err)
	}
}
