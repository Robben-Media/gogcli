package searchconsole

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

type fixtureTransport struct {
	t        *testing.T
	response string
	requests []*http.Request
	bodies   []string
}

func (transport *fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := ""

	if req.Body != nil {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			transport.t.Fatalf("read request body: %v", err)
		}
		body = string(data)
	}
	transport.requests = append(transport.requests, req)
	transport.bodies = append(transport.bodies, body)

	return &http.Response{
		StatusCode: http.StatusOK, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(transport.response)), ContentLength: int64(len(transport.response)),
	}, nil
}

type fixtureProvider struct {
	transport  *fixtureTransport
	options    []mcpcontract.CallOptions
	identities []mcpcontract.Identity
}

func (provider *fixtureProvider) HTTPClient(_ context.Context, id mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	provider.options = append(provider.options, options)
	provider.identities = append(provider.identities, id)

	return &http.Client{Transport: provider.transport}, nil
}

func fixture(t *testing.T, operation, response string) (mcpcontract.Operation, *fixtureProvider) {
	t.Helper()
	transport := &fixtureTransport{t: t, response: response}

	provider := &fixtureProvider{transport: transport}
	for _, candidate := range Operations(provider) {
		if candidate.Definition.Name == operation {
			return candidate, provider
		}
	}

	t.Fatalf("operation %s was not registered", operation)

	return mcpcontract.Operation{}, nil
}

func decodeRun(t *testing.T, operation mcpcontract.Operation, raw string) (any, error) {
	t.Helper()

	call, err := operation.Decode(json.RawMessage(raw))
	if err != nil {
		return nil, fmt.Errorf("decode operation: %w", err)
	}

	result, err := call.Run(context.Background(), mcpcontract.Identity{
		AccountID: "opaque-account", Subject: "subject", Email: "search@example.test", Label: "Search",
		PrincipalID: "principal", ClientName: "client", AuthMode: "oauth", Generation: 9,
	})
	if err != nil {
		return nil, fmt.Errorf("run operation: %w", err)
	}

	return result, nil
}

func TestSearchConsoleListSitesProjectsPropertyTypes(t *testing.T) {
	t.Parallel()
	operation, provider := fixture(t, "searchconsole_list_sites", `{
		"siteEntry": [
			{"siteUrl": "sc-domain:robben.media", "permissionLevel": "SITE_OWNER"},
			{"siteUrl": "https://www.robben.media/", "permissionLevel": "SITE_FULL_USER"}
		]
	}`)

	result, err := decodeRun(t, operation, `{"account_id":"opaque-account"}`)
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[SitesData])
	if len(out.Data.Sites) != 2 || out.Data.Sites[0].SiteType != "domain" ||
		out.Data.Sites[1].SiteType != "url_prefix" || out.Data.Sites[1].PermissionLevel != "SITE_FULL_USER" {
		t.Fatalf("unexpected projection: %#v", out.Data)
	}

	if got := provider.transport.requests[0].URL.Path; got != "/webmasters/v3/sites" {
		t.Fatalf("sites path = %q", got)
	}

	if provider.options[0] != (mcpcontract.CallOptions{Operation: "searchconsole_list_sites", Retry: mcpcontract.SafeRead}) {
		t.Fatalf("unexpected call options: %#v", provider.options[0])
	}
}

func TestSearchConsoleInputSchemasMatchFrozenRequests(t *testing.T) {
	t.Parallel()
	operations := Operations(&fixtureProvider{})
	properties := make(map[string]map[string]bool, len(operations))

	required := make(map[string][]string, len(operations))
	for _, operation := range operations {
		encoded, err := json.Marshal(operation.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s schema: %v", operation.Definition.Name, err)
		}

		var schema struct {
			Required   []string            `json:"required"`
			Properties map[string]struct{} `json:"properties"`
		}
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", operation.Definition.Name, err)
		}

		properties[operation.Definition.Name] = make(map[string]bool, len(schema.Properties))
		for name := range schema.Properties {
			properties[operation.Definition.Name][name] = true
		}
		required[operation.Definition.Name] = schema.Required
	}

	for name, want := range map[string][]string{
		"searchconsole_list_sites": {"account_id"},
		"searchconsole_query":      {"account_id", "site_url", "start_date", "end_date", "dimensions", "filters", "row_limit", "start_row"},
	} {
		for _, field := range want {
			if !properties[name][field] {
				t.Fatalf("%s schema is missing %s", name, field)
			}
		}
	}

	if got := required["searchconsole_query"]; !reflect.DeepEqual(got, []string{"account_id", "site_url", "start_date", "end_date"}) {
		t.Fatalf("searchconsole_query required fields: %#v", got)
	}
}

func TestSearchConsoleQuerySendsFiltersDatesAndDefinitePagination(t *testing.T) {
	t.Parallel()
	operation, provider := fixture(t, "searchconsole_query", `{
		"responseAggregationType": "BY_PAGE",
		"rows": [
			{"keys": ["shoes", "2026-09-01"], "clicks": 20, "impressions": 100, "ctr": 0.2, "position": 4.5},
			{"keys": ["boots", "2026-09-01"], "clicks": 20, "impressions": 80, "ctr": 0.25, "position": 3.5},
			{"keys": ["sandals", "2026-09-01"], "clicks": 10, "impressions": 70, "ctr": 0.14, "position": 5.5}
		]
	}`)

	result, err := decodeRun(t, operation, `{
		"account_id":"opaque-account", "site_url":"https://www.robben.media/marketing/",
		"start_date":"2026-09-01", "end_date":"2026-09-07", "dimensions":["query","date"],
		"filters":[{"dimension":"query","operator":"not_contains","expression":"internal"}],
		"row_limit":2, "start_row":4
	}`)
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[QueryData])
	if out.Data.TimeZone != "America/Los_Angeles" || len(out.Data.Rows) != 2 ||
		out.NextPageToken != "6" || !out.Truncated || out.Data.ResponseAggregationType != "BY_PAGE" {
		t.Fatalf("unexpected result: %#v", out)
	}

	req := provider.transport.requests[0]
	if req.Method != http.MethodPost || !strings.Contains(req.URL.EscapedPath(), "/webmasters/v3/sites/https%3A%2F%2Fwww.robben.media%2Fmarketing%2F/searchAnalytics/query") {
		t.Fatalf("query request %s %s", req.Method, req.URL.EscapedPath())
	}

	var body struct {
		StartDate             string   `json:"startDate"`
		EndDate               string   `json:"endDate"`
		Dimensions            []string `json:"dimensions"`
		RowLimit              int64    `json:"rowLimit"`
		StartRow              int64    `json:"startRow"`
		DimensionFilterGroups []struct {
			GroupType string `json:"groupType"`
			Filters   []struct {
				Dimension  string `json:"dimension"`
				Operator   string `json:"operator"`
				Expression string `json:"expression"`
			} `json:"filters"`
		} `json:"dimensionFilterGroups"`
	}
	if err := json.Unmarshal([]byte(provider.transport.bodies[0]), &body); err != nil {
		t.Fatal(err)
	}

	if body.StartDate != "2026-09-01" || body.EndDate != "2026-09-07" ||
		!reflect.DeepEqual(body.Dimensions, []string{"QUERY", "DATE"}) || body.RowLimit != 3 || body.StartRow != 4 {
		t.Fatalf("unexpected query body: %#v", body)
	}

	group := body.DimensionFilterGroups[0]
	if group.GroupType != "AND" || group.Filters[0].Dimension != "QUERY" || group.Filters[0].Operator != "NOT_CONTAINS" ||
		group.Filters[0].Expression != "internal" {
		t.Fatalf("unexpected filter body: %#v", group)
	}

	if provider.options[0] != (mcpcontract.CallOptions{Operation: "searchconsole_query", Retry: mcpcontract.SafeRead}) {
		t.Fatalf("unexpected call options: %#v", provider.options[0])
	}
}

func TestSearchConsoleExactPageHasNoDefiniteTruncation(t *testing.T) {
	t.Parallel()
	operation, provider := fixture(t, "searchconsole_query", `{
		"rows": [
			{"keys": ["shoes"], "clicks": 20, "impressions": 100, "ctr": 0.2, "position": 4.5},
			{"keys": ["boots"], "clicks": 20, "impressions": 80, "ctr": 0.25, "position": 3.5}
		]
	}`)

	result, err := decodeRun(t, operation, `{
		"account_id":"opaque-account", "site_url":"sc-domain:robben.media",
		"start_date":"2026-09-01", "end_date":"2026-09-07", "dimensions":["query"], "row_limit":2
	}`)
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[QueryData])
	if len(out.Data.Rows) != 2 || out.NextPageToken != "" || out.Truncated {
		t.Fatalf("exact page was reported as truncated: %#v", out)
	}

	var body struct {
		RowLimit int64 `json:"rowLimit"`
	}
	if err := json.Unmarshal([]byte(provider.transport.bodies[0]), &body); err != nil {
		t.Fatal(err)
	}

	if body.RowLimit != 3 {
		t.Fatalf("rowLimit = %d, want 3", body.RowLimit)
	}
}

func TestSearchConsoleOpaqueURLPrefixAndSearchAppearanceRoundTrip(t *testing.T) {
	t.Parallel()
	const siteURL = "https://example.test:8443//marketing//path;query=x/a"
	operation, provider := fixture(t, "searchconsole_query", `{
		"rows": [{"keys": ["rich result"], "clicks": 5, "impressions": 50, "ctr": 0.1, "position": 2}]
	}`)

	result, err := decodeRun(t, operation, fmt.Sprintf(`{
		"account_id":"opaque-account", "site_url":%q,
		"start_date":"2026-09-01", "end_date":"2026-09-07", "dimensions":["search_appearance"],
		"filters":[{"dimension":"search_appearance","operator":"contains","expression":"rich"}], "row_limit":1
	}`, siteURL))
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[QueryData])
	if out.Data.SiteURL != siteURL || out.Data.SiteType != "url_prefix" || len(out.Data.Rows) != 1 ||
		out.Data.Rows[0].Keys[0] != "rich result" || out.NextPageToken != "" || out.Truncated {
		t.Fatalf("opaque property was not preserved: %#v", out)
	}

	req := provider.transport.requests[0]

	wantPath := "/webmasters/v3/sites/" + strings.NewReplacer(
		"/", "%2F", ":", "%3A", ";", "%3B", "=", "%3D",
	).Replace(siteURL) + "/searchAnalytics/query"
	if req.URL.EscapedPath() != wantPath {
		t.Fatalf("encoded property path = %q, want %q", req.URL.EscapedPath(), wantPath)
	}

	var body struct {
		Dimensions            []string `json:"dimensions"`
		DimensionFilterGroups []struct {
			Filters []struct {
				Dimension string `json:"dimension"`
			} `json:"filters"`
		} `json:"dimensionFilterGroups"`
		RowLimit int64 `json:"rowLimit"`
	}
	if err := json.Unmarshal([]byte(provider.transport.bodies[0]), &body); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(body.Dimensions, []string{"SEARCH_APPEARANCE"}) ||
		body.DimensionFilterGroups[0].Filters[0].Dimension != "SEARCH_APPEARANCE" || body.RowLimit != 2 {
		t.Fatalf("unexpected request body: %#v", body)
	}
}

func TestSearchConsoleEmptyRowsAreSuccessful(t *testing.T) {
	t.Parallel()
	operation, _ := fixture(t, "searchconsole_query", `{}`)

	result, err := decodeRun(t, operation, `{
		"account_id":"opaque-account", "site_url":"sc-domain:robben.media",
		"start_date":"2026-09-01", "end_date":"2026-09-07"
	}`)
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[QueryData])
	if out.Data.Rows == nil || len(out.Data.Rows) != 0 || out.NextPageToken != "" || len(out.PartialFailures) != 0 {
		t.Fatalf("empty success was not represented distinctly: %#v", out)
	}
}

func TestSearchConsoleValidationHappensBeforeProviderAcquisition(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"account_id":"a","site_url":"ftp://example.com/","start_date":"2026-09-01","end_date":"2026-09-07"}`,
		`{"account_id":"a","site_url":"sc-domain:","start_date":"2026-09-01","end_date":"2026-09-07"}`,
		`{"account_id":"a","site_url":"https://","start_date":"2026-09-01","end_date":"2026-09-07"}`,
		`{"account_id":"a","site_url":"https://example.com/","start_date":"2026-09-07","end_date":"2026-09-01"}`,
		`{"account_id":"a","site_url":"https://example.com/","start_date":"2026-09-01","end_date":"2026-09-07","dimensions":["invalid"]}`,
		`{"account_id":"a","site_url":"https://example.com/","start_date":"2026-09-01","end_date":"2026-09-07","filters":[{"dimension":"date","operator":"equals","expression":"2026-09-01"}]}`,
		`{"account_id":"a","site_url":"https://example.com/","start_date":"2026-09-01","end_date":"2026-09-07","filters":[{"dimension":"query","operator":"regex","expression":"x"}]}`,
		`{"account_id":"a","site_url":"https://example.com/","start_date":"2026-09-01","end_date":"2026-09-07","row_limit":-1}`,
		`{"account_id":"a","site_url":"https://example.com/","start_date":"2026-09-01","end_date":"2026-09-07","start_row":9223372036854775807}`,
		`{"account_id":"a","site_url":"https://example.com/","start_date":"2026-09-01","end_date":"2026-09-07","row_limit":1001}`,
	} {
		operation, provider := fixture(t, "searchconsole_query", `{}`)
		if _, err := decodeRun(t, operation, raw); err == nil {
			t.Fatalf("accepted invalid arguments %s", raw)
		} else if len(provider.options) != 0 || len(provider.transport.requests) != 0 {
			t.Fatalf("invalid arguments %s reached provider or upstream", raw)
		}
	}
}
