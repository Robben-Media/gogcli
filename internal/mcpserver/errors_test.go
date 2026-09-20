package mcpserver

import (
	"fmt"
	"strings"
	"testing"

	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestPublicErrorMapsGoogleAPIStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		category mcpcontract.ErrorCategory
	}{
		{name: "401", err: &gapi.Error{Code: 401, Body: "token user@gmail.com"}, category: mcpcontract.AuthRequired},
		{name: "403-scope", err: &gapi.Error{Code: 403, Message: "insufficientPermissions", Body: "https://evil.example"}, category: mcpcontract.InsufficientScope},
		{name: "403-quota", err: &gapi.Error{Code: 403, Errors: []gapi.ErrorItem{{Reason: "rateLimitExceeded"}}, Body: "quota user@gmail.com"}, category: mcpcontract.QuotaExhausted},
		{name: "404", err: &gapi.Error{Code: 404, Message: "message abc", Body: "https://gmail.googleapis.com"}, category: mcpcontract.NotFound},
		{name: "429", err: &gapi.Error{Code: 429, Body: "slow down"}, category: mcpcontract.QuotaExhausted},
		{name: "400", err: &gapi.Error{Code: 400, Body: "bad request secret=abc"}, category: mcpcontract.InvalidInput},
		{name: "500", err: &gapi.Error{Code: 500, Body: "upstream https://googleapis.com"}, category: mcpcontract.UpstreamFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := publicError(fmt.Errorf("wrapped: %w", tt.err))
			if got == nil || got.Category != tt.category {
				t.Fatalf("got %#v want %s", got, tt.category)
			}

			if strings.Contains(got.Message, "user@gmail.com") || strings.Contains(got.Message, "https://") || strings.Contains(got.Message, "secret=") {
				t.Fatalf("leaked details: %q", got.Message)
			}
		})
	}
}
