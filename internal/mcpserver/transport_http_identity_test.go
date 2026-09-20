package mcpserver_test

import (
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleops/media"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
	"github.com/steipete/gogcli/internal/mediaartifact"
)

func TestHTTPMediaResourceOwnerAndHarnessIsolation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	accounts := fixtureAccounts()

	owner, _, err := accounts.Get(ctx, "personal")
	if err != nil {
		t.Fatal(err)
	}

	store := mediaartifact.New()
	t.Cleanup(store.Close)

	ref, err := store.Put(ctx, owner, "gmail_get_attachment", "", "text/plain", []byte("private attachment"))
	if err != nil {
		t.Fatal(err)
	}

	callers := make([]mcpserver.HTTPCaller, 0, 4)

	tokens := []string{strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32), strings.Repeat("d", 32)}
	for i, tc := range []struct{ name, principal, account string }{
		{"owner", "fixture", "personal"},
		{"narrow-harness", "fixture", "work"},
		{"other-owner", "other", "personal"},
		{"same-owner-authorized", "fixture", "personal"},
	} {
		callers = append(callers, mcpserver.HTTPCaller{
			ID: tc.name, TokenSHA256: sha256.Sum256([]byte(tokens[i])), PrincipalID: tc.principal,
			AllowOperations: []string{"accounts_list", "gmail_get_attachment"},
			Grants: []mcpcontract.Grant{{
				PrincipalID: tc.principal, AccountIDs: []string{tc.account},
				ClientNames: []string{"app"}, Operations: []string{"gmail:attachment"},
			}},
		})
	}

	provider := &attachmentFixtureProvider{}

	gateway, err := mcpserver.NewHTTPGateway(mcpserver.HTTPHandlerConfig{
		HTTP:     mcpserver.HTTPConfig{Host: httpTestHost, Callers: callers},
		Accounts: accounts, MediaArtifacts: store,
		Operations: media.OperationsWithArtifacts(provider, store),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}

	fixture := &httpFixture{handler: gateway, host: httpTestHost}

	authorized := fixture.connect(t, ctx, tokens[0])
	defer authorized.Close()

	for _, token := range tokens[1:3] {
		denied := fixture.connectWithHeaders(t, ctx, token, map[string]string{"X-Principal": "fixture", "X-Caller-Id": "owner"})
		if _, readErr := denied.ReadResource(ctx, &mcp.ReadResourceParams{URI: ref.URI}); readErr == nil {
			t.Fatal("caller read another harness's private artifact")
		}

		if closeErr := denied.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}

	read, err := authorized.ReadResource(ctx, &mcp.ReadResourceParams{URI: ref.URI})
	if err != nil || len(read.Contents) != 1 || string(read.Contents[0].Blob) != "private attachment" {
		t.Fatalf("authorized continuity: %#v %v", read, err)
	}

	shared := fixture.connect(t, ctx, tokens[3])
	defer shared.Close()

	if _, sharedErr := shared.ReadResource(ctx, &mcp.ReadResourceParams{URI: ref.URI}); sharedErr != nil {
		t.Fatalf("equivalent owner grants should permit resource reuse: %v", sharedErr)
	}

	if provider.calls.Load() != 0 {
		t.Fatal("cached resource retrieval made another Google call")
	}
}

func TestHTTPCompactExecutionGrantPolicyAndAudit(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	provider := &countingProvider{}
	logPath := filepath.Join(t.TempDir(), "audit.log")

	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = logFile.Close() })
	tokens := []string{strings.Repeat("f", 32), strings.Repeat("n", 32)}

	callers := make([]mcpserver.HTTPCaller, 0, 2)
	for i, accountIDs := range [][]string{{"work", "personal"}, {"personal"}} {
		callers = append(callers, mcpserver.HTTPCaller{
			ID: []string{"full", "narrow"}[i], TokenSHA256: sha256.Sum256([]byte(tokens[i])), PrincipalID: "jeremy",
			AllowOperations: []string{"accounts_list", "gmail_search"},
			Grants:          []mcpcontract.Grant{{PrincipalID: "jeremy", AccountIDs: accountIDs, ClientNames: []string{"native-mcp"}, Operations: []string{"gmail:messages.search"}}},
		})
	}

	gateway, err := mcpserver.NewHTTPGateway(mcpserver.HTTPHandlerConfig{
		HTTP:     mcpserver.HTTPConfig{Host: httpTestHost, Callers: callers},
		Accounts: httpFixtureAccounts(), Operations: []mcpcontract.Operation{fakeSearch(provider)},
		DiscoveryMode: mcpserver.DiscoveryCompact,
		Policies:      []config.Policy{{Name: "deny-personal", Account: "personal@example.test", Deny: []string{"gmail:messages.search"}}},
		Logger:        slog.New(slog.NewTextHandler(logFile, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &httpFixture{handler: gateway, host: httpTestHost}

	full := fixture.connect(t, ctx, tokens[0])
	defer full.Close()

	narrow := fixture.connect(t, ctx, tokens[1])
	defer narrow.Close()

	for _, tc := range []struct {
		session *mcp.ClientSession
		account string
		denied  bool
	}{
		{full, "work", false}, {narrow, "work", true}, {full, "personal", true},
	} {
		result, callErr := tc.session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{
			"name": "gmail_search", "arguments": map[string]any{"account_id": tc.account, "query": "fixture"},
		}})
		if callErr != nil {
			t.Fatal(callErr)
		}

		if tc.denied {
			requireCategory(t, result, mcpcontract.Forbidden)
		} else if result.IsError {
			t.Fatalf("authorized execution: %#v", result)
		}
	}

	if provider.calls.Load() != 1 {
		t.Fatalf("denied calls reached provider: %d", provider.calls.Load())
	}

	logs, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(logs), "caller_id=full") || strings.Contains(string(logs), tokens[0]) {
		t.Fatalf("caller attribution missing or credential leaked: %s", logs)
	}
}

func TestHTTPPrincipalQuotaSharedAcrossCredentials(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	fixture := newHTTPFixture(t, httpFixtureOptions{globalSlots: 4, callerSlots: 1, blockSearch: &blockSearch{started: started, release: release}})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	session := fixture.connect(t, ctx, fixture.workToken)
	defer session.Close()
	done := make(chan error, 1)

	go func() {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_search", Arguments: map[string]any{"account_id": "work", "query": "block"}})
		done <- err
	}()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("call did not start")
	}
	request := fixture.raw(t, http.MethodPost, "/mcp", fixture.mailToken, httpTestHost, "", nil,
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	response := doRaw(t, fixture.handler, request)

	close(release)

	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second credential bypassed owner quota: %+v", response)
	}
}

func TestHTTPGatewayRejectsInvalidTypedConfiguration(t *testing.T) {
	t.Parallel()

	_, digest := testBearer(t)
	for _, mutation := range []string{"duplicate-digest", "duplicate-id", "principal-mismatch", "bad-origin", "negative-body"} {
		t.Run(mutation, func(t *testing.T) {
			t.Parallel()

			cfg, err := mcpserver.ParseHTTPConfig(mustJSON(t, map[string]any{"host": httpTestHost, "callers": []map[string]any{minimalCaller("one", digest)}}))
			if err != nil {
				t.Fatal(err)
			}
			body := int64(0)

			switch mutation {
			case "duplicate-digest":
				other := cfg.Callers[0]
				other.ID = "two"
				cfg.Callers = append(cfg.Callers, other)
			case "duplicate-id":
				other := cfg.Callers[0]
				other.TokenSHA256 = sha256.Sum256([]byte("different"))
				cfg.Callers = append(cfg.Callers, other)
			case "principal-mismatch":
				cfg.Callers[0].Grants[0].PrincipalID = "other"
			case "bad-origin":
				cfg.AllowedOrigins = []string{"https://bad/path"}
			case "negative-body":
				body = -1
			}

			if _, err := mcpserver.NewHTTPGateway(mcpserver.HTTPHandlerConfig{HTTP: cfg, Accounts: httpFixtureAccounts(), MaxBodyBytes: body}); err == nil {
				t.Fatal("invalid direct config accepted")
			}
		})
	}
}

func TestHTTPConfigRequiresSingleJSONDocument(t *testing.T) {
	t.Parallel()
	_, digest := testBearer(t)

	base := string(mustJSON(t, map[string]any{"host": httpTestHost, "callers": []map[string]any{minimalCaller("one", digest)}}))
	for _, suffix := range []string{" ", "{}", "null", "1", "[", "[]"} {
		_, err := mcpserver.ParseHTTPConfig([]byte(base + suffix))
		if (err == nil) != (suffix == " ") {
			t.Fatalf("suffix %q: %v", suffix, err)
		}
	}
}

func TestHTTPLegacyBatchSharesOwnerToolQuota(t *testing.T) {
	t.Parallel()
	token, digest := testBearer(t)
	otherToken, otherDigest := distinctBearer(t, token)

	callers := []map[string]any{minimalCaller("first", digest), minimalCaller("second", otherDigest)}
	for _, caller := range callers {
		caller["allow_operations"] = []string{"gmail_search"}
	}

	cfg, err := mcpserver.ParseHTTPConfig(mustJSON(t, map[string]any{"host": httpTestHost, "callers": callers}))
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 3)

	release := make(chan struct{})
	defer close(release)
	op := mcpcontract.NewOperation("gmail_search", nil, func(ctx context.Context, id mcpcontract.Identity, in searchInput) (mcpcontract.Result[searchData], error) {
		started <- struct{}{}

		select {
		case <-release:
		case <-ctx.Done():
			return mcpcontract.Result[searchData]{}, ctx.Err()
		}

		return mcpcontract.NewResult(id, searchData{Account: id.AccountID, Query: in.Query}), nil
	})

	gateway, err := mcpserver.NewHTTPGateway(mcpserver.HTTPHandlerConfig{HTTP: cfg, Accounts: httpFixtureAccounts(), Operations: []mcpcontract.Operation{op}, MaxConcurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &httpFixture{handler: gateway}
	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gmail_search","arguments":{"account_id":"work","query":"bounded"}}}`
	batch := "[" + call + "," + strings.Replace(call, `"id":1`, `"id":2`, 1) + "]"

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	request := fixture.raw(t, http.MethodPost, "/mcp", token, httpTestHost, "", nil, []byte(batch)).WithContext(ctx)

	done := make(chan rawResult, 1)
	go func() { done <- doRaw(t, gateway, request) }()

	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("legacy batch did not start both calls")
		}
	}

	second := fixture.raw(t, http.MethodPost, "/mcp", otherToken, httpTestHost, "", nil, []byte(call)).WithContext(ctx)

	secondDone := make(chan rawResult, 1)
	go func() { secondDone <- doRaw(t, gateway, second) }()

	select {
	case <-started:
		t.Fatal("second credential exceeded owner tool quota through legacy batch")
	case <-time.After(100 * time.Millisecond):
	}
	// Let the batch finish; the waiting caller must then be able to proceed.
	release <- struct{}{}

	release <- struct{}{}

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("waiting caller did not resume")
	}

	release <- struct{}{}

	if response := <-secondDone; response.StatusCode != http.StatusOK || strings.Contains(response.Body, `"isError":true`) {
		t.Fatalf("waiting caller failed: %+v", response)
	}

	if response := <-done; response.StatusCode != http.StatusOK {
		t.Fatalf("batch failed: %+v", response)
	}
}
