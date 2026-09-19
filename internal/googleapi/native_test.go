package googleapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

var errUnexpectedStatus = errors.New("unexpected status")

type metaToken struct {
	createdAt time.Time
	services  []string
}

type metaTokenStore struct {
	inner      *accountconnect.MemoryTokenStore
	mu         sync.Mutex
	extra      map[string]metaToken
	beforeSwap func()
}

func newMetaTokenStore() *metaTokenStore {
	return &metaTokenStore{inner: accountconnect.NewMemoryTokenStore(), extra: make(map[string]metaToken)}
}

func (s *metaTokenStore) key(clientName, email string) string {
	return clientName + "\x00" + email
}

func (s *metaTokenStore) Get(ctx context.Context, clientName, email string) (string, []string, error) {
	token, scopes, err := s.inner.Get(ctx, clientName, email)
	if err != nil {
		return "", nil, fmt.Errorf("get token: %w", err)
	}

	return token, scopes, nil
}

func (s *metaTokenStore) Put(ctx context.Context, clientName, email, token string, scopes []string) error {
	s.mu.Lock()
	key := s.key(clientName, email)

	extra, ok := s.extra[key]
	if !ok {
		extra = metaToken{createdAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), services: []string{"gmail"}}
	}
	s.extra[key] = extra
	s.mu.Unlock()

	if err := s.inner.Put(ctx, clientName, email, token, scopes); err != nil {
		return fmt.Errorf("put token: %w", err)
	}

	return nil
}

func (s *metaTokenStore) CompareAndSwap(ctx context.Context, clientName, email, expected, next string, scopes []string) (bool, error) {
	if s.beforeSwap != nil {
		s.beforeSwap()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, _, err := s.inner.Get(ctx, clientName, email)
	if err != nil {
		if accountconnect.IsTokenNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("cas get: %w", err)
	}

	if current != expected {
		return false, nil
	}

	key := s.key(clientName, email)
	if extra, ok := s.extra[key]; ok {
		s.extra[key] = extra
	}

	if err := s.inner.Put(ctx, clientName, email, next, scopes); err != nil {
		return false, fmt.Errorf("cas put: %w", err)
	}

	return true, nil
}

func (s *metaTokenStore) Delete(ctx context.Context, clientName, email string) error {
	s.mu.Lock()
	delete(s.extra, s.key(clientName, email))
	s.mu.Unlock()

	if err := s.inner.Delete(ctx, clientName, email); err != nil {
		return fmt.Errorf("delete token: %w", err)
	}

	return nil
}

func (s *metaTokenStore) snapshot(clientName, email string) (token string, extra metaToken, ok bool) {
	tok, _, err := s.inner.Get(context.Background(), clientName, email)
	if err != nil {
		return "", metaToken{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	extra, ok = s.extra[s.key(clientName, email)]

	return tok, extra, ok
}

type swapHandler struct {
	mu sync.Mutex
	h  http.Handler
}

func (s *swapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	h := s.h
	s.mu.Unlock()
	h.ServeHTTP(w, r)
}

func (s *swapHandler) set(h http.Handler) {
	s.mu.Lock()
	s.h = h
	s.mu.Unlock()
}

func TestNewNativeProviderRequiresDependencies(t *testing.T) {
	if _, err := NewNativeProvider(NativeOptions{}); !errors.Is(err, errNilNativeRegistry) {
		t.Fatalf("registry err = %v", err)
	}

	if _, err := NewNativeProvider(NativeOptions{Registry: accountconnect.NewMemoryRegistry()}); !errors.Is(err, errNilNativeTokens) {
		t.Fatalf("tokens err = %v", err)
	}

	if _, err := NewNativeProvider(NativeOptions{Registry: accountconnect.NewMemoryRegistry(), Tokens: newMetaTokenStore()}); !errors.Is(err, errNilNativeCredentials) {
		t.Fatalf("credentials err = %v", err)
	}

	provider, err := NewNativeProvider(NativeOptions{
		Registry:    accountconnect.NewMemoryRegistry(),
		Tokens:      newMetaTokenStore(),
		Credentials: func(string) (string, string, error) { return "id", "secret", nil },
	})
	if err != nil || provider == nil {
		t.Fatalf("NewNativeProvider: %v %v", provider, err)
	}
}

func TestNativeHTTPClientTwoAccountsReuseAndRefresh(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	scopes := []string{mcpcontract.GmailReadScope, "openid"}
	personal := fx.putAccount(t, "acct-personal", "sub-personal", "personal@gmail.com", "personal", scopes, "rt-personal")
	work := fx.putAccount(t, "acct-work", "sub-work", "work@company.com", "work", scopes, "rt-work")
	opts := mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead}

	firstPersonal, err := fx.provider.HTTPClient(context.Background(), personal, opts)
	if err != nil {
		t.Fatalf("personal client: %v", err)
	}

	secondPersonal, err := fx.provider.HTTPClient(context.Background(), personal, opts)
	if err != nil {
		t.Fatalf("personal reuse: %v", err)
	}

	workClient, err := fx.provider.HTTPClient(context.Background(), work, opts)
	if err != nil {
		t.Fatalf("work client: %v", err)
	}

	if got := mustDo(t, firstPersonal, fx.api.URL); got != "ok:acct-personal" {
		t.Fatalf("personal body = %q", got)
	}

	if got := mustDo(t, secondPersonal, fx.api.URL); got != "ok:acct-personal" {
		t.Fatalf("personal reuse body = %q", got)
	}

	if got := mustDo(t, workClient, fx.api.URL); got != "ok:acct-work" {
		t.Fatalf("work body = %q", got)
	}

	if fx.tokenCount("rt-personal") != 1 {
		t.Fatalf("personal token fetches = %d, want 1", fx.tokenCount("rt-personal"))
	}

	if fx.tokenCount("rt-work") != 1 {
		t.Fatalf("work token fetches = %d, want 1", fx.tokenCount("rt-work"))
	}

	reversed := personal
	reversed.Scopes = []string{"openid", mcpcontract.GmailReadScope}

	client, err := fx.provider.HTTPClient(context.Background(), reversed, opts)
	if err != nil {
		t.Fatalf("sorted scope cache: %v", err)
	}

	if got := mustDo(t, client, fx.api.URL); got != "ok:acct-personal" {
		t.Fatalf("reversed body = %q", got)
	}

	if fx.tokenCount("rt-personal") != 1 {
		t.Fatalf("scope order should reuse cache, fetches = %d", fx.tokenCount("rt-personal"))
	}
}

func TestNativeHTTPClientRefreshRotationPreservesMetadata(t *testing.T) {
	fx := newNativeFixture(t, 1)
	id := fx.putAccount(t, "acct-rot", "sub-rot", "rot@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-rot")
	created := time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)

	fx.tokens.mu.Lock()
	fx.tokens.extra[fx.tokens.key("personal", "rot@gmail.com")] = metaToken{createdAt: created, services: []string{"gmail", "drive"}}
	fx.tokens.mu.Unlock()

	fx.setRotate("rt-rot", "rt-rot-2", "acct-rot")

	client, err := fx.provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead})
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	if got := mustDo(t, client, fx.api.URL); got != "ok:acct-rot" {
		t.Fatalf("body = %q", got)
	}

	token, extra, ok := fx.tokens.snapshot("personal", "rot@gmail.com")
	if !ok {
		t.Fatal("missing rotated token")
	}

	if token != "rt-rot-2" {
		t.Fatalf("refresh token = %q", token)
	}

	if !extra.createdAt.Equal(created) {
		t.Fatalf("createdAt mutated: %s", extra.createdAt)
	}

	if strings.Join(extra.services, ",") != "gmail,drive" {
		t.Fatalf("services mutated: %v", extra.services)
	}
}

func TestNativeHTTPClientCancelFirstRequest(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	started := make(chan struct{})
	release := make(chan struct{})

	fx.apiHandler.set(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "missing auth", http.StatusUnauthorized)

			return
		}

		select {
		case started <- struct{}{}:
		default:
		}

		select {
		case <-release:
		case <-r.Context().Done():
			return
		}

		_, _ = io.WriteString(w, "ok:acct-cancel")
	}))

	id := fx.putAccount(t, "acct-cancel", "sub-cancel", "cancel@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-cancel")

	client, err := fx.provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead})
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, fx.api.URL, nil)
		if reqErr != nil {
			done <- reqErr

			return
		}

		resp, doErr := client.Do(req)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		done <- doErr
	}()

	<-started
	cancel()

	if err := <-done; err == nil {
		t.Fatal("expected canceled first request")
	}

	close(release)

	if got := mustDo(t, client, fx.api.URL); got != "ok:acct-cancel" {
		t.Fatalf("second request body = %q", got)
	}

	if fx.tokenCount("rt-cancel") != 1 {
		t.Fatalf("canceled request captured token source, fetches=%d", fx.tokenCount("rt-cancel"))
	}
}

func TestNativeHTTPClientCancelDuringRefresh(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})

	fx.tokenHandler.set(fx.tokenHTTP(func() {
		select {
		case entered <- struct{}{}:
		default:
		}

		<-release
	}))

	id := fx.putAccount(t, "acct-refresh-cancel", "sub-refresh-cancel", "refresh-cancel@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-refresh-cancel")

	client, err := fx.provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead})
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, fx.api.URL, nil)
		if reqErr != nil {
			done <- reqErr

			return
		}

		resp, doErr := client.Do(req)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		done <- doErr
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("token refresh did not start")
	}

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected canceled refresh wait")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("canceled caller did not return while token endpoint was stalled")
	}

	close(release)

	if got := mustDo(t, client, fx.api.URL); got != "ok:acct-refresh-cancel" {
		t.Fatalf("second request body = %q", got)
	}

	if fx.tokenCount("rt-refresh-cancel") != 1 {
		t.Fatalf("refresh was not single-flight, fetches=%d", fx.tokenCount("rt-refresh-cancel"))
	}
}

func TestNativeHTTPClientGenerationDisconnectAndNoResurrection(t *testing.T) {
	fx := newNativeFixture(t, 1)
	var blocked atomic.Bool
	entered := make(chan struct{}, 1)
	block := make(chan struct{})

	fx.tokenHandler.set(fx.tokenHTTP(func() {
		if blocked.Load() {
			select {
			case entered <- struct{}{}:
			default:
			}

			<-block
		}
	}))

	id := fx.putAccount(t, "acct-gen", "sub-gen", "gen@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-gen")
	opts := mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead}

	client, err := fx.provider.HTTPClient(context.Background(), id, opts)
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	if got := mustDo(t, client, fx.api.URL); got != "ok:acct-gen" {
		t.Fatalf("body = %q", got)
	}

	stale := id
	id.Generation = 2

	if upErr := fx.registry.Upsert(context.Background(), accountFromIdentity(id)); upErr != nil {
		t.Fatal(upErr)
	}

	if _, staleErr := fx.provider.HTTPClient(context.Background(), stale, opts); !isCategory(staleErr, mcpcontract.AuthRequired) {
		t.Fatalf("stale generation err = %v", err)
	}

	fx.provider.InvalidateAccount(id.AccountID)

	next, err := fx.provider.HTTPClient(context.Background(), id, opts)
	if err != nil {
		t.Fatalf("generation 2 client: %v", err)
	}

	if got := mustDo(t, next, fx.api.URL); got != "ok:acct-gen" {
		t.Fatalf("gen2 body = %q", got)
	}

	blocked.Store(true)
	fx.setRotate("rt-gen", "rt-resurrect", "acct-gen")
	done := make(chan error, 1)

	go func() {
		_, doErr := doAuthorized(next, fx.api.URL)
		done <- doErr
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not start")
	}

	if err := fx.life.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}

	fx.provider.InvalidateAccount(id.AccountID)

	if err := fx.registry.Delete(context.Background(), id.AccountID); err != nil {
		fx.life.Unlock()
		t.Fatal(err)
	}

	if err := fx.tokens.Delete(context.Background(), "personal", "gen@gmail.com"); err != nil {
		fx.life.Unlock()
		t.Fatal(err)
	}

	fx.life.Unlock()
	close(block)

	if err := <-done; err == nil {
		t.Fatal("expected in-flight refresh to fail after disconnect")
	}

	if _, _, ok := fx.tokens.snapshot("personal", "gen@gmail.com"); ok {
		t.Fatal("disconnected refresh token was resurrected")
	}

	if _, err := fx.provider.HTTPClient(context.Background(), id, opts); !isCategory(err, mcpcontract.AuthRequired) {
		t.Fatalf("deleted account err = %v", err)
	}
}

func TestNativeHTTPClientUnknownAndMismatchedRetry(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	id := fx.putAccount(t, "acct-op", "sub-op", "op@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-op")

	_, err := fx.provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "gmail_send", Retry: mcpcontract.SafeRead})
	if !isCategory(err, mcpcontract.InvalidInput) {
		t.Fatalf("unknown op err = %v", err)
	}

	if fx.tokenCount("rt-op") != 0 {
		t.Fatal("unknown operation must not refresh tokens")
	}

	_, err = fx.provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.NonReplayableWrite})
	if !isCategory(err, mcpcontract.InvalidInput) {
		t.Fatalf("mismatch err = %v", err)
	}

	_, err = fx.provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "accounts_list"})
	if !isCategory(err, mcpcontract.InvalidInput) {
		t.Fatalf("local op err = %v", err)
	}
}

func TestNativeHTTPClientScopesAuthModeAndDeadline(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	id := fx.putAccount(t, "acct-scope", "sub-scope", "scope@gmail.com", "personal", []string{"https://mail.google.com/"}, "rt-scope")
	opts := mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead}

	client, err := fx.provider.HTTPClient(context.Background(), id, opts)
	if err != nil {
		t.Fatalf("alias scope: %v", err)
	}

	if got := mustDo(t, client, fx.api.URL); got != "ok:acct-scope" {
		t.Fatalf("body = %q", got)
	}

	id.Scopes = []string{mcpcontract.CalendarReadScope}
	if err := fx.registry.Upsert(context.Background(), accountFromIdentity(id)); err != nil {
		t.Fatal(err)
	}

	fx.provider.InvalidateAccount(id.AccountID)

	if _, err := fx.provider.HTTPClient(context.Background(), id, opts); !isCategory(err, mcpcontract.InsufficientScope) {
		t.Fatalf("missing scope err = %v", err)
	}

	id.AuthMode = "service_account"
	if _, err := fx.provider.HTTPClient(context.Background(), id, opts); !isCategory(err, mcpcontract.AuthRequired) {
		t.Fatalf("service account err = %v", err)
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	id.AuthMode = accountconnect.AuthModeOAuth

	id.Scopes = []string{"https://mail.google.com/"}
	if _, err := fx.provider.HTTPClient(ctx, id, opts); !isCategory(err, mcpcontract.DeadlineExceeded) {
		t.Fatalf("deadline err = %v", err)
	}
}

func TestNativeHTTPClientRejectsForeignRedirect(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	var foreignAuth atomic.Int32
	var foreignHits atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignHits.Add(1)

		if r.Header.Get("Authorization") != "" {
			foreignAuth.Add(1)
		}

		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(foreign.Close)

	fx.apiHandler.set(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL+"/steal", http.StatusFound)
	}))

	id := fx.putAccount(t, "acct-redir", "sub-redir", "redir@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-redir")

	client, err := fx.provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead})
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	_, err = doAuthorized(client, fx.api.URL)
	if err == nil {
		t.Fatal("expected redirect rejection")
	}

	if foreignHits.Load() != 0 || foreignAuth.Load() != 0 {
		t.Fatalf("foreign host hits=%d auth=%d", foreignHits.Load(), foreignAuth.Load())
	}
}

func TestNativeHTTPClientRejectsPendingRecord(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	id := fx.putAccount(t, "acct-pending", "sub-pending", "pending@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-pending")
	rec := accountFromIdentity(id)

	rec.State = accountconnect.RecordStatePending
	if err := fx.registry.Upsert(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	opts := mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead}
	if _, err := fx.provider.HTTPClient(context.Background(), id, opts); !isCategory(err, mcpcontract.AuthRequired) {
		t.Fatalf("pending record err = %v", err)
	}

	rec.State = accountconnect.RecordStateDisconnecting
	if err := fx.registry.Upsert(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	fx.provider.InvalidateAccount(id.AccountID)

	if _, err := fx.provider.HTTPClient(context.Background(), id, opts); !isCategory(err, mcpcontract.AuthRequired) {
		t.Fatalf("disconnecting record err = %v", err)
	}
}

func TestNativeHTTPClientConcurrentAccounts(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	left := fx.putAccount(t, "acct-a", "sub-a", "a@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-a")
	right := fx.putAccount(t, "acct-b", "sub-b", "b@gmail.com", "work", []string{mcpcontract.GmailReadScope}, "rt-b")
	opts := mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead}

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for range 10 {
		wg.Add(2)
		go func() {
			defer wg.Done()

			client, err := fx.provider.HTTPClient(context.Background(), left, opts)
			if err != nil {
				errs <- err

				return
			}

			got, err := doAuthorized(client, fx.api.URL)
			if err != nil {
				errs <- err

				return
			}

			if got != "ok:acct-a" {
				errs <- fmt.Errorf("%w: %s", errUnexpectedStatus, got)
			}
		}()
		go func() {
			defer wg.Done()

			client, err := fx.provider.HTTPClient(context.Background(), right, opts)
			if err != nil {
				errs <- err

				return
			}

			got, err := doAuthorized(client, fx.api.URL)
			if err != nil {
				errs <- err

				return
			}

			if got != "ok:acct-b" {
				errs <- fmt.Errorf("%w: %s", errUnexpectedStatus, got)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatal(err)
	}
}

type nativeFixture struct {
	provider     *NativeProvider
	registry     *accountconnect.MemoryRegistry
	tokens       *metaTokenStore
	life         *accountconnect.Lifecycle
	api          *httptest.Server
	token        *httptest.Server
	apiHandler   *swapHandler
	tokenHandler *swapHandler
	mu           sync.Mutex
	expiresIn    int
	hits         map[string]int
	rotate       map[string]string
	accounts     map[string]string
}

func newNativeFixture(t *testing.T, expiresIn int) *nativeFixture {
	t.Helper()

	fx := &nativeFixture{
		registry:     accountconnect.NewMemoryRegistry(),
		tokens:       newMetaTokenStore(),
		apiHandler:   &swapHandler{},
		tokenHandler: &swapHandler{},
		expiresIn:    expiresIn,
		hits:         map[string]int{},
		rotate:       map[string]string{},
		accounts:     map[string]string{},
	}
	fx.apiHandler.set(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer at.") {
			http.Error(w, "missing auth", http.StatusUnauthorized)

			return
		}

		_, _ = io.WriteString(w, "ok:"+strings.TrimPrefix(auth, "Bearer at."))
	}))
	fx.tokenHandler.set(fx.tokenHTTP(nil))
	fx.api = httptest.NewServer(fx.apiHandler)
	t.Cleanup(fx.api.Close)
	fx.token = httptest.NewServer(fx.tokenHandler)
	t.Cleanup(fx.token.Close)

	fx.life = accountconnect.NewLifecycle()

	provider, err := NewNativeProvider(NativeOptions{
		Registry:    fx.registry,
		Tokens:      fx.tokens,
		Credentials: func(clientName string) (string, string, error) { return "id-" + clientName, "secret", nil },
		Endpoint:    oauth2.Endpoint{TokenURL: fx.token.URL, AuthURL: fx.token.URL + "/auth"},
		Lifecycle:   fx.life,
	})
	if err != nil {
		t.Fatalf("NewNativeProvider: %v", err)
	}

	fx.provider = provider

	return fx
}

func (fx *nativeFixture) tokenHTTP(before func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if before != nil {
			before()
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "form", http.StatusBadRequest)

			return
		}

		refresh := r.FormValue("refresh_token")

		fx.mu.Lock()
		fx.hits[refresh]++
		expires := fx.expiresIn
		rotated := fx.rotate[refresh]
		account := fx.accounts[refresh]
		fx.mu.Unlock()

		body := map[string]any{
			"access_token": "at." + account,
			"token_type":   "Bearer",
			"expires_in":   expires,
		}
		if rotated != "" {
			body["refresh_token"] = rotated
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
}

func (fx *nativeFixture) setRotate(from, to, accountID string) {
	fx.mu.Lock()
	fx.rotate[from] = to
	fx.accounts[to] = accountID
	fx.mu.Unlock()
}

func (fx *nativeFixture) tokenCount(refresh string) int {
	fx.mu.Lock()
	defer fx.mu.Unlock()

	return fx.hits[refresh]
}

func (fx *nativeFixture) putAccount(t *testing.T, accountID, subject, email, client string, scopes []string, refresh string) mcpcontract.Identity {
	t.Helper()

	rec := accountconnect.Record{
		AccountID:   accountID,
		Subject:     subject,
		Email:       email,
		Label:       email,
		PrincipalID: "principal",
		ClientName:  client,
		AuthMode:    accountconnect.AuthModeOAuth,
		Scopes:      append([]string(nil), scopes...),
		Generation:  1,
		UpdatedAt:   time.Now().UTC(),
	}
	if err := fx.registry.Upsert(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	if err := fx.tokens.Put(context.Background(), client, email, refresh, scopes); err != nil {
		t.Fatal(err)
	}

	fx.mu.Lock()
	fx.accounts[refresh] = accountID
	fx.mu.Unlock()

	return rec.Identity()
}

func accountFromIdentity(id mcpcontract.Identity) accountconnect.Record {
	return accountconnect.Record{
		AccountID:   id.AccountID,
		Subject:     id.Subject,
		Email:       id.Email,
		Label:       id.Label,
		PrincipalID: id.PrincipalID,
		ClientName:  id.ClientName,
		AuthMode:    id.AuthMode,
		Scopes:      append([]string(nil), id.Scopes...),
		Generation:  id.Generation,
		UpdatedAt:   id.UpdatedAt,
	}
}

func mustDo(t *testing.T, client *http.Client, rawURL string) string {
	t.Helper()

	body, err := doAuthorized(client, rawURL)
	if err != nil {
		t.Fatal(err)
	}

	return body
}

func doAuthorized(client *http.Client, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %s", errUnexpectedStatus, body)
	}

	return string(body), nil
}

func isCategory(err error, category mcpcontract.ErrorCategory) bool {
	var safe *mcpcontract.Error
	if !errors.As(err, &safe) {
		return false
	}

	return safe.Category == category
}

func TestNativeHTTPClientUsesLiveScopesAndPrincipal(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	id := fx.putAccount(t, "acct-authz", "sub-authz", "authz@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-authz")
	forged := id
	forged.PrincipalID = "other-principal"
	forged.Scopes = []string{mcpcontract.GmailReadScope, mcpcontract.DriveReadScope}
	opts := mcpcontract.CallOptions{Operation: "drive_search", Retry: mcpcontract.SafeRead}

	if _, err := fx.provider.HTTPClient(context.Background(), forged, opts); !isCategory(err, mcpcontract.AuthRequired) {
		t.Fatalf("forged principal err = %v", err)
	}

	id.Scopes = []string{mcpcontract.GmailReadScope, mcpcontract.DriveReadScope}
	if _, err := fx.provider.HTTPClient(context.Background(), id, opts); !isCategory(err, mcpcontract.InsufficientScope) {
		t.Fatalf("snapshot scope expansion err = %v", err)
	}
}

func TestNativeHTTPClientRespectsExpiredContextAfterLock(t *testing.T) {
	fx := newNativeFixture(t, 3600)
	id := fx.putAccount(t, "acct-lock", "sub-lock", "lock@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-lock")

	if err := fx.life.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	time.Sleep(30 * time.Millisecond)
	done := make(chan error, 1)

	go func() {
		_, err := fx.provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead})
		done <- err
	}()

	time.Sleep(20 * time.Millisecond)
	fx.life.Unlock()

	err := <-done
	if !isCategory(err, mcpcontract.DeadlineExceeded) {
		t.Fatalf("expired lock wait err = %v", err)
	}
}
