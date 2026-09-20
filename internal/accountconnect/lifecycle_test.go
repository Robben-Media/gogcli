package accountconnect

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLifecycleNilIsNoop(t *testing.T) {
	t.Parallel()

	var life *Lifecycle
	if err := life.Lock(t.Context()); err != nil {
		t.Fatalf("nil Lock: %v", err)
	}

	life.Unlock()

	if life.Held() {
		t.Fatal("nil Held")
	}
}

func TestLifecycleLockHonorsCancel(t *testing.T) {
	t.Parallel()

	life := NewLifecycle()
	if err := life.Lock(t.Context()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- life.Lock(ctx)
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled wait: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Lock ignored cancellation")
	}

	life.Unlock()
}
