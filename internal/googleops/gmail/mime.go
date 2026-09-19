package gmail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
	"google.golang.org/api/gmail/v1"
)

var (
	scriptPattern     = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	stylePattern      = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlTagPattern    = regexp.MustCompile(`<[^>]*>`)
	whitespacePattern = regexp.MustCompile(`\s+`)
)

func boundedBody(payload *gmail.MessagePart, limit int) (string, bool, error) {
	text, isHTML, err := bestBodyText(payload)
	if err != nil || text == "" {
		return "", false, err
	}

	if isHTML {
		text = stripHTMLTags(text)
	}
	bounded, truncated := boundedBytes(text, limit)

	return bounded, truncated, nil
}

func bestBodyText(payload *gmail.MessagePart) (string, bool, error) {
	if payload == nil {
		return "", false, nil
	}

	if text, err := findPartBody(payload, "text/plain"); err != nil {
		return "", false, err
	} else if text != "" {
		return text, looksLikeHTML(text), nil
	}

	if text, err := findPartBody(payload, "text/html"); err != nil {
		return "", false, err
	} else if text != "" {
		return text, true, nil
	}

	return "", false, nil
}

func findPartBody(payload *gmail.MessagePart, mimeType string) (string, error) {
	if payload == nil {
		return "", nil
	}

	if mimeTypeMatches(payload.MimeType, mimeType) && payload.Body != nil && payload.Body.Data != "" {
		return decodePartBody(payload)
	}

	for _, part := range payload.Parts {
		text, err := findPartBody(part, mimeType)
		if err != nil {
			return "", err
		}

		if text != "" {
			return text, nil
		}
	}

	return "", nil
}

func decodePartBody(part *gmail.MessagePart) (string, error) {
	if part == nil || part.Body == nil || part.Body.Data == "" {
		return "", nil
	}

	raw, err := decodeBase64(part.Body.Data)
	if err != nil {
		return "", err
	}

	encoding := strings.TrimSpace(headerValue(part, "Content-Transfer-Encoding"))
	contentType := strings.TrimSpace(headerValue(part, "Content-Type"))
	mediaType, params, _ := mime.ParseMediaType(contentType)

	charsetLabel := strings.ToLower(strings.TrimSpace(params["charset"]))
	switch strings.ToLower(encoding) {
	case "base64":
		if decoded, decodeErr := decodeAnyBase64(raw); decodeErr == nil {
			raw = decoded
		}
	case "quoted-printable":
		decoded, decodeErr := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw)))
		if decodeErr == nil && (!labelIsUTF8(charsetLabel) || utf8.Valid(raw) == utf8.Valid(decoded)) {
			raw = decoded
		}
	}

	if strings.HasPrefix(strings.ToLower(mediaType), "text/") && charsetLabel != "" {
		if reader, readerErr := charset.NewReaderLabel(charsetLabel, bytes.NewReader(raw)); readerErr == nil {
			if decoded, decodeErr := io.ReadAll(reader); decodeErr == nil {
				raw = decoded
			}
		}
	}

	return string(raw), nil
}

func labelIsUTF8(label string) bool {
	return label == "utf-8" || label == "utf8" || label == "us-ascii"
}

func mimeTypeMatches(partType, want string) bool {
	return normalizeMimeType(partType) == normalizeMimeType(want)
}

func normalizeMimeType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(value)
	if err == nil && mediaType != "" {
		return mediaType
	}

	if index := strings.Index(value, ";"); index >= 0 {
		return strings.TrimSpace(value[:index])
	}

	return value
}

func looksLikeHTML(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))

	return strings.HasPrefix(trimmed, "<!doctype") ||
		strings.HasPrefix(trimmed, "<html") ||
		strings.HasPrefix(trimmed, "<head") ||
		strings.HasPrefix(trimmed, "<body") ||
		strings.HasPrefix(trimmed, "<meta") ||
		strings.Contains(trimmed, "<html")
}

func stripHTMLTags(value string) string {
	value = scriptPattern.ReplaceAllString(value, "")
	value = stylePattern.ReplaceAllString(value, "")
	value = htmlTagPattern.ReplaceAllString(value, " ")
	value = whitespacePattern.ReplaceAllString(value, " ")

	return strings.TrimSpace(value)
}

func headerValue(part *gmail.MessagePart, name string) string {
	if part == nil {
		return ""
	}

	for _, header := range part.Headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}

	return ""
}

func decodeHeaderValue(value string) string {
	decoder := mime.WordDecoder{CharsetReader: charset.NewReaderLabel}
	if decoded, err := decoder.DecodeHeader(value); err == nil {
		return strings.TrimSpace(decoded)
	}

	return strings.TrimSpace(value)
}

func boundedHeaderValue(part *gmail.MessagePart, name string) (string, bool) {
	value := decodeHeaderValue(headerValue(part, name))
	bounded, truncated := boundedBytes(value, maxHeaderBytes)

	return bounded, truncated
}

func boundedString(value string, limit int) string {
	bounded, _ := boundedBytes(value, limit)
	return bounded
}

func boundedBytes(value string, limit int) (string, bool) {
	if limit <= 0 {
		return "", value != ""
	}

	if len(value) <= limit {
		return value, false
	}

	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}

	return value[:cut], true
}

func decodeBase64(value string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}

	if decoded, err := base64.URLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}

	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode Gmail MIME body: %w", err)
	}

	return decoded, nil
}

func decodeAnyBase64(data []byte) ([]byte, error) {
	cleaned := make([]byte, 0, len(data))
	for _, character := range data {
		switch character {
		case '\n', '\r', '\t', ' ':
			continue
		default:
			cleaned = append(cleaned, character)
		}
	}

	return decodeBase64(string(cleaned))
}
