package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/googleops"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

var (
	errUnexpectedCatalogEndpoint = errors.New("unexpected catalog fixture endpoint")
	errMissingCatalogResponse    = errors.New("catalog fixture response is missing")
	errCatalogRequestDeadline    = errors.New("native catalog request has no deadline")
)

type catalogRequest struct {
	Method    string
	Path      string
	AccountID string
}

type catalogTransport struct {
	accountID string
	responses map[string]json.RawMessage
	requests  *[]catalogRequest
}

func (transport *catalogTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if _, ok := request.Context().Deadline(); !ok {
		return nil, errCatalogRequestDeadline
	}

	key, ok := catalogResponseKey(request)
	if !ok {
		return nil, fmt.Errorf("%w: %s %s", errUnexpectedCatalogEndpoint, request.Method, request.URL.Path)
	}

	response, exists := transport.responses[key]
	if !exists {
		return nil, fmt.Errorf("%w for %s", errMissingCatalogResponse, key)
	}

	*transport.requests = append(*transport.requests, catalogRequest{
		Method: request.Method, Path: request.URL.Path, AccountID: transport.accountID,
	})

	return &http.Response{
		StatusCode: http.StatusOK, Status: "200 OK", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(bytes.NewReader(response)), ContentLength: int64(len(response)),
	}, nil
}

func catalogResponseKey(request *http.Request) (string, bool) {
	path := request.URL.Path
	switch {
	case path == "/gmail/v1/users/me/labels":
		return "gmail_labels", true
	case path == "/gmail/v1/users/me/messages" && request.Method == http.MethodGet:
		return "gmail_search_list", true
	case path == "/gmail/v1/users/me/messages/message-first":
		return "gmail_search_message", true
	case path == "/gmail/v1/users/me/threads/thread-acme":
		return "gmail_thread", true
	case path == "/drive/v3/files" && request.Method == http.MethodGet:
		return "drive_search", true
	case path == "/drive/v3/files/file-acme":
		return "drive_get_file", true
	case path == "/v1/documents/doc-acme":
		return "docs_get_text", true
	case path == "/calendar/v3/users/me/calendarList":
		return "calendar_list", true
	case path == "/calendar/v3/calendars/primary/events":
		return "calendar_list_events", true
	case path == "/calendar/v3/freeBusy":
		return "calendar_freebusy", true
	case path == "/v1beta/accountSummaries":
		return "analytics_list_properties", true
	case path == "/v1beta/properties/123/metadata":
		return "analytics_metadata", true
	case path == "/v1beta/properties/123:runReport":
		return "analytics_report", true
	case path == "/webmasters/v3/sites":
		return "searchconsole_list_sites", true
	case strings.HasPrefix(path, "/webmasters/v3/sites/") && strings.HasSuffix(path, "/searchAnalytics/query"):
		return "searchconsole_query", true
	case path == "/v4/spreadsheets/sheet-client-budget":
		return "sheets_get_metadata", true
	case strings.HasPrefix(path, "/v4/spreadsheets/sheet-client-budget/values/"):
		return "sheets_read_range", true
	default:
		return "", false
	}
}

type catalogProvider struct {
	responses  map[string]json.RawMessage
	requests   *[]catalogRequest
	identities []mcpcontract.Identity
	options    []mcpcontract.CallOptions
}

func (provider *catalogProvider) HTTPClient(_ context.Context, identity mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	provider.identities = append(provider.identities, identity.Clone())
	provider.options = append(provider.options, options)

	return &http.Client{Transport: &catalogTransport{
		accountID: identity.AccountID, responses: provider.responses, requests: provider.requests,
	}}, nil
}

type catalogToolCall struct {
	name      string
	arguments map[string]any
	endpoints []catalogRequest
	content   []string
}

func catalogToolCalls() []catalogToolCall {
	return []catalogToolCall{
		{
			name:      "gmail_search",
			arguments: map[string]any{"account_id": "catalog-account", "query": "from:acme@example.test", "max_results": 25},
			endpoints: []catalogRequest{
				{Method: http.MethodGet, Path: "/gmail/v1/users/me/messages", AccountID: "catalog-account"},
				{Method: http.MethodGet, Path: "/gmail/v1/users/me/labels", AccountID: "catalog-account"},
				{Method: http.MethodGet, Path: "/gmail/v1/users/me/messages/message-first", AccountID: "catalog-account"},
			},
			content: []string{"message-first", "thread-acme"},
		},
		{
			name:      "gmail_get_message",
			arguments: map[string]any{"account_id": "catalog-account", "message_id": "message-first"},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/gmail/v1/users/me/messages/message-first", AccountID: "catalog-account"}},
			content:   []string{"message-first", "Acme project update"},
		},
		{
			name:      "gmail_get_thread",
			arguments: map[string]any{"account_id": "catalog-account", "thread_id": "thread-acme", "max_messages": 2},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/gmail/v1/users/me/threads/thread-acme", AccountID: "catalog-account"}},
			content:   []string{"message-first", "Review is Friday"},
		},
		{
			name:      "drive_search",
			arguments: map[string]any{"account_id": "catalog-account", "text": "Acme brief", "max_results": 25},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/drive/v3/files", AccountID: "catalog-account"}},
			content:   []string{"doc-acme", "Acme brief"},
		},
		{
			name:      "drive_get_file",
			arguments: map[string]any{"account_id": "catalog-account", "file_id": "file-acme"},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/drive/v3/files/file-acme", AccountID: "catalog-account"}},
			content:   []string{"file-acme", "Catalog file"},
		},
		{
			name:      "docs_get_text",
			arguments: map[string]any{"account_id": "catalog-account", "document_id": "doc-acme", "max_bytes": 128},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/v1/documents/doc-acme", AccountID: "catalog-account"}},
			content:   []string{"Launch date: 2026-10-01", "tab-main"},
		},
		{
			name:      "calendar_list",
			arguments: map[string]any{"account_id": "catalog-account"},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/calendar/v3/users/me/calendarList", AccountID: "catalog-account"}},
			content:   []string{"primary", "Catalog calendar"},
		},
		{
			name: "calendar_list_events",
			arguments: map[string]any{
				"account_id": "catalog-account", "calendar_id": "primary",
				"time_min": "2026-11-01T00:00:00-05:00", "time_max": "2026-11-02T00:00:00-06:00", "max_results": 25,
			},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/calendar/v3/calendars/primary/events", AccountID: "catalog-account"}},
			content:   []string{"event-all-day", "series-all-day"},
		},
		{
			name: "calendar_freebusy",
			arguments: map[string]any{
				"account_id": "catalog-account", "calendar_ids": []string{"primary", "team", "unavailable"},
				"time_min": "2026-11-01T00:00:00-05:00", "time_max": "2026-11-02T00:00:00-06:00",
			},
			endpoints: []catalogRequest{{Method: http.MethodPost, Path: "/calendar/v3/freeBusy", AccountID: "catalog-account"}},
			content:   []string{"unavailable"},
		},
		{
			name:      "analytics_list_properties",
			arguments: map[string]any{"account_id": "catalog-account", "page_size": 1},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/v1beta/accountSummaries", AccountID: "catalog-account"}},
			content:   []string{"properties/123", "Catalog account"},
		},
		{
			name:      "analytics_metadata",
			arguments: map[string]any{"account_id": "catalog-account", "property": "123", "kind": "dimensions"},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/v1beta/properties/123/metadata", AccountID: "catalog-account"}},
			content:   []string{"date", "Date"},
		},
		{
			name: "analytics_report",
			arguments: map[string]any{
				"account_id": "catalog-account", "property": "properties/123", "metrics": []string{"sessions"},
				"dimensions": []string{"date"}, "start_date": "2026-09-01", "end_date": "2026-09-07", "limit": 100,
			},
			endpoints: []catalogRequest{{Method: http.MethodPost, Path: "/v1beta/properties/123:runReport", AccountID: "catalog-account"}},
			content:   []string{"America/Chicago", "120"},
		},
		{
			name:      "searchconsole_list_sites",
			arguments: map[string]any{"account_id": "catalog-account"},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/webmasters/v3/sites", AccountID: "catalog-account"}},
			content:   []string{"sc-domain:robben.media", "SITE_OWNER"},
		},
		{
			name: "searchconsole_query",
			arguments: map[string]any{
				"account_id": "catalog-account", "site_url": "sc-domain:robben.media",
				"start_date": "2026-09-01", "end_date": "2026-09-07", "dimensions": []string{"date"}, "row_limit": 100,
			},
			endpoints: []catalogRequest{{Method: http.MethodPost, Path: "/webmasters/v3/sites/sc-domain:robben.media/searchAnalytics/query", AccountID: "catalog-account"}},
			content:   []string{"brand", "America/Los_Angeles"},
		},
		{
			name:      "sheets_get_metadata",
			arguments: map[string]any{"account_id": "catalog-account", "spreadsheet_id": "sheet-client-budget"},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/v4/spreadsheets/sheet-client-budget", AccountID: "catalog-account"}},
			content:   []string{"Client Budget", "America/Chicago"},
		},
		{
			name: "sheets_read_range",
			arguments: map[string]any{
				"account_id": "catalog-account", "spreadsheet_id": "sheet-client-budget",
				"range": "'Client Budget'!A1:C4", "major_dimension": "ROWS", "value_render_option": "UNFORMATTED_VALUE",
			},
			endpoints: []catalogRequest{{Method: http.MethodGet, Path: "/v4/spreadsheets/sheet-client-budget/values/'Client Budget'!A1:C4", AccountID: "catalog-account"}},
			content:   []string{"Client Budget", "A1:C4"},
		},
	}
}

func catalogResponses(t *testing.T) map[string]json.RawMessage {
	t.Helper()

	responses := map[string]json.RawMessage{
		"drive_get_file": json.RawMessage(`{"id":"file-acme","name":"Catalog file","mimeType":"application/vnd.google-apps.document"}`),
		"calendar_list": json.RawMessage(`{
			"items": [{"id":"primary","summary":"Catalog calendar","timeZone":"America/Chicago","accessRole":"owner","primary":true}]
		}`),
		"analytics_list_properties": json.RawMessage(`{
			"accountSummaries": [{
				"account": "accounts/123", "displayName": "Catalog account",
				"propertySummaries": [{"property":"properties/123","displayName":"Web","propertyType":"PROPERTY_TYPE_ORDINARY","parent":"accounts/123"}]
			}]
		}`),
		"analytics_metadata": json.RawMessage(`{
			"dimensions": [{"apiName":"date","uiName":"Date","description":"Date","category":"Time"}],
			"metrics": [{"apiName":"sessions","uiName":"Sessions","type":"TYPE_INTEGER"}]
		}`),
		"searchconsole_list_sites": json.RawMessage(`{
			"siteEntry": [{"siteUrl":"sc-domain:robben.media","permissionLevel":"SITE_OWNER"}]
		}`),
		"sheets_get_metadata": json.RawMessage(`{
			"spreadsheetId": "sheet-client-budget",
			"properties": {"title":"Client Budget","locale":"en_US","timeZone":"America/Chicago","autoRecalc":"ON_CHANGE"},
			"sheets": [{"properties":{"sheetId":1,"title":"Client Budget","index":0,"gridProperties":{"rowCount":4,"columnCount":3}}}]
		}`),
	}

	workflowData, err := os.ReadFile("../googleops/testdata/workflow-responses.json")
	if err != nil {
		t.Fatalf("read workflow fixture: %v", err)
	}

	var workflowResponses map[string]json.RawMessage
	if err := json.Unmarshal(workflowData, &workflowResponses); err != nil {
		t.Fatalf("decode workflow fixture: %v", err)
	}

	for key, response := range workflowResponses {
		responses[key] = response
	}

	return responses
}

func TestCatalogToolsAndWorkflowResources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	requests := make([]catalogRequest, 0)
	provider := &catalogProvider{requests: &requests, responses: catalogResponses(t)}
	operations := googleops.Operations(provider)

	allowOperations := []string{"accounts_list"}
	grantOperations := []string{"accounts_list"}
	scopes := make([]string, 0, 7)

	for _, definition := range mcpcontract.Catalog() {
		if definition.Local {
			continue
		}

		allowOperations = append(allowOperations, definition.Name)
		grantOperations = append(grantOperations, definition.Name)
		scopes = append(scopes, definition.Scopes...)
	}

	identity := mcpcontract.Identity{
		AccountID: "catalog-account", Subject: "catalog-subject", Email: "catalog@example.test", Label: "Catalog",
		PrincipalID: "catalog-principal", ClientName: "catalog-client", AuthMode: "oauth", Scopes: scopes, Generation: 1,
	}

	runtime, err := mcpserver.New(mcpserver.Config{
		Principal: mcpcontract.Principal{ID: identity.PrincipalID},
		Grants: []mcpcontract.Grant{{
			PrincipalID: identity.PrincipalID, AccountIDs: []string{identity.AccountID},
			ClientNames: []string{identity.ClientName}, Operations: grantOperations,
		}},
		AllowOperations: allowOperations,
		Operations:      operations,
		Accounts:        access.NewMemoryAccounts(identity),
		Provider:        provider,
		RequestTimeout:  5 * time.Second,
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

	client := mcp.NewClient(&mcp.Implementation{Name: "catalog-test-client", Version: "1"}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	wantTools := []string{
		"accounts_list", "gmail_search", "gmail_get_message", "gmail_get_thread", "drive_search", "drive_get_file",
		"docs_get_text", "calendar_list", "calendar_list_events", "calendar_freebusy", "analytics_list_properties",
		"analytics_metadata", "analytics_report", "searchconsole_list_sites", "searchconsole_query",
		"sheets_get_metadata", "sheets_read_range",
	}
	if len(listed.Tools) != len(wantTools) {
		t.Fatalf("discovered %d tools, want %d: %#v", len(listed.Tools), len(wantTools), listed.Tools)
	}

	discovered := make(map[string]*mcp.Tool, len(listed.Tools))
	for _, tool := range listed.Tools {
		discovered[tool.Name] = tool
	}

	for _, name := range wantTools {
		tool := discovered[name]
		if tool == nil {
			t.Fatalf("tool %s was not discovered", name)
		}

		if tool.Description == "" {
			t.Fatalf("tool %s has no description", name)
		}

		assertObjectSchema(t, name+" input", tool.InputSchema)
		assertObjectSchema(t, name+" output", tool.OutputSchema)
	}

	if len(requests) != 0 || len(provider.identities) != 0 {
		t.Fatalf("discovery reached upstream: requests=%#v identities=%#v", requests, provider.identities)
	}

	catalogResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "accounts_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}

	assertSuccessfulResult(t, "accounts_list", catalogResult)
	var accountOutput struct {
		Accounts []struct {
			AccountID    string   `json:"account_id"`
			Label        string   `json:"label"`
			Email        string   `json:"email"`
			Capabilities []string `json:"capabilities"`
		} `json:"accounts"`
	}
	decodeStructured(t, catalogResult.StructuredContent, &accountOutput)

	if len(accountOutput.Accounts) != 1 || accountOutput.Accounts[0].AccountID != identity.AccountID ||
		accountOutput.Accounts[0].Label != identity.Label || len(accountOutput.Accounts[0].Capabilities) == 0 {
		t.Fatalf("accounts_list output = %#v", accountOutput)
	}

	if len(requests) != 0 || len(provider.identities) != 0 {
		t.Fatalf("accounts_list reached upstream: requests=%#v identities=%#v", requests, provider.identities)
	}

	for _, call := range catalogToolCalls() {
		requestsBefore := len(requests)
		identitiesBefore := len(provider.identities)

		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.arguments})
		if callErr != nil {
			t.Fatalf("%s MCP call: %v", call.name, callErr)
		}

		assertSuccessfulResult(t, call.name, result)

		var envelope struct {
			AccountID       string          `json:"account_id"`
			AccountLabel    string          `json:"account_label"`
			Data            json.RawMessage `json:"data"`
			PartialFailures []struct {
				SourceID string `json:"source_id"`
				Category string `json:"category"`
			} `json:"partial_failures"`
		}
		decodeStructured(t, result.StructuredContent, &envelope)

		if envelope.AccountID != identity.AccountID || envelope.AccountLabel != identity.Label || len(envelope.Data) == 0 {
			t.Fatalf("%s envelope = %#v", call.name, envelope)
		}

		for _, content := range call.content {
			if !strings.Contains(string(envelope.Data), content) {
				t.Fatalf("%s data %s omits %q", call.name, envelope.Data, content)
			}
		}

		if call.name == "calendar_freebusy" && (len(envelope.PartialFailures) != 1 ||
			envelope.PartialFailures[0].SourceID != "unavailable" ||
			envelope.PartialFailures[0].Category != string(mcpcontract.NotFound)) {
			t.Fatalf("calendar_freebusy partial failures = %#v", envelope.PartialFailures)
		}

		newRequests := requests[requestsBefore:]
		if len(newRequests) != len(call.endpoints) || len(provider.identities) != identitiesBefore+1 {
			t.Fatalf("%s upstream calls = %#v provider identities = %#v", call.name, newRequests, provider.identities[identitiesBefore:])
		}

		for index, endpoint := range call.endpoints {
			if newRequests[index] != endpoint {
				t.Fatalf("%s endpoint %d = %#v, want %#v", call.name, index, newRequests[index], endpoint)
			}
		}

		selectedIdentity := provider.identities[identitiesBefore]
		if selectedIdentity.AccountID != identity.AccountID || selectedIdentity.PrincipalID != identity.PrincipalID {
			t.Fatalf("%s selected identity = %#v", call.name, selectedIdentity)
		}

		options := provider.options[identitiesBefore]
		if options.Operation != call.name || options.Retry != mcpcontract.SafeRead {
			t.Fatalf("%s provider options = %#v", call.name, options)
		}
	}

	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	wantResources := []string{"mail", "documents", "calendar", "reporting", "sheets"}
	if len(resources.Resources) != len(wantResources) {
		t.Fatalf("discovered %d workflow resources, want %d", len(resources.Resources), len(wantResources))
	}

	resourceByURI := make(map[string]*mcp.Resource, len(resources.Resources))
	for _, resource := range resources.Resources {
		resourceByURI[resource.URI] = resource
	}

	for _, slug := range wantResources {
		uri := "gog://workflows/v1/" + slug

		resource := resourceByURI[uri]
		if resource == nil || resource.MIMEType != "text/markdown" || resource.Size == 0 || resource.Meta["digest"] == "" {
			t.Fatalf("workflow resource %s = %#v", slug, resource)
		}

		read, readErr := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if readErr != nil {
			t.Fatalf("read workflow %s: %v", slug, readErr)
		}

		if len(read.Contents) != 1 || read.Contents[0].URI != uri ||
			read.Contents[0].MIMEType != "text/markdown" || !strings.Contains(read.Contents[0].Text, "`accounts_list`") {
			t.Fatalf("workflow %s content = %#v", slug, read.Contents)
		}

		if read.Contents[0].Meta["digest"] != resource.Meta["digest"] {
			t.Fatalf("workflow %s read digest does not match discovery digest", slug)
		}
	}
}

func assertObjectSchema(t *testing.T, name string, value any) {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s schema: %v", name, err)
	}

	var schema struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("decode %s schema: %v", name, err)
	}

	if schema.Type != "object" || (len(schema.Properties) == 0 && name != "accounts_list input") {
		t.Fatalf("%s schema = %s", name, encoded)
	}
}

func assertSuccessfulResult(t *testing.T, name string, result *mcp.CallToolResult) {
	t.Helper()

	if result.IsError {
		t.Fatalf("%s returned an error result: %#v", name, result)
	}

	if result.StructuredContent == nil || len(result.Content) == 0 {
		t.Fatalf("%s returned incomplete MCP content: %#v", name, result)
	}
}

func decodeStructured(t *testing.T, value any, target any) {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode structured content: %v", err)
	}

	if err := json.Unmarshal(encoded, target); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
}
