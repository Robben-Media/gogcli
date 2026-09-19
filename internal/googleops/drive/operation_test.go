package drive

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type fakeProvider struct {
	identity mcpcontract.Identity
	options  []mcpcontract.CallOptions
	client   *http.Client
}

func (p *fakeProvider) HTTPClient(ctx context.Context, identity mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	p.options = append(p.options, options)
	return p.client, nil
}

func jsonResponse(value string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(value))}
}

func TestSearchDefaultsSharedDrivesAndProjection(t *testing.T) {
	var request *http.Request
	provider := &fakeProvider{
		identity: mcpcontract.Identity{AccountID: "opaque-a", Label: "Work"},
		client: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			request = req

			return jsonResponse(`{
				"nextPageToken": "page-2",
				"files": [{
					"id": "file-1", "name": "Client brief", "mimeType": "application/vnd.google-apps.document",
					"size": "123", "modifiedTime": "2026-09-17T10:00:00Z", "parents": ["parent-1"]
				}]
			}`), nil
		})},
	}
	ops := Operations(provider)

	call, err := ops[0].Decode(json.RawMessage(`{"account_id":"opaque-a","text":"client's brief","drive_id":"shared-1"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), provider.identity)
	if err != nil {
		t.Fatal(err)
	}

	result, ok := resultAny.(mcpcontract.Result[SearchData])
	if !ok {
		t.Fatalf("got result type %T", resultAny)
	}

	if result.AccountID != "opaque-a" || result.AccountLabel != "Work" || result.NextPageToken != "page-2" {
		t.Fatalf("unexpected envelope: %#v", result)
	}

	if len(provider.options) != 1 || provider.options[0].Operation != "drive_search" || provider.options[0].Retry != mcpcontract.SafeRead {
		t.Fatalf("unexpected provider options: %#v", provider.options)
	}

	if request.URL.Path != "/drive/v3/files" {
		t.Fatalf("unexpected path %s", request.URL.Path)
	}

	query := request.URL.Query()
	if query.Get("q") != `fullText contains 'client\'s brief' and trashed = false` {
		t.Fatalf("unexpected query %q", query.Get("q"))
	}

	for name, expected := range map[string]string{
		"pageSize": "25", "corpora": "allDrives", "driveId": "shared-1",
		"includeItemsFromAllDrives": "true", "supportsAllDrives": "true",
	} {
		if query.Get(name) != expected {
			t.Fatalf("%s = %q, want %q", name, query.Get(name), expected)
		}
	}

	if len(result.Data.Files) != 1 {
		t.Fatalf("unexpected files %#v", result.Data.Files)
	}

	file := result.Data.Files[0]
	if file.ID != "file-1" || file.Name != "Client brief" || file.Size == nil || *file.Size != 123 {
		t.Fatalf("unexpected file %#v", file)
	}

	if file.WebViewLink != "https://drive.google.com/file/d/file-1/view" {
		t.Fatalf("unexpected URL %q", file.WebViewLink)
	}
}

func TestSearchValidationHappensBeforeProvider(t *testing.T) {
	provider := &fakeProvider{}

	ops := Operations(provider)
	for _, raw := range []string{
		`{"account_id":"a","text":"x","max_results":101}`,
		`{"account_id":"a","text":"x","max_results":-1}`,
		`{"account_id":"a","text":"  "}`,
	} {
		if _, err := ops[0].Decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}

	if len(provider.options) != 0 {
		t.Fatalf("validation acquired a client: %#v", provider.options)
	}
}

func TestGetFileProjectionAndErrorMapping(t *testing.T) {
	provider := &fakeProvider{
		identity: mcpcontract.Identity{AccountID: "opaque-a", Label: "Work"},
		client: &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Path != "/drive/v3/files/file-1" {
				t.Fatalf("unexpected path %s", request.URL.Path)
			}

			if request.URL.Query().Get("supportsAllDrives") != "true" {
				t.Fatalf("missing shared-drive support")
			}

			return jsonResponse(`{"id":"file-1","name":"Brief","description":"Latest","starred":true,"webViewLink":"https://example.invalid/file"}`), nil
		})},
	}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{"account_id":"opaque-a","file_id":"file-1"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), provider.identity)
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[GetFileData])
	if result.Data.File.Description != "Latest" || !result.Data.File.Starred || result.Data.File.WebViewLink != "https://example.invalid/file" {
		t.Fatalf("unexpected projection %#v", result.Data.File)
	}

	provider.client = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(`{"error":"denied"}`))}, nil
	})}

	call, err = Operations(provider)[1].Decode(json.RawMessage(`{"account_id":"opaque-a","file_id":"file-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), provider.identity)

	var public *mcpcontract.Error
	if !errors.As(err, &public) || public.Category != mcpcontract.Forbidden || strings.Contains(public.Error(), "denied") {
		t.Fatalf("unexpected error %#v", err)
	}
}

func TestInputSchemas(t *testing.T) {
	t.Parallel()

	ops := Operations(&fakeProvider{})

	tests := []struct {
		index      int
		properties []string
		required   []string
	}{
		{0, []string{"account_id", "text", "max_results", "page_token", "drive_id", "include_shared_drives"}, []string{"account_id", "text"}},
		{1, []string{"account_id", "file_id"}, []string{"account_id", "file_id"}},
	}
	for _, test := range tests {
		raw, err := json.Marshal(ops[test.index].InputSchema)
		if err != nil {
			t.Fatal(err)
		}

		var schema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}

		if len(schema.Properties) != len(test.properties) {
			t.Fatalf("operation %d properties: %#v", test.index, schema.Properties)
		}

		for _, name := range test.properties {
			if _, ok := schema.Properties[name]; !ok {
				t.Fatalf("operation %d missing property %s", test.index, name)
			}
		}

		sort.Strings(schema.Required)

		if strings.Join(schema.Required, ",") != strings.Join(test.required, ",") {
			t.Fatalf("operation %d required: %#v", test.index, schema.Required)
		}
	}
}
