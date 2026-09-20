package accountconnect

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

var errReviewProviderRevoke = errors.New("provider revoke failed")

type spanningRevokeProvider struct {
	revokeEntered   chan struct{}
	releaseRevoke   chan struct{}
	exchangeEntered chan struct{}
	releaseExchange chan struct{}
}

func (p *spanningRevokeProvider) AuthCodeURL(a AuthCodeParams) (string, error) {
	return "https://example.test/auth?state=" + a.State, nil
}

func (p *spanningRevokeProvider) Exchange(context.Context, ExchangeParams) (TokenSet, error) {
	close(p.exchangeEntered)
	<-p.releaseExchange

	return TokenSet{RefreshToken: "new-token", Subject: "subject", Email: "me@gmail.com", Scopes: []string{mcpcontract.GmailReadScope}}, nil
}

func (p *spanningRevokeProvider) Revoke(context.Context, string) error {
	close(p.revokeEntered)
	<-p.releaseRevoke

	return nil
}

func newLifecycleReviewController(t *testing.T, reg Registry, tokens RefreshTokenStore, provider Provider) *Controller {
	t.Helper()

	ctrl, err := NewController(Options{
		Registry: reg, Tokens: tokens, OAuth: provider,
		Credentials: func(string) (ClientCredentials, error) {
			return ClientCredentials{ClientID: "client", ClientSecret: "secret"}, nil
		},
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}

	return ctrl
}

func seedLifecycleReviewAccount(t *testing.T, reg *MemoryRegistry, tokens *MemoryTokenStore) Record {
	t.Helper()

	rec := Record{AccountID: "old-account", Subject: "subject", Email: "me@gmail.com", PrincipalID: "jeremy", ClientName: "default", AuthMode: AuthModeOAuth, Scopes: []string{mcpcontract.GmailReadScope}, Generation: 1, State: RecordStateActive}
	if err := reg.Upsert(t.Context(), rec); err != nil {
		t.Fatal(err)
	}

	if err := tokens.Put(t.Context(), rec.ClientName, rec.Email, "old-token", rec.Scopes); err != nil {
		t.Fatal(err)
	}

	return rec
}

func TestReviewExchangeStartedDuringRevokeCannotActivateAfterRevoke(t *testing.T) {
	reg := NewMemoryRegistry()
	tokens := NewMemoryTokenStore()
	rec := seedLifecycleReviewAccount(t, reg, tokens)
	provider := &spanningRevokeProvider{
		revokeEntered: make(chan struct{}), releaseRevoke: make(chan struct{}),
		exchangeEntered: make(chan struct{}), releaseExchange: make(chan struct{}),
	}
	ctrl := newLifecycleReviewController(t, reg, tokens, provider)

	started, err := ctrl.StartConnect(t.Context(), ConnectRequest{PrincipalID: rec.PrincipalID})
	if err != nil {
		t.Fatal(err)
	}

	disconnectDone := make(chan error, 1)

	go func() {
		_, disconnectErr := ctrl.Disconnect(context.Background(), DisconnectRequest{PrincipalID: rec.PrincipalID, AccountID: rec.AccountID})
		disconnectDone <- disconnectErr
	}()

	<-provider.revokeEntered

	callbackDone := make(chan error, 1)

	go func() {
		_, callbackErr := ctrl.CompleteCallback(context.Background(), CallbackRequest{Code: "code", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()})
		callbackDone <- callbackErr
	}()

	<-provider.exchangeEntered

	close(provider.releaseRevoke)

	if err := <-disconnectDone; err != nil {
		t.Fatal(err)
	}

	close(provider.releaseExchange)

	if err := <-callbackDone; !errors.Is(err, ErrRevokeInProgress) {
		t.Fatalf("callback spanning revoke activated after revoke: %v", err)
	}
}

func TestReviewFailedRevokeRetainsRetrySecret(t *testing.T) {
	reg := NewMemoryRegistry()
	tokens := NewMemoryTokenStore()
	rec := seedLifecycleReviewAccount(t, reg, tokens)
	provider := &fakeProvider{revokeErr: errReviewProviderRevoke}
	ctrl := newLifecycleReviewController(t, reg, tokens, provider)

	result, err := ctrl.Disconnect(t.Context(), DisconnectRequest{PrincipalID: rec.PrincipalID, AccountID: rec.AccountID})
	if err == nil || !result.Retryable || result.State != RecordStateDisconnecting {
		t.Fatalf("failed revoke was not retained as retryable cleanup: result=%+v err=%v", result, err)
	}

	if token, _, getErr := tokens.Get(t.Context(), rec.ClientName, rec.Email); getErr != nil || token != "old-token" {
		t.Fatalf("failed revoke lost retry token: token=%q err=%v", token, getErr)
	}

	stored, ok, getErr := reg.Get(t.Context(), rec.AccountID)
	if getErr != nil || !ok || stored.State != RecordStateDisconnecting || stored.Cleanup == nil || stored.Cleanup.Kind != CleanupRevoke {
		t.Fatalf("failed revoke lost durable cleanup record: record=%+v ok=%v err=%v", stored, ok, getErr)
	}
}

func TestReviewFileRegistryEpochBumpMustPersistOrFail(t *testing.T) {
	// /proc accepts reads of missing paths as not-found but cannot persist a new
	// registry file, which exercises BumpRevokeEpoch's ignored save error.
	path := filepath.Join("/proc", "gogcli-accountconnect-epoch-review.json")

	reg, err := NewFileRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	bumped, bumpErr := reg.BumpRevokeEpoch("default", "subject")
	if bumpErr == nil {
		t.Fatal("epoch bump unexpectedly succeeded on an unwritable registry path")
	}

	reopened, err := NewFileRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	persisted, readErr := reopened.RevokeEpoch("default", "subject")
	if readErr != nil && !errors.Is(readErr, bumpErr) {
		// Both operations should expose their underlying persistence failures. The
		// exact wrapped errors may differ by operation.
		t.Logf("reopen error after failed bump: %v", readErr)
	}

	if persisted != bumped {
		t.Fatalf("epoch bump reported %d but persisted %d; registry API suppressed persistence failure", bumped, persisted)
	}
}

func TestReviewFailedRevokeRetryCompletes(t *testing.T) {
	reg := NewMemoryRegistry()
	tokens := NewMemoryTokenStore()
	rec := seedLifecycleReviewAccount(t, reg, tokens)
	provider := &fakeProvider{revokeErr: errReviewProviderRevoke}
	ctrl := newLifecycleReviewController(t, reg, tokens, provider)

	result, err := ctrl.Disconnect(t.Context(), DisconnectRequest{PrincipalID: rec.PrincipalID, AccountID: rec.AccountID})
	if err == nil || !result.Retryable {
		t.Fatalf("first revoke: result=%+v err=%v", result, err)
	}

	provider.revokeErr = nil

	result, err = ctrl.Disconnect(t.Context(), DisconnectRequest{PrincipalID: rec.PrincipalID, AccountID: rec.AccountID})
	if err != nil {
		t.Fatalf("retry disconnect: %v", err)
	}

	if !result.RevokedRemote || result.Retryable {
		t.Fatalf("retry result=%+v", result)
	}

	if _, _, getErr := tokens.Get(t.Context(), rec.ClientName, rec.Email); !IsTokenNotFound(getErr) {
		t.Fatalf("retry left token: %v", getErr)
	}

	if _, ok, getErr := reg.Get(t.Context(), rec.AccountID); getErr != nil || ok {
		t.Fatalf("retry left record ok=%v err=%v", ok, getErr)
	}
}
