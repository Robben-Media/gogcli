package mcpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/config"
	nativegmail "github.com/steipete/gogcli/internal/googleops/gmail"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

type sliceAccounts []mcpcontract.Identity

func (s sliceAccounts) Get(_ context.Context, id string) (mcpcontract.Identity, bool, error) {
	for _, a := range s {
		if a.AccountID == id {
			return a.Clone(), true, nil
		}
	}

	return mcpcontract.Identity{}, false, nil
}

func (s sliceAccounts) List(_ context.Context, principal string) ([]mcpcontract.Identity, error) {
	out := []mcpcontract.Identity{}

	for _, a := range s {
		if a.PrincipalID == principal {
			out = append(out, a.Clone())
		}
	}

	return out, nil
}

type sliceTransport struct {
	account string
	calls   *atomic.Int64
}

func (s sliceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls.Add(1)

	if _, ok := req.Context().Deadline(); !ok {
		return nil, errNoDeadline
	}
	body := `{"labels":[{"id":"INBOX","name":"INBOX"}]}`

	if strings.Contains(req.URL.Path, "/messages/") {
		message := map[string]any{"id": "m1", "threadId": "t1", "internalDate": "1000", "labelIds": []string{"INBOX"}, "payload": map[string]any{"mimeType": "text/plain", "headers": []map[string]string{{"name": "Subject", "value": s.account}}, "body": map[string]string{"data": "aGVsbG8"}}}
		data, _ := json.Marshal(message)
		body = string(data)
	}

	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

type sliceProvider struct{ calls atomic.Int64 }

func (p *sliceProvider) HTTPClient(_ context.Context, id mcpcontract.Identity, _ mcpcontract.CallOptions) (*http.Client, error) {
	return &http.Client{Transport: sliceTransport{account: id.AccountID, calls: &p.calls}}, nil
}

func TestGmailTwoAccountMCPVerticalSlice(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	accounts := sliceAccounts{
		{AccountID: "personal", Subject: "subject-personal", Email: "personal@example.test", Label: "Personal", PrincipalID: "fixture", ClientName: "app", AuthMode: "oauth", Scopes: []string{mcpcontract.GmailReadScope}, Generation: 1},
		{AccountID: "work", Subject: "subject-work", Email: "work@example.test", Label: "Work", PrincipalID: "fixture", ClientName: "app", AuthMode: "oauth", Scopes: []string{mcpcontract.GmailReadScope}, Generation: 1},
		{AccountID: "secret", Subject: "subject-other", Email: "private@example.test", PrincipalID: "other", ClientName: "app", AuthMode: "oauth", Scopes: []string{mcpcontract.GmailReadScope}, Generation: 1},
	}
	provider := &sliceProvider{}
	grant := mcpcontract.Grant{PrincipalID: "fixture", AccountIDs: []string{"personal", "work"}, ClientNames: []string{"app"}, Operations: []string{"gmail:get"}}

	runtime, err := mcpserver.New(mcpserver.Config{Principal: mcpcontract.Principal{ID: "fixture"}, Grants: []mcpcontract.Grant{grant}, AllowOperations: []string{"accounts_list", "gmail_get_message"}, Operations: nativegmail.Operations(provider), Accounts: accounts, RequestTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	st, ct := mcp.NewInMemoryTransports()

	ss, err := runtime.Server().Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture-client", Version: "1"}, nil)

	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(listed.Tools) != 2 {
		t.Fatalf("unexpected visible tools: %d", len(listed.Tools))
	}

	catalog, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "accounts_list", Arguments: map[string]any{}})
	if err != nil || catalog.IsError {
		t.Fatalf("accounts_list: %#v %v", catalog, err)
	}

	catalogJSON, _ := json.Marshal(catalog.StructuredContent)
	if strings.Contains(string(catalogJSON), "private@example.test") || !strings.Contains(string(catalogJSON), "personal") || !strings.Contains(string(catalogJSON), "work") {
		t.Fatalf("caller catalog: %s", catalogJSON)
	}
	var wg sync.WaitGroup

	for i := 0; i < 12; i++ {
		account := []string{"personal", "work"}[i%2]

		wg.Go(func() {
			got, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_get_message", Arguments: map[string]any{"account_id": account, "message_id": "m1"}})
			if callErr != nil {
				t.Errorf("call: %v", callErr)
				return
			}

			if got.IsError {
				t.Errorf("tool failed: %#v", got)
				return
			}
			data, _ := json.Marshal(got.StructuredContent)

			var result mcpcontract.Result[nativegmail.MessageView]
			if err := json.Unmarshal(data, &result); err != nil {
				t.Errorf("decode: %v", err)
				return
			}

			if result.AccountID != account || result.Data.Subject != account || result.Data.Body != "hello" {
				t.Errorf("cross-account result: %s", data)
			}
		})
	}

	wg.Wait()
	before := provider.calls.Load()

	for _, account := range []string{"secret", "personal@example.test", "missing", ""} {
		got, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_get_message", Arguments: map[string]any{"account_id": account, "message_id": "m1"}})
		if callErr == nil && !got.IsError {
			t.Errorf("accepted unauthorized selector %q", account)
		}
	}
	_, _ = session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_get_thread", Arguments: map[string]any{"account_id": "work", "thread_id": "t1"}})

	if provider.calls.Load() != before {
		t.Fatal("denied call reached Google transport")
	}

	runtime.ReplaceAccess(access.Snapshot{Grants: []mcpcontract.Grant{grant}, AllowOperations: []string{"accounts_list", "gmail_get_message"}, Policies: []config.Policy{{Name: "deny-work", Account: "work@example.test", Deny: []string{"gmail:get"}}}})

	got, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail_get_message", Arguments: map[string]any{"account_id": "work", "message_id": "m1"}})
	if callErr == nil && !got.IsError {
		t.Fatal("policy reload did not revoke access")
	}

	if provider.calls.Load() != before {
		t.Fatal("revoked policy reached Google")
	}
}
