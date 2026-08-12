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
	wantLines := []string{
		"CALENDAR\tSTATUS\tSTART\tEND\tERROR_DOMAIN\tERROR_REASON",
		"bad@example.com\terror\t\t\tglobal\tnotFound",
		"ok@example.com\tbusy\t2025-12-17T10:00:00Z\t2025-12-17T11:00:00Z\t\t",
	}
	if len(lines) != len(wantLines) {
		t.Fatalf("plain rows = %q, want %q", lines, wantLines)
	}
	for i := range wantLines {
		if lines[i] != wantLines[i] {
			t.Fatalf("plain row %d = %q, want %q", i, lines[i], wantLines[i])
		}
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
						"busy": []map[string]any{{"start": "2024-12-13T10:00:00Z", "end": "2024-12-13T11:00:00Z"}},
					},
					"good@example.com": map[string]any{
						"busy": []map[string]any{{"start": "2024-12-13T10:30:00Z", "end": "2024-12-13T11:30:00Z"}},
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
	var jsonStderr string
	out := captureStdout(t, func() {
		jsonStderr = captureStderr(t, func() {
			execErr = Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "conflicts",
				"--from", "2024-12-13T09:00:00Z",
				"--to", "2024-12-13T14:00:00Z",
				"--calendars", "primary,good@example.com,bad@example.com",
			})
		})
	})
	if execErr == nil {
		t.Fatal("expected nonzero exit for source calendar errors")
	}
	if ExitCode(execErr) == 0 {
		t.Fatalf("expected nonzero exit code, got 0 (%v)", execErr)
	}

	var transformedErr error
	var transformedStderr string
	transformedOut := captureStdout(t, func() {
		transformedStderr = captureStderr(t, func() {
			transformedErr = Execute([]string{
				"--json", "--results-only", "--select", "start,end",
				"--account", "a@b.com",
				"calendar", "conflicts",
				"--from", "2024-12-13T09:00:00Z",
				"--to", "2024-12-13T14:00:00Z",
				"--calendars", "primary,good@example.com,bad@example.com",
			})
		})
	})
	if transformedErr == nil || ExitCode(transformedErr) != ExitCode(execErr) {
		t.Fatalf("transformed error = %v, want exit code %d", transformedErr, ExitCode(execErr))
	}
	if transformedStderr != jsonStderr {
		t.Fatalf("transformed stderr changed\ngot:  %q\nwant: %q", transformedStderr, jsonStderr)
	}
	var transformedConflicts []map[string]any
	if err := json.Unmarshal([]byte(transformedOut), &transformedConflicts); err != nil {
		t.Fatalf("transformed JSON parse: %v\nout=%q", err, transformedOut)
	}
	if len(transformedConflicts) != 1 || transformedConflicts[0]["start"] != "2024-12-13T10:30:00Z" || transformedConflicts[0]["end"] != "2024-12-13T11:00:00Z" {
		t.Fatalf("unexpected transformed conflicts: %#v", transformedConflicts)
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
	if parsed.Count != 1 || len(parsed.Conflicts) != 1 {
		t.Fatalf("expected one primary conflict, got count=%d conflicts=%v", parsed.Count, parsed.Conflicts)
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

	var plainErr error
	plainOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			plainErr = Execute([]string{
				"--plain",
				"--account", "a@b.com",
				"calendar", "conflicts",
				"--from", "2024-12-13T09:00:00Z",
				"--to", "2024-12-13T14:00:00Z",
				"--calendars", "primary,good@example.com,bad@example.com",
			})
		})
	})
	if plainErr == nil || ExitCode(plainErr) == 0 {
		t.Fatalf("plain execution error = %v, want nonzero incomplete result", plainErr)
	}
	wantPlain := "TYPE\tSTATUS\tCALENDAR\tERROR_DOMAIN\tERROR_REASON\tSTART\tEND\tCALENDARS\n" +
		"error\tincomplete\tbad@example.com\tglobal\tnotFound\t\t\t\n" +
		"conflict\tincomplete\t\t\t\t2024-12-13T10:30:00Z\t2024-12-13T11:00:00Z\tgood@example.com, primary\n"
	if plainOut != wantPlain {
		t.Fatalf("plain output = %q, want %q", plainOut, wantPlain)
	}
	if strings.Contains(plainOut, "INCOMPLETE:") || strings.Contains(plainOut, "No conflicts found") {
		t.Fatalf("plain output leaked human prose: %q", plainOut)
	}
}

func TestCalendarConflicts_SourceErrors_PlainUsesOneSchemaWithConflicts(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/freeBusy") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"primary": map[string]any{
						"busy": []map[string]any{{"start": "2024-12-13T10:00:00Z", "end": "2024-12-13T12:00:00Z"}},
					},
					"team@example.com": map[string]any{
						"busy": []map[string]any{{"start": "2024-12-13T11:00:00Z", "end": "2024-12-13T13:00:00Z"}},
					},
					"bad@example.com": map[string]any{
						"errors": []map[string]any{{"domain": "global", "reason": "notFound"}},
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
				"--plain",
				"--account", "a@b.com",
				"calendar", "conflicts",
				"--from", "2024-12-13T09:00:00Z",
				"--to", "2024-12-13T14:00:00Z",
				"--calendars", "primary,team@example.com,bad@example.com",
			})
		})
	})
	if execErr == nil || ExitCode(execErr) == 0 {
		t.Fatalf("execution error = %v, want nonzero incomplete result", execErr)
	}
	want := "TYPE\tSTATUS\tCALENDAR\tERROR_DOMAIN\tERROR_REASON\tSTART\tEND\tCALENDARS\n" +
		"error\tincomplete\tbad@example.com\tglobal\tnotFound\t\t\t\n" +
		"conflict\tincomplete\t\t\t\t2024-12-13T11:00:00Z\t2024-12-13T12:00:00Z\tprimary, team@example.com\n"
	if out != want {
		t.Fatalf("plain output = %q, want one stable schema %q", out, want)
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
