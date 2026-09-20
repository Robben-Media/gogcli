package googleapi

import (
	"context"
	"sync/atomic"
)

type nativeUpstreamKey struct{}

// WithUpstreamCounter attaches an API attempt counter to ctx.
// OAuth refresh uses a detached context and is not counted.
func WithUpstreamCounter(ctx context.Context) (context.Context, *atomic.Int64) {
	if ctx == nil {
		ctx = context.Background()
	}

	counter := &atomic.Int64{}

	return context.WithValue(ctx, nativeUpstreamKey{}, counter), counter
}

// AddUpstreamCall increments the request's Google API attempt counter.
// NativeRetryTransport calls this immediately before each base RoundTrip, including retries.
func AddUpstreamCall(ctx context.Context) {
	if ctx == nil {
		return
	}

	counter, ok := ctx.Value(nativeUpstreamKey{}).(*atomic.Int64)
	if !ok || counter == nil {
		return
	}

	counter.Add(1)
}

// UpstreamCalls returns the number of native Google API RoundTrip attempts on ctx.
func UpstreamCalls(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}

	counter, ok := ctx.Value(nativeUpstreamKey{}).(*atomic.Int64)
	if !ok || counter == nil {
		return 0
	}

	return counter.Load()
}
