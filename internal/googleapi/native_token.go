package googleapi

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

type nativeTokenSource struct {
	provider    *NativeProvider
	accountID   string
	email       string
	clientName  string
	principalID string
	generation  uint64
	revokeEpoch uint64
	fence       uint64
	invalid     atomic.Bool

	mu       sync.Mutex
	inner    oauth2.TokenSource
	refresh  string
	current  *oauth2.Token
	inflight *nativeRefreshWait
}

func (s *nativeTokenSource) invalidate() {
	if s == nil {
		return
	}

	s.invalid.Store(true)
}

func (s *nativeTokenSource) blocked() bool {
	if s == nil || s.invalid.Load() {
		return true
	}

	if s.provider != nil && s.provider.accountFenced(s.accountID, s.fence) {
		s.invalid.Store(true)

		return true
	}

	return false
}

type nativeRefreshWait struct {
	done chan struct{}
	tok  *oauth2.Token
	err  error
}

func (s *nativeTokenSource) token(ctx context.Context) (*oauth2.Token, error) {
	if ctx == nil {
		return nil, &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "request context is required"}
	}

	if s.blocked() {
		return nil, &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	wait := s.beginRefresh(ctx)
	if wait == nil {
		s.mu.Lock()
		tok := s.current
		s.mu.Unlock()

		return tok, nil
	}

	select {
	case <-wait.done:
		if wait.err != nil {
			return nil, wait.err
		}

		return wait.tok, nil
	case <-ctx.Done():
		return nil, NativePublicError(ctx.Err())
	}
}

func (s *nativeTokenSource) beginRefresh(ctx context.Context) *nativeRefreshWait {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current != nil && s.current.Valid() {
		return nil
	}

	if s.inflight != nil {
		return s.inflight
	}

	wait := &nativeRefreshWait{done: make(chan struct{})}
	s.inflight = wait

	go s.runRefresh(ctx, wait)

	return wait
}

func (s *nativeTokenSource) persistTimeout() time.Duration {
	if s.provider != nil && s.provider.timeout > 0 {
		return s.provider.timeout
	}

	return defaultHTTPTimeout
}

func (s *nativeTokenSource) runRefresh(ctx context.Context, wait *nativeRefreshWait) {
	tok, err := s.inner.Token()
	if err != nil {
		if s.blocked() {
			err = &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
		} else {
			err = NativePublicError(err)
		}

		tok = nil
	} else if persistErr := s.persistRotated(ctx, tok); persistErr != nil {
		err = persistErr
		tok = nil
	} else if s.blocked() {
		err = &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
		tok = nil
	}

	s.mu.Lock()
	s.inflight = nil

	if err == nil {
		s.current = tok
	}

	wait.tok = tok
	wait.err = err
	close(wait.done)
	s.mu.Unlock()
}

type nativeAuthTransport struct {
	source *nativeTokenSource
	base   http.RoundTripper
}

func (t *nativeAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "request is required"}
	}

	tok, err := t.source.token(req.Context())
	if err != nil {
		return nil, err
	}

	authorized := req.Clone(req.Context())
	tok.SetAuthHeader(authorized)

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(authorized)
	if err != nil {
		return nil, fmt.Errorf("googleapi: google request: %w", err)
	}

	return resp, nil
}

func (s *nativeTokenSource) persistRotated(ctx context.Context, tok *oauth2.Token) error {
	if tok == nil || tok.RefreshToken == "" {
		return nil
	}

	s.mu.Lock()
	if s.blocked() {
		s.mu.Unlock()

		return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	if tok.RefreshToken == s.refresh {
		s.mu.Unlock()

		return nil
	}
	s.mu.Unlock()

	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.persistTimeout())
	defer cancel()

	if err := s.provider.lockLifecycle(storeCtx); err != nil {
		return NativePublicError(err)
	}

	rec, ok, err := s.provider.registry.Get(storeCtx, s.accountID)
	if err != nil {
		s.provider.unlockLifecycle()

		return NativePublicError(err)
	}

	if !s.liveUsable(rec, ok) {
		s.provider.unlockLifecycle()
		s.invalid.Store(true)

		return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	stored, storedScopes, getErr := s.provider.tokens.Get(storeCtx, rec.ClientName, rec.Email)
	if getErr != nil || stored == "" {
		s.provider.unlockLifecycle()
		s.invalid.Store(true)

		if getErr != nil && !accountconnect.IsTokenNotFound(getErr) {
			return NativePublicError(getErr)
		}

		return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	scopes := rec.Scopes
	if len(storedScopes) > 0 {
		scopes = storedScopes
	}

	if err := s.provider.tokens.Put(storeCtx, rec.ClientName, rec.Email, tok.RefreshToken, scopes); err != nil {
		s.provider.unlockLifecycle()

		return NativePublicError(err)
	}

	s.provider.unlockLifecycle()

	s.mu.Lock()
	s.refresh = tok.RefreshToken
	s.mu.Unlock()

	return nil
}

func (s *nativeTokenSource) liveUsable(rec accountconnect.Record, ok bool) bool {
	if s.blocked() || !ok || !rec.IsActive() {
		return false
	}

	return rec.Generation == s.generation && rec.Email == s.email && rec.ClientName == s.clientName && rec.PrincipalID == s.principalID && rec.RevokeEpoch == s.revokeEpoch
}

func (p *NativeProvider) cachedClient(ctx context.Context, rec accountconnect.Record) (*nativeCachedClient, error) {
	id := rec.Identity()
	key := identityCacheKey(id)

	p.mu.Lock()
	if cached, ok := p.cache[key]; ok && cached != nil && cached.source != nil && !cached.source.invalid.Load() {
		p.mu.Unlock()

		return cached, nil
	}

	fence := p.fences[rec.AccountID]
	p.mu.Unlock()

	created, err := p.newCachedClient(ctx, rec, fence)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	live, ok, liveErr := p.registry.Get(ctx, rec.AccountID)
	if liveErr != nil {
		created.source.invalidate()

		return nil, NativePublicError(liveErr)
	}

	if !ok || live.Generation != rec.Generation || p.fences[rec.AccountID] != created.source.fence || !live.IsActive() {
		created.source.invalidate()

		return nil, &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	if cached, exists := p.cache[key]; exists && cached != nil && cached.source != nil && !cached.source.invalid.Load() {
		created.source.invalidate()

		return cached, nil
	}

	p.cache[key] = created

	return created, nil
}

//nolint:contextcheck // OAuth refresh must use a detached context so the first request cannot cancel later token reuse.
func (p *NativeProvider) newCachedClient(ctx context.Context, rec accountconnect.Record, fence uint64) (*nativeCachedClient, error) {
	refresh, _, err := p.tokens.Get(ctx, rec.ClientName, rec.Email)
	if err != nil {
		if accountconnect.IsTokenNotFound(err) {
			return nil, &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
		}

		return nil, NativePublicError(err)
	}

	if refresh == "" {
		return nil, &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	clientID, clientSecret, err := p.credentials(rec.ClientName)
	if err != nil {
		return nil, NativePublicError(err)
	}

	if clientID == "" || clientSecret == "" {
		return nil, &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	cfg := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     p.endpoint,
	}
	refreshCtx := context.WithValue(context.Background(), oauth2.HTTPClient, p.refreshHTTP)
	source := &nativeTokenSource{
		provider:    p,
		accountID:   rec.AccountID,
		email:       rec.Email,
		clientName:  rec.ClientName,
		principalID: rec.PrincipalID,
		generation:  rec.Generation,
		revokeEpoch: rec.RevokeEpoch,
		fence:       fence,
		inner:       cfg.TokenSource(refreshCtx, &oauth2.Token{RefreshToken: refresh}),
		refresh:     refresh,
	}

	return &nativeCachedClient{source: source}, nil
}
