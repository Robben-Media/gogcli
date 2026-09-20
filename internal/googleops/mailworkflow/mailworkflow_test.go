package mailworkflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"testing"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"

	native "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mailcompose"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

type fixture struct {
	options    []mcpcontract.CallOptions
	identities []mcpcontract.Identity
	transport  *fakeTransport
}

type fakeTransport struct {
	signature          string
	verificationStatus string
	customAlias        bool
	writeResponse      string
	failurePath        string
	paths              []string
	requests           []map[string]any
	writeBody          []byte
}

func (f *fixture) HTTPClient(_ context.Context, id mcpcontract.Identity, opts mcpcontract.CallOptions) (*http.Client, error) {
	def, ok := mcpcontract.Lookup(opts.Operation)
	if !ok {
		return nil, invalid("unknown operation")
	}

	if opts.Retry != def.Retry {
		return nil, invalid("retry class does not match catalog")
	}

	f.options = append(f.options, opts)
	f.identities = append(f.identities, id)

	return &http.Client{Transport: &native.NativeRetryTransport{
		Base:  f.transport,
		Class: opts.Retry,
	}}, nil
}

func (t *fakeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var payload map[string]any

	if request.Body != nil {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, fmt.Errorf("read fixture request: %w", err)
		}

		if len(body) != 0 && json.Unmarshal(body, &payload) == nil {
			t.requests = append(t.requests, payload)
		}
	}

	t.paths = append(t.paths, request.URL.Path)
	if t.failurePath != "" && strings.HasSuffix(request.URL.Path, t.failurePath) {
		return &http.Response{
			Status:     http.StatusText(http.StatusInternalServerError),
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":500}}`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	}
	response := "{}"

	switch {
	case strings.HasSuffix(request.URL.Path, "/settings/sendAs"):
		response = fmt.Sprintf(`{"sendAs":[{"sendAsEmail":"alice@example.com","displayName":"Alice Example","isPrimary":%t,"verificationStatus":%q,"signature":%q}]}`, !t.customAlias, t.verificationStatus, t.signature)
	case strings.HasPrefix(request.URL.Path, "/gmail/v1/users/me/messages/") && request.URL.Path != "/gmail/v1/users/me/messages/send":
		response = `{"id":"source-message","threadId":"thread-1","payload":{"headers":[
			{"name":"Message-ID","value":"<original@mail.example>"},
			{"name":"References","value":"<first@mail.example>"},
			{"name":"Subject","value":"Project update"}
		]}}`
	case strings.HasSuffix(request.URL.Path, "/drafts"):
		raw, err := requestRaw(payload)
		if err != nil {
			return nil, err
		}
		t.writeBody = raw
		response = `{"id":"draft-1","message":{"id":"message-1","threadId":"thread-1"}}`
	case request.URL.Path == "/gmail/v1/users/me/messages/send":
		raw, err := requestRaw(payload)
		if err != nil {
			return nil, err
		}
		t.writeBody = raw
		response = `{"id":"message-1","threadId":"thread-1"}`
	}

	return &http.Response{
		Status:     http.StatusText(http.StatusOK),
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(t.writeResponseValue(response))),
		Header:     make(http.Header),
		Request:    request,
	}, nil
}

func (t *fakeTransport) writeResponseValue(value string) string {
	writePath := len(t.paths) != 0 && (strings.HasSuffix(t.paths[len(t.paths)-1], "/drafts") || t.paths[len(t.paths)-1] == "/gmail/v1/users/me/messages/send")
	if t.writeResponse != "" && writePath {
		return t.writeResponse
	}

	return value
}

func requestRaw(payload map[string]any) ([]byte, error) {
	message, _ := payload["message"].(map[string]any)

	raw, _ := message["raw"].(string)
	if raw == "" {
		raw, _ = payload["raw"].(string)
	}

	if raw == "" {
		return nil, invalid("request contained no raw message")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode fixture raw message: %w", err)
	}

	return decoded, nil
}

func validRequest() EmailRequest {
	return EmailRequest{
		Selection: mcpcontract.Selection{AccountID: "account-1"},
		To:        []string{"Bob Example <bob@example.com>"},
		Subject:   "Project update",
		Content: Content{
			Format: mailcompose.FormatPlainAndHTML,
			Plain:  "Hello",
			HTML:   `<p onclick="evil()">Hello</p>`,
		},
		IncludeSignature: true,
	}
}

func newFixture(signature string) *fixture {
	return &fixture{transport: &fakeTransport{signature: signature}}
}

func testContext(t *testing.T, calls int64) context.Context {
	t.Helper()
	return native.WithUpstreamBudget(t.Context(), calls)
}

func identity() mcpcontract.Identity {
	return mcpcontract.Identity{AccountID: "account-1", Email: "alice@example.com", PrincipalID: "jeremy"}
}

func requireSafeError(t *testing.T, err error, category mcpcontract.ErrorCategory) {
	t.Helper()

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != category {
		t.Fatalf("error = %v, want category %q", err, category)
	}
}

func TestPrepareVerifiesSenderAndReturnsRenderedPreview(t *testing.T) {
	defer func(value func() time.Time) { now = value }(now)
	now = func() time.Time { return time.Date(2026, time.September, 19, 19, 30, 0, 0, time.UTC) }

	f := newFixture("<p>Alice Example</p>")

	out, err := prepare(testContext(t, 1), f, identity(), validRequest())
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	if out.Data.Sender.Email != "alice@example.com" || !out.Data.Sender.SignatureIncluded {
		t.Fatalf("sender = %#v", out.Data.Sender)
	}

	if len(out.Data.To) != 1 || out.Data.To[0].Email != "bob@example.com" {
		t.Fatalf("recipients = %#v", out.Data.To)
	}

	if !strings.Contains(out.Data.Preview.Plain, "Hello") || !strings.Contains(out.Data.Preview.Plain, "Alice Example") {
		t.Fatalf("plain preview = %q", out.Data.Preview.Plain)
	}

	if !strings.Contains(out.Data.Preview.HTML, "<p>Hello</p>") || strings.Contains(out.Data.Preview.HTML, "onclick") || !strings.Contains(out.Data.Preview.HTML, "Alice Example") {
		t.Fatalf("HTML preview = %q", out.Data.Preview.HTML)
	}

	if len(out.Data.ContentDigest) != digestHexLength || len(f.transport.paths) != 1 || !strings.HasSuffix(f.transport.paths[0], "/settings/sendAs") {
		t.Fatalf("digest = %q, paths = %#v", out.Data.ContentDigest, f.transport.paths)
	}
}

func TestDraftComposesMIMEAndPreservesReplyThread(t *testing.T) {
	defer func(value func() time.Time) { now = value }(now)
	now = func() time.Time { return time.Date(2026, time.September, 19, 19, 30, 0, 0, time.UTC) }

	f := newFixture("<p>Alice Example</p>")
	in := validRequest()
	in.Reply = &ReplyContext{SourceMessageID: "source-message", ThreadID: "thread-1", ExpectedSubject: "Project update"}

	out, err := draft(testContext(t, 3), f, identity(), DraftInput{EmailRequest: in})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}

	if out.Data.DraftID != "draft-1" || out.Data.MessageID != "message-1" || out.Data.ThreadID != "thread-1" {
		t.Fatalf("draft data = %#v", out.Data)
	}

	if len(f.transport.paths) != 3 || !strings.HasSuffix(f.transport.paths[1], "/messages/source-message") || !strings.HasSuffix(f.transport.paths[2], "/drafts") {
		t.Fatalf("paths = %#v", f.transport.paths)
	}

	if len(f.options) != 2 || f.options[0].Operation != "mail_compose_prepare" || f.options[0].Retry != mcpcontract.SafeRead ||
		f.options[1].Operation != "mail_compose_draft" || f.options[1].Retry != mcpcontract.NonReplayableWrite {
		t.Fatalf("client options = %#v", f.options)
	}

	message, err := mail.ReadMessage(bytes.NewReader(f.transport.writeBody))
	if err != nil {
		t.Fatalf("read MIME: %v", err)
	}

	if got := message.Header.Get("From"); !strings.Contains(got, "alice@example.com") || !strings.Contains(got, "Alice Example") {
		t.Fatalf("From = %q", got)
	}

	if got, want := message.Header.Get("In-Reply-To"), "<original@mail.example>"; got != want {
		t.Fatalf("In-Reply-To = %q, want %q", got, want)
	}

	if got := message.Header.Get("References"); !strings.Contains(got, "<first@mail.example>") || !strings.Contains(got, "<original@mail.example>") {
		t.Fatalf("References = %q", got)
	}
}

func TestSendMalformedSuccessfulBodyIsOutcomeUnknown(t *testing.T) {
	f := newFixture("<p>Alice Example</p>")
	f.transport.writeResponse = "not-json"
	_, err := send(testContext(t, 2), f, identity(), SendInput{EmailRequest: validRequest()})
	requireSafeError(t, err, mcpcontract.OutcomeUnknown)

	if len(f.transport.paths) != 2 || len(f.transport.writeBody) == 0 {
		t.Fatalf("paths = %#v, write bytes = %d", f.transport.paths, len(f.transport.writeBody))
	}

	if len(f.options) != 2 || f.options[0].Operation != "mail_compose_prepare" || f.options[1].Operation != "mail_compose_send" ||
		f.options[0].Retry != mcpcontract.SafeRead || f.options[1].Retry != mcpcontract.NonReplayableWrite {
		t.Fatalf("client options = %#v", f.options)
	}
}

func TestPreflightReadFailureIsNotOutcomeUnknown(t *testing.T) {
	f := newFixture("<p>Alice Example</p>")
	f.transport.failurePath = "/settings/sendAs"
	_, err := send(testContext(t, 2), f, identity(), SendInput{EmailRequest: validRequest()})
	requireSafeError(t, err, mcpcontract.UpstreamFailure)

	if len(f.transport.paths) != 2 || !strings.HasSuffix(f.transport.paths[0], "/settings/sendAs") {
		t.Fatalf("paths = %#v", f.transport.paths)
	}

	f = newFixture("<p>Alice Example</p>")
	f.transport.failurePath = "/messages/source-message"
	in := validRequest()
	in.Reply = &ReplyContext{SourceMessageID: "source-message", ThreadID: "thread-1", ExpectedSubject: "Project update"}
	_, err = draft(testContext(t, 3), f, identity(), DraftInput{EmailRequest: in})
	requireSafeError(t, err, mcpcontract.UpstreamFailure)

	if len(f.transport.paths) != 3 || !strings.HasSuffix(f.transport.paths[1], "/messages/source-message") || !strings.HasSuffix(f.transport.paths[2], "/messages/source-message") {
		t.Fatalf("reply paths = %#v", f.transport.paths)
	}
}

func TestSemanticInputFailsBeforeProvider(t *testing.T) {
	f := newFixture("")
	cases := []string{
		`{"account_id":"account-1","to":[],"subject":"Project update","content":{"format":"plain","plain":"Hello"}}`,
		`{"account_id":"account-1","to":["same@example.com"],"cc":["Same@Example.com"],"subject":"Project update","content":{"format":"plain","plain":"Hello"}}`,
	}

	for _, raw := range cases {
		for _, operation := range Operations(f) {
			if _, err := operation.Decode(json.RawMessage(raw)); err == nil {
				t.Fatalf("%s accepted invalid semantic input", operation.Definition.Name)
			}
		}
	}

	if len(f.identities) != 0 {
		t.Fatalf("invalid operations reached provider: %#v", f.identities)
	}
}

func TestWorkflowMapsResidualComposeValidation(t *testing.T) {
	in := validRequest()
	in.To = []string{strings.Repeat("a", 1024-len("@example.com")) + "@example.com"}
	_, err := compose(in, &gmailapi.SendAs{SendAsEmail: "alice@example.com", DisplayName: "Alice", IsPrimary: true}, mailcompose.Signature{}, nil)
	requireSafeError(t, err, mcpcontract.InvalidInput)
}

func TestStaleSignatureDigestRejectsWriteBeforeMutation(t *testing.T) {
	f := newFixture("<p>Alice Example</p>")

	prepared, err := prepare(testContext(t, 1), f, identity(), validRequest())
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	f.transport.signature = "<p>New Alice</p>"
	f.transport.customAlias = true
	f.transport.verificationStatus = "accepted"
	_, err = send(testContext(t, 2), f, identity(), SendInput{
		EmailRequest:          validRequest(),
		ExpectedContentDigest: prepared.Data.ContentDigest,
	})
	requireSafeError(t, err, mcpcontract.InvalidInput)

	if len(f.transport.paths) != 2 || strings.HasSuffix(f.transport.paths[1], "/messages/send") {
		t.Fatalf("paths = %#v", f.transport.paths)
	}
}

func TestUnverifiedSenderIsRejected(t *testing.T) {
	f := newFixture("")
	f.transport.verificationStatus = "pending"
	f.transport.customAlias = true
	in := validRequest()
	in.From = "alice@example.com"
	in.IncludeSignature = false
	_, err := prepare(testContext(t, 1), f, identity(), in)
	requireSafeError(t, err, mcpcontract.Forbidden)
}

func TestBudgetAndInputLimitsRejectBeforeProvider(t *testing.T) {
	f := newFixture("")
	in := validRequest()
	in.Attachments = []Attachment{{Filename: "large.bin", ContentType: "application/octet-stream", Data: bytes.Repeat([]byte{0}, mailcompose.DefaultLimits().MaxAttachmentBytes+1)}}
	_, err := draft(context.Background(), f, identity(), DraftInput{EmailRequest: in})
	requireSafeError(t, err, mcpcontract.InvalidInput)

	limited := newFixture("<p>Alice Example</p>")
	_, err = draft(testContext(t, 1), limited, identity(), DraftInput{EmailRequest: validRequest()})
	requireSafeError(t, err, mcpcontract.BudgetExhausted)

	if len(f.identities) != 0 || len(limited.identities) != 0 {
		t.Fatalf("invalid requests reached provider: %#v %#v", f.identities, limited.identities)
	}
}

func TestOperationsRejectInvalidArgumentsBeforeProvider(t *testing.T) {
	f := newFixture("")
	for _, operation := range Operations(f) {
		raw := `{"account_id":"account-1","to":["broken"],"subject":"Missing"}`
		if _, err := operation.Decode(json.RawMessage(raw)); err == nil {
			t.Fatalf("%s accepted invalid input", operation.Definition.Name)
		}
	}

	if len(f.identities) != 0 {
		t.Fatalf("invalid operations reached provider: %#v", f.identities)
	}
}
