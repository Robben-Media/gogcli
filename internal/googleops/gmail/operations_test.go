package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

type recordingProvider struct {
	client  *http.Client
	options []mcpcontract.CallOptions
	err     error
}

func (p *recordingProvider) HTTPClient(_ context.Context, _ mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	p.options = append(p.options, options)
	if p.err != nil {
		return nil, p.err
	}

	return p.client, nil
}

var errMustNotFetchClient = errors.New("must not fetch client")

func TestOperationsDefinitionAndSchemas(t *testing.T) {
	operations := Operations(&recordingProvider{})
	if len(operations) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(operations))
	}

	want := []string{"gmail_search", "gmail_get_message", "gmail_get_thread"}
	for index, name := range want {
		operation := operations[index]
		if operation.Definition.Name != name {
			t.Fatalf("operation %d = %q, want %q", index, operation.Definition.Name, name)
		}

		if operation.Definition.Retry != mcpcontract.SafeRead {
			t.Fatalf("%s retry = %q, want safe_read", name, operation.Definition.Retry)
		}

		if operation.InputSchema.Properties["account_id"] == nil {
			t.Fatalf("%s input schema has no account_id", name)
		}
	}

	search := operations[0]
	if search.InputSchema.Properties["query"] == nil {
		t.Fatal("gmail_search schema has no query")
	}

	if property := search.InputSchema.Properties["include_body"]; property == nil || property.Type != "boolean" || string(property.Default) != "false" {
		t.Fatalf("gmail_search include_body schema = %#v", search.InputSchema.Properties["include_body"])
	}

	getMessage := operations[1]
	if property := getMessage.InputSchema.Properties["include_body"]; property == nil || property.Type != "boolean" || string(property.Default) != "true" {
		t.Fatalf("gmail_get_message include_body schema = %#v", getMessage.InputSchema.Properties["include_body"])
	}

	if property := getMessage.InputSchema.Properties["max_body_bytes"]; property == nil || string(property.Default) != "65536" {
		t.Fatalf("gmail_get_message max_body_bytes schema = %#v", property)
	}
}

func TestSearchMetadataFirstPaginationAndLabels(t *testing.T) {
	var requestsMu sync.Mutex
	var requests []*http.Request

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()

		requests = append(requests, r)
		requestsMu.Unlock()

		switch {
		case r.URL.Path == "/gmail/v1/users/me/messages" && r.Method == http.MethodGet:
			if r.URL.Query().Get("q") != "from:test@example.com" {
				t.Errorf("q = %q", r.URL.Query().Get("q"))
			}

			if got := r.URL.Query().Get("maxResults"); got != "2" {
				t.Errorf("maxResults = %q, want 2", got)
			}

			if got := r.URL.Query().Get("pageToken"); got != "next" {
				t.Errorf("pageToken = %q, want next", got)
			}
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1","threadId":"t1"},{"id":"m2","threadId":"t1"}],"nextPageToken":"after","resultSizeEstimate":10}`))
		case r.URL.Path == "/gmail/v1/users/me/labels":
			_, _ = w.Write([]byte(`{"labels":[{"id":"INBOX","name":"Inbox"},{"id":"Label_1","name":"Client"}]}`))
		case r.URL.Path == "/gmail/v1/users/me/messages/m1" || r.URL.Path == "/gmail/v1/users/me/messages/m2":
			if got := r.URL.Query().Get("format"); got != "metadata" {
				t.Errorf("%s format = %q, want metadata", r.URL.Path, got)
			}
			_, _ = fmt.Fprintf(w, `{"id":%q,"threadId":"t1","internalDate":"1765900800000","labelIds":["INBOX","Label_1"],"payload":{"headers":[{"name":"From","value":"Sender <sender@example.com>"},{"name":"Subject","value":"Client brief"},{"name":"Date","value":"Thu, 18 Sep 2026 08:00:00 -0500"}]}}`, strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{url: server.URL}}
	operation := Operations(provider)[0]

	call, err := operation.Decode(json.RawMessage(`{"account_id":"acct","query":"from:test@example.com","max_results":2,"page_token":"next"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if call.AccountID != "acct" || len(call.Actions) != 1 || call.Actions[0] != "gmail:messages.search" {
		t.Fatalf("unexpected call: %#v", call)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	result, ok := resultAny.(mcpcontract.Result[SearchData])
	if !ok {
		t.Fatalf("unexpected result type %T", resultAny)
	}

	requestsMu.Lock()

	requests = append([]*http.Request(nil), requests...)

	requestsMu.Unlock()

	if result.AccountID != "acct" || result.AccountLabel != "Work mailbox" {
		t.Fatalf("unexpected result identity: %#v", result)
	}

	if len(result.Data.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(result.Data.Messages))
	}

	if result.Data.Messages[0].Body != "" || result.Data.Messages[0].BodyTruncated {
		t.Fatalf("metadata search returned body state %#v", result.Data.Messages[0])
	}

	if result.Data.Messages[0].LabelIDs[1] != "Label_1" || result.Data.Messages[0].LabelNames[1] != "Client" {
		t.Fatalf("label IDs/names = %#v / %#v", result.Data.Messages[0].LabelIDs, result.Data.Messages[0].LabelNames)
	}

	if result.Data.Messages[0].InternalDate != "2025-12-16T16:00:00Z" {
		t.Fatalf("internal date = %q", result.Data.Messages[0].InternalDate)
	}

	if result.NextPageToken != "after" || !result.Truncated || result.Data.ResultSizeEstimate != 10 {
		t.Fatalf("pagination result = %#v", result)
	}

	if len(provider.options) != 1 || provider.options[0].Operation != "gmail_search" || provider.options[0].Retry != mcpcontract.SafeRead {
		t.Fatalf("provider options = %#v", provider.options)
	}
}

func TestGetMessageOmittedIncludeBodyDefaultsTrue(t *testing.T) {
	var formats []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		formats = append(formats, r.URL.Query().Get("format"))
		if r.URL.Path != "/gmail/v1/users/me/messages/m1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		plain := base64.RawURLEncoding.EncodeToString([]byte("plain body"))
		html := base64.RawURLEncoding.EncodeToString([]byte("<p>html body</p>"))
		_, _ = fmt.Fprintf(w, `{"id":"m1","threadId":"t1","snippet":"plain snippet","payload":{"headers":[{"name":"From","value":"sender@example.com"},{"name":"Subject","value":"Hello"}],"parts":[{"mimeType":"text/plain","body":{"data":%q}},{"mimeType":"text/html","body":{"data":%q}},{"filename":"brief.pdf","mimeType":"application/pdf","body":{"attachmentId":"a1","size":123}}]}}`, plain, html)
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, url: server.URL}}
	operation := Operations(provider)[1]

	call, err := operation.Decode(json.RawMessage(`{"account_id":"acct","message_id":"m1"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	result := resultAny.(mcpcontract.Result[MessageView])
	if result.Data.Body != "plain body" {
		t.Fatalf("body = %q, want plain body", result.Data.Body)
	}

	if len(result.Data.Attachments) != 1 || result.Data.Attachments[0].AttachmentID != "a1" {
		t.Fatalf("attachments = %#v", result.Data.Attachments)
	}

	if formats[len(formats)-1] != "full" {
		t.Fatalf("format = %v, want full", formats)
	}
}

func TestGetMessageExplicitFalseUsesMetadataAndBoundsBeforeClient(t *testing.T) {
	invalid := Operations(&recordingProvider{err: errMustNotFetchClient})

	rawInputs := []string{
		`{"account_id":"acct","message_id":"m1","max_body_bytes":262145}`,
		`{"account_id":"acct","message_id":"","include_body":false}`,
		`{"account":"acct","message_id":"m1"}`,
		`{"account_id":"","message_id":"m1"}`,
	}
	for _, raw := range rawInputs {
		if _, err := invalid[1].Decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected invalid input %s to fail", raw)
		}
	}

	var format string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		format = r.URL.Query().Get("format")
		_, _ = w.Write([]byte(`{"id":"m1","threadId":"t1","payload":{"headers":[{"name":"Subject","value":"Metadata only"}]}}`))
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, url: server.URL}}
	operation := Operations(provider)[1]

	call, err := operation.Decode(json.RawMessage(`{"account_id":"acct","message_id":"m1","include_body":false}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	result := resultAny.(mcpcontract.Result[MessageView])
	if result.Data.Body != "" || format != "metadata" {
		t.Fatalf("body = %q format = %q, want metadata-only", result.Data.Body, format)
	}
}

func TestSearchHydrationFailureFailsWholePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1"},{"id":"m2"}]}`))
		case "/gmail/v1/users/me/labels":
			_, _ = w.Write([]byte(`{"labels":[]}`))
		case "/gmail/v1/users/me/messages/m1", "/gmail/v1/users/me/messages/m2":
			http.Error(w, "missing", http.StatusNotFound)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, url: server.URL}}

	call, err := Operations(provider)[0].Decode(json.RawMessage(`{"account_id":"acct","query":"all"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	result, err := call.Run(context.Background(), testIdentity())
	if err == nil {
		t.Fatalf("expected whole-page failure, got result=%v", result)
	}

	var contractErr *mcpcontract.Error
	if !errors.As(err, &contractErr) || contractErr.Category != mcpcontract.NotFound {
		t.Fatalf("error = %v, want typed not_found", err)
	}
}

func TestGetThreadPreservesOrderAndReportsTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/gmail/v1/users/me/labels":
			_, _ = w.Write([]byte(`{"labels":[{"id":"INBOX","name":"Inbox"}]}`))
		case r.URL.Path == "/gmail/v1/users/me/threads/t1" && r.URL.Query().Get("format") == "full":
			first := base64.RawURLEncoding.EncodeToString([]byte("first message body"))
			second := base64.RawURLEncoding.EncodeToString([]byte("second message body"))
			_, _ = fmt.Fprintf(w, `{"id":"t1","snippet":"thread","messages":[{"id":"m1","threadId":"t1","labelIds":["INBOX"],"payload":{"headers":[{"name":"Subject","value":"One"}],"mimeType":"text/plain","body":{"data":%q}}},{"id":"m2","threadId":"t1","payload":{"headers":[{"name":"Subject","value":"Two"}],"mimeType":"text/plain","body":{"data":%q}}}]}`, first, second)
		default:
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{base: http.DefaultTransport, url: server.URL}}

	call, err := Operations(provider)[2].Decode(json.RawMessage(`{"account_id":"acct","thread_id":"t1","max_messages":1,"max_body_bytes":8}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	result := resultAny.(mcpcontract.Result[ThreadView])
	if len(result.Data.Messages) != 1 || result.Data.Messages[0].ID != "m1" {
		t.Fatalf("messages = %#v, want first message only", result.Data.Messages)
	}

	if !result.Data.MessagesTruncated || !result.Data.Messages[0].BodyTruncated || !result.Truncated {
		t.Fatalf("truncation state = %#v", result)
	}

	if result.Data.Messages[0].Body != "first me" {
		t.Fatalf("bounded body = %q", result.Data.Messages[0].Body)
	}
}

func testIdentity() mcpcontract.Identity {
	return mcpcontract.Identity{
		AccountID:   "acct",
		Subject:     "subject-1",
		Email:       "work@example.com",
		Label:       "Work mailbox",
		PrincipalID: "principal-1",
		ClientName:  "oauth-client",
		AuthMode:    "oauth",
	}
}

type rewriteTransport struct {
	base http.RoundTripper
	url  string
}

func (t *rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	rewritten := request.Clone(request.Context())
	rewritten.URL.Scheme = "http"
	rewritten.URL.Host = strings.TrimPrefix(t.url, "http://")

	rewritten.Host = ""
	if t.base == nil {
		response, err := http.DefaultTransport.RoundTrip(rewritten)
		if err != nil {
			return nil, fmt.Errorf("rewrite test request: %w", err)
		}

		return response, nil
	}

	response, err := t.base.RoundTrip(rewritten)
	if err != nil {
		return nil, fmt.Errorf("rewrite test request: %w", err)
	}

	return response, nil
}

func TestGetMessageReturnsLabelIDsWithoutFabricatedNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gmail/v1/users/me/messages/m1" {
			t.Errorf("unexpected request %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)

			return
		}
		_, _ = w.Write([]byte(`{"id":"m1","threadId":"t1","labelIds":["Label_1","INBOX"],"payload":{"headers":[{"name":"Subject","value":"Labels"}]}}`))
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{url: server.URL}}
	operation := Operations(provider)[1]

	call, err := operation.Decode(json.RawMessage(`{"account_id":"acct","message_id":"m1","include_body":false}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	result := resultAny.(mcpcontract.Result[MessageView])
	if strings.Join(result.Data.LabelIDs, ",") != "Label_1,INBOX" {
		t.Fatalf("label IDs = %#v", result.Data.LabelIDs)
	}

	if result.Data.LabelNames != nil {
		t.Fatalf("label names fabricated without metadata: %#v", result.Data.LabelNames)
	}
}

func TestSearchUsesLabelCacheForWarmRequests(t *testing.T) {
	var labelRequests int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			_, _ = w.Write([]byte(`{"messages":[]}`))
		case "/gmail/v1/users/me/labels":
			labelRequests++
			_, _ = w.Write([]byte(`{"labels":[{"id":"INBOX","name":"Inbox"}]}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{url: server.URL}}
	operation := Operations(provider)[0]

	call, err := operation.Decode(json.RawMessage(`{"account_id":"acct","query":"all"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	firstAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatalf("cold run: %v", err)
	}

	secondAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatalf("warm run: %v", err)
	}

	first := firstAny.(mcpcontract.Result[SearchData])
	second := secondAny.(mcpcontract.Result[SearchData])

	if labelRequests != 1 {
		t.Fatalf("label requests = %d, want 1", labelRequests)
	}

	if !first.Data.LabelsFetchedAt.Equal(second.Data.LabelsFetchedAt) {
		t.Fatalf("warm labels_fetched_at changed: %q -> %q", first.Data.LabelsFetchedAt, second.Data.LabelsFetchedAt)
	}

	if len(second.Data.Messages) != 0 || second.Data.Messages == nil {
		t.Fatalf("warm messages = %#v, want empty non-nil slice", second.Data.Messages)
	}
}

func TestSearchDoesNotCacheFailedLabelRequests(t *testing.T) {
	var labelRequests int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			_, _ = w.Write([]byte(`{"messages":[]}`))
		case "/gmail/v1/users/me/labels":
			labelRequests++

			http.Error(w, "unavailable", http.StatusInternalServerError)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{url: server.URL}}
	operation := Operations(provider)[0]

	call, err := operation.Decode(json.RawMessage(`{"account_id":"acct","query":"all"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	for run := 0; run < 2; run++ {
		_, err := call.Run(context.Background(), testIdentity())
		if err == nil {
			t.Fatalf("run %d unexpectedly succeeded", run)
		}
	}

	if labelRequests != 2 {
		t.Fatalf("label requests = %d, want one failed request per run", labelRequests)
	}
}

func TestSearchTerminalErrorCancelsBlockedSiblingRequest(t *testing.T) {
	m1Started := make(chan struct{})
	m1Canceled := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1"},{"id":"m2"}]}`))
		case "/gmail/v1/users/me/labels":
			_, _ = w.Write([]byte(`{"labels":[]}`))
		case "/gmail/v1/users/me/messages/m1":
			close(m1Started)
			<-r.Context().Done()
			close(m1Canceled)
		case "/gmail/v1/users/me/messages/m2":
			select {
			case <-m1Started:
			case <-r.Context().Done():
				t.Error("m2 finished before m1 started")
			}

			http.Error(w, "missing", http.StatusNotFound)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{url: server.URL}}

	call, err := Operations(provider)[0].Decode(json.RawMessage(`{"account_id":"acct","query":"all"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	_, err = call.Run(context.Background(), testIdentity())
	if err == nil {
		t.Fatal("expected whole-page failure")
	}

	var contractErr *mcpcontract.Error
	if !errors.As(err, &contractErr) || contractErr.Category != mcpcontract.NotFound {
		t.Fatalf("error = %v, want typed not_found", err)
	}

	select {
	case <-m1Canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("terminal m2 error did not cancel blocked m1 request")
	}
}

func TestSearchLabelNamesOmitUnknownLabelIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1","threadId":"t1"}]}`))
		case "/gmail/v1/users/me/labels":
			_, _ = w.Write([]byte(`{"labels":[{"id":"INBOX","name":"Inbox"},{"id":"BLANK","name":""}]}`))
		case "/gmail/v1/users/me/messages/m1":
			_, _ = w.Write([]byte(`{"id":"m1","threadId":"t1","labelIds":["INBOX","BLANK","MISSING"],"payload":{"headers":[{"name":"Subject","value":"Labels"}]}}`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{url: server.URL}}

	call, err := Operations(provider)[0].Decode(json.RawMessage(`{"account_id":"acct","query":"all"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	result := resultAny.(mcpcontract.Result[SearchData])

	labels := result.Data.Messages[0]
	if strings.Join(labels.LabelIDs, ",") != "INBOX,BLANK,MISSING" {
		t.Fatalf("label IDs = %#v", labels.LabelIDs)
	}

	if strings.Join(labels.LabelNames, ",") != "Inbox" {
		t.Fatalf("label names = %#v, want known name only", labels.LabelNames)
	}
}

func TestGetMessageIncludesInlineAndExternalAttachmentsWithoutSelectedBody(t *testing.T) {
	inlineData := base64.RawURLEncoding.EncodeToString([]byte("inline attachment"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages/m1":
			_, _ = fmt.Fprintf(w, `{"id":"m1","threadId":"t1","payload":{"mimeType":"multipart/mixed","parts":[{"partId":"0","mimeType":"text/plain","headers":[{"name":"Content-Type","value":"text/plain; charset=utf-8"}],"body":{"attachmentId":"body-a1","size":13}},{"partId":"1","mimeType":"text/plain","filename":"notes.txt","headers":[{"name":"Content-Disposition","value":"attachment"}],"body":{"data":%q,"size":17}},{"partId":"2","mimeType":"application/pdf","filename":"brief.pdf","body":{"attachmentId":"a2","size":123}}]}}`, inlineData)
		case "/gmail/v1/users/me/messages/m1/attachments/body-a1":
			_, _ = fmt.Fprintf(w, `{"size":13,"data":%q}`, base64.RawURLEncoding.EncodeToString([]byte("external body")))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := &recordingProvider{}
	provider.client = &http.Client{Transport: &rewriteTransport{url: server.URL}}

	call, err := Operations(provider)[1].Decode(json.RawMessage(`{"account_id":"acct","message_id":"m1"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	resultAny, err := call.Run(context.Background(), testIdentity())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	result := resultAny.(mcpcontract.Result[MessageView])
	if result.Data.Body != "external body" {
		t.Fatalf("selected body = %q", result.Data.Body)
	}

	if len(result.Data.Attachments) != 2 {
		t.Fatalf("attachments = %#v, want inline and external attachment", result.Data.Attachments)
	}

	inline := result.Data.Attachments[0]
	if inline.PartID != "1" || inline.AttachmentID != "" || inline.Filename != "notes.txt" || inline.SizeBytes != 17 {
		t.Fatalf("inline attachment = %#v", inline)
	}

	external := result.Data.Attachments[1]
	if external.PartID != "2" || external.AttachmentID != "a2" || external.Filename != "brief.pdf" || external.SizeBytes != 123 {
		t.Fatalf("external attachment = %#v", external)
	}
}
