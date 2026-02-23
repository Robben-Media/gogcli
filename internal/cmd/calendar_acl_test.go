package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// Test Calendar ACL List Command
func TestCalendarAclListCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/calendars/primary/acl") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":    "user:alice@example.com",
						"role":  "owner",
						"scope": map[string]any{"type": "user", "value": "alice@example.com"},
					},
					{
						"id":    "user:bob@example.com",
						"role":  "reader",
						"scope": map[string]any{"type": "user", "value": "bob@example.com"},
					},
				},
				"nextPageToken": "",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &CalendarAclListCmd{}
		if err := runKong(t, cmd, []string{"primary"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Rules []struct {
			ID    string `json:"id"`
			Role  string `json:"role"`
			Scope struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"scope"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(parsed.Rules))
	}
	if parsed.Rules[0].Role != "owner" {
		t.Fatalf("unexpected role: %q", parsed.Rules[0].Role)
	}
}

func TestCalendarAclListCmd_EmptyCalendar(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for empty calendar ID")
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &CalendarAclListCmd{}
	err = runKong(t, cmd, []string{""}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty calendar ID")
	}
}

// Test Calendar ACL Get Command
func TestCalendarAclGetCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/calendars/primary/acl/user:alice@example.com") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "user:alice@example.com",
				"role": "owner",
				"scope": map[string]any{
					"type":  "user",
					"value": "alice@example.com",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &CalendarAclGetCmd{}
		if err := runKong(t, cmd, []string{"primary", "user:alice@example.com"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Rule struct {
			ID    string `json:"id"`
			Role  string `json:"role"`
			Scope struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"scope"`
		} `json:"rule"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Rule.ID != "user:alice@example.com" {
		t.Fatalf("unexpected id: %q", parsed.Rule.ID)
	}
	if parsed.Rule.Role != "owner" {
		t.Fatalf("unexpected role: %q", parsed.Rule.Role)
	}
}

func TestCalendarAclGetCmd_MissingRuleID(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for missing rule ID")
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &CalendarAclGetCmd{}
	err = runKong(t, cmd, []string{"primary", ""}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty rule ID")
	}
}

// Test Calendar ACL Insert Command
func TestCalendarAclInsertCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/calendars/primary/acl") {
			var body struct {
				Role  string `json:"role"`
				Scope struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"scope"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body.Role != "reader" {
				http.Error(w, "unexpected role", http.StatusBadRequest)
				return
			}
			if body.Scope.Type != "user" {
				http.Error(w, "unexpected scope type", http.StatusBadRequest)
				return
			}
			if body.Scope.Value != "bob@example.com" {
				http.Error(w, "unexpected scope value", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "user:bob@example.com",
				"role": body.Role,
				"scope": map[string]any{
					"type":  body.Scope.Type,
					"value": body.Scope.Value,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &CalendarAclInsertCmd{}
		if err := runKong(t, cmd, []string{"primary", "--scope-type", "user", "--scope-value", "bob@example.com", "--role", "reader"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Rule struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"rule"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Rule.ID != "user:bob@example.com" {
		t.Fatalf("unexpected id: %q", parsed.Rule.ID)
	}
}

func TestCalendarAclInsertCmd_DefaultScope(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/calendars/primary/acl") {
			var body struct {
				Role  string `json:"role"`
				Scope struct {
					Type string `json:"type"`
				} `json:"scope"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body.Scope.Type != "default" {
				http.Error(w, "expected default scope", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "default",
				"role": body.Role,
				"scope": map[string]any{
					"type": body.Scope.Type,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &CalendarAclInsertCmd{}
		// default scope doesn't need scope-value
		if err := runKong(t, cmd, []string{"primary", "--scope-type", "default", "--role", "reader"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Rule struct {
			ID    string `json:"id"`
			Role  string `json:"role"`
			Scope struct {
				Type string `json:"type"`
			} `json:"scope"`
		} `json:"rule"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Rule.Scope.Type != "default" {
		t.Fatalf("unexpected scope type: %q", parsed.Rule.Scope.Type)
	}
}

func TestCalendarAclInsertCmd_InvalidScopeType(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for invalid scope type")
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &CalendarAclInsertCmd{}
	err = runKong(t, cmd, []string{"primary", "--scope-type", "invalid", "--role", "reader"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for invalid scope type")
	}
}

func TestCalendarAclInsertCmd_MissingScopeValue(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for missing scope value")
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &CalendarAclInsertCmd{}
	// user scope needs scope-value
	err = runKong(t, cmd, []string{"primary", "--scope-type", "user", "--role", "reader"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing scope value")
	}
}

func TestCalendarAclInsertCmd_InvalidRole(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for invalid role")
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &CalendarAclInsertCmd{}
	err = runKong(t, cmd, []string{"primary", "--scope-type", "user", "--scope-value", "bob@example.com", "--role", "invalid"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

// Test Calendar ACL Delete Command
func TestCalendarAclDeleteCmd_Force(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/calendars/primary/acl/user:bob@example.com") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &CalendarAclDeleteCmd{}
		if err := runKong(t, cmd, []string{"primary", "user:bob@example.com"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.ID != "user:bob@example.com" {
		t.Fatalf("unexpected id: %q", parsed.ID)
	}
	if !parsed.Deleted {
		t.Fatal("expected deleted to be true")
	}
}

func TestCalendarAclDeleteCmd_RequiresConfirmation(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called without confirmation")
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", NoInput: true}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &CalendarAclDeleteCmd{}
	err = runKong(t, cmd, []string{"primary", "user:bob@example.com"}, ctx, flags)
	if err == nil {
		t.Fatal("expected confirmation error")
	}
}

// Test Calendar ACL Patch Command
func TestCalendarAclPatchCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/calendars/primary/acl/user:bob@example.com") {
			var body struct {
				Role string `json:"role"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body.Role != "writer" {
				http.Error(w, "unexpected role", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "user:bob@example.com",
				"role": body.Role,
				"scope": map[string]any{
					"type":  "user",
					"value": "bob@example.com",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &CalendarAclPatchCmd{}
		if err := runKong(t, cmd, []string{"primary", "user:bob@example.com", "--role", "writer"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Rule struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"rule"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Rule.ID != "user:bob@example.com" {
		t.Fatalf("unexpected id: %q", parsed.Rule.ID)
	}
	if parsed.Rule.Role != "writer" {
		t.Fatalf("unexpected role: %q", parsed.Rule.Role)
	}
}

func TestCalendarAclPatchCmd_NoUpdates(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called when no updates provided")
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &CalendarAclPatchCmd{}
	err = runKong(t, cmd, []string{"primary", "user:bob@example.com"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for no updates")
	}
	if !strings.Contains(err.Error(), "no updates provided") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCalendarAclPatchCmd_InvalidRole(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for invalid role")
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &CalendarAclPatchCmd{}
	err = runKong(t, cmd, []string{"primary", "user:bob@example.com", "--role", "invalid"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

// Test Calendar ACL Watch Command
func TestCalendarAclWatchCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/calendars/primary/acl/watch") {
			var body struct {
				ID      string `json:"id"`
				Type    string `json:"type"`
				Address string `json:"address"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body.Type != "web_hook" {
				http.Error(w, "unexpected type", http.StatusBadRequest)
				return
			}
			if body.Address != "https://example.com/webhook" {
				http.Error(w, "unexpected address", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          body.ID,
				"type":        body.Type,
				"address":     body.Address,
				"resourceId":  "resource-123",
				"resourceUri": "https://www.googleapis.com/calendar/v3/calendars/primary/acl",
				"expiration":  "1735689600000",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &CalendarAclWatchCmd{}
		if err := runKong(t, cmd, []string{"primary", "--webhook-url", "https://example.com/webhook", "--channel-id", "test-channel-123"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Channel struct {
			ID         string `json:"id"`
			ResourceID string `json:"resourceId"`
			Type       string `json:"type"`
		} `json:"channel"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Channel.ID != "test-channel-123" {
		t.Fatalf("unexpected channel id: %q", parsed.Channel.ID)
	}
	if parsed.Channel.ResourceID != "resource-123" {
		t.Fatalf("unexpected resource id: %q", parsed.Channel.ResourceID)
	}
}

func TestCalendarAclWatchCmd_MissingWebhookURL(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called without webhook URL")
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &CalendarAclWatchCmd{}
	err = runKong(t, cmd, []string{"primary"}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing webhook URL")
	}
}

// Test Execute integration for calendar ACL commands
func TestExecute_CalendarAclList_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/calendars/primary/acl") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "user:alice@example.com", "role": "owner", "scope": map[string]any{"type": "user", "value": "alice@example.com"}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "acl", "list", "primary"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"user:alice@example.com"`) {
			t.Fatalf("expected user:alice@example.com in out=%q", out)
		}
	})
}

func TestExecute_CalendarAclGet_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/calendars/primary/acl/user:alice@example.com") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "user:alice@example.com",
				"role": "owner",
				"scope": map[string]any{
					"type":  "user",
					"value": "alice@example.com",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "acl", "get", "primary", "user:alice@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"owner"`) {
			t.Fatalf("expected owner role in out=%q", out)
		}
	})
}

func TestExecute_CalendarAclInsert_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/calendars/primary/acl") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "user:bob@example.com",
				"role": "reader",
				"scope": map[string]any{
					"type":  "user",
					"value": "bob@example.com",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "acl", "insert", "primary", "--scope-type", "user", "--scope-value", "bob@example.com", "--role", "reader"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"user:bob@example.com"`) {
			t.Fatalf("expected user:bob@example.com in out=%q", out)
		}
	})
}

func TestExecute_CalendarAclDelete_Force(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/calendars/primary/acl/user:bob@example.com") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "--force", "calendar", "acl", "delete", "primary", "user:bob@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"deleted"`) || !strings.Contains(out, `"user:bob@example.com"`) {
			t.Fatalf("expected deleted and user:bob@example.com in out=%q", out)
		}
	})
}

func TestExecute_CalendarAclPatch_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/calendars/primary/acl/user:bob@example.com") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "user:bob@example.com",
				"role": "writer",
				"scope": map[string]any{
					"type":  "user",
					"value": "bob@example.com",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "acl", "patch", "primary", "user:bob@example.com", "--role", "writer"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"writer"`) {
			t.Fatalf("expected writer role in out=%q", out)
		}
	})
}

func TestExecute_CalendarAclWatch_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/calendars/primary/acl/watch") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "channel-123",
				"type":        "web_hook",
				"resourceId":  "resource-456",
				"resourceUri": "https://www.googleapis.com/calendar/v3/calendars/primary/acl",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		out := captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "acl", "watch", "primary", "--webhook-url", "https://example.com/webhook", "--channel-id", "channel-123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(out, `"channel-123"`) {
			t.Fatalf("expected channel-123 in out=%q", out)
		}
	})
}
