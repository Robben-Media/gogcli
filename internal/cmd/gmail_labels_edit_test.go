package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// TestGmailLabelsPatchCmd tests partial update of a label
func TestGmailLabelsPatchCmd_NameOnly(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "Old Name", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/users/me/labels/Label_1"):
			var body struct {
				Name string `json:"name,omitempty"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body.Name != "New Name" {
				http.Error(w, "unexpected name", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                    "Label_1",
				"name":                  body.Name,
				"type":                  "user",
				"labelListVisibility":   "labelShow",
				"messageListVisibility": "show",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsPatchCmd{}
		if err := runKong(t, cmd, []string{"Label_1", "--name", "New Name"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Label struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"label"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Label.ID != "Label_1" {
		t.Fatalf("unexpected id: %q", parsed.Label.ID)
	}
	if parsed.Label.Name != "New Name" {
		t.Fatalf("unexpected name: %q", parsed.Label.Name)
	}
}

func TestGmailLabelsPatchCmd_ResolveNameToID(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "My Label", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/users/me/labels/Label_1"):
			var body struct {
				Name string `json:"name,omitempty"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "Label_1",
				"name": body.Name,
				"type": "user",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsPatchCmd{}
		// Use name "My Label" which should resolve to ID "Label_1"
		if err := runKong(t, cmd, []string{"My Label", "--name", "Updated Label"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Label struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"label"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Label.ID != "Label_1" {
		t.Fatalf("unexpected id: %q", parsed.Label.ID)
	}
	if parsed.Label.Name != "Updated Label" {
		t.Fatalf("unexpected name: %q", parsed.Label.Name)
	}
}

func TestGmailLabelsPatchCmd_VisibilityFlags(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "Test", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/users/me/labels/Label_1"):
			var body struct {
				LabelListVisibility   string `json:"labelListVisibility,omitempty"`
				MessageListVisibility string `json:"messageListVisibility,omitempty"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body.LabelListVisibility != "labelHide" {
				http.Error(w, "unexpected labelListVisibility", http.StatusBadRequest)
				return
			}
			if body.MessageListVisibility != "hide" {
				http.Error(w, "unexpected messageListVisibility", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                    "Label_1",
				"name":                  "Test",
				"type":                  "user",
				"labelListVisibility":   body.LabelListVisibility,
				"messageListVisibility": body.MessageListVisibility,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsPatchCmd{}
		if err := runKong(t, cmd, []string{"Label_1", "--label-list-visibility", "labelHide", "--message-list-visibility", "hide"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Label struct {
			ID                    string `json:"id"`
			LabelListVisibility   string `json:"labelListVisibility"`
			MessageListVisibility string `json:"messageListVisibility"`
		} `json:"label"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Label.LabelListVisibility != "labelHide" {
		t.Fatalf("unexpected labelListVisibility: %q", parsed.Label.LabelListVisibility)
	}
	if parsed.Label.MessageListVisibility != "hide" {
		t.Fatalf("unexpected messageListVisibility: %q", parsed.Label.MessageListVisibility)
	}
}

func TestGmailLabelsPatchCmd_NoUpdates(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	// Server shouldn't be called for validation error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when no updates provided")
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailLabelsPatchCmd{}
	err = runKong(t, cmd, []string{"Label_1"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for no updates")
	}
	if !strings.Contains(err.Error(), "no updates provided") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailLabelsPatchCmd_EmptyLabel(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	// Server shouldn't be called for validation error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for empty label")
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailLabelsPatchCmd{Name: "Test"}
	err = runKong(t, cmd, []string{"", "--name", "Test"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty label")
	}
}

// TestGmailLabelsUpdateCmd tests full replacement of a label
func TestGmailLabelsUpdateCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "Old Name", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/me/labels/Label_1"):
			var body struct {
				Name                  string `json:"name"`
				LabelListVisibility   string `json:"labelListVisibility"`
				MessageListVisibility string `json:"messageListVisibility"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body.Name != "Updated Name" {
				http.Error(w, "unexpected name", http.StatusBadRequest)
				return
			}
			if body.LabelListVisibility != "labelShow" {
				http.Error(w, "unexpected labelListVisibility", http.StatusBadRequest)
				return
			}
			if body.MessageListVisibility != "show" {
				http.Error(w, "unexpected messageListVisibility", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                    "Label_1",
				"name":                  body.Name,
				"type":                  "user",
				"labelListVisibility":   body.LabelListVisibility,
				"messageListVisibility": body.MessageListVisibility,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsUpdateCmd{}
		if err := runKong(t, cmd, []string{"Label_1", "--name", "Updated Name"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Label struct {
			ID                    string `json:"id"`
			Name                  string `json:"name"`
			LabelListVisibility   string `json:"labelListVisibility"`
			MessageListVisibility string `json:"messageListVisibility"`
		} `json:"label"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Label.ID != "Label_1" {
		t.Fatalf("unexpected id: %q", parsed.Label.ID)
	}
	if parsed.Label.Name != "Updated Name" {
		t.Fatalf("unexpected name: %q", parsed.Label.Name)
	}
	if parsed.Label.LabelListVisibility != "labelShow" {
		t.Fatalf("unexpected labelListVisibility: %q", parsed.Label.LabelListVisibility)
	}
	if parsed.Label.MessageListVisibility != "show" {
		t.Fatalf("unexpected messageListVisibility: %q", parsed.Label.MessageListVisibility)
	}
}

func TestGmailLabelsUpdateCmd_ResolveNameToID(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "My Label", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/me/labels/Label_1"):
			var body struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "Label_1",
				"name": body.Name,
				"type": "user",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsUpdateCmd{}
		// Use name "My Label" which should resolve to ID "Label_1"
		if err := runKong(t, cmd, []string{"My Label", "--name", "New Name"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Label struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"label"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Label.ID != "Label_1" {
		t.Fatalf("unexpected id: %q", parsed.Label.ID)
	}
	if parsed.Label.Name != "New Name" {
		t.Fatalf("unexpected name: %q", parsed.Label.Name)
	}
}

func TestGmailLabelsUpdateCmd_CustomVisibility(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "Test", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/me/labels/Label_1"):
			var body struct {
				Name                  string `json:"name"`
				LabelListVisibility   string `json:"labelListVisibility"`
				MessageListVisibility string `json:"messageListVisibility"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body.LabelListVisibility != "labelShowIfUnread" {
				http.Error(w, "unexpected labelListVisibility", http.StatusBadRequest)
				return
			}
			if body.MessageListVisibility != "hide" {
				http.Error(w, "unexpected messageListVisibility", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                    "Label_1",
				"name":                  body.Name,
				"type":                  "user",
				"labelListVisibility":   body.LabelListVisibility,
				"messageListVisibility": body.MessageListVisibility,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsUpdateCmd{}
		if err := runKong(t, cmd, []string{"Label_1", "--name", "Test", "--label-list-visibility", "labelShowIfUnread", "--message-list-visibility", "hide"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Label struct {
			LabelListVisibility   string `json:"labelListVisibility"`
			MessageListVisibility string `json:"messageListVisibility"`
		} `json:"label"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Label.LabelListVisibility != "labelShowIfUnread" {
		t.Fatalf("unexpected labelListVisibility: %q", parsed.Label.LabelListVisibility)
	}
	if parsed.Label.MessageListVisibility != "hide" {
		t.Fatalf("unexpected messageListVisibility: %q", parsed.Label.MessageListVisibility)
	}
}

func TestGmailLabelsUpdateCmd_EmptyLabel(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	// Server shouldn't be called for validation error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for empty label")
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailLabelsUpdateCmd{}
	err = runKong(t, cmd, []string{"", "--name", "Test"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty label")
	}
}

func TestGmailLabelsUpdateCmd_MissingName(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	// Server shouldn't be called for validation error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for missing name")
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailLabelsUpdateCmd{}
	// Kong should fail to parse because --name is required
	err = runKong(t, cmd, []string{"Label_1"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing required --name flag")
	}
}

// Test execute-level integration for new commands
func TestExecute_GmailLabelsPatch_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/labels/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "MyLabel", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels/Label_1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "Label_1",
				"name": "Renamed",
				"type": "user",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "labels", "patch", "Label_1", "--name", "Renamed"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"Label_1"`) {
			t.Fatalf("expected Label_1 in out=%q", out)
		}
		if !strings.Contains(out, `"Renamed"`) {
			t.Fatalf("expected Renamed in out=%q", out)
		}
	})
}

func TestExecute_GmailLabelsUpdate_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/labels/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "OldLabel", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels/Label_1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "Label_1",
				"name": "NewLabel",
				"type": "user",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "labels", "update", "Label_1", "--name", "NewLabel"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"Label_1"`) {
			t.Fatalf("expected Label_1 in out=%q", out)
		}
		if !strings.Contains(out, `"NewLabel"`) {
			t.Fatalf("expected NewLabel in out=%q", out)
		}
	})
}
