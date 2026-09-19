package gmail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestBestBodyPartPrefersPlainAndFallsBackToHTML(t *testing.T) {
	plain := base64.RawURLEncoding.EncodeToString([]byte("plain alternative"))
	html := base64.RawURLEncoding.EncodeToString([]byte("<p>html alternative</p>"))
	multipart := &gmail.MessagePart{
		MimeType: "multipart/alternative",
		Parts: []*gmail.MessagePart{
			{MimeType: "text/plain", Body: &gmail.MessagePartBody{Data: plain}},
			{MimeType: "text/html", Body: &gmail.MessagePartBody{Data: html}},
		},
	}

	part, isHTML, found, err := bestBodyPart(multipart)
	if err != nil || !found {
		t.Fatalf("bestBodyPart: found=%v err=%v", found, err)
	}

	if got := mustDecodeBody(t, part); got != "plain alternative" || isHTML {
		t.Fatalf("plain selection = %q html=%v", got, isHTML)
	}

	multipart.Parts = multipart.Parts[1:]

	part, isHTML, found, err = bestBodyPart(multipart)
	if err != nil || !found {
		t.Fatalf("bestBodyPart html fallback: found=%v err=%v", found, err)
	}

	if got := mustDecodeBody(t, part); got != "<p>html alternative</p>" || !isHTML {
		t.Fatalf("html selection = %q html=%v", got, isHTML)
	}
}

func TestBestBodyPartExcludesTextAttachments(t *testing.T) {
	attachment := base64.RawURLEncoding.EncodeToString([]byte("attachment"))
	html := base64.RawURLEncoding.EncodeToString([]byte("<p>main body</p>"))
	payload := &gmail.MessagePart{
		MimeType: "multipart/mixed",
		Parts: []*gmail.MessagePart{
			{
				MimeType: "multipart/alternative",
				Parts: []*gmail.MessagePart{
					{
						MimeType: "text/plain",
						Headers: []*gmail.MessagePartHeader{
							{Name: "Content-Disposition", Value: "attachment"},
						},
						Filename: "notes.txt",
						Body:     &gmail.MessagePartBody{Data: attachment},
					},
					{MimeType: "text/html", Body: &gmail.MessagePartBody{Data: html}},
				},
			},
		},
	}

	api := newBodyTestService(t, nil)

	body, truncated, bodyAttachmentID, err := boundedBody(context.Background(), api, "m1", payload, 64)
	if err != nil {
		t.Fatalf("boundedBody: %v", err)
	}

	if body != "main body" || truncated || bodyAttachmentID != "" {
		t.Fatalf("body = %q truncated=%v selected_attachment=%q", body, truncated, bodyAttachmentID)
	}
}

func TestBoundedBodyFetchesExternalNonAttachmentBody(t *testing.T) {
	payload := &gmail.MessagePart{
		MimeType: "text/plain",
		Headers: []*gmail.MessagePartHeader{
			{Name: "Content-Type", Value: "text/plain; charset=utf-8"},
		},
		Body: &gmail.MessagePartBody{AttachmentId: "body-a1", Size: 13},
	}

	tests := []struct {
		name      string
		limit     int
		want      string
		truncated bool
	}{
		{name: "full", limit: 1024, want: "external body"},
		{name: "bounded", limit: 8, want: "external", truncated: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var paths []string
			api := newBodyTestService(t, func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.URL.Path != "/gmail/v1/users/me/messages/m1/attachments/body-a1" {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				_, _ = fmt.Fprintf(w, `{"size":13,"data":%q}`, base64.RawURLEncoding.EncodeToString([]byte("external body")))
			})

			body, truncated, bodyAttachmentID, err := boundedBody(context.Background(), api, "m1", payload, test.limit)
			if err != nil {
				t.Fatalf("boundedBody: %v", err)
			}

			if body != test.want || truncated != test.truncated || bodyAttachmentID != "body-a1" {
				t.Fatalf("body = %q truncated=%v selected_attachment=%q", body, truncated, bodyAttachmentID)
			}

			if len(paths) != 1 {
				t.Fatalf("attachment requests = %v", paths)
			}

			if attachments, _ := attachmentViews(payload, bodyAttachmentID); len(attachments) != 0 {
				t.Fatalf("external body appeared in attachments: %#v", attachments)
			}
		})
	}
}

func TestBoundedBodyDoesNotFetchExternalBodyAboveHardCap(t *testing.T) {
	payload := &gmail.MessagePart{
		MimeType: "text/plain",
		Body:     &gmail.MessagePartBody{AttachmentId: "body-a1", Size: maxBodyBytes + 1},
	}
	api := newBodyTestService(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected fetch %s", r.URL.Path)
	})

	body, truncated, bodyAttachmentID, err := boundedBody(context.Background(), api, "m1", payload, maxBodyBytes)
	if err != nil {
		t.Fatalf("boundedBody: %v", err)
	}

	if body != "" || !truncated || bodyAttachmentID != "body-a1" {
		t.Fatalf("body = %q truncated=%v selected_attachment=%q", body, truncated, bodyAttachmentID)
	}
}

func TestBoundedBodyPreservesTransferEncodedLiteralText(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		transfer string
		bodySize int64
	}{
		{name: "quoted-printable equals escape", raw: "a=3Db", transfer: "quoted-printable", bodySize: 3},
		{name: "literal quoted-printable-like text", raw: "code=20", transfer: "quoted-printable", bodySize: 7},
		{name: "literal base64-like text", raw: "aGVsbG8=", transfer: "base64", bodySize: 5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := &gmail.MessagePart{
				MimeType: "text/plain",
				Headers: []*gmail.MessagePartHeader{
					{Name: "Content-Type", Value: "text/plain; charset=utf-8"},
					{Name: "Content-Transfer-Encoding", Value: test.transfer},
				},
				Body: &gmail.MessagePartBody{
					Data: base64.RawURLEncoding.EncodeToString([]byte(test.raw)),
					Size: test.bodySize,
				},
			}

			api := newBodyTestService(t, nil)

			body, truncated, _, err := boundedBody(context.Background(), api, "m1", payload, 64)
			if err != nil {
				t.Fatalf("boundedBody: %v", err)
			}

			if body != test.raw || truncated {
				t.Fatalf("body = %q truncated=%v, want literal %q", body, truncated, test.raw)
			}
		})
	}
}

func TestBoundedBodyAppliesDeclaredCharsetAfterGmailBase64URL(t *testing.T) {
	payload := &gmail.MessagePart{
		MimeType: "text/plain",
		Headers: []*gmail.MessagePartHeader{
			{Name: "Content-Type", Value: "text/plain; charset=iso-8859-1"},
			{Name: "Content-Transfer-Encoding", Value: "quoted-printable"},
		},
		Body: &gmail.MessagePartBody{
			Data: base64.RawURLEncoding.EncodeToString([]byte("caf\xe9")),
			Size: 4,
		},
	}

	api := newBodyTestService(t, nil)

	body, truncated, _, err := boundedBody(context.Background(), api, "m1", payload, 64)
	if err != nil {
		t.Fatalf("boundedBody: %v", err)
	}

	if body != "café" || truncated {
		t.Fatalf("body = %q truncated=%v, want café", body, truncated)
	}
}

func TestBoundedBodyConvertsHTMLAndStripsScripts(t *testing.T) {
	source := "<html><body><script>secret()</script><style>a{}</style><p>Hello <b>world</b></body></html>"
	payload := &gmail.MessagePart{
		MimeType: "text/html",
		Headers:  []*gmail.MessagePartHeader{{Name: "Content-Type", Value: "text/html; charset=utf-8"}},
		Body:     &gmail.MessagePartBody{Data: base64.RawURLEncoding.EncodeToString([]byte(source))},
	}

	api := newBodyTestService(t, nil)

	body, truncated, _, err := boundedBody(context.Background(), api, "m1", payload, 128)
	if err != nil {
		t.Fatalf("boundedBody: %v", err)
	}

	if body != "Hello world" || truncated {
		t.Fatalf("body = %q truncated=%v", body, truncated)
	}
}

func TestBoundedBodyReturnsTypedDecodeErrors(t *testing.T) {
	tests := []struct {
		name    string
		payload *gmail.MessagePart
	}{
		{
			name: "invalid Gmail base64",
			payload: &gmail.MessagePart{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: "not-base64!!"},
			},
		},
		{
			name: "malformed content type",
			payload: &gmail.MessagePart{
				MimeType: "text/plain",
				Headers:  []*gmail.MessagePartHeader{{Name: "Content-Type", Value: "text/plain; charset="}},
				Body:     &gmail.MessagePartBody{Data: base64.RawURLEncoding.EncodeToString([]byte("body"))},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := newBodyTestService(t, nil)

			_, _, _, err := boundedBody(context.Background(), api, "m1", test.payload, 64)
			if err == nil {
				t.Fatal("expected decode error")
			}

			var contractErr *mcpcontract.Error
			if !errors.As(err, &contractErr) || contractErr.Category != mcpcontract.UpstreamFailure {
				t.Fatalf("error = %v, want typed upstream_failure", err)
			}
		})
	}
}

func TestDecodeHeaderValueUsesEncodedWordCharset(t *testing.T) {
	payload := &gmail.MessagePart{Headers: []*gmail.MessagePartHeader{
		{Name: "Subject", Value: "=?iso-8859-1?q?caf=E9_report?="},
	}}
	if got := decodeHeaderValue(headerValue(payload, "Subject")); got != "café report" {
		t.Fatalf("decoded header = %q", got)
	}
}

func TestBoundedBytesDoesNotSplitRune(t *testing.T) {
	value := strings.Repeat("é", 8)

	bounded, truncated := boundedBytes(value, 5)
	if bounded != "éé" || !truncated {
		t.Fatalf("bounded = %q truncated=%v", bounded, truncated)
	}
}

func mustDecodeBody(t *testing.T, part *gmail.MessagePart) string {
	t.Helper()

	text, err := decodePartBody(part)
	if err != nil {
		t.Fatalf("decodePartBody: %v", err)
	}

	return text
}

func newBodyTestService(t *testing.T, handler http.HandlerFunc) *gmail.Service {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		handler(w, r)
	}))
	t.Cleanup(server.Close)

	service, err := gmail.NewService(context.Background(), option.WithHTTPClient(&http.Client{Transport: &rewriteTransport{url: server.URL}}))
	if err != nil {
		t.Fatalf("gmail service: %v", err)
	}

	return service
}
