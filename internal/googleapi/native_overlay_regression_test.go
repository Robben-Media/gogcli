package googleapi

import (
	"context"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestBiteStalledRefreshDoesNotConsumeOnlyAPISlot(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	fx.provider.slots = make(chan struct{}, 1)
	stalled := fx.putAccount(t, "acct-stalled", "sub-stalled", "stalled@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-stalled")
	warm := fx.putAccount(t, "acct-warm", "sub-warm", "warm@gmail.com", "work", []string{mcpcontract.GmailReadScope}, "rt-warm")
	opts := mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead}

	stalledClient, err := fx.provider.HTTPClient(context.Background(), stalled, opts)
	if err != nil {
		t.Fatal(err)
	}

	warmClient, err := fx.provider.HTTPClient(context.Background(), warm, opts)
	if err != nil {
		t.Fatal(err)
	}

	if got := mustDo(t, warmClient, fx.api.URL); got != "ok:acct-warm" {
		t.Fatalf("warm body = %q", got)
	}

	entered := make(chan struct{}, 1)
	release := make(chan struct{})

	fx.tokenHandler.set(fx.tokenHTTP(func() {
		select {
		case entered <- struct{}{}:
		default:
		}

		<-release
	}))

	stalledDone := make(chan error, 1)

	go func() {
		_, doErr := doAuthorized(stalledClient, fx.api.URL)
		stalledDone <- doErr
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("stalled refresh did not start")
	}

	warmDone := make(chan error, 1)

	go func() {
		_, doErr := doAuthorized(warmClient, fx.api.URL)
		warmDone <- doErr
	}()

	select {
	case err := <-warmDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("stalled refresh consumed the only API slot")
	}

	close(release)

	if err := <-stalledDone; err != nil {
		t.Fatal(err)
	}
}

func TestBiteOldRotationCannotOverwriteNewerReconnect(t *testing.T) {
	fx := newNativeFixture(t, 1)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})

	fx.tokenHandler.set(fx.tokenHTTP(func() {
		select {
		case entered <- struct{}{}:
		default:
		}

		<-release
	}))
	id := fx.putAccount(t, "acct-reconnect", "sub-reconnect", "reconnect@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-old")
	fx.setRotate("rt-old", "rt-rotated-old", id.AccountID)

	client, err := fx.provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)

	go func() {
		_, doErr := doAuthorized(client, fx.api.URL)
		done <- doErr
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("old refresh did not start")
	}

	if err := fx.life.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}

	next := accountFromIdentity(id)

	next.Generation = 2
	if err := fx.registry.Upsert(context.Background(), next); err != nil {
		fx.life.Unlock()
		t.Fatal(err)
	}

	if err := fx.tokens.Put(context.Background(), next.ClientName, next.Email, "rt-new", next.Scopes); err != nil {
		fx.life.Unlock()
		t.Fatal(err)
	}

	fx.provider.InvalidateAccount(id.AccountID)
	fx.life.Unlock()
	close(release)

	if err := <-done; err == nil {
		t.Fatal("old refresh unexpectedly remained authorized")
	}

	token, _, ok := fx.tokens.snapshot(next.ClientName, next.Email)
	if !ok || token != "rt-new" {
		t.Fatalf("new reconnect token overwritten: token=%q ok=%v", token, ok)
	}
}
