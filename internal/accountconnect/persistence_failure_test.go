package accountconnect

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type mutationErrorStore struct {
	RefreshTokenStore
	failure error
}

func (s mutationErrorStore) Put(ctx context.Context, clientName, email, token string, scopes []string) error {
	if err := s.RefreshTokenStore.Put(ctx, clientName, email, token, scopes); err != nil {
		return fmt.Errorf("fixture put: %w", err)
	}

	return s.failure
}

type activationErrorRegistry struct{ Registry }

func (r activationErrorRegistry) Commit(ctx context.Context, rec Record) (Record, Record, bool, error) {
	if rec.State == RecordStateActive {
		return Record{}, Record{}, false, errRegistryFailed
	}

	committed, previous, existed, err := r.Registry.Commit(ctx, rec)
	if err != nil {
		return Record{}, Record{}, false, fmt.Errorf("fixture commit: %w", err)
	}

	return committed, previous, existed, nil
}

func TestCallbackPersistenceFailureRetainsPendingAndNotifies(t *testing.T) {
	for _, tc := range []struct {
		name            string
		putError        error
		activationError bool
		rekey           bool
	}{
		{name: "put after mutation", putError: errRegistryFailed},
		{name: "canceled put", putError: context.Canceled},
		{name: "rekey put after mutation", putError: errRegistryFailed, rekey: true},
		{name: "activation", activationError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			provider := &fakeProvider{}
			ctrl := testController(t, provider)
			inv := &recordingInvalidator{life: ctrl.Lifecycle()}
			ctrl.invalidator = inv

			first, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
			if err != nil {
				t.Fatal(err)
			}

			connected, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "first", State: first.SessionID, BrowserID: first.SessionID, RedirectURL: ctrl.RedirectURL()})
			if err != nil {
				t.Fatal(err)
			}

			original, ok, err := ctrl.registry.Get(ctx, connected.Account.AccountID)
			if err != nil || !ok {
				t.Fatalf("original: ok=%v err=%v", ok, err)
			}
			inv.notify = 0
			inv.notifyHeld = nil
			inner := ctrl.tokens

			expectedError := tc.putError
			if tc.activationError {
				ctrl.registry = activationErrorRegistry{Registry: ctrl.registry}
				expectedError = errRegistryFailed
			} else {
				ctrl.tokens = mutationErrorStore{RefreshTokenStore: inner, failure: tc.putError}
			}

			nextEmail := original.Email
			if tc.rekey {
				nextEmail = "new@example.test"
			}
			provider.exchange = func(ExchangeParams) (TokenSet, error) {
				return TokenSet{RefreshToken: "new-refresh", Subject: original.Subject, Email: nextEmail, Scopes: original.Scopes}, nil
			}

			re, err := ctrl.StartReconnect(ctx, ReconnectRequest{PrincipalID: "jeremy", AccountID: original.AccountID})
			if err != nil {
				t.Fatal(err)
			}

			_, err = ctrl.CompleteCallback(ctx, CallbackRequest{Code: "second", State: re.SessionID, BrowserID: re.SessionID, RedirectURL: ctrl.RedirectURL()})
			if !errors.Is(err, expectedError) {
				t.Fatalf("callback error = %v", err)
			}

			pending, ok, err := ctrl.registry.Get(ctx, original.AccountID)
			if err != nil || !ok || pending.IsActive() || pending.State != RecordStatePending {
				t.Fatalf("failed persistence must remain pending: %+v ok=%v err=%v", pending, ok, err)
			}

			token, _, err := inner.Get(ctx, original.ClientName, nextEmail)
			if err != nil || token != "new-refresh" {
				t.Fatalf("fixture mutation missing: err=%v", err)
			}

			if tc.rekey && (pending.Cleanup == nil || pending.Cleanup.Email != original.Email) {
				t.Fatalf("old key cleanup lost: %+v", pending.Cleanup)
			}

			if inv.notify != 1 {
				t.Fatalf("notification count=%d", inv.notify)
			}

			for _, held := range inv.notifyHeld {
				if held {
					t.Fatal("notification held lifecycle lock")
				}
			}
		})
	}
}
