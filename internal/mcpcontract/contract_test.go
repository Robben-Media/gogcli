package mcpcontract

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCatalogBoundary(t *testing.T) {
	t.Parallel()

	defs := Catalog()
	if len(defs) != 17 {
		t.Fatalf("got %d pilot tools", len(defs))
	}

	seen := map[string]bool{}
	for _, d := range defs {
		if seen[d.Name] {
			t.Fatalf("duplicate %s", d.Name)
		}

		seen[d.Name] = true
		if d.Local {
			if d.Name != "accounts_list" || len(d.Actions) != 0 {
				t.Fatal("invalid local operation")
			}

			continue
		}

		if d.Retry != SafeRead || len(d.Actions) == 0 || len(d.Scopes) == 0 {
			t.Fatalf("unsafe incomplete definition: %s", d.Name)
		}
	}
	defs[1].Actions[0] = "gmail:send"

	got, _ := Lookup("gmail_search")
	if got.Actions[0] != "gmail:messages.search" {
		t.Fatal("catalog can be mutated through copies")
	}
}

type metadataInput struct {
	Selection
	Kind string `json:"kind"`
}
type metadataOutput struct {
	Names []string `json:"names"`
}

func TestDecodeAuthorizationProjection(t *testing.T) {
	t.Parallel()
	op := NewOperation("analytics_metadata", nil, func(context.Context, Identity, metadataInput) (Result[metadataOutput], error) {
		return Result[metadataOutput]{}, nil
	})

	for _, tc := range []struct {
		kind    string
		actions []string
	}{
		{"dimensions", []string{"analytics:dimensions"}}, {"metrics", []string{"analytics:metrics"}}, {"both", []string{"analytics:dimensions", "analytics:metrics"}},
	} {
		raw, _ := json.Marshal(metadataInput{Selection: Selection{AccountID: "opaque-a"}, Kind: tc.kind})

		call, err := op.Decode(raw)
		if err != nil || !reflect.DeepEqual(call.Actions, tc.actions) {
			t.Fatalf("%s: %#v %v", tc.kind, call.Actions, err)
		}
	}

	for _, raw := range []string{`{}`, `null`, `{"account_id":"a","kind":"unknown"}`, `{"account_id":"a","kind":"both","principal_id":"admin"}`, `{"account_id":"a","kind":"both"} {}`} {
		if _, err := op.Decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestOperationCannotMutateCapturedActions(t *testing.T) {
	t.Parallel()
	op := NewOperation("gmail_get_message", nil, func(context.Context, Identity, Selection) (Result[metadataOutput], error) {
		return Result[metadataOutput]{}, nil
	})
	op.Definition.Actions[0] = "gmail:send"

	call, err := op.Decode(json.RawMessage(`{"account_id":"a"}`))
	if err != nil || len(call.Actions) != 1 || call.Actions[0] != "gmail:get" {
		t.Fatalf("mutable actions: %v %v", call.Actions, err)
	}
}

func TestValidatorDoesNotExposeRawErrors(t *testing.T) {
	t.Parallel()
	op := NewOperation("gmail_get_message", func(Selection) error { return errPrivateDetail }, func(context.Context, Identity, Selection) (Result[metadataOutput], error) {
		return Result[metadataOutput]{}, nil
	})

	_, err := op.Decode(json.RawMessage(`{"account_id":"a"}`))
	if err == nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("unsafe validation error: %v", err)
	}
}

var errPrivateDetail = errors.New("private detail")
