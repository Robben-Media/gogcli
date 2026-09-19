package accountconnect

import (
	"context"
	"errors"
	"fmt"
	"sync"

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

// SecretsTokenStore adapts the existing keyring-backed store.
type SecretsTokenStore struct {
	store secrets.Store
}

func NewSecretsTokenStore(store secrets.Store) (*SecretsTokenStore, error) {
	if store == nil {
		return nil, ErrNilTokens
	}

	return &SecretsTokenStore{store: store}, nil
}

func (s *SecretsTokenStore) Get(_ context.Context, clientName, email string) (string, []string, error) {
	tok, err := s.store.GetToken(clientName, email)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return "", nil, errTokenNotFound
		}

		return "", nil, fmt.Errorf("get refresh token: %w", err)
	}

	return tok.RefreshToken, append([]string(nil), tok.Scopes...), nil
}

func (s *SecretsTokenStore) Put(_ context.Context, clientName, email, token string, scopes []string) error {
	existing, err := s.store.GetToken(clientName, email)
	if err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return fmt.Errorf("read existing token metadata: %w", err)
	}

	next := existing
	next.Email = email
	next.Client = clientName

	next.RefreshToken = token
	if len(scopes) > 0 {
		next.Scopes = append([]string(nil), scopes...)
	}

	if err := s.store.SetToken(clientName, email, next); err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}

	return nil
}

func (s *SecretsTokenStore) Delete(_ context.Context, clientName, email string) error {
	if err := s.store.DeleteToken(clientName, email); err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}

	return nil
}

func IsTokenNotFound(err error) bool {
	return errors.Is(err, errTokenNotFound)
}
