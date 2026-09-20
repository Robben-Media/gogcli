package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/access"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

func TestConfigRejectsInvalidDiscoveryAndBudget(t *testing.T) {
	t.Parallel()

	cfg := fixtureConfig(nil)

	cfg.DiscoveryMode = "spray"
	if _, err := mcpserver.New(cfg); err == nil {
		t.Fatal("invalid discovery mode accepted")
	}

	cfg = fixtureConfig(nil)

	cfg.MaxUpstreamCalls = -1
	if _, err := mcpserver.New(cfg); err == nil {
		t.Fatal("negative budget accepted")
	}

	cfg = fixtureConfig(nil)

	cfg.MaxUpstreamCalls = 257
	if _, err := mcpserver.New(cfg); err == nil {
		t.Fatal("oversized budget accepted")
	}

	cfg = fixtureConfig(nil)
	cfg.MaxBodyBytes = 256

	if _, err := mcpserver.New(cfg); err == nil {
		t.Fatal("undersized body limit accepted")
	}

	cfg = fixtureConfig(nil)
	cfg.MaxBodyBytes = 512

	if _, err := mcpserver.New(cfg); err != nil {
		t.Fatal(err)
	}
}

func compactConfig(operations []mcpcontract.Operation) mcpserver.Config {
	cfg := fixtureConfig(operations)
	cfg.DiscoveryMode = mcpserver.DiscoveryCompact

	return cfg
}

func stubOperation(name string, retry mcpcontract.RetryClass) mcpcontract.Operation {
	def, ok := mcpcontract.Lookup(name)
	if !ok {
		panic("unknown catalog operation: " + name)
	}

	if retry != "" {
		def.Retry = retry
	}

	return mcpcontract.Operation{
		Definition: def,
		Decode: func(json.RawMessage) (mcpcontract.Call, error) {
			return mcpcontract.Call{}, &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "stub"}
		},
	}
}

func manyStubOperations(base mcpcontract.Operation) []mcpcontract.Operation {
	names := make([]string, 0, 16)

	for _, def := range mcpcontract.Catalog() {
		if def.Local {
			continue
		}
		names = append(names, def.Name)
	}

	out := make([]mcpcontract.Operation, 0, 1+len(names)*4)

	out = append(out, base)
	for i := 0; i < 64; i++ {
		out = append(out, stubOperation(names[i%len(names)], ""))
	}

	return out
}

func listedNames(t *testing.T, listed *mcp.ListToolsResult) []string {
	t.Helper()

	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}

	return names
}

func callJSON(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func TestCompactCatalogSizeUnchangedWithManyOperations(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	provider := &countingProvider{}

	runtime, err := mcpserver.New(compactConfig(manyStubOperations(fakeSearch(provider))))
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	names := listedNames(t, listed)
	if len(names) != 4 {
		t.Fatalf("compact catalog size = %d (%v)", len(names), names)
	}

	want := map[string]bool{
		"accounts_list":         true,
		"capabilities_search":   true,
		"capabilities_describe": true,
		"capabilities_execute":  true,
	}
	for _, name := range names {
		if !want[name] {
			t.Fatalf("unexpected compact tool %q", name)
		}

		delete(want, name)
	}

	if len(want) != 0 {
		t.Fatalf("missing compact tools: %v", want)
	}

	for _, tool := range listed.Tools {
		blob, _ := json.Marshal(tool)
		if strings.Contains(string(blob), `"properties"`) && tool.Name != "capabilities_search" && tool.Name != "capabilities_describe" && tool.Name != "capabilities_execute" && tool.Name != "accounts_list" {
			t.Fatalf("native schema leaked into tools/list: %s", blob)
		}

		if tool.Name == "gmail_search" {
			t.Fatal("native operation advertised in compact mode")
		}
	}
}

func TestCompactSearchDescribeExecuteGates(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	provider := &countingProvider{}
	writeOp := stubOperation("gmail_get_thread", mcpcontract.ProviderKeyedWrite)
	driveOp := stubOperation("drive_search", "")
	sheetsOp := stubOperation("sheets_read_range", "")
	cfg := compactConfig([]mcpcontract.Operation{fakeSearch(provider), writeOp, driveOp, sheetsOp})
	cfg.AllowOperations = []string{"accounts_list", "gmail_search", "gmail_get_thread", "drive_search", "sheets_read_range"}
	cfg.Grants = []mcpcontract.Grant{{
		PrincipalID: "fixture",
		AccountIDs:  []string{"personal", "work"},
		ClientNames: []string{"app"},
		Operations:  []string{"gmail:messages.search", "gmail:thread.get", "drive:search"},
	}}

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	search, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_search", Arguments: map[string]any{"query": "email inbox"}})
	if err != nil || search.IsError {
		t.Fatalf("search: %#v %v", search, err)
	}

	searchBlob := callJSON(t, search)
	if strings.Contains(searchBlob, "input_schema") || strings.Contains(searchBlob, `"properties"`) {
		t.Fatalf("search leaked schema: %s", searchBlob)
	}

	if !strings.Contains(searchBlob, `"gmail_search"`) || !strings.Contains(searchBlob, `"service":"gmail"`) {
		t.Fatalf("search missed gmail: %s", searchBlob)
	}

	if strings.Contains(searchBlob, "drive_search") || strings.Contains(searchBlob, "gmail_get_thread") {
		t.Fatalf("search included irrelevant or write op: %s", searchBlob)
	}

	blank, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_search", Arguments: map[string]any{"query": "   "}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, blank, mcpcontract.InvalidInput)

	none, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_search", Arguments: map[string]any{"query": "spreadsheet range"}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, none, mcpcontract.NotFound)

	if !strings.Contains(callJSON(t, none), "no eligible capability; narrow service/task or check grants") {
		t.Fatalf("missing matching failure: %s", callJSON(t, none))
	}

	if strings.Contains(callJSON(t, none), "input_schema") || strings.Contains(callJSON(t, none), "gmail_search") {
		t.Fatalf("no-match leaked fallback: %s", callJSON(t, none))
	}

	denied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{"name": "sheets_read_range"}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, denied, mcpcontract.Forbidden)

	deniedBlob := callJSON(t, denied)
	if strings.Contains(deniedBlob, "input_schema") || strings.Contains(deniedBlob, "file metadata") {
		t.Fatalf("describe leaked schema: %s", deniedBlob)
	}

	writeDenied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{"name": "gmail_get_thread"}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, writeDenied, mcpcontract.Forbidden)

	if strings.Contains(callJSON(t, writeDenied), "input_schema") {
		t.Fatalf("write describe leaked schema: %s", callJSON(t, writeDenied))
	}

	described, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{"name": "gmail_search"}})
	if err != nil || described.IsError {
		t.Fatalf("describe: %#v %v", described, err)
	}

	describedBlob := callJSON(t, described)
	if !strings.Contains(describedBlob, "account_id") || !strings.Contains(describedBlob, "input_schema") || !strings.Contains(describedBlob, "guidance") || !strings.Contains(describedBlob, "service_guidance") {
		t.Fatalf("describe missing schema/guidance: %s", describedBlob)
	}

	if strings.Contains(describedBlob, `"service":"admin"`) || strings.Contains(describedBlob, `"service":"slides"`) {
		t.Fatalf("describe dumped other services: %s", describedBlob)
	}

	if strings.Contains(describedBlob, "inspect_paths") && strings.Contains(describedBlob, `"truncated":true`) {
		t.Fatalf("small schema should stay exact: %s", describedBlob)
	}

	before := provider.calls.Load()

	writeExec, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{
		"name":      "gmail_get_thread",
		"arguments": map[string]any{"account_id": "work", "thread_id": "t1"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, writeExec, mcpcontract.Forbidden)

	if provider.calls.Load() != before {
		t.Fatal("write execute reached Google")
	}

	recursive, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{
		"name":      "capabilities_execute",
		"arguments": map[string]any{"name": "gmail_search"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, recursive, mcpcontract.Forbidden)

	if provider.calls.Load() != before {
		t.Fatal("recursive execute reached Google")
	}

	missingAccount, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{
		"name":      "gmail_search",
		"arguments": map[string]any{"query": "from:client"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, missingAccount, mcpcontract.InvalidInput)

	secret, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{
		"name":      "gmail_search",
		"arguments": map[string]any{"account_id": "secret", "query": "from:client"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, secret, mcpcontract.Forbidden)

	if provider.calls.Load() != before {
		t.Fatal("forbidden execute reached Google")
	}

	got, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{
		"name":      "gmail_search",
		"arguments": map[string]any{"account_id": "work", "query": "from:client"},
	}})
	if err != nil || got.IsError {
		t.Fatalf("execute: %#v %v", got, err)
	}

	body, _ := json.Marshal(got.StructuredContent)
	if !strings.Contains(string(body), `"account":"work"`) {
		t.Fatalf("execute result: %s", body)
	}

	if got.Meta["upstream_calls"] == nil {
		t.Fatalf("missing usage meta: %#v", got.Meta)
	}

	if provider.calls.Load() != before+1 {
		t.Fatalf("upstream calls = %d", provider.calls.Load())
	}
}

func TestExpandedWriteGateHidesWriteOperations(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	provider := &countingProvider{}
	cfg := fixtureConfig([]mcpcontract.Operation{
		fakeSearch(provider),
		stubOperation("gmail_get_thread", mcpcontract.ProviderKeyedWrite),
	})
	cfg.AllowOperations = []string{"accounts_list", "gmail_search", "gmail_get_thread"}
	cfg.Grants = []mcpcontract.Grant{{
		PrincipalID: "fixture",
		AccountIDs:  []string{"personal", "work"},
		ClientNames: []string{"app"},
		Operations:  []string{"gmail:messages.search", "gmail:thread.get"},
	}}

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, tool := range listed.Tools {
		if tool.Name == "gmail_get_thread" {
			t.Fatal("write operation advertised while EnableWrites is false")
		}
	}
}

func TestCompactAccountsListUnchanged(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	provider := &countingProvider{}

	runtime, err := mcpserver.New(compactConfig([]mcpcontract.Operation{fakeSearch(provider)}))
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	catalog, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "accounts_list", Arguments: map[string]any{}})
	if err != nil || catalog.IsError {
		t.Fatalf("accounts_list: %#v %v", catalog, err)
	}

	payload, _ := json.Marshal(catalog.StructuredContent)
	if strings.Contains(string(payload), "private@example.test") || !strings.Contains(string(payload), "personal") {
		t.Fatalf("catalog: %s", payload)
	}
}

func TestDefaultDiscoveryRemainsExpanded(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	provider := &countingProvider{}

	runtime, err := mcpserver.New(fixtureConfig([]mcpcontract.Operation{fakeSearch(provider)}))
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(listed.Tools) != 2 {
		t.Fatalf("expanded tools = %d", len(listed.Tools))
	}
}

func TestCompactSearchBoundsHugeDescriptions(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	provider := &countingProvider{}
	op := fakeSearch(provider)
	op.Definition.Description = strings.Repeat("provider-description ", 500)

	runtime, err := mcpserver.New(compactConfig([]mcpcontract.Operation{op}))
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	search, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_search", Arguments: map[string]any{"query": "email inbox"}})
	if err != nil || search.IsError {
		t.Fatalf("search: %#v %v", search, err)
	}

	blob := callJSON(t, search)
	if len(blob) > 8<<10 {
		t.Fatalf("search reply %d bytes", len(blob))
	}
	var searchOut struct {
		Operations []struct {
			Description string `json:"description"`
		} `json:"operations"`
	}

	data, _ := json.Marshal(search.StructuredContent)
	if unmarshalErr := json.Unmarshal(data, &searchOut); unmarshalErr != nil || len(searchOut.Operations) != 1 {
		t.Fatalf("search payload: %s %v", data, unmarshalErr)
	}

	if n := len([]rune(searchOut.Operations[0].Description)); n > 320 {
		t.Fatalf("search description runes = %d", n)
	}

	described, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{"name": "gmail_search"}})
	if err != nil || described.IsError {
		t.Fatalf("describe: %#v %v", described, err)
	}
	var describedOut struct {
		Description string `json:"description"`
	}

	describedData, _ := json.Marshal(described.StructuredContent)
	if err := json.Unmarshal(describedData, &describedOut); err != nil {
		t.Fatal(err)
	}

	n := len([]rune(describedOut.Description))
	if n <= 320 || n > 1200 {
		t.Fatalf("describe description runes = %d", n)
	}
}

func TestCompactSearchMalformedInputsAndLimit(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	provider := &countingProvider{}

	runtime, err := mcpserver.New(compactConfig([]mcpcontract.Operation{fakeSearch(provider)}))
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	unknown, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_search", Arguments: map[string]any{"query": "email", "bogus": true}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, unknown, mcpcontract.InvalidInput)

	punct, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_search", Arguments: map[string]any{"query": "???"}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, punct, mcpcontract.InvalidInput)

	long, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_search", Arguments: map[string]any{"query": strings.Repeat("mail ", 50)}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, long, mcpcontract.InvalidInput)

	missing, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, missing, mcpcontract.InvalidInput)

	badPath, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{"name": "gmail_search", "schema_path": "body.query"}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, badPath, mcpcontract.InvalidInput)

	if strings.Contains(callJSON(t, badPath), "input_schema") {
		t.Fatalf("bad path leaked schema: %s", callJSON(t, badPath))
	}

	execMissing, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{"name": "gmail_search"}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, execMissing, mcpcontract.InvalidInput)
}

func TestCompactDescribeBoundsLargeSchemaPath(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	provider := &countingProvider{}
	op := fakeSearch(provider)
	op.InputSchema = largeDescribeSchema()

	runtime, err := mcpserver.New(compactConfig([]mcpcontract.Operation{op}))
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	described, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{"name": "gmail_search"}})
	if err != nil || described.IsError {
		t.Fatalf("describe: %#v %v", described, err)
	}

	blob := callJSON(t, described)
	if len(blob) > 48_000 {
		t.Fatalf("describe dumped %d bytes", len(blob))
	}

	if strings.Contains(blob, "nestedBlob-secret") {
		t.Fatalf("describe leaked nested schema: %s", blob)
	}

	if !strings.Contains(blob, "requests") || !strings.Contains(blob, "inspect_paths") || !strings.Contains(blob, "service_guidance") {
		t.Fatalf("describe missing shallow fields: %s", blob)
	}

	drilled, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":        "gmail_search",
		"schema_path": "input.properties.requests.items",
	}})
	if err != nil || drilled.IsError {
		t.Fatalf("drill: %#v %v", drilled, err)
	}

	drilledBlob := callJSON(t, drilled)
	if strings.Contains(drilledBlob, "nestedBlob-secret") && len(drilledBlob) > 48_000 {
		t.Fatalf("drill dumped nested blob: %d", len(drilledBlob))
	}

	if !strings.Contains(drilledBlob, "replaceAllText") {
		t.Fatalf("drill missed nested field: %s", drilledBlob)
	}
}

func TestCompactWritesRequireEnableWrites(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	writeOp := fakeThreadWrite()
	cfg := compactConfig([]mcpcontract.Operation{writeOp})
	cfg.AllowOperations = []string{"accounts_list", "gmail_get_thread"}
	cfg.Grants = []mcpcontract.Grant{{
		PrincipalID: "fixture",
		AccountIDs:  []string{"personal", "work"},
		ClientNames: []string{"app"},
		Operations:  []string{"gmail:thread.get"},
	}}

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	search, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_search", Arguments: map[string]any{"query": "thread"}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, search, mcpcontract.NotFound)

	cfg.EnableWrites = true

	enabled, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	enabledSession := connectRuntime(t, ctx, enabled)
	defer enabledSession.Close()

	found, err := enabledSession.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_search", Arguments: map[string]any{"query": "thread"}})
	if err != nil || found.IsError {
		t.Fatalf("enabled search: %#v %v", found, err)
	}

	if !strings.Contains(callJSON(t, found), "gmail_get_thread") {
		t.Fatalf("enabled search missed write: %s", callJSON(t, found))
	}

	got, err := enabledSession.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{
		"name":      "gmail_get_thread",
		"arguments": map[string]any{"account_id": "work", "thread_id": "t1"},
	}})
	if err != nil || got.IsError {
		t.Fatalf("enabled execute: %#v %v", got, err)
	}
}

func TestAccountCapabilitiesUseRegisteredOperationsAndWrites(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	provider := &countingProvider{}

	runtime, err := mcpserver.New(compactConfig(manyStubOperations(fakeSearch(provider))))
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	catalog, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "accounts_list", Arguments: map[string]any{}})
	if err != nil || catalog.IsError {
		t.Fatalf("accounts_list: %#v %v", catalog, err)
	}

	payload, _ := json.Marshal(catalog.StructuredContent)
	if strings.Contains(string(payload), "gmail_search") || strings.Count(string(payload), `"gmail.read"`) > 2 {
		t.Fatalf("capabilities listed operations instead of groups: %s", payload)
	}

	if !strings.Contains(string(payload), "gmail.read") {
		t.Fatalf("missing gmail.read: %s", payload)
	}

	if strings.Contains(string(payload), "gmail.write") {
		t.Fatalf("write group advertised: %s", payload)
	}
}

type threadInput struct {
	mcpcontract.Selection
	ThreadID string `json:"thread_id"`
}

type threadData struct {
	ThreadID string `json:"thread_id"`
}

func fakeThreadWrite() mcpcontract.Operation {
	op := mcpcontract.NewOperation[threadInput, mcpcontract.Result[threadData]]("gmail_get_thread", nil, func(_ context.Context, id mcpcontract.Identity, in threadInput) (mcpcontract.Result[threadData], error) {
		return mcpcontract.NewResult(id, threadData{ThreadID: in.ThreadID}), nil
	})
	op.Definition.Retry = mcpcontract.NonReplayableWrite

	return op
}

func largeDescribeSchema() *jsonschema.Schema {
	blob := &jsonschema.Schema{Type: "string", Description: strings.Repeat("nestedBlob-secret ", 4000)}
	request := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"replaceAllText": {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"containsText": {Type: "object", Properties: map[string]*jsonschema.Schema{"text": {Type: "string"}, "nestedBlob": blob}},
				},
			},
		},
	}

	return &jsonschema.Schema{
		Type:     "object",
		Required: []string{"account_id"},
		Properties: map[string]*jsonschema.Schema{
			"account_id": {Type: "string"},
			"requests":   {Type: "array", Items: request},
		},
	}
}

type blockingListAccounts struct {
	*access.MemoryAccounts
	block   atomic.Bool
	entered chan struct{}
}

func (a *blockingListAccounts) List(ctx context.Context, principal string) ([]mcpcontract.Identity, error) {
	if a.block.Load() {
		select {
		case <-a.entered:
		default:
			close(a.entered)
		}

		<-ctx.Done()

		return nil, fmt.Errorf("list accounts: %w", ctx.Err())
	}

	identities, err := a.MemoryAccounts.List(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	return identities, nil
}

func TestCompactExecuteDoesNotWaitOnAccountList(t *testing.T) {
	t.Parallel()

	accounts := &blockingListAccounts{MemoryAccounts: fixtureAccounts(), entered: make(chan struct{})}
	cfg := compactConfig([]mcpcontract.Operation{fakeSearch(&countingProvider{})})
	cfg.Accounts = accounts

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, t.Context(), runtime)
	defer session.Close()

	accounts.block.Store(true)
	started := time.Now()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "capabilities_execute",
		Arguments: map[string]any{
			"name":      "gmail_search",
			"arguments": map[string]any{"account_id": "work", "query": "from:client"},
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("execute: %#v %v", result, err)
	}

	if time.Since(started) > time.Second {
		t.Fatal("compact execute waited on AccountSource.List")
	}
}

func TestCompactExecuteHonorsMaxConcurrency(t *testing.T) {
	t.Parallel()

	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	operation := mcpcontract.NewOperation[searchInput, mcpcontract.Result[searchData]]("gmail_search", nil, func(_ context.Context, identity mcpcontract.Identity, in searchInput) (mcpcontract.Result[searchData], error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}

		return mcpcontract.NewResult(identity, searchData{Account: identity.AccountID, Query: in.Query}), nil
	})
	cfg := compactConfig([]mcpcontract.Operation{operation})
	cfg.MaxConcurrency = 1
	cfg.RequestTimeout = 40 * time.Millisecond

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, t.Context(), runtime)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = session.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      "capabilities_execute",
			Arguments: map[string]any{"name": "gmail_search", "arguments": map[string]any{"account_id": "work", "query": "hold"}},
		})
	}()

	t.Cleanup(func() {
		close(release)
		<-firstDone
	})

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first execute did not start")
	}

	waitCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	result, err := session.CallTool(waitCtx, &mcp.CallToolParams{
		Name:      "capabilities_execute",
		Arguments: map[string]any{"name": "gmail_search", "arguments": map[string]any{"account_id": "work", "query": "wait"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, result, mcpcontract.DeadlineExceeded)

	if calls.Load() != 1 {
		t.Fatal("second compact execute bypassed MaxConcurrency")
	}
}

type failingGetAccounts struct {
	*access.MemoryAccounts
}

var errRegistryUnavailable = errors.New("registry unavailable")

func (a *failingGetAccounts) Get(_ context.Context, _ string) (mcpcontract.Identity, bool, error) {
	return mcpcontract.Identity{}, false, errRegistryUnavailable
}

func TestAccountsListPropagatesBackendErrors(t *testing.T) {
	t.Parallel()

	cfg := compactConfig([]mcpcontract.Operation{fakeSearch(&countingProvider{})})
	cfg.Accounts = &failingGetAccounts{MemoryAccounts: fixtureAccounts()}

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, t.Context(), runtime)
	defer session.Close()

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "accounts_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, result, mcpcontract.UpstreamFailure)
}

func TestCompactExecutePreservesLargeIntegers(t *testing.T) {
	t.Parallel()

	const exact = "9007199254740993"

	op := mcpcontract.NewOperation[struct {
		mcpcontract.Selection
		ID json.Number `json:"id"`
	}, mcpcontract.Result[searchData]]("gmail_search", nil, func(_ context.Context, id mcpcontract.Identity, in struct {
		mcpcontract.Selection
		ID json.Number `json:"id"`
	},
	) (mcpcontract.Result[searchData], error) {
		return mcpcontract.NewResult(id, searchData{Account: id.AccountID, Query: in.ID.String()}), nil
	})

	runtime, err := mcpserver.New(compactConfig([]mcpcontract.Operation{op}))
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, t.Context(), runtime)
	defer session.Close()

	got, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "capabilities_execute",
		Arguments: json.RawMessage(`{"name":"gmail_search","arguments":{"account_id":"work","id":9007199254740993}}`),
	})
	if err != nil || got.IsError {
		t.Fatalf("execute: %#v %v", got, err)
	}

	body, _ := json.Marshal(got.StructuredContent)
	if !strings.Contains(string(body), exact) {
		t.Fatalf("large integer rounded: %s", body)
	}
}
