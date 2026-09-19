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
		AccessToken:  "access",
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

	first, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy", ClientName: "default"})
	if err != nil {
		t.Fatalf("StartConnect personal: %v", err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt-personal", Subject: "sub-personal", Email: "me@gmail.com", Scopes: DefaultConnectScopes()}, nil
	}

	if _, cbErr := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "code-1", State: first.SessionID, BrowserID: first.SessionID, RedirectURL: ctrl.RedirectURL()}); cbErr != nil {
		t.Fatalf("complete personal: %v", cbErr)
	}

	second, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy", ClientName: "default", Label: "Work"})
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

	if _, err := ctrl.Disconnect(ctx, DisconnectRequest{PrincipalID: "jeremy", AccountID: first.Account.AccountID}); err == nil {
		t.Fatal("expected delete failure")
	}

	if len(inv.ids) == 0 || inv.ids[0] != first.Account.AccountID {
		t.Fatalf("invalidate first: %v", inv.ids)
	}

	if _, err := ctrl.GetRecord(ctx, first.Account.AccountID); err != nil {
		t.Fatalf("record should remain after failed delete: %v", err)
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
	if len(DefaultConnectScopes()) != 3 {
		t.Fatalf("default=%v", DefaultConnectScopes())
	}

	got := filterRequestedScopes(nil)
	if len(got) != 3 {
		t.Fatalf("empty request expanded: %v", got)
	}

	extended := reconnectScopes([]string{"https://www.googleapis.com/auth/gmail.readonly"}, []string{"https://www.googleapis.com/auth/calendar.readonly"})
	if len(extended) < 3 {
		t.Fatalf("reconnect extension %v", extended)
	}
	got = filterRequestedScopes([]string{"https://www.googleapis.com/auth/calendar.readonly"})
	foundGmail := false

	for _, scope := range got {
		if scope == "https://www.googleapis.com/auth/gmail.readonly" {
			foundGmail = true
		}
	}

	if !foundGmail {
		t.Fatalf("gmail omitted from nonempty request: %v", got)
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

type recordingInvalidator struct{ ids []string }

func (r *recordingInvalidator) InvalidateAccount(accountID string) {
	r.ids = append(r.ids, accountID)
}

var errRegistryFailed = errors.New("forced registry failure")
