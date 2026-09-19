package access

import (
	"context"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestBroaderScopeDiscoveryAndAuthorization(t *testing.T) {
	t.Parallel()
	identity := testIdentity("acct-personal", "personal@example.test", "personal")
	identity.Scopes = []string{"https://mail.google.com/"}

	authorizer, err := NewAuthorizer(testSnapshot("gmail_search"), NewMemoryAccounts(identity))
	if err != nil {
		t.Fatal(err)
	}

	principal := mcpcontract.Principal{ID: "local"}
	if visible, err := authorizer.Visible(principal, "gmail_search"); err != nil || !visible {
		t.Fatalf("broader grant hidden: visible=%v err=%v", visible, err)
	}

	if _, err := authorizer.Authorize(context.Background(), principal, identity.AccountID, "gmail_search", []string{"gmail:messages.search"}); err != nil {
		t.Fatalf("broader grant denied: %v", err)
	}
}

func TestGrantPatternsMustMatchCatalog(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{"gmail:mesages.search", "foo:bar", ":"} {
		if err := (Snapshot{Grants: []mcpcontract.Grant{{Operations: []string{pattern}}}}).Validate(); err == nil {
			t.Errorf("accepted unknown grant pattern %q", pattern)
		}
	}

	for _, pattern := range []string{"gmail:*", "gmail:read", "gmail:messages.*", "search-console:query", "gmail_search"} {
		if err := (Snapshot{Grants: []mcpcontract.Grant{{Operations: []string{pattern}}}}).Validate(); err != nil {
			t.Errorf("rejected catalog pattern %q: %v", pattern, err)
		}
	}
}
