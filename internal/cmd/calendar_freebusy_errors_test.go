package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func TestCalendarFreeBusy_TextEmitsBusyAndErrorRows(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/freeBusy") && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"calendars": map[string]any{
				"ok@example.com": map[string]any{
					"busy": []map[string]any{
						{"start": "2025-12-17T10:00:00Z", "end": "2025-12-17T11:00:00Z"},
					},
				},
				"bad@example.com": map[string]any{
					"errors": []map[string]any{
						{"domain": "global", "reason": "notFound"},
					},
				},
			},
		})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--plain",
				"--account", "a@b.com",
				"calendar", "freebusy",
				"ok@example.com,bad@example.com",
				"--from", "2025-12-17T00:00:00Z",
				"--to", "2025-12-18T00:00:00Z",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	lines := nonEmptyLines(out)
	if len(lines) < 3 {
		t.Fatalf("expected header + at least 2 data rows, got %q", out)
	}
	if lines[0] != "CALENDAR\tSTATUS\tSTART\tEND\tERROR_DOMAIN\tERROR_REASON" {
		t.Fatalf("unexpected header: %q", lines[0])
	}

	wantBusy := "ok@example.com\tbusy\t2025-12-17T10:00:00Z\t2025-12-17T11:00:00Z\t\t"
	wantError := "bad@example.com\terror\t\t\tglobal\tnotFound"
	if !containsLine(lines, wantBusy) {
		t.Fatalf("missing busy row %q in output %q", wantBusy, out)
	}
	if !containsLine(lines, wantError) {
		t.Fatalf("missing error row %q in output %q", wantError, out)
	}
}

func TestCalendarFreeBusy_JSONRetainsPerCalendarErrors(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/freeBusy") && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"calendars": map[string]any{
				"bad@example.com": map[string]any{
					"errors": []map[string]any{
						{"domain": "calendar", "reason": "notFound"},
					},
				},
			},
		})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "freebusy",
				"bad@example.com",
				"--from", "2025-12-17T00:00:00Z",
				"--to", "2025-12-18T00:00:00Z",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Calendars map[string]struct {
			Errors []struct {
				Domain string `json:"domain"`
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"calendars"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	cal, ok := parsed.Calendars["bad@example.com"]
	if !ok {
		t.Fatalf("missing calendar in JSON: %q", out)
	}
	if len(cal.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d (%q)", len(cal.Errors), out)
	}
	if cal.Errors[0].Domain != "calendar" || cal.Errors[0].Reason != "notFound" {
		t.Fatalf("unexpected error payload: %+v", cal.Errors[0])
	}
}

func TestCalendarConflicts_SourceErrors_JSONIncompleteAndNonzero(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/freeBusy") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"primary": map[string]any{
						"busy": []map[string]any{},
					},
					"bad@example.com": map[string]any{
						"errors": []map[string]any{
							{"domain": "global", "reason": "notFound"},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
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

	var execErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			execErr = Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "conflicts",
				"--from", "2024-12-13T09:00:00Z",
				"--to", "2024-12-13T14:00:00Z",
				"--calendars", "primary,bad@example.com",
			})
		})
	})
	if execErr == nil {
		t.Fatal("expected nonzero exit for source calendar errors")
	}
	if ExitCode(execErr) == 0 {
		t.Fatalf("expected nonzero exit code, got 0 (%v)", execErr)
	}

	var parsed struct {
		Conflicts  []map[string]any `json:"conflicts"`
		Count      int              `json:"count"`
		Incomplete bool             `json:"incomplete"`
		Errors     []struct {
			Calendar string `json:"calendar"`
			Domain   string `json:"domain"`
			Reason   string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Incomplete {
		t.Fatalf("expected incomplete=true, out=%q", out)
	}
	if parsed.Count != 0 || len(parsed.Conflicts) != 0 {
		t.Fatalf("expected no conflicts, got count=%d conflicts=%v", parsed.Count, parsed.Conflicts)
	}
	if len(parsed.Errors) != 1 {
		t.Fatalf("expected 1 source error, got %+v (out=%q)", parsed.Errors, out)
	}
	if parsed.Errors[0].Calendar != "bad@example.com" {
		t.Fatalf("unexpected calendar: %+v", parsed.Errors[0])
	}
	if parsed.Errors[0].Domain != "global" || parsed.Errors[0].Reason != "notFound" {
		t.Fatalf("unexpected error fields: %+v", parsed.Errors[0])
	}
}

func TestCalendarConflicts_SourceErrors_SuppressCleanNoConflictMessage(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/freeBusy") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"primary": map[string]any{
						"busy": []map[string]any{},
					},
					"bad@example.com": map[string]any{
						"errors": []map[string]any{
							{"domain": "global", "reason": "notFound"},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
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

	var execErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			execErr = Execute([]string{
				"--account", "a@b.com",
				"calendar", "conflicts",
				"--from", "2024-12-13T09:00:00Z",
				"--to", "2024-12-13T14:00:00Z",
				"--calendars", "primary,bad@example.com",
			})
		})
	})
	if execErr == nil {
		t.Fatal("expected nonzero exit for source calendar errors")
	}
	if strings.Contains(out, "No conflicts found") {
		t.Fatalf("must not report clean no-conflict result when a calendar failed: %q", out)
	}
	if !strings.Contains(out, "bad@example.com") {
		t.Fatalf("expected failed calendar in output: %q", out)
	}
	if !strings.Contains(out, "notFound") {
		t.Fatalf("expected error reason in output: %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "incomplete") {
		t.Fatalf("expected incomplete status in output: %q", out)
	}
}

func nonEmptyLines(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}
