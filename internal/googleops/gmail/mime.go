package gmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
	gmailapi "google.golang.org/api/gmail/v1"
)

var (
	scriptPattern     = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	stylePattern      = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlTagPattern    = regexp.MustCompile(`<[^>]*>`)
	whitespacePattern = regexp.MustCompile(`\s+`)
)

func boundedBody(ctx context.Context, api *gmailapi.Service, messageID string, payload *gmailapi.MessagePart, limit int) (string, bool, string, error) {
	part, isHTML, found, err := bestBodyPart(payload)
	if err != nil || !found {
		return "", false, "", err
	}

	selectedAttachmentID := ""
	if part.Body != nil && part.Body.AttachmentId != "" && part.Body.Data == "" {
		selectedAttachmentID = part.Body.AttachmentId
		if part.Body.Size > maxBodyBytes {
			return "", true, selectedAttachmentID, nil
		}

		attachment, fetchErr := api.Users.Messages.Attachments.Get("me", messageID, selectedAttachmentID).Context(ctx).Do()
		if fetchErr != nil {
			return "", false, "", publicError(fetchErr)
		}

		if attachment == nil {
			return "", false, "", publicError(errEmptyMessage)
		}

		external := *part
		external.Body = attachment
		part = &external
	}

	text, err := decodePartBody(part)
	if err != nil {
		return "", false, selectedAttachmentID, publicError(err)
	}

	if isHTML || looksLikeHTML(text) {
		text = stripHTMLTags(text)
	}
	bounded, truncated := boundedBytes(text, limit)

	return bounded, truncated, selectedAttachmentID, nil
}

func bestBodyPart(payload *gmailapi.MessagePart) (*gmailapi.MessagePart, bool, bool, error) {
	if payload == nil {
		return nil, false, false, nil
	}

	part, found, err := findBodyPart(payload, "text/plain")
	if err != nil || found {
		return part, false, found, err
	}

	part, found, err = findBodyPart(payload, "text/html")
	if err != nil || found {
		return part, true, found, err
	}

	return nil, false, false, nil
}

func findBodyPart(payload *gmailapi.MessagePart, mimeType string) (*gmailapi.MessagePart, bool, error) {
	if payload == nil {
		return nil, false, nil
	}

	if mimeTypeMatches(payload.MimeType, mimeType) && !isAttachmentPart(payload) && hasBodyData(payload) {
		return payload, true, nil
	}

	for _, part := range payload.Parts {
		if part == nil {
			continue
		}

		selected, found, err := findBodyPart(part, mimeType)
		if err != nil {
			return nil, false, err
		}

		if found {
			return selected, true, nil
		}
	}

	return nil, false, nil
}

func isAttachmentPart(part *gmailapi.MessagePart) bool {
	if part == nil {
		return true
	}

	if strings.TrimSpace(part.Filename) != "" {
		return true
	}

	disposition := strings.TrimSpace(headerValue(part, "Content-Disposition"))
	if index := strings.Index(disposition, ";"); index >= 0 {
		disposition = disposition[:index]
	}

	return strings.EqualFold(strings.TrimSpace(disposition), "attachment")
}

func hasBodyData(part *gmailapi.MessagePart) bool {
	return part != nil && part.Body != nil && (part.Body.Data != "" || part.Body.AttachmentId != "")
}

func decodePartBody(part *gmailapi.MessagePart) (string, error) {
	if part == nil || part.Body == nil || part.Body.Data == "" {
		return "", nil
	}

	raw, err := decodeBase64(part.Body.Data)
	if err != nil {
		return "", err
	}

	contentType := strings.TrimSpace(headerValue(part, "Content-Type"))
	if contentType == "" {
		contentType = strings.TrimSpace(part.MimeType)
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", fmt.Errorf("parse Gmail MIME content type: %w", err)
	}

	// Decode the documented Gmail base64url transport exactly once, then the
	// declared MIME charset. Do not infer a second transfer decode from a
	// retained Content-Transfer-Encoding header or Body.Size; raw RFC822 MIME
	// decoding belongs to a separate raw-message operation if one is added.
	charsetLabel := strings.ToLower(strings.TrimSpace(params["charset"]))

	if strings.HasPrefix(mediaType, "text/") && charsetLabel != "" {
		decoded, err := decodeCharset(raw, charsetLabel)
		if err != nil {
			return "", err
		}
		raw = decoded
	}

	return string(raw), nil
}

func decodeCharset(raw []byte, label string) ([]byte, error) {
	reader, err := charset.NewReaderLabel(label, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("open Gmail body charset %q: %w", label, err)
	}

	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("decode Gmail body charset %q: %w", label, err)
	}

	return decoded, nil
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

func headerValue(part *gmailapi.MessagePart, name string) string {
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

func boundedHeaderValue(part *gmailapi.MessagePart, name string) (string, bool) {
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

	decoded, err := base64.URLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode Gmail base64url body: %w", err)
	}

	return decoded, nil
}
