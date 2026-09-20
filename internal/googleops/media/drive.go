package media

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

var allowedExportMIME = map[string]bool{
	"text/plain":                true,
	"text/csv":                  true,
	"text/tab-separated-values": true,
	"text/html":                 true,
	"application/rtf":           true,
	"application/pdf":           true,
	"application/epub+zip":      true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
}

func (s *service) downloadFile(ctx context.Context, id mcpcontract.Identity, in DownloadInput) (mcpcontract.Result[FileContentData], error) {
	if in.IncludeMetadata {
		if err := requireBudget(ctx, 2); err != nil {
			return mcpcontract.Result[FileContentData]{}, err
		}
	}

	bundle, err := s.identityClient(ctx, id, opDownload)
	if err != nil {
		return mcpcontract.Result[FileContentData]{}, err
	}

	limit := maxBytesOrDefault(in.MaxBytes)

	meta := driveFileMeta{ID: in.FileID}
	if in.IncludeMetadata {
		var lookupErr error

		meta, lookupErr = s.getDriveMetadata(ctx, bundle, in.FileID)
		if lookupErr != nil {
			return mcpcontract.Result[FileContentData]{}, lookupErr
		}

		if meta.Size > 0 && meta.Size > int64(limit) {
			return mcpcontract.Result[FileContentData]{}, oversize()
		}
	}

	query := url.Values{"alt": []string{"media"}, "supportsAllDrives": []string{"true"}}
	if in.AcknowledgeAbuse {
		query.Set("acknowledgeAbuse", "true")
	}
	endpoint := driveFileURL(in.FileID) + "?" + query.Encode()

	payload, header, err := bundle.do(ctx, http.MethodGet, endpoint, nil, "", limit)
	if err != nil {
		return mcpcontract.Result[FileContentData]{}, err
	}

	if meta.MimeType == "" {
		meta.MimeType = header.Get("Content-Type")
	}

	encoding, inline, artifact, err := s.deliver(ctx, id, opDownload, artifactName(meta.Name), firstNonEmpty(meta.MimeType, mimeOctetStream), in.Delivery, payload)
	if err != nil {
		return mcpcontract.Result[FileContentData]{}, err
	}

	return mcpcontract.NewResult(id, FileContentData{
		FileID:    in.FileID,
		Name:      meta.Name,
		MimeType:  meta.MimeType,
		SizeBytes: int64(len(payload)),
		Encoding:  encoding,
		Data:      inline,
		Artifact:  artifact,
	}), nil
}

func (s *service) exportFile(ctx context.Context, id mcpcontract.Identity, in ExportInput) (mcpcontract.Result[FileContentData], error) {
	bundle, err := s.identityClient(ctx, id, opExport)
	if err != nil {
		return mcpcontract.Result[FileContentData]{}, err
	}

	limit := maxBytesOrDefault(in.MaxBytes)
	query := url.Values{"mimeType": []string{in.MimeType}}
	endpoint := driveFileURL(in.FileID) + "/export?" + query.Encode()

	payload, _, err := bundle.do(ctx, http.MethodGet, endpoint, nil, "", limit)
	if err != nil {
		return mcpcontract.Result[FileContentData]{}, err
	}

	encoding, inline, artifact, err := s.deliver(ctx, id, opExport, "", in.MimeType, in.Delivery, payload)
	if err != nil {
		return mcpcontract.Result[FileContentData]{}, err
	}

	return mcpcontract.NewResult(id, FileContentData{
		FileID:    in.FileID,
		MimeType:  in.MimeType,
		SizeBytes: int64(len(payload)),
		Encoding:  encoding,
		Data:      inline,
		Artifact:  artifact,
	}), nil
}

func (s *service) createFile(ctx context.Context, id mcpcontract.Identity, in CreateInput) (mcpcontract.Result[DriveWriteData], error) {
	bundle, err := s.identityClient(ctx, id, opCreate)
	if err != nil {
		return mcpcontract.Result[DriveWriteData]{}, err
	}

	raw, err := decodePayload(in.Encoding, in.Data)
	if err != nil {
		return mcpcontract.Result[DriveWriteData]{}, err
	}

	metadata := map[string]any{"name": in.Name}
	if in.MimeType != "" {
		metadata["mimeType"] = in.MimeType
	}

	if in.Description != "" {
		metadata["description"] = in.Description
	}

	if in.ParentID != "" {
		metadata["parents"] = []string{in.ParentID}
	}

	body, contentType, err := multipartUpload(metadata, raw, in.MimeType)
	if err != nil {
		return mcpcontract.Result[DriveWriteData]{}, err
	}

	endpoint := "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart&supportsAllDrives=true"

	payload, _, err := bundle.do(ctx, http.MethodPost, endpoint, body, contentType, maxDecodedBytes)
	if err != nil {
		return mcpcontract.Result[DriveWriteData]{}, err
	}

	parsed, err := parseDriveWrite(payload)
	if err != nil {
		return mcpcontract.Result[DriveWriteData]{}, err
	}

	return mcpcontract.NewResult(id, parsed), nil
}

func (s *service) updateFile(ctx context.Context, id mcpcontract.Identity, in UpdateInput) (mcpcontract.Result[DriveWriteData], error) {
	bundle, err := s.identityClient(ctx, id, opUpdate)
	if err != nil {
		return mcpcontract.Result[DriveWriteData]{}, err
	}

	raw, err := decodePayload(in.Encoding, in.Data)
	if err != nil {
		return mcpcontract.Result[DriveWriteData]{}, err
	}

	metadata := map[string]any{}
	if in.Name != "" {
		metadata["name"] = in.Name
	}

	if in.MimeType != "" {
		metadata["mimeType"] = in.MimeType
	}

	if in.Description != "" {
		metadata["description"] = in.Description
	}

	body, contentType, err := multipartUpload(metadata, raw, in.MimeType)
	if err != nil {
		return mcpcontract.Result[DriveWriteData]{}, err
	}

	endpoint := "https://www.googleapis.com/upload/drive/v3/files/" + url.PathEscape(in.FileID) + "?uploadType=multipart&supportsAllDrives=true"

	payload, _, err := bundle.do(ctx, http.MethodPatch, endpoint, body, contentType, maxDecodedBytes)
	if err != nil {
		return mcpcontract.Result[DriveWriteData]{}, err
	}

	parsed, err := parseDriveWrite(payload)
	if err != nil {
		return mcpcontract.Result[DriveWriteData]{}, err
	}

	return mcpcontract.NewResult(id, parsed), nil
}

type driveFileMeta struct {
	ID       string
	Name     string
	MimeType string
	Size     int64
}

func (s *service) getDriveMetadata(ctx context.Context, bundle *httpClientBundle, fileID string) (driveFileMeta, error) {
	query := url.Values{"fields": []string{"id,name,mimeType,size"}, "supportsAllDrives": []string{"true"}}

	payload, _, err := bundle.do(ctx, http.MethodGet, driveFileURL(fileID)+"?"+query.Encode(), nil, "", maxDecodedBytes)
	if err != nil {
		return driveFileMeta{}, err
	}

	var parsed driveAPIFile
	if err = json.Unmarshal(payload, &parsed); err != nil {
		return driveFileMeta{}, publicError(err)
	}

	meta := driveFileMeta{ID: parsed.ID, Name: parsed.Name, MimeType: parsed.MIMEType}
	if parsed.Size != "" {
		size, parseErr := strconv.ParseInt(parsed.Size, 10, 64)
		if parseErr == nil {
			meta.Size = size
		}
	}

	return meta, nil
}

func driveFileURL(fileID string) string {
	return "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(fileID)
}

func multipartUpload(metadata map[string]any, data []byte, mimeType string) ([]byte, string, error) {
	if err := validateMIME(mimeType); err != nil {
		return nil, "", err
	}

	if mimeType == "" {
		mimeType = mimeOctetStream
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", "application/json; charset=UTF-8")

	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", publicError(err)
	}

	if err = json.NewEncoder(part).Encode(metadata); err != nil {
		return nil, "", publicError(err)
	}

	mediaHeader := textproto.MIMEHeader{}
	mediaHeader.Set("Content-Type", mimeType)

	media, err := writer.CreatePart(mediaHeader)
	if err != nil {
		return nil, "", publicError(err)
	}

	if _, err = media.Write(data); err != nil {
		return nil, "", publicError(err)
	}

	if err = writer.Close(); err != nil {
		return nil, "", publicError(err)
	}

	return body.Bytes(), "multipart/related; boundary=" + writer.Boundary(), nil
}

type driveAPIFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MIMEType string `json:"mimeType"` //nolint:tagliatelle // Drive API wire name.
	Size     string `json:"size"`
}

func parseDriveWrite(payload []byte) (DriveWriteData, error) {
	var parsed driveAPIFile
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return DriveWriteData{}, writeError(err)
	}

	if strings.TrimSpace(parsed.ID) == "" {
		return DriveWriteData{}, outcomeUnknown("Google returned no Drive file ID; reconcile before repeating")
	}

	out := DriveWriteData{FileID: parsed.ID, Name: parsed.Name, MimeType: parsed.MIMEType}
	if parsed.Size != "" {
		if size, parseErr := strconv.ParseInt(parsed.Size, 10, 64); parseErr == nil {
			out.SizeBytes = size
		}
	}

	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}
