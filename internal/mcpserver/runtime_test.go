package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

type countingProvider struct {
	calls atomic.Int64
}

func (p *countingProvider) HTTPClient(_ context.Context, id mcpcontract.Identity, _ mcpcontract.CallOptions) (*http.Client, error) {
	p.calls.Add(1)

	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		googleapi.AddUpstreamCall(req.Context())

		if _, ok := req.Context().Deadline(); !ok {
			return nil, errNoDeadline
		}
		body := `{"ok":true,"account":"` + id.AccountID + `"}`

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type deadlineError string

func (e deadlineError) Error() string { return string(e) }

const errNoDeadline deadlineError = "native API request has no deadline"

type searchInput struct {
	mcpcontract.Selection
	Query string `json:"query"`
}

type searchData struct {
	Account string `json:"account"`
	Query   string `json:"query"`
}

func fakeSearch(provider mcpcontract.ClientProvider) mcpcontract.Operation {
	return mcpcontract.NewOperation[searchInput, mcpcontract.Result[searchData]](
		"gmail_search",
		nil,
		func(ctx context.Context, id mcpcontract.Identity, in searchInput) (mcpcontract.Result[searchData], error) {
			client, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[searchData]{}, fmt.Errorf("gmail search: %w", err)
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://gmail.googleapis.com/gmail/v1/users/me/messages/list", strings.NewReader("{}"))
			if err != nil {
				return mcpcontract.Result[searchData]{}, fmt.Errorf("gmail search: %w", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				return mcpcontract.Result[searchData]{}, fmt.Errorf("gmail search: %w", err)
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)

			return mcpcontract.NewResult(id, searchData{Account: id.AccountID, Query: in.Query}), nil
		},
	)
}

func fixtureAccounts() *access.MemoryAccounts {
	return access.NewMemoryAccounts(
		mcpcontract.Identity{AccountID: "personal", Subject: "subject-personal", Email: "personal@example.test", Label: "Personal", PrincipalID: "fixture", ClientName: "app", AuthMode: "oauth", Scopes: []string{mcpcontract.GmailReadScope}, Generation: 1},
		mcpcontract.Identity{AccountID: "work", Subject: "subject-work", Email: "work@example.test", Label: "Work", PrincipalID: "fixture", ClientName: "app", AuthMode: "oauth", Scopes: []string{mcpcontract.GmailReadScope}, Generation: 1},
		mcpcontract.Identity{AccountID: "secret", Subject: "subject-other", Email: "private@example.test", PrincipalID: "other", ClientName: "app", AuthMode: "oauth", Scopes: []string{mcpcontract.GmailReadScope}, Generation: 1},
	)
}

func fixtureConfig(operations []mcpcontract.Operation) mcpserver.Config {
	return mcpserver.Config{
		Principal:       mcpcontract.Principal{ID: "fixture"},
		Grants:          []mcpcontract.Grant{{PrincipalID: "fixture", AccountIDs: []string{"personal", "work"}, ClientNames: []string{"app"}, Operations: []string{"gmail:messages.search"}}},
		AllowOperations: []string{"accounts_list", "gmail_search"},
		Operations:      operations,
		Accounts:        fixtureAccounts(),

		RequestTimeout: 5 * time.Second,
	}
}

func TestRuntimeFakeOperationAndDeniedZeroUpstream(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

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
		t.Fatalf("visible tools: %d", len(listed.Tools))
	}

	catalog, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "accounts_list", Arguments: map[string]any{}})
	if err != nil || catalog.IsError {
		t.Fatalf("accounts_list: %#v %v", catalog, err)
	}

	payload, _ := json.Marshal(catalog.StructuredContent)
	if strings.Contains(string(payload), "private@example.test") || !strings.Contains(string(payload), "personal") {
		t.Fatalf("catalog: %s", payload)
	}

	got, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_search", Arguments: map[string]any{"account_id": "work", "query": "from:client"}})
	if err != nil || got.IsError {
		t.Fatalf("search: %#v %v", got, err)
	}

	body, _ := json.Marshal(got.StructuredContent)
	if !strings.Contains(string(body), `"account":"work"`) {
		t.Fatalf("result: %s", body)
	}

	before := provider.calls.Load()

	denied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_search", Arguments: map[string]any{"account_id": "secret", "query": "from:client"}})
	if err == nil && !denied.IsError {
		t.Fatal("secret account accepted")
	}

	if provider.calls.Load() != before {
		t.Fatal("forbidden call reached Google")
	}
}

func TestRuntimeStdioTransportLogsStayOffStdout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	provider := &countingProvider{}

	runtime, err := mcpserver.New(fixtureConfig([]mcpcontract.Operation{fakeSearch(provider)}))
	if err != nil {
		t.Fatal(err)
	}

	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runtime.Run(ctx, &mcp.IOTransport{Reader: serverReader, Writer: serverWriter})
	}()

	t.Cleanup(func() {
		_ = clientReader.Close()
		_ = clientWriter.Close()
		_ = serverReader.Close()
		_ = serverWriter.Close()

		cancel()
		<-errCh
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test", Version: "1"}, nil)

	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientReader, Writer: clientWriter}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	got, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "accounts_list", Arguments: map[string]any{}})
	if err != nil || got.IsError {
		t.Fatalf("stdio accounts_list: %#v %v", got, err)
	}
}

func connectRuntime(t *testing.T, ctx context.Context, runtime *mcpserver.Runtime) *mcp.ClientSession {
	t.Helper()

	st, ct := mcp.NewInMemoryTransports()

	ss, err := runtime.Server().Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "fixture-client", Version: "1"}, nil)

	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}

	return session
}

type attrLogger struct {
	attrs []slog.Attr
}

func (a *attrLogger) Enabled(context.Context, slog.Level) bool { return true }
func (a *attrLogger) Handle(_ context.Context, record slog.Record) error {
	record.Attrs(func(attr slog.Attr) bool {
		a.attrs = append(a.attrs, attr)
		return true
	})

	return nil
}
func (a *attrLogger) WithAttrs([]slog.Attr) slog.Handler { return a }
func (a *attrLogger) WithGroup(string) slog.Handler      { return a }

func TestRuntimeLogsUpstreamCalls(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	handler := &attrLogger{}
	provider := &countingProvider{}
	cfg := fixtureConfig([]mcpcontract.Operation{fakeSearch(provider)})
	cfg.Logger = slog.New(handler)

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	got, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_search", Arguments: map[string]any{"account_id": "work", "query": "from:client"}})
	if err != nil || got.IsError {
		t.Fatalf("search: %#v %v", got, err)
	}

	var calls int64 = -1

	for _, attr := range handler.attrs {
		if attr.Key == "upstream_calls" {
			calls = attr.Value.Int64()
		}
	}

	if calls != 1 {
		t.Fatalf("upstream_calls=%d attrs=%v", calls, handler.attrs)
	}
}

func TestAccountsListRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	provider := &countingProvider{}

	runtime, err := mcpserver.New(fixtureConfig([]mcpcontract.Operation{fakeSearch(provider)}))
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	got, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "accounts_list", Arguments: map[string]any{"bogus": true}})
	if err == nil && (got == nil || !got.IsError) {
		t.Fatal("unknown accounts_list fields must fail")
	}
}

func TestResyncDropsToolsWhenSoleAccountRemoved(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	provider := &countingProvider{}
	accounts := fixtureAccounts()
	cfg := fixtureConfig([]mcpcontract.Operation{fakeSearch(provider)})
	cfg.Accounts = accounts

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

	if len(listed.Tools) != 2 {
		t.Fatalf("before delete: %d", len(listed.Tools))
	}

	accounts.Delete("personal")
	accounts.Delete("work")
	runtime.ResyncTools()

	listed, err = session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, tool := range listed.Tools {
		if tool.Name == "gmail_search" {
			t.Fatal("gmail tool remained after sole eligible accounts were removed")
		}
	}
}

type flipAccounts struct {
	inner *access.MemoryAccounts
	fail  atomic.Bool
}

func (f *flipAccounts) Get(ctx context.Context, id string) (mcpcontract.Identity, bool, error) {
	identity, ok, err := f.inner.Get(ctx, id)
	if err != nil {
		return identity, ok, fmt.Errorf("fixture get: %w", err)
	}

	return identity, ok, nil
}

func (f *flipAccounts) List(ctx context.Context, principal string) ([]mcpcontract.Identity, error) {
	if f.fail.Load() {
		return nil, errCorruptRegistry
	}

	identities, err := f.inner.List(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("fixture list: %w", err)
	}

	return identities, nil
}

var errCorruptRegistry = errors.New("corrupt registry")

func TestReloadAccessKeepsPriorSnapshotOnRegistryError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	provider := &countingProvider{}
	accounts := &flipAccounts{inner: fixtureAccounts()}
	cfg := fixtureConfig([]mcpcontract.Operation{fakeSearch(provider)})
	cfg.Accounts = accounts

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	before, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	accounts.fail.Store(true)

	err = runtime.ReloadAccess(access.Snapshot{
		AllowOperations: []string{"accounts_list", "gmail_search"},
		Grants:          []mcpcontract.Grant{{PrincipalID: "fixture", AccountIDs: []string{"work"}, ClientNames: []string{"app"}, Operations: []string{"gmail_search"}}},
	})
	if err == nil {
		t.Fatal("expected registry list error")
	}

	after, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(after.Tools) != len(before.Tools) {
		t.Fatalf("catalog changed after failed reload: %d -> %d", len(before.Tools), len(after.Tools))
	}
}

func TestEncodedToolResultIsBounded(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	payload := strings.Repeat("n", 80)
	op := mcpcontract.NewOperation[searchInput, mcpcontract.Result[searchData]]("gmail_search", nil, func(context.Context, mcpcontract.Identity, searchInput) (mcpcontract.Result[searchData], error) {
		return mcpcontract.NewResult(mcpcontract.Identity{AccountID: "work", Label: "Work"}, searchData{Account: "work", Query: payload}), nil
	})
	cfg := fixtureConfig([]mcpcontract.Operation{op})
	cfg.MaxBodyBytes = 180

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	got, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_search", Arguments: map[string]any{"account_id": "work", "query": "q"}})
	if err == nil && (got == nil || !got.IsError) {
		t.Fatal("duplicated content must be counted against the result bound")
	}
}
