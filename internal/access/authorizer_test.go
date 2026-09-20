package access

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

func testIdentity(id, email, client string) mcpcontract.Identity {
	return mcpcontract.Identity{
		AccountID:   id,
		Subject:     "subject-" + id,
		Email:       email,
		Label:       email,
		PrincipalID: "local",
		ClientName:  client,
		AuthMode:    authModeOAuth,
		Scopes:      []string{mcpcontract.GmailReadScope, mcpcontract.AnalyticsReadScope},
		Generation:  1,
	}
}

func testSnapshot(operations ...string) Snapshot {
	return Snapshot{
		AllowOperations: operations,
		Grants: []mcpcontract.Grant{{
			PrincipalID: "local",
			AccountIDs:  []string{"acct-personal", "acct-work"},
			ClientNames: []string{"personal", "workspace"},
			Operations:  operations,
		}},
	}
}

func TestAuthorizeGrantAndPolicyIntersection(t *testing.T) {
	t.Parallel()

	personal := testIdentity("acct-personal", "me@gmail.com", "personal")
	work := testIdentity("acct-work", "work@example.com", "workspace")

	authorizer, err := NewAuthorizer(testSnapshot("gmail_search", "accounts_list"), NewMemoryAccounts(personal, work))
	if err != nil {
		t.Fatal(err)
	}

	got, err := authorizer.Authorize(context.Background(), mcpcontract.Principal{ID: "local"}, "acct-personal", "gmail_search", []string{"gmail:messages.search"})
	if err != nil {
		t.Fatalf("authorize personal: %v", err)
	}

	if got.AccountID != "acct-personal" {
		t.Fatalf("got %+v", got)
	}

	got.Scopes[0] = "mutated"
	if personal.Scopes[0] != mcpcontract.GmailReadScope {
		t.Fatal("identity scopes escaped the clone boundary")
	}
}

func TestAuthorizeUnknownAccountAndWrongPrincipal(t *testing.T) {
	t.Parallel()

	personal := testIdentity("acct-personal", "me@gmail.com", "personal")

	authorizer, err := NewAuthorizer(testSnapshot("gmail_search"), NewMemoryAccounts(personal))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := authorizer.Authorize(context.Background(), mcpcontract.Principal{ID: "local"}, "guessed@gmail.com", "gmail_search", []string{"gmail:messages.search"}); err == nil {
		t.Fatal("email used as authority must fail")
	}

	if _, err := authorizer.Authorize(context.Background(), mcpcontract.Principal{ID: "other"}, "acct-personal", "gmail_search", []string{"gmail:messages.search"}); !isForbidden(err) {
		t.Fatalf("wrong principal: %v", err)
	}
}

func TestAuthorizeActionsCannotWidenCatalog(t *testing.T) {
	t.Parallel()

	personal := testIdentity("acct-personal", "me@gmail.com", "personal")

	authorizer, err := NewAuthorizer(testSnapshot("gmail_search"), NewMemoryAccounts(personal))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := authorizer.Authorize(context.Background(), mcpcontract.Principal{ID: "local"}, "acct-personal", "gmail_search", []string{"gmail:send"}); !isForbidden(err) {
		t.Fatalf("widened action: %v", err)
	}
}

func TestAuthorizePolicyDenyCannotBeWidenedByGrant(t *testing.T) {
	t.Parallel()

	personal := testIdentity("acct-personal", "me@gmail.com", "personal")
	snapshot := testSnapshot("gmail_search")
	snapshot.Policies = []config.Policy{{
		Name:    "no-search",
		Account: "me@gmail.com",
		Deny:    []string{"gmail:messages.search"},
	}}

	authorizer, err := NewAuthorizer(snapshot, NewMemoryAccounts(personal))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := authorizer.Authorize(context.Background(), mcpcontract.Principal{ID: "local"}, "acct-personal", "gmail_search", []string{"gmail:messages.search"}); !isForbidden(err) {
		t.Fatalf("grant must not widen deny: %v", err)
	}
}

func TestVisibleAnyActionAnalytics(t *testing.T) {
	t.Parallel()

	work := testIdentity("acct-work", "work@example.com", "workspace")
	snapshot := testSnapshot("analytics_metadata", "gmail_search")
	snapshot.Policies = []config.Policy{{
		Name:    "metrics-only",
		Account: "work@example.com",
		Allow:   []string{"analytics:metrics"},
	}}

	authorizer, err := NewAuthorizer(snapshot, NewMemoryAccounts(work))
	if err != nil {
		t.Fatal(err)
	}

	principal := mcpcontract.Principal{ID: "local"}

	visible, err := authorizer.Visible(t.Context(), principal, "analytics_metadata")
	if err != nil {
		t.Fatal(err)
	}

	if !visible {
		t.Fatal("analytics_metadata should list when any kind is allowed")
	}

	if _, err := authorizer.Authorize(context.Background(), principal, "acct-work", "analytics_metadata", []string{"analytics:metrics"}); err != nil {
		t.Fatalf("metrics kind: %v", err)
	}

	if _, err := authorizer.Authorize(context.Background(), principal, "acct-work", "analytics_metadata", []string{"analytics:dimensions", "analytics:metrics"}); !isForbidden(err) {
		t.Fatalf("both kinds require every selected action: %v", err)
	}
}

func TestReplaceSnapshotIsAtomic(t *testing.T) {
	t.Parallel()

	personal := testIdentity("acct-personal", "me@gmail.com", "personal")

	authorizer, err := NewAuthorizer(testSnapshot("gmail_search"), NewMemoryAccounts(personal))
	if err != nil {
		t.Fatal(err)
	}

	var started atomic.Bool
	go func() {
		started.Store(true)
		authorizer.Replace(testSnapshot("accounts_list"))
	}()

	principal := mcpcontract.Principal{ID: "local"}
	for i := 0; i < 50; i++ {
		_, err := authorizer.Authorize(context.Background(), principal, "acct-personal", "gmail_search", []string{"gmail:messages.search"})
		if err != nil && !isForbidden(err) {
			t.Fatalf("unexpected %#v", err)
		}
	}

	if !started.Load() {
		t.Log("replace raced with authorize")
	}
}

func TestListAccountsCallerFiltered(t *testing.T) {
	t.Parallel()

	personal := testIdentity("acct-personal", "me@gmail.com", "personal")
	other := testIdentity("acct-other", "other@gmail.com", "personal")
	other.PrincipalID = "someone-else"

	authorizer, err := NewAuthorizer(testSnapshot("accounts_list", "gmail_search"), NewMemoryAccounts(personal, other))
	if err != nil {
		t.Fatal(err)
	}

	got, err := authorizer.ListAccounts(context.Background(), mcpcontract.Principal{ID: "local"})
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 || got[0].AccountID != "acct-personal" {
		t.Fatalf("got %#v", got)
	}
}

func isForbidden(err error) bool {
	var typed *mcpcontract.Error
	return errors.As(err, &typed) && typed.Category == mcpcontract.Forbidden
}

func TestAuthorizeActionGrantCoversTool(t *testing.T) {
	t.Parallel()

	personal := testIdentity("acct-personal", "me@gmail.com", "personal")
	snapshot := Snapshot{
		AllowOperations: []string{"gmail_get_message", "accounts_list"},
		Grants: []mcpcontract.Grant{{
			PrincipalID: "local",
			AccountIDs:  []string{"acct-personal"},
			ClientNames: []string{"personal"},
			Operations:  []string{"gmail:get"},
		}},
	}

	authorizer, err := NewAuthorizer(snapshot, NewMemoryAccounts(personal))
	if err != nil {
		t.Fatal(err)
	}

	principal := mcpcontract.Principal{ID: "local"}

	visible, err := authorizer.Visible(t.Context(), principal, "gmail_get_message")
	if err != nil {
		t.Fatal(err)
	}

	if !visible {
		t.Fatal("action grant should advertise the tool")
	}

	if _, err := authorizer.Authorize(context.Background(), principal, "acct-personal", "gmail_get_message", []string{"gmail:get"}); err != nil {
		t.Fatalf("action grant: %v", err)
	}
}

func TestAuthorizeAccountIDIsExact(t *testing.T) {
	t.Parallel()

	upper := testIdentity("A", "upper@example.test", "personal")
	lower := testIdentity("a", "lower@example.test", "personal")
	snapshot := Snapshot{
		AllowOperations: []string{"gmail_search"},
		Grants: []mcpcontract.Grant{{
			PrincipalID: "local",
			AccountIDs:  []string{"A"},
			ClientNames: []string{"personal"},
			Operations:  []string{"gmail_search"},
		}},
	}

	authorizer, err := NewAuthorizer(snapshot, NewMemoryAccounts(upper, lower))
	if err != nil {
		t.Fatal(err)
	}

	principal := mcpcontract.Principal{ID: "local"}
	if _, err := authorizer.Authorize(context.Background(), principal, "A", "gmail_search", []string{"gmail:messages.search"}); err != nil {
		t.Fatalf("uppercase account: %v", err)
	}

	if _, err := authorizer.Authorize(context.Background(), principal, "a", "gmail_search", []string{"gmail:messages.search"}); !isForbidden(err) {
		t.Fatalf("lowercase account must stay distinct: %v", err)
	}
}

func TestVisibleRequiresMatchingScopes(t *testing.T) {
	t.Parallel()

	personal := testIdentity("acct-personal", "me@gmail.com", "personal")
	snapshot := testSnapshot("gmail_search", "drive_search")

	authorizer, err := NewAuthorizer(snapshot, NewMemoryAccounts(personal))
	if err != nil {
		t.Fatal(err)
	}

	principal := mcpcontract.Principal{ID: "local"}

	visible, err := authorizer.Visible(t.Context(), principal, "drive_search")
	if err != nil {
		t.Fatal(err)
	}

	if visible {
		t.Fatal("drive_search must not list without drive scopes")
	}
}

func TestAuthorizeRejectsEmptyAuthMode(t *testing.T) {
	t.Parallel()

	personal := testIdentity("acct-personal", "me@gmail.com", "personal")
	personal.AuthMode = ""

	authorizer, err := NewAuthorizer(testSnapshot("gmail_search"), NewMemoryAccounts(personal))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := authorizer.Authorize(context.Background(), mcpcontract.Principal{ID: "local"}, "acct-personal", "gmail_search", []string{"gmail:messages.search"}); !isForbidden(err) {
		t.Fatalf("empty auth mode: %v", err)
	}
}

func TestUnknownAllowOperationRejected(t *testing.T) {
	t.Parallel()

	personal := testIdentity("acct-personal", "me@gmail.com", "personal")

	_, err := NewAuthorizer(testSnapshot("gmail_serach"), NewMemoryAccounts(personal))
	if err == nil {
		t.Fatal("typo allow operation must fail startup")
	}
}
