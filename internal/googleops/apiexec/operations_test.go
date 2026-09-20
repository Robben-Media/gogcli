package apiexec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/steipete/gogcli/internal/googlecatalog"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

var (
	errMustNotFetchClient = errors.New("must not fetch client")
	errProviderDenied     = errors.New("no")
)

type recordingProvider struct {
	client     *http.Client
	options    []mcpcontract.CallOptions
	identities []mcpcontract.Identity
	err        error
	calls      int
}

func (p *recordingProvider) HTTPClient(_ context.Context, id mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	p.calls++
	p.options = append(p.options, options)

	p.identities = append(p.identities, id)
	if p.err != nil {
		return nil, p.err
	}

	return p.client, nil
}

type rewriteTransport struct {
	url      string
	saw      []*http.Request
	err      error
	base     http.RoundTripper
	response *http.Response
}

func (t *rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())

	t.saw = append(t.saw, cloned)
	if t.err != nil {
		return nil, t.err
	}

	if t.response != nil {
		return t.response, nil
	}

	rewritten := request.Clone(request.Context())
	rewritten.URL.Scheme = "http"
	rewritten.URL.Host = strings.TrimPrefix(t.url, "http://")
	rewritten.Host = ""

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	response, err := base.RoundTrip(rewritten)
	if err != nil {
		return nil, err //nolint:wrapcheck // Transparent test transport.
	}

	return response, nil
}

func testIdentity() mcpcontract.Identity {
	return mcpcontract.Identity{AccountID: "acct", Label: "Work"}
}

func messagesGet() googlecatalog.Method {
	return googlecatalog.Method{
		ID:          "gmail.users.messages.get",
		ToolName:    "google_gmail_users_messages_get",
		Service:     "gmail",
		Version:     "v1",
		Action:      "gmail:gmail.users.messages.get",
		Description: "Gets the specified message.",
		BaseURL:     "https://gmail.googleapis.com/",
		Path:        "gmail/v1/users/{userId}/messages/{id}",
		HTTPMethod:  http.MethodGet,
		Scopes:      []string{"https://www.googleapis.com/auth/gmail.readonly"},
		ReadOnly:    true,
		Parameters: map[string]googlecatalog.Parameter{
			"userId":          {Schema: googlecatalog.Schema{Type: "string", Default: "me"}, Location: "path", Required: true},
			"id":              {Schema: googlecatalog.Schema{Type: "string"}, Location: "path", Required: true},
			"format":          {Schema: googlecatalog.Schema{Type: "string", Enum: []any{"minimal", "full", "raw", "metadata"}, Default: "full"}, Location: "query"},
			"metadataHeaders": {Schema: googlecatalog.Schema{Type: "string"}, Location: "query", Repeated: true},
			"alt":             {Schema: googlecatalog.Schema{Type: "string", Enum: []any{"json", "media", "proto"}}, Location: "query"},
			"uploadType":      {Schema: googlecatalog.Schema{Type: "string"}, Location: "query"},
			"fields":          {Schema: googlecatalog.Schema{Type: "string"}, Location: "query"},
		},
	}
}

func messagesList() googlecatalog.Method {
	method := messagesGet()
	method.ID = "gmail.users.messages.list"
	method.ToolName = "google_gmail_users_messages_list"
	method.Action = "gmail:gmail.users.messages.list"
	method.Path = "gmail/v1/users/{userId}/messages"
	method.Parameters = map[string]googlecatalog.Parameter{
		"userId":     {Schema: googlecatalog.Schema{Type: "string", Default: "me"}, Location: "path", Required: true},
		"q":          {Schema: googlecatalog.Schema{Type: "string"}, Location: "query"},
		"pageToken":  {Schema: googlecatalog.Schema{Type: "string"}, Location: "query"},
		"maxResults": {Schema: googlecatalog.Schema{Type: "integer"}, Location: "query"},
	}

	return method
}

func eventsInsert() googlecatalog.Method {
	minZero := "0"
	maxOne := "1"
	created := &googlecatalog.Schema{Type: "string", Format: "date-time", ReadOnly: true}

	return googlecatalog.Method{
		ID:         "calendar.events.insert",
		ToolName:   "google_calendar_events_insert",
		Service:    "calendar",
		Version:    "v3",
		Action:     "calendar:calendar.events.insert",
		BaseURL:    "https://www.googleapis.com/calendar/v3/",
		Path:       "calendars/{calendarId}/events",
		HTTPMethod: http.MethodPost,
		Scopes:     []string{"https://www.googleapis.com/auth/calendar"},
		Parameters: map[string]googlecatalog.Parameter{
			"calendarId":            {Schema: googlecatalog.Schema{Type: "string"}, Location: "path", Required: true},
			"conferenceDataVersion": {Schema: googlecatalog.Schema{Type: "integer", Minimum: &minZero, Maximum: &maxOne}, Location: "query"},
			"sendUpdates":           {Schema: googlecatalog.Schema{Type: "string", Enum: []any{"all", "externalOnly", "none"}}, Location: "query"},
		},
		Request: &googlecatalog.Schema{
			Type: "object",
			Properties: map[string]*googlecatalog.Schema{
				"summary": {Type: "string"},
				"created": created,
			},
			AdditionalPropertiesForbidden: true,
		},
	}
}

func peopleGet() googlecatalog.Method {
	return googlecatalog.Method{
		ID:         "people.people.get",
		ToolName:   "google_people_people_get",
		Service:    "people",
		Version:    "v1",
		Action:     "people:people.people.get",
		BaseURL:    "https://people.googleapis.com/",
		Path:       "v1/{+resourceName}",
		HTTPMethod: http.MethodGet,
		ReadOnly:   true,
		Scopes:     []string{"https://www.googleapis.com/auth/contacts.readonly"},
		Parameters: map[string]googlecatalog.Parameter{
			"resourceName": {Schema: googlecatalog.Schema{Type: "string", Pattern: `^people/[^/]+$`}, Location: "path", Required: true},
			"personFields": {Schema: googlecatalog.Schema{Type: "string"}, Location: "query"},
		},
	}
}

func TestInputSchemaUsesNamedParameters(t *testing.T) {
	ops := FromMethods(nil, []googlecatalog.Method{messagesGet(), eventsInsert()})
	if len(ops) != 2 {
		t.Fatalf("operations = %d", len(ops))
	}

	get := ops[0]
	if get.InputSchema.Properties["account_id"] == nil || get.InputSchema.Properties["id"] == nil || get.InputSchema.Properties["userId"] == nil {
		t.Fatalf("missing named fields: %#v", get.InputSchema.Properties)
	}

	if get.InputSchema.Properties["body"] != nil {
		t.Fatal("GET advertised a body")
	}

	if get.InputSchema.Properties["alt"] != nil || get.InputSchema.Properties["uploadType"] != nil {
		t.Fatal("media escape parameters were advertised")
	}

	if get.Definition.Retry != mcpcontract.SafeRead {
		t.Fatalf("retry = %s", get.Definition.Retry)
	}

	insert := ops[1]
	if insert.InputSchema.Properties["body"] == nil || insert.InputSchema.Properties["calendarId"] == nil {
		t.Fatal("insert schema missing body or calendarId")
	}

	if insert.Definition.Retry != mcpcontract.NonReplayableWrite {
		t.Fatalf("insert retry = %s", insert.Definition.Retry)
	}

	if created := schemaProperty(insert.InputSchema, insert.InputSchema.Properties["body"], "created"); created != nil && !created.ReadOnly {
		t.Fatal("read-only created leaked as writable")
	}
}

func schemaProperty(root *jsonschema.Schema, schema *jsonschema.Schema, name string) *jsonschema.Schema {
	if schema == nil {
		return nil
	}

	if schema.Properties != nil {
		if property := schema.Properties[name]; property != nil {
			return property
		}
	}

	if ref := strings.TrimPrefix(schema.Ref, "#/$defs/"); ref != "" && root != nil && root.Defs != nil {
		return schemaProperty(root, root.Defs[ref], name)
	}

	return nil
}

func TestInvalidArgsDoNotAcquireClient(t *testing.T) {
	provider := &recordingProvider{err: errMustNotFetchClient}
	op := FromMethods(provider, []googlecatalog.Method{messagesGet(), eventsInsert()})[0]
	insert := FromMethods(provider, []googlecatalog.Method{eventsInsert()})[0]

	for _, raw := range []string{
		`{"account_id":"acct","id":"m1","format":"bogus"}`,
		`{"account_id":"acct","id":"m1","alt":"media"}`,
		`{"account_id":"acct","id":"m1","uploadType":"media"}`,
		`{"account_id":"acct","id":"../x"}`,
		`{"account_id":"acct"}`,
		`{"id":"m1"}`,
		`{"account_id":"acct","id":"m1","unknown":true}`,
		`{"account_id":"acct","id":"m1","quotaUser":"spread"}`,
	} {
		if _, err := op.Decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}

	for _, raw := range []string{
		`{"account_id":"acct","calendarId":"primary","conferenceDataVersion":2}`,
		`{"account_id":"acct","calendarId":"primary","sendUpdates":"maybe"}`,
		`{"account_id":"acct","calendarId":"primary","body":{"created":"2020-01-01T00:00:00Z"}}`,
		`{"account_id":"acct","calendarId":"primary","body":{"nope":true}}`,
		`{"account_id":"acct","calendarId":"primary"}`,
	} {
		if _, err := insert.Decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}

	if provider.calls != 0 {
		t.Fatalf("provider calls = %d", provider.calls)
	}
}

func TestGetBuildsPinnedPathAndQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","threadId":"t1"}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{url: server.URL}
	provider := &recordingProvider{client: &http.Client{Transport: transport}}

	call, err := FromMethods(provider, []googlecatalog.Method{messagesGet()})[0].Decode(json.RawMessage(`{"account_id":"acct","userId":"me","id":"m1","format":"metadata","metadataHeaders":["From","To"],"fields":"id"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[json.RawMessage])
	if result.Truncated || result.AccountID != "acct" || !strings.Contains(string(result.Data), `"id":"m1"`) {
		t.Fatalf("result = %#v", result)
	}

	if len(transport.saw) != 1 {
		t.Fatalf("requests = %d", len(transport.saw))
	}

	got := transport.saw[0]
	if got.URL.Scheme != "https" || got.URL.Host != "gmail.googleapis.com" {
		t.Fatalf("url = %s", got.URL)
	}

	if got.URL.Path != "/gmail/v1/users/me/messages/m1" {
		t.Fatalf("path = %s", got.URL.Path)
	}

	query := got.URL.Query()
	if query.Get("format") != "metadata" || query.Get("fields") != "id" {
		t.Fatalf("query = %s", got.URL.RawQuery)
	}

	if got := query["metadataHeaders"]; len(got) != 2 || got[0] != "From" || got[1] != "To" {
		t.Fatalf("headers = %#v", got)
	}

	if got.Header.Get("Authorization") != "" {
		t.Fatal("executor set Authorization")
	}

	if len(provider.options) != 1 || provider.options[0].Operation != "google_gmail_users_messages_get" || provider.options[0].Retry != mcpcontract.SafeRead {
		t.Fatalf("options = %#v", provider.options)
	}
}

func TestListPreservesNextPageTokenWithoutFetchingNextPage(t *testing.T) {
	hits := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"messages":[{"id":"m1"}],"nextPageToken":"page-2"}`))
	}))
	defer server.Close()

	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	call, err := FromMethods(provider, []googlecatalog.Method{messagesList()})[0].Decode(json.RawMessage(`{"account_id":"acct","userId":"me"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[json.RawMessage])
	if result.NextPageToken != "page-2" || result.Truncated || hits != 1 {
		t.Fatalf("token=%q truncated=%v hits=%d", result.NextPageToken, result.Truncated, hits)
	}
}

func TestPostJSONBodyAndWriteTimeoutUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/calendar/v3/calendars/primary/events" {
			t.Errorf("path = %s", r.URL.Path)
		}

		if string(body) != `{"summary":"Standup"}` {
			t.Errorf("body = %s", body)
		}
		_, _ = w.Write([]byte(`{"id":"evt1","summary":"Standup"}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{url: server.URL}
	provider := &recordingProvider{client: &http.Client{Transport: transport}}
	op := FromMethods(provider, []googlecatalog.Method{eventsInsert()})[0]

	call, err := op.Decode(json.RawMessage(`{"account_id":"acct","calendarId":"primary","conferenceDataVersion":1,"body":{"summary":"Standup"}}`))
	if err != nil {
		t.Fatal(err)
	}

	if _, runErr := call.Run(context.Background(), testIdentity()); runErr != nil {
		t.Fatal(runErr)
	}

	if transport.saw[0].Method != http.MethodPost {
		t.Fatalf("method = %s", transport.saw[0].Method)
	}

	if provider.options[0].Retry != mcpcontract.NonReplayableWrite {
		t.Fatalf("retry = %s", provider.options[0].Retry)
	}

	timeout := &rewriteTransport{err: context.DeadlineExceeded}
	failing := &recordingProvider{client: &http.Client{Transport: timeout}}

	call, err = FromMethods(failing, []googlecatalog.Method{eventsInsert()})[0].Decode(json.RawMessage(`{"account_id":"acct","calendarId":"primary","body":{"summary":"Standup"}}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), testIdentity())

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable {
		t.Fatalf("err = %#v", err)
	}

	if failing.calls != 1 {
		t.Fatalf("timeout skipped provider: %d", failing.calls)
	}
}

func TestGetTimeoutIsDeadline(t *testing.T) {
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{err: context.DeadlineExceeded}}}

	call, err := FromMethods(provider, []googlecatalog.Method{messagesGet()})[0].Decode(json.RawMessage(`{"account_id":"acct","id":"m1"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), testIdentity())

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.DeadlineExceeded {
		t.Fatalf("err = %#v", err)
	}
}

func TestReservedPathAndPattern(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/people/me" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"resourceName":"people/me"}`))
	}))
	defer server.Close()

	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	op := FromMethods(provider, []googlecatalog.Method{peopleGet()})[0]
	if _, err := op.Decode(json.RawMessage(`{"account_id":"acct","resourceName":"people/me/extra"}`)); err == nil {
		t.Fatal("accepted extra segment")
	}

	if _, err := op.Decode(json.RawMessage(`{"account_id":"acct","resourceName":"https://evil.example/people/me"}`)); err == nil {
		t.Fatal("accepted url")
	}

	if provider.calls != 0 {
		t.Fatal("invalid reserved path used the provider")
	}

	call, err := op.Decode(json.RawMessage(`{"account_id":"acct","resourceName":"people/me","personFields":"names"}`))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := call.Run(context.Background(), testIdentity()); err != nil {
		t.Fatal(err)
	}
}

func TestTruncatedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"nextPageToken":"keep",`))

		chunk := bytesRepeat('a', 1024*1024)
		for i := 0; i < 9; i++ {
			_, _ = w.Write(chunk)
		}
		_, _ = w.Write([]byte(`"x":1}`))
	}))
	defer server.Close()

	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	call, err := FromMethods(provider, []googlecatalog.Method{messagesList()})[0].Decode(json.RawMessage(`{"account_id":"acct","userId":"me"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[json.RawMessage])
	if !result.Truncated || result.NextPageToken != "" || string(result.Data) != "null" {
		t.Fatalf("truncated result = %#v", result)
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}

	return out
}

func TestActualCatalogMethods(t *testing.T) {
	get := catalogMethod(t, "gmail.users.messages.get")
	insert := catalogMethod(t, "calendar.events.insert")
	send := catalogMethod(t, "gmail.users.messages.send")

	ops := FromMethods(nil, []googlecatalog.Method{get, insert, send})
	if len(ops) != 3 {
		t.Fatalf("ops = %d", len(ops))
	}

	if ops[0].InputSchema.Properties["id"] == nil || ops[1].InputSchema.Properties["body"] == nil {
		t.Fatal("catalog schemas missing expected fields")
	}

	if ops[2].InputSchema.Properties["uploadType"] != nil || ops[2].InputSchema.Properties["alt"] != nil {
		t.Fatal("media parameters advertised on send")
	}

	if _, err := ops[2].Decode(json.RawMessage(`{"account_id":"acct","userId":"me","uploadType":"multipart","body":{"raw":"YQ=="}}`)); err == nil {
		t.Fatal("uploadType accepted")
	}

	provider := &recordingProvider{err: errProviderDenied}
	if _, err := FromMethods(provider, []googlecatalog.Method{get})[0].Decode(json.RawMessage(`{"account_id":"acct","id":"m1","format":"not-a-format"}`)); err == nil {
		t.Fatal("catalog enum accepted invalid format")
	}

	if provider.calls != 0 {
		t.Fatal("invalid catalog args hit provider")
	}
}

func catalogMethod(t *testing.T, id string) googlecatalog.Method {
	t.Helper()

	for _, method := range googlecatalog.MustLoad().Methods {
		if method.ID == id {
			return method
		}
	}

	t.Fatalf("missing %s", id)

	return googlecatalog.Method{}
}

func TestOperationsMatchAPIDefinitions(t *testing.T) {
	started := time.Now()
	ops := Operations(nil)
	defs := mcpcontract.APIDefinitions()

	methods := map[string]googlecatalog.Method{}
	for _, method := range googlecatalog.MustLoad().Methods {
		methods[method.ToolName] = method
	}

	byName := make(map[string]mcpcontract.Operation, len(ops))
	for _, op := range ops {
		if _, exists := byName[op.Definition.Name]; exists {
			t.Fatalf("duplicate %s", op.Definition.Name)
		}

		def, ok := mcpcontract.Lookup(op.Definition.Name)
		if !ok || def.Local {
			t.Fatalf("emitted unknown %s", op.Definition.Name)
		}

		if op.Definition.Name != def.Name || op.Definition.Retry != def.Retry || op.Definition.AnyScope != def.AnyScope {
			t.Fatalf("%s mismatch %#v vs %#v", def.Name, op.Definition, def)
		}

		if strings.Join(op.Definition.Actions, ",") != strings.Join(def.Actions, ",") {
			t.Fatalf("%s actions %v vs %v", def.Name, op.Definition.Actions, def.Actions)
		}

		if op.InputSchema == nil || op.InputSchema.Properties["account_id"] == nil {
			t.Fatalf("%s missing account_id", op.Definition.Name)
		}

		if schemaCycle(op.InputSchema, map[*jsonschema.Schema]bool{}, map[*jsonschema.Schema]bool{}) {
			t.Fatalf("%s schema contains cyclic pointers instead of refs", op.Definition.Name)
		}

		data, err := json.Marshal(op.InputSchema)
		if err != nil {
			t.Fatalf("%s schema marshal %v", op.Definition.Name, err)
		}

		if strings.Contains(string(data), `"$ref":"#/$defs/`) && op.InputSchema.Defs == nil {
			t.Fatalf("%s $ref without $defs", op.Definition.Name)
		}

		if _, err := op.InputSchema.Resolve(nil); err != nil {
			t.Fatalf("%s schema resolve: %v", op.Definition.Name, err)
		}
		byName[op.Definition.Name] = op
	}

	gated := 0

	for _, def := range defs {
		method, ok := methods[def.Name]
		if !ok {
			t.Fatalf("definition %s missing from catalog", def.Name)
		}

		reason := UnsupportedReason(method)
		if reason != "" {
			gated++

			if _, exists := byName[def.Name]; exists {
				t.Fatalf("gated %s was emitted: %s", def.Name, reason)
			}

			continue
		}

		if _, ok := byName[def.Name]; !ok {
			t.Fatalf("missing operation %s", def.Name)
		}
	}

	if gated == 0 {
		t.Fatal("expected media-only methods to be gated")
	}

	if len(ops)+gated != len(defs) {
		t.Fatalf("operations=%d gated=%d definitions=%d", len(ops), gated, len(defs))
	}

	if time.Since(started) > 20*time.Second {
		t.Fatalf("Operations took %s", time.Since(started))
	}
}

func TestSheetsRangePath(t *testing.T) {
	method := catalogMethod(t, "sheets.spreadsheets.values.get")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"range":"Sheet1!A1:B2","values":[["a"]]}`))
	}))
	defer server.Close()
	transport := &rewriteTransport{url: server.URL}
	provider := &recordingProvider{client: &http.Client{Transport: transport}}
	raw := `{"account_id":"acct","spreadsheetId":"sheet-1","range":"Sheet1!A1:B2","majorDimension":"ROWS"}`

	call, err := FromMethods(provider, []googlecatalog.Method{method})[0].Decode(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := call.Run(context.Background(), testIdentity()); err != nil {
		t.Fatal(err)
	}

	got := transport.saw[0]
	if !strings.Contains(got.URL.EscapedPath(), "/v4/spreadsheets/sheet-1/values/Sheet1") {
		t.Fatalf("path = %s", got.URL.EscapedPath())
	}

	if got.URL.Query().Get("majorDimension") != "ROWS" {
		t.Fatalf("query = %s", got.URL.RawQuery)
	}
}

func TestMediaOnlyMethodsAreGated(t *testing.T) {
	for _, id := range []string{"drive.files.export", "youtube.thumbnails.set", "youtube.captions.download", "chat.media.download", "youtube.videos.insert", "youtube.captions.insert", "youtube.playlistImages.insert"} {
		method := catalogMethod(t, id)
		if UnsupportedReason(method) == "" {
			t.Fatalf("%s should require media transport", id)
		}

		if ops := FromMethods(nil, []googlecatalog.Method{method}); len(ops) != 0 {
			t.Fatalf("%s was emitted", id)
		}
	}

	create := catalogMethod(t, "drive.files.create")
	if UnsupportedReason(create) != "" {
		t.Fatalf("drive.files.create should allow JSON metadata: %s", UnsupportedReason(create))
	}

	if ops := FromMethods(nil, []googlecatalog.Method{create}); len(ops) != 1 {
		t.Fatal("drive.files.create was gated")
	}

	download := catalogMethod(t, "drive.files.download")
	if UnsupportedReason(download) != "" {
		t.Fatalf("drive.files.download should return JSON LRO: %s", UnsupportedReason(download))
	}

	if ops := FromMethods(nil, []googlecatalog.Method{download}); len(ops) != 1 {
		t.Fatal("drive.files.download was gated")
	}

	for _, id := range []string{"youtube.captions.update", "youtube.playlistImages.update"} {
		method := catalogMethod(t, id)
		if UnsupportedReason(method) != "" {
			t.Fatalf("%s metadata update was gated: %s", id, UnsupportedReason(method))
		}

		if ops := FromMethods(nil, []googlecatalog.Method{method}); len(ops) != 1 {
			t.Fatalf("%s was omitted", id)
		}
	}

	emitted := map[string]bool{}
	for _, op := range Operations(nil) {
		emitted[op.Definition.Name] = true
	}

	for _, id := range []string{"youtube.videos.insert", "youtube.captions.insert", "youtube.playlistImages.insert"} {
		name := catalogMethod(t, id).ToolName
		if emitted[name] {
			t.Fatalf("Operations emitted mandatory media method %s", id)
		}
	}
}

func TestStreamingLiveChatIsGated(t *testing.T) {
	method := catalogMethod(t, "youtube.youtube.v3.liveChat.messages.stream")

	reason := UnsupportedReason(method)
	if reason == "" {
		t.Fatal("live chat stream should be unsupported")
	}

	if reason == "media upload and download are not available" {
		t.Fatalf("streaming used media reason: %s", reason)
	}

	if ops := FromMethods(nil, []googlecatalog.Method{method}); len(ops) != 0 {
		t.Fatal("FromMethods emitted live chat stream")
	}

	for _, op := range Operations(nil) {
		if op.Definition.Name == method.ToolName {
			t.Fatalf("Operations emitted %s", method.ToolName)
		}
	}
}

func TestInputSchemaMarshalsCyclicCatalogTypes(t *testing.T) {
	method := catalogMethod(t, "calendar.events.insert")

	op := FromMethods(nil, []googlecatalog.Method{method})[0]
	if op.InputSchema.Properties["body"] == nil || op.InputSchema.Properties["body"].Ref == "" {
		t.Fatalf("body should be a $ref, got %#v", op.InputSchema.Properties["body"])
	}

	if len(op.InputSchema.Defs) == 0 {
		t.Fatal("expected $defs")
	}

	data, err := json.Marshal(op.InputSchema)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `"$ref":"#/$defs/`) {
		t.Fatalf("missing $ref in %s", data[:200])
	}
}

func TestMalformedJSONOutcomes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	readProvider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	call, err := FromMethods(readProvider, []googlecatalog.Method{messagesGet()})[0].Decode(json.RawMessage(`{"account_id":"acct","id":"m1"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), testIdentity())

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.UpstreamFailure {
		t.Fatalf("read err = %#v", err)
	}

	writeProvider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	call, err = FromMethods(writeProvider, []googlecatalog.Method{eventsInsert()})[0].Decode(json.RawMessage(`{"account_id":"acct","calendarId":"primary","body":{"summary":"x"}}`))
	if err != nil {
		t.Fatal(err)
	}

	_, err = call.Run(context.Background(), testIdentity())
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable {
		t.Fatalf("write err = %#v", err)
	}
}

func TestMapDoErrorPreservesBudget(t *testing.T) {
	budget := &mcpcontract.Error{Category: mcpcontract.BudgetExhausted, Message: "API call budget exhausted; narrow the task or explicitly start a new bounded request"}

	got := mapDoError(budget, mcpcontract.NonReplayableWrite)
	if !errors.Is(got, budget) {
		t.Fatalf("got %#v", got)
	}
}

func TestTruncatedWriteIsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"`))
		_, _ = w.Write(bytesRepeat('a', int(maxResponseBytes)+1))
	}))
	defer server.Close()
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{url: server.URL}}}

	call, err := FromMethods(provider, []googlecatalog.Method{eventsInsert()})[0].Decode(json.RawMessage(`{"account_id":"acct","calendarId":"primary","body":{"summary":"x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = call.Run(context.Background(), testIdentity())

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable {
		t.Fatalf("err = %#v", err)
	}
}

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

func TestRejectReadOnlyNestedInArray(t *testing.T) {
	method := catalogMethod(t, "admin.customers.chrome.printers.batchCreatePrinters")

	ops := FromMethods(nil, []googlecatalog.Method{method})
	if len(ops) != 1 {
		t.Fatalf("ops = %d", len(ops))
	}
	provider := &recordingProvider{err: errMustNotFetchClient}
	op := FromMethods(provider, []googlecatalog.Method{method})[0]

	readonly := `{"account_id":"acct","parent":"customers/my_customer","body":{"requests":[{"parent":"customers/my_customer","printer":{"displayName":"Lobby","createTime":"2020-01-01T00:00:00Z"}}]}}`
	if _, err := op.Decode(json.RawMessage(readonly)); err == nil {
		t.Fatal("accepted read-only createTime inside requests[]")
	}

	okBody := `{"account_id":"acct","parent":"customers/my_customer","body":{"requests":[{"parent":"customers/my_customer","printer":{"displayName":"Lobby"}}]}}`
	if _, err := op.Decode(json.RawMessage(okBody)); err != nil {
		t.Fatal(err)
	}

	if provider.calls != 0 {
		t.Fatalf("provider calls = %d", provider.calls)
	}
}

func TestRequiredForCurrentMethodOnly(t *testing.T) {
	insert := FromMethods(nil, []googlecatalog.Method{catalogMethod(t, "calendar.events.insert")})[0]
	if _, err := insert.Decode(json.RawMessage(`{"account_id":"acct","calendarId":"primary","body":{"summary":"standup"}}`)); err == nil {
		t.Fatal("insert accepted body without start/end")
	}

	if _, err := insert.Decode(json.RawMessage(`{"account_id":"acct","calendarId":"primary","body":{"summary":"standup","start":{"dateTime":"2020-01-01T10:00:00Z"},"end":{"dateTime":"2020-01-01T10:30:00Z"}}}`)); err != nil {
		t.Fatal(err)
	}

	patch := FromMethods(nil, []googlecatalog.Method{catalogMethod(t, "calendar.events.patch")})[0]
	if _, err := patch.Decode(json.RawMessage(`{"account_id":"acct","calendarId":"primary","eventId":"evt1","body":{"summary":"renamed"}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestDriveFilesDownloadReturnsOperationJSON(t *testing.T) {
	method := catalogMethod(t, "drive.files.download")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/drive/v3/files/file-1/download" {
			t.Errorf("path = %s", r.URL.Path)
		}

		if strings.Contains(r.URL.RawQuery, "alt=media") {
			t.Errorf("media query = %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"name":"operations/abc","done":false}`))
	}))
	defer server.Close()

	transport := &rewriteTransport{url: server.URL}
	provider := &recordingProvider{client: &http.Client{Transport: transport}}

	call, err := FromMethods(provider, []googlecatalog.Method{method})[0].Decode(json.RawMessage(`{"account_id":"acct","fileId":"file-1"}`))
	if err != nil {
		t.Fatal(err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatal(err)
	}

	result := resultAny.(mcpcontract.Result[json.RawMessage])
	if !strings.Contains(string(result.Data), `"name":"operations/abc"`) || result.Truncated {
		t.Fatalf("result = %s", result.Data)
	}

	if len(transport.saw) != 1 {
		t.Fatalf("followed extra URLs: %d", len(transport.saw))
	}
}

func TestRejectReadOnlyRefWrapper(t *testing.T) {
	method := catalogMethod(t, "bigquery.jobs.insert")
	provider := &recordingProvider{err: errMustNotFetchClient}

	op := FromMethods(provider, []googlecatalog.Method{method})[0]
	if _, err := op.Decode(json.RawMessage(`{"account_id":"acct","projectId":"p","body":{"statistics":{"creationTime":"1"}}}`)); err == nil {
		t.Fatal("accepted read-only Job.statistics")
	}

	if provider.calls != 0 {
		t.Fatalf("provider calls = %d", provider.calls)
	}
}

func TestDiscoveryAnyAllowsBoundedJSON(t *testing.T) {
	method := eventsInsert()
	method.Request.Properties["metadata"] = &googlecatalog.Schema{Type: "any", Description: "opaque provider payload"}
	op := FromMethods(nil, []googlecatalog.Method{method})[0]

	raw := `{"account_id":"acct","calendarId":"primary","body":{"summary":"standup","metadata":[{"k":"v"},123,true]}}`
	if _, err := op.Decode(json.RawMessage(raw)); err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(op.InputSchema)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(data), `"type":"any"`) {
		t.Fatalf("emitted invalid JSON Schema type any: %s", data[:300])
	}

	if _, err := op.InputSchema.Resolve(nil); err != nil {
		t.Fatal(err)
	}
}

func TestBigQueryInsertAllowsJSONNull(t *testing.T) {
	method := catalogMethod(t, "bigquery.tabledata.insertAll")
	provider := &recordingProvider{err: errMustNotFetchClient}
	op := FromMethods(provider, []googlecatalog.Method{method})[0]

	if _, err := op.Decode(json.RawMessage(`{"account_id":"acct","projectId":"p","datasetId":"d","tableId":"t","body":{"rows":[{"json":{"nullable_column":null}}]}}`)); err != nil {
		t.Fatal(err)
	}

	if provider.calls != 0 {
		t.Fatalf("provider calls = %d", provider.calls)
	}
}

func TestWriteBodyReadUnexpectedEOFIsUnknown(t *testing.T) {
	body := &partialReadCloser{rest: []byte(`{"id":"created"`), err: io.ErrUnexpectedEOF}
	provider := &recordingProvider{client: &http.Client{Transport: &rewriteTransport{
		response: &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)},
	}}}

	call, err := FromMethods(provider, []googlecatalog.Method{eventsInsert()})[0].Decode(json.RawMessage(`{"account_id":"acct","calendarId":"primary","body":{"summary":"x"}}`))
	if err != nil {
		t.Fatal(err)
	}

	_, err = call.Run(context.Background(), testIdentity())

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable {
		t.Fatalf("err = %#v", err)
	}
}

type partialReadCloser struct {
	rest []byte
	err  error
}

func (p *partialReadCloser) Read(b []byte) (int, error) {
	if len(p.rest) == 0 {
		return 0, p.err
	}

	n := copy(b, p.rest)
	p.rest = p.rest[n:]

	return n, nil
}

func (p *partialReadCloser) Close() error { return nil }
