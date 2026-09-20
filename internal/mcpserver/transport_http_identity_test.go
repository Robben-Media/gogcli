package mcpserver_test

import (
	"crypto/sha256"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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

	callers := make([]mcpserver.HTTPCaller, 0, 3)

	tokens := []string{strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)}
	for i, tc := range []struct{ name, principal, account string }{
		{"owner", "fixture", "personal"},
		{"narrow-harness", "fixture", "work"},
		{"other-owner", "other", "personal"},
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

	for _, token := range tokens[1:] {
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

	if provider.calls.Load() != 0 {
		t.Fatal("cached resource retrieval made another Google call")
	}
}
