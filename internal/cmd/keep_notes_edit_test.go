package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	keepapi "google.golang.org/api/keep/v1"
	"google.golang.org/api/option"
)

func TestKeepNotes_Create_TextNote_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/notes" {
			var note keepapi.Note
			if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if note.Title != "Test Note" {
				t.Fatalf("expected title 'Test Note', got %q", note.Title)
			}
			if note.Body == nil || note.Body.Text == nil {
				t.Fatal("expected text body")
			}
			if note.Body.Text.Text != "Hello world" {
				t.Fatalf("expected body 'Hello world', got %q", note.Body.Text.Text)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":       "notes/abc123",
				"title":      note.Title,
				"createTime": "2026-01-15T10:00:00.000Z",
				"body": map[string]any{
					"text": map[string]any{"text": note.Body.Text.Text},
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
		if err := Execute([]string{"keep", "notes", "create", "--account", account, "--json", "--title", "Test Note", "--body", "Hello world"}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	note, ok := payload["note"].(map[string]any)
	if !ok {
		t.Fatal("expected note in response")
	}
	if note["name"] != "notes/abc123" {
		t.Fatalf("expected name 'notes/abc123', got %v", note["name"])
	}
	if note["title"] != "Test Note" {
		t.Fatalf("expected title 'Test Note', got %v", note["title"])
	}
}

func TestKeepNotes_Create_ListNote_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/notes" {
			var note keepapi.Note
			if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if note.Body == nil || note.Body.List == nil {
				t.Fatal("expected list body")
			}
			if len(note.Body.List.ListItems) != 3 {
				t.Fatalf("expected 3 list items, got %d", len(note.Body.List.ListItems))
			}
			// Item at index 1 should be checked
			if !note.Body.List.ListItems[1].Checked {
				t.Fatal("expected item at index 1 to be checked")
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":       "notes/list123",
				"title":      "Shopping List",
				"createTime": "2026-01-15T10:00:00.000Z",
				"body": map[string]any{
					"list": map[string]any{
						"listItems": note.Body.List.ListItems,
					},
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
			"keep", "notes", "create",
			"--account", account,
			"--json",
			"--title", "Shopping List",
			"--list-items", "Milk",
			"--list-items", "Bread",
			"--list-items", "Eggs",
			"--checked", "1",
		}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	note, ok := payload["note"].(map[string]any)
	if !ok {
		t.Fatal("expected note in response")
	}
	if note["name"] != "notes/list123" {
		t.Fatalf("expected name 'notes/list123', got %v", note["name"])
	}
}

func TestKeepNotes_Create_FromFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	// Create temp file with body content
	bodyFile := filepath.Join(t.TempDir(), "body.txt")
	bodyContent := "Content from file\nMultiple lines"
	if err := os.WriteFile(bodyFile, []byte(bodyContent), 0o600); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/notes" {
			var note keepapi.Note
			if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if note.Body == nil || note.Body.Text == nil {
				t.Fatal("expected text body")
			}
			if note.Body.Text.Text != bodyContent {
				t.Fatalf("expected body content from file, got %q", note.Body.Text.Text)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":       "notes/file123",
				"title":      "",
				"createTime": "2026-01-15T10:00:00.000Z",
				"body": map[string]any{
					"text": map[string]any{"text": note.Body.Text.Text},
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
		if err := Execute([]string{"keep", "notes", "create", "--account", account, "--json", "--body-from-file", bodyFile}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	note, ok := payload["note"].(map[string]any)
	if !ok {
		t.Fatal("expected note in response")
	}
	if note["name"] != "notes/file123" {
		t.Fatalf("expected name 'notes/file123', got %v", note["name"])
	}
}

func TestKeepNotes_Create_NoContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	stderr := captureStderr(t, func() {
		if err := Execute([]string{"keep", "notes", "create", "--account", account, "--title", "Test"}); err == nil {
			t.Fatal("expected error")
		} else if !strings.Contains(err.Error(), "must provide") {
			t.Fatalf("expected content required error, got %v", err)
		}
	})
	_ = stderr
}

func TestKeepNotes_Create_MutuallyExclusive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	stderr := captureStderr(t, func() {
		if err := Execute([]string{"keep", "notes", "create", "--account", account, "--body", "text", "--list-items", "item"}); err == nil {
			t.Fatal("expected error")
		} else if !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("expected mutually exclusive error, got %v", err)
		}
	})
	_ = stderr
}

func TestKeepNotes_Delete_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/notes/abc123" {
			w.WriteHeader(http.StatusNoContent)
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
		if err := Execute([]string{"keep", "notes", "delete", "notes/abc123", "--account", account, "--json", "--force"}); err != nil {
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
	if payload["name"] != "notes/abc123" {
		t.Fatalf("expected name 'notes/abc123', got %v", payload["name"])
	}
}

func TestKeepNotes_Delete_ShortID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should prepend "notes/" to short ID
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/notes/shortid" {
			w.WriteHeader(http.StatusNoContent)
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
		if err := Execute([]string{"keep", "notes", "delete", "shortid", "--account", account, "--force"}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})
	if !strings.Contains(stderr, "Deleted note notes/shortid") {
		t.Fatalf("expected delete message in stderr, got %q", stderr)
	}
}

func TestKeepNotes_Delete_RequiresConfirmation(t *testing.T) {
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

	// NoInput should cause confirmation to fail with exit code 2
	if err := Execute([]string{"keep", "notes", "delete", "abc123", "--account", account, "--no-input"}); err == nil {
		t.Fatal("expected confirmation error")
	} else if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %v", ExitCode(err))
	}
}

func TestKeepNotes_Delete_EmptyID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	// Empty ID should fail
	if err := Execute([]string{"keep", "notes", "delete", "", "--account", account, "--force"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestKeepNotes_TextOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	account := "a@b.com"
	_ = writeKeepSA(t, account)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/notes" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":       "notes/xyz",
				"title":      "My Note",
				"createTime": "2026-01-15T10:00:00.000Z",
				"body": map[string]any{
					"text": map[string]any{"text": "body content"},
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
		if err := Execute([]string{"keep", "notes", "create", "--account", account, "--title", "My Note", "--body", "test"}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	if !strings.Contains(stdout, "name\tnotes/xyz") {
		t.Fatalf("expected name in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "title\tMy Note") {
		t.Fatalf("expected title in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "type\ttext") {
		t.Fatalf("expected type in output, got %q", stdout)
	}
}
