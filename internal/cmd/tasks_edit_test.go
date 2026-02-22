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

func TestTasksMove_JSON(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Move is a POST to /tasks/v1/lists/{tasklistId}/tasks/{taskId}/move
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/tasks/v1/lists/tl1/tasks/t1/move") {
			parent := r.URL.Query().Get("parent")
			previous := r.URL.Query().Get("previous")

			// Verify query params
			if parent != "parent123" {
				t.Errorf("expected parent=parent123, got %s", parent)
			}
			if previous != "prev456" {
				t.Errorf("expected previous=prev456, got %s", previous)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "t1",
				"title":    "Moved Task",
				"parent":   parent,
				"position": "00000000000000000001",
				"status":   "needsAction",
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
		if err := runKong(t, &TasksMoveCmd{}, []string{"tl1", "t1", "--parent", "parent123", "--previous", "prev456"}, ctx, flags); err != nil {
			t.Fatalf("move: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	task, ok := payload["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected task in response")
	}
	if task["id"] != "t1" {
		t.Fatalf("expected id t1, got %v", task["id"])
	}
	if task["parent"] != "parent123" {
		t.Fatalf("expected parent parent123, got %v", task["parent"])
	}
}

func TestTasksMove_NoOptions(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Move without parent/previous should still work (moves to top of list)
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/tasks/v1/lists/tl1/tasks/t1/move") {
			parent := r.URL.Query().Get("parent")
			previous := r.URL.Query().Get("previous")

			// Both should be empty
			if parent != "" {
				t.Errorf("expected empty parent, got %s", parent)
			}
			if previous != "" {
				t.Errorf("expected empty previous, got %s", previous)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "t1",
				"title":    "Moved Task",
				"parent":   "",
				"position": "00000000000000000000",
				"status":   "needsAction",
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
		if err := runKong(t, &TasksMoveCmd{}, []string{"tl1", "t1"}, ctx, flags); err != nil {
			t.Fatalf("move: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	task, ok := payload["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected task in response")
	}
	if task["id"] != "t1" {
		t.Fatalf("expected id t1, got %v", task["id"])
	}
}

func TestTasksMove_Text(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/tasks/v1/lists/tl1/tasks/t1/move") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "t1",
				"title":    "Moved Task",
				"parent":   "p1",
				"position": "00000000000000000001",
				"status":   "needsAction",
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

	if err := runKong(t, &TasksMoveCmd{}, []string{"tl1", "t1", "--parent", "p1"}, ctx, flags); err != nil {
		t.Fatalf("move: %v", err)
	}
}

func TestTasksMove_Validation(t *testing.T) {
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

	// Empty tasklistId
	cmd := &TasksMoveCmd{TasklistID: "", TaskID: "t1"}
	if err := cmd.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty tasklistId") {
		t.Fatalf("expected empty tasklistId error, got %v", err)
	}

	// Empty taskId
	cmd2 := &TasksMoveCmd{TasklistID: "tl1", TaskID: ""}
	if err := cmd2.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty taskId") {
		t.Fatalf("expected empty taskId error, got %v", err)
	}

	// Whitespace gets trimmed to empty
	cmd3 := &TasksMoveCmd{TasklistID: "   ", TaskID: "t1"}
	if err := cmd3.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty tasklistId") {
		t.Fatalf("expected empty tasklistId error, got %v", err)
	}
}

func TestTasksReplace_JSON(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Update is PUT to /tasks/v1/lists/{tasklistId}/tasks/{taskId}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/tasks/v1/lists/tl1/tasks/t1") {
			// Read the request body to verify all fields are sent
			body, _ := io.ReadAll(r.Body)
			var task map[string]any
			if err := json.Unmarshal(body, &task); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}

			// Verify required field
			if task["title"] != "Replaced Task" {
				t.Errorf("expected title 'Replaced Task', got %v", task["title"])
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "t1",
				"title":   "Replaced Task",
				"notes":   "New notes",
				"status":  "completed",
				"due":     "2025-01-20T00:00:00.000Z",
				"updated": "2025-01-15T14:00:00.000Z",
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
		if err := runKong(t, &TasksReplaceCmd{}, []string{"tl1", "t1", "--title", "Replaced Task", "--notes", "New notes", "--status", "completed", "--due", "2025-01-20"}, ctx, flags); err != nil {
			t.Fatalf("replace: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	task, ok := payload["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected task in response")
	}
	if task["title"] != "Replaced Task" {
		t.Fatalf("expected title 'Replaced Task', got %v", task["title"])
	}
	if task["status"] != "completed" {
		t.Fatalf("expected status completed, got %v", task["status"])
	}
}

func TestTasksReplace_RequiredTitle(t *testing.T) {
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

	// Without --title, kong should fail due to required:""
	// But we test the direct call too
	cmd := &TasksReplaceCmd{TasklistID: "tl1", TaskID: "t1", Title: ""}
	if err := cmd.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty title") {
		t.Fatalf("expected empty title error, got %v", err)
	}
}

func TestTasksReplace_InvalidStatus(t *testing.T) {
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

	cmd := &TasksReplaceCmd{TasklistID: "tl1", TaskID: "t1", Title: "Test", Status: "invalid"}
	if err := cmd.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "invalid --status") {
		t.Fatalf("expected invalid status error, got %v", err)
	}
}

func TestTasksReplace_Text(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/tasks/v1/lists/tl1/tasks/t1") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "t1",
				"title":   "Replaced Task",
				"status":  "needsAction",
				"updated": "2025-01-15T14:00:00.000Z",
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

	if err := runKong(t, &TasksReplaceCmd{}, []string{"tl1", "t1", "--title", "Replaced Task"}, ctx, flags); err != nil {
		t.Fatalf("replace: %v", err)
	}
}

func TestTasksReplace_Validation(t *testing.T) {
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

	// Empty tasklistId
	cmd := &TasksReplaceCmd{TasklistID: "", TaskID: "t1", Title: "Test"}
	if err := cmd.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty tasklistId") {
		t.Fatalf("expected empty tasklistId error, got %v", err)
	}

	// Empty taskId
	cmd2 := &TasksReplaceCmd{TasklistID: "tl1", TaskID: "", Title: "Test"}
	if err := cmd2.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty taskId") {
		t.Fatalf("expected empty taskId error, got %v", err)
	}

	// Whitespace gets trimmed to empty
	cmd3 := &TasksReplaceCmd{TasklistID: "   ", TaskID: "t1", Title: "Test"}
	if err := cmd3.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty tasklistId") {
		t.Fatalf("expected empty tasklistId error, got %v", err)
	}

	cmd4 := &TasksReplaceCmd{TasklistID: "tl1", TaskID: "   ", Title: "Test"}
	if err := cmd4.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "empty taskId") {
		t.Fatalf("expected empty taskId error, got %v", err)
	}
}

func TestTasksReplace_ValidStatuses(t *testing.T) {
	origNew := newTasksService
	t.Cleanup(func() { newTasksService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "t1",
				"title":   "Task",
				"status":  "completed",
				"updated": "2025-01-15T14:00:00.000Z",
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

	// needsAction should be valid
	cmd1 := &TasksReplaceCmd{TasklistID: "tl1", TaskID: "t1", Title: "Task", Status: "needsAction"}
	if err := cmd1.Run(ctx, flags); err != nil {
		t.Fatalf("needsAction should be valid: %v", err)
	}

	// completed should be valid
	cmd2 := &TasksReplaceCmd{TasklistID: "tl1", TaskID: "t1", Title: "Task", Status: "completed"}
	if err := cmd2.Run(ctx, flags); err != nil {
		t.Fatalf("completed should be valid: %v", err)
	}

	// empty status should be valid (not updating status)
	cmd3 := &TasksReplaceCmd{TasklistID: "tl1", TaskID: "t1", Title: "Task", Status: ""}
	if err := cmd3.Run(ctx, flags); err != nil {
		t.Fatalf("empty status should be valid: %v", err)
	}
}
