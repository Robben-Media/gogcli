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

// Test Calendar Calendars List Command
func TestCalendarCalendarsListCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/me/calendarList") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "primary", "summary": "Primary", "accessRole": "owner", "primary": true},
					{"id": "work@example.com", "summary": "Work", "accessRole": "writer"},
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

		cmd := &CalendarCalendarsListCmd{}
		if err := runKong(t, cmd, []string{}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Calendars []*calendar.CalendarListEntry `json:"calendars"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Calendars) != 2 {
		t.Fatalf("expected 2 calendars, got %d", len(parsed.Calendars))
	}
	if parsed.Calendars[0].Id != "primary" {
		t.Fatalf("unexpected id: %q", parsed.Calendars[0].Id)
	}
}

// Test Calendar Calendars Get Command
func TestCalendarCalendarsGetCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/me/calendarList/primary") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "primary",
				"summary":    "Primary Calendar",
				"accessRole": "owner",
				"primary":    true,
				"timeZone":   "America/New_York",
				"hidden":     false,
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

		cmd := &CalendarCalendarsGetCmd{}
		if err := runKong(t, cmd, []string{"primary"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Calendar struct {
			ID         string `json:"id"`
			Summary    string `json:"summary"`
			AccessRole string `json:"accessRole"`
			Primary    bool   `json:"primary"`
		} `json:"calendar"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Calendar.ID != "primary" {
		t.Fatalf("unexpected id: %q", parsed.Calendar.ID)
	}
	if parsed.Calendar.Summary != "Primary Calendar" {
		t.Fatalf("unexpected summary: %q", parsed.Calendar.Summary)
	}
}

func TestCalendarCalendarsGetCmd_EmptyCalendarID(t *testing.T) {
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

	cmd := &CalendarCalendarsGetCmd{}
	err = runKong(t, cmd, []string{""}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty calendar ID")
	}
}

// Test Calendar Calendars Insert Command
func TestCalendarCalendarsInsertCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/me/calendarList") {
			var body struct {
				Id              string `json:"id"`
				SummaryOverride string `json:"summaryOverride"`
				Hidden          bool   `json:"hidden"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body.Id != "work@example.com" {
				http.Error(w, "unexpected id", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":              body.Id,
				"summary":         "Work Calendar",
				"summaryOverride": body.SummaryOverride,
				"hidden":          body.Hidden,
				"accessRole":      "writer",
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

		cmd := &CalendarCalendarsInsertCmd{}
		if err := runKong(t, cmd, []string{"work@example.com", "--summary", "My Work"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Calendar struct {
			ID              string `json:"id"`
			Summary         string `json:"summary"`
			SummaryOverride string `json:"summaryOverride"`
		} `json:"calendar"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Calendar.ID != "work@example.com" {
		t.Fatalf("unexpected id: %q", parsed.Calendar.ID)
	}
	if parsed.Calendar.SummaryOverride != "My Work" {
		t.Fatalf("unexpected summary override: %q", parsed.Calendar.SummaryOverride)
	}
}

// Test Calendar Calendars Delete Command
func TestCalendarCalendarsDeleteCmd_Force(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/users/me/calendarList/work@example.com") {
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

		cmd := &CalendarCalendarsDeleteCmd{}
		if err := runKong(t, cmd, []string{"work@example.com"}, ctx, flags); err != nil {
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
	if parsed.ID != "work@example.com" {
		t.Fatalf("unexpected id: %q", parsed.ID)
	}
	if !parsed.Deleted {
		t.Fatal("expected deleted to be true")
	}
}

func TestCalendarCalendarsDeleteCmd_RequiresConfirmation(t *testing.T) {
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

	cmd := &CalendarCalendarsDeleteCmd{}
	err = runKong(t, cmd, []string{"work@example.com"}, ctx, flags)
	if err == nil {
		t.Fatal("expected confirmation error")
	}
}

// Test Calendar Calendars Patch Command
func TestCalendarCalendarsPatchCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/users/me/calendarList/work@example.com") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":              "work@example.com",
				"summary":         "Work Calendar",
				"summaryOverride": "My Work",
				"hidden":          true,
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

		cmd := &CalendarCalendarsPatchCmd{}
		if err := runKong(t, cmd, []string{"work@example.com", "--summary", "My Work", "--hidden"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Calendar struct {
			ID              string `json:"id"`
			SummaryOverride string `json:"summaryOverride"`
			Hidden          bool   `json:"hidden"`
		} `json:"calendar"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Calendar.ID != "work@example.com" {
		t.Fatalf("unexpected id: %q", parsed.Calendar.ID)
	}
	if parsed.Calendar.SummaryOverride != "My Work" {
		t.Fatalf("unexpected summary override: %q", parsed.Calendar.SummaryOverride)
	}
	if !parsed.Calendar.Hidden {
		t.Fatal("expected hidden to be true")
	}
}

// Test Calendar Calendars Watch Command
func TestCalendarCalendarsWatchCmd_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/me/calendarList/watch") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "channel-123",
				"type":        "web_hook",
				"resourceId":  "resource-456",
				"resourceUri": "https://www.googleapis.com/calendar/v3/users/me/calendarList",
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

		cmd := &CalendarCalendarsWatchCmd{}
		if err := runKong(t, cmd, []string{"--webhook-url", "https://example.com/webhook", "--channel-id", "channel-123"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Channel struct {
			ID         string `json:"id"`
			ResourceID string `json:"resourceId"`
		} `json:"channel"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Channel.ID != "channel-123" {
		t.Fatalf("unexpected channel id: %q", parsed.Channel.ID)
	}
}

func TestCalendarCalendarsWatchCmd_MissingWebhookURL(t *testing.T) {
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

	cmd := &CalendarCalendarsWatchCmd{}
	err = runKong(t, cmd, []string{}, ctx, flags)
	if err == nil {
		t.Fatal("expected error for missing webhook URL")
	}
}
