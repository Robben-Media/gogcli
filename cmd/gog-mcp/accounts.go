package main

import (
	"context"
	"fmt"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

type registryAccounts struct {
	registry accountconnect.Registry
}

func (r registryAccounts) Get(ctx context.Context, accountID string) (mcpcontract.Identity, bool, error) {
	record, ok, err := r.registry.Get(ctx, accountID)
	if err != nil {
		return mcpcontract.Identity{}, false, fmt.Errorf("lookup account: %w", err)
	}
	if !ok || !record.IsActive() {
		return mcpcontract.Identity{}, false, nil
	}

	return record.Identity().Clone(), true, nil
}

func (r registryAccounts) List(ctx context.Context, principalID string) ([]mcpcontract.Identity, error) {
	records, err := r.registry.List(ctx, principalID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	out := make([]mcpcontract.Identity, 0, len(records))
	for _, record := range records {
		if !record.IsActive() {
			continue
		}

		out = append(out, record.Identity().Clone())
	}

	return out, nil
}
