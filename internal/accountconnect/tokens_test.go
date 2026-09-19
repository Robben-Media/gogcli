package accountconnect

import (
	"context"
	"sync"
	"testing"
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
