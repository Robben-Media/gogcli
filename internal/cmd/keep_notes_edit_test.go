package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	keepapi "google.golang.org/api/keep/v1"
	"google.golang.org/api/option"
)

func TestKeepNotesCreate_TextNote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			var note keepapi.Note
			if err := json.Unmarshal(body, &note); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if note.Title != "Test Note" {
				t.Fatalf("expected title 'Test Note', got %q", note.Title)
			}
			if note.Body == nil || note.Body.Text == nil {
				t.Fatalf("expected text body")
			}
			if note.Body.Text.Text != "Hello world" {
				t.Fatalf("expected body 'Hello world', got %q", note.Body.Text.Text)
			}

			resp := keepapi.Note{
				Name:       "notes/abc123",
				Title:      note.Title,
				Body:       note.Body,
				CreateTime: "2026-01-01T00:00:00Z",
				UpdateTime: "2026-01-01T00:00:00Z",
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
			if err := Execute([]string{"--plain", "--account", account, "keep", "notes", "create", "--title", "Test Note", "--body", "Hello world"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "name\tnotes/abc123") {
		t.Fatalf("expected name in output, got: %q", out)
	}
	if !strings.Contains(out, "title\tTest Note") {
		t.Fatalf("expected title in output, got: %q", out)
	}
	if !strings.Contains(out, "type\ttext") {
		t.Fatalf("expected type text in output, got: %q", out)
	}
}

func TestKeepNotesCreate_ListNote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			var note keepapi.Note
			if err := json.Unmarshal(body, &note); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if note.Body == nil || note.Body.List == nil {
				t.Fatalf("expected list body")
			}
			if len(note.Body.List.ListItems) != 3 {
				t.Fatalf("expected 3 list items, got %d", len(note.Body.List.ListItems))
			}
			if note.Body.List.ListItems[0].Text.Text != "Buy milk" {
				t.Fatalf("expected first item 'Buy milk', got %q", note.Body.List.ListItems[0].Text.Text)
			}
			// Item at index 1 should be checked
			if !note.Body.List.ListItems[1].Checked {
				t.Fatalf("expected second item to be checked")
			}
			// Item at index 2 should not be checked
			if note.Body.List.ListItems[2].Checked {
				t.Fatalf("expected third item to not be checked")
			}

			resp := keepapi.Note{
				Name:       "notes/xyz789",
				Title:      note.Title,
				Body:       note.Body,
				CreateTime: "2026-01-01T00:00:00Z",
				UpdateTime: "2026-01-01T00:00:00Z",
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
			if err := Execute([]string{"--plain", "--account", account, "keep", "notes", "create", "--list-items", "Buy milk", "--list-items", "Buy eggs", "--list-items", "Buy bread", "--checked", "1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "name\tnotes/xyz789") {
		t.Fatalf("expected name in output, got: %q", out)
	}
	if !strings.Contains(out, "type\tlist") {
		t.Fatalf("expected type list in output, got: %q", out)
	}
}

func TestKeepNotesCreate_BodyFromFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	// Create temp file with body content
	bodyFile := filepath.Join(t.TempDir(), "body.txt")
	if err := os.WriteFile(bodyFile, []byte("Content from file"), 0o600); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			var note keepapi.Note
			if err := json.Unmarshal(body, &note); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if note.Body == nil || note.Body.Text == nil {
				t.Fatalf("expected text body")
			}
			if note.Body.Text.Text != "Content from file" {
				t.Fatalf("expected body 'Content from file', got %q", note.Body.Text.Text)
			}

			resp := keepapi.Note{
				Name:       "notes/fromfile",
				Body:       note.Body,
				CreateTime: "2026-01-01T00:00:00Z",
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
			if err := Execute([]string{"--plain", "--account", account, "keep", "notes", "create", "--body-from-file", bodyFile}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "name\tnotes/fromfile") {
		t.Fatalf("expected name in output, got: %q", out)
	}
}

func TestKeepNotesCreate_NoContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	// Use Execute to test the full command path
	err := Execute([]string{"--plain", "--account", account, "keep", "notes", "create"})
	if err == nil {
		t.Fatalf("expected error for no content")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestKeepNotesCreate_MutuallyExclusive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	// Use Execute to test the full command path with mutually exclusive flags
	err := Execute([]string{"--plain", "--account", account, "keep", "notes", "create", "--body", "text", "--list-items", "item"})
	if err == nil {
		t.Fatalf("expected error for mutually exclusive flags")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestKeepNotesCreate_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/notes":
			resp := keepapi.Note{
				Name:       "notes/json123",
				Title:      "JSON Note",
				Body:       &keepapi.Section{Text: &keepapi.TextContent{Text: "content"}},
				CreateTime: "2026-01-01T00:00:00Z",
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
			if err := Execute([]string{"--json", "keep", "notes", "create", "--body", "test", "--account", account}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var payload struct {
		Note map[string]any `json:"note"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if payload.Note["name"] != "notes/json123" {
		t.Fatalf("unexpected note: %#v", payload.Note)
	}
}

func TestKeepNotesDelete_Plain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/notes/abc123":
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
			if err := Execute([]string{"--force", "--plain", "--account", account, "keep", "notes", "delete", "abc123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "Deleted note notes/abc123") {
		t.Fatalf("expected delete message, got: %q", stderr)
	}
}

func TestKeepNotesDelete_WithNotePrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/notes/abc123":
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
			if err := Execute([]string{"--force", "--plain", "--account", account, "keep", "notes", "delete", "notes/abc123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "Deleted note notes/abc123") {
		t.Fatalf("expected delete message, got: %q", stderr)
	}
}

func TestKeepNotesDelete_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/notes/xyz789":
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
			if err := Execute([]string{"--json", "--force", "--account", account, "keep", "notes", "delete", "xyz789"}); err != nil {
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
	if payload["name"] != "notes/xyz789" {
		t.Fatalf("expected name=notes/xyz789, got: %v", payload)
	}
}

func TestKeepNotesDelete_RequiresConfirmation(t *testing.T) {
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

	err := Execute([]string{"--no-input", "--plain", "--account", account, "keep", "notes", "delete", "abc123"})
	if err == nil {
		t.Fatalf("expected error for confirmation required")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}
