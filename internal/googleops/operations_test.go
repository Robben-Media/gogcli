package googleops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestFrozenPilotSchemas(t *testing.T) {
	ops := Operations(nil)
	if len(ops) != 16 {
		t.Fatalf("got %d Google tools", len(ops))
	}
	schemas := map[string]map[string]any{}

	for _, op := range ops {
		def, ok := mcpcontract.Lookup(op.Definition.Name)
		if !ok || def.Local || !reflect.DeepEqual(def, op.Definition) {
			t.Fatalf("catalog drift: %s", op.Definition.Name)
		}

		if _, exists := schemas[def.Name]; exists {
			t.Fatalf("duplicate tool: %s", def.Name)
		}
		schemas[def.Name] = map[string]any{"input": op.InputSchema, "output": op.OutputSchema}
	}

	data, err := json.MarshalIndent(schemas, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	data = append(data, '\n')

	path := filepath.Join("testdata", "tool-schemas.json")
	if os.Getenv("UPDATE_MCP_SCHEMAS") == "1" {
		if err = os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err = os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != string(want) {
		t.Fatal("public MCP schema changed; review contract before updating snapshot with UPDATE_MCP_SCHEMAS=1")
	}
}
