package mcpserver_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

const (
	httpTestHost   = "google-mcp.example.com"
	httpTestOrigin = "https://google-mcp.example.com"
)

func TestHTTPGatewayTwoCallersAndSharedOwnerGrants(t *testing.T) {
	t.Parallel()

	fixture := newHTTPFixture(t, httpFixtureOptions{})

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	work := fixture.connect(t, ctx, fixture.workToken)
	defer work.Close()

	mail := fixture.connect(t, ctx, fixture.mailToken)
	defer mail.Close()

	workTools, err := work.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	mailTools, err := mail.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !hasTool(workTools, "gmail_search") || hasTool(workTools, "gmail_get_message") {
		t.Fatalf("work tools %#v", names(workTools))
	}

	if !hasTool(mailTools, "gmail_get_message") || hasTool(mailTools, "gmail_search") {
		t.Fatalf("mail tools %#v", names(mailTools))
	}

	searchOK, searchErr := work.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_search", Arguments: map[string]any{"account_id": "work", "query": "from:client"}})
	if searchErr != nil || searchOK.IsError {
		t.Fatalf("work search: %#v %v", searchOK, searchErr)
	}

	denied, err := work.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_search", Arguments: map[string]any{"account_id": "personal", "query": "from:client"}})
	if err == nil && (denied == nil || !denied.IsError) {
		t.Fatal("work token used personal account")
	}

	got, err := mail.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_get_message", Arguments: map[string]any{"account_id": "personal"}})
	if err != nil || got.IsError {
		t.Fatalf("mail get: %#v %v", got, err)
	}

	deniedMail, err := mail.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_get_message", Arguments: map[string]any{"account_id": "work"}})
	if err == nil && (deniedMail == nil || !deniedMail.IsError) {
		t.Fatal("mail token used work account")
	}

	listed, err := work.CallTool(ctx, &mcp.CallToolParams{Name: "accounts_list", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("accounts_list: %#v %v", listed, err)
	}

	payload, _ := json.Marshal(listed.StructuredContent)
	if !strings.Contains(string(payload), "work") || strings.Contains(string(payload), "personal") {
		t.Fatalf("work catalog %s", payload)
	}
}

func TestHTTPGatewaySDKProtocolAndResources(t *testing.T) {
	t.Parallel()

	fixture := newHTTPFixture(t, httpFixtureOptions{})

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	session := fixture.connect(t, ctx, fixture.workToken)
	defer session.Close()

	listed, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(listed.Resources) == 0 {
		t.Fatal("expected workflow resources")
	}

	got, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: listed.Resources[0].URI})
	if err != nil || got == nil || len(got.Contents) == 0 {
		t.Fatalf("read resource: %#v %v", got, err)
	}
}

func TestHTTPGatewayAuthOriginHostAndSpoofedIdentity(t *testing.T) {
	t.Parallel()

	fixture := newHTTPFixture(t, httpFixtureOptions{})
	initBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)

	missing := fixture.raw(t, http.MethodPost, "/mcp", fixture.workToken, httpTestHost, "", nil, initBody)
	missing.Header.Del("Authorization")

	unauthorized := doRaw(t, fixture.handler, missing)
	if unauthorized.StatusCode != http.StatusUnauthorized || !strings.Contains(unauthorized.Body, "unauthorized") || strings.Contains(unauthorized.Body, fixture.workToken) {
		t.Fatalf("missing token: %+v", unauthorized)
	}

	invalid := fixture.raw(t, http.MethodPost, "/mcp", strings.Repeat("n", 40), httpTestHost, "", nil, initBody)

	got := doRaw(t, fixture.handler, invalid)
	if got.StatusCode != http.StatusUnauthorized || got.Body != unauthorized.Body {
		t.Fatalf("invalid token %d %q want %q", got.StatusCode, got.Body, unauthorized.Body)
	}

	wrongHost := fixture.raw(t, http.MethodPost, "/mcp", fixture.workToken, "evil.example.com", "", map[string]string{
		"X-Forwarded-Host": httpTestHost,
		"X-Forwarded-For":  "1.2.3.4",
		"Forwarded":        "host=" + httpTestHost,
	}, initBody)
	if resp := doRaw(t, fixture.handler, wrongHost); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("spoofed host %d %q", resp.StatusCode, resp.Body)
	}

	wrongOrigin := fixture.raw(t, http.MethodPost, "/mcp", fixture.workToken, httpTestHost, "https://evil.example.com", nil, initBody)
	if resp := doRaw(t, fixture.handler, wrongOrigin); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("origin %d %q", resp.StatusCode, resp.Body)
	}

	okOrigin := fixture.raw(t, http.MethodPost, "/mcp", fixture.workToken, httpTestHost, httpTestOrigin, nil, initBody)
	if resp := doRaw(t, fixture.handler, okOrigin); resp.StatusCode != http.StatusOK {
		t.Fatalf("allowed origin %d %q", resp.StatusCode, resp.Body)
	}

	noOrigin := fixture.raw(t, http.MethodPost, "/mcp", fixture.workToken, httpTestHost, "", nil, initBody)
	if resp := doRaw(t, fixture.handler, noOrigin); resp.StatusCode != http.StatusOK {
		t.Fatalf("missing origin %d %q", resp.StatusCode, resp.Body)
	}

	otherPath := fixture.raw(t, http.MethodGet, "/health", fixture.workToken, httpTestHost, "", nil, nil)
	if resp := doRaw(t, fixture.handler, otherPath); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("health %d %q", resp.StatusCode, resp.Body)
	}

	unauthPath := fixture.raw(t, http.MethodGet, "/health", "", httpTestHost, "", nil, nil)
	if resp := doRaw(t, fixture.handler, unauthPath); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth health %d", resp.StatusCode)
	}

	spoof := fixture.raw(t, http.MethodPost, "/mcp", fixture.workToken, httpTestHost, "", map[string]string{
		"X-Principal":      "other",
		"X-Caller-Id":      "hermes-mail",
		"X-Forwarded-User": "other",
	}, initBody)
	if resp := doRaw(t, fixture.handler, spoof); resp.StatusCode != http.StatusOK {
		t.Fatalf("spoof headers %d %q", resp.StatusCode, resp.Body)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	session := fixture.connectWithHeaders(t, ctx, fixture.workToken, map[string]string{
		"X-Principal": "other",
		"X-Caller-Id": "hermes-mail",
	})
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if hasTool(tools, "gmail_get_message") || !hasTool(tools, "gmail_search") {
		t.Fatalf("spoofed identity took mail grants: %#v", names(tools))
	}
}

func TestHTTPGatewayBodyAndConcurrencyBounds(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	fixture := newHTTPFixture(t, httpFixtureOptions{
		globalSlots: 1,
		callerSlots: 1,
		maxBody:     2048,
		blockSearch: &blockSearch{started: started, release: release},
	})

	huge := fixture.raw(t, http.MethodPost, "/mcp", fixture.workToken, httpTestHost, "", nil, bytes.Repeat([]byte("a"), 4096))
	huge.Header.Set("Content-Type", "application/json")
	huge.Header.Set("Accept", "application/json, text/event-stream")

	if resp := doRaw(t, fixture.handler, huge); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize %d %q", resp.StatusCode, resp.Body)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	session := fixture.connect(t, ctx, fixture.workToken)
	defer session.Close()

	errCh := make(chan error, 1)

	go func() {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_search", Arguments: map[string]any{"account_id": "work", "query": "block"}})
		errCh <- err
	}()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("blocked call did not start")
	}

	busy := fixture.raw(t, http.MethodPost, "/mcp", fixture.workToken, httpTestHost, "", nil, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
	if resp := doRaw(t, fixture.handler, busy); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("busy %d %q", resp.StatusCode, resp.Body)
	}

	close(release)

	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestHTTPGatewayShutdownAndCancel(t *testing.T) {
	t.Parallel()

	fixture := newHTTPFixture(t, httpFixtureOptions{})
	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error, 1)

	go func() {
		errCh <- mcpserver.ListenAndServeHTTP(ctx, "127.0.0.1:0", fixture.handler, time.Second)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("http server did not shut down")
	}
}

func TestNewHTTPGatewayRejectsIncompleteConfig(t *testing.T) {
	t.Parallel()

	_, err := mcpserver.NewHTTPGateway(mcpserver.HTTPHandlerConfig{
		Accounts: access.NewMemoryAccounts(),
	})
	if !errorsIsHTTP(err) {
		t.Fatalf("got %v", err)
	}
}

type httpFixtureOptions struct {
	globalSlots int
	callerSlots int
	maxBody     int64
	blockSearch *blockSearch
}

type blockSearch struct {
	started chan struct{}
	release chan struct{}
}

type httpFixture struct {
	handler   http.Handler
	host      string
	workToken string
	mailToken string
}

func newHTTPFixture(t *testing.T, opts httpFixtureOptions) *httpFixture {
	t.Helper()

	workToken, workDigest := testBearer(t)
	mailToken, mailDigest := distinctBearer(t, workToken)

	cfg, err := mcpserver.ParseHTTPConfig(mustJSON(t, map[string]any{
		"host":            httpTestHost,
		"allowed_origins": []string{httpTestOrigin},
		"callers": []map[string]any{
			{
				"id": "hermes-work", "token_sha256": workDigest, "principal_id": "jeremy",
				"allow_operations": []string{"accounts_list", "gmail_search"},
				"grants": []map[string]any{{
					"principal_id": "jeremy", "account_ids": []string{"work"}, "client_names": []string{"native-mcp"}, "operations": []string{"gmail:messages.search"},
				}},
			},
			{
				"id": "hermes-mail", "token_sha256": mailDigest, "principal_id": "jeremy",
				"allow_operations": []string{"accounts_list", "gmail_get_message"},
				"grants": []map[string]any{{
					"principal_id": "jeremy", "account_ids": []string{"personal"}, "client_names": []string{"native-mcp"}, "operations": []string{"gmail:get"},
				}},
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}

	provider := &countingProvider{}

	operations := []mcpcontract.Operation{fakeSearch(provider), fakeGetMessage(provider)}
	if opts.blockSearch != nil {
		operations[0] = blockingSearch(opts.blockSearch)
	}

	callerSlots := 8
	globalSlots := 16

	if opts.callerSlots > 0 {
		callerSlots = opts.callerSlots
	}

	if opts.globalSlots > 0 {
		globalSlots = opts.globalSlots
	}

	gateway, err := mcpserver.NewHTTPGateway(mcpserver.HTTPHandlerConfig{
		HTTP:              cfg,
		Operations:        operations,
		Accounts:          httpFixtureAccounts(),
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout:    5 * time.Second,
		MaxConcurrency:    callerSlots,
		GlobalConcurrency: globalSlots,
		MaxBodyBytes:      opts.maxBody,
		MaxUpstreamCalls:  8,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &httpFixture{
		handler:   gateway,
		host:      httpTestHost,
		workToken: workToken,
		mailToken: mailToken,
	}
}

func httpFixtureAccounts() *access.MemoryAccounts {
	return access.NewMemoryAccounts(
		mcpcontract.Identity{AccountID: "personal", Subject: "subject-personal", Email: "personal@example.test", Label: "Personal", PrincipalID: "jeremy", ClientName: "native-mcp", AuthMode: "oauth", Scopes: []string{mcpcontract.GmailReadScope}, Generation: 1},
		mcpcontract.Identity{AccountID: "work", Subject: "subject-work", Email: "work@example.test", Label: "Work", PrincipalID: "jeremy", ClientName: "native-mcp", AuthMode: "oauth", Scopes: []string{mcpcontract.GmailReadScope}, Generation: 1},
	)
}

func (f *httpFixture) connect(t *testing.T, ctx context.Context, token string) *mcp.ClientSession {
	t.Helper()
	return f.connectWithHeaders(t, ctx, token, nil)
}

func (f *httpFixture) connectWithHeaders(t *testing.T, ctx context.Context, token string, extra map[string]string) *mcp.ClientSession {
	t.Helper()

	server := httptest.NewServer(f.handler)
	t.Cleanup(server.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "http-fixture", Version: "1"}, nil)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             server.URL + "/mcp",
		HTTPClient:           &http.Client{Transport: &headerTransport{base: server.Client().Transport, host: f.host, token: token, extra: extra}},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	return session
}

func (f *httpFixture) raw(t *testing.T, method, path, token, host, origin string, extra map[string]string, body []byte) *http.Request {
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req := httptest.NewRequestWithContext(t.Context(), method, "http://"+host+path, reader)

	req.Host = host
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if origin != "" {
		req.Header.Set("Origin", origin)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
	}

	for key, value := range extra {
		req.Header.Set(key, value)
	}

	return req
}

type rawResult struct {
	StatusCode int
	Body       string
}

func doRaw(t *testing.T, handler http.Handler, req *http.Request) rawResult {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rawResult{StatusCode: rec.Code, Body: rec.Body.String()}
}

type headerTransport struct {
	base  http.RoundTripper
	host  string
	token string
	extra map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())

	clone.Host = t.host
	if t.token != "" {
		clone.Header.Set("Authorization", "Bearer "+t.token)
	}

	for key, value := range t.extra {
		clone.Header.Set(key, value)
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(clone)
	if err != nil {
		return nil, fmt.Errorf("http fixture round trip: %w", err)
	}

	return resp, nil
}

func fakeGetMessage(provider mcpcontract.ClientProvider) mcpcontract.Operation {
	return mcpcontract.NewOperation[mcpcontract.Selection, mcpcontract.Result[searchData]](
		"gmail_get_message",
		nil,
		func(ctx context.Context, id mcpcontract.Identity, _ mcpcontract.Selection) (mcpcontract.Result[searchData], error) {
			client, err := provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "gmail_get_message", Retry: mcpcontract.SafeRead})
			if err != nil {
				return mcpcontract.Result[searchData]{}, fmt.Errorf("gmail get message: %w", err)
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gmail.googleapis.com/gmail/v1/users/me/messages/x", nil)
			if err != nil {
				return mcpcontract.Result[searchData]{}, fmt.Errorf("gmail get message: %w", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				return mcpcontract.Result[searchData]{}, fmt.Errorf("gmail get message: %w", err)
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)

			return mcpcontract.NewResult(id, searchData{Account: id.AccountID}), nil
		},
	)
}

func blockingSearch(block *blockSearch) mcpcontract.Operation {
	return mcpcontract.NewOperation[searchInput, mcpcontract.Result[searchData]](
		"gmail_search",
		nil,
		func(ctx context.Context, id mcpcontract.Identity, in searchInput) (mcpcontract.Result[searchData], error) {
			select {
			case <-block.started:
			default:
				close(block.started)
			}

			select {
			case <-block.release:
			case <-ctx.Done():
				return mcpcontract.Result[searchData]{}, ctx.Err()
			}

			return mcpcontract.NewResult(id, searchData{Account: id.AccountID, Query: in.Query}), nil
		},
	)
}

func hasTool(listed *mcp.ListToolsResult, name string) bool {
	if listed == nil {
		return false
	}

	for _, tool := range listed.Tools {
		if tool.Name == name {
			return true
		}
	}

	return false
}

func names(listed *mcp.ListToolsResult) []string {
	if listed == nil {
		return nil
	}

	out := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		out = append(out, tool.Name)
	}

	return out
}

func distinctBearer(t *testing.T, other string) (token, digest string) {
	t.Helper()

	token = strings.Repeat("m", 32) + t.Name()
	if token == other {
		token += "x"
	}

	sum := sha256.Sum256([]byte(token))

	return token, hex.EncodeToString(sum[:])
}

func errorsIsHTTP(err error) bool {
	return err != nil && strings.Contains(err.Error(), "http config")
}
