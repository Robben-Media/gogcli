package access

import (
	"errors"
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestNativeMediaHonorsExistingCLIDenies(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ operation, deniedAction, scope string }{
		{"gmail_get_attachment", "gmail:attachment", mcpcontract.GmailReadScope},
		{"drive_download_file", "drive:download", mcpcontract.DriveReadScope},
		{"drive_export_file", "drive:download", mcpcontract.DriveReadScope},
		{"drive_create_file", "drive:upload", "https://www.googleapis.com/auth/drive.file"},
		{"drive_update_file", "drive:upload", "https://www.googleapis.com/auth/drive.file"},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			t.Parallel()

			id := testIdentity("acct-personal", "me@example.test", "personal")
			id.Scopes = []string{tc.scope}
			snapshot := testSnapshot(tc.operation)
			snapshot.Policies = []config.Policy{{Name: "existing-cli-deny", Deny: []string{tc.deniedAction}}}

			authorizer, err := NewAuthorizer(snapshot, NewMemoryAccounts(id))
			if err != nil {
				t.Fatal(err)
			}

			def, ok := mcpcontract.Lookup(tc.operation)
			if !ok {
				t.Fatal("missing media operation")
			}

			if _, err := authorizer.Authorize(t.Context(), mcpcontract.Principal{ID: "local"}, id.AccountID, tc.operation, def.Actions); !isForbidden(err) {
				t.Fatalf("existing CLI denial bypassed: %v", err)
			}
		})
	}
}

func TestDriveMediaScopesRemainResourceLimited(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		operation string
		scope     string
		allowed   bool
	}{
		{"drive_download_file", "https://www.googleapis.com/auth/drive.file", true},
		{"drive_export_file", "https://www.googleapis.com/auth/drive.file", true},
		{"drive_create_file", mcpcontract.DriveReadScope, false},
		{"drive_update_file", mcpcontract.DriveReadScope, false},
		{"drive_search", "https://www.googleapis.com/auth/drive.file", false},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			t.Parallel()

			id := testIdentity("acct-personal", "me@example.test", "personal")
			id.Scopes = []string{tc.scope}

			authorizer, err := NewAuthorizer(testSnapshot(tc.operation), NewMemoryAccounts(id))
			if err != nil {
				t.Fatal(err)
			}

			def, _ := mcpcontract.Lookup(tc.operation)

			_, err = authorizer.Authorize(t.Context(), mcpcontract.Principal{ID: "local"}, id.AccountID, tc.operation, def.Actions)
			if tc.allowed {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var public *mcpcontract.Error
				if !errors.As(err, &public) || public.Category != mcpcontract.InsufficientScope {
					t.Fatalf("scope boundary: %v", err)
				}
			}
		})
	}
}
