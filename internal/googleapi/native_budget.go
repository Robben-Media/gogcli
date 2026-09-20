package googleapi

import (
	"context"
	"sync/atomic"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

type nativeBudgetKey struct{}

type nativeCallBudget struct {
	limit int64
	used  atomic.Int64
}

// WithUpstreamBudget bounds Google API HTTP attempts, including retries. Nested
// operations inherit the established budget instead of creating a fresh one.
// OAuth refresh is separately bounded by the provider and is not an API quota
// unit. This counter does not claim to measure Google's method-specific quotas.
func WithUpstreamBudget(ctx context.Context, maxCalls int64) context.Context {
	if _, ok := ctx.Value(nativeBudgetKey{}).(*nativeCallBudget); ok {
		return ctx
	}

	if maxCalls < 0 {
		maxCalls = 0
	}

	return context.WithValue(ctx, nativeBudgetKey{}, &nativeCallBudget{limit: maxCalls})
}

// UpstreamBudgetRemaining returns -1 if the caller did not install a budget.
func UpstreamBudgetRemaining(ctx context.Context) int64 {
	budget, ok := ctx.Value(nativeBudgetKey{}).(*nativeCallBudget)
	if !ok {
		return -1
	}

	return max(int64(0), budget.limit-budget.used.Load())
}

func consumeUpstreamBudget(ctx context.Context) error {
	budget, ok := ctx.Value(nativeBudgetKey{}).(*nativeCallBudget)
	if !ok {
		return nil
	}

	for {
		used := budget.used.Load()
		if used >= budget.limit {
			return &mcpcontract.Error{Category: mcpcontract.BudgetExhausted, Message: "API call budget exhausted; narrow the task or explicitly start a new bounded request", Retryable: false}
		}

		if budget.used.CompareAndSwap(used, used+1) {
			return nil
		}
	}
}
