package mailcompose

import (
	"crypto/sha256"
	"encoding/base64"
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

	inline := make([]attachmentInput, 0, len(attachments))

	regular := make([]attachmentInput, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.summary.Disposition == dispositionInline {
			inline = append(inline, attachment)
			continue
		}

		regular = append(regular, attachment)
	}

	if len(inline) > 0 && htmlText == "" {
		return nil, validationError("inline attachments require HTML content")
	}

	relatedType := "text/html"
	if plain != "" && htmlText != "" {
		relatedType = "multipart/alternative"
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

		boundary := chooseBoundary("alternative", plain+htmlText, htmlText)
		if err := writeHeader(&headers, "Content-Type", "multipart/alternative; boundary="+quoteBoundary(boundary)); err != nil {
			return nil, err
		}

		headers.WriteString("\r\n")
		writeAlternativeBody(&body, boundary, plain, htmlText)

		return []byte(headers.String() + body.String()), nil
	}

	if len(regular) == 0 {
		boundary := chooseBoundary("related", plain+htmlText, htmlText)

		contentType := `multipart/related; type="` + relatedType + `"; boundary=` + quoteBoundary(boundary)

		if err := writeHeader(&headers, "Content-Type", contentType); err != nil {
			return nil, err
		}

		headers.WriteString("\r\n")
		writeRelatedBody(&body, boundary, format, plain, htmlText, inline)

		return []byte(headers.String() + body.String()), nil
	}

	boundary := chooseBoundary("mixed", plain+htmlText, htmlText)
	if err := writeHeader(&headers, "Content-Type", "multipart/mixed; boundary="+quoteBoundary(boundary)); err != nil {
		return nil, err
	}

	headers.WriteString("\r\n")

	if len(inline) > 0 {
		relatedBoundary := chooseBoundary("mixed-related", plain+htmlText, htmlText)
		_, _ = fmt.Fprintf(&body, "--%s\r\nContent-Type: multipart/related; type=\"%s\"; boundary=%s\r\n\r\n", boundary, relatedType, quoteBoundary(relatedBoundary))
		writeRelatedBody(&body, relatedBoundary, format, plain, htmlText, inline)
	} else {
		writeBodyPart(&body, boundary, format, plain, htmlText)
	}

	for _, attachment := range regular {
		if err := writeAttachmentPart(&body, boundary, attachment.summary, attachment.data); err != nil {
			return nil, err
		}
	}

	_, _ = fmt.Fprintf(&body, "--%s--\r\n", boundary)

	return []byte(headers.String() + body.String()), nil
}

func writeBodyPart(buffer *strings.Builder, boundary string, format ContentFormat, plain, htmlText string) {
	hasAlternative := htmlText != "" && plain != ""
	if !hasAlternative {
		contentType := "text/plain"
		content := plain

		if format == FormatHTML {
			contentType = "text/html"
			content = htmlText
		}

		writeTextPart(buffer, boundary, contentType, content)

		return
	}

	alternativeBoundary := chooseBoundary("nested-alternative", plain+htmlText, htmlText)
	_, _ = fmt.Fprintf(buffer, "--%s\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n", boundary, quoteBoundary(alternativeBoundary))
	writeAlternativeBody(buffer, alternativeBoundary, plain, htmlText)
}

func writeRelatedBody(buffer *strings.Builder, boundary string, format ContentFormat, plain, htmlText string, inline []attachmentInput) {
	writeBodyPart(buffer, boundary, format, plain, htmlText)

	for _, attachment := range inline {
		_ = writeAttachmentPart(buffer, boundary, attachment.summary, attachment.data)
	}

	_, _ = fmt.Fprintf(buffer, "--%s--\r\n", boundary)
}

func writeAlternativeBody(buffer *strings.Builder, boundary, plain, htmlText string) {
	writeTextPart(buffer, boundary, "text/plain", plain)
	writeTextPart(buffer, boundary, "text/html", htmlText)
	_, _ = fmt.Fprintf(buffer, "--%s--\r\n", boundary)
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

func writeAttachmentPart(buffer *strings.Builder, boundary string, summary AttachmentSummary, data []byte) error {
	disposition := summary.Disposition
	if disposition == "" {
		disposition = dispositionAttachment
	}

	dispositionHeader := mime.FormatMediaType(disposition, map[string]string{"filename": summary.Filename})
	if dispositionHeader == "" {
		return validationError("invalid attachment filename %q", summary.Filename)
	}

	_, _ = fmt.Fprintf(buffer, "\r\n--%s\r\nContent-Type: %s\r\n", boundary, summary.ContentType)
	if summary.ContentID != "" {
		_, _ = fmt.Fprintf(buffer, "Content-ID: <%s>\r\n", summary.ContentID)
	}

	_, _ = fmt.Fprintf(buffer, "Content-Transfer-Encoding: base64\r\nContent-Disposition: %s\r\n\r\n", dispositionHeader)

	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		buffer.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}

	if encoded != "" {
		buffer.WriteString(encoded + "\r\n")
	}

	return nil
}
