package outfmt

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestFromFlags(t *testing.T) {
	if _, err := FromFlags(true, true); err == nil {
		t.Fatalf("expected error when combining --json and --plain")
	}

	got, err := FromFlags(true, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !got.JSON || got.Plain {
		t.Fatalf("unexpected mode: %#v", got)
	}
}

func TestContextMode(t *testing.T) {
	ctx := context.Background()

	if IsJSON(ctx) || IsPlain(ctx) {
		t.Fatalf("expected default text")
	}
	ctx = WithMode(ctx, Mode{JSON: true})

	if !IsJSON(ctx) || IsPlain(ctx) {
		t.Fatalf("expected json-only")
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, map[string]any{"ok": true}); err != nil {
		t.Fatalf("err: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatalf("expected output")
	}
}

func TestWriteJSONWithConfig_ResultsOnlyUsesDeclaredPrimaryResult(t *testing.T) {
	var buf bytes.Buffer
	value := PrimaryResult(
		map[string]any{
			"files":         []map[string]any{{"id": "f1"}},
			"nextPageToken": "next",
		},
		[]map[string]any{{"id": "f1"}},
	)

	if err := WriteJSONWithConfig(&buf, value, JSONConfig{ResultsOnly: true}); err != nil {
		t.Fatalf("WriteJSONWithConfig: %v", err)
	}

	if got, want := buf.String(), "[\n  {\n    \"id\": \"f1\"\n  }\n]\n"; got != want {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestWriteJSONWithConfig_SelectionPreservesNestedShapeAndOmitsMissingPaths(t *testing.T) {
	var buf bytes.Buffer
	value := []map[string]any{
		{"id": "m1", "sender": map[string]any{"email": "a@example.com", "name": "A"}},
		{"id": "m2", "sender": map[string]any{"name": "B"}},
	}

	if err := WriteJSONWithConfig(&buf, value, JSONConfig{Select: []string{"id", "sender.email"}}); err != nil {
		t.Fatalf("WriteJSONWithConfig: %v", err)
	}

	if got, want := buf.String(), "[\n  {\n    \"id\": \"m1\",\n    \"sender\": {\n      \"email\": \"a@example.com\"\n    }\n  },\n  {\n    \"id\": \"m2\"\n  }\n]\n"; got != want {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestWriteJSONWithConfig_ComposesResultsOnlyBeforeSelection(t *testing.T) {
	var buf bytes.Buffer
	files := []map[string]any{{"id": "f1", "name": "Doc", "mimeType": "text/plain"}}
	value := PrimaryResult(map[string]any{"files": files, "nextPageToken": "next"}, files)

	if err := WriteJSONWithConfig(&buf, value, JSONConfig{ResultsOnly: true, Select: []string{"id", "name"}}); err != nil {
		t.Fatalf("WriteJSONWithConfig: %v", err)
	}

	if got, want := buf.String(), "[\n  {\n    \"id\": \"f1\",\n    \"name\": \"Doc\"\n  }\n]\n"; got != want {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestWriteJSONWithConfig_RejectsNonProjectableTargets(t *testing.T) {
	var buf bytes.Buffer

	err := WriteJSONWithConfig(&buf, "not-an-object", JSONConfig{Select: []string{"id"}})
	if err == nil || !strings.Contains(err.Error(), "cannot select fields from JSON string") {
		t.Fatalf("expected clear projection error, got %v", err)
	}

	if buf.Len() != 0 {
		t.Fatalf("unexpected partial output: %q", buf.String())
	}
}

func TestWriteJSONWithConfig_ResultsOnlyRequiresExplicitContract(t *testing.T) {
	var buf bytes.Buffer

	err := WriteJSONWithConfig(&buf, map[string]any{"files": []any{}, "nextPageToken": "next"}, JSONConfig{ResultsOnly: true})
	if err == nil || !strings.Contains(err.Error(), "does not declare a primary result") {
		t.Fatalf("expected explicit primary-result error, got %v", err)
	}
}

func TestConfigureJSON_AppliesToWriteJSONAndRestoresPreviousConfig(t *testing.T) {
	restore, err := ConfigureJSON(JSONConfig{Select: []string{"id"}})
	if err != nil {
		t.Fatalf("ConfigureJSON: %v", err)
	}

	var selected bytes.Buffer
	if err := WriteJSON(&selected, map[string]any{"id": "f1", "name": "Doc"}); err != nil {
		t.Fatalf("WriteJSON configured: %v", err)
	}

	restore()

	var ordinary bytes.Buffer
	if err := WriteJSON(&ordinary, map[string]any{"id": "f1", "name": "Doc"}); err != nil {
		t.Fatalf("WriteJSON restored: %v", err)
	}

	if got, want := selected.String(), "{\n  \"id\": \"f1\"\n}\n"; got != want {
		t.Fatalf("selected output mismatch: got %q want %q", got, want)
	}

	if got, want := ordinary.String(), "{\n  \"id\": \"f1\",\n  \"name\": \"Doc\"\n}\n"; got != want {
		t.Fatalf("ordinary output mismatch: got %q want %q", got, want)
	}
}

func TestConfigureJSON_RejectsInvalidSelectionPaths(t *testing.T) {
	for _, path := range []string{"", ".id", "sender.", "sender..email"} {
		if _, err := ConfigureJSON(JSONConfig{Select: []string{path}}); err == nil {
			t.Fatalf("expected invalid path %q to fail", path)
		}
	}
}

func TestFromEnvAndParseError(t *testing.T) {
	t.Setenv("GOG_JSON", "yes")
	t.Setenv("GOG_PLAIN", "0")
	mode := FromEnv()

	if !mode.JSON || mode.Plain {
		t.Fatalf("unexpected env mode: %#v", mode)
	}

	if err := (&ParseError{msg: "boom"}).Error(); err != "boom" {
		t.Fatalf("unexpected parse error: %q", err)
	}
}

func TestFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey{}, "nope")
	if got := FromContext(ctx); got != (Mode{}) {
		t.Fatalf("expected zero mode, got %#v", got)
	}
}
