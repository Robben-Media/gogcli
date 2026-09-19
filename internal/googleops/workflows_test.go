package googleops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	analyticsops "github.com/steipete/gogcli/internal/googleops/analytics"
	calendarops "github.com/steipete/gogcli/internal/googleops/calendar"
	docsops "github.com/steipete/gogcli/internal/googleops/docs"
	driveops "github.com/steipete/gogcli/internal/googleops/drive"
	gmailops "github.com/steipete/gogcli/internal/googleops/gmail"
	searchconsoleops "github.com/steipete/gogcli/internal/googleops/searchconsole"
	sheetsops "github.com/steipete/gogcli/internal/googleops/sheets"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

type workflowExample struct {
	workflow  string
	tool      string
	arguments string
}

var workflowExamples = []workflowExample{
	{workflow: "mail_thread", tool: "gmail_search", arguments: `{"account_id":"work-fixture","query":"from:acme@example.test subject:Acme","max_results":25}`},
	{workflow: "mail_thread", tool: "gmail_get_thread", arguments: `{"account_id":"work-fixture","thread_id":"thread-acme","max_messages":2,"max_body_bytes":1024}`},
	{workflow: "document_facts", tool: "drive_search", arguments: `{"account_id":"work-fixture","text":"Acme brief","max_results":25}`},
	{workflow: "document_facts", tool: "docs_get_text", arguments: `{"account_id":"work-fixture","document_id":"doc-acme","max_bytes":48}`},
	{workflow: "calendar_dst", tool: "calendar_list_events", arguments: `{"account_id":"work-fixture","calendar_id":"primary","time_min":"2026-11-01T00:00:00-05:00","time_max":"2026-11-02T00:00:00-06:00","max_results":25}`},
	{workflow: "calendar_dst", tool: "calendar_freebusy", arguments: `{"account_id":"work-fixture","calendar_ids":["primary","team","unavailable"],"time_min":"2026-11-01T00:00:00-05:00","time_max":"2026-11-02T00:00:00-06:00"}`},
	{workflow: "marketing_period", tool: "analytics_report", arguments: `{"account_id":"work-fixture","property":"properties/123","metrics":["sessions"],"dimensions":["date"],"start_date":"2026-09-01","end_date":"2026-09-07","limit":100}`},
	{workflow: "marketing_period", tool: "searchconsole_query", arguments: `{"account_id":"work-fixture","site_url":"sc-domain:robben.media","start_date":"2026-09-01","end_date":"2026-09-07","dimensions":["date"],"row_limit":100}`},
	{workflow: "sparse_sheet", tool: "sheets_read_range", arguments: `{"account_id":"work-fixture","spreadsheet_id":"sheet-client-budget","range":"'Client Budget'!A1:C4","major_dimension":"ROWS","value_render_option":"UNFORMATTED_VALUE"}`},
}

type workflowAccount struct {
	AccountID string `json:"account_id"`
	Email     string `json:"email"`
	Label     string `json:"label"`
}

type workflowCase struct {
	ID       string         `json:"id"`
	Expected map[string]any `json:"expected"`
}

type frozenWorkflows struct {
	Accounts  []workflowAccount `json:"accounts"`
	Workflows []workflowCase    `json:"workflows"`
}

var (
	errUnexpectedWorkflowRequest = errors.New("unexpected workflow HTTP request")
	errWorkflowResponseMissing   = errors.New("workflow fixture has no response")
)

type workflowTransport struct {
	responses map[string]json.RawMessage
}

func (transport *workflowTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	key, ok := workflowResponseKey(request)
	if !ok {
		return nil, fmt.Errorf("%w: %s %s", errUnexpectedWorkflowRequest, request.Method, request.URL.String())
	}

	response, exists := transport.responses[key]
	if !exists {
		return nil, fmt.Errorf("%w for %s", errWorkflowResponseMissing, key)
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("encode workflow response %s: %w", key, err)
	}

	return &http.Response{
		StatusCode: http.StatusOK, Status: "200 OK", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(bytes.NewReader(encoded)), ContentLength: int64(len(encoded)),
	}, nil
}

func workflowResponseKey(request *http.Request) (string, bool) {
	path := request.URL.Path
	switch {
	case path == "/gmail/v1/users/me/messages" && request.Method == http.MethodGet:
		return "gmail_search_list", true
	case path == "/gmail/v1/users/me/labels":
		return "gmail_labels", true
	case path == "/gmail/v1/users/me/messages/message-first":
		return "gmail_search_message", true
	case path == "/gmail/v1/users/me/threads/thread-acme":
		return "gmail_thread", true
	case path == "/drive/v3/files":
		return "drive_search", true
	case path == "/v1/documents/doc-acme":
		return "docs_get_text", true
	case path == "/calendar/v3/calendars/primary/events":
		return "calendar_list_events", true
	case path == "/calendar/v3/freeBusy":
		return "calendar_freebusy", true
	case path == "/v1beta/properties/123:runReport":
		return "analytics_report", true
	case strings.HasPrefix(path, "/webmasters/v3/sites/") && strings.HasSuffix(path, "/searchAnalytics/query"):
		return "searchconsole_query", true
	case strings.HasPrefix(path, "/v4/spreadsheets/sheet-client-budget/values/"):
		return "sheets_read_range", true
	default:
		return "", false
	}
}

type workflowProvider struct {
	transport  *workflowTransport
	options    []mcpcontract.CallOptions
	identities []mcpcontract.Identity
}

func (provider *workflowProvider) HTTPClient(_ context.Context, identity mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	provider.options = append(provider.options, options)
	provider.identities = append(provider.identities, identity)

	return &http.Client{Transport: provider.transport}, nil
}

type workflowFixture struct {
	provider   *workflowProvider
	operations map[string]mcpcontract.Operation
	identity   mcpcontract.Identity
	frozen     frozenWorkflows
}

func newWorkflowFixture(t *testing.T) *workflowFixture {
	t.Helper()
	var frozen frozenWorkflows

	frozenData, err := os.ReadFile(filepath.Join("testdata", "..", "..", "mcpcontract", "testdata", "workflows.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err = json.Unmarshal(frozenData, &frozen); err != nil {
		t.Fatal(err)
	}

	identities := make(map[string]mcpcontract.Identity, len(frozen.Accounts))
	for _, account := range frozen.Accounts {
		identities[account.AccountID] = mcpcontract.Identity{
			AccountID: account.AccountID, Subject: "subject-" + account.AccountID, Email: account.Email,
			Label: account.Label, PrincipalID: "fixture-principal", ClientName: "fixture-client",
			AuthMode: "oauth", Generation: 1,
		}
	}

	identity, ok := identities["work-fixture"]
	if !ok {
		t.Fatal("work-fixture identity is missing")
	}
	var responses map[string]json.RawMessage

	responseData, err := os.ReadFile(filepath.Join("testdata", "workflow-responses.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err = json.Unmarshal(responseData, &responses); err != nil {
		t.Fatal(err)
	}
	provider := &workflowProvider{transport: &workflowTransport{responses: responses}}

	operations := make(map[string]mcpcontract.Operation)
	for _, operation := range Operations(provider) {
		operations[operation.Definition.Name] = operation
	}

	if len(operations) != 16 {
		t.Fatalf("got %d operations, want 16", len(operations))
	}

	return &workflowFixture{provider: provider, operations: operations, identity: identity, frozen: frozen}
}

func (fixture *workflowFixture) expected(t *testing.T, workflow, name string) any {
	t.Helper()

	for _, candidate := range fixture.frozen.Workflows {
		if candidate.ID == workflow {
			return candidate.Expected[name]
		}
	}

	t.Fatalf("frozen workflow %s is missing", workflow)

	return nil
}

func runWorkflow[T any](t *testing.T, fixture *workflowFixture, workflow, tool string) mcpcontract.Result[T] {
	t.Helper()
	var arguments string

	for _, example := range workflowExamples {
		if example.workflow == workflow && example.tool == tool {
			arguments = example.arguments
			break
		}
	}

	if arguments == "" {
		t.Fatalf("no executable example for %s/%s", workflow, tool)
	}

	operation, ok := fixture.operations[tool]
	if !ok {
		t.Fatalf("operation %s is missing", tool)
	}

	call, err := operation.Decode(json.RawMessage(arguments))
	if err != nil {
		t.Fatalf("decode %s: %v", tool, err)
	}

	if call.AccountID != fixture.identity.AccountID {
		t.Fatalf("%s selected account %q, want %q", tool, call.AccountID, fixture.identity.AccountID)
	}
	before := len(fixture.provider.identities)

	resultAny, err := call.Run(context.Background(), fixture.identity)
	if err != nil {
		t.Fatalf("run %s: %v", tool, err)
	}

	result, ok := resultAny.(mcpcontract.Result[T])
	if !ok {
		t.Fatalf("%s returned %T", tool, resultAny)
	}

	if result.AccountID != fixture.identity.AccountID || result.AccountLabel != fixture.identity.Label {
		t.Fatalf("%s account envelope = %q/%q", tool, result.AccountID, result.AccountLabel)
	}

	after := len(fixture.provider.identities)
	if after != before+1 {
		t.Fatalf("%s acquired %d clients", tool, after-before)
	}

	if fixture.provider.identities[before].AccountID != fixture.identity.AccountID {
		t.Fatalf("%s propagated account %q", tool, fixture.provider.identities[before].AccountID)
	}

	return result
}

func assertProviderWorkflow(t *testing.T, fixture *workflowFixture, tools ...string) {
	t.Helper()

	if len(fixture.provider.options) != len(tools) {
		t.Fatalf("provider calls = %#v", fixture.provider.options)
	}

	for index, tool := range tools {
		if fixture.provider.options[index].Operation != tool || fixture.provider.options[index].Retry != mcpcontract.SafeRead {
			t.Fatalf("provider call %d = %#v, want %s", index, fixture.provider.options[index], tool)
		}
	}
}

func TestWorkflowMailSearchToThread(t *testing.T) {
	t.Parallel()
	fixture := newWorkflowFixture(t)

	search := runWorkflow[gmailops.SearchData](t, fixture, "mail_thread", "gmail_search")
	if len(search.Data.Messages) != 1 || search.Data.Messages[0].ID != "message-first" ||
		search.Data.Messages[0].ThreadID != fixture.expected(t, "mail_thread", "thread_id") {
		t.Fatalf("search result = %#v", search.Data)
	}
	thread := runWorkflow[gmailops.ThreadView](t, fixture, "mail_thread", "gmail_get_thread")

	wantOrder := fixture.expected(t, "mail_thread", "message_order").([]any)
	if thread.Data.ID != fixture.expected(t, "mail_thread", "thread_id") || len(thread.Data.Messages) != len(wantOrder) {
		t.Fatalf("thread result = %#v", thread.Data)
	}
	var combined strings.Builder

	for index, want := range wantOrder {
		if thread.Data.Messages[index].ID != want {
			t.Fatalf("message %d = %q, want %q", index, thread.Data.Messages[index].ID, want)
		}

		combined.WriteString(thread.Data.Messages[index].Body)
		combined.WriteString("\n")
	}

	for _, fact := range fixture.expected(t, "mail_thread", "facts").([]any) {
		if !strings.Contains(combined.String(), fact.(string)) {
			t.Fatalf("thread bodies omit frozen fact %q: %q", fact, combined.String())
		}
	}

	assertProviderWorkflow(t, fixture, "gmail_search", "gmail_get_thread")
}

func TestWorkflowDriveSearchToDocumentFacts(t *testing.T) {
	t.Parallel()
	fixture := newWorkflowFixture(t)

	search := runWorkflow[driveops.SearchData](t, fixture, "document_facts", "drive_search")
	if len(search.Data.Files) != 1 || search.Data.Files[0].ID != fixture.expected(t, "document_facts", "document_id") ||
		search.Data.Files[0].WebViewLink == "" {
		t.Fatalf("document source = %#v", search.Data)
	}

	document := runWorkflow[docsops.GetTextData](t, fixture, "document_facts", "docs_get_text")
	if document.Data.DocumentID != fixture.expected(t, "document_facts", "document_id") ||
		document.Data.URL == "" || !strings.Contains(document.Data.Text, fixture.expected(t, "document_facts", "launch_date").(string)) {
		t.Fatalf("document result = %#v", document.Data)
	}

	if !document.Truncated {
		t.Fatalf("bounded document read was not marked truncated: %#v", document)
	}

	assertProviderWorkflow(t, fixture, "drive_search", "docs_get_text")
}

func TestWorkflowCalendarDSTAvailability(t *testing.T) {
	t.Parallel()
	fixture := newWorkflowFixture(t)

	events := runWorkflow[calendarops.CalendarEventsData](t, fixture, "calendar_dst", "calendar_list_events")
	if len(events.Data.Events) != 1 || !events.Data.Events[0].AllDay || events.Data.Events[0].RecurringEventID == "" ||
		events.Data.Events[0].OriginalStart == nil {
		t.Fatalf("expanded recurring events = %#v", events.Data.Events)
	}
	freeBusy := runWorkflow[calendarops.FreeBusyData](t, fixture, "calendar_dst", "calendar_freebusy")

	start, err := time.Parse(time.RFC3339, freeBusy.Data.TimeMin)
	if err != nil {
		t.Fatal(err)
	}

	end, err := time.Parse(time.RFC3339, freeBusy.Data.TimeMax)
	if err != nil {
		t.Fatal(err)
	}

	wantHours := fixture.expected(t, "calendar_dst", "duration_hours").(float64)
	if end.Sub(start).Hours() != wantHours {
		t.Fatalf("interval = %v hours, want %v", end.Sub(start).Hours(), wantHours)
	}
	var failed *calendarops.FreeBusyCalendar

	for index := range freeBusy.Data.Calendars {
		calendar := &freeBusy.Data.Calendars[index]
		if calendar.CalendarID == "unavailable" {
			failed = calendar
		}
	}

	if failed == nil || failed.Status != "error" || len(failed.Busy) != 0 || len(freeBusy.PartialFailures) != 1 ||
		freeBusy.PartialFailures[0].SourceID != "unavailable" {
		t.Fatalf("failed calendar was treated as available: %#v %#v", failed, freeBusy.PartialFailures)
	}

	if len(freeBusy.Data.Calendars[0].Busy) == 0 {
		t.Fatal("all-day busy interval was omitted")
	}

	assertProviderWorkflow(t, fixture, "calendar_list_events", "calendar_freebusy")
}

func TestWorkflowMarketingPeriod(t *testing.T) {
	t.Parallel()
	fixture := newWorkflowFixture(t)

	report := runWorkflow[analyticsops.ReportData](t, fixture, "marketing_period", "analytics_report")
	if report.Data.TimeZone != fixture.expected(t, "marketing_period", "ga4_timezone") ||
		report.Data.StartDate != fixture.expected(t, "marketing_period", "start_date") ||
		report.Data.EndDate != fixture.expected(t, "marketing_period", "end_date") || len(report.Data.Rows) != 1 {
		t.Fatalf("GA4 report = %#v", report.Data)
	}

	sessions, err := strconv.ParseFloat(report.Data.Rows[0].Metrics[0], 64)
	if err != nil {
		t.Fatal(err)
	}

	if sessions != fixture.expected(t, "marketing_period", "sessions").(float64) {
		t.Fatalf("sessions = %v", sessions)
	}

	query := runWorkflow[searchconsoleops.QueryData](t, fixture, "marketing_period", "searchconsole_query")
	if query.Data.TimeZone != fixture.expected(t, "marketing_period", "search_console_timezone") ||
		query.Data.StartDate != fixture.expected(t, "marketing_period", "start_date") ||
		query.Data.EndDate != fixture.expected(t, "marketing_period", "end_date") {
		t.Fatalf("Search Console query = %#v", query.Data)
	}

	clicks := 0.0
	for _, row := range query.Data.Rows {
		clicks += row.Clicks
	}

	if clicks != fixture.expected(t, "marketing_period", "clicks").(float64) {
		t.Fatalf("clicks = %v", clicks)
	}

	assertProviderWorkflow(t, fixture, "analytics_report", "searchconsole_query")
}

func TestWorkflowSparseSheetSum(t *testing.T) {
	t.Parallel()
	fixture := newWorkflowFixture(t)

	sheet := runWorkflow[sheetsops.ReadRangeData](t, fixture, "sparse_sheet", "sheets_read_range")
	if sheet.Data.RequestedRange != fixture.expected(t, "sparse_sheet", "range").(string) {
		t.Fatalf("range = %q", sheet.Data.RequestedRange)
	}
	expectedRows := fixture.expected(t, "sparse_sheet", "rows").([]any)

	actualRows := make([][]any, 0, len(sheet.Data.Rows))
	for _, row := range sheet.Data.Rows {
		if row == nil {
			row = []any{}
		}
		actualRows = append(actualRows, row)
	}

	rows := make([][]any, 0, len(expectedRows))
	for _, expectedRow := range expectedRows {
		row := expectedRow.([]any)
		if row == nil {
			row = []any{}
		}
		rows = append(rows, row)
	}

	if !reflect.DeepEqual(actualRows, rows) {
		t.Fatalf("sparse rows = %#v, want %#v", actualRows, rows)
	}

	if sheet.Data.Rows[1][1] != "" || !reflect.DeepEqual(sheet.Data.Rows[2], []any{nil, nil, nil}) {
		t.Fatalf("blank values were not preserved: %#v", sheet.Data.Rows)
	}
	sum := 0.0

	for _, row := range sheet.Data.Rows {
		if len(row) > 2 {
			if value, ok := row[2].(float64); ok {
				sum += value
			}
		}
	}

	if sum != fixture.expected(t, "sparse_sheet", "sum").(float64) {
		t.Fatalf("sum = %v", sum)
	}

	assertProviderWorkflow(t, fixture, "sheets_read_range")
}
