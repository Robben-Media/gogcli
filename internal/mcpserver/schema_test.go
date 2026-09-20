package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestProjectSchemaKeepsSmallSchemasExact(t *testing.T) {
	t.Parallel()

	schema := &jsonschema.Schema{
		Type:     "object",
		Required: []string{"account_id", "query"},
		Properties: map[string]*jsonschema.Schema{
			"account_id": {Type: "string"},
			"query":      {Type: "string", Description: "mailbox search"},
		},
	}

	raw, inspect, truncated := projectSchema(schema, schema, "input")

	want, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}

	if truncated || len(inspect) != 0 || string(raw) != string(want) {
		t.Fatalf("small schema mutated: truncated=%v inspect=%v raw=%s want=%s", truncated, inspect, raw, want)
	}
}

func TestProjectSchemaSummarizesLargeTreesAndWalksPaths(t *testing.T) {
	t.Parallel()

	schema := largeDocsSchema()

	raw, inspect, truncated := projectSchema(schema, schema, "input")
	if !truncated {
		t.Fatal("large schema was not summarized")
	}

	if len(raw) > maxDescribeSchemaBytes {
		t.Fatalf("summarized schema still %d bytes", len(raw))
	}

	if strings.Contains(string(raw), "nestedBlob-secret") {
		t.Fatal("default summary leaked nested blob")
	}

	if !strings.Contains(string(raw), "requests") || !strings.Contains(string(raw), "writeControl") {
		t.Fatalf("summary missing shallow fields: %s", raw)
	}

	if !containsPath(inspect, "input.properties.requests") || !containsPath(inspect, "input.properties.writeControl") {
		t.Fatalf("inspect paths = %v", inspect)
	}

	node, err := walkSchemaPath(schema, []string{"properties", "requests", "items", "replaceAllText", "containsText"})
	if err != nil {
		t.Fatal(err)
	}

	drilled, drilledInspect, drilledTrunc := projectSchema(schema, node, "input.properties.requests.items.replaceAllText.containsText")
	if !drilledTrunc {
		t.Fatal("huge nested object should still be marked truncated")
	}

	if len(drilled) > maxDescribeSchemaBytes {
		t.Fatalf("drilled schema still %d bytes", len(drilled))
	}

	if !strings.Contains(string(drilled), "nestedBlob") {
		t.Fatalf("drilldown missed nested field: %s", drilled)
	}

	if len(drilledInspect) != 0 {
		t.Fatalf("leaf inspect paths = %v", drilledInspect)
	}

	if _, err = walkSchemaPath(schema, []string{"properties", "missing"}); err == nil {
		t.Fatal("missing path accepted")
	}
}

func TestDescribeOperationSchemasRejectsBadPaths(t *testing.T) {
	t.Parallel()

	op := mcpcontract.Operation{InputSchema: largeDocsSchema(), OutputSchema: &jsonschema.Schema{Type: "object"}}
	if _, _, _, _, _, err := describeOperationSchemas(op, "body.requests"); err == nil {
		t.Fatal("path without input/output accepted")
	}

	if _, _, _, _, _, err := describeOperationSchemas(op, strings.Repeat("input.properties.x", 40)); err == nil {
		t.Fatal("oversized path accepted")
	}

	in, out, path, inspect, truncated, err := describeOperationSchemas(op, "input.properties.requests")
	if err != nil {
		t.Fatal(err)
	}

	if path != "input.properties.requests" || string(out) != "{}" || !truncated {
		t.Fatalf("path=%s out=%s truncated=%v inspect=%v", path, out, truncated, inspect)
	}

	if !strings.Contains(string(in), `"type":"array"`) || strings.Contains(string(in), "nestedBlob-secret") {
		t.Fatalf("array summary leaked nested schema: %s", in)
	}
}

func largeDocsSchema() *jsonschema.Schema {
	blob := &jsonschema.Schema{Type: "string", Description: strings.Repeat("nestedBlob-secret ", 4000)}
	contains := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"text":       {Type: "string"},
			"nestedBlob": blob,
		},
		Required: []string{"text"},
	}
	replace := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"containsText": contains,
			"replaceText":  {Type: "string"},
		},
	}
	request := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"replaceAllText": replace,
			"insertText": {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"text":       {Type: "string"},
					"nestedBlob": blob,
				},
			},
		},
	}

	return &jsonschema.Schema{
		Type:     "object",
		Required: []string{"account_id", "document_id"},
		Properties: map[string]*jsonschema.Schema{
			"account_id":  {Type: "string"},
			"document_id": {Type: "string"},
			"requests":    {Type: "array", Items: request},
			"writeControl": {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"requiredRevisionId": {Type: "string"},
					"nestedBlob":         blob,
				},
			},
		},
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}

	return false
}

func TestWalkSchemaPathFollowsJSONSchemaRefs(t *testing.T) {
	t.Parallel()

	blob := &jsonschema.Schema{Type: "string", Description: strings.Repeat("nestedBlob-secret ", 4000)}
	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"account_id": {Type: "string"},
			"requests":   {Type: "array", Items: &jsonschema.Schema{Ref: "#/$defs/Request"}},
		},
		Defs: map[string]*jsonschema.Schema{
			"Request": {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"replaceAllText": {Ref: "#/$defs/ReplaceAllTextRequest"},
				},
			},
			"ReplaceAllTextRequest": {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"containsText": {Ref: "#/$defs/SubstringMatchCriteria"},
					"replaceText":  {Type: "string"},
				},
			},
			"SubstringMatchCriteria": {
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{"text": {Type: "string"}, "nestedBlob": blob},
			},
			"Document": {Type: "object", Properties: map[string]*jsonschema.Schema{"body": {Ref: "#/$defs/Body"}}},
			"Body":     {Type: "object", Properties: map[string]*jsonschema.Schema{"content": {Type: "array"}}},
		},
	}

	raw, inspect, truncated := projectSchema(schema, schema, "input")
	if !truncated {
		t.Fatal("ref schema was not summarized")
	}

	if strings.Contains(string(raw), "nestedBlob-secret") || strings.Contains(string(raw), `"Document"`) {
		t.Fatalf("summary leaked defs or nested blob: %s", raw)
	}

	if !containsPath(inspect, "input.properties.requests") {
		t.Fatalf("usable inspect path missing: %v", inspect)
	}

	for _, path := range inspect {
		if strings.HasPrefix(path, "input.defs.") {
			t.Fatalf("inspect path hid fields behind defs: %v", inspect)
		}
	}

	node, err := walkSchemaPath(schema, []string{"properties", "requests", "items", "replaceAllText", "containsText"})
	if err != nil {
		t.Fatal(err)
	}

	drilled, _, _ := projectSchema(schema, node, "input.properties.requests.items.replaceAllText.containsText")
	if !strings.Contains(string(drilled), "nestedBlob") {
		t.Fatalf("ref walk missed nested field: %s", drilled)
	}

	viaDefs, err := walkSchemaPath(schema, []string{"defs", "Request"})
	if err != nil {
		t.Fatal(err)
	}

	if viaDefs == nil || viaDefs.Properties["replaceAllText"] == nil {
		t.Fatal("defs path should still resolve")
	}
}

func TestSummarizePreservesLeafCombinators(t *testing.T) {
	t.Parallel()

	schema := &jsonschema.Schema{
		Type:        "object",
		Description: strings.Repeat("wide-description ", 800),
		AnyOf:       []*jsonschema.Schema{{Type: "string"}, {Type: "integer"}},
	}

	raw, inspect, truncated := projectSchema(schema, schema, "input")
	if !truncated {
		t.Fatal("wide combinator schema was not summarized")
	}

	if len(raw) > maxDescribeSchemaBytes {
		t.Fatalf("summarized combinator schema %d bytes", len(raw))
	}

	if !strings.Contains(string(raw), `"anyOf"`) || !strings.Contains(string(raw), `"string"`) || !strings.Contains(string(raw), `"integer"`) {
		t.Fatalf("leaf anyOf dropped: %s inspect=%v", raw, inspect)
	}
}

func TestSummarizedSchemaStaysWithinByteBudget(t *testing.T) {
	t.Parallel()

	properties := make(map[string]*jsonschema.Schema, 64)

	description := strings.Repeat("d", 240)
	for i := 0; i < 64; i++ {
		properties[fmt.Sprintf("field_%02d", i)] = &jsonschema.Schema{Type: "string", Description: description}
	}

	schema := &jsonschema.Schema{Type: "object", Properties: properties}

	raw, inspect, truncated := projectSchema(schema, schema, "input")
	if !truncated {
		t.Fatal("wide object schema was not summarized")
	}

	if len(raw) > maxDescribeSchemaBytes {
		t.Fatalf("summarized schema %d bytes inspect=%d", len(raw), len(inspect))
	}

	if !strings.Contains(string(raw), "field_") && len(inspect) == 0 {
		t.Fatal("summary omitted every field and inspect path")
	}
}

func TestWalkSchemaPathFollowsNot(t *testing.T) {
	t.Parallel()

	schema := &jsonschema.Schema{
		Type:        "object",
		Description: strings.Repeat("wide-not-description ", 600),
		Not: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"flag": {Type: "boolean"},
			},
		},
	}

	raw, inspect, truncated := projectSchema(schema, schema, "input")
	if !truncated {
		t.Fatal("wide not schema was not summarized")
	}

	if !containsPath(inspect, "input.not") {
		t.Fatalf("missing not inspect path: %v raw=%s", inspect, raw)
	}

	node, err := walkSchemaPath(schema, []string{"not", "properties", "flag"})
	if err != nil || node == nil || node.Type != "boolean" {
		t.Fatalf("not path: %#v %v", node, err)
	}
}
