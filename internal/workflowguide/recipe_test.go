package workflowguide

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRecipesCoverTwentyFamiliesAndStayLocal(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"admin":           "admin.change_user_lifecycle",
		"docs":            "docs.edit_existing",
		"slides":          "slides.edit_existing",
		"sheets":          "sheets.update_cells_or_format",
		"calendar":        "calendar.create_meeting",
		"drive":           "drive.share_file",
		"businessprofile": "businessprofile.resolve_location",
		"analytics":       "analytics.run_bounded_report",
		"searchconsole":   "searchconsole.performance_report",
		"chat":            "chat.send_message",
		"people":          "contacts.find_person",
		"tasks":           "tasks.create_or_update",
		"groups":          "groups.change_membership",
		"keep":            "keep.list_or_get_notes",
		"classroom":       "classroom.resolve_course",
		"bigquery":        "bigquery.run_read_query",
		"youtube":         "youtube.list_channel_videos",
		"tagmanager":      "tagmanager.inspect_container",
		"gmail":           "gmail.find_message",
		"devices":         "devices.resolve_resource",
	}

	got := map[string]bool{}

	for id := range recipesByIntent {
		recipe, ok := LookupRecipe(id)
		if !ok {
			t.Fatalf("missing recipe %s", id)
		}

		got[recipe.Service] = true
		if recipe.IntentID != id {
			t.Fatalf("intent mismatch %s", id)
		}

		if len(recipe.Steps) > MaxRecipeSteps {
			t.Fatalf("%s has %d steps", id, len(recipe.Steps))
		}
	}

	for service, intent := range want {
		if !got[service] {
			t.Fatalf("missing family %s", service)
		}

		if _, ok := LookupRecipe(intent); !ok {
			t.Fatalf("missing intent %s", intent)
		}
	}

	keep, _ := LookupRecipe("keep.list_or_get_notes")
	if keep.Gap == "" || len(keep.Steps) != 0 {
		t.Fatalf("keep should be an identity gap: %+v", keep)
	}

	bp, _ := LookupRecipe("businessprofile.resolve_location")
	if bp.Gap == "" || len(bp.Steps) != 2 {
		t.Fatalf("business profile should expose bounded reads and preserve remaining gaps: %+v", bp)
	}

	if _, ok := LookupRecipe("sheets.update_format_or_structure"); !ok {
		t.Fatal("missing format/structure sheets recipe")
	}
}

func TestAdminUserKeyDoesNotSkipGet(t *testing.T) {
	t.Parallel()

	prepared, err := Prepare("admin.change_user_lifecycle", "google_admin_directory_users_patch", map[string]string{
		"user_key":        "user@example.com",
		"lifecycle_patch": `{"suspended":true}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	if prepared.FactsStatus != FactsCallerSupplied {
		t.Fatal(prepared.FactsStatus)
	}

	if !hasStep(prepared, "google_admin_directory_users_get") {
		t.Fatalf("user_key skipped get: %+v", prepared.Steps)
	}

	skipped, err := Prepare("admin.change_user_lifecycle", "google_admin_directory_users_get", map[string]string{
		"user_key":              "id123",
		"lifecycle_patch":       `{"suspended":true}`,
		"immutable_user_id":     "id123",
		"current_suspended":     "false",
		"current_primary_email": "user@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	if hasStep(skipped, "google_admin_directory_users_get") && skipped.Steps[0].Kind == KindLookup {
		if skipped.CallEstimate.Max >= prepared.CallEstimate.Max {
			t.Fatalf("current-state facts did not reduce get: %+v vs %+v", skipped.CallEstimate, prepared.CallEstimate)
		}
	}

	if hasLookup(skipped, "google_admin_directory_users_get") {
		t.Fatalf("current-state facts should skip get: %+v", skipped.Steps)
	}
}

func TestLookupOutputsDoNotPermanentlyBlock(t *testing.T) {
	t.Parallel()

	docs, err := Prepare("docs.edit_existing", "google_docs_documents_batchupdate", map[string]string{
		"document_id": "doc1",
		"requests":    `[{"insertText":{"location":{"index":1},"text":"Hi"}}]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	if docs.Status != StatusReady && docs.Status != StatusBlocked {
		t.Fatalf("status %s", docs.Status)
	}

	if hasMissing(docs, "revision_id") {
		t.Fatalf("revision_id permanently blocked: %+v", docs.MissingPrerequisites)
	}

	if !hasUnresolved(docs, "revision_id") {
		t.Fatalf("expected unresolved revision_id: %+v", docs.UnresolvedOutputs)
	}

	withRev, err := Prepare("docs.edit_existing", "google_docs_documents_get", map[string]string{
		"document_id": "doc1",
		"requests":    "[]",
		"revision_id": "rev-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	if hasLookup(withRev, "google_docs_documents_get") {
		t.Fatalf("revision_id should skip get: %+v", withRev.Steps)
	}

	if withRev.CallEstimate.Max >= docs.CallEstimate.Max {
		t.Fatalf("skip did not lower estimate %d vs %d", withRev.CallEstimate.Max, docs.CallEstimate.Max)
	}
}

func TestCalendarTimezoneIsCallerRequired(t *testing.T) {
	t.Parallel()

	blocked, err := Prepare("calendar.create_meeting", "google_calendar_events_insert", map[string]string{
		"calendar_id":  "primary",
		"event_id":     "abcde",
		"event":        `{"summary":"Standup"}`,
		"send_updates": "none",
	})
	if err != nil {
		t.Fatal(err)
	}

	if blocked.Status != StatusBlocked || !hasMissing(blocked, "time_zone") {
		t.Fatalf("timed event missing timezone: %+v", blocked)
	}

	ok, err := Prepare("calendar.create_meeting", "google_calendar_events_insert", map[string]string{
		"calendar_id":  "primary",
		"event_id":     "abcde",
		"event":        `{"summary":"Standup"}`,
		"send_updates": "none",
		"all_day":      "true",
	})
	if err != nil {
		t.Fatal(err)
	}

	if hasMissing(ok, "time_zone") {
		t.Fatalf("all-day still required timezone: %+v", ok.MissingPrerequisites)
	}
}

func TestSelectedOperationMismatchAndUnknownIntent(t *testing.T) {
	t.Parallel()

	miss, err := Prepare("docs.edit_existing", "gmail_search", nil)
	if err != nil {
		t.Fatal(err)
	}

	if miss.Status != StatusNotApplicable || len(miss.Steps) != 0 {
		t.Fatalf("mismatch: %+v", miss)
	}

	if _, err := Prepare("not.a.real.intent", "google_docs_documents_get", nil); !errors.Is(err, ErrUnknownIntent) {
		t.Fatalf("unknown intent: %v", err)
	}
}

func TestRecipeCloneAndKnownFactBounds(t *testing.T) {
	t.Parallel()

	recipe, _ := LookupRecipe("drive.share_file")
	recipe.AppliesTo[0] = "tampered"

	again, _ := LookupRecipe("drive.share_file")
	if again.AppliesTo[0] == "tampered" {
		t.Fatal("recipe shares mutable slices")
	}

	if msg := ValidateKnownFacts(map[string]string{strings.Repeat("k", 65): "v"}); msg == "" {
		t.Fatal("oversized key accepted")
	}

	encoded, err := json.Marshal(recipe)
	if err != nil || len(encoded) == 0 {
		t.Fatal(err)
	}
}

func hasStep(prepared PreparedRecipe, operation string) bool {
	for _, step := range prepared.Steps {
		if step.Operation == operation {
			return true
		}
	}

	return false
}

func hasLookup(prepared PreparedRecipe, operation string) bool {
	for _, step := range prepared.Steps {
		if step.Operation == operation && step.Kind == KindLookup {
			return true
		}
	}

	return false
}

func hasMissing(prepared PreparedRecipe, key string) bool {
	for _, fact := range prepared.MissingPrerequisites {
		if fact.Key == key {
			return true
		}
	}

	return false
}

func hasUnresolved(prepared PreparedRecipe, key string) bool {
	for _, fact := range prepared.UnresolvedOutputs {
		if fact.Key == key {
			return true
		}
	}

	return false
}

func TestCalendarFalseAllDayStillRequiresTimezone(t *testing.T) {
	t.Parallel()
	facts := map[string]string{"calendar_id": "primary", "event_id": "abcde", "event": "{}", "send_updates": "none", "all_day": "false"}

	p, err := Prepare("calendar.create_meeting", "google_calendar_events_insert", facts)
	if err != nil {
		t.Fatal(err)
	}

	if !hasMissing(p, "time_zone") || p.Status != StatusBlocked {
		t.Fatalf("timed event lost timezone prerequisite: %+v", p)
	}
	facts["all_day"] = "true"

	p, err = Prepare("calendar.create_meeting", "google_calendar_events_insert", facts)
	if err != nil {
		t.Fatal(err)
	}

	if hasMissing(p, "time_zone") {
		t.Fatal("all-day event required timezone")
	}
}

func TestRecipeLookupSourceAndExactCounts(t *testing.T) {
	t.Parallel()

	facts := map[string]string{"document_id": "doc", "requests": "[]"}
	for _, tc := range []struct {
		key, value, source string
		calls              int
	}{
		{"revision_id", "", "previous_step:revision_id", 2},
		{"revision_id", "   ", "previous_step:revision_id", 2},
		{" revision_id", "rev", "previous_step:revision_id", 2},
		{"revision_id", "rev", "fact:revision_id", 1},
	} {
		inputs := map[string]string{"document_id": facts["document_id"], "requests": facts["requests"], tc.key: tc.value}

		p, err := Prepare("docs.edit_existing", "google_docs_documents_batchupdate", inputs)
		if err != nil {
			t.Fatal(err)
		}

		if p.CallEstimate.Min != tc.calls || p.CallEstimate.Max != tc.calls {
			t.Fatalf("calls: %+v", p.CallEstimate)
		}
		found := false

		for _, step := range p.Steps {
			for _, input := range step.RequiredInputs {
				if input.Name == "writeControl.requiredRevisionId" {
					found = true

					if input.Source != tc.source {
						t.Fatalf("source %s want %s", input.Source, tc.source)
					}
				}
			}
		}

		if !found {
			t.Fatal("missing revision input")
		}
	}
}

func TestEveryRecipePreparationFitsBound(t *testing.T) {
	t.Parallel()

	for _, recipe := range recipesByIntent {
		if len(recipe.AppliesTo) == 0 {
			continue
		}

		prepared, err := Prepare(recipe.IntentID, recipe.AppliesTo[0], nil)
		if err != nil {
			t.Fatal(err)
		}

		encoded, err := json.Marshal(prepared)
		if err != nil {
			t.Fatal(err)
		}
		// Reserve per-step runtime availability labels absent from this static view.
		if len(encoded)+64*len(prepared.Steps) > 4096 {
			t.Errorf("%s exceeds bounded preparation: %d", recipe.IntentID, len(encoded))
		}
	}
}

func TestLookupInputSources(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ intent, operation, fact, field string }{
		{"youtube.list_channel_videos", "google_youtube_playlistitems_list", "uploads_playlist_id", "playlistId"},
		{"gmail.find_message", "google_gmail_users_messages_get", "message_id", "id"},
	} {
		t.Run(tc.intent, func(t *testing.T) {
			t.Parallel()

			for _, supplied := range []bool{false, true} {
				facts := map[string]string{}

				want := "previous_step:" + tc.fact
				if supplied {
					facts[tc.fact] = "exact-id"
					want = "fact:" + tc.fact
				}

				prepared, err := Prepare(tc.intent, tc.operation, facts)
				if err != nil {
					t.Fatal(err)
				}
				found := false

				for _, step := range prepared.Steps {
					if step.Operation != tc.operation {
						continue
					}

					for _, input := range step.RequiredInputs {
						if input.Name == tc.field {
							found = true

							if input.Source != want {
								t.Fatalf("supplied=%t source=%q want=%q", supplied, input.Source, want)
							}
						}
					}
				}

				if !found {
					t.Fatal("lookup input absent")
				}
			}
		})
	}
}

func TestRecipeFactReferencesDeclared(t *testing.T) {
	t.Parallel()

	for _, recipe := range recipesByIntent {
		facts := map[string]bool{}
		for _, fact := range recipe.RequiredFacts {
			facts[fact.Key] = true
		}

		for _, step := range recipe.Steps {
			for _, input := range step.Inputs {
				if key, ok := strings.CutPrefix(input.Source, "fact:"); ok && !facts[key] {
					t.Errorf("%s input %s references undeclared fact %q", recipe.IntentID, input.Name, key)
				}
			}
		}
	}
}

func TestBusinessProfileRecipeReusesExactParent(t *testing.T) {
	t.Parallel()

	for _, parent := range []string{"", "accounts/123"} {
		prepared, err := Prepare("businessprofile.resolve_location", "businessprofile_list_locations", map[string]string{"parent": parent})
		if err != nil {
			t.Fatal(err)
		}

		want := 2
		if parent != "" {
			want = 1
		}

		if prepared.Status != StatusReady || len(prepared.Steps) != want || prepared.CallEstimate.Max != want {
			t.Fatalf("parent %q: %+v", parent, prepared)
		}

		if hasLookup(prepared, "businessprofile_list_accounts") != (parent == "") {
			t.Fatalf("unexpected account discovery: %+v", prepared)
		}

		if !hasLookup(prepared, "businessprofile_list_locations") || prepared.Gap == "" {
			t.Fatalf("missing read or availability caveat: %+v", prepared)
		}
	}
}
