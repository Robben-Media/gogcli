package accountconnect

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/secrets"
)

// RefreshTokenStore holds refresh tokens keyed by OAuth client bucket and email.
type RefreshTokenStore interface {
	Get(_ context.Context, clientName, email string) (token string, scopes []string, err error)
	Put(_ context.Context, clientName, email, token string, scopes []string) error
	Delete(_ context.Context, clientName, email string) error
}

// MemoryTokenStore is a test double. It never logs token values.
type MemoryTokenStore struct {
	mu     sync.Mutex
	tokens map[string]memoryToken
}

type memoryToken struct {
	token  string
	scopes []string
}

func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{tokens: make(map[string]memoryToken)}
}

func tokenKey(clientName, email string) string {
	return clientName + "\x00" + email
}

func (s *MemoryTokenStore) Get(_ context.Context, clientName, email string) (string, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.tokens[tokenKey(clientName, email)]
	if !ok {
		return "", nil, errTokenNotFound
	}

	return item.token, append([]string(nil), item.scopes...), nil
}

func (s *MemoryTokenStore) Put(_ context.Context, clientName, email, token string, scopes []string) error {
	if token == "" {
		return ErrMissingRefresh
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens[tokenKey(clientName, email)] = memoryToken{token: token, scopes: append([]string(nil), scopes...)}

	return nil
}

func (s *MemoryTokenStore) Delete(_ context.Context, clientName, email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tokens, tokenKey(clientName, email))

	return nil
}

var errTokenNotFound = errors.New("accountconnect: refresh token not found")

const defaultTokenStoreTimeout = 30 * time.Second

type tokenJobResult struct {
	err    error
	token  string
	scopes []string
}

type tokenJob struct {
	run  func() tokenJobResult
	done chan tokenJobResult
}

// SecretsTokenStore adapts the existing keyring-backed store.
// Get, Put, and Delete run on one process-wide worker. A caller deadline
// poisons the store; later operations fail closed and queued work does not start.
// Each operation is bounded by timeout (default 30s) even when ctx has no deadline.
type SecretsTokenStore struct {
	store    secrets.Store
	jobs     chan tokenJob
	timeout  time.Duration
	poisoned atomic.Bool
	poison   atomic.Value
}

func NewSecretsTokenStore(store secrets.Store) (*SecretsTokenStore, error) {
	if store == nil {
		return nil, ErrNilTokens
	}

	s := &SecretsTokenStore{store: store, jobs: make(chan tokenJob), timeout: defaultTokenStoreTimeout}
	go s.serve()

	return s, nil
}

func (s *SecretsTokenStore) serve() {
	for job := range s.jobs {
		if err := s.unavailable(); err != nil {
			job.done <- tokenJobResult{err: err}
			continue
		}

		job.done <- job.run()
	}
}

func (s *SecretsTokenStore) failClosed(err error) {
	if s.poisoned.Swap(true) {
		return
	}

	if err != nil {
		s.poison.Store(err)
	}
}

func (s *SecretsTokenStore) unavailable() error {
	if !s.poisoned.Load() {
		return nil
	}

	if err, ok := s.poison.Load().(error); ok && err != nil {
		return fmt.Errorf("%w: %w", ErrTokenStoreUnavailable, err)
	}

	return ErrTokenStoreUnavailable
}

func (s *SecretsTokenStore) bound(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}

	timeout := s.timeout
	if timeout <= 0 {
		timeout = defaultTokenStoreTimeout
	}

	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}

	if timeout < 0 {
		timeout = 0
	}

	return context.WithTimeout(ctx, timeout)
}

func (s *SecretsTokenStore) abort(ctx context.Context) error {
	if err := s.unavailable(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		s.failClosed(err)

		return fmt.Errorf("token store: %w", err)
	}

	return nil
}

func (s *SecretsTokenStore) do(ctx context.Context, run func(context.Context) tokenJobResult) tokenJobResult {
	if err := s.unavailable(); err != nil {
		return tokenJobResult{err: err}
	}

	ctx, cancel := s.bound(ctx)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return tokenJobResult{err: fmt.Errorf("token store: %w", err)}
	}

	job := tokenJob{run: func() tokenJobResult { return run(ctx) }, done: make(chan tokenJobResult, 1)}
	select {
	case s.jobs <- job:
	case <-ctx.Done():
		return tokenJobResult{err: fmt.Errorf("token store: %w", ctx.Err())}
	}

	select {
	case res := <-job.done:
		return res
	case <-ctx.Done():
		err := ctx.Err()
		s.failClosed(err)

		return tokenJobResult{err: fmt.Errorf("token store: %w", err)}
	}
}

func (s *SecretsTokenStore) Get(ctx context.Context, clientName, email string) (string, []string, error) {
	res := s.do(ctx, func(ctx context.Context) tokenJobResult {
		if err := s.abort(ctx); err != nil {
			return tokenJobResult{err: err}
		}

		tok, getErr := s.store.GetToken(clientName, email)
		if getErr != nil {
			if errors.Is(getErr, keyring.ErrKeyNotFound) {
				return tokenJobResult{err: errTokenNotFound}
			}

			return tokenJobResult{err: fmt.Errorf("get refresh token: %w", getErr)}
		}

		return tokenJobResult{token: tok.RefreshToken, scopes: append([]string(nil), tok.Scopes...)}
	})
	if res.err != nil {
		return "", nil, res.err
	}

	return res.token, res.scopes, nil
}

func (s *SecretsTokenStore) Put(ctx context.Context, clientName, email, token string, scopes []string) error {
	if token == "" {
		return ErrMissingRefresh
	}

	res := s.do(ctx, func(ctx context.Context) tokenJobResult {
		if err := s.abort(ctx); err != nil {
			return tokenJobResult{err: err}
		}

		existing, err := s.store.GetToken(clientName, email)
		if err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
			return tokenJobResult{err: fmt.Errorf("read existing token metadata: %w", err)}
		}

		if err := s.abort(ctx); err != nil {
			return tokenJobResult{err: err}
		}

		next := existing
		next.Email = email
		next.Client = clientName

		next.RefreshToken = token
		if len(scopes) > 0 {
			next.Scopes = append([]string(nil), scopes...)
		}

		if err := s.store.SetToken(clientName, email, next); err != nil {
			return tokenJobResult{err: fmt.Errorf("store refresh token: %w", err)}
		}

		return tokenJobResult{}
	})

	return res.err
}

func (s *SecretsTokenStore) Delete(ctx context.Context, clientName, email string) error {
	res := s.do(ctx, func(ctx context.Context) tokenJobResult {
		if err := s.abort(ctx); err != nil {
			return tokenJobResult{err: err}
		}

		if err := s.store.DeleteToken(clientName, email); err != nil {
			return tokenJobResult{err: fmt.Errorf("delete refresh token: %w", err)}
		}

		return tokenJobResult{}
	})

	return res.err
}

func IsTokenNotFound(err error) bool {
	return errors.Is(err, errTokenNotFound)
}
