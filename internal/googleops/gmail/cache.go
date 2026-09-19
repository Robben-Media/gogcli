package gmail

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	labelCacheLimit = 128
	labelCacheTTL   = 60 * time.Second
)

type labelMetadata struct {
	Names     map[string]string
	FetchedAt time.Time
}

type labelCacheKey struct {
	AccountID   string
	PrincipalID string
	ClientName  string
	AuthMode    string
	Scopes      string
	Generation  uint64
}

type labelCache struct {
	mu      sync.Mutex
	entries map[labelCacheKey]labelMetadata
	order   []labelCacheKey
}

func newLabelCache() *labelCache {
	return &labelCache{entries: make(map[labelCacheKey]labelMetadata)}
}

func newLabelCacheKey(identity mcpcontract.Identity) labelCacheKey {
	return labelCacheKey{
		AccountID:   identity.AccountID,
		PrincipalID: identity.PrincipalID,
		ClientName:  identity.ClientName,
		AuthMode:    identity.AuthMode,
		Scopes:      canonicalScopes(identity.Scopes),
		Generation:  identity.Generation,
	}
}

func canonicalScopes(scopes []string) string {
	ordered := append([]string(nil), scopes...)
	sort.Strings(ordered)
	digest := sha256.New()

	var length [8]byte
	for _, scope := range ordered {
		binary.BigEndian.PutUint64(length[:], uint64(len(scope)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(scope))
	}

	return hex.EncodeToString(digest.Sum(nil))
}

func (c *labelCache) get(key labelCacheKey) (map[string]string, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	metadata, ok := c.entries[key]
	if !ok {
		return nil, time.Time{}, false
	}

	if time.Since(metadata.FetchedAt) >= labelCacheTTL {
		c.removeLocked(key)
		return nil, time.Time{}, false
	}

	return cloneLabelNames(metadata.Names), metadata.FetchedAt, true
}

func (c *labelCache) set(key labelCacheKey, metadata labelMetadata) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; !exists {
		c.order = append(c.order, key)
	}

	c.entries[key] = labelMetadata{
		Names:     cloneLabelNames(metadata.Names),
		FetchedAt: metadata.FetchedAt,
	}
	for len(c.order) > labelCacheLimit {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
}

func (c *labelCache) removeLocked(key labelCacheKey) {
	delete(c.entries, key)

	for index, ordered := range c.order {
		if ordered == key {
			c.order = append(c.order[:index], c.order[index+1:]...)
			return
		}
	}
}

func cloneLabelNames(names map[string]string) map[string]string {
	if names == nil {
		return nil
	}

	clone := make(map[string]string, len(names))
	for key, value := range names {
		clone[key] = value
	}

	return clone
}
