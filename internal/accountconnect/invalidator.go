package accountconnect

// Invalidator drops cached Google clients/token sources after credential changes.
// Native providers must key caches by account ID and generation; InvalidateAccount
// evicts every generation for that account and must fence in-flight token sources
// so a later refresh cannot persist a rotated refresh token for that account.
//
// InvalidateAccount may be invoked while Lifecycle is held. Implementations must
// not call Lifecycle.Lock (self-deadlock). Cache fencing belongs here.
//
// ConnectionsChanged is invoked only after Lifecycle is released, once a durable
// connect/disconnect mutation has committed or a disconnect left no registry
// record. The MCP entrypoint uses it to ResyncTools against committed state.
type Invalidator interface {
	InvalidateAccount(accountID string)
	ConnectionsChanged()
}

// NopInvalidator is used when no client cache exists yet.
type NopInvalidator struct{}

func (NopInvalidator) InvalidateAccount(string) {}

func (NopInvalidator) ConnectionsChanged() {}
