package googlecatalog

import (
	"regexp"
	"strings"
	"testing"
)

var checksumPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestGeneratedCatalogLoads(t *testing.T) {
	manifest := MustLoad()

	if manifest.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %d, want %d", manifest.SchemaVersion, SchemaVersion)
	}

	if manifest.Source.ModulePath != "google.golang.org/api" || manifest.Source.ModuleVersion != "v0.260.0" {
		t.Fatalf("unexpected pinned module %s@%s", manifest.Source.ModulePath, manifest.Source.ModuleVersion)
	}

	if !checksumPattern.MatchString(manifest.Source.APIListSHA256) {
		t.Fatalf("invalid API list checksum %q", manifest.Source.APIListSHA256)
	}

	if len(manifest.Source.Documents) != 29 {
		t.Fatalf("document count = %d, want 29", len(manifest.Source.Documents))
	}

	if len(manifest.Methods) != 1108 {
		t.Fatalf("method count = %d, want 1108", len(manifest.Methods))
	}

	if len(manifest.Scopes) != 184 {
		t.Fatalf("scope count = %d, want 184", len(manifest.Scopes))
	}

	documents := make(map[string]Document, len(manifest.Source.Documents))
	for _, document := range manifest.Source.Documents {
		if document.DiscoveryID == "" || document.Service == "" || document.Version == "" {
			t.Fatalf("incomplete document %s", document.LocalPath)
		}

		if !checksumPattern.MatchString(document.SHA256) {
			t.Fatalf("invalid source checksum for %s", document.LocalPath)
		}
		documents[documentKey(document)] = document
	}

	seenTools := make(map[string]bool, len(manifest.Methods))

	seenActions := make(map[string]bool, len(manifest.Methods))
	for _, method := range manifest.Methods {
		if seenTools[method.ToolName] {
			t.Fatalf("duplicate tool name %q", method.ToolName)
		}
		seenTools[method.ToolName] = true

		if !strings.HasPrefix(method.Action, method.Service+":") {
			t.Fatalf("action for %s = %q, want service-prefixed raw action", method.ID, method.Action)
		}

		if seenActions[method.Action] {
			t.Fatalf("duplicate action %q", method.Action)
		}
		seenActions[method.Action] = true

		document, ok := documents[documentKey(Document{Service: method.Service, Version: method.Version})]
		if !ok {
			t.Fatalf("method %s has no document", method.ID)
		}

		if err := validateSchemaRefs(method.Request, document.Schemas, method.ID); err != nil {
			t.Fatal(err)
		}

		if err := validateSchemaRefs(method.Response, document.Schemas, method.ID); err != nil {
			t.Fatal(err)
		}

		for _, scope := range method.Scopes {
			if _, ok := manifest.Scopes[scope]; !ok {
				t.Fatalf("method %s uses undeclared scope %q", method.ID, scope)
			}
		}
	}
}

func TestMethodByToolName(t *testing.T) {
	method, ok, err := MethodByToolName("google_gmail_users_messages_get")
	if err != nil {
		t.Fatal(err)
	}

	if !ok || method.ID != "gmail.users.messages.get" || method.Service != "gmail" {
		t.Fatalf("unexpected method %+v, found=%t", method, ok)
	}

	if _, ok, err := MethodByToolName("google_does_not_exist"); err != nil || ok {
		t.Fatalf("missing lookup returned found=%t err=%v", ok, err)
	}
}

func TestGatedMethods(t *testing.T) {
	methods, err := Methods()
	if err != nil {
		t.Fatal(err)
	}

	gated := 0

	for _, method := range methods {
		if len(method.Gates) == 0 {
			if len(method.Scopes) == 0 {
				t.Fatalf("ungated method %s has no scopes", method.ID)
			}

			continue
		}

		gated++

		if len(method.Scopes) != 0 {
			t.Fatalf("gated method %s unexpectedly has scopes", method.ID)
		}

		if len(method.Gates) != 1 || method.Gates[0].Kind != GateMissingScopes {
			t.Fatalf("method %s has unexpected gates %+v", method.ID, method.Gates)
		}

		if method.Supported() || method.UnavailableReason() == "" {
			t.Fatalf("gated method %s reports support", method.ID)
		}
	}

	if gated != 69 {
		t.Fatalf("gated method count = %d, want 69", gated)
	}
}

func TestCloudIdentityGroupsIdentityBoundary(t *testing.T) {
	methods, err := Methods()
	if err != nil {
		t.Fatal(err)
	}

	groupCount := 0
	otherCloudIdentityCount := 0

	for _, method := range methods {
		if method.Service != "cloud_identity" {
			continue
		}

		if strings.HasPrefix(method.ID, "cloudidentity.devices.") {
			continue
		}

		if strings.HasPrefix(method.ID, "cloudidentity.groups.") {
			groupCount++

			if len(method.Identity.Modes) != 2 ||
				method.Identity.Modes[0] != IdentityUser ||
				method.Identity.Modes[1] != IdentityServiceAccount {
				t.Fatalf("group method %s has modes %v", method.ID, method.Identity.Modes)
			}

			if method.Identity.AuthModel != "OAuth user or service account" {
				t.Fatalf("group method %s has auth model %q", method.ID, method.Identity.AuthModel)
			}

			supported, _ := method.SupportedForIdentity(IdentityUser)
			if !supported {
				t.Fatalf("group method %s does not support user OAuth", method.ID)
			}

			continue
		}

		otherCloudIdentityCount++

		if len(method.Identity.Modes) != 1 || method.Identity.Modes[0] != IdentityServiceAccount {
			t.Fatalf("unrelated Cloud Identity method %s has modes %v", method.ID, method.Identity.Modes)
		}

		supported, reason := method.SupportedForIdentity(IdentityUser)
		if supported || reason == "" {
			t.Fatalf("unrelated Cloud Identity method %s unexpectedly supports user OAuth", method.ID)
		}
	}

	if groupCount != 20 {
		t.Fatalf("Cloud Identity Groups method count = %d, want 20", groupCount)
	}

	if otherCloudIdentityCount != 26 {
		t.Fatalf("other Cloud Identity method count = %d, want 26", otherCloudIdentityCount)
	}
}

//nolint:wsl_v5 // keep each method-family assertion next to its fixture loop.
func TestCloudIdentityDevicesIdentityBoundary(t *testing.T) {
	methods, err := Methods()
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, method := range methods {
		if !strings.HasPrefix(method.ID, "cloudidentity.devices.") {
			continue
		}
		count++
		if len(method.Identity.Modes) != 2 ||
			method.Identity.Modes[0] != IdentityUser ||
			method.Identity.Modes[1] != IdentityServiceAccount {
			t.Fatalf("device method %s has modes %v", method.ID, method.Identity.Modes)
		}
		if method.Identity.AuthModel != "OAuth user or service account" {
			t.Fatalf("device method %s has auth model %q", method.ID, method.Identity.AuthModel)
		}
		supported, _ := method.SupportedForIdentity(IdentityUser)
		if !supported {
			t.Fatalf("device method %s does not support user OAuth", method.ID)
		}
	}

	if count != 17 {
		t.Fatalf("Cloud Identity Devices method count = %d, want 17", count)
	}
}

//nolint:wsl_v5 // recursive source-faithfulness assertions are clearer inline.
func TestMethodSpecificRequiredFields(t *testing.T) {
	manifest := MustLoad()
	count := 0
	var eventStart, eventEnd, eventICalUID *Schema

	for _, document := range manifest.Source.Documents {
		methodIDs := make(map[string]bool)
		for _, method := range manifest.Methods {
			if method.Service == document.Service && method.Version == document.Version {
				methodIDs[method.ID] = true
			}
		}

		seen := make(map[*Schema]bool)
		var walk func(*Schema)
		walk = func(schema *Schema) {
			if schema == nil || seen[schema] {
				return
			}
			seen[schema] = true
			if len(schema.RequiredFor) > 0 {
				count++
				for _, methodID := range schema.RequiredFor {
					if !methodIDs[methodID] {
						t.Fatalf("%s schema requires unknown method %q", document.LocalPath, methodID)
					}
				}
			}
			walk(schema.Items)
			walk(schema.AdditionalProperties)

			for _, candidate := range schema.AnyOf {
				walk(candidate)
			}
			for _, candidate := range schema.OneOf {
				walk(candidate)
			}
			for _, candidate := range schema.AllOf {
				walk(candidate)
			}
			for _, property := range schema.Properties {
				walk(property)
			}
		}

		for _, schema := range document.Schemas {
			walk(schema)
		}

		if document.Service != "calendar" {
			continue
		}
		eventStart = document.Schemas["Event"].Properties["start"]
		eventEnd = document.Schemas["Event"].Properties["end"]
		eventICalUID = document.Schemas["Event"].Properties["iCalUID"]
		if len(document.Schemas["Event"].Required) != 0 {
			t.Fatal("Event has global required fields")
		}
	}

	if count != 64 {
		t.Fatalf("method-specific required annotations = %d, want 64", count)
	}
	if eventStart == nil || eventEnd == nil || eventICalUID == nil {
		t.Fatal("Calendar Event required-field fixtures are missing")
	}
	if strings.Join(eventStart.RequiredFor, ",") != "calendar.events.import,calendar.events.insert,calendar.events.update" {
		t.Fatalf("Event.start required_for = %v", eventStart.RequiredFor)
	}
	if strings.Join(eventEnd.RequiredFor, ",") != "calendar.events.import,calendar.events.insert,calendar.events.update" {
		t.Fatalf("Event.end required_for = %v", eventEnd.RequiredFor)
	}
	if strings.Join(eventICalUID.RequiredFor, ",") != "calendar.events.import" {
		t.Fatalf("Event.iCalUID required_for = %v", eventICalUID.RequiredFor)
	}
}

//nolint:wsl_v5 // keep the expected and observed count checks adjacent.
func TestAnalyticsSafeReadMethods(t *testing.T) {
	expected := map[string]int{
		"analyticsadmin.accounts.runAccessReport":       2,
		"analyticsadmin.properties.runAccessReport":     2,
		"analyticsdata.properties.batchRunPivotReports": 1,
		"analyticsdata.properties.batchRunReports":      1,
		"analyticsdata.properties.checkCompatibility":   1,
		"analyticsdata.properties.runRealtimeReport":    1,
		"analyticsdata.properties.runPivotReport":       1,
		"analyticsdata.properties.runReport":            1,
	}
	seen := make(map[string]int, len(expected))

	methods, err := Methods()
	if err != nil {
		t.Fatal(err)
	}

	for _, method := range methods {
		if _, tracked := expected[method.ID]; !tracked {
			continue
		}
		seen[method.ID]++
		if !method.ReadOnly {
			t.Fatalf("%s is not read-only", method.ID)
		}
	}
	for methodID, count := range expected {
		if seen[methodID] != count {
			t.Fatalf("%s read-only count = %d, want %d", methodID, seen[methodID], count)
		}
	}
}

//nolint:wsl_v5 // keep upload endpoint assertions next to the located method.
func TestMediaUploadProtocolPaths(t *testing.T) {
	manifest := MustLoad()
	for _, method := range manifest.Methods {
		if method.ID != "drive.files.create" {
			continue
		}
		if method.Media == nil || method.Media.Upload == nil {
			t.Fatal("drive.files.create has no media upload")
		}
		upload := method.Media.Upload
		if upload.SimplePath != "/upload/drive/v3/files" || upload.MultipartPath != "/upload/drive/v3/files" {
			t.Fatalf("simple/multipart paths = %q/%q", upload.SimplePath, upload.MultipartPath)
		}
		if upload.ResumablePath != "/resumable/upload/drive/v3/files" {
			t.Fatalf("resumable path = %q", upload.ResumablePath)
		}

		return
	}
	t.Fatal("drive.files.create is missing")
}

func TestSchemaReferenceValidation(t *testing.T) {
	definitions := map[string]*Schema{
		"Message": {Type: "object", Properties: map[string]*Schema{
			"thread": {Ref: "Thread"},
		}},
		"Thread": {Type: "object"},
	}

	request := &Schema{Ref: "Message"}
	if err := validateSchemaRefs(request, definitions, "test.method"); err != nil {
		t.Fatal(err)
	}

	if request.Ref != "Message" {
		t.Fatalf("reference was rewritten to %q", request.Ref)
	}

	err := validateSchemaRefs(&Schema{Ref: "Missing"}, definitions, "test.method")
	if err == nil || !strings.Contains(err.Error(), `missing schema "Missing"`) {
		t.Fatalf("missing reference error = %v", err)
	}
}

func TestSchemaReferenceCycleTraversal(t *testing.T) {
	loop := &Schema{Type: "object"}
	loop.Properties = map[string]*Schema{"self": loop}
	definitions := map[string]*Schema{"Loop": loop}
	request := &Schema{Ref: "Loop"}

	if err := validateSchemaRefs(request, definitions, "test.cycle"); err != nil {
		t.Fatal(err)
	}

	if request.Ref != "Loop" || loop.Properties["self"] != loop {
		t.Fatal("cycle schema was expanded or copied")
	}
}
