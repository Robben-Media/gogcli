package mailcompose

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

func renderMIME(
	messageID string,
	date time.Time,
	from Address,
	to, cc, bcc []Address,
	subject string,
	reply ReplySummary,
	format ContentFormat,
	plain, htmlText string,
	attachments []attachmentInput,
) ([]byte, error) {
	var headers strings.Builder

	if err := writeHeader(&headers, "From", formatAddress(from)); err != nil {
		return nil, err
	}

	if err := writeAddressHeader(&headers, "To", to); err != nil {
		return nil, err
	}

	if len(cc) > 0 {
		if err := writeAddressHeader(&headers, "Cc", cc); err != nil {
			return nil, err
		}
	}

	if len(bcc) > 0 {
		if err := writeAddressHeader(&headers, "Bcc", bcc); err != nil {
			return nil, err
		}
	}

	if err := writeSubjectHeader(&headers, subject); err != nil {
		return nil, err
	}

	if err := writeHeader(&headers, "Date", date.Format(time.RFC1123Z)); err != nil {
		return nil, err
	}

	if err := writeHeader(&headers, "Message-ID", messageID); err != nil {
		return nil, err
	}

	if err := writeHeader(&headers, "MIME-Version", "1.0"); err != nil {
		return nil, err
	}

	if reply.InReplyTo != "" {
		if err := writeHeader(&headers, "In-Reply-To", reply.InReplyTo); err != nil {
			return nil, err
		}
	}

	if len(reply.References) > 0 {
		if err := writeStructuredHeader(&headers, "References", reply.References, ""); err != nil {
			return nil, err
		}
	}

	var body strings.Builder

	if len(attachments) == 0 {
		if format == FormatPlain && htmlText == "" {
			if err := writePartHeaders(&headers, "text/plain"); err != nil {
				return nil, err
			}

			headers.WriteString("\r\n")

			if err := writeQuotedPrintable(&headers, plain); err != nil {
				return nil, err
			}

			return []byte(headers.String()), nil
		}

		if format == FormatHTML && plain == "" {
			if err := writePartHeaders(&headers, "text/html"); err != nil {
				return nil, err
			}

			headers.WriteString("\r\n")

			if err := writeQuotedPrintable(&headers, htmlText); err != nil {
				return nil, err
			}

			return []byte(headers.String()), nil
		}

		alternativeBoundary := chooseBoundary("alternative", plain+htmlText, htmlText)
		if err := writeHeader(&headers, "Content-Type", "multipart/alternative; boundary="+quoteBoundary(alternativeBoundary)); err != nil {
			return nil, err
		}

		headers.WriteString("\r\n")

		writeTextPart(&body, alternativeBoundary, "text/plain", plain)
		writeTextPart(&body, alternativeBoundary, "text/html", htmlText)
		_, _ = fmt.Fprintf(&body, "--%s--\r\n", alternativeBoundary)

		return []byte(headers.String() + body.String()), nil
	}

	mixedBoundary := chooseBoundary("mixed", plain+htmlText, htmlText)
	if err := writeHeader(&headers, "Content-Type", "multipart/mixed; boundary="+quoteBoundary(mixedBoundary)); err != nil {
		return nil, err
	}

	headers.WriteString("\r\n")

	hasAlternative := htmlText != "" && plain != ""
	if hasAlternative {
		alternativeBoundary := chooseBoundary("mixed-alternative", plain+htmlText, htmlText)
		_, _ = fmt.Fprintf(&body, "--%s\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n", mixedBoundary, quoteBoundary(alternativeBoundary))
		writeTextPart(&body, alternativeBoundary, "text/plain", plain)
		writeTextPart(&body, alternativeBoundary, "text/html", htmlText)
		_, _ = fmt.Fprintf(&body, "--%s--\r\n", alternativeBoundary)
	} else {
		contentType := "text/plain"
		if format == FormatHTML {
			contentType = "text/html"
		}

		content := plain
		if format == FormatHTML {
			content = htmlText
		}

		writeTextPart(&body, mixedBoundary, contentType, content)
	}

	for _, attachment := range attachments {
		if err := writeAttachment(&body, mixedBoundary, attachment.summary, attachment.data); err != nil {
			return nil, err
		}
	}
	_, _ = fmt.Fprintf(&body, "--%s--\r\n", mixedBoundary)

	return []byte(headers.String() + body.String()), nil
}

type attachmentInput struct {
	summary AttachmentSummary
	data    []byte
}

func writeHeader(buffer *strings.Builder, name, value string) error {
	line := name + ": " + value
	if len(line) > maxHeaderLineBytes {
		return validationError("%s cannot fit in an RFC 5322 header line", name)
	}

	buffer.WriteString(line + "\r\n")

	return nil
}

func writeAddressHeader(buffer *strings.Builder, name string, addresses []Address) error {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, formatAddress(address))
	}

	return writeStructuredHeader(buffer, name, values, ",")
}

// writeStructuredHeader folds only between complete values. A value that cannot
// occupy a physical line is rejected instead of being split illegally.
func writeStructuredHeader(buffer *strings.Builder, name string, values []string, separator string) error {
	if len(values) == 0 {
		return validationError("%s is required", name)
	}

	prefix := name + ":"
	separatorLength := len(separator)
	current := ""

	for index, value := range values {
		capacity := maxHeaderLineBytes
		if index < len(values)-1 {
			capacity -= separatorLength
		}

		delimiter := " "
		if separator != "" {
			delimiter = separator + " "
		}

		if index == 0 {
			current = prefix + " " + value
		} else if candidate := current + delimiter + value; len(candidate) <= capacity {
			current = candidate
		} else {
			folded := current + separator
			if len(folded) > maxHeaderLineBytes {
				return validationError("%s contains a value that cannot fit in an RFC 5322 header line", name)
			}

			buffer.WriteString(folded + "\r\n")
			current = " " + value
		}

		if len(current) > capacity {
			return validationError("%s contains a value that cannot fit in an RFC 5322 header line", name)
		}
	}

	buffer.WriteString(current + "\r\n")

	return nil
}

func writeSubjectHeader(buffer *strings.Builder, subject string) error {
	if isASCII(subject) {
		return writeHeader(buffer, "Subject", subject)
	}

	words, err := encodedHeaderWords(subject)
	if err != nil {
		return err
	}

	return writeStructuredHeader(buffer, "Subject", words, "")
}

// encodedHeaderWords splits UTF-8 input at rune boundaries into bounded RFC 2047
// words so their separators remain legal folding points.
func encodedHeaderWords(value string) ([]string, error) {
	words := make([]string, 0, len(value)/18+1)

	for len(value) > 0 {
		end := len(value)
		for end > 0 && (!utf8.ValidString(value[:end]) || len(mime.QEncoding.Encode("utf-8", value[:end])) > 75) {
			end--
		}

		if end == 0 {
			return nil, validationError("subject cannot be encoded as RFC 2047 words")
		}

		words = append(words, mime.QEncoding.Encode("utf-8", value[:end]))
		value = value[end:]
	}

	return words, nil
}

func formatAddress(address Address) string {
	return (&mail.Address{Name: address.Name, Address: address.Email}).String()
}

func isASCII(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] > 0x7f {
			return false
		}
	}

	return true
}

func writePartHeaders(buffer *strings.Builder, contentType string) error {
	if err := writeHeader(buffer, "Content-Type", contentType+"; charset=utf-8"); err != nil {
		return err
	}

	return writeHeader(buffer, "Content-Transfer-Encoding", "quoted-printable")
}

func chooseBoundary(kind, protected, htmlText string) string {
	for index := 0; ; index++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", kind, index)))

		boundary := "=_gog-" + hex.EncodeToString(digest[:])[:24]
		if !containsBoundaryLine(protected, boundary) && !strings.Contains(htmlText, boundary) {
			return boundary
		}
	}
}

func quoteBoundary(boundary string) string {
	return `"` + boundary + `"`
}

func containsBoundaryLine(value, boundary string) bool {
	delimiter := "\r\n--" + boundary + "\r\n"
	return strings.HasPrefix(value, delimiter) || strings.Contains(value, delimiter)
}

func writeQuotedPrintable(writer io.Writer, value string) error {
	qp := quotedprintable.NewWriter(writer)
	if _, err := qp.Write([]byte(value)); err != nil {
		return fmt.Errorf("write quoted-printable body: %w", err)
	}

	if err := qp.Close(); err != nil {
		return fmt.Errorf("close quoted-printable body: %w", err)
	}

	return nil
}
