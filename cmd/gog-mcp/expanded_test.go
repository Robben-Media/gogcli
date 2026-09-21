package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

type expandedTransport struct{ calls atomic.Int64 }

func (tr *expandedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.calls.Add(1)
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"spreadsheetId":"created-fixture"}`)), Request: req}, nil
}

type expandedProvider struct{ transport *expandedTransport }

func (p expandedProvider) HTTPClient(_ context.Context, _ mcpcontract.Identity, opts mcpcontract.CallOptions) (*http.Client, error) {
	return &http.Client{Transport: &googleapi.NativeRetryTransport{Base: p.transport, Class: opts.Retry}}, nil
}

func TestExtendedAuthoringThroughCompactProtocol(t *testing.T) {
	t.Parallel()

	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "writes_disabled", true: "writes_enabled"}[enabled], func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			registry := accountconnect.NewMemoryRegistry()
			for _, account := range []string{"allowed", "other"} {
				if err := registry.Upsert(ctx, accountconnect.Record{AccountID: account, Subject: "subject-" + account, Email: account + "@example.test", Label: account, PrincipalID: "fixture", ClientName: "native-mcp", AuthMode: accountconnect.AuthModeOAuth, Scopes: []string{mcpcontract.SheetsWriteScope, mcpcontract.DocsWriteScope}, Generation: 1, State: accountconnect.RecordStateActive, UpdatedAt: time.Now().UTC()}); err != nil {
					t.Fatal(err)
				}
			}
			transport := &expandedTransport{}
			runtime, err := mcpserver.New(mcpserver.Config{
				Principal:       mcpcontract.Principal{ID: "fixture"},
				Grants:          []mcpcontract.Grant{{PrincipalID: "fixture", AccountIDs: []string{"allowed"}, ClientNames: []string{"native-mcp"}, Operations: []string{"sheets:workflow.create", "google_docs_documents_batchupdate"}}},
				AllowOperations: []string{"accounts_list", "sheets_create_spreadsheet", "google_docs_documents_batchupdate"},
				Operations:      configuredOperations(cli{APICatalog: true}, expandedProvider{transport}), Accounts: registryAccounts{registry: registry},
				DiscoveryMode: mcpserver.DiscoveryCompact, EnableWrites: enabled, MaxUpstreamCalls: 1,
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			if err != nil {
				t.Fatal(err)
			}
			serverTransport, clientTransport := mcp.NewInMemoryTransports()
			serverSession, err := runtime.Server().Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer serverSession.Close()
			client := mcp.NewClient(&mcp.Implementation{Name: "expanded-fixture", Version: "test"}, nil)
			session, err := client.Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			listed, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(listed.Tools) != 4 {
				t.Fatalf("compact tools: %d", len(listed.Tools))
			}
			if enabled {
				for _, path := range []string{"", "input.properties.body.properties.requests.items.properties.insertText.properties.location"} {
					described, describeErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_describe", Arguments: map[string]any{"name": "google_docs_documents_batchupdate", "schema_path": path}})
					if describeErr != nil || described.IsError {
						t.Fatalf("describe path=%s err=%v result=%+v", path, describeErr, described)
					}
					encoded, encodeErr := json.Marshal(described)
					if encodeErr != nil || len(encoded) > 48000 {
						t.Fatalf("unbounded describe path=%s bytes=%d err=%v", path, len(encoded), encodeErr)
					}
					if path != "" && !strings.Contains(string(encoded), "index") {
						t.Fatalf("missing resolved Location.index: %s", encoded)
					}
				}
				if transport.calls.Load() != 0 {
					t.Fatal("schema inspection called Google")
				}
			}
			for _, account := range []string{"other", "allowed"} {
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{"name": "sheets_create_spreadsheet", "arguments": map[string]any{"account_id": account, "title": "Fixture", "sheet_title": "Data", "rows": []any{[]any{map[string]any{"text": "=literal"}}}}}})
				if err != nil {
					t.Fatal(err)
				}
				wantSuccess := enabled && account == "allowed"
				if result.IsError == wantSuccess {
					t.Fatalf("account=%s enabled=%v result=%+v", account, enabled, result)
				}
				if !wantSuccess && transport.calls.Load() != 0 {
					t.Fatal("denied call reached Google transport")
				}
				if wantSuccess {
					data, err := json.Marshal(result)
					if err != nil {
						t.Fatal(err)
					}
					if transport.calls.Load() != 1 || !strings.Contains(string(data), "created-fixture") || !strings.Contains(string(data), `"upstream_calls":1`) {
						t.Fatalf("call evidence: %s", data)
					}
				}
			}
		})
	}
}

func TestLegacyOperationsPreserved(t *testing.T) {
	t.Parallel()
	if got := len(configuredOperations(cli{}, nil)); got != 16 {
		t.Fatalf("default tool count changed: %d", got)
	}
}

func TestExpandedSchemasSerializeForDiscovery(t *testing.T) {
	t.Parallel()
	operations := configuredOperations(cli{APICatalog: true}, nil)
	if len(operations) < 900 {
		t.Fatalf("extended catalog unexpectedly small: %d", len(operations))
	}
	for _, operation := range operations {
		if schemaCycle(operation.InputSchema, make(map[*jsonschema.Schema]bool), make(map[*jsonschema.Schema]bool)) {
			t.Errorf("%s schema contains cyclic pointers instead of refs", operation.Definition.Name)
			continue
		}
		if _, err := json.Marshal(operation.InputSchema); err != nil {
			t.Errorf("%s schema cannot be discovered: %v", operation.Definition.Name, err)
		}
	}
}

// Cycles must use JSON Schema references, never recursive Go pointers: the SDK's
// custom MarshalJSON otherwise overflows the process stack before returning.
func schemaCycle(s *jsonschema.Schema, active, done map[*jsonschema.Schema]bool) bool {
	if s == nil || done[s] {
		return false
	}
	if active[s] {
		return true
	}
	active[s] = true
	children := []*jsonschema.Schema{s.Items, s.AdditionalProperties, s.Not}
	for _, group := range []map[string]*jsonschema.Schema{s.Properties, s.Defs, s.Definitions} {
		for _, child := range group {
			children = append(children, child)
		}
	}
	for _, group := range [][]*jsonschema.Schema{s.AnyOf, s.OneOf, s.AllOf, s.PrefixItems} {
		children = append(children, group...)
	}
	for _, child := range children {
		if schemaCycle(child, active, done) {
			return true
		}
	}
	delete(active, s)
	done[s] = true
	return false
}

func TestServerRejectsExplicitZeroAndInvalidBudgetBeforeOpeningStores(t *testing.T) {
	t.Parallel()
	for _, limit := range []int64{0, -1, 257} {
		if err := serve(cli{MaxUpstreamCalls: limit}, slog.New(slog.NewTextHandler(io.Discard, nil))); !errors.Is(err, errAPICallBudget) {
			t.Fatalf("limit=%d err=%v", limit, err)
		}
	}
}
