package googleapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

var (
	errConnectionRefused = errors.New("connection refused")
	errRawLeakBody       = errors.New("raw body user@gmail.com https://evil.example")
)

func TestNativePublicErrorPreservesTypedContract(t *testing.T) {
	original := &mcpcontract.Error{Category: mcpcontract.Forbidden, Message: "already safe", Retryable: false}

	got := NativePublicError(fmt.Errorf("wrap: %w", original))
	if got != original {
		t.Fatalf("typed error was rewritten: %#v", got)
	}

	if NativePublicError(nil) != nil {
		t.Fatal("nil should stay nil")
	}
}

func TestNativePublicErrorClassifications(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		category  mcpcontract.ErrorCategory
		retryable bool
	}{
		{name: "canceled", err: context.Canceled, category: mcpcontract.DeadlineExceeded, retryable: false},
		{name: "deadline", err: context.DeadlineExceeded, category: mcpcontract.DeadlineExceeded, retryable: true},
		{name: "400", err: &gapi.Error{Code: 400, Body: `{"error":"bad <user@gmail.com> https://evil.example/x"}`}, category: mcpcontract.InvalidInput},
		{name: "401", err: &gapi.Error{Code: 401, Message: "invalid token for user@gmail.com"}, category: mcpcontract.AuthRequired},
		{name: "403 scope reason", err: &gapi.Error{Code: 403, Errors: []gapi.ErrorItem{{Reason: "insufficientAuthenticationScopes"}}}, category: mcpcontract.InsufficientScope},
		{name: "403 scope body", err: &gapi.Error{Code: 403, Body: "Request had insufficient authentication scopes for user@gmail.com"}, category: mcpcontract.InsufficientScope},
		{name: "403 permissions", err: &gapi.Error{Code: 403, Errors: []gapi.ErrorItem{{Reason: "insufficientPermissions"}}}, category: mcpcontract.InsufficientScope},
		{name: "403 rate", err: &gapi.Error{Code: 403, Errors: []gapi.ErrorItem{{Reason: "rateLimitExceeded"}}}, category: mcpcontract.QuotaExhausted, retryable: true},
		{name: "403 quota", err: &gapi.Error{Code: 403, Errors: []gapi.ErrorItem{{Reason: "quotaExceeded"}}}, category: mcpcontract.QuotaExhausted, retryable: true},
		{name: "403 daily", err: &gapi.Error{Code: 403, Errors: []gapi.ErrorItem{{Reason: "dailyLimitExceeded"}}}, category: mcpcontract.QuotaExhausted, retryable: false},
		{name: "403 other", err: &gapi.Error{Code: 403, Errors: []gapi.ErrorItem{{Reason: "forbidden"}}}, category: mcpcontract.Forbidden},
		{name: "404", err: &gapi.Error{Code: 404, Message: "missing https://mail.google.com/"}, category: mcpcontract.NotFound},
		{name: "429", err: &gapi.Error{Code: 429}, category: mcpcontract.QuotaExhausted, retryable: true},
		{name: "500", err: &gapi.Error{Code: 500, Body: "upstream https://googleapis.com"}, category: mcpcontract.UpstreamFailure, retryable: true},
		{name: "invalid_grant", err: &oauth2.RetrieveError{ErrorCode: "invalid_grant", Body: []byte("token for user@gmail.com")}, category: mcpcontract.AuthRequired},
		{name: "url timeout", err: &url.Error{Op: "Get", URL: "https://gmail.googleapis.com/v1/users/user@gmail.com", Err: context.DeadlineExceeded}, category: mcpcontract.DeadlineExceeded, retryable: true},
		{name: "url network", err: &url.Error{Op: "Get", URL: "https://gmail.googleapis.com/v1/users/user@gmail.com", Err: errConnectionRefused}, category: mcpcontract.UpstreamFailure, retryable: true},
		{name: "net timeout", err: &net.DNSError{Name: "gmail.googleapis.com", IsTimeout: true}, category: mcpcontract.DeadlineExceeded, retryable: true},
		{name: "unknown", err: errRawLeakBody, category: mcpcontract.UpstreamFailure, retryable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NativePublicError(tt.err)
			if got == nil {
				t.Fatal("nil error")
			}

			if got.Category != tt.category || got.Retryable != tt.retryable {
				t.Fatalf("got %#v", got)
			}

			if strings.Contains(got.Message, "@") || strings.Contains(got.Message, "http") || strings.Contains(got.Message, "evil") || strings.Contains(got.Error(), "user@gmail.com") {
				t.Fatalf("leaked details: %s", got.Error())
			}
		})
	}
}

func TestNativePublicErrorRetrieve400(t *testing.T) {
	got := NativePublicError(&oauth2.RetrieveError{
		Response: &http.Response{StatusCode: 400},
		Body:     []byte("invalid_request for user@gmail.com at https://oauth2.googleapis.com/token"),
	})
	if got.Category != mcpcontract.InvalidInput || got.Retryable {
		t.Fatalf("got %#v", got)
	}

	if strings.Contains(got.Error(), "user@gmail.com") || strings.Contains(got.Error(), "https://") {
		t.Fatalf("leaked retrieve body: %s", got.Error())
	}
}
