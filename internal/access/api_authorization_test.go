package access

import (
	"errors"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestGeneratedBatchUpdateRequiresTargetWriteScope(t *testing.T) {
	t.Parallel()
	const name = "google_slides_presentations_batchupdate"

	def, ok := mcpcontract.Lookup(name)
	if !ok {
		t.Fatal("missing operation")
	}

	for _, scope := range []string{mcpcontract.DriveReadScope, mcpcontract.SlidesWriteScope} {
		id := testIdentity("acct-personal", "me@example.test", "personal")
		id.Scopes = []string{scope}

		auth, err := NewAuthorizer(testSnapshot(name), NewMemoryAccounts(id))
		if err != nil {
			t.Fatal(err)
		}

		_, err = auth.Authorize(t.Context(), mcpcontract.Principal{ID: "local"}, id.AccountID, name, def.Actions)
		if scope == mcpcontract.SlidesWriteScope {
			if err != nil {
				t.Fatalf("valid action/scope denied: %v", err)
			}
		} else {
			var safe *mcpcontract.Error
			if !errors.As(err, &safe) || safe.Category != mcpcontract.InsufficientScope {
				t.Fatalf("readonly token: %v", err)
			}
		}
	}
}
