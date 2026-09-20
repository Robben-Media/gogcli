package mcpserver_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/googleops/apiexec"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
	"github.com/steipete/gogcli/internal/workflowguide"
)

func TestDescribeOmitsPreparationAndKeepsFourTools(t *testing.T) {
	t.Parallel()

	session := recipeSession(t, []string{"google_docs_documents_get", "google_docs_documents_batchupdate"}, true, nil, docsScopes())

	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	names := listedNames(t, listed)
	if len(names) != 4 {
		t.Fatalf("tools = %v", names)
	}

	described, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{"name": "google_docs_documents_get"}})
	if err != nil || described.IsError {
		t.Fatalf("describe: %#v %v", described, err)
	}

	blob := callJSON(t, described)
	if strings.Contains(blob, `"preparation"`) {
		t.Fatalf("default describe leaked preparation: %s", blob)
	}

	if !strings.Contains(blob, `"schema_path"`) && !strings.Contains(blob, "input_schema") {
		t.Fatalf("describe lost schema: %s", blob)
	}
}

func TestRecipeSkipsLookupWhenOutputFactsPresent(t *testing.T) {
	t.Parallel()

	session := recipeSession(t, []string{"google_docs_documents_get", "google_docs_documents_batchupdate"}, true, nil, docsScopes())

	before, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":      "google_docs_documents_batchupdate",
		"intent_id": "docs.edit_existing",
		"known_facts": map[string]string{
			"document_id": "doc1",
			"requests":    `[{"insertText":{"text":"Hi"}}]`,
		},
	}})
	if err != nil || before.IsError {
		t.Fatalf("describe: %#v %v", before, err)
	}

	beforePrep := preparation(t, before)
	if beforePrep.Status == workflowguide.StatusBlocked && hasPrepMissing(beforePrep, "revision_id") {
		t.Fatalf("revision blocked recipe: %+v", beforePrep)
	}

	if !hasPrepLookup(beforePrep, "google_docs_documents_get") {
		t.Fatalf("expected get: %+v", beforePrep.Steps)
	}

	after, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":      "google_docs_documents_batchupdate",
		"intent_id": "docs.edit_existing",
		"known_facts": map[string]string{
			"document_id": "doc1",
			"requests":    `[{"insertText":{"text":"Hi"}}]`,
			"revision_id": "rev-1",
		},
	}})
	if err != nil || after.IsError {
		t.Fatalf("describe: %#v %v", after, err)
	}

	afterPrep := preparation(t, after)
	if hasPrepLookup(afterPrep, "google_docs_documents_get") {
		t.Fatalf("revision_id should skip get: %+v", afterPrep.Steps)
	}

	if afterPrep.CallEstimate.Max >= beforePrep.CallEstimate.Max {
		t.Fatalf("estimate %d vs %d", afterPrep.CallEstimate.Max, beforePrep.CallEstimate.Max)
	}

	if afterPrep.FactsStatus != workflowguide.FactsCallerSupplied {
		t.Fatalf("facts_status %s", afterPrep.FactsStatus)
	}
}

func TestAdminUserKeyDoesNotSkipGetOverMCP(t *testing.T) {
	t.Parallel()

	session := recipeSession(t, []string{"google_admin_directory_users_get", "google_admin_directory_users_patch"}, true, nil, []string{"https://www.googleapis.com/auth/admin.directory.user"})

	got, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":      "google_admin_directory_users_patch",
		"intent_id": "admin.change_user_lifecycle",
		"known_facts": map[string]string{
			"user_key":        "user@example.com",
			"lifecycle_patch": `{"suspended":true}`,
		},
	}})
	if err != nil || got.IsError {
		t.Fatalf("describe: %#v %v", got, err)
	}

	prep := preparation(t, got)
	if !hasPrepLookup(prep, "google_admin_directory_users_get") {
		t.Fatalf("user_key skipped get: %+v", prep)
	}

	skipped, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":      "google_admin_directory_users_patch",
		"intent_id": "admin.change_user_lifecycle",
		"known_facts": map[string]string{
			"user_key":              "id123",
			"lifecycle_patch":       `{"suspended":true}`,
			"immutable_user_id":     "id123",
			"current_suspended":     "false",
			"current_primary_email": "user@example.com",
		},
	}})
	if err != nil || skipped.IsError {
		t.Fatalf("describe: %#v %v", skipped, err)
	}

	if hasPrepLookup(preparation(t, skipped), "google_admin_directory_users_get") {
		t.Fatalf("current-state facts should skip get: %s", callJSON(t, skipped))
	}
}

func TestCalendarTimezoneBlocksAndUnauthorizedWriteIsGated(t *testing.T) {
	t.Parallel()

	session := recipeSession(t, []string{"google_calendar_events_insert", "google_calendar_events_get"}, true, nil, []string{"https://www.googleapis.com/auth/calendar"})

	blocked, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":      "google_calendar_events_insert",
		"intent_id": "calendar.create_meeting",
		"known_facts": map[string]string{
			"calendar_id":  "primary",
			"event_id":     "abcde123",
			"event":        `{"summary":"x"}`,
			"send_updates": "none",
		},
	}})
	if err != nil || blocked.IsError {
		t.Fatalf("describe: %#v %v", blocked, err)
	}

	prep := preparation(t, blocked)
	if prep.Status != workflowguide.StatusBlocked || !hasPrepMissing(prep, "time_zone") {
		t.Fatalf("timezone: %+v", prep)
	}

	gated := recipeSession(t, []string{"google_docs_documents_get", "google_docs_documents_batchupdate"}, false, []string{"google_docs_documents_get"}, docsScopes())

	denied, err := gated.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":      "google_docs_documents_get",
		"intent_id": "docs.edit_existing",
		"known_facts": map[string]string{
			"document_id": "doc1",
			"requests":    `[]`,
		},
	}})
	if err != nil || denied.IsError {
		t.Fatalf("describe: %#v %v", denied, err)
	}

	deniedPrep := preparation(t, denied)
	if deniedPrep.Status != workflowguide.StatusUnavailable {
		t.Fatalf("status %s", deniedPrep.Status)
	}

	for _, step := range deniedPrep.Steps {
		if step.Operation == "google_docs_documents_batchupdate" {
			if step.Availability != "write_disabled" && step.Availability != "missing_grant" {
				t.Fatalf("write availability %s", step.Availability)
			}

			if len(step.RequiredInputs) != 0 {
				t.Fatalf("leaked inputs: %+v", step.RequiredInputs)
			}
		}
	}

	if strings.Contains(callJSON(t, denied), `"writeControl"`) {
		t.Fatalf("leaked write schema: %s", callJSON(t, denied))
	}
}

func TestRecipeDescribeHasNoUpstreamAndBounds(t *testing.T) {
	t.Parallel()

	session := recipeSession(t, []string{"google_docs_documents_get", "google_docs_documents_batchupdate"}, true, nil, docsScopes())

	got, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":        "google_docs_documents_get",
		"intent_id":   "docs.edit_existing",
		"schema_path": "input",
		"known_facts": map[string]string{"document_id": "doc1", "requests": "[]"},
	}})
	if err != nil || got.IsError {
		t.Fatalf("describe: %#v %v", got, err)
	}

	blob := callJSON(t, got)
	if strings.Contains(blob, `"upstream_calls":`) && !strings.Contains(blob, `"upstream_calls":0`) && !strings.Contains(blob, `"upstream_calls": 0`) {
		t.Fatalf("upstream: %s", blob)
	}
	prep := preparation(t, got)

	encoded, _ := json.Marshal(prep)
	if len(encoded) > 4096 {
		t.Fatalf("preparation %d bytes", len(encoded))
	}

	unknown, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":      "google_docs_documents_get",
		"intent_id": "not.a.recipe",
	}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, unknown, mcpcontract.InvalidInput)

	oversize, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":      "google_docs_documents_get",
		"intent_id": "docs.edit_existing",
		"known_facts": map[string]string{
			strings.Repeat("k", 65): "v",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	requireCategory(t, oversize, mcpcontract.InvalidInput)

	mismatch, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{
		"name":      "google_docs_documents_get",
		"intent_id": "gmail.find_message",
	}})
	if err != nil || mismatch.IsError {
		t.Fatalf("mismatch: %#v %v", mismatch, err)
	}

	if preparation(t, mismatch).Status != workflowguide.StatusNotApplicable {
		t.Fatalf("expected not_applicable: %s", callJSON(t, mismatch))
	}
}

func recipeSession(t *testing.T, names []string, writes bool, grantNames []string, scopes []string) *mcp.ClientSession {
	t.Helper()

	ops := make([]mcpcontract.Operation, 0, len(names))

	registry := map[string]mcpcontract.Operation{}
	for _, op := range apiexec.Operations(nil) {
		registry[op.Definition.Name] = op
	}
	allow := make([]string, 1, 1+len(names))
	allow[0] = "accounts_list"

	for _, name := range names {
		op, ok := registry[name]
		if !ok {
			t.Fatalf("missing real operation %s", name)
		}

		ops = append(ops, op)
		allow = append(allow, name)
	}

	if grantNames == nil {
		grantNames = names
	}

	accounts := access.NewMemoryAccounts(mcpcontract.Identity{
		AccountID:   "work",
		Subject:     "subject-work",
		Email:       "work@example.test",
		Label:       "Work",
		PrincipalID: "fixture",
		ClientName:  "app",
		AuthMode:    "oauth",
		Scopes:      scopes,
		Generation:  1,
	})

	cfg := compactConfig(ops)
	cfg.EnableWrites = writes
	cfg.AllowOperations = allow
	cfg.Accounts = accounts
	cfg.Grants = []mcpcontract.Grant{{
		PrincipalID: "fixture",
		AccountIDs:  []string{"work"},
		ClientNames: []string{"app"},
		Operations:  grantNames,
	}}

	runtime, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	return connectRuntime(t, t.Context(), runtime)
}

func docsScopes() []string {
	return []string{
		"https://www.googleapis.com/auth/documents",
		"https://www.googleapis.com/auth/drive",
	}
}

func preparation(t *testing.T, result *mcp.CallToolResult) workflowguide.PreparedRecipe {
	t.Helper()

	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}

	var envelope struct {
		Preparation *workflowguide.PreparedRecipe `json:"preparation"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}

	if envelope.Preparation == nil {
		t.Fatalf("missing preparation: %s", raw)
	}

	return *envelope.Preparation
}

func hasPrepLookup(prep workflowguide.PreparedRecipe, operation string) bool {
	for _, step := range prep.Steps {
		if step.Operation == operation && step.Kind == workflowguide.KindLookup {
			return true
		}
	}

	return false
}

func hasPrepMissing(prep workflowguide.PreparedRecipe, key string) bool {
	for _, fact := range prep.MissingPrerequisites {
		if fact.Key == key {
			return true
		}
	}

	return false
}
