package analytics

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
		AccountID: "opaque-account", Subject: "subject", Email: "analytics@example.test", Label: "Marketing",
		PrincipalID: "principal", ClientName: "client", AuthMode: "oauth", Generation: 3,
	})
	if err != nil {
		return nil, fmt.Errorf("run operation: %w", err)
	}

	return result, nil
}

func TestAnalyticsListPropertiesFlattensAccountSummaries(t *testing.T) {
	t.Parallel()
	operation, provider := fixture(t, "analytics_list_properties", `{
		"nextPageToken": "properties-next",
		"accountSummaries": [{
			"account": "accounts/100", "displayName": "Robben",
			"propertySummaries": [
				{"property": "properties/200", "displayName": "Web", "propertyType": "PROPERTY_TYPE_ORDINARY", "parent": "accounts/100"},
				{"property": "properties/201", "displayName": "App", "propertyType": "PROPERTY_TYPE_ORDINARY", "parent": "accounts/100"}
			]
		}]
	}`)

	result, err := decodeRun(t, operation, `{"account_id":"opaque-account"}`)
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[PropertiesData])
	if len(out.Data.Properties) != 2 || out.Data.Properties[0].ResourceName != "properties/200" ||
		out.Data.Properties[1].AccountResourceName != "accounts/100" || out.NextPageToken != "properties-next" {
		t.Fatalf("unexpected result: %#v", out)
	}

	if got := provider.transport.requests[0].URL.Query().Get("pageSize"); got != "50" {
		t.Fatalf("pageSize = %q", got)
	}

	if provider.options[0] != (mcpcontract.CallOptions{Operation: "analytics_list_properties", Retry: mcpcontract.SafeRead}) {
		t.Fatalf("unexpected call options: %#v", provider.options[0])
	}
}

func TestAnalyticsInputSchemasMatchFrozenRequests(t *testing.T) {
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
			Required   []string `json:"required"`
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
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
		"analytics_list_properties": {"account_id", "page_size", "page_token"},
		"analytics_metadata":        {"account_id", "property", "kind"},
		"analytics_report":          {"account_id", "property", "metrics", "dimensions", "start_date", "end_date", "limit", "offset"},
	} {
		for _, field := range want {
			if !properties[name][field] {
				t.Fatalf("%s schema is missing %s", name, field)
			}
		}
	}

	if got := required["analytics_report"]; !reflect.DeepEqual(got, []string{"account_id", "property", "metrics", "start_date", "end_date"}) {
		t.Fatalf("analytics_report required fields: %#v", got)
	}

	if pageSize := operations[0].InputSchema.Properties["page_size"]; pageSize == nil ||
		!strings.Contains(pageSize.Description, "account summaries per page") ||
		!strings.Contains(pageSize.Description, "paging remains account-summary based") {
		t.Fatalf("page_size description does not describe account-summary paging: %#v", pageSize)
	}

	report := operations[2].InputSchema
	if report == nil || report.Properties["start_date"] == nil || report.Properties["end_date"] == nil {
		t.Fatal("analytics_report schema is missing explicit date fields")
	}

	for field, description := range map[string]string{
		"start_date": report.Properties["start_date"].Description,
		"end_date":   report.Properties["end_date"].Description,
	} {
		normalized := strings.ToLower(description)
		if !strings.Contains(normalized, "inclusive") || !strings.Contains(normalized, "yyyy-mm-dd") ||
			!strings.Contains(normalized, "relative date expressions are not supported") {
			t.Fatalf("%s description does not describe explicit dates: %q", field, description)
		}
	}
}

func TestAnalyticsMetadataProjectsOnlyAuthorizedKind(t *testing.T) {
	t.Parallel()
	operation, provider := fixture(t, "analytics_metadata", `{
		"name": "properties/200/metadata",
		"dimensions": [{"apiName": "date", "uiName": "Date", "description": "Date", "category": "Time", "deprecatedApiNames": ["ga_date"]}],
		"metrics": [{"apiName": "sessions", "uiName": "Sessions", "type": "TYPE_INTEGER", "blockedReasons": ["NO_REVENUE_METRICS"]}]
	}`)
	const raw = `{"account_id":"opaque-account","property":"200","kind":"dimensions"}`

	call, err := operation.Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(call.Actions, []string{"analytics:dimensions"}) {
		t.Fatalf("metadata actions = %#v", call.Actions)
	}

	result, err := decodeRun(t, operation, raw)
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[MetadataData])
	if out.Data.Kind != "dimensions" || len(out.Data.Dimensions) != 1 || out.Data.Metrics != nil {
		t.Fatalf("unauthorized projection was returned: %#v", out.Data)
	}

	if out.Data.Dimensions[0].APIName != "date" || len(out.Data.Dimensions[0].DeprecatedAPINames) != 1 {
		t.Fatalf("metadata was not preserved: %#v", out.Data.Dimensions[0])
	}

	if got := provider.transport.requests[0].URL.Path; got != "/v1beta/properties/200/metadata" {
		t.Fatalf("metadata path = %q", got)
	}
}

func TestAnalyticsReportPreservesTotalsTimezoneAndOffsetCursor(t *testing.T) {
	t.Parallel()
	operation, provider := fixture(t, "analytics_report", `{
		"dimensionHeaders": [{"name": "date"}],
		"metricHeaders": [{"name": "sessions", "type": "TYPE_INTEGER"}],
		"metadata": {"timeZone": "America/Chicago", "currencyCode": "USD", "subjectToThresholding": true},
		"rowCount": 150,
		"rows": [{"dimensionValues": [{"value": "20260901"}], "metricValues": [{"value": "120"}]}],
		"totals": [{"dimensionValues": [{"value": "RESERVED_TOTAL"}], "metricValues": [{"value": "270"}]}]
	}`)

	result, err := decodeRun(t, operation, `{
		"account_id":"opaque-account", "property":"properties/200", "metrics":["sessions"],
		"dimensions":["date"], "start_date":"2026-09-01", "end_date":"2026-09-07"
	}`)
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[ReportData])
	if out.Data.TimeZone != "America/Chicago" || out.Data.CurrencyCode != "USD" || out.Data.RowCount != 150 ||
		!out.Data.SubjectToThreshold || len(out.Data.Totals) != 1 || out.Data.Totals[0].Metrics[0] != "270" {
		t.Fatalf("report details were not preserved: %#v", out.Data)
	}

	if out.NextPageToken != "100" || !out.Truncated {
		t.Fatalf("offset cursor = %q truncated=%v", out.NextPageToken, out.Truncated)
	}

	req := provider.transport.requests[0]
	if req.Method != http.MethodPost || req.URL.Path != "/v1beta/properties/200:runReport" {
		t.Fatalf("report request %s %s", req.Method, req.URL.Path)
	}

	var body struct {
		Dimensions         []map[string]string `json:"dimensions"`
		Metrics            []map[string]string `json:"metrics"`
		DateRanges         []map[string]string `json:"dateRanges"`
		Limit              string              `json:"limit"`
		Offset             string              `json:"offset"`
		MetricAggregations []string            `json:"metricAggregations"`
	}
	if err := json.Unmarshal([]byte(provider.transport.bodies[0]), &body); err != nil {
		t.Fatal(err)
	}

	if len(body.Dimensions) != 1 || body.Dimensions[0]["name"] != "date" || len(body.Metrics) != 1 ||
		body.Metrics[0]["name"] != "sessions" || body.DateRanges[0]["startDate"] != "2026-09-01" ||
		body.DateRanges[0]["endDate"] != "2026-09-07" || body.Limit != "100" || body.Offset != "" ||
		!reflect.DeepEqual(body.MetricAggregations, []string{"TOTAL"}) {
		t.Fatalf("unexpected report body: %#v", body)
	}

	if provider.options[0] != (mcpcontract.CallOptions{Operation: "analytics_report", Retry: mcpcontract.SafeRead}) {
		t.Fatalf("unexpected call options: %#v", provider.options[0])
	}
}

func TestAnalyticsValidationHappensBeforeProviderAcquisition(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"account_id":"a"}`,
		`{"account_id":"a","property":"../200","kind":"dimensions"}`,
		`{"account_id":"a","property":"200","kind":"both","metrics":["sessions"],"dimensions":["date"],"start_date":"today","end_date":"2026-09-07"}`,
		`{"account_id":"a","property":"200","kind":"both","metrics":["sessions"],"dimensions":["date"],"start_date":"2026-09-07","end_date":"2026-09-01"}`,
		`{"account_id":"a","property":"200","kind":"both","metrics":["bad name"],"dimensions":["date"],"start_date":"2026-09-01","end_date":"2026-09-07"}`,
		`{"account_id":"a","property":"200","kind":"both","metrics":["sessions"],"dimensions":["date"],"start_date":"2026-09-01","end_date":"2026-09-07","limit":1001}`,
		`{"account_id":"a","property":"200","kind":"both","metrics":["sessions"],"dimensions":["date"],"start_date":"2026-09-01","end_date":"2026-09-07","offset":-1}`,
	} {
		operation, provider := fixture(t, "analytics_report", `{}`)
		if _, err := decodeRun(t, operation, raw); err == nil {
			t.Fatalf("accepted invalid arguments %s", raw)
		} else if len(provider.options) != 0 || len(provider.transport.requests) != 0 {
			t.Fatalf("invalid arguments %s reached provider or upstream", raw)
		}
	}
}
