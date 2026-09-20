package accountconnect

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeProvider struct {
	mu        sync.Mutex
	last      AuthCodeParams
	verifiers map[string]struct{}
	exchange  func(ExchangeParams) (TokenSet, error)
	revokeErr error
	revoked   []string
}

func (f *fakeProvider) AuthCodeURL(params AuthCodeParams) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.last = params
	if f.verifiers == nil {
		f.verifiers = map[string]struct{}{}
	}

	if params.Verifier != "" {
		f.verifiers[params.Verifier] = struct{}{}
	}

	return "https://accounts.google.com/o/oauth2/auth?state=" + params.State, nil
}

func (f *fakeProvider) Exchange(_ context.Context, params ExchangeParams) (TokenSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if params.Verifier == "" {
		return TokenSet{}, ErrInvalidPKCE
	}

	if _, ok := f.verifiers[params.Verifier]; !ok {
		return TokenSet{}, ErrInvalidPKCE
	}

	if f.exchange != nil {
		return f.exchange(params)
	}

	return TokenSet{
		RefreshToken: "refresh-1",
		Subject:      "sub-personal",
		Email:        "me@gmail.com",
		Scopes:       []string{"https://www.googleapis.com/auth/gmail.readonly"},
	}, nil
}

func (f *fakeProvider) Revoke(_ context.Context, refreshToken string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked = append(f.revoked, refreshToken)

	return f.revokeErr
}

func testController(t *testing.T, provider *fakeProvider) *Controller {
	t.Helper()

	ctrl, err := NewController(Options{
		Registry: NewMemoryRegistry(),
		Tokens:   NewMemoryTokenStore(),
		OAuth:    provider,
		Credentials: func(string) (ClientCredentials, error) {
			return ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		Now:         func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) },
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	return ctrl
}

func TestConnectTwoAccountsCoexist(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	ctx := context.Background()

	first, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatalf("StartConnect personal: %v", err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-personal", Subject: "sub-personal", Email: "me@gmail.com", Scopes: DefaultConnectScopes()}, nil
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "code-1", State: first.SessionID, BrowserID: first.SessionID, RedirectURL: ctrl.RedirectURL()}); cbErr != nil {
		t.Fatalf("complete personal: %v", cbErr)
	}

	second, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy", Label: "Work"})
	if err != nil {
		t.Fatalf("StartConnect work: %v", err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-work", Subject: "sub-work", Email: "me@company.com", Scopes: DefaultConnectScopes()}, nil
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "code-2", State: second.SessionID, BrowserID: second.SessionID, RedirectURL: ctrl.RedirectURL()}); cbErr != nil {
		t.Fatalf("complete work: %v", cbErr)
	}

	views, err := ctrl.ListAccounts(ctx, "jeremy")
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}

	if len(views) != 2 {
		t.Fatalf("got %d accounts", len(views))
	}

	if views[0].AccountID == views[1].AccountID {
		t.Fatal("account IDs must differ")
	}
}

func TestCallbackRejectsStateAndPKCEAndRedirect(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	ctx := context.Background()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatalf("StartConnect: %v", err)
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: "nope", BrowserID: "nope", RedirectURL: ctrl.RedirectURL()}); !errors.Is(cbErr, ErrInvalidState) {
		t.Fatalf("state: %v", cbErr)
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: "http://evil.example/callback"}); !errors.Is(cbErr, ErrInvalidRedirect) {
		t.Fatalf("redirect: %v", cbErr)
	}

	started, err = ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatalf("StartConnect 2: %v", err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) { return TokenSet{}, ErrInvalidPKCE }

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()}); !errors.Is(cbErr, ErrInvalidPKCE) {
		t.Fatalf("pkce: %v", cbErr)
	}
}

func TestMissingRefreshTokenOnFirstConnect(t *testing.T) {
	provider := &fakeProvider{exchange: func(ExchangeParams) (TokenSet, error) {
		return TokenSet{Subject: "sub", Email: "me@gmail.com", Scopes: []string{"openid"}}, nil
	}}
	ctrl := testController(t, provider)
	ctx := context.Background()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatalf("StartConnect: %v", err)
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()}); !errors.Is(cbErr, ErrMissingRefresh) {
		t.Fatalf("missing refresh: %v", cbErr)
	}
}

func TestReconnectReusesRefreshAndBumpsGeneration(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	ctx := context.Background()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatalf("StartConnect: %v", err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-1", Subject: "sub-personal", Email: "me@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}

	first, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	rec, err := ctrl.GetRecord(ctx, first.Account.AccountID)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}

	if rec.Generation != 1 {
		t.Fatalf("generation=%d", rec.Generation)
	}

	re, err := ctrl.StartReconnect(ctx, ReconnectRequest{PrincipalID: "jeremy", AccountID: rec.AccountID})
	if err != nil {
		t.Fatalf("StartReconnect: %v", err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{Subject: "sub-personal", Email: "me@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/calendar.readonly"}}, nil
	}

	second, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c2", State: re.SessionID, BrowserID: re.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatalf("reconnect complete: %v", err)
	}

	if second.Created {
		t.Fatal("reconnect must update the same record")
	}

	updated, err := ctrl.GetRecord(ctx, rec.AccountID)
	if err != nil {
		t.Fatalf("GetRecord 2: %v", err)
	}

	if updated.Generation != 2 {
		t.Fatalf("generation=%d", updated.Generation)
	}

	token, _, err := ctrl.tokens.Get(ctx, updated.ClientName, updated.Email)
	if err != nil || token != "rt-1" {
		t.Fatalf("reused refresh: %q %v", token, err)
	}

	if len(updated.Scopes) != 2 {
		t.Fatalf("scopes=%v", updated.Scopes)
	}
}

func TestReconnectRejectsSubjectChange(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	ctx := context.Background()
	started, _ := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-1", Subject: "sub-personal", Email: "me@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}

	first, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	re, _ := ctrl.StartReconnect(ctx, ReconnectRequest{PrincipalID: "jeremy", AccountID: first.Account.AccountID})
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-2", Subject: "sub-other", Email: "other@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c2", State: re.SessionID, BrowserID: re.SessionID, RedirectURL: ctrl.RedirectURL()}); !errors.Is(cbErr, ErrSubjectMismatch) {
		t.Fatalf("subject: %v", cbErr)
	}
}

func TestDisconnectIsolatesOtherAccounts(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	ctx := context.Background()

	a, _ := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-a", Subject: "sub-a", Email: "a@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}
	first, _ := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "1", State: a.SessionID, BrowserID: a.SessionID, RedirectURL: ctrl.RedirectURL()})

	b, _ := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-b", Subject: "sub-b", Email: "b@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}
	second, _ := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "2", State: b.SessionID, BrowserID: b.SessionID, RedirectURL: ctrl.RedirectURL()})

	got, err := ctrl.Disconnect(ctx, DisconnectRequest{PrincipalID: "jeremy", AccountID: first.Account.AccountID})
	if err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	if !got.RevokedRemote || !got.OtherDeploymentsMayBeAffected {
		t.Fatalf("result=%+v", got)
	}

	if !strings.Contains(got.Message, "other deployments") && !strings.Contains(strings.ToLower(got.Message), "other") {
		t.Fatalf("message=%q", got.Message)
	}

	if _, err := ctrl.GetRecord(ctx, first.Account.AccountID); !errors.Is(err, ErrUnknownAccount) {
		t.Fatalf("deleted: %v", err)
	}

	if _, err := ctrl.GetRecord(ctx, second.Account.AccountID); err != nil {
		t.Fatalf("other account: %v", err)
	}

	if _, _, err := ctrl.tokens.Get(ctx, "default", "b@gmail.com"); err != nil {
		t.Fatalf("other token: %v", err)
	}
}

func TestListDoesNotLeakOtherPrincipal(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	ctx := context.Background()
	started, _ := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt", Subject: "sub", Email: "me@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()}); cbErr != nil {
		t.Fatalf("complete: %v", cbErr)
	}

	views, err := ctrl.ListAccounts(ctx, "other")
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(views) != 0 {
		t.Fatalf("leaked %v", views)
	}
}

func TestCallbackRejectsForeignBrowser(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	ctx := context.Background()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: "other-browser", RedirectURL: ctrl.RedirectURL()}); !errors.Is(cbErr, ErrInvalidState) {
		t.Fatalf("foreign browser: %v", cbErr)
	}
}

func TestCallbackFailsClosedWithoutGrantedScopes(t *testing.T) {
	provider := &fakeProvider{exchange: func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt", Subject: "sub", Email: "me@gmail.com"}, nil
	}}
	ctrl := testController(t, provider)
	ctx := context.Background()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()}); !errors.Is(cbErr, ErrMissingGrantedScopes) {
		t.Fatalf("empty grant: %v", cbErr)
	}

	views, err := ctrl.ListAccounts(ctx, "jeremy")
	if err != nil || len(views) != 0 {
		t.Fatalf("leaked record %v %v", views, err)
	}
}

func TestCallbackDoesNotStoreTokenIfRegistryFails(t *testing.T) {
	provider := &fakeProvider{exchange: func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-new", Subject: "sub", Email: "me@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}}
	tokens := NewMemoryTokenStore()
	reg := &failingRegistry{inner: NewMemoryRegistry(), failCommit: true}

	ctrl, err := NewController(Options{
		Registry: reg,
		Tokens:   tokens,
		OAuth:    provider,
		Credentials: func(string) (ClientCredentials, error) {
			return ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		Now:         func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) },
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()}); err == nil {
		t.Fatal("expected registry failure")
	}

	if _, _, err := tokens.Get(ctx, "default", "me@gmail.com"); !IsTokenNotFound(err) {
		t.Fatalf("token stored despite registry failure: %v", err)
	}
}

func TestDisconnectInvalidatesBeforeRegistryFailure(t *testing.T) {
	provider := &fakeProvider{}
	inv := &recordingInvalidator{}
	reg := &failingRegistry{inner: NewMemoryRegistry()}

	ctrl, err := NewController(Options{
		Registry:    reg,
		Tokens:      NewMemoryTokenStore(),
		OAuth:       provider,
		Invalidator: inv,
		Credentials: func(string) (ClientCredentials, error) {
			return ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		Now:         func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) },
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	started, _ := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt", Subject: "sub", Email: "me@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}

	first, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatal(err)
	}
	reg.failDelete = true
	inv.ids = nil

	got, err := ctrl.Disconnect(ctx, DisconnectRequest{PrincipalID: "jeremy", AccountID: first.Account.AccountID})
	if err == nil {
		t.Fatal("expected delete failure")
	}

	if !got.Retryable || got.State != RecordStateDisconnecting {
		t.Fatalf("retryable disconnecting: %+v", got)
	}

	if len(inv.ids) == 0 || inv.ids[0] != first.Account.AccountID {
		t.Fatalf("invalidate first: %v", inv.ids)
	}

	if _, err := ctrl.GetRecord(ctx, first.Account.AccountID); !errors.Is(err, ErrAccountUnavailable) {
		t.Fatalf("inactive record: %v", err)
	}
}

func TestConcurrentCallbacksShareSubject(t *testing.T) {
	provider := &fakeProvider{exchange: func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt", Subject: "sub-same", Email: "same@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}}
	ctrl := testController(t, provider)
	ctx := context.Background()

	firstStart, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}

	secondStart, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}
	var start, done sync.WaitGroup
	start.Add(2)
	done.Add(2)
	errCh := make(chan error, 2)

	run := func(sessionID string) {
		defer done.Done()

		start.Done()
		start.Wait()

		_, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: sessionID, BrowserID: sessionID, RedirectURL: ctrl.RedirectURL()})
		errCh <- cbErr
	}
	go run(firstStart.SessionID)
	go run(secondStart.SessionID)

	done.Wait()
	close(errCh)

	for cbErr := range errCh {
		if cbErr != nil {
			t.Fatalf("callback: %v", cbErr)
		}
	}

	views, err := ctrl.ListAccounts(ctx, "jeremy")
	if err != nil {
		t.Fatal(err)
	}

	if len(views) != 1 {
		t.Fatalf("duplicate records: %+v", views)
	}
}

func TestEmailChangeReusesOldTokenKey(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	ctx := context.Background()
	started, _ := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-old", Subject: "sub-stable", Email: "old@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}

	first, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatal(err)
	}

	re, err := ctrl.StartReconnect(ctx, ReconnectRequest{PrincipalID: "jeremy", AccountID: first.Account.AccountID})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{Subject: "sub-stable", Email: "new@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}

	second, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c2", State: re.SessionID, BrowserID: re.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatalf("email change: %v", err)
	}

	if second.Account.AccountID != first.Account.AccountID || second.Account.Email != "new@gmail.com" {
		t.Fatalf("account %+v", second.Account)
	}

	token, _, err := ctrl.tokens.Get(ctx, "default", "new@gmail.com")
	if err != nil || token != "rt-old" {
		t.Fatalf("migrated token %q %v", token, err)
	}

	if _, _, err := ctrl.tokens.Get(ctx, "default", "old@gmail.com"); !IsTokenNotFound(err) {
		t.Fatalf("old key remains: %v", err)
	}
}

func TestDefaultConnectScopesAreGmailOnly(t *testing.T) {
	ctrl := testController(t, &fakeProvider{})
	if len(DefaultConnectScopes()) != 3 {
		t.Fatalf("default=%v", DefaultConnectScopes())
	}

	got := ctrl.filterRequestedScopes(nil)
	if len(got) != 3 {
		t.Fatalf("empty request expanded: %v", got)
	}

	extended := ctrl.reconnectScopes([]string{"https://www.googleapis.com/auth/gmail.readonly"}, []string{"https://www.googleapis.com/auth/calendar.readonly"})
	if len(extended) < 3 {
		t.Fatalf("reconnect extension %v", extended)
	}
	got = ctrl.filterRequestedScopes([]string{"https://www.googleapis.com/auth/calendar.readonly"})
	foundGmail := false
	foundCal := false

	for _, scope := range got {
		if scope == "https://www.googleapis.com/auth/gmail.readonly" {
			foundGmail = true
		}

		if scope == "https://www.googleapis.com/auth/calendar.readonly" {
			foundCal = true
		}
	}

	if foundGmail || !foundCal {
		t.Fatalf("explicit calendar request forced gmail: %v", got)
	}

	empty := ctrl.filterRequestedScopes([]string{})
	for _, scope := range empty {
		if scope == "https://www.googleapis.com/auth/gmail.readonly" {
			t.Fatalf("explicit empty request forced gmail: %v", empty)
		}
	}
}

func TestRejectsSharedMailboxAcrossPrincipals(t *testing.T) {
	provider := &fakeProvider{exchange: func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt", Subject: "sub-shared", Email: "shared@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}}
	ctrl := testController(t, provider)
	ctx := context.Background()

	first, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: first.SessionID, BrowserID: first.SessionID, RedirectURL: ctrl.RedirectURL()}); cbErr != nil {
		t.Fatal(cbErr)
	}

	second, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "other"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c2", State: second.SessionID, BrowserID: second.SessionID, RedirectURL: ctrl.RedirectURL()}); !errors.Is(err, ErrAccountOwned) {
		t.Fatalf("shared mailbox: %v", err)
	}
}

type failingRegistry struct {
	inner      Registry
	failCommit bool
	failDelete bool
}

func (r *failingRegistry) Get(ctx context.Context, accountID string) (Record, bool, error) {
	rec, ok, err := r.inner.Get(ctx, accountID)
	if err != nil {
		return Record{}, false, fmt.Errorf("get: %w", err)
	}

	return rec, ok, nil
}

func (r *failingRegistry) List(ctx context.Context, principalID string) ([]Record, error) {
	recs, err := r.inner.List(ctx, principalID)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}

	return recs, nil
}

func (r *failingRegistry) Upsert(ctx context.Context, record Record) error {
	if err := r.inner.Upsert(ctx, record); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}

	return nil
}

func (r *failingRegistry) Delete(ctx context.Context, accountID string) error {
	if r.failDelete {
		return errRegistryFailed
	}

	if err := r.inner.Delete(ctx, accountID); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	return nil
}

func (r *failingRegistry) Commit(ctx context.Context, rec Record) (Record, Record, bool, error) {
	if r.failCommit {
		return Record{}, Record{}, false, errRegistryFailed
	}

	committed, previous, existed, err := r.inner.Commit(ctx, rec)
	if err != nil {
		return Record{}, Record{}, false, fmt.Errorf("commit: %w", err)
	}

	return committed, previous, existed, nil
}

func (r *failingRegistry) RevokeEpoch(clientName, subject string) (uint64, error) {
	epoch, err := r.inner.RevokeEpoch(clientName, subject)
	if err != nil {
		return 0, fmt.Errorf("revoke epoch: %w", err)
	}

	return epoch, nil
}

func (r *failingRegistry) BumpRevokeEpoch(clientName, subject string) (uint64, error) {
	epoch, err := r.inner.BumpRevokeEpoch(clientName, subject)
	if err != nil {
		return 0, fmt.Errorf("bump revoke epoch: %w", err)
	}

	return epoch, nil
}

func (r *failingRegistry) Epochs() (map[string]uint64, error) {
	epochs, err := r.inner.Epochs()
	if err != nil {
		return nil, fmt.Errorf("epochs: %w", err)
	}

	return epochs, nil
}

type recordingInvalidator struct {
	ids        []string
	notify     int
	held       []bool
	notifyHeld []bool
	life       *Lifecycle
}

func (r *recordingInvalidator) InvalidateAccount(accountID string) {
	r.ids = append(r.ids, accountID)
	if r.life != nil {
		r.held = append(r.held, r.life.Held())
	}
}

func (r *recordingInvalidator) ConnectionsChanged() {
	r.notify++
	if r.life != nil {
		r.notifyHeld = append(r.notifyHeld, r.life.Held())
	}
}

var errRegistryFailed = errors.New("forced registry failure")

func TestCommitRejectsEmailMatchWithDifferentSubject(t *testing.T) {
	reg := NewMemoryRegistry()
	ctx := t.Context()

	first := Record{
		AccountID: "account-a", Subject: "subject-1", Email: "same@gmail.com",
		PrincipalID: "jeremy", ClientName: "default", AuthMode: AuthModeOAuth,
		Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}, Generation: 1, State: RecordStateActive,
	}
	if _, _, _, err := reg.Commit(ctx, first); err != nil {
		t.Fatal(err)
	}

	second := first
	second.AccountID = "account-b"

	second.Subject = "subject-2"
	if _, _, _, err := reg.Commit(ctx, second); !errors.Is(err, ErrSubjectMismatch) {
		t.Fatalf("email reuse transferred account: %v", err)
	}

	stored, ok, err := reg.Get(ctx, "account-a")
	if err != nil || !ok || stored.Subject != "subject-1" {
		t.Fatalf("original record mutated: %+v ok=%v err=%v", stored, ok, err)
	}
}

func TestListAccountsRetriesTokenKeyCleanup(t *testing.T) {
	ctrl := testController(t, &fakeProvider{})
	ctx := t.Context()

	rec := Record{
		AccountID: "account-a", Subject: "subject", Email: "new@gmail.com",
		PrincipalID: "jeremy", ClientName: "default", AuthMode: AuthModeOAuth,
		Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}, Generation: 1, State: RecordStateActive,
		Cleanup: &Cleanup{Kind: CleanupTokenKey, ClientName: "default", Email: "old@gmail.com"},
	}
	if err := ctrl.registry.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}

	if err := ctrl.tokens.Put(ctx, "default", "old@gmail.com", "old-token", rec.Scopes); err != nil {
		t.Fatal(err)
	}

	views, err := ctrl.ListAccounts(ctx, "jeremy")
	if err != nil {
		t.Fatal(err)
	}

	if len(views) != 1 || views[0].CleanupPending {
		t.Fatalf("cleanup not retried: %+v", views)
	}

	if _, _, err := ctrl.tokens.Get(ctx, "default", "old@gmail.com"); !IsTokenNotFound(err) {
		t.Fatalf("old token remained: %v", err)
	}
}

func TestDisconnectMissingRefreshCompletesLocal(t *testing.T) {
	ctrl := testController(t, &fakeProvider{})
	ctx := t.Context()

	rec := Record{
		AccountID: "account-a", Subject: "subject", Email: "me@gmail.com",
		PrincipalID: "jeremy", ClientName: "default", AuthMode: AuthModeOAuth,
		Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}, Generation: 1, State: RecordStateActive,
	}
	if err := ctrl.registry.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}

	got, err := ctrl.Disconnect(ctx, DisconnectRequest{PrincipalID: "jeremy", AccountID: rec.AccountID})
	if err != nil {
		t.Fatalf("missing token disconnect: %v", err)
	}

	if got.RevokedRemote || got.Retryable {
		t.Fatalf("expected local-only completion: %+v", got)
	}

	if _, err := ctrl.GetRecord(ctx, rec.AccountID); !errors.Is(err, ErrUnknownAccount) {
		t.Fatalf("row remained: %v", err)
	}
}

func TestCapabilitiesForScopesAcceptsBroaderGrant(t *testing.T) {
	got := capabilitiesForScopes([]string{"https://www.googleapis.com/auth/gmail.modify"})
	if len(got) != 1 || got[0] != "gmail.read" {
		t.Fatalf("modify should grant gmail.read: %v", got)
	}
}

func TestConnectRejectsSameEmailDifferentSubject(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	ctx := t.Context()

	firstStart, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-1", Subject: "subject-1", Email: "same@gmail.com", Scopes: []string{mcpcontractGmail()}}, nil
	}

	first, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: firstStart.SessionID, BrowserID: firstStart.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatal(err)
	}

	secondStart, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-2", Subject: "subject-2", Email: "same@gmail.com", Scopes: []string{mcpcontractGmail()}}, nil
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c2", State: secondStart.SessionID, BrowserID: secondStart.SessionID, RedirectURL: ctrl.RedirectURL()}); !errors.Is(cbErr, ErrSubjectMismatch) {
		t.Fatalf("same email different subject: %v", cbErr)
	}

	stored, err := ctrl.GetRecord(ctx, first.Account.AccountID)
	if err != nil {
		t.Fatal(err)
	}

	if stored.Subject != "subject-1" || stored.Generation != 1 {
		t.Fatalf("original identity mutated: %+v", stored)
	}

	token, _, err := ctrl.tokens.Get(ctx, stored.ClientName, stored.Email)
	if err != nil || token != "rt-1" {
		t.Fatalf("original grant overwritten: %q %v", token, err)
	}
}

func mcpcontractGmail() string {
	return "https://www.googleapis.com/auth/gmail.readonly"
}

func TestEmailChangeKeepsCleanupDebtWhenOldKeyDeleteFails(t *testing.T) {
	provider := &fakeProvider{}
	tokens := &scriptedTokenStore{inner: NewMemoryTokenStore()}

	ctrl, err := NewController(Options{
		Registry: NewMemoryRegistry(),
		Tokens:   tokens,
		OAuth:    provider,
		Credentials: func(string) (ClientCredentials, error) {
			return ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		Now:         func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) },
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-old", Subject: "sub-stable", Email: "old@gmail.com", Scopes: []string{mcpcontractGmail()}}, nil
	}

	first, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatal(err)
	}

	re, err := ctrl.StartReconnect(ctx, ReconnectRequest{PrincipalID: "jeremy", AccountID: first.Account.AccountID})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{Subject: "sub-stable", Email: "new@gmail.com", Scopes: []string{mcpcontractGmail()}}, nil
	}
	tokens.deleteErr = errTokenDeleteFailed

	second, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c2", State: re.SessionID, BrowserID: re.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatalf("email change with cleanup failure: %v", err)
	}

	if !second.CleanupPending {
		t.Fatalf("expected cleanup debt: %+v", second)
	}

	if _, _, getErr := tokens.Get(ctx, "default", "old@gmail.com"); getErr != nil {
		t.Fatalf("old key should remain for retry: %v", getErr)
	}

	rec, ok, err := ctrl.registry.Get(ctx, first.Account.AccountID)
	if err != nil || !ok {
		t.Fatalf("load account: ok=%v err=%v", ok, err)
	}

	if rec.Cleanup == nil || rec.Cleanup.Kind != CleanupTokenKey || rec.Cleanup.Email != "old@gmail.com" {
		t.Fatalf("cleanup debt not persisted: %+v", rec)
	}

	tokens.deleteErr = nil

	views, err := ctrl.ListAccounts(ctx, "jeremy")
	if err != nil {
		t.Fatal(err)
	}

	if len(views) != 1 || views[0].CleanupPending {
		t.Fatalf("cleanup retry did not finish: %+v", views)
	}

	if _, _, err := tokens.Get(ctx, "default", "old@gmail.com"); !IsTokenNotFound(err) {
		t.Fatalf("old key remained after retry: %v", err)
	}
}

func TestConnectionsChangedRunsAfterUnlock(t *testing.T) {
	provider := &fakeProvider{}
	inv := &recordingInvalidator{}

	ctrl, err := NewController(Options{
		Registry:    NewMemoryRegistry(),
		Tokens:      NewMemoryTokenStore(),
		OAuth:       provider,
		Invalidator: inv,
		Credentials: func(string) (ClientCredentials, error) {
			return ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		Now:         func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) },
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	inv.life = ctrl.Lifecycle()
	ctx := t.Context()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt", Subject: "sub", Email: "me@gmail.com", Scopes: []string{mcpcontractGmail()}}, nil
	}

	first, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatal(err)
	}

	if inv.notify == 0 {
		t.Fatal("connect did not notify connection change")
	}

	for _, held := range inv.notifyHeld {
		if held {
			t.Fatal("ConnectionsChanged ran while Lifecycle was held")
		}
	}

	if _, err := ctrl.Disconnect(ctx, DisconnectRequest{PrincipalID: "jeremy", AccountID: first.Account.AccountID}); err != nil {
		t.Fatal(err)
	}

	if inv.notify < 2 {
		t.Fatalf("disconnect did not notify: %d", inv.notify)
	}

	for _, held := range inv.notifyHeld {
		if held {
			t.Fatal("ConnectionsChanged ran while Lifecycle was held")
		}
	}
}

func TestScopeChoicesDoNotRequireGmail(t *testing.T) {
	for _, choice := range ScopeChoices() {
		if choice.Required {
			t.Fatalf("scope choice marked required: %+v", choice)
		}
	}
}

type scriptedTokenStore struct {
	inner     *MemoryTokenStore
	deleteErr error
}

func (s *scriptedTokenStore) Get(ctx context.Context, clientName, email string) (string, []string, error) {
	token, scopes, err := s.inner.Get(ctx, clientName, email)
	if err != nil {
		return "", nil, fmt.Errorf("scripted get: %w", err)
	}

	return token, scopes, nil
}

func (s *scriptedTokenStore) Put(ctx context.Context, clientName, email, token string, scopes []string) error {
	if err := s.inner.Put(ctx, clientName, email, token, scopes); err != nil {
		return fmt.Errorf("scripted put: %w", err)
	}

	return nil
}

func (s *scriptedTokenStore) Delete(ctx context.Context, clientName, email string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}

	if err := s.inner.Delete(ctx, clientName, email); err != nil {
		return fmt.Errorf("scripted delete: %w", err)
	}

	return nil
}

var errTokenDeleteFailed = errors.New("forced token delete failure")

type blockingPutStore struct {
	inner   *MemoryTokenStore
	mu      sync.Mutex
	block   bool
	started chan struct{}
	release chan struct{}
}

func (s *blockingPutStore) Get(ctx context.Context, clientName, email string) (string, []string, error) {
	token, scopes, err := s.inner.Get(ctx, clientName, email)
	if err != nil {
		return "", nil, fmt.Errorf("blocking get: %w", err)
	}

	return token, scopes, nil
}

func (s *blockingPutStore) Put(ctx context.Context, clientName, email, token string, scopes []string) error {
	s.mu.Lock()
	block := s.block
	s.mu.Unlock()

	if block {
		select {
		case <-s.started:
		default:
			close(s.started)
		}

		select {
		case <-s.release:
		case <-ctx.Done():
			return fmt.Errorf("blocking put: %w", ctx.Err())
		}
	}

	if err := s.inner.Put(ctx, clientName, email, token, scopes); err != nil {
		return fmt.Errorf("blocking put: %w", err)
	}

	return nil
}

func (s *blockingPutStore) Delete(ctx context.Context, clientName, email string) error {
	if err := s.inner.Delete(ctx, clientName, email); err != nil {
		return fmt.Errorf("blocking delete: %w", err)
	}

	return nil
}

func TestPutTimeoutKeepsPendingRekeyCleanup(t *testing.T) {
	provider := &fakeProvider{}
	tokens := &blockingPutStore{inner: NewMemoryTokenStore(), started: make(chan struct{}), release: make(chan struct{})}

	ctrl, err := NewController(Options{
		Registry: NewMemoryRegistry(), Tokens: tokens, OAuth: provider,
		Credentials: func(string) (ClientCredentials, error) {
			return ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		Now:         func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) },
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-old", Subject: "sub-stable", Email: "old@gmail.com", Scopes: []string{mcpcontractGmail()}}, nil
	}

	first, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatal(err)
	}

	re, err := ctrl.StartReconnect(ctx, ReconnectRequest{PrincipalID: "jeremy", AccountID: first.Account.AccountID})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-new", Subject: "sub-stable", Email: "new@gmail.com", Scopes: []string{mcpcontractGmail()}}, nil
	}

	tokens.mu.Lock()
	tokens.block = true
	tokens.mu.Unlock()

	putCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)

	go func() {
		_, cbErr := ctrl.CompleteCallback(putCtx, CallbackRequest{Code: "c2", State: re.SessionID, BrowserID: re.SessionID, RedirectURL: ctrl.RedirectURL()})
		done <- cbErr
	}()

	select {
	case <-tokens.started:
	case <-time.After(2 * time.Second):
		t.Fatal("put did not start")
	}

	cbErr := <-done
	if !errors.Is(cbErr, context.DeadlineExceeded) {
		t.Fatalf("expected unknown put: %v", cbErr)
	}

	rec, ok, err := ctrl.registry.Get(ctx, first.Account.AccountID)
	if err != nil || !ok {
		t.Fatalf("pending row missing: ok=%v err=%v", ok, err)
	}

	if rec.State != RecordStatePending || rec.Email != "new@gmail.com" {
		t.Fatalf("pending identity: %+v", rec)
	}

	if rec.Cleanup == nil || rec.Cleanup.Kind != CleanupTokenKey || rec.Cleanup.Email != "old@gmail.com" {
		t.Fatalf("old email cleanup missing on pending: %+v", rec)
	}

	if rec.Subject != "sub-stable" {
		t.Fatalf("subject changed: %+v", rec)
	}

	close(tokens.release)
}

func TestDisconnectPreservesUnsettledTokenKeyDebt(t *testing.T) {
	ctrl := testController(t, &fakeProvider{})
	ctx := t.Context()
	tokens := &scriptedTokenStore{inner: NewMemoryTokenStore(), deleteErr: errTokenDeleteFailed}
	ctrl.tokens = tokens

	rec := Record{
		AccountID: "account-a", Subject: "subject", Email: "new@gmail.com",
		PrincipalID: "jeremy", ClientName: "default", AuthMode: AuthModeOAuth,
		Scopes: []string{mcpcontractGmail()}, Generation: 1, State: RecordStateActive,
		Cleanup: &Cleanup{Kind: CleanupTokenKey, ClientName: "default", Email: "old@gmail.com"},
	}
	if err := ctrl.registry.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}

	if err := tokens.inner.Put(ctx, "default", "old@gmail.com", "old-token", rec.Scopes); err != nil {
		t.Fatal(err)
	}

	result, err := ctrl.Disconnect(ctx, DisconnectRequest{PrincipalID: "jeremy", AccountID: rec.AccountID})
	if !errors.Is(err, ErrCleanupPending) || result.State != RecordStateActive || !result.Retryable {
		t.Fatalf("disconnect overwrote debt: result=%+v err=%v", result, err)
	}

	stored, ok, getErr := ctrl.registry.Get(ctx, rec.AccountID)
	if getErr != nil || !ok || stored.State != RecordStateActive || stored.Cleanup == nil || stored.Cleanup.Kind != CleanupTokenKey || stored.Cleanup.Email != "old@gmail.com" {
		t.Fatalf("debt lost: %+v ok=%v err=%v", stored, ok, getErr)
	}
}

func TestRekeyDoesNotReplaceUnsettledCleanupDebt(t *testing.T) {
	provider := &fakeProvider{}
	tokens := &scriptedTokenStore{inner: NewMemoryTokenStore(), deleteErr: errTokenDeleteFailed}

	ctrl, err := NewController(Options{
		Registry: NewMemoryRegistry(), Tokens: tokens, OAuth: provider,
		Credentials: func(string) (ClientCredentials, error) {
			return ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		Now:         func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) },
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-a", Subject: "sub-stable", Email: "a@gmail.com", Scopes: []string{mcpcontractGmail()}}, nil
	}

	first, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatal(err)
	}

	re, err := ctrl.StartReconnect(ctx, ReconnectRequest{PrincipalID: "jeremy", AccountID: first.Account.AccountID})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-b", Subject: "sub-stable", Email: "b@gmail.com", Scopes: []string{mcpcontractGmail()}}, nil
	}

	second, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c2", State: re.SessionID, BrowserID: re.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatalf("A->B: %v", err)
	}

	if !second.CleanupPending {
		t.Fatalf("expected A cleanup debt: %+v", second)
	}

	re2, err := ctrl.StartReconnect(ctx, ReconnectRequest{PrincipalID: "jeremy", AccountID: first.Account.AccountID})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-c", Subject: "sub-stable", Email: "c@gmail.com", Scopes: []string{mcpcontractGmail()}}, nil
	}

	if _, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c3", State: re2.SessionID, BrowserID: re2.SessionID, RedirectURL: ctrl.RedirectURL()}); !errors.Is(err, ErrCleanupPending) {
		t.Fatalf("B->C replaced debt: %v", err)
	}

	stored, ok, getErr := ctrl.registry.Get(ctx, first.Account.AccountID)
	if getErr != nil || !ok || stored.Email != "b@gmail.com" || stored.Cleanup == nil || stored.Cleanup.Email != "a@gmail.com" {
		t.Fatalf("A debt lost after B->C: %+v ok=%v err=%v", stored, ok, getErr)
	}
}

func TestDisconnectLocalFailureWithoutTokenNotRevokedRemote(t *testing.T) {
	provider := &fakeProvider{}
	reg := &failingRegistry{inner: NewMemoryRegistry(), failDelete: true}

	ctrl, err := NewController(Options{
		Registry: reg, Tokens: NewMemoryTokenStore(), OAuth: provider,
		Credentials: func(string) (ClientCredentials, error) {
			return ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		Now:         func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) },
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	rec := Record{
		AccountID: "account-a", Subject: "subject", Email: "me@gmail.com",
		PrincipalID: "jeremy", ClientName: "default", AuthMode: AuthModeOAuth,
		Scopes: []string{mcpcontractGmail()}, Generation: 1, State: RecordStateActive,
	}
	if err := ctrl.registry.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}

	got, discErr := ctrl.Disconnect(ctx, DisconnectRequest{PrincipalID: "jeremy", AccountID: rec.AccountID})
	if discErr == nil || got.RevokedRemote || !got.Retryable {
		t.Fatalf("missing token local failure: %+v err=%v", got, discErr)
	}
}

var errMutatedPut = errors.New("put reported failure after mutation")

type mutatingPutStore struct {
	inner    *MemoryTokenStore
	failNext bool
}

func (s *mutatingPutStore) Get(ctx context.Context, clientName, email string) (string, []string, error) {
	token, scopes, err := s.inner.Get(ctx, clientName, email)
	if err != nil {
		return "", nil, fmt.Errorf("mutating get: %w", err)
	}

	return token, scopes, nil
}

func (s *mutatingPutStore) Put(ctx context.Context, clientName, email, token string, scopes []string) error {
	if err := s.inner.Put(ctx, clientName, email, token, scopes); err != nil {
		return fmt.Errorf("mutating put: %w", err)
	}

	if s.failNext {
		return errMutatedPut
	}

	return nil
}

func (s *mutatingPutStore) Delete(ctx context.Context, clientName, email string) error {
	if err := s.inner.Delete(ctx, clientName, email); err != nil {
		return fmt.Errorf("mutating delete: %w", err)
	}

	return nil
}

func TestPutErrorAfterMutationLeavesPendingAndNotifies(t *testing.T) {
	provider := &fakeProvider{}
	tokens := &mutatingPutStore{inner: NewMemoryTokenStore(), failNext: true}
	inv := &recordingInvalidator{}

	ctrl, err := NewController(Options{
		Registry: NewMemoryRegistry(), Tokens: tokens, OAuth: provider, Invalidator: inv,
		Credentials: func(string) (ClientCredentials, error) {
			return ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		Now:         func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) },
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	inv.life = ctrl.Lifecycle()
	ctx := t.Context()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "new-token", Subject: "subject", Email: "me@example.test", Scopes: []string{mcpcontractGmail(), "https://www.googleapis.com/auth/calendar.readonly"}}, nil
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()}); !errors.Is(cbErr, errMutatedPut) {
		t.Fatalf("callback: %v", cbErr)
	}

	if inv.notify == 0 {
		t.Fatal("durable pending change did not notify")
	}

	for _, held := range inv.notifyHeld {
		if held {
			t.Fatal("ConnectionsChanged ran while Lifecycle was held")
		}
	}

	recs, err := ctrl.registry.List(ctx, "jeremy")
	if err != nil || len(recs) != 1 {
		t.Fatalf("records: %v %v", recs, err)
	}

	stored := recs[0]
	if stored.State != RecordStatePending || len(stored.Scopes) != 2 {
		t.Fatalf("expected pending new grant: %+v", stored)
	}

	if _, getErr := ctrl.GetRecord(ctx, stored.AccountID); !errors.Is(getErr, ErrAccountUnavailable) {
		t.Fatalf("pending remained usable: %v", getErr)
	}

	token, scopes, err := tokens.inner.Get(ctx, stored.ClientName, stored.Email)
	if err != nil || token != "new-token" || len(scopes) != 2 {
		t.Fatalf("token=%q scopes=%v err=%v", token, scopes, err)
	}
}
