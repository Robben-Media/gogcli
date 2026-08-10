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

func TestExecute_CalendarEvents_Text_WithPaging(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendars/c1/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "S", "start": map[string]any{"dateTime": "2025-12-17T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-12-17T11:00:00Z"}},
				},
				"nextPageToken": "npt",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
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

	out := captureStdout(t, func() {
		errOut := captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "calendar", "events", "c1", "--from", "2025-12-17T00:00:00Z", "--to", "2025-12-18T00:00:00Z"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(errOut, "# Next page: --page npt") {
			t.Fatalf("unexpected stderr=%q", errOut)
		}
	})
	if !strings.Contains(out, "ID") || !strings.Contains(out, "START") || !strings.Contains(out, "e1") || !strings.Contains(out, "S") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_CalendarEvents_Text_AllReportsPartialFailure(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/users/me/calendarList"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1"},
					{"id": "c2"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c1/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "S1\tteam\nsync", "start": map[string]any{"dateTime": "2025-12-17T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-12-17T11:00:00Z"}},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c2/events"):
			http.Error(w, "access denied", http.StatusForbidden)
			return
		default:
			http.NotFound(w, r)
			return
		}
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
	var errOut string
	out := captureStdout(t, func() {
		errOut = captureStderr(t, func() {
			execErr = Execute([]string{"--plain", "--account", "a@b.com", "calendar", "events", "--all", "--from", "2025-12-17T00:00:00Z", "--to", "2025-12-18T00:00:00Z"})
		})
	})
	if execErr == nil {
		t.Fatal("expected partial failure to return nonzero")
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected header, event, and error records; got %q", out)
	}
	if lines[0] != "TYPE\tCALENDAR\tID\tSTART\tEND\tSUMMARY\tERROR" {
		t.Fatalf("unexpected header=%q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "event\tc1\te1\t") || !strings.Contains(lines[1], "\tS1 team sync\t") {
		t.Fatalf("unexpected event record=%q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "calendar_error\tc2\t\t\t\t\t") || !strings.Contains(lines[2], "access denied") {
		t.Fatalf("unexpected error record=%q", lines[2])
	}
	if strings.Contains(lines[2], "\n") || strings.Count(lines[2], "\t") != 6 {
		t.Fatalf("error record is not parseable TSV: %q", lines[2])
	}
	if !strings.Contains(errOut, "calendar c2:") || !strings.Contains(errOut, "access denied") {
		t.Fatalf("missing actionable diagnostic: %q", errOut)
	}

	weekdayOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			execErr = Execute([]string{"--plain", "--account", "a@b.com", "calendar", "events", "--all", "--weekday", "--from", "2025-12-17T00:00:00Z", "--to", "2025-12-18T00:00:00Z"})
		})
	})
	if execErr == nil {
		t.Fatal("expected weekday partial failure to return nonzero")
	}
	weekdayLines := strings.Split(strings.TrimSpace(weekdayOut), "\n")
	if len(weekdayLines) != 3 || weekdayLines[0] != "TYPE\tCALENDAR\tID\tSTART\tSTART_DOW\tEND\tEND_DOW\tSUMMARY\tERROR" {
		t.Fatalf("unexpected weekday records=%q", weekdayOut)
	}
	if strings.Count(weekdayLines[1], "\t") != 8 || strings.Count(weekdayLines[2], "\t") != 8 || !strings.HasPrefix(weekdayLines[2], "calendar_error\tc2\t") {
		t.Fatalf("weekday rows are not stable TSV: %q", weekdayOut)
	}
}

func TestExecute_CalendarEvents_JSON_CalendarsReportsPartialFailure(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/users/me/calendarList"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"id": "c1"}, {"id": "c2"}},
			})
		case strings.Contains(r.URL.Path, "/calendars/c1/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{
					"id": "e1", "summary": "S1",
					"start": map[string]any{"dateTime": "2025-12-17T10:00:00Z"},
					"end":   map[string]any{"dateTime": "2025-12-17T11:00:00Z"},
				}},
			})
		case strings.Contains(r.URL.Path, "/calendars/c2/events"):
			http.Error(w, "access denied", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
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
	var errOut string
	out := captureStdout(t, func() {
		errOut = captureStderr(t, func() {
			execErr = Execute([]string{"--json", "--account", "a@b.com", "calendar", "events", "--calendars", "c1,c2", "--from", "2025-12-17T00:00:00Z", "--to", "2025-12-18T00:00:00Z"})
		})
	})
	if execErr == nil {
		t.Fatal("expected partial failure to return nonzero")
	}

	var parsed struct {
		Events []map[string]any `json:"events"`
		Errors []struct {
			CalendarID string `json:"calendarId"`
			Error      string `json:"error"`
		} `json:"errors"`
		Complete bool `json:"complete"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nout=%q", err, out)
	}
	if parsed.Complete {
		t.Fatalf("expected complete=false: %#v", parsed)
	}
	if len(parsed.Events) != 1 || parsed.Events[0]["id"] != "e1" {
		t.Fatalf("successful events not preserved: %#v", parsed.Events)
	}
	if len(parsed.Errors) != 1 || parsed.Errors[0].CalendarID != "c2" || !strings.Contains(parsed.Errors[0].Error, "access denied") {
		t.Fatalf("failed calendar not reported: %#v", parsed.Errors)
	}
	if !strings.Contains(errOut, "calendar c2:") || !strings.Contains(errOut, "access denied") {
		t.Fatalf("missing actionable diagnostic: %q", errOut)
	}
}
