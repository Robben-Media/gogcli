package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googleops/authoring"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

type authoringMeter struct {
	fixtureCalls atomic.Int64
	mcpCalls     atomic.Int64
	jsonBytes    atomic.Int64

	mu        sync.Mutex
	durations []int64
}

func (m *authoringMeter) record(start time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.durations = append(m.durations, time.Since(start).Nanoseconds())
}

func (m *authoringMeter) reset() {
	m.fixtureCalls.Store(0)
	m.mcpCalls.Store(0)
	m.jsonBytes.Store(0)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.durations = nil
}

func (m *authoringMeter) percentile(fraction float64) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.durations) == 0 {
		return 0
	}

	ordered := append([]int64(nil), m.durations...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return float64(ordered[int(fraction*float64(len(ordered)-1))])
}

func (m *authoringMeter) report(b *testing.B) {
	b.Helper()
	if b.N == 0 {
		return
	}

	count := float64(b.N)
	b.ReportMetric(m.percentile(.5), "p50_ns")
	b.ReportMetric(m.percentile(.95), "p95_ns")
	b.ReportMetric(float64(m.fixtureCalls.Load())/count, "fixture_attempts/op")
	b.ReportMetric(float64(m.mcpCalls.Load())/count, "mcp_tool_calls/op")
	b.ReportMetric(float64(m.jsonBytes.Load())/count, "structured_json_bytes/op")
}

func (m *authoringMeter) recordResult(result *mcp.CallToolResult) error {
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return fmt.Errorf("marshal authoring result: %w", err)
	}
	m.jsonBytes.Add(int64(len(data)))
	return nil
}

type authoringFixtureCall struct {
	method string
	path   string
	body   string
}

type authoringFixtureTransport struct {
	calls atomic.Int64
	meter *authoringMeter

	mu     sync.Mutex
	record []authoringFixtureCall
}

func (t *authoringFixtureTransport) resetCalls() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record = nil
}

func (t *authoringFixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	if t.meter != nil {
		t.meter.fixtureCalls.Add(1)
	}

	var bodyBytes []byte
	if req.GetBody != nil {
		bodyReader, bodyErr := req.GetBody()
		if bodyErr != nil {
			return nil, fmt.Errorf("open authoring request body: %w", bodyErr)
		}
		var readErr error
		bodyBytes, readErr = io.ReadAll(bodyReader)
		_ = bodyReader.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read authoring request body: %w", readErr)
		}
	} else if req.Body != nil {
		var readErr error
		bodyBytes, readErr = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read authoring request body: %w", readErr)
		}
		req.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}
	call := authoringFixtureCall{method: req.Method, path: req.URL.Path, body: string(bodyBytes)}
	t.mu.Lock()
	t.record = append(t.record, call)
	t.mu.Unlock()

	valid := req.Method == http.MethodPost && ((strings.HasSuffix(call.path, "/documents") && !strings.Contains(call.path, ":")) ||
		(strings.Contains(call.path, "/documents/") && strings.HasSuffix(call.path, ":batchUpdate")) ||
		strings.HasSuffix(call.path, "/presentations") ||
		(strings.Contains(call.path, "/presentations/") && strings.HasSuffix(call.path, ":batchUpdate")) ||
		strings.HasSuffix(call.path, "/spreadsheets"))
	if !valid {
		return nil, &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "unexpected authoring fixture request", Retryable: false}
	}

	body := `{}`

	path := call.path
	switch {
	case strings.HasSuffix(path, ":batchUpdate"):
	case strings.HasSuffix(path, "/documents"):
		body = `{"documentId":"doc-fixture","title":"Authoring Fixture"}`
	case strings.HasSuffix(path, "/presentations"):
		body = `{"presentationId":"slides-fixture","title":"Authoring Fixture","slides":[{"objectId":"starter-slide"}]}`
	case strings.HasSuffix(path, "/spreadsheets"):
		body = `{"spreadsheetId":"sheets-fixture","spreadsheetUrl":"https://sheets.example.test/fixture","properties":{"title":"Authoring Fixture"}}`
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

type authoringFixtureProvider struct{ base *authoringFixtureTransport }

func (p authoringFixtureProvider) HTTPClient(_ context.Context, _ mcpcontract.Identity, opts mcpcontract.CallOptions) (*http.Client, error) {
	return &http.Client{Transport: &googleapi.NativeRetryTransport{Base: p.base, Class: opts.Retry}}, nil
}

func authoringIdentity() mcpcontract.Identity {
	return mcpcontract.Identity{
		AccountID:   "authoring-fixture",
		Subject:     "subject-authoring-fixture",
		Email:       "authoring@example.test",
		Label:       "Authoring",
		PrincipalID: "fixture",
		ClientName:  "app",
		AuthMode:    "oauth",
		Scopes: []string{
			mcpcontract.DocsWriteScope,
			mcpcontract.SlidesWriteScope,
			mcpcontract.SheetsWriteScope,
		},
		Generation: 1,
	}
}

func authoringSession(tb testing.TB, provider mcpcontract.ClientProvider, maxUpstreamCalls int64) *mcp.ClientSession {
	tb.Helper()

	id := authoringIdentity()
	runtime, err := mcpserver.New(mcpserver.Config{
		Principal: mcpcontract.Principal{ID: "fixture"},
		Grants: []mcpcontract.Grant{{
			PrincipalID: "fixture",
			AccountIDs:  []string{id.AccountID},
			ClientNames: []string{id.ClientName},
			Operations:  []string{"docs:workflow.create", "slides:workflow.create", "sheets:workflow.create"},
		}},
		AllowOperations: []string{
			"accounts_list",
			"docs_create_document",
			"slides_create_presentation",
			"sheets_create_spreadsheet",
		},
		Operations:       authoring.Operations(provider),
		Accounts:         authoringMemoryAccounts{identity: id},
		DiscoveryMode:    mcpserver.DiscoveryCompact,
		EnableWrites:     true,
		MaxUpstreamCalls: maxUpstreamCalls,
		RequestTimeout:   5 * time.Second,
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		tb.Fatal(err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := runtime.Server().Connect(context.Background(), serverTransport, nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "authoring-workflow-fixture", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = session.Close() })

	return session
}

func callAuthoringTool(meter *authoringMeter, session *mcp.ClientSession, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	meter.mcpCalls.Add(1)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", name, err)
	}
	return result, nil
}

func authoringWorkflowCases() map[string]struct {
	query     string
	created   string
	arguments map[string]any
} {
	return map[string]struct {
		query     string
		created   string
		arguments map[string]any
	}{
		"docs": {
			query:   "create document",
			created: "doc-fixture",
			arguments: map[string]any{
				"account_id": "authoring-fixture",
				"title":      "Authoring Fixture",
				"paragraphs": []any{map[string]any{"text": "Fixture paragraph"}},
			},
		},
		"slides": {
			query:   "create presentation",
			created: "slides-fixture",
			arguments: map[string]any{
				"account_id": "authoring-fixture",
				"title":      "Authoring Fixture",
				"slides":     []any{map[string]any{"title": "Title", "body": []any{"Body"}}},
			},
		},
		"sheets": {
			query:   "create spreadsheet",
			created: "sheets-fixture",
			arguments: map[string]any{
				"account_id":  "authoring-fixture",
				"title":       "Authoring Fixture",
				"sheet_title": "Data",
				"header":      true,
				"rows":        []any{[]any{map[string]any{"text": "Header"}, map[string]any{"number": 2}}},
			},
		},
	}
}

func authoringCompactWorkflow(tb testing.TB, meter *authoringMeter, session *mcp.ClientSession, query, operation string, arguments map[string]any, validateResult bool) mcpcontract.Result[authoring.Created] {
	tb.Helper()

	search, err := callAuthoringTool(meter, session, "capabilities_search", map[string]any{"query": query, "limit": 5})
	if err != nil || search.IsError {
		tb.Fatalf("search err=%v result=%+v", err, search)
	}
	if searchRecordErr := meter.recordResult(search); searchRecordErr != nil {
		tb.Fatal(searchRecordErr)
	}

	described, err := callAuthoringTool(meter, session, "capabilities_describe", map[string]any{"name": operation})
	if err != nil || described.IsError {
		tb.Fatalf("describe err=%v result=%+v", err, described)
	}
	if describeRecordErr := meter.recordResult(described); describeRecordErr != nil {
		tb.Fatal(describeRecordErr)
	}

	executed, err := callAuthoringTool(meter, session, "capabilities_execute", map[string]any{
		"name": operation, "arguments": arguments,
	})
	if err != nil || executed.IsError {
		tb.Fatalf("execute err=%v result=%+v", err, executed)
	}
	if executeRecordErr := meter.recordResult(executed); executeRecordErr != nil {
		tb.Fatal(executeRecordErr)
	}

	var envelope mcpcontract.Result[authoring.Created]
	if !validateResult {
		return envelope
	}
	if err := json.Unmarshal(mustAuthoringJSON(tb, executed.StructuredContent), &envelope); err != nil {
		tb.Fatal(err)
	}
	return envelope
}

func mustAuthoringJSON(tb testing.TB, value any) []byte {
	tb.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		tb.Fatal(err)
	}
	return data
}

func TestAuthoringWorkflowFixtureOutputsAndCalls(t *testing.T) {
	meter := &authoringMeter{}
	base := &authoringFixtureTransport{meter: meter}
	session := authoringSession(t, authoringFixtureProvider{base: base}, 64)

	createdByOperation := map[string]string{
		"docs_create_document":       "doc-fixture",
		"slides_create_presentation": "slides-fixture",
		"sheets_create_spreadsheet":  "sheets-fixture",
	}
	expectedCalls := map[string]int64{
		"docs_create_document":       2,
		"slides_create_presentation": 2,
		"sheets_create_spreadsheet":  1,
	}
	queries := map[string]string{
		"docs_create_document":       "create document",
		"slides_create_presentation": "create presentation",
		"sheets_create_spreadsheet":  "create spreadsheet",
	}

	for operation, created := range createdByOperation {
		key := strings.Split(operation, "_")[0]
		base.calls.Store(0)
		base.resetCalls()
		meter.reset()
		envelope := authoringCompactWorkflow(t, meter, session, queries[operation], operation, authoringWorkflowArguments()[key], true)
		if envelope.Data.ID != created || !envelope.Data.Populated || envelope.Data.Title != "Authoring Fixture" {
			t.Fatalf("%s created = %#v", operation, envelope.Data)
		}
		if base.calls.Load() != expectedCalls[operation] || meter.fixtureCalls.Load() != expectedCalls[operation] || meter.mcpCalls.Load() != 3 {
			t.Fatalf("%s calls transport=%d meter=%d mcp=%d", operation, base.calls.Load(), meter.fixtureCalls.Load(), meter.mcpCalls.Load())
		}
		assertAuthoringFixtureCalls(t, base, key)
	}
}

func TestAuthoringWorkflowBudgetPreventsDocumentCreate(t *testing.T) {
	meter := &authoringMeter{}
	base := &authoringFixtureTransport{meter: meter}
	session := authoringSession(t, authoringFixtureProvider{base: base}, 1)

	result, err := callAuthoringTool(meter, session, "capabilities_execute", map[string]any{
		"name": "docs_create_document", "arguments": authoringWorkflowArguments()["docs"],
	})
	if err != nil || !result.IsError {
		t.Fatalf("budget result err=%v result=%+v", err, result)
	}

	var failure mcpcontract.Error
	if err := json.Unmarshal(mustAuthoringJSON(t, result.StructuredContent), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Category != mcpcontract.BudgetExhausted || base.calls.Load() != 0 {
		t.Fatalf("category=%q calls=%d error=%+v", failure.Category, base.calls.Load(), failure)
	}
}

func assertAuthoringFixtureCalls(t *testing.T, base *authoringFixtureTransport, workflow string) {
	t.Helper()

	base.mu.Lock()
	calls := append([]authoringFixtureCall(nil), base.record...)
	base.mu.Unlock()

	expected := map[string][]authoringFixtureCall{
		"docs": {
			{method: http.MethodPost, path: "/v1/documents", body: `{"title":"Authoring Fixture"}`},
			{method: http.MethodPost, path: "/v1/documents/doc-fixture:batchUpdate", body: "Fixture paragraph"},
		},
		"slides": {
			{method: http.MethodPost, path: "/v1/presentations", body: `{"title":"Authoring Fixture"}`},
			{method: http.MethodPost, path: "/v1/presentations/slides-fixture:batchUpdate", body: "title_001"},
		},
		"sheets": {
			{method: http.MethodPost, path: "/v4/spreadsheets", body: "Header"},
		},
	}
	want := expected[workflow]
	if len(calls) != len(want) {
		t.Fatalf("%s fixture calls = %#v, want %d", workflow, calls, len(want))
	}
	for index, call := range want {
		if calls[index].method != call.method || calls[index].path != call.path {
			t.Fatalf("%s fixture call %d = %s %s, want %s %s", workflow, index, calls[index].method, calls[index].path, call.method, call.path)
		}
		if !strings.Contains(calls[index].body, call.body) {
			t.Fatalf("%s fixture call %d body = %s, want substring %s", workflow, index, calls[index].body, call.body)
		}
	}
	if workflow == "sheets" && !strings.Contains(calls[0].body, `"frozenRowCount":1`) {
		t.Fatalf("sheets fixture body missing frozen header: %s", calls[0].body)
	}
}

func authoringWorkflowArguments() map[string]map[string]any {
	arguments := make(map[string]map[string]any)
	for name, workflow := range authoringWorkflowCases() {
		arguments[name] = workflow.arguments
	}
	return arguments
}

func BenchmarkAuthoringWorkflowFixture(b *testing.B) {
	meter := &authoringMeter{}
	base := &authoringFixtureTransport{meter: meter}
	session := authoringSession(b, authoringFixtureProvider{base: base}, 64)

	type workflow struct {
		name      string
		operation string
		query     string
		arguments map[string]any
		calls     int64
	}

	workflows := []workflow{
		{name: "docs_create", operation: "docs_create_document", query: "create document", arguments: authoringWorkflowArguments()["docs"], calls: 2},
		{name: "slides_create", operation: "slides_create_presentation", query: "create presentation", arguments: authoringWorkflowArguments()["slides"], calls: 2},
		{name: "sheets_create", operation: "sheets_create_spreadsheet", query: "create spreadsheet", arguments: authoringWorkflowArguments()["sheets"], calls: 1},
	}

	for _, item := range workflows {
		envelope := authoringCompactWorkflow(b, meter, session, item.query, item.operation, item.arguments, true)
		if envelope.Data.ID == "" || !envelope.Data.Populated {
			b.Fatalf("%s warmup created=%#v", item.name, envelope.Data)
		}
	}

	for _, item := range workflows {
		b.Run(item.name, func(b *testing.B) {
			meter.reset()
			base.calls.Store(0)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				start := time.Now()
				_ = authoringCompactWorkflow(b, meter, session, item.query, item.operation, item.arguments, false)
				meter.record(start)
			}
			b.StopTimer()
			expectedCalls := item.calls * int64(b.N)
			if base.calls.Load() != expectedCalls || meter.fixtureCalls.Load() != expectedCalls || meter.mcpCalls.Load() != 3*int64(b.N) {
				b.Fatalf("calls transport=%d meter=%d mcp=%d want fixture=%d mcp=%d", base.calls.Load(), meter.fixtureCalls.Load(), meter.mcpCalls.Load(), expectedCalls, 3*int64(b.N))
			}
			meter.report(b)
		})
	}
}

type authoringMemoryAccounts struct{ identity mcpcontract.Identity }

func (a authoringMemoryAccounts) Get(context.Context, string) (mcpcontract.Identity, bool, error) {
	if a.identity.AccountID == "" {
		return mcpcontract.Identity{}, false, nil
	}
	return a.identity.Clone(), true, nil
}

func (a authoringMemoryAccounts) List(_ context.Context, principalID string) ([]mcpcontract.Identity, error) {
	if principalID != "" && a.identity.PrincipalID != principalID {
		return []mcpcontract.Identity{}, nil
	}
	return []mcpcontract.Identity{a.identity.Clone()}, nil
}
