package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestGetMessageUsesCentralPublicErrorMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":403,"message":"user@example.com denied","errors":[{"reason":"dailyLimitExceeded"}]}}`, http.StatusForbidden)
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{url: server.URL}}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{"account_id":"acct","message_id":"m1","include_body":false}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	_, err = call.Run(context.Background(), testIdentity())

	var public *mcpcontract.Error
	if !errors.As(err, &public) || public.Category != mcpcontract.QuotaExhausted || public.Retryable {
		t.Fatalf("error = %#v, want non-retryable quota_exhausted", err)
	}

	if strings.Contains(public.Error(), "user@example.com") || strings.Contains(public.Error(), "denied") {
		t.Fatalf("public error leaked upstream detail: %q", public.Error())
	}
}

func TestPublicErrorPreservesTypedContractErrorsThroughWrappers(t *testing.T) {
	typed := &mcpcontract.Error{Category: mcpcontract.InsufficientScope, Message: "additional scope required"}
	wrapped := fmt.Errorf("provider setup: %w", typed)

	var public *mcpcontract.Error
	if !errors.As(publicError(wrapped), &public) || public != typed {
		t.Fatalf("public error = %#v, want original typed error", public)
	}
}
