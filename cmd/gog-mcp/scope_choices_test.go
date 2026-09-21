package main

import (
	"errors"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestExplicitBusinessProfileConsentDoesNotAdmitUnknownScopes(t *testing.T) {
	t.Parallel()

	choices, err := configuredConnectScopes(mcpcontract.BusinessManageScope)
	if err != nil || len(choices) != 1 || choices[0].Scope != mcpcontract.BusinessManageScope {
		t.Fatalf("explicit Business Profile consent: choices=%v err=%v", choices, err)
	}

	_, err = configuredConnectScopes("https://www.googleapis.com/auth/not-a-reviewed-scope")
	if !errors.Is(err, errUnknownScope) {
		t.Fatalf("unknown scope admitted: %v", err)
	}
}
