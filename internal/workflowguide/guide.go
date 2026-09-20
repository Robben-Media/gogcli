// Package workflowguide supplies local, reviewed service instructions on demand.
// These are planning guidance, not an implementation of approval or quota policy.
package workflowguide

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// Intent gives the agent a bounded approach and a reason to stop, rather than
// suggesting additional API calls regardless of the user's task.
type Intent struct {
	ID            string `json:"intent_id"`
	Intent        string `json:"intent"`
	MinimumCalls  string `json:"minimum_calls"`
	Approval      string `json:"approval"`
	StopCondition string `json:"stop_condition"`
	NextAction    string `json:"next_action"`
}

type Guide struct {
	Service        string   `json:"service"`
	ReviewedAt     string   `json:"reviewed_at,omitempty"`
	Prerequisites  []string `json:"prerequisites"`
	ReadStrategy   []string `json:"read_strategy"`
	WriteStrategy  []string `json:"write_strategy"`
	QuotaCaveats   []string `json:"quota_caveats"`
	Intents        []Intent `json:"intents,omitempty"`
	Sources        []string `json:"sources,omitempty"`
	ResearchStatus string   `json:"research_status,omitempty"`
}

//go:embed services.json
var guideData []byte

var guides = loadGuides()

func loadGuides() map[string]Guide {
	var entries []Guide
	if err := json.Unmarshal(guideData, &entries); err != nil {
		panic(err)
	}

	out := make(map[string]Guide, len(entries))
	for _, entry := range entries {
		out[entry.Service] = entry
	}

	return out
}

// ForAction returns only the selected service's instructions. It performs no
// network requests. Resource content cannot modify this static guidance.
func ForAction(action string) Guide {
	service, _, _ := strings.Cut(action, ":")
	switch {
	case strings.HasPrefix(service, "admin_"):
		service = "admin"
	case strings.HasPrefix(service, "analytics"):
		service = "analytics"
	case strings.HasPrefix(service, "mybusiness"):
		service = "businessprofile"
	case service == "cloud_identity" && strings.Contains(strings.ToLower(action), "cloudidentity.devices."):
		service = "devices"
	case service == "cloud_identity" && strings.Contains(strings.ToLower(action), "cloudidentity.groups."):
		service = "groups"
	case service == "contacts":
		service = "people"
	}

	if guide, ok := guides[service]; ok {
		return clone(guide)
	}

	return Guide{
		Service: service, ResearchStatus: "Service-specific guidance has not yet been incorporated; inspect exact operation requirements.",
		Prerequisites: []string{"Use an explicitly granted account_id and exact provider resource IDs."},
		ReadStrategy:  []string{"Retrieve one bounded page or the exact resource; do not fan out or fetch attachments without task need."},
		WriteStrategy: []string{"Confirm the exact target, payload, notification effects and task authorization before a write.", "Do not replay a write after an unknown outcome; reconcile the exact target first."},
		QuotaCaveats:  []string{"Retries consume the same API attempt budget. Request count does not establish provider quota units."},
	}
}

func clone(g Guide) Guide {
	g.Prerequisites = append([]string(nil), g.Prerequisites...)
	g.ReadStrategy = append([]string(nil), g.ReadStrategy...)
	g.WriteStrategy = append([]string(nil), g.WriteStrategy...)
	g.QuotaCaveats = append([]string(nil), g.QuotaCaveats...)
	g.Intents = append([]Intent(nil), g.Intents...)
	g.Sources = append([]string(nil), g.Sources...)

	return g
}
