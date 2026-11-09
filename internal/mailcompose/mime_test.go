package mailcompose

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"net/mail"
	"regexp"
	"strings"
	"testing"
	"time"
)

// RFC 2387 section 3.1 requires type to match the root part's media type.
func TestRelatedRootMediaType(t *testing.T) {
	t.Parallel()

	for _, format := range []ContentFormat{FormatHTML, FormatPlainAndHTML} {
		for _, regular := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/regular=%t", format, regular), func(t *testing.T) {
				t.Parallel()
				in := validInput()

				in.Content = Content{Format: format, HTML: `<img src="cid:logo@example.com">`}
				if format == FormatPlainAndHTML {
					in.Content.Plain = "logo"
				}

				in.Attachments = []Attachment{{Filename: "logo.png", ContentType: "image/png", Data: []byte("png"), Disposition: "inline", ContentID: "logo@example.com"}}
				if regular {
					in.Attachments = append(in.Attachments, Attachment{Filename: "report.txt", ContentType: "text/plain", Data: []byte("report")})
				}

				out, err := Compose(in)
				if err != nil {
					t.Fatal(err)
				}

				msg, err := mail.ReadMessage(bytes.NewReader(out.Raw))
				if err != nil {
					t.Fatal(err)
				}

				mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
				if err != nil {
					t.Fatal(err)
				}

				body := msg.Body
				if regular {
					part, partErr := multipart.NewReader(body, params["boundary"]).NextPart()
					if partErr != nil {
						t.Fatal(partErr)
					}

					mediaType, params, err = mime.ParseMediaType(part.Header.Get("Content-Type"))
					if err != nil {
						t.Fatal(err)
					}
					body = part
				}

				if mediaType != "multipart/related" {
					t.Fatalf("media type = %q", mediaType)
				}

				part, err := multipart.NewReader(body, params["boundary"]).NextPart()
				if err != nil {
					t.Fatal(err)
				}

				rootType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
				if err != nil {
					t.Fatal(err)
				}

				if params["type"] != rootType {
					t.Fatalf("related type %q differs from root %q", params["type"], rootType)
				}
			})
		}
	}
}

var encodedWordPattern = regexp.MustCompile(`=\?[^?]+\?[qQbB]\?[^?]*\?=`)

func renderTestMIME(t *testing.T, from Address, to []Address, subject string, attachments []attachmentInput) *mail.Message {
	t.Helper()

	raw, err := renderMIME("<id@example.com>", time.Unix(0, 0).UTC(), from, to, nil, nil, subject, ReplySummary{}, FormatPlain, "Hello", "", attachments)
	if err != nil {
		t.Fatalf("renderMIME: %v", err)
	}

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("read MIME: %v", err)
	}

	return msg
}

// rawHeaderLines returns the physical lines of one top-level header field.
func rawHeaderLines(t *testing.T, raw, name string) []string {
	t.Helper()

	head, _, _ := strings.Cut(raw, "\r\n\r\n")
	var lines []string

	for _, line := range strings.Split(head, "\r\n") {
		switch {
		case strings.HasPrefix(line, name+":"):
			lines = append(lines, line)
		case len(lines) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")):
			lines = append(lines, line)
		case len(lines) > 0:
			return lines
		}
	}

	if len(lines) == 0 {
		t.Fatalf("missing %s header in %q", name, head)
	}

	return lines
}

// RFC 2047 section 6.2 keeps whitespace between an encoded-word and plain text,
// so every chunk of a non-ASCII Subject must be an encoded-word (Fleet #805).
func TestSubjectRoundTripsExactly(t *testing.T) {
	t.Parallel()

	long := "Cancel account + refund request – $1,376 renewal charged Nov 6, Zoë’s follow-up — résumé attached, naïve café invoice ✓ for the Q4 retainer and reporting summary"
	if len(long) <= 150 {
		t.Fatalf("long subject is %d bytes, want > 150", len(long))
	}

	for _, subject := range []string{
		"Cancel account + refund request – $1,376 renewal charged Nov 6",
		"Test – en dash subject folding check charged Nov 6 abcdefghij",
		"Quarterly retainer invoice and reporting summary for the account – Nov",
		long,
		"Project update",
	} {
		t.Run(subject, func(t *testing.T) {
			t.Parallel()

			raw, err := renderMIME("<id@example.com>", time.Unix(0, 0).UTC(), Address{Email: "a@example.com"}, []Address{{Email: "b@example.com"}}, nil, nil, subject, ReplySummary{}, FormatPlain, "Hello", "", nil)
			if err != nil {
				t.Fatalf("renderMIME: %v", err)
			}

			msg, err := mail.ReadMessage(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("read MIME: %v", err)
			}

			decoded, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
			if err != nil {
				t.Fatalf("decode subject: %v", err)
			}

			if decoded != subject {
				t.Fatalf("decoded subject = %q, want %q", decoded, subject)
			}

			lines := rawHeaderLines(t, string(raw), "Subject")
			for _, line := range lines {
				if len(line) > 78 {
					t.Errorf("subject line is %d bytes: %q", len(line), line)
				}

				for _, word := range encodedWordPattern.FindAllString(line, -1) {
					if len(word) > 75 {
						t.Errorf("encoded-word is %d bytes: %q", len(word), word)
					}
				}
			}

			if subject == "Project update" && (len(lines) != 1 || lines[0] != "Subject: Project update") {
				t.Fatalf("ASCII subject lines = %q", lines)
			}

			if subject == long && len(lines) < 2 {
				t.Fatalf("long subject did not fold: %q", lines)
			}
		})
	}
}

func TestDisplayNamesAndFilenameRoundTripExactly(t *testing.T) {
	t.Parallel()

	fromName := "Jeremy Johnson – Robben Media Client Services and Accounts Receivable Desk"
	toName := "Zoë Ångström – Williams Appraisal Support Team, Quarterly Retainer Billing"
	filename := "Rechnung – Übersicht Q4 Zoë’s café.pdf"

	msg := renderTestMIME(t,
		Address{Name: fromName, Email: "a@example.com"},
		[]Address{{Name: toName, Email: "b@example.com"}, {Name: "Café Ops", Email: "c@example.com"}},
		"Project update",
		[]attachmentInput{{summary: AttachmentSummary{Filename: filename, ContentType: "application/pdf"}, data: []byte("pdf")}},
	)

	from, err := msg.Header.AddressList("From")
	if err != nil || len(from) != 1 || from[0].Name != fromName {
		t.Fatalf("From = %#v, %v", from, err)
	}

	to, err := msg.Header.AddressList("To")
	if err != nil || len(to) != 2 || to[0].Name != toName || to[1].Name != "Café Ops" {
		t.Fatalf("To = %#v, %v", to, err)
	}

	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}

	reader := multipart.NewReader(msg.Body, params["boundary"])
	for {
		part, partErr := reader.NextPart()
		if partErr != nil {
			t.Fatalf("attachment part not found: %v", partErr)
		}

		if part.FileName() != "" {
			if part.FileName() != filename {
				t.Fatalf("filename = %q, want %q", part.FileName(), filename)
			}

			return
		}
	}
}
