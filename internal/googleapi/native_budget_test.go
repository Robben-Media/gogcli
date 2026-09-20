package googleapi

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestBudgetCountsRetriesAndStopsBeforeExtraRequest(t *testing.T) {
	ctx, counter := WithUpstreamCounter(WithUpstreamBudget(t.Context(), 1))
	script := &scriptedTransport{responses: []*http.Response{{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("quota"))}}}
	transport := &NativeRetryTransport{Base: script, Class: mcpcontract.SafeRead, BaseDelay: time.Nanosecond}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.RoundTrip(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.BudgetExhausted || safe.Retryable {
		t.Fatalf("budget error: %v", err)
	}

	if script.calls != 1 || counter.Load() != 1 || UpstreamBudgetRemaining(ctx) != 0 {
		t.Fatalf("attempts=%d counter=%d remaining=%d", script.calls, counter.Load(), UpstreamBudgetRemaining(ctx))
	}
}

func TestBudgetCannotBeResetAndConcurrentCallsStayBounded(t *testing.T) {
	ctx := WithUpstreamBudget(t.Context(), 7)
	ctx = WithUpstreamBudget(ctx, 1000)
	var successes atomic.Int64

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if consumeUpstreamBudget(ctx) == nil {
				successes.Add(1)
			}
		})
	}

	wg.Wait()

	if successes.Load() != 7 || UpstreamBudgetRemaining(ctx) != 0 {
		t.Fatalf("consumed %d", successes.Load())
	}
}

func TestWriteWithNoBudgetNeverReachesNetwork(t *testing.T) {
	script := &scriptedTransport{}
	transport := &NativeRetryTransport{Base: script, Class: mcpcontract.NonReplayableWrite}

	req, err := http.NewRequestWithContext(WithUpstreamBudget(t.Context(), 0), http.MethodPost, "https://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.RoundTrip(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.BudgetExhausted || script.calls != 0 {
		t.Fatalf("error=%v calls=%d", err, script.calls)
	}
}
