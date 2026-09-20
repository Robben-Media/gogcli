package accountconnect

import (
	"context"
	"fmt"
	"sync/atomic"
)

// Lifecycle is the single critical section for registry and refresh-token
// mutations. The MCP server must pass the same instance to Controller and the
// native Google client provider.
//
// Hold the lock for: registry Commit/Delete, token Get/Put/Delete,
// persistRotated registry/token checks, and HTTPClient cache fill from
// registry+token. Do not hold it across Google network calls (code exchange,
// API RoundTrip, remote revoke). SecretsTokenStore runs those token operations
// on one worker and fails closed if a caller deadline expires while waiting.
//
// InvalidateAccount may run while the lock is held and must not re-enter it.
// ConnectionsChanged must run only after Unlock.
type Lifecycle struct {
	mu   chan struct{}
	held atomic.Int32
}

func NewLifecycle() *Lifecycle {
	mu := make(chan struct{}, 1)
	mu <- struct{}{}

	return &Lifecycle{mu: mu}
}

// Lock acquires the critical section or returns ctx.Err() if the request is
// cancelled or expires while waiting. A nil Lifecycle is a no-op success.
func (l *Lifecycle) Lock(ctx context.Context) error {
	if l == nil {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("lock lifecycle: %w", err)
	}

	select {
	case <-l.mu:
		if err := ctx.Err(); err != nil {
			l.mu <- struct{}{}
			return fmt.Errorf("lock lifecycle: %w", err)
		}

		l.held.Add(1)

		return nil
	case <-ctx.Done():
		return fmt.Errorf("lock lifecycle: %w", ctx.Err())
	}
}

func (l *Lifecycle) Unlock() {
	if l == nil {
		return
	}

	l.held.Add(-1)

	select {
	case l.mu <- struct{}{}:
	default:
		panic("accountconnect: unlock of unlocked Lifecycle")
	}
}

// Held reports whether the lock is currently owned.
// It is for adapters and tests; it does not replace Lock.
func (l *Lifecycle) Held() bool {
	return l != nil && l.held.Load() > 0
}
