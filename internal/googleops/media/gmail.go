package media

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func (s *service) getAttachment(ctx context.Context, id mcpcontract.Identity, in AttachmentInput) (mcpcontract.Result[AttachmentData], error) {
	bundle, err := s.identityClient(ctx, id, opAttachment)
	if err != nil {
		return mcpcontract.Result[AttachmentData]{}, err
	}

	limit := maxBytesOrDefault(in.MaxBytes)
	endpoint := "https://gmail.googleapis.com/gmail/v1/users/me/messages/" + url.PathEscape(in.MessageID) + "/attachments/" + url.PathEscape(in.AttachmentID)

	payload, _, err := bundle.do(ctx, http.MethodGet, endpoint, nil, "", gmailJSONBound(limit))
	if err != nil {
		return mcpcontract.Result[AttachmentData]{}, err
	}

	var parsed struct {
		Size int64  `json:"size"`
		Data string `json:"data"`
	}
	if err = json.Unmarshal(payload, &parsed); err != nil {
		return mcpcontract.Result[AttachmentData]{}, publicError(err)
	}

	if parsed.Size > int64(limit) {
		return mcpcontract.Result[AttachmentData]{}, oversize()
	}

	raw, err := decodeGmailData(parsed.Data, parsed.Size)
	if err != nil {
		return mcpcontract.Result[AttachmentData]{}, err
	}

	if int64(len(raw)) > int64(limit) {
		return mcpcontract.Result[AttachmentData]{}, oversize()
	}

	encoding, inline, artifact, err := s.deliver(ctx, id, opAttachment, "", mimeOctetStream, in.Delivery, raw)
	if err != nil {
		return mcpcontract.Result[AttachmentData]{}, err
	}

	return mcpcontract.NewResult(id, AttachmentData{
		MessageID:    in.MessageID,
		AttachmentID: in.AttachmentID,
		SizeBytes:    int64(len(raw)),
		Encoding:     encoding,
		Data:         inline,
		Artifact:     artifact,
	}), nil
}

func gmailJSONBound(decoded int) int {
	return gmailJSONOverhead + base64.RawURLEncoding.EncodedLen(decoded)
}

func decodeGmailData(value string, size int64) ([]byte, error) {
	if value == "" {
		if size == 0 {
			return []byte{}, nil
		}

		return nil, invalid("attachment bytes are missing")
	}

	if raw, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return raw, nil
	}

	if raw, err := base64.URLEncoding.DecodeString(value); err == nil {
		return raw, nil
	}

	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, invalid("attachment bytes are invalid")
	}

	return raw, nil
}
