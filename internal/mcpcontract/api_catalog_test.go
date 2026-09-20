package mcpcontract

import (
	"strings"
	"testing"
)

func TestGeneratedMutationScopeBoundaries(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, read, write string }{
		{"google_slides_presentations_batchupdate", DriveReadScope, SlidesWriteScope},
		{"google_drive_files_copy", "https://www.googleapis.com/auth/drive.photos.readonly", "https://www.googleapis.com/auth/drive.file"},
		{"google_people_othercontacts_copyothercontacttomycontactsgroup", "https://www.googleapis.com/auth/contacts.other.readonly", "https://www.googleapis.com/auth/contacts"},
	} {
		def, ok := Lookup(tc.name)
		if !ok {
			t.Fatalf("missing %s", tc.name)
		}

		if ScopesSatisfied([]string{tc.read}, def) {
			t.Fatalf("read-only token authorized %s", tc.name)
		}

		if !ScopesSatisfied([]string{tc.write}, def) {
			t.Fatalf("write scope rejected %s", tc.name)
		}

		for _, action := range def.Actions {
			if action != strings.ToLower(action) {
				t.Fatalf("uncanonical action %s", action)
			}
		}
	}
}
