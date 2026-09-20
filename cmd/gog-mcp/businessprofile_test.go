package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

func TestBusinessProfileReadThroughCompactProtocol(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		scope, grant bool
	}{
		{"allowed", true, true}, {"missing_scope", false, true}, {"missing_grant", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			registry := accountconnect.NewMemoryRegistry()
			scopes := []string{mcpcontract.DriveReadScope}
			if tc.scope {
				scopes = append(scopes, mcpcontract.BusinessManageScope)
			}
			for _, account := range []string{"allowed", "other"} {
				if err := registry.Upsert(ctx, accountconnect.Record{AccountID: account, Subject: "subject-" + account, Email: account + "@example.test", Label: account, PrincipalID: "fixture", ClientName: "native-mcp", AuthMode: accountconnect.AuthModeOAuth, Scopes: scopes, Generation: 1, State: accountconnect.RecordStateActive, UpdatedAt: time.Now().UTC()}); err != nil {
					t.Fatal(err)
				}
			}
			grants := []string{"drive:list"}
			if tc.grant {
				grants = append(grants, "businessprofile:accounts.list")
			}
			transport := &expandedTransport{}
			runtime, err := mcpserver.New(mcpserver.Config{Principal: mcpcontract.Principal{ID: "fixture"}, Grants: []mcpcontract.Grant{{PrincipalID: "fixture", AccountIDs: []string{"allowed"}, ClientNames: []string{"native-mcp"}, Operations: grants}}, AllowOperations: []string{"accounts_list", "businessprofile_list_accounts"}, Operations: configuredOperations(cli{APICatalog: true}, expandedProvider{transport}), Accounts: registryAccounts{registry: registry}, DiscoveryMode: mcpserver.DiscoveryCompact, MaxUpstreamCalls: 1, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			if err != nil {
				t.Fatal(err)
			}
			serverTransport, clientTransport := mcp.NewInMemoryTransports()
			serverSession, err := runtime.Server().Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer serverSession.Close()
			session, err := mcp.NewClient(&mcp.Implementation{Name: "gbp-fixture", Version: "test"}, nil).Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			for _, account := range []string{"other", "allowed"} {
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{"name": "businessprofile_list_accounts", "arguments": map[string]any{"account_id": account}}})
				if err != nil {
					t.Fatal(err)
				}
				wantSuccess := tc.scope && tc.grant && account == "allowed"
				if result.IsError == wantSuccess {
					t.Fatalf("account=%s result=%+v", account, result)
				}
				wantCalls := int64(0)
				if wantSuccess {
					wantCalls = 1
				}
				if transport.calls.Load() != wantCalls {
					t.Fatalf("account=%s calls=%d want=%d", account, transport.calls.Load(), wantCalls)
				}
			}
		})
	}
}
