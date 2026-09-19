package googleapi

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

type blockingNativeTokens struct {
	inner    RefreshTokenStore
	mu       sync.Mutex
	blockGet bool
	entered  chan struct{}
	release  chan struct{}
}

func (s *blockingNativeTokens) Get(ctx context.Context, clientName, email string) (string, []string, error) {
	s.mu.Lock()
	block := s.blockGet
	s.mu.Unlock()

	if block {
		select {
		case s.entered <- struct{}{}:
		default:
		}

		select {
		case <-s.release:
		case <-ctx.Done():
			return "", nil, fmt.Errorf("blocked get: %w", ctx.Err())
		}
	}

	token, scopes, err := s.inner.Get(ctx, clientName, email)
	if err != nil {
		return "", nil, fmt.Errorf("inner get: %w", err)
	}

	return token, scopes, nil
}

func (s *blockingNativeTokens) Put(ctx context.Context, clientName, email, token string, scopes []string) error {
	if err := s.inner.Put(ctx, clientName, email, token, scopes); err != nil {
		return fmt.Errorf("inner put: %w", err)
	}

	return nil
}

func TestBlockedRotationStoreReleasesLifecycle(t *testing.T) {
	fx := newNativeFixture(t, 1)
	fx.provider.timeout = 50 * time.Millisecond
	blocked := &blockingNativeTokens{inner: fx.tokens, entered: make(chan struct{}, 1), release: make(chan struct{})}
	fx.provider.tokens = blocked
	id := fx.putAccount(t, "acct-bound", "sub-bound", "bound@gmail.com", "personal", []string{mcpcontract.GmailReadScope}, "rt-bound")
	fx.setRotate("rt-bound", "rt-bound-2", id.AccountID)

	client, err := fx.provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "gmail_search", Retry: mcpcontract.SafeRead})
	if err != nil {
		t.Fatal(err)
	}

	blocked.mu.Lock()
	blocked.blockGet = true
	blocked.mu.Unlock()

	done := make(chan error, 1)

	go func() {
		_, doErr := doAuthorized(client, fx.api.URL)
		done <- doErr
	}()

	select {
	case <-blocked.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("persist get did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := fx.life.Lock(ctx); err != nil {
		t.Fatalf("blocked persist held Lifecycle: %v", err)
	}

	if !fx.life.Held() {
		t.Fatal("expected lock ownership after persist released")
	}

	fx.life.Unlock()
	close(blocked.release)

	if err := <-done; err == nil {
		t.Fatal("expected persist timeout")
	}
}
