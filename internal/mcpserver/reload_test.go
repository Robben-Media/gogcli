package mcpserver_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

type pausedAccounts struct {
	*access.MemoryAccounts
	pause   atomic.Bool
	entered chan struct{}
	resume  chan struct{}
}

func (a *pausedAccounts) List(ctx context.Context, principal string) ([]mcpcontract.Identity, error) {
	if a.pause.CompareAndSwap(true, false) {
		close(a.entered)
		<-a.resume
	}

	identities, err := a.MemoryAccounts.List(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("paused accounts: %w", err)
	}

	return identities, nil
}

func TestReloadCannotBeOverwrittenByStaleResync(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	provider := &countingProvider{}
	accounts := &pausedAccounts{MemoryAccounts: fixtureAccounts(), entered: make(chan struct{}), resume: make(chan struct{})}
	cfg := fixtureConfig([]mcpcontract.Operation{fakeSearch(provider)})
	cfg.Accounts = accounts

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, runtime)
	defer session.Close()

	accounts.pause.Store(true)
	resynced := make(chan struct{})

	go func() { runtime.ResyncTools(); close(resynced) }()

	select {
	case <-accounts.entered:
	case <-ctx.Done():
		t.Fatal("resync did not enter registry")
	}

	reloaded := make(chan error, 1)
	go func() {
		reloaded <- runtime.ReloadAccess(access.Snapshot{Grants: cfg.Grants, AllowOperations: []string{"accounts_list"}})
	}()

	select {
	case reloadErr := <-reloaded:
		close(accounts.resume)
		<-resynced
		t.Fatalf("reload passed in-flight old snapshot: %v", reloadErr)
	case <-time.After(50 * time.Millisecond):
	}

	close(accounts.resume)
	<-resynced

	if reloadErr := <-reloaded; reloadErr != nil {
		t.Fatal(reloadErr)
	}

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(tools.Tools) != 1 || tools.Tools[0].Name != "accounts_list" {
		t.Fatalf("stale tool catalog: %#v", tools.Tools)
	}
}
