package googleapi

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestNativeHTTPClientStalledWriteIsNotReplayed(t *testing.T) {
	t.Parallel()

	const requestTimeout = 2 * time.Second

	fx := newNativeFixture(t, 3600)

	var hits atomic.Int32

	release := make(chan struct{})

	t.Cleanup(func() { close(release) })
	fx.apiHandler.set(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))

	provider, err := NewNativeProvider(NativeOptions{
		Registry:       fx.registry,
		Tokens:         fx.tokens,
		Credentials:    func(clientName string) (string, string, error) { return "id-" + clientName, "secret", nil },
		Endpoint:       oauth2.Endpoint{TokenURL: fx.token.URL, AuthURL: fx.token.URL + "/auth"},
		Lifecycle:      fx.life,
		RequestTimeout: requestTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}

	id := fx.putAccount(t, "acct-write", "sub-write", "write@example.com", "personal", []string{mcpcontract.DocsWriteScope}, "rt-write")

	client, err := provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "docs_create_document", Retry: mcpcontract.NonReplayableWrite})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ctx, counter := WithUpstreamCounter(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fx.api.URL+"/v1/documents", strings.NewReader(`{"title":"stalled"}`))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	// Write operations map errors after Do with NativeWritePublicError.
	public := NativeWritePublicError(err)
	if public == nil || public.Category != mcpcontract.OutcomeUnknown || public.Retryable {
		t.Fatalf("error = %#v (%v)", public, err)
	}

	if hits.Load() != 1 || counter.Load() != 1 {
		t.Fatalf("server saw %d writes, upstream_calls = %d, want exactly one attempt", hits.Load(), counter.Load())
	}
}
