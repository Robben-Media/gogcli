package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The Google adapter snapshot owns the other sixteen tool contracts.
func TestFrozenAccountCatalogSchema(t *testing.T) {
	input, output := accountsListSchemas()

	data, err := json.MarshalIndent(map[string]any{"input": input, "output": output}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	data = append(data, '\n')

	path := filepath.Join("testdata", "accounts-list-schema.json")
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
		t.Fatal("accounts_list schema changed; review contract before updating with UPDATE_MCP_SCHEMAS=1")
	}
}
