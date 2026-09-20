package workflowguide

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSelectedServiceGuidanceIsBoundedAndIndependent(t *testing.T) {
	actions := make([]string, 0, 7+len(guides))

	actions = append(actions, []string{"admin_directory:api.users.insert", "docs:api.documents.batchUpdate", "slides:api.presentations.batchUpdate", "sheets:get", "calendar:events", "drive:get", "mybusiness_business_information:api.locations.patch"}...)
	for service := range guides {
		actions = append(actions, service+":api.get")
	}

	for _, action := range actions {
		g := ForAction(action)
		if g.ReviewedAt == "" || len(g.Sources) == 0 || len(g.Intents) == 0 {
			t.Fatalf("missing researched guidance for %s", action)
		}

		b, err := json.Marshal(g)
		if err != nil || len(b) > 16000 {
			t.Fatalf("guidance too large: %s bytes=%d err=%v", action, len(b), err)
		}

		g.ReadStrategy[0] = "tampered"
		if ForAction(action).ReadStrategy[0] == "tampered" {
			t.Fatal("guidance shares mutable slices")
		}

		for _, source := range g.Sources {
			if !strings.HasPrefix(source, "https://developers.google.com/") && !strings.HasPrefix(source, "https://cloud.google.com/") && !strings.HasPrefix(source, "https://docs.cloud.google.com/") && !strings.HasPrefix(source, "https://support.google.com/") {
				t.Fatalf("unreviewed source: %s", source)
			}
		}
	}
}
