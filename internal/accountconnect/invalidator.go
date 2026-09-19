package accountconnect

// Invalidator drops cached Google clients/token sources after credential changes.
// Native providers must key caches by account ID and generation; InvalidateAccount
// evicts every generation for that account and must fence in-flight token sources
// so a later refresh cannot persist a rotated refresh token for that account.
type Invalidator interface {
	InvalidateAccount(accountID string)
}

// NopInvalidator is used when no client cache exists yet.
type NopInvalidator struct{}

func (NopInvalidator) InvalidateAccount(string) {}
