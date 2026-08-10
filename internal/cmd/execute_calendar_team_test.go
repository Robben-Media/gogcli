package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/option"
)

func TestExecute_CalendarTeam_JSON(t *testing.T) {
	origCalSvc := newCalendarService
	origCloudSvc := newCloudIdentityService
	t.Cleanup(func() {
		newCalendarService = origCalSvc
		newCloudIdentityService = origCloudSvc
	})

	// Mock Cloud Identity server
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "groups:lookup"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "groups/abc123",
			})
		case strings.Contains(r.URL.Path, "groups/abc123/memberships"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{
					{
						"preferredMemberKey": map[string]any{"id": "alice@example.com"},
						"type":               "USER",
					},
					{
						"preferredMemberKey": map[string]any{"id": "bob@example.com"},
						"type":               "USER",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cloudSrv.Close()

	cloudSvc, err := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(cloudSrv.Client()),
		option.WithEndpoint(cloudSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService (cloud): %v", err)
	}
	newCloudIdentityService = func(context.Context, string) (*cloudidentity.Service, error) { return cloudSvc, nil }

	// Mock Calendar server
	calSrv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendars/alice@example.com/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "ev1",
						"summary": "Daily Standup",
						"start":   map[string]any{"dateTime": "2026-01-05T09:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T09:30:00Z"},
					},
				},
			})
		case strings.Contains(r.URL.Path, "/calendars/bob@example.com/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "ev1", // Same event (shared meeting)
						"summary": "Daily Standup",
						"start":   map[string]any{"dateTime": "2026-01-05T09:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T09:30:00Z"},
					},
					{
						"id":      "ev2",
						"summary": "Bob's 1:1",
						"start":   map[string]any{"dateTime": "2026-01-05T14:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T15:00:00Z"},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})))
	defer calSrv.Close()

	calSvc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(calSrv.Client()),
		option.WithEndpoint(calSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService (cal): %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return calSvc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "team", "engineering@example.com",
				"--from", "2026-01-05T00:00:00Z",
				"--to", "2026-01-06T00:00:00Z",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Group    string `json:"group"`
		TimeMin  string `json:"timeMin"`
		TimeMax  string `json:"timeMax"`
		Timezone string `json:"timezone"`
		Events   []struct {
			Who     string `json:"who"`
			ID      string `json:"id"`
			Summary string `json:"summary"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	// Should have 2 events (ev1 deduplicated, ev2 unique)
	if len(parsed.Events) != 2 {
		t.Fatalf("expected 2 events (deduplicated), got %d: %+v", len(parsed.Events), parsed.Events)
	}

	// ev1 should show both attendees
	foundStandup := false
	for _, ev := range parsed.Events {
		if ev.Summary == "Daily Standup" {
			foundStandup = true
			if !strings.Contains(ev.Who, "alice") || !strings.Contains(ev.Who, "bob") {
				t.Fatalf("expected both attendees in deduplicated event, got: %s", ev.Who)
			}
		}
	}
	if !foundStandup {
		t.Fatal("Daily Standup event not found")
	}
}

func TestExecute_CalendarTeam_FreeBusy(t *testing.T) {
	origCalSvc := newCalendarService
	origCloudSvc := newCloudIdentityService
	t.Cleanup(func() {
		newCalendarService = origCalSvc
		newCloudIdentityService = origCloudSvc
	})

	// Mock Cloud Identity server
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "groups:lookup"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/abc"})
		case strings.Contains(r.URL.Path, "groups/abc/memberships"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{
					{"preferredMemberKey": map[string]any{"id": "alice@example.com"}, "type": "USER"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cloudSrv.Close()

	cloudSvc, _ := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(cloudSrv.Client()),
		option.WithEndpoint(cloudSrv.URL+"/"),
	)
	newCloudIdentityService = func(context.Context, string) (*cloudidentity.Service, error) { return cloudSvc, nil }

	// Mock Calendar server
	calSrv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "freeBusy") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"alice@example.com": map[string]any{
						"busy": []map[string]any{
							{"start": "2026-01-05T09:00:00Z", "end": "2026-01-05T10:00:00Z"},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer calSrv.Close()

	calSvc, _ := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(calSrv.Client()),
		option.WithEndpoint(calSrv.URL+"/"),
	)
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return calSvc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "team", "eng@example.com",
				"--freebusy",
				"--from", "2026-01-05T00:00:00Z",
				"--to", "2026-01-06T00:00:00Z",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		FreeBusy []struct {
			Email string   `json:"email"`
			Busy  []string `json:"busy"`
		} `json:"freebusy"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}

	if len(parsed.FreeBusy) != 1 {
		t.Fatalf("expected 1 freebusy entry, got %d", len(parsed.FreeBusy))
	}
	if parsed.FreeBusy[0].Email != "alice@example.com" {
		t.Fatalf("unexpected email: %s", parsed.FreeBusy[0].Email)
	}
	if len(parsed.FreeBusy[0].Busy) != 1 {
		t.Fatalf("expected 1 busy block, got %d", len(parsed.FreeBusy[0].Busy))
	}
}

func TestExecute_CalendarTeam_Text(t *testing.T) {
	origCalSvc := newCalendarService
	origCloudSvc := newCloudIdentityService
	t.Cleanup(func() {
		newCalendarService = origCalSvc
		newCloudIdentityService = origCloudSvc
	})

	// Mock Cloud Identity server
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "groups:lookup"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/abc"})
		case strings.Contains(r.URL.Path, "groups/abc/memberships"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{
					{"preferredMemberKey": map[string]any{"id": "alice@example.com"}, "type": "USER"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cloudSrv.Close()

	cloudSvc, _ := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(cloudSrv.Client()),
		option.WithEndpoint(cloudSrv.URL+"/"),
	)
	newCloudIdentityService = func(context.Context, string) (*cloudidentity.Service, error) { return cloudSvc, nil }

	// Mock Calendar server
	calSrv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/alice@example.com/events") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "ev1",
						"summary": "Team Meeting",
						"start":   map[string]any{"dateTime": "2026-01-05T10:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T11:00:00Z"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer calSrv.Close()

	calSvc, _ := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(calSrv.Client()),
		option.WithEndpoint(calSrv.URL+"/"),
	)
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return calSvc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--account", "a@b.com",
				"calendar", "team", "eng@example.com",
				"--from", "2026-01-05T00:00:00Z",
				"--to", "2026-01-06T00:00:00Z",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	// Check text output format
	if !strings.Contains(out, "WHO") || !strings.Contains(out, "START") || !strings.Contains(out, "SUMMARY") {
		t.Fatalf("missing table headers in output: %q", out)
	}
	if !strings.Contains(out, "alice@example.com") {
		t.Fatalf("missing alice in output: %q", out)
	}
	if !strings.Contains(out, "Team Meeting") {
		t.Fatalf("missing event summary in output: %q", out)
	}
}

func executeCalendarTeamTest(t *testing.T, format string, members []string, calendarHandler http.Handler) (string, string, error) {
	t.Helper()

	origCalSvc := newCalendarService
	origCloudSvc := newCloudIdentityService
	t.Cleanup(func() {
		newCalendarService = origCalSvc
		newCloudIdentityService = origCloudSvc
	})

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "groups:lookup"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/abc123"})
		case strings.Contains(r.URL.Path, "groups/abc123/memberships"):
			memberships := make([]map[string]any, 0, len(members))
			for _, member := range members {
				memberships = append(memberships, map[string]any{
					"preferredMemberKey": map[string]any{"id": member},
					"type":               "USER",
				})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"memberships": memberships})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(cloudSrv.Close)

	cloudSvc, err := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(cloudSrv.Client()),
		option.WithEndpoint(cloudSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService (cloud): %v", err)
	}
	newCloudIdentityService = func(context.Context, string) (*cloudidentity.Service, error) { return cloudSvc, nil }

	calSrv := httptest.NewServer(withPrimaryCalendar(calendarHandler))
	t.Cleanup(calSrv.Close)

	calSvc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(calSrv.Client()),
		option.WithEndpoint(calSrv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService (cal): %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return calSvc, nil }

	var execErr error
	var errOut string
	out := captureStdout(t, func() {
		errOut = captureStderr(t, func() {
			execErr = Execute([]string{
				"--" + format,
				"--account", "a@b.com",
				"calendar", "team", "engineering@example.com",
				"--from", "2026-01-05T00:00:00Z",
				"--to", "2026-01-06T00:00:00Z",
			})
		})
	})
	return out, errOut, execErr
}

func TestExecute_CalendarTeam_JSON_PartialFailure(t *testing.T) {
	out, errOut, execErr := executeCalendarTeamTest(t, "json", []string{"alice@example.com", "bob@example.com"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendars/alice@example.com/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "ev1",
						"summary": "Alice Sync",
						"start":   map[string]any{"dateTime": "2026-01-05T09:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T09:30:00Z"},
					},
				},
			})
		case strings.Contains(r.URL.Path, "/calendars/bob@example.com/events"):
			http.Error(w, "access denied", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	if execErr == nil {
		t.Fatal("expected partial failure to return nonzero")
	}
	if ExitCode(execErr) == 0 {
		t.Fatalf("expected nonzero exit code, got 0 (%v)", execErr)
	}

	var parsed struct {
		Events []struct {
			Who     string `json:"who"`
			ID      string `json:"id"`
			Summary string `json:"summary"`
		} `json:"events"`
		Errors []struct {
			CalendarID string `json:"calendarId"`
			Error      string `json:"error"`
		} `json:"errors"`
		Complete bool `json:"complete"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Complete {
		t.Fatalf("expected complete=false: %#v", parsed)
	}
	if len(parsed.Events) != 1 || parsed.Events[0].ID != "ev1" || parsed.Events[0].Who != "alice@example.com" {
		t.Fatalf("successful events not preserved: %#v", parsed.Events)
	}
	if len(parsed.Errors) != 1 || parsed.Errors[0].CalendarID != "bob@example.com" || !strings.Contains(parsed.Errors[0].Error, "access denied") {
		t.Fatalf("failed calendar not reported: %#v", parsed.Errors)
	}
	if !strings.Contains(errOut, "bob@example.com") || !strings.Contains(errOut, "access denied") {
		t.Fatalf("missing actionable diagnostic: %q", errOut)
	}
}

func TestExecute_CalendarTeam_Plain_PartialFailure(t *testing.T) {
	out, _, execErr := executeCalendarTeamTest(t, "plain", []string{"alice@example.com", "bob@example.com"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendars/alice@example.com/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "ev1",
						"summary": "Alice Sync",
						"start":   map[string]any{"dateTime": "2026-01-05T09:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T09:30:00Z"},
					},
				},
			})
		case strings.Contains(r.URL.Path, "/calendars/bob@example.com/events"):
			http.Error(w, "access denied", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	if execErr == nil || ExitCode(execErr) == 0 {
		t.Fatalf("plain execution error = %v, want nonzero incomplete result", execErr)
	}

	lines := nonEmptyTeamLines(out)
	if len(lines) < 2 {
		t.Fatalf("expected event and error records, got %q", out)
	}
	if lines[0] != "TYPE\tWHO\tSTART\tEND\tSUMMARY\tERROR" {
		t.Fatalf("unexpected header=%q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "event\talice@example.com\t") || !strings.Contains(lines[1], "\tAlice Sync\t") {
		t.Fatalf("unexpected event record=%q", lines[1])
	}
	foundError := false
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "calendar_error\tbob@example.com\t") && strings.Contains(line, "access denied") {
			foundError = true
			if strings.Count(line, "\t") != 5 {
				t.Fatalf("error record is not parseable TSV: %q", line)
			}
		}
	}
	if !foundError {
		t.Fatalf("missing calendar_error record: %q", out)
	}
	if strings.Contains(out, "No events found") {
		t.Fatalf("must not use empty-result path on partial failure: %q", out)
	}
}

func TestExecute_CalendarTeam_JSON_AllFailed(t *testing.T) {
	out, _, execErr := executeCalendarTeamTest(t, "json", []string{"alice@example.com", "bob@example.com"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/events") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	if execErr == nil || ExitCode(execErr) == 0 {
		t.Fatalf("all-failed execution error = %v, want nonzero", execErr)
	}

	var parsed struct {
		Events   []map[string]any `json:"events"`
		Errors   []map[string]any `json:"errors"`
		Complete bool             `json:"complete"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Complete {
		t.Fatalf("expected complete=false on all-failed run: %#v", parsed)
	}
	if len(parsed.Events) != 0 {
		t.Fatalf("expected no events on all-failed run: %#v", parsed.Events)
	}
	if len(parsed.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %#v", parsed.Errors)
	}
}

func TestExecute_CalendarTeam_JSON_SuccessfulEmpty(t *testing.T) {
	out, _, execErr := executeCalendarTeamTest(t, "json", []string{"alice@example.com"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/alice@example.com/events") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
			return
		}
		http.NotFound(w, r)
	}))
	if execErr != nil {
		t.Fatalf("successful empty schedule must succeed: %v", execErr)
	}

	var parsed struct {
		Events   []map[string]any `json:"events"`
		Errors   []map[string]any `json:"errors"`
		Complete bool             `json:"complete"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Complete {
		t.Fatalf("expected complete=true for successful empty schedule: %#v", parsed)
	}
	if len(parsed.Events) != 0 {
		t.Fatalf("expected no events: %#v", parsed.Events)
	}
	if len(parsed.Errors) != 0 {
		t.Fatalf("expected no errors: %#v", parsed.Errors)
	}
}

func TestExecute_CalendarTeam_Plain_SuccessfulKeepsFourColumnSchema(t *testing.T) {
	out, _, execErr := executeCalendarTeamTest(t, "plain", []string{"alice@example.com"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/alice@example.com/events") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "ev1",
						"summary": "Standup\tplanning\nnotes",
						"start":   map[string]any{"dateTime": "2026-01-05T09:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-01-05T09:30:00Z"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	if execErr != nil {
		t.Fatalf("successful plain schedule must succeed: %v", execErr)
	}

	lines := nonEmptyTeamLines(out)
	want := []string{
		"WHO\tSTART\tEND\tSUMMARY",
		"alice@example.com\t09:00\t09:30\tStandup planning notes",
	}
	if len(lines) != len(want) {
		t.Fatalf("plain rows = %q, want %q", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("plain row %d = %q, want %q", i, lines[i], want[i])
		}
	}
	if strings.Contains(out, "TYPE\t") || strings.Contains(out, "calendar_error") {
		t.Fatalf("successful plain output must keep 4-column schema: %q", out)
	}
}

func TestExecute_CalendarTeam_Plain_AllFailed(t *testing.T) {
	out, _, execErr := executeCalendarTeamTest(t, "plain", []string{"bob@example.com", "alice@example.com"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/bob@example.com/events") {
			http.Error(w, "bob denied\tsecret", http.StatusForbidden)
			return
		}
		if strings.Contains(r.URL.Path, "/calendars/alice@example.com/events") {
			http.Error(w, "alice missing", http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	if execErr == nil || ExitCode(execErr) == 0 {
		t.Fatalf("all-failed plain execution error = %v, want nonzero", execErr)
	}

	lines := nonEmptyTeamLines(out)
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 error rows, got %q", out)
	}
	if lines[0] != "TYPE\tWHO\tSTART\tEND\tSUMMARY\tERROR" {
		t.Fatalf("unexpected header=%q", lines[0])
	}
	// Deterministic multi-error order by calendar id.
	if !strings.HasPrefix(lines[1], "calendar_error\talice@example.com\t") || !strings.Contains(lines[1], "alice missing") {
		t.Fatalf("expected alice error first: %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "calendar_error\tbob@example.com\t") || !strings.Contains(lines[2], "bob denied secret") {
		t.Fatalf("expected bob error second and tab-safe: %q", lines[2])
	}
	if strings.Contains(out, "No events found") {
		t.Fatalf("all-failed must not use empty success path: %q", out)
	}
	for _, line := range lines[1:] {
		if strings.Count(line, "\t") != 5 {
			t.Fatalf("error record is not parseable TSV: %q", line)
		}
	}
}

func nonEmptyTeamLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
