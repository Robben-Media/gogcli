package access

import (
	"context"
	"sync"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

// AccountSource is the registry snapshot used for identity resolution.
type AccountSource interface {
	Get(ctx context.Context, accountID string) (mcpcontract.Identity, bool, error)
	List(ctx context.Context, principalID string) ([]mcpcontract.Identity, error)
}

// MemoryAccounts is a test double. Identities are cloned on the way out.
type MemoryAccounts struct {
	mu      sync.Mutex
	records map[string]mcpcontract.Identity
}

func NewMemoryAccounts(identities ...mcpcontract.Identity) *MemoryAccounts {
	accounts := &MemoryAccounts{records: make(map[string]mcpcontract.Identity, len(identities))}
	for _, identity := range identities {
		accounts.records[identity.AccountID] = identity.Clone()
	}

	return accounts
}

func (a *MemoryAccounts) Get(_ context.Context, accountID string) (mcpcontract.Identity, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	identity, ok := a.records[accountID]
	if !ok {
		return mcpcontract.Identity{}, false, nil
	}

	return identity.Clone(), true, nil
}

func (a *MemoryAccounts) List(_ context.Context, principalID string) ([]mcpcontract.Identity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := make([]mcpcontract.Identity, 0, len(a.records))
	for _, identity := range a.records {
		if principalID != "" && identity.PrincipalID != principalID {
			continue
		}
		out = append(out, identity.Clone())
	}

	return out, nil
}

func (a *MemoryAccounts) Put(identity mcpcontract.Identity) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.records == nil {
		a.records = make(map[string]mcpcontract.Identity)
	}
	a.records[identity.AccountID] = identity.Clone()
}

func (a *MemoryAccounts) Delete(accountID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.records, accountID)
}
