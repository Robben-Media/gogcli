package accountconnect

import "sync"

// Lifecycle is the single critical section for registry and refresh-token
// mutations. The MCP server must pass the same instance to Controller and the
// native Google client provider.
//
// Hold the lock for: registry Commit/Delete, token Put/Delete, native
// persistRotated final registry/token checks, and HTTPClient cache fill from
// registry+token. Do not hold it across Google network calls (code exchange,
// API RoundTrip, remote revoke).
type Lifecycle struct {
	mu sync.Mutex
}

func NewLifecycle() *Lifecycle {
	return &Lifecycle{}
}

func (l *Lifecycle) Lock() {
	if l != nil {
		l.mu.Lock()
	}
}

func (l *Lifecycle) Unlock() {
	if l != nil {
		l.mu.Unlock()
	}
}
