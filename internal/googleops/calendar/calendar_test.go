package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

type fixtureTransport struct {
	t        *testing.T
	response string
	requests []*http.Request
	bodies   []string
}

func (t *fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.t.Helper()
	body := ""

	if req.Body != nil {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			t.t.Fatalf("read request body: %v", err)
		}
		body = string(data)
	}
	t.requests = append(t.requests, req)
	t.bodies = append(t.bodies, body)

	return &http.Response{
		StatusCode: http.StatusOK, Status: "200 OK", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(t.response)), ContentLength: int64(len(t.response)),
	}, nil
}

type fixtureProvider struct {
	transport  *fixtureTransport
	options    []mcpcontract.CallOptions
	identities []mcpcontract.Identity
}

func (p *fixtureProvider) HTTPClient(_ context.Context, id mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	p.options = append(p.options, options)
	p.identities = append(p.identities, id)

	return &http.Client{Transport: p.transport}, nil
}

func fixture(t *testing.T, operation, response string) (mcpcontract.Operation, *fixtureProvider) {
	t.Helper()
	transport := &fixtureTransport{t: t, response: response}

	provider := &fixtureProvider{transport: transport}
	for _, candidate := range Operations(provider) {
		if candidate.Definition.Name == operation {
			return candidate, provider
		}
	}

	t.Fatalf("operation %s was not registered", operation)

	return mcpcontract.Operation{}, nil
}

func decodeRun(t *testing.T, operation mcpcontract.Operation, raw string) (any, error) {
	t.Helper()

	call, err := operation.Decode(json.RawMessage(raw))
	if err != nil {
		return nil, fmt.Errorf("decode operation: %w", err)
	}

	result, err := call.Run(context.Background(), mcpcontract.Identity{
		AccountID: "opaque-account", Subject: "subject", Email: "user@example.test", Label: "Work",
		PrincipalID: "principal", ClientName: "client", AuthMode: "oauth", Generation: 7,
	})
	if err != nil {
		return nil, fmt.Errorf("run operation: %w", err)
	}

	return result, nil
}

func TestCalendarListProjectsMetadataAndUsesDefaultBounds(t *testing.T) {
	t.Parallel()
	operation, provider := fixture(t, "calendar_list", `{
		"nextPageToken": "calendar-next",
		"items": [
			{"id": "primary", "summary": "Work", "timeZone": "America/Chicago", "accessRole": "owner", "primary": true, "selected": true},
			{"id": "holidays", "summary": "Holidays", "timeZone": "UTC", "accessRole": "reader", "deleted": true}
		]
	}`)

	result, err := decodeRun(t, operation, `{"account_id":"opaque-account"}`)
	if err != nil {
		t.Fatal(err)
	}

	out, ok := result.(mcpcontract.Result[CalendarListData])
	if !ok {
		t.Fatalf("got result type %T", result)
	}

	if out.AccountID != "opaque-account" || out.AccountLabel != "Work" || out.NextPageToken != "calendar-next" {
		t.Fatalf("unexpected envelope: %#v", out)
	}

	if len(out.Data.Calendars) != 2 || out.Data.Calendars[0].TimeZone != "America/Chicago" ||
		!out.Data.Calendars[0].Primary || !out.Data.Calendars[1].Deleted {
		t.Fatalf("unexpected projection: %#v", out.Data.Calendars)
	}

	if got := provider.transport.requests[0].URL.Query().Get("maxResults"); got != "100" {
		t.Fatalf("maxResults = %q", got)
	}

	if provider.options[0] != (mcpcontract.CallOptions{Operation: "calendar_list", Retry: mcpcontract.SafeRead}) {
		t.Fatalf("unexpected call options: %#v", provider.options[0])
	}
}

func TestCalendarInputSchemasMatchFrozenRequests(t *testing.T) {
	t.Parallel()
	operations := Operations(&fixtureProvider{})
	schemas := make(map[string]map[string]bool, len(operations))

	required := make(map[string][]string, len(operations))
	for _, operation := range operations {
		encoded, err := json.Marshal(operation.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s schema: %v", operation.Definition.Name, err)
		}

		var schema struct {
			Required   []string            `json:"required"`
			Properties map[string]struct{} `json:"properties"`
		}
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", operation.Definition.Name, err)
		}

		schemas[operation.Definition.Name] = make(map[string]bool, len(schema.Properties))
		for name := range schema.Properties {
			schemas[operation.Definition.Name][name] = true
		}
		required[operation.Definition.Name] = schema.Required
	}

	for name, want := range map[string][]string{
		"calendar_list":        {"account_id", "max_results", "page_token"},
		"calendar_list_events": {"account_id", "calendar_id", "time_min", "time_max", "max_results", "page_token", "query"},
		"calendar_freebusy":    {"account_id", "calendar_ids", "time_min", "time_max"},
	} {
		for _, field := range want {
			if !schemas[name][field] {
				t.Fatalf("%s schema is missing %s", name, field)
			}
		}
	}

	if got := required["calendar_list_events"]; !reflect.DeepEqual(got, []string{"account_id", "calendar_id", "time_min", "time_max"}) {
		t.Fatalf("calendar_list_events required fields: %#v", got)
	}
}

func TestCalendarListEventsPreservesAllDayExpandedRecurrenceInstances(t *testing.T) {
	t.Parallel()
	operation, provider := fixture(t, "calendar_list_events", `{
		"summary": "Work", "timeZone": "America/Chicago", "accessRole": "reader", "nextPageToken": "events-next",
		"items": [
			{
				"id": "all-day-instance", "summary": "Travel", "transparency": "opaque",
				"recurringEventId": "travel-series",
				"start": {"date": "2026-11-01", "timeZone": "America/Chicago"},
				"end": {"date": "2026-11-02", "timeZone": "America/Chicago"},
				"originalStartTime": {"date": "2026-11-01", "timeZone": "America/Chicago"}
			},
			{
				"id": "timed-instance", "summary": "Travel review",
				"recurringEventId": "review-series",
				"start": {"dateTime": "2026-11-01T09:00:00-05:00", "timeZone": "America/Chicago"},
				"end": {"dateTime": "2026-11-01T09:30:00-05:00", "timeZone": "America/Chicago"},
				"originalStartTime": {"dateTime": "2026-11-01T09:00:00-05:00", "timeZone": "America/Chicago"}
			}
		]
	}`)

	result, err := decodeRun(t, operation, `{
		"account_id":"opaque-account", "calendar_id":"work@example.test",
		"time_min":"2026-11-01T00:00:00-05:00", "time_max":"2026-11-02T00:00:00-06:00", "query":"travel"
	}`)
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[CalendarEventsData])
	if out.Data.TimeZone != "America/Chicago" || len(out.Data.Events) != 2 || out.NextPageToken != "events-next" {
		t.Fatalf("unexpected result: %#v", out)
	}

	first := out.Data.Events[0]
	if !first.AllDay || first.Start.Date != "2026-11-01" || first.Start.DateTime != "" ||
		first.End.Date != "2026-11-02" || first.RecurringEventID != "travel-series" ||
		first.OriginalStart == nil || first.OriginalStart.Date != "2026-11-01" || len(first.Recurrence) != 0 {
		t.Fatalf("all-day expanded instance was not preserved: %#v", first)
	}

	second := out.Data.Events[1]
	if second.RecurringEventID != "review-series" || second.OriginalStart == nil ||
		second.OriginalStart.DateTime != "2026-11-01T09:00:00-05:00" ||
		second.Start.DateTime != "2026-11-01T09:00:00-05:00" || second.Start.TimeZone != "America/Chicago" {
		t.Fatalf("timed expanded instance was not preserved: %#v", second)
	}

	req := provider.transport.requests[0]
	if req.URL.EscapedPath() != "/calendar/v3/calendars/work%40example.test/events" {
		t.Fatalf("events path = %q", req.URL.EscapedPath())
	}

	values := req.URL.Query()
	for key, want := range map[string]string{
		"maxResults": "25", "singleEvents": "true", "orderBy": "startTime", "q": "travel",
		"timeMin": "2026-11-01T00:00:00-05:00", "timeMax": "2026-11-02T00:00:00-06:00",
	} {
		if got := values.Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestCalendarFreeBusyReportsPartialFailuresWithoutCallingFailedCalendarFree(t *testing.T) {
	t.Parallel()
	operation, _ := fixture(t, "calendar_freebusy", `{
		"calendars": {
			"work@example.test": {"busy": [{"start": "2026-11-01T09:00:00-05:00", "end": "2026-11-01T09:30:00-05:00"}]},
			"broken@example.test": {"errors": [{"domain": "calendar", "reason": "notFound"}]}
		}
	}`)

	result, err := decodeRun(t, operation, `{
		"account_id":"opaque-account", "calendar_ids":["work@example.test","missing@example.test","broken@example.test"],
		"time_min":"2026-11-01T00:00:00-05:00", "time_max":"2026-11-02T00:00:00-06:00"
	}`)
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[FreeBusyData])
	if len(out.Data.Calendars) != 3 || len(out.PartialFailures) != 2 {
		t.Fatalf("unexpected result: %#v", out)
	}

	if out.Data.Calendars[0].Status != "ok" || len(out.Data.Calendars[0].Busy) != 1 {
		t.Fatalf("successful calendar was not preserved: %#v", out.Data.Calendars[0])
	}

	if out.Data.Calendars[1].Status != "error" || out.Data.Calendars[2].Status != "error" {
		t.Fatalf("failed calendars were marked successful: %#v", out.Data.Calendars)
	}

	if out.PartialFailures[1].Category != string(mcpcontract.NotFound) || out.PartialFailures[1].SourceID != "broken@example.test" {
		t.Fatalf("unexpected failure: %#v", out.PartialFailures[1])
	}
}

func TestCalendarValidationHappensBeforeProviderAcquisition(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"account_id":"a"}`,
		`{"account_id":"a","calendar_id":"work@example.test","time_min":"2026-11-01","time_max":"2026-11-02T00:00:00Z"}`,
		`{"account_id":"a","calendar_id":"work@example.test","time_min":"2026-11-02T00:00:00Z","time_max":"2026-11-01T00:00:00Z"}`,
		`{"account_id":"a","calendar_id":"work@example.test","time_min":"2024-01-01T00:00:00Z","time_max":"2027-01-01T00:00:00Z"}`,
		`{"account_id":"a","calendar_id":"work@example.test","time_min":"2026-11-01T00:00:00Z","time_max":"2026-11-02T00:00:00Z","max_results":251}`,
		`{"account_id":"a","calendar_ids":["a","a"],"time_min":"2026-11-01T00:00:00Z","time_max":"2026-11-02T00:00:00Z"}`,
	} {
		operation, provider := fixture(t, "calendar_list_events", `{}`)
		if _, err := decodeRun(t, operation, raw); err == nil {
			t.Fatalf("accepted invalid arguments %s", raw)
		} else if len(provider.options) != 0 || len(provider.transport.requests) != 0 {
			t.Fatalf("invalid arguments %s reached provider or upstream", raw)
		}
	}
}

func TestCalendarFreeBusyRequestIsBounded(t *testing.T) {
	t.Parallel()
	operation, provider := fixture(t, "calendar_freebusy", `{"calendars":{"a":{"busy":[]}}}`)

	_, err := decodeRun(t, operation, `{"account_id":"a","calendar_ids":["a"],"time_min":"2026-11-01T00:00:00-05:00","time_max":"2026-11-02T00:00:00-06:00"}`)
	if err != nil {
		t.Fatal(err)
	}

	var body struct {
		CalendarExpansionMax int64      `json:"calendarExpansionMax"`
		TimeMin              string     `json:"timeMin"`
		TimeMax              string     `json:"timeMax"`
		Items                []struct{} `json:"items"`
	}
	if err := json.Unmarshal([]byte(provider.transport.bodies[0]), &body); err != nil {
		t.Fatal(err)
	}

	if body.CalendarExpansionMax != 1 || len(body.Items) != 1 || body.TimeMin == "" || body.TimeMax == "" {
		t.Fatalf("unexpected free/busy request: %#v", body)
	}

	if escaped := url.QueryEscape("2026-11-01T00:00:00-05:00"); escaped == "" {
		t.Fatal("expected RFC3339 value")
	}
}
