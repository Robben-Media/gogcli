package gmail

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestLabelCacheUsesCompleteIdentityKey(t *testing.T) {
	base := testIdentity()
	base.Scopes = []string{"https://mail.google.com/scope-a", "https://mail.google.com/scope-b"}
	baseKey := newLabelCacheKey(base)

	sameScopes := base

	sameScopes.Scopes = []string{"https://mail.google.com/scope-b", "https://mail.google.com/scope-a"}
	if got := newLabelCacheKey(sameScopes); got != baseKey {
		t.Fatal("scope order changed the cache key")
	}

	cache := newLabelCache()
	cache.set(baseKey, labelMetadata{
		Names:     map[string]string{"INBOX": "Inbox"},
		FetchedAt: time.Now().UTC(),
	})

	if _, _, cached := cache.get(baseKey); !cached {
		t.Fatal("base identity did not hit its cache entry")
	}

	variants := make([]mcpcontract.Identity, 1, 7)
	variants[0] = base
	variants = append(variants, base)
	variants[1].AccountID = "other-account"
	variants = append(variants, base)
	variants[2].PrincipalID = "other-principal"
	variants = append(variants, base)
	variants[3].ClientName = "other-client"
	variants = append(variants, base)
	variants[4].AuthMode = "other-auth"
	variants = append(variants, base)
	variants[5].Scopes = []string{"https://mail.google.com/scope-a"}
	variants = append(variants, base)
	variants[6].Generation++

	for index, identity := range variants[1:] {
		if _, _, cached := cache.get(newLabelCacheKey(identity)); cached {
			t.Fatalf("identity variant %d unexpectedly hit base cache entry", index)
		}
	}
}

func TestLabelCacheExpiresAfterTTL(t *testing.T) {
	cache := newLabelCache()
	key := newLabelCacheKey(testIdentity())
	metadata := labelMetadata{
		Names:     map[string]string{"INBOX": "Inbox"},
		FetchedAt: time.Now().UTC().Add(-labelCacheTTL - time.Second),
	}
	cache.set(key, metadata)

	if names, _, cached := cache.get(key); cached {
		t.Fatalf("expired entry returned metadata %#v", names)
	}
}

func TestLabelCacheEvictsOldestEntry(t *testing.T) {
	cache := newLabelCache()
	base := testIdentity()

	for index := 0; index <= labelCacheLimit; index++ {
		identity := base
		identity.AccountID = strconv.Itoa(index)
		key := newLabelCacheKey(identity)
		cache.set(key, labelMetadata{Names: map[string]string{}, FetchedAt: time.Now().UTC()})
	}

	oldest := base

	oldest.AccountID = "0"
	if _, _, cached := cache.get(newLabelCacheKey(oldest)); cached {
		t.Fatal("oldest entry survived eviction")
	}

	second := base

	second.AccountID = "1"
	if _, _, cached := cache.get(newLabelCacheKey(second)); !cached {
		t.Fatal("second entry was evicted prematurely")
	}
}

func TestLabelCacheConcurrentAccess(t *testing.T) {
	cache := newLabelCache()
	base := testIdentity()

	keys := make([]labelCacheKey, 32)
	for index := range keys {
		identity := base
		identity.AccountID = strconv.Itoa(index)
		keys[index] = newLabelCacheKey(identity)
		cache.set(keys[index], labelMetadata{
			Names:     map[string]string{"INBOX": "Inbox"},
			FetchedAt: time.Now().UTC(),
		})
	}

	var wg sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()

			for iteration := 0; iteration < 100; iteration++ {
				key := keys[(worker+iteration)%len(keys)]
				cache.get(key)
				cache.set(key, labelMetadata{
					Names:     map[string]string{"INBOX": "Inbox " + strconv.Itoa(iteration)},
					FetchedAt: time.Now().UTC(),
				})
			}
		}(worker)
	}

	wg.Wait()
}

func TestLabelCacheClonesMetadata(t *testing.T) {
	cache := newLabelCache()
	key := newLabelCacheKey(testIdentity())
	source := map[string]string{"INBOX": "Inbox"}
	cache.set(key, labelMetadata{Names: source, FetchedAt: time.Now().UTC()})

	names, _, _ := cache.get(key)
	names["INBOX"] = "changed"

	_, _, cached := cache.get(key)
	if !cached {
		t.Fatal("cache entry disappeared")
	}

	clone, _, _ := cache.get(key)
	if clone["INBOX"] != "Inbox" {
		t.Fatalf("cached label was mutated to %q", clone["INBOX"])
	}
}
