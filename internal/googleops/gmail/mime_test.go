package gmail

import (
	"encoding/base64"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestBestBodyTextPrefersPlainAndFallsBackToHTML(t *testing.T) {
	plain := base64.RawURLEncoding.EncodeToString([]byte("plain alternative"))
	html := base64.RawURLEncoding.EncodeToString([]byte("<p>html alternative</p>"))
	multipart := &gmail.MessagePart{
		MimeType: "multipart/alternative",
		Parts: []*gmail.MessagePart{
			{MimeType: "text/plain", Body: &gmail.MessagePartBody{Data: plain}},
			{MimeType: "text/html", Body: &gmail.MessagePartBody{Data: html}},
		},
	}

	text, isHTML, err := bestBodyText(multipart)
	if err != nil {
		t.Fatalf("bestBodyText: %v", err)
	}

	if text != "plain alternative" || isHTML {
		t.Fatalf("plain selection = %q html=%v", text, isHTML)
	}

	multipart.Parts = multipart.Parts[1:]

	text, isHTML, err = bestBodyText(multipart)
	if err != nil {
		t.Fatalf("bestBodyText html fallback: %v", err)
	}

	if text != "<p>html alternative</p>" || !isHTML {
		t.Fatalf("html selection = %q html=%v", text, isHTML)
	}
}

func TestBoundedBodyDecodesQuotedPrintableAndCharset(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte("caf=E9"))
	payload := &gmail.MessagePart{
		MimeType: "text/plain",
		Headers: []*gmail.MessagePartHeader{
			{Name: "Content-Type", Value: "text/plain; charset=iso-8859-1"},
			{Name: "Content-Transfer-Encoding", Value: "quoted-printable"},
		},
		Body: &gmail.MessagePartBody{Data: encoded},
	}

	body, truncated, err := boundedBody(payload, 32)
	if err != nil {
		t.Fatalf("boundedBody: %v", err)
	}

	if body != "café" || truncated {
		t.Fatalf("body = %q truncated=%v", body, truncated)
	}
}

func TestBoundedBodyDecodesNestedBase64TransferEncoding(t *testing.T) {
	inner := base64.StdEncoding.EncodeToString([]byte("transfer encoded body"))
	encoded := base64.RawURLEncoding.EncodeToString([]byte(inner))
	payload := &gmail.MessagePart{
		MimeType: "text/plain",
		Headers: []*gmail.MessagePartHeader{
			{Name: "Content-Type", Value: "text/plain; charset=utf-8"},
			{Name: "Content-Transfer-Encoding", Value: "base64"},
		},
		Body: &gmail.MessagePartBody{Data: encoded},
	}

	body, _, err := boundedBody(payload, 128)
	if err != nil {
		t.Fatalf("boundedBody: %v", err)
	}

	if body != "transfer encoded body" {
		t.Fatalf("body = %q", body)
	}
}

func TestBoundedBodyConvertsHTMLAndStripsScripts(t *testing.T) {
	source := "<html><body><script>secret()</script><style>a{}</style><p>Hello <b>world</b></body></html>"
	encoded := base64.RawURLEncoding.EncodeToString([]byte(source))
	payload := &gmail.MessagePart{
		MimeType: "text/html",
		Headers:  []*gmail.MessagePartHeader{{Name: "Content-Type", Value: "text/html; charset=utf-8"}},
		Body:     &gmail.MessagePartBody{Data: encoded},
	}

	body, truncated, err := boundedBody(payload, 128)
	if err != nil {
		t.Fatalf("boundedBody: %v", err)
	}

	if body != "Hello world" || truncated {
		t.Fatalf("body = %q truncated=%v", body, truncated)
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

func TestBoundedBodyReturnsDecodeError(t *testing.T) {
	payload := &gmail.MessagePart{
		MimeType: "text/plain",
		Body:     &gmail.MessagePartBody{Data: "not-base64!!"},
	}
	if _, _, err := boundedBody(payload, 10); err == nil {
		t.Fatal("expected base64 decode error")
	}
}
