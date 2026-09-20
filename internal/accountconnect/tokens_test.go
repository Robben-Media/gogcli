package accountconnect

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/secrets"
)

func TestMemoryTokenStoreConcurrent(t *testing.T) {
	store := NewMemoryTokenStore()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			email := "user@example.com"
			if err := store.Put(ctx, "default", email, "rt", []string{"openid"}); err != nil {
				t.Errorf("put: %v", err)
			}

			if _, _, err := store.Get(ctx, "default", email); err != nil {
				t.Errorf("get: %v", err)
			}
		}()
	}

	wg.Wait()
}

func TestSecretsTokenStoreDeadlinePoisonsStore(t *testing.T) {
	backend := &blockingSecrets{release: make(chan struct{})}

	store, err := NewSecretsTokenStore(backend)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, _, err = store.Get(ctx, "default", "me@gmail.com")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}

	close(backend.release)

	if _, _, err := store.Get(context.Background(), "default", "me@gmail.com"); !errors.Is(err, ErrTokenStoreUnavailable) {
		t.Fatalf("poison: %v", err)
	}
}

type blockingSecrets struct {
	release chan struct{}
}

func (s *blockingSecrets) Keys() ([]string, error) { return nil, nil }

func (s *blockingSecrets) SetToken(string, string, secrets.Token) error { return nil }

func (s *blockingSecrets) GetToken(string, string) (secrets.Token, error) {
	<-s.release
	return secrets.Token{RefreshToken: "rt"}, nil
}

func (s *blockingSecrets) DeleteToken(string, string) error { return nil }

func (s *blockingSecrets) ListTokens() ([]secrets.Token, error) { return nil, nil }

func (s *blockingSecrets) GetDefaultAccount(string) (string, error) { return "", nil }

func (s *blockingSecrets) SetDefaultAccount(string, string) error { return nil }

func TestSecretsTokenStoreTimeoutDoesNotApplyLatePut(t *testing.T) {
	backend := &gatedSecrets{
		tokens:  map[string]secrets.Token{"default\x00me@gmail.com": {RefreshToken: "first", Email: "me@gmail.com", Client: "default"}},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	store, err := NewSecretsTokenStore(backend)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()

	putErr := store.Put(ctx, "default", "me@gmail.com", "late-old", []string{"openid"})
	if !errors.Is(putErr, context.DeadlineExceeded) {
		t.Fatalf("timeout put: %v", putErr)
	}

	if laterErr := store.Put(context.Background(), "default", "me@gmail.com", "newer-reconnect", []string{"openid"}); !errors.Is(laterErr, ErrTokenStoreUnavailable) {
		t.Fatalf("poisoned put: %v", laterErr)
	}

	close(backend.release)

	token, _, err := backend.snapshot("default", "me@gmail.com")
	if err != nil {
		t.Fatal(err)
	}

	if token != "first" {
		t.Fatalf("late put overwrote store: %q", token)
	}
}

func TestSecretsTokenStoreSingleWorker(t *testing.T) {
	backend := &gatedSecrets{tokens: map[string]secrets.Token{}}

	store, err := NewSecretsTokenStore(backend)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := store.Put(context.Background(), "default", "me@gmail.com", "rt", []string{"openid"}); err != nil {
				t.Errorf("put: %v", err)
			}

			if _, _, err := store.Get(context.Background(), "default", "me@gmail.com"); err != nil {
				t.Errorf("get: %v", err)
			}
		}()
	}

	wg.Wait()

	if got := backend.maxConcurrent.Load(); got > 1 {
		t.Fatalf("token store ran %d backend ops at once", got)
	}
}

type gatedSecrets struct {
	mu            sync.Mutex
	tokens        map[string]secrets.Token
	started       chan struct{}
	release       chan struct{}
	startedOnce   sync.Once
	inFlight      atomic.Int32
	maxConcurrent atomic.Int32
}

func (s *gatedSecrets) key(client, email string) string {
	return client + "\x00" + email
}

func (s *gatedSecrets) enter() {
	n := s.inFlight.Add(1)
	for {
		cur := s.maxConcurrent.Load()
		if n <= cur || s.maxConcurrent.CompareAndSwap(cur, n) {
			break
		}
	}

	if s.started != nil {
		s.startedOnce.Do(func() { close(s.started) })
	}

	if s.release != nil {
		<-s.release
	}
}

func (s *gatedSecrets) leave() {
	s.inFlight.Add(-1)
}

func (s *gatedSecrets) snapshot(client, email string) (string, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tok, ok := s.tokens[s.key(client, email)]
	if !ok {
		return "", nil, errTokenNotFound
	}

	return tok.RefreshToken, append([]string(nil), tok.Scopes...), nil
}

func (s *gatedSecrets) Keys() ([]string, error) { return nil, nil }

func (s *gatedSecrets) SetToken(client, email string, tok secrets.Token) error {
	s.enter()
	defer s.leave()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens[s.key(client, email)] = tok

	return nil
}

func (s *gatedSecrets) GetToken(client, email string) (secrets.Token, error) {
	s.enter()
	defer s.leave()

	s.mu.Lock()
	defer s.mu.Unlock()

	tok, ok := s.tokens[s.key(client, email)]
	if !ok {
		return secrets.Token{}, keyring.ErrKeyNotFound
	}

	return tok, nil
}

func (s *gatedSecrets) DeleteToken(client, email string) error {
	s.enter()
	defer s.leave()

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tokens, s.key(client, email))

	return nil
}

func (s *gatedSecrets) ListTokens() ([]secrets.Token, error) { return nil, nil }

func (s *gatedSecrets) GetDefaultAccount(string) (string, error) { return "", nil }

func (s *gatedSecrets) SetDefaultAccount(string, string) error { return nil }

func TestSecretsTokenStoreTimeoutDoesNotReturnLateGet(t *testing.T) {
	backend := &gatedSecrets{
		tokens:  map[string]secrets.Token{"default\x00me@gmail.com": {RefreshToken: "secret-rt", Email: "me@gmail.com", Client: "default"}},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	store, err := NewSecretsTokenStore(backend)
	if err != nil {
		t.Fatal(err)
	}
	store.timeout = 40 * time.Millisecond

	token, scopes, getErr := store.Get(context.Background(), "default", "me@gmail.com")
	if !errors.Is(getErr, context.DeadlineExceeded) {
		t.Fatalf("timeout get: token=%q scopes=%v err=%v", token, scopes, getErr)
	}

	if token != "" || scopes != nil {
		t.Fatalf("timed-out get returned captured result token=%q scopes=%v", token, scopes)
	}

	if _, _, err := store.Get(context.Background(), "default", "me@gmail.com"); !errors.Is(err, ErrTokenStoreUnavailable) {
		t.Fatalf("poison: %v", err)
	}

	close(backend.release)
}

func TestSecretsTokenStoreCancelAfterAdmissionPoisons(t *testing.T) {
	backend := &gatedSecrets{
		tokens:  map[string]secrets.Token{"default\x00me@gmail.com": {RefreshToken: "secret-rt", Email: "me@gmail.com", Client: "default"}},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	store, err := NewSecretsTokenStore(backend)
	if err != nil {
		t.Fatal(err)
	}
	store.timeout = time.Minute

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() {
		_, _, getErr := store.Get(ctx, "default", "me@gmail.com")
		done <- getErr
	}()

	select {
	case <-backend.started:
	case <-time.After(2 * time.Second):
		t.Fatal("get was not admitted")
	}

	cancel()

	getErr := <-done
	if !errors.Is(getErr, context.Canceled) {
		t.Fatalf("canceled get: %v", getErr)
	}

	if _, _, err := store.Get(context.Background(), "default", "me@gmail.com"); !errors.Is(err, ErrTokenStoreUnavailable) {
		t.Fatalf("cancel after admission must poison: %v", err)
	}

	close(backend.release)
}
