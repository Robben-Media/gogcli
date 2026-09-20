package mcpserver_test

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

func limitOperation(run func(context.Context, mcpcontract.Identity, searchInput) (mcpcontract.Result[searchData], error)) mcpcontract.Operation {
	return mcpcontract.NewOperation[searchInput, mcpcontract.Result[searchData]]("gmail_search", nil, run)
}

func limitSession(t *testing.T, cfg mcpserver.Config) *mcp.ClientSession {
	t.Helper()

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, t.Context(), runtime)
	t.Cleanup(func() { _ = session.Close() })

	return session
}

func searchParams(query string) *mcp.CallToolParams {
	return &mcp.CallToolParams{Name: "gmail_search", Arguments: map[string]any{"account_id": "work", "query": query}}
}

func requireCategory(t *testing.T, result *mcp.CallToolResult, category mcpcontract.ErrorCategory) {
	t.Helper()

	if result == nil || !result.IsError {
		t.Fatalf("expected tool error, got %#v", result)
	}

	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}

	var public mcpcontract.Error
	if err := json.Unmarshal(data, &public); err != nil {
		t.Fatal(err)
	}

	if public.Category != category {
		t.Fatalf("category = %s, want %s", public.Category, category)
	}
}

func TestRuntimeDeadlineReachesOperation(t *testing.T) {
	t.Parallel()
	operation := limitOperation(func(ctx context.Context, _ mcpcontract.Identity, _ searchInput) (mcpcontract.Result[searchData], error) {
		<-ctx.Done()
		return mcpcontract.Result[searchData]{}, ctx.Err()
	})
	cfg := fixtureConfig([]mcpcontract.Operation{operation})
	cfg.RequestTimeout = 20 * time.Millisecond
	session := limitSession(t, cfg)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, searchParams("deadline"))
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, result, mcpcontract.DeadlineExceeded)
}

func TestRuntimeClientCancellationReachesOperation(t *testing.T) {
	t.Parallel()
	started, stopped := make(chan struct{}), make(chan struct{})
	operation := limitOperation(func(ctx context.Context, _ mcpcontract.Identity, _ searchInput) (mcpcontract.Result[searchData], error) {
		close(started)
		<-ctx.Done()
		close(stopped)

		return mcpcontract.Result[searchData]{}, ctx.Err()
	})
	session := limitSession(t, fixtureConfig([]mcpcontract.Operation{operation}))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)

	go func() {
		_, err := session.CallTool(ctx, searchParams("cancel"))
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("operation did not start")
	}

	cancel()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("client cancellation did not reach server operation")
	}

	if err := <-done; err == nil {
		t.Fatal("canceled client call succeeded")
	}
}

func TestRuntimeSemaphoreWaitHasDeadline(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	operation := limitOperation(func(_ context.Context, identity mcpcontract.Identity, in searchInput) (mcpcontract.Result[searchData], error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}

		return mcpcontract.NewResult(identity, searchData{Account: identity.AccountID, Query: in.Query}), nil
	})
	cfg := fixtureConfig([]mcpcontract.Operation{operation})
	cfg.MaxConcurrency = 1
	cfg.RequestTimeout = 30 * time.Millisecond
	session := limitSession(t, cfg)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = session.CallTool(t.Context(), searchParams("hold"))
	}()

	t.Cleanup(func() { close(release); <-firstDone })

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first operation did not start")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, searchParams("wait"))
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, result, mcpcontract.DeadlineExceeded)

	if calls.Load() != 1 {
		t.Fatal("waiting request executed without a slot")
	}
}

func TestRuntimeBoundsRequestsAndResults(t *testing.T) {
	t.Parallel()

	for _, largeInput := range []bool{true, false} {
		t.Run(map[bool]string{true: "input", false: "output"}[largeInput], func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int64
			operation := limitOperation(func(_ context.Context, identity mcpcontract.Identity, _ searchInput) (mcpcontract.Result[searchData], error) {
				calls.Add(1)
				return mcpcontract.NewResult(identity, searchData{Account: identity.AccountID, Query: strings.Repeat("private", 1000)}), nil
			})
			cfg := fixtureConfig([]mcpcontract.Operation{operation})
			cfg.MaxBodyBytes = 512
			session := limitSession(t, cfg)

			query := "small"
			if largeInput {
				query = strings.Repeat("x", 512)
			}

			result, err := session.CallTool(t.Context(), searchParams(query))
			if err != nil {
				t.Fatal(err)
			}

			requireCategory(t, result, mcpcontract.InvalidInput)

			if largeInput && calls.Load() != 0 {
				t.Fatal("oversized input reached operation")
			}

			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), "private") {
				t.Fatal("oversized result leaked into tool response")
			}
		})
	}
}

func TestOversizedNonReplayableWriteIsOutcomeUnknown(t *testing.T) {
	t.Parallel()

	operation := limitOperation(func(_ context.Context, identity mcpcontract.Identity, _ searchInput) (mcpcontract.Result[searchData], error) {
		return mcpcontract.NewResult(identity, searchData{Account: identity.AccountID, Query: strings.Repeat("created-secret ", 1000)}), nil
	})
	operation.Definition.Retry = mcpcontract.NonReplayableWrite
	cfg := fixtureConfig([]mcpcontract.Operation{operation})
	cfg.EnableWrites = true
	cfg.MaxBodyBytes = 2048
	session := limitSession(t, cfg)

	result, err := session.CallTool(t.Context(), searchParams("create"))
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, result, mcpcontract.OutcomeUnknown)

	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "created-secret") {
		t.Fatal("oversized write leaked raw result")
	}

	if !strings.Contains(string(encoded), "account_id=work") || strings.Contains(string(encoded), `"retryable":true`) {
		t.Fatalf("missing bounded identity or retry: %s", encoded)
	}
}

func TestWriteEncodeFailureIsOutcomeUnknown(t *testing.T) {
	t.Parallel()

	operation := mcpcontract.NewOperation[searchInput, mcpcontract.Result[float64]]("gmail_search", nil, func(_ context.Context, _ mcpcontract.Identity, _ searchInput) (mcpcontract.Result[float64], error) {
		return mcpcontract.Result[float64]{AccountID: "work", Data: math.NaN()}, nil
	})
	operation.Definition.Retry = mcpcontract.NonReplayableWrite
	cfg := fixtureConfig([]mcpcontract.Operation{operation})
	cfg.EnableWrites = true
	session := limitSession(t, cfg)

	result, err := session.CallTool(t.Context(), searchParams("create"))
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, result, mcpcontract.OutcomeUnknown)

	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), `"retryable":true`) || strings.Contains(string(encoded), "NaN") {
		t.Fatalf("encode failure leaked or retryable: %s", encoded)
	}
}

type minWriteData struct {
	ID      string `json:"id"`
	Padding string `json:"padding"`
}

func TestMinBodyWriteFallbackFits(t *testing.T) {
	t.Parallel()

	const id15 = "abcdefghijklmno"

	operation := mcpcontract.NewOperation[searchInput, mcpcontract.Result[minWriteData]]("gmail_search", nil, func(_ context.Context, _ mcpcontract.Identity, _ searchInput) (mcpcontract.Result[minWriteData], error) {
		return mcpcontract.NewResult(mcpcontract.Identity{AccountID: id15, Label: "Work"}, minWriteData{ID: id15, Padding: strings.Repeat("p", 400)}), nil
	})
	operation.Definition.Retry = mcpcontract.NonReplayableWrite
	cfg := fixtureConfig([]mcpcontract.Operation{operation})
	cfg.EnableWrites = true
	cfg.MaxBodyBytes = 512
	session := limitSession(t, cfg)

	result, err := session.CallTool(t.Context(), searchParams("create"))
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, result, mcpcontract.OutcomeUnknown)

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	if int64(len(encoded)) > 512 {
		t.Fatalf("fallback %d bytes: %s", len(encoded), encoded)
	}

	if strings.Contains(string(encoded), `"retryable":true`) || strings.Contains(string(encoded), strings.Repeat("p", 40)) {
		t.Fatalf("fallback leaked payload or retry: %s", encoded)
	}
}

func TestClientVisibleResultIncludesSDKOverhead(t *testing.T) {
	t.Parallel()

	for _, padding := range []string{"x", strings.Repeat("x", 50), strings.Repeat("x", 200), strings.Repeat("x", 800)} {
		t.Run(strconv.Itoa(len(padding)), func(t *testing.T) {
			t.Parallel()

			operation := limitOperation(func(_ context.Context, id mcpcontract.Identity, _ searchInput) (mcpcontract.Result[searchData], error) {
				return mcpcontract.NewResult(id, searchData{Query: padding}), nil
			})
			cfg := fixtureConfig([]mcpcontract.Operation{operation})
			cfg.MaxBodyBytes = 512
			session := limitSession(t, cfg)

			result, err := session.CallTool(t.Context(), searchParams("q"))
			if err != nil {
				t.Fatal(err)
			}

			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}

			if int64(len(encoded)) > 512 {
				t.Fatalf("client-visible result has %d bytes: %s", len(encoded), encoded)
			}
		})
	}
}

func TestOversizedMailResultRetainsKnownResource(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"draft_id", "message_id"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			operation := mcpcontract.NewOperation[searchInput, mcpcontract.Result[map[string]string]]("gmail_search", nil, func(_ context.Context, id mcpcontract.Identity, _ searchInput) (mcpcontract.Result[map[string]string], error) {
				return mcpcontract.NewResult(id, map[string]string{key: "known-mail-id", "preview": strings.Repeat("p", 4000)}), nil
			})
			operation.Definition.Retry = mcpcontract.NonReplayableWrite
			cfg := fixtureConfig([]mcpcontract.Operation{operation})
			cfg.EnableWrites = true
			cfg.MaxBodyBytes = 2048
			session := limitSession(t, cfg)

			result, err := session.CallTool(t.Context(), searchParams("create"))
			if err != nil {
				t.Fatal(err)
			}

			requireCategory(t, result, mcpcontract.OutcomeUnknown)

			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(string(encoded), "resource_id=known-mail-id") {
				t.Fatalf("lost successful mail resource ID: %s", encoded)
			}
		})
	}
}
