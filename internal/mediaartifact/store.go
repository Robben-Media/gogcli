// Package mediaartifact holds temporary media in bounded process memory. It
// neither accepts local paths nor fetches URLs, and never persists user data.
package mediaartifact

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	MaxItemBytes      = 2 << 20
	maxBytes          = 64 << 20
	maxEntries        = 64
	lifetime          = 15 * time.Minute
	maxMetadataBytes  = 64 << 10
	maxIdentityScopes = 512
	URIPrefix         = "gog://media/"
)

// Binding is metadata for the runtime's fresh authorization check. Possession
// of a URI alone does not permit retrieval of the corresponding bytes.
type Binding struct {
	Identity  mcpcontract.Identity
	Operation string
	Reference mcpcontract.MediaReference
}

type entry struct {
	binding Binding
	data    []byte
	cost    int
	timer   *time.Timer
}

type Store struct {
	mu      sync.Mutex
	entries map[string]entry
	bytes   int
	closed  bool
	ttl     time.Duration
	now     func() time.Time
}

func New() *Store {
	return &Store{entries: make(map[string]entry), now: time.Now, ttl: lifetime}
}

func (s *Store) Put(ctx context.Context, id mcpcontract.Identity, operation, name, mimeType string, data []byte) (mcpcontract.MediaReference, error) {
	if err := ctx.Err(); err != nil {
		return mcpcontract.MediaReference{}, &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: "media storage canceled"}
	}

	def, ok := mcpcontract.Lookup(operation)
	if !ok || def.Local || def.Retry != mcpcontract.SafeRead || id.AccountID == "" || id.Subject == "" || id.PrincipalID == "" || id.ClientName == "" || id.AuthMode != "oauth" || id.Generation == 0 {
		return mcpcontract.MediaReference{}, fmt.Errorf("store media: %w", mcpcontract.Invalid("media requires a resolved OAuth account and read operation"))
	}

	if len(data) > MaxItemBytes || len(name) > 512 || len(mimeType) > 128 || strings.ContainsAny(name+mimeType, "\r\n\x00") {
		return mcpcontract.MediaReference{}, fmt.Errorf("store media: %w", mcpcontract.Invalid("media exceeds content or metadata limits"))
	}

	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return mcpcontract.MediaReference{}, &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "media reference could not be created"}
	}

	metadataBytes := len(name) + len(mimeType) + len(id.AccountID) + len(id.Subject) + len(id.Email) + len(id.Label) + len(id.PrincipalID) + len(id.ClientName) + len(id.AuthMode)
	if len(id.Scopes) > maxIdentityScopes {
		return mcpcontract.MediaReference{}, fmt.Errorf("store media: %w", mcpcontract.Invalid("account metadata exceeds storage limits"))
	}

	for _, scope := range id.Scopes {
		metadataBytes += len(scope)
	}

	if metadataBytes > maxMetadataBytes {
		return mcpcontract.MediaReference{}, fmt.Errorf("store media: %w", mcpcontract.Invalid("account metadata exceeds storage limits"))
	}
	cost := len(data) + metadataBytes
	digest := sha256.Sum256(data)
	uri := URIPrefix + hex.EncodeToString(random[:])

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return mcpcontract.MediaReference{}, fmt.Errorf("store media: %w", mcpcontract.Invalid("media storage is closed"))
	}
	now := s.now()
	s.expire(now)

	if len(s.entries) >= maxEntries || cost > maxBytes-s.bytes {
		return mcpcontract.MediaReference{}, &mcpcontract.Error{Category: mcpcontract.BudgetExhausted, Message: "temporary media storage is full; wait for existing references to expire", Retryable: false}
	}

	ref := mcpcontract.MediaReference{URI: uri, Name: strings.Clone(name), MIMEType: strings.Clone(mimeType), SizeBytes: len(data), SHA256: hex.EncodeToString(digest[:]), ExpiresAt: now.Add(s.ttl)}
	timer := time.AfterFunc(s.ttl, func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.remove(uri)
	})
	s.entries[uri] = entry{binding: Binding{Identity: storageIdentity(id), Operation: strings.Clone(operation), Reference: ref}, data: append([]byte(nil), data...), cost: cost, timer: timer}
	s.bytes += cost

	return ref, nil
}

// Lookup returns only a detached authorization binding, never payload bytes.
func (s *Store) Lookup(uri string) (Binding, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.expire(s.now())

	item, ok := s.entries[uri]
	if !ok {
		return Binding{}, false
	}

	bound := item.binding
	bound.Identity = bound.Identity.Clone()

	return bound, true
}

// Read must follow authorization of the binding's original operation. The
// identity comparison fences stale references after reconnect or account reuse.
func (s *Store) Read(ctx context.Context, uri string, current mcpcontract.Identity) ([]byte, mcpcontract.MediaReference, error) {
	if err := ctx.Err(); err != nil {
		return nil, mcpcontract.MediaReference{}, &mcpcontract.Error{Category: mcpcontract.DeadlineExceeded, Message: "media retrieval canceled"}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.expire(s.now())

	item, ok := s.entries[uri]
	if !ok || !sameIdentity(item.binding.Identity, current) {
		return nil, mcpcontract.MediaReference{}, &mcpcontract.Error{Category: mcpcontract.NotFound, Message: "media reference is unavailable or expired"}
	}

	return append([]byte(nil), item.data...), item.binding.Reference, nil
}

func sameIdentity(a, b mcpcontract.Identity) bool {
	return a.AccountID == b.AccountID && a.Subject == b.Subject && a.PrincipalID == b.PrincipalID && a.Email == b.Email && a.ClientName == b.ClientName && a.Generation == b.Generation && a.AuthMode == b.AuthMode && sameScopes(a.Scopes, b.Scopes)
}

func (s *Store) expire(now time.Time) {
	for uri, item := range s.entries {
		if !now.Before(item.binding.Reference.ExpiresAt) {
			s.remove(uri)
		}
	}
}

func (s *Store) remove(uri string) {
	item, ok := s.entries[uri]
	if !ok {
		return
	}

	item.timer.Stop()
	s.bytes -= item.cost
	clear(item.data)
	delete(s.entries, uri)
}

// Close clears retained bytes and cancels expiry callbacks. Further puts fail.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	for uri := range s.entries {
		s.remove(uri)
	}
}

// Detach strings as well as the scope slice so small caller substrings cannot
// retain backing buffers larger than the accounted metadata.
func storageIdentity(id mcpcontract.Identity) mcpcontract.Identity {
	id.AccountID = strings.Clone(id.AccountID)
	id.Subject = strings.Clone(id.Subject)
	id.Email = strings.Clone(id.Email)
	id.Label = strings.Clone(id.Label)
	id.PrincipalID = strings.Clone(id.PrincipalID)
	id.ClientName = strings.Clone(id.ClientName)
	id.AuthMode = strings.Clone(id.AuthMode)

	scopes := make([]string, len(id.Scopes))
	for i, scope := range id.Scopes {
		scopes[i] = strings.Clone(scope)
	}
	id.Scopes = scopes

	return id
}

// A newly narrower resource-limited scope must not authorize cached bytes read
// under the old broad scope without another provider check.
func sameScopes(a, b []string) bool {
	if len(a) != len(b) || len(b) > maxIdentityScopes {
		return false
	}
	left, right := slices.Clone(a), slices.Clone(b)
	slices.Sort(left)
	slices.Sort(right)

	return slices.Equal(left, right)
}
