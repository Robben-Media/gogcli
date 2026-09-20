package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googleops"
	"github.com/steipete/gogcli/internal/googleops/apiexec"
	"github.com/steipete/gogcli/internal/googleops/authoring"
	nativegmail "github.com/steipete/gogcli/internal/googleops/gmail"
	"github.com/steipete/gogcli/internal/googleops/mailworkflow"
	"github.com/steipete/gogcli/internal/googleops/media"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
	"github.com/steipete/gogcli/internal/mediaartifact"
	"github.com/steipete/gogcli/internal/outfmt"
)

const (
	workflowFixtureAccount  = "work-fixture"
	workflowDeniedAccount   = "denied-fixture"
	workflowUnknownAccount  = "unknown-fixture"
	workflowFixtureQuery    = "Acme"
	workflowFixtureMax      = 25
	workflowFixtureMaxCalls = 64
)

var workflowFixtureSubjects = []string{"Review is Friday", "Budget is 2500 USD"}

// workflowMeter measures fixture HTTP attempts, MCP tool calls, and
// structured-content JSON returned by the selected workflow. The byte metric
// excludes duplicate MCP TextContent and SDK result metadata. Attempts are not
// Google quota units.
type workflowMeter struct {
	fixtureCalls atomic.Int64
	mcpCalls     atomic.Int64
	jsonBytes    atomic.Int64

	mu        sync.Mutex
	durations []int64
}

func (m *workflowMeter) recordDuration(start time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.durations = append(m.durations, time.Since(start).Nanoseconds())
}

func (m *workflowMeter) reset() {
	m.fixtureCalls.Store(0)
	m.mcpCalls.Store(0)
	m.jsonBytes.Store(0)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.durations = nil
}

func (m *workflowMeter) percentile(fraction float64) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.durations) == 0 {
		return 0
	}

	ordered := append([]int64(nil), m.durations...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := int(fraction * float64(len(ordered)-1))
	return float64(ordered[index])
}

func (m *workflowMeter) report(b *testing.B) {
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

func (m *workflowMeter) recordResult(result *mcp.CallToolResult) error {
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return err
	}
	m.jsonBytes.Add(int64(len(data)))
	return nil
}

func (m *workflowMeter) resultWriter() io.Writer {
	return measuredWriter{meter: m}
}

type measuredWriter struct{ meter *workflowMeter }

func (w measuredWriter) Write(p []byte) (int, error) {
	w.meter.jsonBytes.Add(int64(len(p)))
	return len(p), nil
}

type meteredFixtureTransport struct {
	base  *nativeBenchmarkTransport
	meter *workflowMeter
}

func (t *meteredFixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.meter.fixtureCalls.Add(1)
	return t.base.RoundTrip(req)
}

type validatedFixtureTransport struct{ inner http.RoundTripper }

func (t *validatedFixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.Method != http.MethodGet || req.URL == nil ||
		req.URL.Scheme != "https" || req.URL.Host != "gmail.googleapis.com" {
		return nil, &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "unexpected Gmail fixture request", Retryable: false}
	}

	path := req.URL.Path
	valid := strings.HasSuffix(path, "/messages") || strings.HasSuffix(path, "/labels") ||
		strings.HasSuffix(path, "/message-first") || strings.HasSuffix(path, "/message-second")
	if !valid {
		return nil, &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "unexpected Gmail fixture endpoint", Retryable: false}
	}

	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil || len(bytes.TrimSpace(body)) != 0 {
			return nil, &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "Gmail GET fixture request has a body", Retryable: false}
		}
		req.Body = http.NoBody
	}

	return t.inner.RoundTrip(req)
}

func workflowIdentity() mcpcontract.Identity {
	return mcpcontract.Identity{
		AccountID:   workflowFixtureAccount,
		Subject:     "subject-work-fixture",
		Email:       "work@example.test",
		Label:       "Work",
		PrincipalID: "fixture",
		ClientName:  "app",
		AuthMode:    "oauth",
		Scopes:      []string{mcpcontract.GmailReadScope},
		Generation:  1,
	}
}

func workflowOperations(provider mcpcontract.ClientProvider, fullCatalog bool) []mcpcontract.Operation {
	if !fullCatalog {
		return nativegmail.Operations(provider)
	}

	// Match the runtime registry selected by gog-mcp --api-catalog. Only the
	// Gmail operation is granted below, so compact discovery still scans and
	// filters the full opt-in registry before returning the same Gmail facts.
	artifacts := mediaartifact.New()
	operations := googleops.Operations(provider)
	operations = append(operations, media.OperationsWithArtifacts(provider, artifacts)...)
	operations = append(operations, authoring.Operations(provider)...)
	operations = append(operations, mailworkflow.Operations(provider)...)
	operations = append(operations, apiexec.Operations(provider)...)

	return operations
}

func workflowRuntime(tb testing.TB, provider mcpcontract.ClientProvider, mode mcpserver.DiscoveryMode) *mcp.ClientSession {
	tb.Helper()
	return workflowRuntimeCatalog(tb, provider, mode, workflowFixtureMaxCalls, false)
}

func workflowRuntimeCatalog(tb testing.TB, provider mcpcontract.ClientProvider, mode mcpserver.DiscoveryMode, maxUpstreamCalls int64, fullCatalog bool) *mcp.ClientSession {
	tb.Helper()
	id := workflowIdentity()
	denied := id
	denied.AccountID = workflowDeniedAccount
	denied.Subject = "subject-denied-fixture"
	denied.PrincipalID = "other-principal"

	runtime, err := mcpserver.New(mcpserver.Config{
		Principal: mcpcontract.Principal{ID: "fixture"},
		Grants: []mcpcontract.Grant{{
			PrincipalID: "fixture",
			AccountIDs:  []string{workflowFixtureAccount},
			ClientNames: []string{"app"},
			Operations:  []string{"gmail:messages.search"},
		}},
		AllowOperations:  []string{"accounts_list", "gmail_search"},
		Operations:       workflowOperations(provider, fullCatalog),
		Accounts:         access.NewMemoryAccounts(id, denied),
		DiscoveryMode:    mode,
		EnableWrites:     false,
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

	client := mcp.NewClient(&mcp.Implementation{Name: "workflow-fixture", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = session.Close() })

	return session
}

func callWorkflowTool(meter *workflowMeter, session *mcp.ClientSession, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	meter.mcpCalls.Add(1)
	return session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
}

func workflowGmailEnvelope(tb testing.TB, result *mcp.CallToolResult) {
	tb.Helper()

	var envelope mcpcontract.Result[nativegmail.SearchData]
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		tb.Fatal(err)
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		tb.Fatalf("decode Gmail MCP result %s: %v", data, err)
	}

	if len(envelope.Data.Messages) != len(workflowFixtureSubjects) {
		tb.Fatalf("Gmail facts = %#v", envelope.Data.Messages)
	}
	for index, subject := range workflowFixtureSubjects {
		if envelope.Data.Messages[index].Subject != subject {
			tb.Fatalf("message %d subject = %q, want %q", index, envelope.Data.Messages[index].Subject, subject)
		}
	}
	if envelope.AccountID != workflowFixtureAccount || envelope.Truncated {
		tb.Fatalf("unexpected Gmail envelope: account=%q truncated=%v", envelope.AccountID, envelope.Truncated)
	}
}

func workflowCLIFacts(tb testing.TB, ctx context.Context, meter *workflowMeter) {
	tb.Helper()

	sink := &bytes.Buffer{}
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		tb.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writePipe
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(sink, readPipe)
		close(done)
	}()

	command := GmailMessagesSearchCmd{Query: []string{workflowFixtureQuery}, Max: workflowFixtureMax, Timezone: "UTC"}
	runErr := command.Run(ctx, &RootFlags{Account: workflowIdentity().Email})
	os.Stdout = original
	_ = writePipe.Close()
	<-done
	_ = readPipe.Close()
	if runErr != nil {
		tb.Fatal(runErr)
	}

	var envelope struct {
		Messages []messageItem `json:"messages"`
	}
	if err := json.Unmarshal(sink.Bytes(), &envelope); err != nil {
		tb.Fatalf("decode CLI JSON %q: %v", sink.String(), err)
	}
	if len(envelope.Messages) != len(workflowFixtureSubjects) {
		tb.Fatalf("CLI facts = %#v", envelope.Messages)
	}
	for index, subject := range workflowFixtureSubjects {
		if envelope.Messages[index].Subject != subject {
			tb.Fatalf("CLI message %d subject = %q, want %q", index, envelope.Messages[index].Subject, subject)
		}
	}
	meter.jsonBytes.Add(int64(len(sink.Bytes())))
}

func TestNativeMCPGmailWorkflowFixtureFacts(t *testing.T) {
	meter := &workflowMeter{}
	base := &nativeBenchmarkTransport{}
	provider := nativeBenchmarkProvider{client: &http.Client{Transport: &validatedFixtureTransport{inner: &meteredFixtureTransport{base: base, meter: meter}}}}
	id := workflowIdentity()
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})

	service, err := gmailapi.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: &validatedFixtureTransport{inner: &meteredFixtureTransport{base: base, meter: meter}}}))
	if err != nil {
		t.Fatal(err)
	}
	originalService := newGmailService
	newGmailService = func(context.Context, string) (*gmailapi.Service, error) { return service, nil }
	t.Cleanup(func() { newGmailService = originalService })

	operations := nativegmail.Operations(provider)
	op := operations[0]
	raw := json.RawMessage(`{"account_id":"work-fixture","query":"Acme","max_results":25}`)
	call, err := op.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := call.Run(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	checked, ok := direct.(mcpcontract.Result[nativegmail.SearchData])
	if !ok {
		t.Fatalf("direct result type = %T", direct)
	}
	if len(checked.Data.Messages) != 2 {
		t.Fatalf("direct messages = %#v", checked.Data.Messages)
	}
	for index, subject := range workflowFixtureSubjects {
		if checked.Data.Messages[index].Subject != subject {
			t.Fatalf("direct message %d subject = %q, want %q", index, checked.Data.Messages[index].Subject, subject)
		}
	}
	workflowCLIFacts(t, ctx, meter)

	expanded := workflowRuntime(t, provider, mcpserver.DiscoveryExpanded)
	result, err := callWorkflowTool(meter, expanded, "gmail_search", map[string]any{
		"account_id": workflowFixtureAccount, "query": workflowFixtureQuery, "max_results": workflowFixtureMax,
	})
	if err != nil || result.IsError {
		t.Fatalf("expanded search err=%v result=%+v", err, result)
	}
	workflowGmailEnvelope(t, result)

	compact := workflowRuntime(t, provider, mcpserver.DiscoveryCompact)
	search, err := callWorkflowTool(meter, compact, "capabilities_search", map[string]any{"query": "gmail search", "limit": 5})
	if err != nil || search.IsError {
		t.Fatalf("search err=%v result=%+v", err, search)
	}
	var found struct {
		Operations []struct{ Name string } `json:"operations"`
	}
	if decodeErr := json.Unmarshal(mustJSON(t, search.StructuredContent), &found); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	var searchable bool
	for _, operation := range found.Operations {
		searchable = searchable || operation.Name == "gmail_search"
	}
	if !searchable {
		t.Fatalf("gmail_search not discovered: %s", mustJSON(t, search.StructuredContent))
	}

	described, err := callWorkflowTool(meter, compact, "capabilities_describe", map[string]any{"name": "gmail_search"})
	if err != nil || described.IsError {
		t.Fatalf("describe err=%v result=%+v", err, described)
	}
	var describedOperation struct {
		Name string `json:"name"`
	}
	if decodeErr := json.Unmarshal(mustJSON(t, described.StructuredContent), &describedOperation); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if describedOperation.Name != "gmail_search" {
		t.Fatalf("described operation = %q", describedOperation.Name)
	}

	fullCatalog := workflowRuntimeCatalog(t, provider, mcpserver.DiscoveryCompact, workflowFixtureMaxCalls, true)
	fullSearch, err := callWorkflowTool(meter, fullCatalog, "capabilities_search", map[string]any{"query": "gmail search", "limit": 5})
	if err != nil || fullSearch.IsError {
		t.Fatalf("full-catalog search err=%v result=%+v", err, fullSearch)
	}
	var fullFound struct {
		Operations []struct {
			Name string `json:"name"`
		} `json:"operations"`
	}
	if decodeErr := json.Unmarshal(mustJSON(t, fullSearch.StructuredContent), &fullFound); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if len(fullFound.Operations) != 1 || fullFound.Operations[0].Name != "gmail_search" {
		t.Fatalf("full-catalog granted operations = %#v", fullFound.Operations)
	}
	deniedOperation, err := callWorkflowTool(meter, fullCatalog, "capabilities_execute", map[string]any{
		"name": "drive_search", "arguments": map[string]any{"account_id": workflowFixtureAccount, "text": workflowFixtureQuery},
	})
	if err != nil || !deniedOperation.IsError {
		t.Fatalf("drive_search err=%v result=%+v", err, deniedOperation)
	}
	var deniedFailure mcpcontract.Error
	if decodeErr := json.Unmarshal(mustJSON(t, deniedOperation.StructuredContent), &deniedFailure); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if deniedFailure.Category != mcpcontract.Forbidden {
		t.Fatalf("drive_search category = %q", deniedFailure.Category)
	}

	executed, err := callWorkflowTool(meter, compact, "capabilities_execute", map[string]any{
		"name": "gmail_search",
		"arguments": map[string]any{
			"account_id": workflowFixtureAccount, "query": workflowFixtureQuery, "max_results": workflowFixtureMax,
		},
	})
	if err != nil || executed.IsError {
		t.Fatalf("execute err=%v result=%+v", err, executed)
	}
	workflowGmailEnvelope(t, executed)
}

func TestNativeMCPGmailWorkflowDeniedAndUnknownAccounts(t *testing.T) {
	meter := &workflowMeter{}
	base := &nativeBenchmarkTransport{}
	provider := nativeBenchmarkProvider{client: &http.Client{Transport: &validatedFixtureTransport{inner: &meteredFixtureTransport{base: base, meter: meter}}}}
	session := workflowRuntime(t, provider, mcpserver.DiscoveryCompact)

	for _, account := range []string{workflowDeniedAccount, workflowUnknownAccount} {
		before := base.calls.Load()
		result, err := callWorkflowTool(meter, session, "capabilities_execute", map[string]any{
			"name": "gmail_search",
			"arguments": map[string]any{
				"account_id": account, "query": workflowFixtureQuery, "max_results": workflowFixtureMax,
			},
		})
		if err != nil || !result.IsError {
			t.Fatalf("account %s err=%v result=%+v", account, err, result)
		}
		if base.calls.Load() != before || meter.fixtureCalls.Load() != 0 {
			t.Fatalf("account %s reached fixture transport: before=%d after=%d", account, before, base.calls.Load())
		}
		var failure mcpcontract.Error
		if err := json.Unmarshal(mustJSON(t, result.StructuredContent), &failure); err != nil {
			t.Fatal(err)
		}
		if failure.Category != mcpcontract.Forbidden {
			t.Fatalf("account %s category = %q", account, failure.Category)
		}
	}
}

func TestNativeMCPGmailWorkflowMetricCounts(t *testing.T) {
	meter := &workflowMeter{}
	base := &nativeBenchmarkTransport{}
	provider := nativeBenchmarkProvider{client: &http.Client{Transport: &validatedFixtureTransport{inner: &meteredFixtureTransport{base: base, meter: meter}}}}

	compact := workflowRuntime(t, provider, mcpserver.DiscoveryCompact)
	meter.reset()
	if err := validateCompactWorkflow(t, meter, compact, map[string]any{
		"account_id": workflowFixtureAccount, "query": workflowFixtureQuery, "max_results": workflowFixtureMax,
	}, true); err != nil {
		t.Fatal(err)
	}
	if base.calls.Load() != 4 || meter.fixtureCalls.Load() != 4 || meter.mcpCalls.Load() != 3 {
		t.Fatalf("compact counts transport=%d meter=%d mcp=%d", base.calls.Load(), meter.fixtureCalls.Load(), meter.mcpCalls.Load())
	}

	expanded := workflowRuntime(t, provider, mcpserver.DiscoveryExpanded)
	base.calls.Store(0)
	meter.reset()
	result, err := callWorkflowTool(meter, expanded, "gmail_search", map[string]any{
		"account_id": workflowFixtureAccount, "query": workflowFixtureQuery, "max_results": workflowFixtureMax,
	})
	if err != nil || result.IsError {
		t.Fatalf("expanded search err=%v result=%+v", err, result)
	}
	workflowGmailEnvelope(t, result)
	if base.calls.Load() != 4 || meter.fixtureCalls.Load() != 4 || meter.mcpCalls.Load() != 1 {
		t.Fatalf("expanded counts transport=%d meter=%d mcp=%d", base.calls.Load(), meter.fixtureCalls.Load(), meter.mcpCalls.Load())
	}
}

type budgetScriptTransport struct{ calls atomic.Int64 }

func (t *budgetScriptTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	attempt := t.calls.Add(1)
	if attempt > 2 {
		return nil, &mcpcontract.Error{Category: mcpcontract.BudgetExhausted, Message: "budget test third attempt reached transport", Retryable: false}
	}

	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"messages":[]}`)),
		Request:    req,
	}, nil
}

type retryFixtureProvider struct{ base *budgetScriptTransport }

func (p retryFixtureProvider) HTTPClient(context.Context, mcpcontract.Identity, mcpcontract.CallOptions) (*http.Client, error) {
	return &http.Client{Transport: &googleapi.NativeRetryTransport{
		Base:          p.base,
		Class:         mcpcontract.SafeRead,
		MaxRetries429: 2,
		MaxRetries5xx: 0,
		BaseDelay:     time.Nanosecond,
	}}, nil
}

func TestNativeMCPGmailWorkflowStrictUpstreamBudget(t *testing.T) {
	base := &budgetScriptTransport{}
	provider := retryFixtureProvider{base: base}
	session := workflowRuntimeCatalog(t, provider, mcpserver.DiscoveryCompact, 2, false)

	result, err := callWorkflowTool(&workflowMeter{}, session, "capabilities_execute", map[string]any{
		"name": "gmail_search",
		"arguments": map[string]any{
			"account_id": workflowFixtureAccount, "query": workflowFixtureQuery, "max_results": workflowFixtureMax,
		},
	})
	if err != nil || !result.IsError {
		t.Fatalf("budgeted request err=%v result=%+v", err, result)
	}

	var failure mcpcontract.Error
	if err := json.Unmarshal(mustJSON(t, result.StructuredContent), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Category != mcpcontract.BudgetExhausted {
		t.Fatalf("category = %q, message = %q", failure.Category, failure.Message)
	}
	if base.calls.Load() != 2 {
		t.Fatalf("transport calls = %d, want 2", base.calls.Load())
	}
}

func mustJSON(tb testing.TB, value any) []byte {
	tb.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		tb.Fatal(err)
	}
	return data
}

func validateCompactWorkflow(tb testing.TB, meter *workflowMeter, session *mcp.ClientSession, executeArguments map[string]any, validateResult bool) error {
	tb.Helper()

	search, err := callWorkflowTool(meter, session, "capabilities_search", map[string]any{"query": "gmail search", "limit": 5})
	if err != nil {
		return err
	}
	if search.IsError {
		return errFixtureToolResult
	}
	if recordErr := meter.recordResult(search); recordErr != nil {
		return recordErr
	}
	described, err := callWorkflowTool(meter, session, "capabilities_describe", map[string]any{"name": "gmail_search"})
	if err != nil {
		return err
	}
	if described.IsError {
		return errFixtureToolResult
	}
	if recordErr := meter.recordResult(described); recordErr != nil {
		return recordErr
	}
	executed, err := callWorkflowTool(meter, session, "capabilities_execute", map[string]any{
		"name": "gmail_search", "arguments": executeArguments,
	})
	if err != nil {
		return err
	}
	if executed.IsError {
		return errFixtureToolResult
	}
	if recordErr := meter.recordResult(executed); recordErr != nil {
		return recordErr
	}
	if validateResult {
		workflowGmailEnvelope(tb, executed)
	}
	return nil
}

func BenchmarkNativeMCPGmailWorkflowFixture(b *testing.B) {
	meter := &workflowMeter{}
	base := &nativeBenchmarkTransport{}
	provider := nativeBenchmarkProvider{client: &http.Client{Transport: &validatedFixtureTransport{inner: &meteredFixtureTransport{base: base, meter: meter}}}}
	id := workflowIdentity()
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})

	service, err := gmailapi.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: &validatedFixtureTransport{inner: &meteredFixtureTransport{base: base, meter: meter}}}))
	if err != nil {
		b.Fatal(err)
	}
	originalService := newGmailService
	newGmailService = func(context.Context, string) (*gmailapi.Service, error) { return service, nil }
	b.Cleanup(func() { newGmailService = originalService })

	expanded := workflowRuntime(b, provider, mcpserver.DiscoveryExpanded)
	compact := workflowRuntime(b, provider, mcpserver.DiscoveryCompact)
	compactFullCatalog := workflowRuntimeCatalog(b, provider, mcpserver.DiscoveryCompact, workflowFixtureMaxCalls, true)

	executeArguments := map[string]any{
		"account_id": workflowFixtureAccount, "query": workflowFixtureQuery, "max_results": workflowFixtureMax,
	}
	runCLI := func() error {
		command := GmailMessagesSearchCmd{Query: []string{workflowFixtureQuery}, Max: workflowFixtureMax, Timezone: "UTC"}
		return command.Run(ctx, &RootFlags{Account: id.Email})
	}
	runExpanded := func() error {
		result, err := callWorkflowTool(meter, expanded, "gmail_search", executeArguments)
		if err != nil {
			return err
		}
		if result.IsError {
			return errFixtureToolResult
		}
		return meter.recordResult(result)
	}
	runCompact := func() error {
		return validateCompactWorkflow(b, meter, compact, executeArguments, false)
	}
	runCompactFullCatalog := func() error {
		return validateCompactWorkflow(b, meter, compactFullCatalog, executeArguments, false)
	}

	// Warm and validate all protocol paths before timing. CLI facts are captured
	// so benchmark warmup does not pollute benchmark stdout.
	workflowCLIFacts(b, ctx, meter)
	expandedResult, warmErr := callWorkflowTool(meter, expanded, "gmail_search", executeArguments)
	if warmErr != nil || expandedResult.IsError {
		b.Fatalf("expanded warmup err=%v result=%+v", warmErr, expandedResult)
	}
	workflowGmailEnvelope(b, expandedResult)
	if compactWarmErr := validateCompactWorkflow(b, meter, compact, executeArguments, true); compactWarmErr != nil {
		b.Fatal(compactWarmErr)
	}
	if fullCatalogWarmErr := validateCompactWorkflow(b, meter, compactFullCatalog, executeArguments, true); fullCatalogWarmErr != nil {
		b.Fatal(fullCatalogWarmErr)
	}
	meter.reset()

	routes := []struct {
		name string
		run  func() error
	}{
		{name: "cli_handler", run: func() error {
			readPipe, writePipe, err := os.Pipe()
			if err != nil {
				return err
			}
			original := os.Stdout
			os.Stdout = writePipe
			done := make(chan error, 1)
			go func() {
				_, copyErr := io.Copy(meter.resultWriter(), readPipe)
				_ = readPipe.Close()
				done <- copyErr
			}()
			runErr := runCLI()
			os.Stdout = original
			_ = writePipe.Close()
			if copyErr := <-done; copyErr != nil {
				return copyErr
			}
			return runErr
		}},
		{name: "mcp_expanded_warm_execute", run: runExpanded},
		{name: "mcp_compact_search_describe_execute", run: runCompact},
		{name: "mcp_compact_full_catalog_search_describe_execute", run: runCompactFullCatalog},
	}

	for _, route := range routes {
		b.Run(route.name, func(b *testing.B) {
			meter.reset()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				start := time.Now()
				if err := route.run(); err != nil {
					b.Fatal(err)
				}
				meter.recordDuration(start)
			}
			b.StopTimer()
			meter.report(b)
		})
	}

	meter.reset()
	result, postErr := compact.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "capabilities_execute", Arguments: map[string]any{"name": "gmail_search", "arguments": executeArguments},
	})
	if postErr != nil {
		b.Fatal(postErr)
	}
	if result.IsError {
		b.Fatal(errFixtureToolResult)
	}
	workflowGmailEnvelope(b, result)
}

var errFixtureToolResult = errors.New("MCP tool result")
