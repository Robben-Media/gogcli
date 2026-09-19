package docs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestGetTextDefaultsToFirstTabAndWalksNestedContent(t *testing.T) {
	var request *http.Request
	provider := &fakeProvider{
		identity: mcpcontract.Identity{AccountID: "opaque-a", Label: "Work"},
		client: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			request = req

			return jsonResponse(`{
				"documentId": "doc-1", "title": "Client brief", "revisionId": "rev-7",
				"body": {"content": [{"paragraph": {"elements": [{"textRun": {"content": "Primary tab"}}]}}]},
				"tabs": [{
					"tabProperties": {"tabId": "tab-1", "title": "Q3", "index": 1},
					"documentTab": {"body": {"content": [
						{"paragraph": {"elements": [{"textRun": {"content": "Alpha\n"}}]}},
						{"table": {"tableRows": [
							{"tableCells": [
								{"content": [
									{"paragraph": {"elements": [{"textRun": {"content": "One\n"}}]}},
									{"table": {"tableRows": [
											{"tableCells": [{"content": [{"paragraph": {"elements": [{"textRun": {"content": "Two\n"}}]}}]},
												{"content": [{"paragraph": {"elements": [{"textRun": {"content": "Three\n"}}]}}]}]}
										]}}
									]}
							]},
							{"tableCells": [
								{"content": [{"paragraph": {"elements": [{"textRun": {"content": "Four\n"}}]}}]}
							]},
							{"tableCells": [
								{"content": [{"tableOfContents": {"content": [
									{"paragraph": {"elements": [{"textRun": {"content": "Contents\n"}}]}}
								]}}]}
							]}
						]}}
					]}}
				}]
			}`), nil
		})},
	}

	call, err := Operations(provider)[0].Decode(json.RawMessage(`{"account_id":"opaque-a","document_id":"doc-1"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), provider.identity)
	if err != nil {
		t.Fatal(err)
	}
	result := resultAny.(mcpcontract.Result[GetTextData])

	data := result.Data
	if data.Text != "Alpha\nOne\nTwo\nThree\nFour\nContents\n" {
		t.Fatalf("unexpected text %q", data.Text)
	}

	if data.TabID != "tab-1" || data.ParagraphCount != 6 || data.TableCount != 2 || data.MaxBytes != defaultMaxBytes || result.Truncated {
		t.Fatalf("unexpected extraction %#v", data)
	}

	if data.URL != "https://docs.google.com/document/d/doc-1/edit?tab=tab-1" {
		t.Fatalf("unexpected URL %q", data.URL)
	}

	if len(data.Tabs) != 1 || data.Tabs[0].ID != "tab-1" || data.Tabs[0].Title != "Q3" {
		t.Fatalf("unexpected tabs %#v", data.Tabs)
	}

	if request.URL.Path != "/v1/documents/doc-1" || request.URL.Query().Get("includeTabsContent") != "true" {
		t.Fatalf("unexpected request %s", request.URL.String())
	}

	fields := request.URL.Query().Get("fields")
	if strings.Contains(fields, ",body,") || !strings.Contains(fields, "paragraph.elements.textRun.content") || !strings.Contains(fields, "tableOfContents") {
		t.Fatalf("unexpected field projection %q", fields)
	}

	if len(provider.options) != 1 || provider.options[0].Operation != "docs_get_text" || provider.options[0].Retry != mcpcontract.SafeRead {
		t.Fatalf("unexpected provider options %#v", provider.options)
	}
}

func TestGetTextTruncatesOnRuneBoundaryBeforeProvider(t *testing.T) {
	provider := &fakeProvider{client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"documentId":"doc-1","tabs":[{"tabProperties":{"tabId":"tab-1"},"documentTab":{"body":{"content":[{"paragraph":{"elements":[{"textRun":{"content":"αβγ\n"}}]}}]}}}]}`), nil
	})}}

	call, err := Operations(provider)[0].Decode(json.RawMessage(`{"account_id":"opaque-a","document_id":"doc-1","max_bytes":3}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), provider.identity)
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[GetTextData])
	if result.Data.Text != "α" || !result.Truncated || !utf8ValidString(result.Data.Text) {
		t.Fatalf("unexpected truncation %#v", result.Data)
	}

	if len(provider.options) != 1 {
		t.Fatal("valid input should acquire the provider")
	}

	provider.options = nil
	for _, raw := range []string{
		`{"account_id":"a","document_id":"doc-1","max_bytes":1048577}`,
		`{"account_id":"a","document_id":"doc-1","max_bytes":-1}`,
		`{"account_id":"a"}`,
	} {
		if _, err := Operations(provider)[0].Decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}

	if len(provider.options) != 0 {
		t.Fatalf("invalid input acquired a provider: %#v", provider.options)
	}
}

func TestGetTextMapsTypedGoogleErrors(t *testing.T) {
	tests := []struct {
		reason   string
		category mcpcontract.ErrorCategory
	}{
		{"insufficientAuthenticationScopes", mcpcontract.InsufficientScope},
		{"insufficientPermissions", mcpcontract.InsufficientScope},
		{"quotaExceeded", mcpcontract.QuotaExhausted},
	}

	for _, test := range tests {
		provider := &fakeProvider{
			identity: mcpcontract.Identity{AccountID: "opaque-a"},
			client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				body := `{"error":{"code":403,"message":"denied","errors":[{"reason":"` + test.reason + `"}]}}`
				return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})},
		}

		call, err := Operations(provider)[0].Decode(json.RawMessage(`{"account_id":"opaque-a","document_id":"doc-1"}`))
		if err != nil {
			t.Fatal(err)
		}
		_, err = call.Run(context.Background(), provider.identity)

		var public *mcpcontract.Error
		if !errors.As(err, &public) || public.Category != test.category || strings.Contains(public.Error(), "denied") {
			t.Fatalf("%s: unexpected error %#v", test.reason, err)
		}
	}
}

func TestLimitedBodyStopsAtByteBoundary(t *testing.T) {
	body := &limitedBody{ReadCloser: io.NopCloser(strings.NewReader("abcdef")), remaining: 3}
	buffer := make([]byte, 8)

	n, err := body.Read(buffer)
	if n != 3 || err != nil || string(buffer[:n]) != "abc" {
		t.Fatalf("first read %d %v %q", n, err, buffer[:n])
	}

	n, err = body.Read(buffer)
	if n != 0 || err == nil || !strings.Contains(err.Error(), "bounded read limit") {
		t.Fatalf("second read %d %v", n, err)
	}

	exact := &limitedBody{ReadCloser: io.NopCloser(strings.NewReader("abcd")), remaining: 4}
	n, err = exact.Read(make([]byte, 8))

	nextN, nextErr := exact.Read(make([]byte, 1))
	if n != 4 || err != nil || nextN != 0 || !errors.Is(nextErr, io.EOF) {
		t.Fatalf("exact response was rejected: %d %v", n, err)
	}
}

func TestGetTextMapsNotFoundSafely(t *testing.T) {
	provider := &fakeProvider{
		identity: mcpcontract.Identity{AccountID: "opaque-a"},
		client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{"error":"private id"}`))}, nil
		})},
	}

	call, err := Operations(provider)[0].Decode(json.RawMessage(`{"account_id":"opaque-a","document_id":"doc-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), provider.identity)

	var public *mcpcontract.Error
	if !errors.As(err, &public) || public.Category != mcpcontract.NotFound || strings.Contains(public.Error(), "private id") {
		t.Fatalf("unexpected error %#v", err)
	}
}

func TestInputSchema(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(Operations(&fakeProvider{})[0].InputSchema)
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

	expectedProperties := []string{"account_id", "document_id", "tab_id", "max_bytes"}
	if len(schema.Properties) != len(expectedProperties) {
		t.Fatalf("properties: %#v", schema.Properties)
	}

	for _, name := range expectedProperties {
		if _, ok := schema.Properties[name]; !ok {
			t.Fatalf("missing property %s", name)
		}
	}

	sort.Strings(schema.Required)

	if strings.Join(schema.Required, ",") != "account_id,document_id" {
		t.Fatalf("required: %#v", schema.Required)
	}
}

func utf8ValidString(value string) bool { return utf8.ValidString(value) }
