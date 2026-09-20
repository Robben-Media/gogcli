package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestListCalendarIDsEvents_IndependentPagination(t *testing.T) {
	var mu sync.Mutex
	type req struct {
		cal   string
		page  string
		count int
	}
	var seen []req
	pageHits := map[string]int{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/calendars/") || !strings.HasSuffix(r.URL.Path, "/events") {
			http.NotFound(w, r)
			return
		}
		calID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/calendars/"), "/events")
		page := r.URL.Query().Get("pageToken")

		mu.Lock()
		pageHits[calID+"|"+page]++
		seen = append(seen, req{cal: calID, page: page, count: pageHits[calID+"|"+page]})
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch calID {
		case "cal-busy":
			switch page {
			case "":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"id": "b1", "summary": "Busy 1", "start": map[string]any{"dateTime": "2025-01-01T09:00:00Z"}, "end": map[string]any{"dateTime": "2025-01-01T09:30:00Z"}},
					},
					"nextPageToken": "busy-page-2",
				})
			case "busy-page-2":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"id": "b2", "summary": "Busy 2", "start": map[string]any{"dateTime": "2025-01-01T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-01-01T10:30:00Z"}},
					},
					"nextPageToken": "busy-page-3",
				})
			case "busy-page-3":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"id": "b3", "summary": "Busy 3", "start": map[string]any{"dateTime": "2025-01-01T11:00:00Z"}, "end": map[string]any{"dateTime": "2025-01-01T11:30:00Z"}},
					},
					"nextPageToken": "",
				})
			default:
				http.Error(w, "unexpected busy pageToken "+page, http.StatusBadRequest)
			}
		case "cal-short":
			switch page {
			case "":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"id": "s1", "summary": "Short 1", "start": map[string]any{"dateTime": "2025-01-01T08:00:00Z"}, "end": map[string]any{"dateTime": "2025-01-01T08:30:00Z"}},
					},
					"nextPageToken": "short-page-2",
				})
			case "short-page-2":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"id": "s2", "summary": "Short 2", "start": map[string]any{"dateTime": "2025-01-01T12:00:00Z"}, "end": map[string]any{"dateTime": "2025-01-01T12:30:00Z"}},
					},
					"nextPageToken": "",
				})
			default:
				http.Error(w, "unexpected short pageToken "+page, http.StatusBadRequest)
			}
		default:
			http.NotFound(w, r)
		}
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	ids := []string{"cal-busy", "cal-short"}

	// Page 1: one request per calendar, no page tokens.
	out1 := captureStdout(t, func() {
		if err := listCalendarIDsEvents(ctx, svc, ids, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 1, "", "", "", "", "", false); err != nil {
			t.Fatalf("page1: %v", err)
		}
	})
	var page1 struct {
		Events        []map[string]any `json:"events"`
		NextPageToken string           `json:"nextPageToken"`
		Complete      bool             `json:"complete"`
		Errors        []map[string]any `json:"errors"`
	}
	if err := json.Unmarshal([]byte(out1), &page1); err != nil {
		t.Fatalf("page1 json: %v\nout=%s", err, out1)
	}
	if !page1.Complete || len(page1.Errors) != 0 {
		t.Fatalf("page1 completeness: %#v", page1)
	}
	if page1.NextPageToken == "" {
		t.Fatalf("page1 expected aggregate nextPageToken, got empty")
	}
	gotIDs := eventIDs(page1.Events)
	if !sameStringSet(gotIDs, []string{"b1", "s1"}) {
		t.Fatalf("page1 events=%v", gotIDs)
	}

	mu.Lock()
	firstSeen := append([]req(nil), seen...)
	seen = nil
	mu.Unlock()
	if len(firstSeen) != 2 {
		t.Fatalf("page1 expected 2 requests, got %#v", firstSeen)
	}
	for _, r := range firstSeen {
		if r.page != "" {
			t.Fatalf("page1 expected empty pageToken, got %#v", r)
		}
	}

	// Page 2: only unfinished calendars, each with its own Google token.
	out2 := captureStdout(t, func() {
		if err := listCalendarIDsEvents(ctx, svc, ids, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 1, page1.NextPageToken, "", "", "", "", false); err != nil {
			t.Fatalf("page2: %v", err)
		}
	})
	var page2 struct {
		Events        []map[string]any `json:"events"`
		NextPageToken string           `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out2), &page2); err != nil {
		t.Fatalf("page2 json: %v\nout=%s", err, out2)
	}
	gotIDs = eventIDs(page2.Events)
	if !sameStringSet(gotIDs, []string{"b2", "s2"}) {
		t.Fatalf("page2 events=%v", gotIDs)
	}
	if page2.NextPageToken == "" {
		t.Fatalf("page2 expected remaining aggregate token for cal-busy")
	}
	if page2.NextPageToken == page1.NextPageToken {
		t.Fatalf("page2 token should differ from page1")
	}

	mu.Lock()
	secondSeen := append([]req(nil), seen...)
	seen = nil
	mu.Unlock()
	if len(secondSeen) != 2 {
		t.Fatalf("page2 expected 2 requests, got %#v", secondSeen)
	}
	pageByCal := map[string]string{}
	for _, r := range secondSeen {
		pageByCal[r.cal] = r.page
	}
	if pageByCal["cal-busy"] != "busy-page-2" || pageByCal["cal-short"] != "short-page-2" {
		t.Fatalf("page2 page tokens not independent: %#v", pageByCal)
	}

	// Page 3: short calendar exhausted; only busy continues. No repeats.
	out3 := captureStdout(t, func() {
		if err := listCalendarIDsEvents(ctx, svc, ids, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 1, page2.NextPageToken, "", "", "", "", false); err != nil {
			t.Fatalf("page3: %v", err)
		}
	})
	var page3 struct {
		Events        []map[string]any `json:"events"`
		NextPageToken string           `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out3), &page3); err != nil {
		t.Fatalf("page3 json: %v\nout=%s", err, out3)
	}
	gotIDs = eventIDs(page3.Events)
	if !sameStringSet(gotIDs, []string{"b3"}) {
		t.Fatalf("page3 events=%v", gotIDs)
	}
	if page3.NextPageToken != "" {
		t.Fatalf("page3 expected empty aggregate token, got %q", page3.NextPageToken)
	}

	mu.Lock()
	thirdSeen := append([]req(nil), seen...)
	mu.Unlock()
	if len(thirdSeen) != 1 || thirdSeen[0].cal != "cal-busy" || thirdSeen[0].page != "busy-page-3" {
		t.Fatalf("page3 expected only cal-busy with busy-page-3, got %#v", thirdSeen)
	}

	// Across pages: no repeats, no silent omissions.
	all := append(append(eventIDs(page1.Events), eventIDs(page2.Events)...), eventIDs(page3.Events)...)
	if !sameStringSet(all, []string{"b1", "b2", "b3", "s1", "s2"}) {
		t.Fatalf("aggregate event set incorrect: %v", all)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5 unique events across pages, got %v", all)
	}
}

func TestListCalendarIDsEvents_MalformedAggregatePageToken(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	for _, bad := range []string{
		"not-a-valid-multi-cursor",
		multiCalendarPageCursorPrefix + base64.RawURLEncoding.EncodeToString([]byte(`{}`)),
		multiCalendarPageCursorPrefix,
	} {
		called = false
		err = listCalendarIDsEvents(ctx, svc, []string{"cal1", "cal2"}, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 10, bad, "", "", "", "", false)
		if err == nil {
			t.Fatalf("expected usage error for malformed aggregate cursor %q", bad)
		}
		var ee *ExitError
		if !errors.As(err, &ee) || ee.Code != 2 {
			t.Fatalf("expected usage ExitError code 2 for %q, got %v", bad, err)
		}
		if called {
			t.Fatalf("malformed cursor %q must fail before event requests", bad)
		}
	}
}

func TestListCalendarIDsEvents_IncompatibleAggregatePageToken(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	// Cursor selection must match the current multi-calendar selection exactly.
	foreign, err := encodeMultiCalendarPageCursor([]string{"other-cal"}, map[string]string{"other-cal": "tok"})
	if err != nil {
		t.Fatalf("encode foreign: %v", err)
	}
	// Legitimate partial progress for c1+c2 must not be reusable after expanding selection.
	expanded, err := encodeMultiCalendarPageCursor([]string{"cal1", "cal2"}, map[string]string{"cal1": "tok1"})
	if err != nil {
		t.Fatalf("encode expanded: %v", err)
	}
	// Shrinking selection is also incompatible.
	shrunk, err := encodeMultiCalendarPageCursor([]string{"cal1", "cal2"}, map[string]string{"cal1": "tok1"})
	if err != nil {
		t.Fatalf("encode shrunk: %v", err)
	}

	cases := []struct {
		name      string
		cursor    string
		selection []string
	}{
		{name: "foreign calendar", cursor: foreign, selection: []string{"cal1", "cal2"}},
		{name: "expanded selection", cursor: expanded, selection: []string{"cal1", "cal2", "cal3"}},
		{name: "shrunk selection", cursor: shrunk, selection: []string{"cal1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			err := listCalendarIDsEvents(ctx, svc, tc.selection, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 10, tc.cursor, "", "", "", "", false)
			if err == nil {
				t.Fatal("expected usage error for incompatible aggregate cursor")
			}
			var ee *ExitError
			if !errors.As(err, &ee) || ee.Code != 2 {
				t.Fatalf("expected usage ExitError code 2, got %v", err)
			}
			if called {
				t.Fatal("incompatible cursor must fail before event requests")
			}
		})
	}
}

func TestListCalendarIDsEvents_FirstPageFailureKeepsRetryableCursor(t *testing.T) {
	var mu sync.Mutex
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/events") {
			http.NotFound(w, r)
			return
		}
		calID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/calendars/"), "/events")
		page := r.URL.Query().Get("pageToken")
		mu.Lock()
		pages = append(pages, calID+"|"+page)
		mu.Unlock()
		switch calID {
		case "cal-ok":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "ok1", "summary": "OK", "start": map[string]any{"dateTime": "2025-01-01T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-01-01T11:00:00Z"}},
				},
				"nextPageToken": "ok-page-2",
			})
		case "cal-fail":
			http.Error(w, "temporary outage", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
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
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	ids := []string{"cal-ok", "cal-fail"}

	var listErr error
	out := captureStdout(t, func() {
		listErr = listCalendarIDsEvents(ctx, svc, ids, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 1, "", "", "", "", "", false)
	})
	if listErr == nil {
		t.Fatal("expected partial failure on first page")
	}
	var parsed struct {
		Events        []map[string]any `json:"events"`
		NextPageToken string           `json:"nextPageToken"`
		Complete      bool             `json:"complete"`
	}
	if unmarshalErr := json.Unmarshal([]byte(out), &parsed); unmarshalErr != nil {
		t.Fatalf("json: %v out=%s", unmarshalErr, out)
	}
	if parsed.Complete {
		t.Fatal("expected complete=false")
	}
	if !sameStringSet(eventIDs(parsed.Events), []string{"ok1"}) {
		t.Fatalf("expected successful events preserved, got %#v", parsed.Events)
	}
	if parsed.NextPageToken == "" {
		t.Fatal("first-page failure must leave a retryable aggregate cursor")
	}
	cursor, err := decodeMultiCalendarPageCursor(parsed.NextPageToken)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !sameStringSet(cursor.Selection, ids) {
		t.Fatalf("cursor selection=%v want %v", cursor.Selection, ids)
	}
	// Failed calendar must remain unfinished and distinct from exhausted.
	failTok, failOK := cursor.Next["cal-fail"]
	if !failOK {
		t.Fatalf("failed calendar missing from unfinished map: %#v", cursor.Next)
	}
	if failTok != "" {
		t.Fatalf("first-page failure should retry from start, got token %q", failTok)
	}
	if cursor.Next["cal-ok"] != "ok-page-2" {
		t.Fatalf("successful calendar continuation lost: %#v", cursor.Next)
	}

	// Retry with the emitted cursor: only unfinished calendars are requested,
	// including the failed one from its first page.
	mu.Lock()
	pages = nil
	mu.Unlock()
	out2 := captureStdout(t, func() {
		if err := listCalendarIDsEvents(ctx, svc, ids, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 1, parsed.NextPageToken, "", "", "", "", false); err == nil {
			// cal-fail still fails in this fixture; partial failure is expected.
			t.Fatalf("expected cal-fail to still fail on retry fixture")
		}
	})
	_ = out2
	mu.Lock()
	gotPages := append([]string(nil), pages...)
	mu.Unlock()
	if !sameStringSet(gotPages, []string{"cal-ok|ok-page-2", "cal-fail|"}) {
		t.Fatalf("retry requests=%#v", gotPages)
	}
}

func TestExecute_CalendarEvents_Text_MultiCalendarPagingHint(t *testing.T) {
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
				"items": []map[string]any{
					{"id": "e1", "summary": "E1", "start": map[string]any{"dateTime": "2025-12-17T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-12-17T11:00:00Z"}},
				},
				"nextPageToken": "c1-next",
			})
		case strings.Contains(r.URL.Path, "/calendars/c2/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e2", "summary": "E2", "start": map[string]any{"dateTime": "2025-12-17T12:00:00Z"}, "end": map[string]any{"dateTime": "2025-12-17T13:00:00Z"}},
				},
				"nextPageToken": "c2-next",
			})
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

	var errOut string
	out := captureStdout(t, func() {
		errOut = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "--account", "a@b.com", "calendar", "events", "--calendars", "c1,c2", "--from", "2025-12-17T00:00:00Z", "--to", "2025-12-18T00:00:00Z"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "e1") || !strings.Contains(out, "e2") {
		t.Fatalf("unexpected out=%q", out)
	}
	if !strings.Contains(errOut, "# Next page: --page ") {
		t.Fatalf("expected multi-calendar page hint, stderr=%q", errOut)
	}
	// Hint must carry an opaque aggregate cursor, not a raw single-calendar token alone.
	if strings.Contains(errOut, "# Next page: --page c1-next") || strings.Contains(errOut, "# Next page: --page c2-next") {
		t.Fatalf("expected opaque aggregate cursor, stderr=%q", errOut)
	}
}

func TestListCalendarIDsEvents_FailedContinuationKeepsCursor(t *testing.T) {
	var mu sync.Mutex
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/events") {
			http.NotFound(w, r)
			return
		}
		calID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/calendars/"), "/events")
		page := r.URL.Query().Get("pageToken")
		mu.Lock()
		pages = append(pages, calID+"|"+page)
		mu.Unlock()
		if calID == "cal-ok" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "ok2", "summary": "OK", "start": map[string]any{"dateTime": "2025-01-01T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-01-01T11:00:00Z"}},
				},
				"nextPageToken": "",
			})
			return
		}
		if calID == "cal-fail" {
			http.Error(w, "temporary outage", http.StatusServiceUnavailable)
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
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cursor, err := encodeMultiCalendarPageCursor([]string{"cal-ok", "cal-fail"}, map[string]string{
		"cal-ok":   "ok-page-2",
		"cal-fail": "fail-page-2",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var listErr error
	out := captureStdout(t, func() {
		listErr = listCalendarIDsEvents(ctx, svc, []string{"cal-ok", "cal-fail"}, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 1, cursor, "", "", "", "", false)
	})
	if listErr == nil {
		t.Fatal("expected partial failure")
	}
	var parsed struct {
		NextPageToken string `json:"nextPageToken"`
		Complete      bool   `json:"complete"`
	}
	if unmarshalErr := json.Unmarshal([]byte(out), &parsed); unmarshalErr != nil {
		t.Fatalf("json: %v out=%s", unmarshalErr, out)
	}
	if parsed.Complete {
		t.Fatal("expected complete=false")
	}
	if parsed.NextPageToken == "" {
		t.Fatal("failed calendar must remain in aggregate cursor")
	}
	decoded, err := decodeMultiCalendarPageCursor(parsed.NextPageToken)
	if err != nil {
		t.Fatalf("decode next: %v", err)
	}
	if decoded.Next["cal-fail"] != "fail-page-2" {
		t.Fatalf("expected preserved fail token, got %#v", decoded.Next)
	}
	if _, ok := decoded.Next["cal-ok"]; ok {
		t.Fatalf("exhausted ok calendar should not remain unfinished: %#v", decoded.Next)
	}
	if !sameStringSet(decoded.Selection, []string{"cal-ok", "cal-fail"}) {
		t.Fatalf("selection should remain complete: %#v", decoded.Selection)
	}
	mu.Lock()
	gotPages := append([]string(nil), pages...)
	mu.Unlock()
	if !sameStringSet(gotPages, []string{"cal-ok|ok-page-2", "cal-fail|fail-page-2"}) {
		t.Fatalf("unexpected requests: %#v", gotPages)
	}
}

func TestListCalendarEvents_SingleCalendarPageTokenUnchanged(t *testing.T) {
	var gotPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/cal1/events") && r.Method == http.MethodGet {
			gotPage = r.URL.Query().Get("pageToken")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "Event", "start": map[string]any{"dateTime": "2025-01-01T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-01-01T11:00:00Z"}},
				},
				"nextPageToken": "google-next",
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		if err := listCalendarEvents(ctx, svc, "cal1", "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 10, "google-prev", "", "", "", "", false); err != nil {
			t.Fatalf("listCalendarEvents: %v", err)
		}
	})
	if gotPage != "google-prev" {
		t.Fatalf("single-calendar page token not forwarded: %q", gotPage)
	}
	var parsed struct {
		Next string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json: %v", err)
	}
	if parsed.Next != "google-next" {
		t.Fatalf("single-calendar nextPageToken changed: %q", parsed.Next)
	}
}

func eventIDs(events []map[string]any) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		if id, ok := e["id"].(string); ok {
			out = append(out, id)
		}
	}
	return out
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	m := make(map[string]int, len(want))
	for _, w := range want {
		m[w]++
	}
	for _, g := range got {
		if m[g] == 0 {
			return false
		}
		m[g]--
	}
	return true
}
