package mailcompose

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

var testDate = time.Date(2026, time.September, 19, 14, 30, 0, 0, time.FixedZone("Chicago", -5*60*60))

func validInput() Input {
	return Input{
		From:    "Alice Example <Alice@Example.COM>",
		To:      []string{"Bob Example <Bob@Example.com>"},
		Subject: "Project update",
		Content: Content{
			Format: FormatPlainWithHTML,
			Plain:  "Hello\nWorld",
		},
		Date: testDate,
	}
}

type messagePart struct {
	Headers     textproto.MIMEHeader
	ContentType string
	Params      map[string]string
	Body        []byte
}

func compose(t *testing.T, in Input) Result {
	t.Helper()

	result, err := Compose(in)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}

	return result
}

func parseMessageParts(t *testing.T, raw []byte) []messagePart {
	t.Helper()

	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("mail.ReadMessage: %v", err)
	}

	return parseParts(t, textproto.MIMEHeader(message.Header), message.Body)
}

func parseParts(t *testing.T, header textproto.MIMEHeader, body io.Reader) []messagePart {
	t.Helper()
	contentType := header.Get("Content-Type")

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("mime.ParseMediaType(%q): %v", contentType, err)
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(body, params["boundary"])
		var parts []messagePart

		for {
			part, nextErr := reader.NextPart()
			if errors.Is(nextErr, io.EOF) {
				return parts
			}

			if nextErr != nil {
				t.Fatalf("multipart.NextPart: %v", nextErr)
			}

			parts = append(parts, parseParts(t, part.Header, part)...)
		}
	}

	partBody, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("io.ReadAll(%s): %v", mediaType, err)
	}

	return []messagePart{{
		Headers: header, ContentType: mediaType, Params: params, Body: partBody,
	}}
}

func decodedBody(t *testing.T, part messagePart) string {
	t.Helper()

	switch part.Headers.Get("Content-Transfer-Encoding") {
	case "quoted-printable":
		decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(part.Body)))
		if err != nil {
			t.Fatalf("decode quoted-printable: %v", err)
		}

		return string(decoded)
	case "base64":
		decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(part.Body)))
		if err != nil {
			t.Fatalf("decode base64: %v", err)
		}

		return string(decoded)
	default:
		return string(part.Body)
	}
}

func findParts(parts []messagePart, contentType string) []messagePart {
	var found []messagePart

	for _, part := range parts {
		if part.ContentType == contentType {
			found = append(found, part)
		}
	}

	return found
}

func TestComposeIsDeterministicAndGeneratesHTMLAlternative(t *testing.T) {
	first := compose(t, validInput())

	second, err := Compose(validInput())
	if err != nil {
		t.Fatalf("Compose repeat: %v", err)
	}

	if !bytes.Equal(first.Raw, second.Raw) || first.Base64URL != second.Base64URL {
		t.Fatalf("composition was not deterministic")
	}
	parts := parseMessageParts(t, first.Raw)
	plainParts := findParts(parts, "text/plain")

	htmlParts := findParts(parts, "text/html")
	if len(plainParts) != 1 || len(htmlParts) != 1 {
		t.Fatalf("expected one plain and one HTML part, got %d and %d", len(plainParts), len(htmlParts))
	}

	if got, want := decodedBody(t, plainParts[0]), "Hello\r\nWorld"; got != want {
		t.Fatalf("plain body = %q, want %q", got, want)
	}

	if got, want := decodedBody(t, htmlParts[0]), "<p>Hello<br>World</p>"; got != want {
		t.Fatalf("HTML body = %q, want %q", got, want)
	}

	if got := first.Summary.Date.Format(time.RFC1123Z); got != "Sat, 19 Sep 2026 19:30:00 +0000" {
		t.Fatalf("normalized date = %q", got)
	}
}

func TestComposeAddsExplicitSignatureAndSanitizesHTML(t *testing.T) {
	in := validInput()
	in.Content = Content{
		Format: FormatPlainAndHTML,
		Plain:  "Hello",
		HTML:   `<p onclick="alert(1)">Hi</p><script>alert(2)</script><iframe src="https://attacker.example"></iframe><img src="https://tracking.example/pixel" alt="Logo"><a href="javascript:alert(3)">bad</a><a href="https://example.com">good</a>`,
	}
	in.Signature = Signature{
		Plain: "Alice\nRobben Media",
		HTML:  `<p>Alice<br>Robben Media</p>`,
	}
	in.IncludeSignature = true
	result := compose(t, in)

	parts := parseMessageParts(t, result.Raw)
	plain := findParts(parts, "text/plain")

	htmlParts := findParts(parts, "text/html")
	if len(plain) != 1 || len(htmlParts) != 1 {
		t.Fatalf("expected alternative parts, got %d plain and %d HTML", len(plain), len(htmlParts))
	}
	plainText := decodedBody(t, plain[0])

	htmlText := decodedBody(t, htmlParts[0])
	if strings.Count(plainText, "Robben Media") != 1 || strings.Count(htmlText, "Robben Media") != 1 {
		t.Fatalf("signature was not added exactly once: plain %q, HTML %q", plainText, htmlText)
	}

	for _, forbidden := range []string{"<script", "<iframe", "onclick", "https://tracking.example", "javascript:"} {
		if strings.Contains(htmlText, forbidden) {
			t.Fatalf("HTML contains forbidden content %q: %s", forbidden, htmlText)
		}
	}

	if !strings.Contains(htmlText, `<a href="https://example.com">good</a>`) {
		t.Fatalf("safe link was removed: %s", htmlText)
	}

	if !result.Summary.SignatureIncluded {
		t.Fatalf("summary did not report signature")
	}

	if len(result.Summary.Warnings) == 0 {
		t.Fatalf("expected sanitization warnings")
	}
}

func TestComposeHTMLAlwaysHasPlainFallback(t *testing.T) {
	in := validInput()
	in.Content = Content{
		Format: FormatHTML,
		HTML:   `<p>Hello<br>World</p><a href="https://example.com">Example</a>`,
	}
	in.Signature.HTML = `<p>Alice</p>`
	in.IncludeSignature = true
	result := compose(t, in)
	parts := parseMessageParts(t, result.Raw)
	t.Logf("raw=%q parts=%#v", result.Raw, parts)
	plainParts := findParts(parts, "text/plain")

	htmlParts := findParts(parts, "text/html")
	if len(plainParts) != 1 || len(htmlParts) != 1 {
		t.Fatalf("expected generated plain fallback, got %d plain and %d HTML", len(plainParts), len(htmlParts))
	}

	plain := decodedBody(t, plainParts[0])
	for _, want := range []string{"Hello", "World", "Example (https://example.com)", "Alice"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("plain fallback missing %q: %q", want, plain)
		}
	}
}

func TestComposePlainOnlyStaysPlain(t *testing.T) {
	in := validInput()
	in.Content = Content{Format: FormatPlain, Plain: "Hello"}
	result := compose(t, in)

	parts := parseMessageParts(t, result.Raw)
	if len(parts) != 1 || parts[0].ContentType != "text/plain" {
		t.Fatalf("plain-only composition produced %#v", parts)
	}
}

func TestComposeNormalizesAddressesAndPreservesThreading(t *testing.T) {
	in := validInput()
	in.Cc = []string{"Carol <carol@example.com>"}
	in.Bcc = []string{"Diana <diana@example.com>"}
	in.Reply = Reply{
		InReplyTo:  "<original@mail.example>",
		References: []string{"<first@mail.example>", "<original@mail.example>"},
		ThreadID:   "thread_123",
	}
	result := compose(t, in)

	parts := parseMessageParts(t, result.Raw)
	if len(parts) == 0 {
		t.Fatalf("MIME has no leaf parts")
	}

	message, err := mail.ReadMessage(bytes.NewReader(result.Raw))
	if err != nil {
		t.Fatalf("mail.ReadMessage: %v", err)
	}
	headers := message.Header

	from, err := mail.ParseAddress(headers.Get("From"))
	if err != nil {
		t.Fatalf("parse From: %v", err)
	}

	if from.Name != "Alice Example" || from.Address != "Alice@example.com" {
		t.Fatalf("From = %#v", from)
	}

	if got, want := headers.Get("To"), `"Bob Example" <Bob@example.com>`; got != want {
		t.Fatalf("To = %q, want %q", got, want)
	}

	if got := headers.Get("Bcc"); got == "" {
		t.Fatalf("Bcc is missing from Gmail raw message")
	}

	if got, want := headers.Get("In-Reply-To"), "<original@mail.example>"; got != want {
		t.Fatalf("In-Reply-To = %q, want %q", got, want)
	}

	if got, want := headers.Get("References"), "<first@mail.example> <original@mail.example>"; got != want {
		t.Fatalf("References = %q, want %q", got, want)
	}

	if len(result.Summary.Bcc) != 1 || result.Summary.Bcc[0].Email != "diana@example.com" {
		t.Fatalf("summary Bcc = %#v", result.Summary.Bcc)
	}

	if result.Summary.Reply.ThreadID != "thread_123" {
		t.Fatalf("summary thread ID = %q", result.Summary.Reply.ThreadID)
	}
}

func TestComposeRejectsHeaderInjection(t *testing.T) {
	cases := map[string]func(*Input){
		"from":        func(in *Input) { in.From = "a@example.com\r\nBcc: evil@example.com" },
		"to":          func(in *Input) { in.To = []string{"a@example.com\r\nBcc: evil@example.com"} },
		"subject":     func(in *Input) { in.Subject = "Hello\r\nBcc: evil@example.com" },
		"in-reply-to": func(in *Input) { in.Reply.InReplyTo = "<a@example.com>\r\nBcc: evil@example.com" },
		"reference":   func(in *Input) { in.Reply.References = []string{"<a@example.com>\r\nBcc: evil@example.com"} },
		"filename": func(in *Input) {
			in.Attachments = []Attachment{{Filename: "a.txt\r\nBcc: evil@example.com", ContentType: "text/plain", Data: []byte("x")}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput()
			mutate(&in)

			if _, err := Compose(in); err == nil {
				t.Fatalf("expected header injection rejection")
			}
		})
	}
}

func TestComposeRejectsMarkdownFormat(t *testing.T) {
	in := validInput()
	in.Content = Content{Format: "markdown", Plain: "# Hello"}

	_, err := Compose(in)
	if err == nil || !strings.Contains(err.Error(), `"markdown" is unsupported`) {
		t.Fatalf("markdown error = %v", err)
	}
}

func TestComposeSignatureIsExplicitAndFormatMatched(t *testing.T) {
	in := validInput()
	in.Content = Content{Format: FormatPlain, Plain: "Hello"}
	in.Signature = Signature{Plain: "Alice", HTML: "<p>HTML Alice</p>"}
	in.IncludeSignature = true
	result := compose(t, in)

	parts := parseMessageParts(t, result.Raw)
	if len(parts) != 1 || parts[0].ContentType != "text/plain" {
		t.Fatalf("signed plain-only message produced %#v", parts)
	}

	if got := decodedBody(t, parts[0]); !strings.Contains(got, "Alice") || strings.Contains(got, "<p>") {
		t.Fatalf("signed plain body = %q", got)
	}

	excluded := validInput()
	excluded.Content = Content{Format: FormatHTML, HTML: "<p>Hello</p>"}
	excluded.Signature = Signature{Plain: "Alice", HTML: "<p>Signed</p>"}
	excluded.IncludeSignature = false
	excludedResult := compose(t, excluded)

	excludedParts := parseMessageParts(t, excludedResult.Raw)
	if len(excludedResult.Summary.Warnings) != 0 || excludedResult.Summary.SignatureIncluded {
		t.Fatalf("excluded signature summary = %#v", excludedResult.Summary)
	}

	for _, part := range excludedParts {
		if body := decodedBody(t, part); strings.Contains(body, "Signed") {
			t.Fatalf("excluded signature appeared in %s: %q", part.ContentType, body)
		}
	}
}

func TestComposePlainWithHTMLKeepsBodyAndSignature(t *testing.T) {
	in := validInput()
	in.Content = Content{Format: FormatPlainWithHTML, Plain: "Original body"}
	in.Signature = Signature{Plain: "Alice"}
	in.IncludeSignature = true
	result := compose(t, in)

	htmlParts := findParts(parseMessageParts(t, result.Raw), "text/html")
	if len(htmlParts) != 1 {
		t.Fatalf("HTML parts = %#v", htmlParts)
	}

	html := decodedBody(t, htmlParts[0])
	if !strings.Contains(html, "<p>Original body</p>") || !strings.Contains(html, "<p>Alice</p>") {
		t.Fatalf("HTML body = %q, want original body and signature", html)
	}
}

func TestComposeRejectsInvalidRecipientsAndThreading(t *testing.T) {
	valid := validInput()
	if _, err := Compose(valid); err != nil {
		t.Fatalf("baseline Compose: %v", err)
	}

	cases := map[string]func(*Input){
		"duplicate": func(in *Input) {
			in.To = []string{"same@example.com", "Same@Example.com"}
		},
		"too-many-recipients": func(in *Input) {
			limit := DefaultLimits().MaxRecipientsPerField + 1
			for index := 0; index < limit; index++ {
				in.To = append(in.To, fmt.Sprintf("recipient-%d@example.com", index))
			}
		},
		"bad-in-reply-to": func(in *Input) { in.Reply.InReplyTo = "message@example.com" },
		"bad-reference":   func(in *Input) { in.Reply.References = []string{"<not-an-id>"} },
		"bad-thread-id":   func(in *Input) { in.Reply.ThreadID = "thread/id" },
		"too-many-references": func(in *Input) {
			limit := DefaultLimits().MaxReferences + 1
			for index := 0; index < limit; index++ {
				in.Reply.References = append(in.Reply.References, fmt.Sprintf("<id-%d@example.com>", index))
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput()
			mutate(&in)

			if _, err := Compose(in); err == nil {
				t.Fatalf("expected %q rejection", name)
			}
		})
	}
}

func TestComposeSanitizerRemovesUnsafeLinksAndRemoteImages(t *testing.T) {
	in := validInput()
	in.Content = Content{
		Format: FormatHTML,
		HTML:   `<p><a href="javascript:alert(1)">bad</a><a href="https://example.com">Example</a></p><img src="https://tracking.example/pixel" alt="Pixel">`,
	}
	result := compose(t, in)

	htmlParts := findParts(parseMessageParts(t, result.Raw), "text/html")
	if len(htmlParts) != 1 {
		t.Fatalf("HTML parts = %#v", htmlParts)
	}

	html := decodedBody(t, htmlParts[0])
	if strings.Contains(html, "javascript:") || strings.Contains(html, "<img") || strings.Contains(html, "tracking.example") {
		t.Fatalf("unsafe HTML remained: %q", html)
	}

	if !strings.Contains(html, `<a href="https://example.com">Example</a>`) {
		t.Fatalf("safe link lost: %q", html)
	}
}

func TestComposeEnforcesBodyAndAttachmentSizeLimits(t *testing.T) {
	in := validInput()

	in.Content = Content{Format: FormatHTML, HTML: "<p>" + strings.Repeat("x", DefaultLimits().MaxBodyBytes) + "</p>"}
	if _, err := Compose(in); err == nil || !strings.Contains(err.Error(), "body is") {
		t.Fatalf("body limit error = %v", err)
	}

	attachment := validInput()

	attachment.Attachments = []Attachment{{
		Filename:    "large.bin",
		ContentType: "application/octet-stream",
		Data:        bytes.Repeat([]byte{0}, DefaultLimits().MaxAttachmentBytes+1),
	}}
	if _, err := Compose(attachment); err == nil || !strings.Contains(err.Error(), "large.bin") {
		t.Fatalf("attachment limit error = %v", err)
	}
}

func TestComposeFoldsMaximumRecipientAndReferenceHeaders(t *testing.T) {
	in := validInput()

	in.To = make([]string, 0, DefaultLimits().MaxRecipientsPerField)
	for index := 0; index < DefaultLimits().MaxRecipientsPerField; index++ {
		local := strings.Repeat("r", 900-len(fmt.Sprintf("%d", index)))
		in.To = append(in.To, fmt.Sprintf("%s%d@example.com", local, index))
	}

	in.Reply = Reply{References: make([]string, 0, DefaultLimits().MaxReferences)}
	for index := 0; index < DefaultLimits().MaxReferences; index++ {
		local := strings.Repeat("r", 900-len(fmt.Sprintf("%d", index)))
		in.Reply.References = append(in.Reply.References, fmt.Sprintf("<%s%d@mail.example>", local, index))
	}

	result := compose(t, in)
	lines := strings.Split(string(result.Raw), "\r\n")
	physicalHeaders := 0

	for _, line := range lines {
		if line == "" {
			break
		}

		if len(line) > 998 {
			t.Fatalf("header line contains %d bytes: %.80q", len(line), line)
		}

		physicalHeaders++
	}

	message, err := mail.ReadMessage(bytes.NewReader(result.Raw))
	if err != nil {
		t.Fatalf("read folded MIME: %v", err)
	}

	if got := message.Header.Get("To"); len(strings.Split(got, ",")) != len(in.To) {
		t.Fatalf("folded To contains %d addresses, want %d: %q", len(strings.Split(got, ",")), len(in.To), got)
	}

	if got := message.Header.Get("References"); len(strings.Fields(got)) != len(in.Reply.References) {
		t.Fatalf("folded References contains %d IDs, want %d", len(strings.Fields(got)), len(in.Reply.References))
	}

	if physicalHeaders < 4 {
		t.Fatalf("expected folded physical header lines, got %d", physicalHeaders)
	}
}

func TestComposeRejectsUnbreakableAddressHeader(t *testing.T) {
	in := validInput()
	in.To = []string{strings.Repeat("a", 1024-len("@example.com")) + "@example.com"}

	_, err := Compose(in)

	var validation ValidationError
	if err == nil || !errors.As(err, &validation) || !strings.Contains(err.Error(), "To contains a value") {
		t.Fatalf("unbreakable To error = %v", err)
	}
}

func TestStructuredHeaderReservesSeparatorBeforeFolding(t *testing.T) {
	values := []string{strings.Repeat("a", 494), strings.Repeat("b", 498), "x"}
	var headers strings.Builder

	if err := writeStructuredHeader(&headers, "To", values, ","); err != nil {
		t.Fatalf("write folded header: %v", err)
	}

	for _, line := range strings.Split(strings.TrimSuffix(headers.String(), "\r\n"), "\r\n") {
		if len(line) > 998 {
			t.Fatalf("folded header line contains %d bytes: %q", len(line), line)
		}
	}
}

func TestComposeFoldsEncodedSubject(t *testing.T) {
	in := validInput()
	in.Subject = strings.TrimSuffix(strings.Repeat("Sujeté ", 30), " ")
	result := compose(t, in)

	message, err := mail.ReadMessage(bytes.NewReader(result.Raw))
	if err != nil {
		t.Fatalf("read encoded subject MIME: %v", err)
	}

	decoded, err := (&mime.WordDecoder{}).DecodeHeader(message.Header.Get("Subject"))
	if err != nil {
		t.Fatalf("decode subject: %v", err)
	}

	if strings.ContainsAny(decoded, "\r") || strings.TrimSpace(decoded) != in.Subject {
		t.Fatalf("decoded subject = %q, want %q", decoded, in.Subject)
	}
}

func TestComposeAllowsNineMebibyteAttachmentUnderRawCap(t *testing.T) {
	in := validInput()
	data := bytes.Repeat([]byte{0xa5}, 9<<20)
	in.Attachments = []Attachment{{Filename: "nine.bin", ContentType: "application/octet-stream", Data: data}}

	result := compose(t, in)
	if len(result.Raw) >= DefaultLimits().MaxMessageBytes {
		t.Fatalf("raw message = %d bytes, want under %d", len(result.Raw), DefaultLimits().MaxMessageBytes)
	}

	if len(result.Summary.Attachments) != 1 || result.Summary.Attachments[0].Size != len(data) {
		t.Fatalf("attachment summary = %#v", result.Summary.Attachments)
	}
}

func TestComposeExtractsBodyFromFullHTMLDocument(t *testing.T) {
	in := validInput()
	in.Content = Content{
		Format: FormatHTML,
		HTML:   `<html><head><title>Not body text</title></head><body><p>Visible body</p></body></html>`,
	}
	result := compose(t, in)
	parts := parseMessageParts(t, result.Raw)

	htmlParts := findParts(parts, "text/html")
	if len(htmlParts) != 1 {
		t.Fatalf("HTML parts = %#v", htmlParts)
	}

	html := decodedBody(t, htmlParts[0])

	plain := result.Summary.Preview.Plain
	if strings.Contains(html, "Not body text") || strings.Contains(plain, "Not body text") {
		t.Fatalf("head leaked into output: HTML %q, plain %q", html, plain)
	}

	if !strings.Contains(html, "<p>Visible body</p>") || !strings.Contains(plain, "Visible body") {
		t.Fatalf("body lost: HTML %q, plain %q", html, plain)
	}
}

func TestValidateSemanticMatchesComposeWithoutDateOrSender(t *testing.T) {
	valid := validInput()
	valid.Date = time.Time{}

	valid.From = ""
	if err := ValidateSemantic(valid); err != nil {
		t.Fatalf("ValidateSemantic: %v", err)
	}

	emptyTo := valid
	emptyTo.To = nil

	if err := ValidateSemantic(emptyTo); err == nil || !strings.Contains(err.Error(), "To is required") {
		t.Fatalf("empty To error = %v", err)
	}

	duplicate := valid
	duplicate.To = []string{"same@example.com"}

	duplicate.Bcc = []string{"Same@Example.com"}
	if err := ValidateSemantic(duplicate); err == nil || !strings.Contains(err.Error(), "duplicate recipient") {
		t.Fatalf("cross-field duplicate error = %v", err)
	}
}

func TestComposeMessageIDIncludesRecipientsAndAttachmentBytes(t *testing.T) {
	base := validInput()
	base.Date = testDate
	first := compose(t, base)

	otherRecipient := validInput()
	otherRecipient.Date = testDate
	otherRecipient.To = []string{"Other <other@example.com>"}
	second := compose(t, otherRecipient)

	if first.Summary.MessageID == second.Summary.MessageID {
		t.Fatalf("different recipients produced the same message ID")
	}

	base.Attachments = []Attachment{{Filename: "a.bin", ContentType: "application/octet-stream", Data: []byte("one")}}
	third := compose(t, base)
	base.Attachments[0].Data = []byte("two")
	fourth := compose(t, base)

	if third.Summary.MessageID == fourth.Summary.MessageID {
		t.Fatalf("different attachment bytes produced the same message ID")
	}
}

func TestComposeWrapsAttachmentsAndEnforcesLimits(t *testing.T) {
	in := validInput()
	in.Attachments = []Attachment{
		{Filename: "notes.txt", ContentType: "text/plain; charset=utf-8", Data: []byte("attachment text")},
		{Filename: "résumé.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")},
	}
	result := compose(t, in)
	parts := parseMessageParts(t, result.Raw)
	var bodyPlain int

	for _, part := range parts {
		if part.ContentType == "text/plain" && part.Headers.Get("Content-Disposition") == "" {
			bodyPlain++
		}
	}

	if bodyPlain != 1 || len(findParts(parts, "text/html")) != 1 {
		t.Fatalf("attachment message lost alternative body")
	}

	if len(result.Summary.Attachments) != 2 {
		t.Fatalf("summary attachments = %#v", result.Summary.Attachments)
	}

	message, err := mail.ReadMessage(bytes.NewReader(result.Raw))
	if err != nil {
		t.Fatalf("mail.ReadMessage: %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("top-level type = %q, params %#v, err %v", mediaType, params, err)
	}
	reader := multipart.NewReader(message.Body, params["boundary"])
	var sawResume bool

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("multipart.NextPart: %v", err)
		}

		disposition, dispositionParams, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err == nil && disposition == "attachment" && dispositionParams["filename"] == "résumé.pdf" {
			data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
			if err != nil {
				t.Fatalf("decode attachment: %v", err)
			}

			if string(data) != "%PDF-1.4" {
				t.Fatalf("attachment data = %q", data)
			}
			sawResume = true
		}
	}

	if !sawResume {
		t.Fatalf("UTF-8 attachment was not parsed")
	}

	tooMany := validInput()
	for index := 0; index <= DefaultLimits().MaxAttachments; index++ {
		tooMany.Attachments = append(tooMany.Attachments, Attachment{Filename: "a.txt", ContentType: "text/plain", Data: []byte("x")})
	}

	if _, err := Compose(tooMany); err == nil {
		t.Fatalf("expected attachment limit rejection")
	}
}
