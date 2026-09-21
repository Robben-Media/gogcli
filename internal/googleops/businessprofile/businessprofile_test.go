package businessprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

type fixtureTransport struct {
	target *url.URL
}

// RoundTrip keeps the generated client's path and query but redirects the
// request onto the local httptest server, so tests never touch live Google.
func (transport *fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = transport.target.Scheme
	req.URL.Host = transport.target.Host

	response, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("fixture round trip: %w", err)
	}

	return response, nil
}

type recordingProvider struct {
	transport  *fixtureTransport
	options    []mcpcontract.CallOptions
	identities []mcpcontract.Identity
}

func (provider *recordingProvider) HTTPClient(_ context.Context, id mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	provider.options = append(provider.options, options)
	provider.identities = append(provider.identities, id)

	return &http.Client{Transport: provider.transport}, nil
}

type recordingServer struct {
	status   int
	response string
	path     string
	query    url.Values
	calls    int
}

func (recorder *recordingServer) handle(w http.ResponseWriter, r *http.Request) {
	recorder.calls++
	recorder.path = r.URL.EscapedPath()
	recorder.query = r.URL.Query()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(recorder.status)
	_, _ = io.WriteString(w, recorder.response)
}

func fixture(t *testing.T, operation string, status int, response string) (mcpcontract.Operation, *recordingProvider, *recordingServer) {
	t.Helper()

	recorder := &recordingServer{status: status, response: response}
	server := httptest.NewServer(http.HandlerFunc(recorder.handle))
	t.Cleanup(server.Close)

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse fixture endpoint: %v", err)
	}

	provider := &recordingProvider{transport: &fixtureTransport{target: target}}
	for _, candidate := range Operations(provider) {
		if candidate.Definition.Name == operation {
			return candidate, provider, recorder
		}
	}

	t.Fatalf("operation %s was not registered", operation)

	return mcpcontract.Operation{}, nil, nil
}

func decodeRun(t *testing.T, operation mcpcontract.Operation, raw string, id mcpcontract.Identity) (any, error) {
	t.Helper()

	call, err := operation.Decode(json.RawMessage(raw))
	if err != nil {
		return nil, fmt.Errorf("decode arguments: %w", err)
	}

	result, err := call.Run(context.Background(), id)
	if err != nil {
		return nil, fmt.Errorf("run operation: %w", err)
	}

	return result, nil
}

func testIdentity(accountID string) mcpcontract.Identity {
	return mcpcontract.Identity{
		AccountID: accountID, Subject: "subject", Email: "gbp@example.test", Label: "GBP " + accountID,
		PrincipalID: "principal", ClientName: "client", AuthMode: "oauth", Generation: 9,
		Scopes: []string{"https://www.googleapis.com/auth/business.manage"},
	}
}

func TestBusinessProfileListAccountsProjectsOneBoundedPage(t *testing.T) {
	t.Parallel()
	operation, provider, recorder := fixture(t, "businessprofile_list_accounts", http.StatusOK, `{
		"accounts": [
			{"name": "accounts/1001", "accountName": "Robben Media", "type": "PERSONAL", "role": "PRIMARY_OWNER", "permissionLevel": "OWNER_LEVEL"},
			{"name": "accounts/1002", "accountName": "Williams Appraisal", "type": "LOCATION_GROUP", "role": "OWNER"},
			null
		],
		"nextPageToken": "acct-tok-9"
	}`)

	result, err := decodeRun(t, operation, `{"account_id":"opaque-account"}`, testIdentity("opaque-account"))
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[AccountsData])
	if len(out.Data.Accounts) != 2 {
		t.Fatalf("unexpected account projection: %#v", out.Data)
	}

	first, second := out.Data.Accounts[0], out.Data.Accounts[1]
	if first.Name != "accounts/1001" || first.AccountName != "Robben Media" || first.Type != "PERSONAL" || first.Role != "PRIMARY_OWNER" {
		t.Fatalf("unexpected first account: %#v", first)
	}

	if second.Name != "accounts/1002" || second.AccountName != "Williams Appraisal" || second.Type != "LOCATION_GROUP" || second.Role != "OWNER" {
		t.Fatalf("unexpected second account: %#v", second)
	}

	if recorder.path != "/v1/accounts" {
		t.Fatalf("accounts path = %q", recorder.path)
	}

	if got := recorder.query.Get("pageSize"); got != "20" {
		t.Fatalf("pageSize = %q, want the documented maximum 20", got)
	}

	if _, exists := recorder.query["pageToken"]; exists && recorder.query.Get("pageToken") != "" {
		t.Fatalf("unexpected pageToken for first page: %v", recorder.query)
	}

	if out.NextPageToken != "acct-tok-9" {
		t.Fatalf("next page token = %q", out.NextPageToken)
	}

	if out.Truncated {
		t.Fatal("a bounded page must never claim truncation")
	}

	if out.AccountID != "opaque-account" || out.AccountLabel != "GBP opaque-account" {
		t.Fatalf("unexpected envelope identity: %q %q", out.AccountID, out.AccountLabel)
	}

	want := mcpcontract.CallOptions{Operation: "businessprofile_list_accounts", Retry: mcpcontract.SafeRead}
	if provider.options[0] != want {
		t.Fatalf("unexpected call options: %#v", provider.options[0])
	}
}

func TestBusinessProfileListAccountsKeepsIdentitiesSeparate(t *testing.T) {
	t.Parallel()
	operation, provider, recorder := fixture(t, "businessprofile_list_accounts", http.StatusOK, `{"accounts": [], "nextPageToken": "shared-tok"}`)

	call, err := operation.Decode(json.RawMessage(`{"account_id":"identity-a"}`))
	if err != nil {
		t.Fatal(err)
	}

	first, err := call.Run(context.Background(), testIdentity("identity-a"))
	if err != nil {
		t.Fatal(err)
	}

	second, err := call.Run(context.Background(), testIdentity("identity-b"))
	if err != nil {
		t.Fatal(err)
	}

	if len(provider.identities) != 2 || provider.identities[0].AccountID != "identity-a" || provider.identities[1].AccountID != "identity-b" {
		t.Fatalf("identities were not forwarded separately: %#v", provider.identities)
	}

	outFirst := first.(mcpcontract.Result[AccountsData])

	outSecond := second.(mcpcontract.Result[AccountsData])
	if outFirst.AccountID != "identity-a" || outSecond.AccountID != "identity-b" {
		t.Fatalf("result envelopes crossed identities: %q %q", outFirst.AccountID, outSecond.AccountID)
	}

	if outFirst.FetchedAt.IsZero() || outSecond.FetchedAt.IsZero() {
		t.Fatal("freshness timestamps are required")
	}

	if recorder.calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", recorder.calls)
	}
}

func TestBusinessProfileListLocationsSendsParentBoundsMaskAndPreservesToken(t *testing.T) {
	t.Parallel()
	operation, provider, recorder := fixture(t, "businessprofile_list_locations", http.StatusOK, `{
		"locations": [
			{"name": "locations/55", "title": "Downtown Office", "storeCode": "DT-1", "websiteUri": "https://robben.media/downtown", "phoneNumbers": {"primaryPhone": "+15550000000"}},
			null
		],
		"nextPageToken": "loc-tok-4"
	}`)

	result, err := decodeRun(t, operation, `{"account_id":"opaque-account","parent":"accounts/123","page_size":7,"page_token":"loc-tok-3"}`, testIdentity("opaque-account"))
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[LocationsData])
	if out.Data.Parent != "accounts/123" || len(out.Data.Locations) != 1 {
		t.Fatalf("unexpected locations projection: %#v", out.Data)
	}

	location := out.Data.Locations[0]
	if location.Name != "locations/55" || location.Title != "Downtown Office" || location.StoreCode != "DT-1" || location.WebsiteURI != "https://robben.media/downtown" {
		t.Fatalf("unexpected location: %#v", location)
	}

	if recorder.path != "/v1/accounts/123/locations" {
		t.Fatalf("locations path = %q", recorder.path)
	}

	if got := recorder.query.Get("pageSize"); got != "7" {
		t.Fatalf("pageSize = %q, want 7", got)
	}

	if got := recorder.query.Get("pageToken"); got != "loc-tok-3" {
		t.Fatalf("pageToken = %q, want explicit loc-tok-3", got)
	}

	if got := recorder.query.Get("readMask"); got != "name,title,storeCode,websiteUri" {
		t.Fatalf("readMask = %q, want the narrow documented mask", got)
	}

	if out.NextPageToken != "loc-tok-4" {
		t.Fatalf("next page token = %q", out.NextPageToken)
	}

	if out.Truncated {
		t.Fatal("a bounded page must never claim truncation")
	}

	want := mcpcontract.CallOptions{Operation: "businessprofile_list_locations", Retry: mcpcontract.SafeRead}
	if provider.options[0] != want {
		t.Fatalf("unexpected call options: %#v", provider.options[0])
	}
}

func TestBusinessProfileListLocationsDefaultsToDocumentedMaximum(t *testing.T) {
	t.Parallel()
	operation, _, recorder := fixture(t, "businessprofile_list_locations", http.StatusOK, `{"locations": [{"name": "locations/9", "title": "Only One"}]}`)

	result, err := decodeRun(t, operation, `{"account_id":"opaque-account","parent":"accounts/77"}`, testIdentity("opaque-account"))
	if err != nil {
		t.Fatal(err)
	}

	out := result.(mcpcontract.Result[LocationsData])
	if len(out.Data.Locations) != 1 || out.Data.Locations[0].Name != "locations/9" {
		t.Fatalf("unexpected locations: %#v", out.Data)
	}

	if got := recorder.query.Get("pageSize"); got != "100" {
		t.Fatalf("pageSize = %q, want default 100", got)
	}

	if got := recorder.query.Get("pageToken"); got != "" {
		t.Fatalf("pageToken must stay empty for the first page: %v", recorder.query)
	}

	if out.NextPageToken != "" || out.Truncated {
		t.Fatalf("exact page was reported as truncated: %#v", out)
	}
}

func TestBusinessProfileListLocationsValidationHappensBeforeUpstream(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"account_id":"a","parent":""}`,
		`{"account_id":"a","parent":"accounts/"}`,
		`{"account_id":"a","parent":"accounts/ "}`,
		`{"account_id":"a","parent":"locations/123"}`,
		`{"account_id":"a","parent":"123"}`,
		`{"account_id":"a","parent":"accounts/123/locations/9"}`,
		`{"account_id":"a","parent":" accounts/123"}`,
		`{"account_id":"a","parent":"accounts/123","page_size":101}`,
		`{"account_id":"a","parent":"accounts/123","page_size":-1}`,
	} {
		operation, provider, recorder := fixture(t, "businessprofile_list_locations", http.StatusOK, `{}`)
		if _, err := decodeRun(t, operation, raw, testIdentity("a")); err == nil {
			t.Fatalf("accepted invalid arguments %s", raw)
		} else if len(provider.options) != 0 || recorder.calls != 0 {
			t.Fatalf("invalid arguments %s reached provider or upstream", raw)
		} else if !isInvalidInput(err) {
			t.Fatalf("invalid arguments %s produced non-contract error: %v", raw, err)
		}
	}
}

func isInvalidInput(err error) bool {
	var safe *mcpcontract.Error

	return errors.As(err, &safe) && safe.Category == mcpcontract.InvalidInput
}

func TestBusinessProfileUpstreamErrorsStayPublicSafe(t *testing.T) {
	t.Parallel()
	const secret = "secret-project-ref-9931"
	errorResponse := `{"error": {"code": 403, "message": "` + secret + `", "status": "PERMISSION_DENIED"}}`

	for _, tc := range []struct {
		operation string
		raw       string
	}{
		{"businessprofile_list_accounts", `{"account_id":"opaque-account"}`},
		{"businessprofile_list_locations", `{"account_id":"opaque-account","parent":"accounts/123"}`},
	} {
		operation, _, _ := fixture(t, tc.operation, http.StatusForbidden, errorResponse)
		if _, runErr := decodeRun(t, operation, tc.raw, testIdentity("opaque-account")); runErr == nil {
			t.Fatalf("%s accepted an upstream failure", tc.operation)
		} else if safe := contractError(runErr); safe == nil {
			t.Fatalf("%s upstream error is not a contract error: %v", tc.operation, runErr)
		} else if safe.Category != mcpcontract.Forbidden || strings.Contains(safe.Message, secret) {
			t.Fatalf("%s leaked unsafe error detail: %#v", tc.operation, safe)
		}
	}
}

func contractError(err error) *mcpcontract.Error {
	var safe *mcpcontract.Error
	if errors.As(err, &safe) {
		return safe
	}

	return nil
}

func TestBusinessProfileInputSchemasMatchFrozenRequests(t *testing.T) {
	t.Parallel()
	operations := Operations(&recordingProvider{})
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
		"businessprofile_list_accounts":  {"account_id", "page_token"},
		"businessprofile_list_locations": {"account_id", "parent", "page_size", "page_token"},
	} {
		for _, field := range want {
			if !properties[name][field] {
				t.Fatalf("%s schema is missing %s", name, field)
			}
		}
	}

	if got := required["businessprofile_list_locations"]; !reflect.DeepEqual(got, []string{"account_id", "parent"}) {
		t.Fatalf("businessprofile_list_locations required fields: %#v", got)
	}

	if got := required["businessprofile_list_accounts"]; !reflect.DeepEqual(got, []string{"account_id"}) {
		t.Fatalf("businessprofile_list_accounts required fields: %#v", got)
	}
}

func TestAccountsContinuationPreservesOpaqueToken(t *testing.T) {
	t.Parallel()
	operation, _, recorder := fixture(t, "businessprofile_list_accounts", http.StatusOK, `{"accounts":[],"nextPageToken":"next"}`)

	_, err := decodeRun(t, operation, `{"account_id":"a","page_token":"  A+/=?%  "}`, testIdentity("a"))
	if err != nil {
		t.Fatal(err)
	}

	if recorder.calls != 1 || recorder.query.Get("pageToken") != "  A+/=?%  " {
		t.Fatalf("continuation changed: calls=%d query=%v", recorder.calls, recorder.query)
	}
}

func TestParentPathMetacharactersRejectedBeforeProvider(t *testing.T) {
	t.Parallel()

	for _, parent := range []string{"accounts/.", "accounts/..", "accounts/a?x=y", "accounts/a#frag", "accounts/%2F", "accounts/a\\b", "accounts/a b", "accounts/a\n"} {
		operation, provider, recorder := fixture(t, "businessprofile_list_locations", http.StatusOK, `{}`)

		raw, err := json.Marshal(map[string]string{"account_id": "a", "parent": parent})
		if err != nil {
			t.Fatal(err)
		}

		_, err = decodeRun(t, operation, string(raw), testIdentity("a"))
		if !isInvalidInput(err) || len(provider.options) != 0 || recorder.calls != 0 {
			t.Fatalf("parent %q: err=%v calls=%d", parent, err, recorder.calls)
		}
	}
}
