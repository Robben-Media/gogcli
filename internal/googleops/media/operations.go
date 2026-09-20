package media

import (
	"context"
	"encoding/base64"
	"strings"
	"unicode/utf8"

	nativegoogleapi "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	defaultMaxBytes      = 1 << 20
	maxDecodedBytes      = 2 << 20
	maxIDBytes           = 2048
	maxNameBytes         = 1024
	maxArtifactNameBytes = 512
	encodingBase64       = "base64"
	encodingUTF8         = "utf8"
	mimeOctetStream      = "application/octet-stream"
	opAttachment         = "gmail_get_attachment"
	opDownload           = "drive_download_file"
	opExport             = "drive_export_file"
	opCreate             = "drive_create_file"
	opUpdate             = "drive_update_file"
	deliveryArtifact     = "artifact"
	deliveryInline       = "inline"
	gmailJSONOverhead    = 128
)

type AttachmentInput struct {
	mcpcontract.Selection
	MessageID    string `json:"message_id" jsonschema:"Gmail message ID returned by gmail_search or gmail_get_message; required"`
	AttachmentID string `json:"attachment_id" jsonschema:"Gmail attachment ID from message payload; required"`
	MaxBytes     int    `json:"max_bytes,omitempty" jsonschema:"Maximum decoded bytes; default 1048576, maximum 2097152"`
	Delivery     string `json:"delivery,omitempty" jsonschema:"artifact when a store is configured, or inline base64; default artifact with a store, inline otherwise"`
}

type AttachmentData struct {
	MessageID    string                      `json:"message_id"`
	AttachmentID string                      `json:"attachment_id"`
	SizeBytes    int64                       `json:"size_bytes"`
	Encoding     string                      `json:"encoding,omitempty"`
	Data         string                      `json:"data,omitempty"`
	Artifact     *mcpcontract.MediaReference `json:"artifact,omitempty"`
}

type DownloadInput struct {
	mcpcontract.Selection
	FileID           string `json:"file_id" jsonschema:"Google Drive file ID; required"`
	MaxBytes         int    `json:"max_bytes,omitempty" jsonschema:"Maximum decoded bytes; default 1048576, maximum 2097152"`
	IncludeMetadata  bool   `json:"include_metadata,omitempty" jsonschema:"If true, fetch metadata before bytes to refuse known oversize; uses a second API read"`
	AcknowledgeAbuse bool   `json:"acknowledge_abuse,omitempty" jsonschema:"Acknowledge malware risk when Google requires it to download"`
	Delivery         string `json:"delivery,omitempty" jsonschema:"artifact when a store is configured, or inline base64; default artifact with a store, inline otherwise"`
}

type FileContentData struct {
	FileID    string                      `json:"file_id"`
	Name      string                      `json:"name,omitempty"`
	MimeType  string                      `json:"mime_type,omitempty"`
	SizeBytes int64                       `json:"size_bytes"`
	Encoding  string                      `json:"encoding,omitempty"`
	Data      string                      `json:"data,omitempty"`
	Artifact  *mcpcontract.MediaReference `json:"artifact,omitempty"`
}

type ExportInput struct {
	mcpcontract.Selection
	FileID   string `json:"file_id" jsonschema:"Google Drive file ID of a Docs, Sheets, or Slides file; required"`
	MimeType string `json:"mime_type" jsonschema:"Destination MIME type; required"`
	MaxBytes int    `json:"max_bytes,omitempty" jsonschema:"Maximum decoded bytes; default 1048576, maximum 2097152"`
	Delivery string `json:"delivery,omitempty" jsonschema:"artifact when a store is configured, or inline base64; default artifact with a store, inline otherwise"`
}

type CreateInput struct {
	mcpcontract.Selection
	FileID      string `json:"file_id,omitempty" jsonschema:"Optional predetermined ID from Drive generateIds; persist before creating and reconcile unknown outcomes"`
	Name        string `json:"name" jsonschema:"File name; required"`
	MimeType    string `json:"mime_type,omitempty" jsonschema:"Content MIME type"`
	ParentID    string `json:"parent_id,omitempty" jsonschema:"Optional parent folder ID"`
	Description string `json:"description,omitempty" jsonschema:"Optional file description"`
	Encoding    string `json:"encoding,omitempty" jsonschema:"Payload encoding: utf8 or base64; default utf8"`
	Data        string `json:"data" jsonschema:"File bytes encoded per encoding; required"`
}

type UpdateInput struct {
	mcpcontract.Selection
	ExpectedVersion int64  `json:"expected_version,omitempty" jsonschema:"Optional positive Drive version; rejects stale content and uses atomic If-Match, zero means no precondition"`
	FileID          string `json:"file_id" jsonschema:"Google Drive file ID to replace; required"`
	Name            string `json:"name,omitempty" jsonschema:"Optional new file name"`
	MimeType        string `json:"mime_type,omitempty" jsonschema:"Optional new content MIME type"`
	Description     string `json:"description,omitempty" jsonschema:"Optional file description"`
	Encoding        string `json:"encoding,omitempty" jsonschema:"Payload encoding: utf8 or base64; default utf8"`
	Data            string `json:"data" jsonschema:"Replacement bytes encoded per encoding; required"`
}

type DriveWriteData struct {
	FileID    string `json:"file_id"`
	Name      string `json:"name,omitempty"`
	MimeType  string `json:"mime_type,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

// Operations returns the five native media tools in catalog order. Without a
// store, successful reads default to inline base64.
func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return OperationsWithArtifacts(provider, nil)
}

// OperationsWithArtifacts returns the same tools. Successful reads store bytes
// through artifacts and return a MediaReference unless delivery=inline.
func OperationsWithArtifacts(provider mcpcontract.ClientProvider, artifacts mcpcontract.MediaArtifacts) []mcpcontract.Operation {
	svc := &service{provider: provider, artifacts: artifacts}

	return []mcpcontract.Operation{
		mcpcontract.NewOperation(opAttachment, svc.validateAttachment, svc.getAttachment),
		mcpcontract.NewOperation(opDownload, svc.validateDownload, svc.downloadFile),
		mcpcontract.NewOperation(opExport, svc.validateExport, svc.exportFile),
		mcpcontract.NewOperation(opCreate, svc.validateCreate, svc.createFile),
		mcpcontract.NewOperation(opUpdate, svc.validateUpdate, svc.updateFile),
	}
}

type service struct {
	provider  mcpcontract.ClientProvider
	artifacts mcpcontract.MediaArtifacts
}

func (s *service) validateAttachment(in AttachmentInput) error {
	if err := validateID("message_id", in.MessageID); err != nil {
		return err
	}

	if err := validateID("attachment_id", in.AttachmentID); err != nil {
		return err
	}

	if err := s.validateDelivery(in.Delivery); err != nil {
		return err
	}

	return validateMaxBytes(in.MaxBytes)
}

func (s *service) validateDownload(in DownloadInput) error {
	if err := validateID("file_id", in.FileID); err != nil {
		return err
	}

	if err := s.validateDelivery(in.Delivery); err != nil {
		return err
	}

	return validateMaxBytes(in.MaxBytes)
}

func (s *service) validateExport(in ExportInput) error {
	if err := validateID("file_id", in.FileID); err != nil {
		return err
	}

	if !allowedExportMIME[in.MimeType] {
		return invalid("mime_type is not a supported export type")
	}

	if err := s.validateDelivery(in.Delivery); err != nil {
		return err
	}

	return validateMaxBytes(in.MaxBytes)
}

func (s *service) validateCreate(in CreateInput) error {
	if in.FileID != "" {
		if err := validateID("file_id", in.FileID); err != nil {
			return err
		}
	}

	if err := validateName(in.Name, true); err != nil {
		return err
	}

	if err := validateMIME(in.MimeType); err != nil {
		return err
	}

	if in.ParentID != "" {
		if err := validateID("parent_id", in.ParentID); err != nil {
			return err
		}
	}

	if _, err := decodePayload(in.Encoding, in.Data); err != nil {
		return err
	}

	return nil
}

func (s *service) validateUpdate(in UpdateInput) error {
	if in.ExpectedVersion < 0 {
		return invalid("expected_version must be positive or omitted")
	}

	if err := validateID("file_id", in.FileID); err != nil {
		return err
	}

	if in.Name != "" {
		if err := validateName(in.Name, false); err != nil {
			return err
		}
	}

	if err := validateMIME(in.MimeType); err != nil {
		return err
	}

	if _, err := decodePayload(in.Encoding, in.Data); err != nil {
		return err
	}

	return nil
}

func validateMaxBytes(value int) error {
	if value < 0 || value > maxDecodedBytes {
		return invalid("max_bytes must be between 1 and 2097152, or omitted for the default")
	}

	return nil
}

func maxBytesOrDefault(value int) int {
	if value == 0 {
		return defaultMaxBytes
	}

	return value
}

func (s *service) validateDelivery(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return nil
	case deliveryInline:
		return nil
	case deliveryArtifact:
		if s == nil || s.artifacts == nil {
			return invalid("artifact delivery requires a media store")
		}

		return nil
	default:
		return invalid("delivery must be artifact or inline")
	}
}

func (s *service) deliver(ctx context.Context, id mcpcontract.Identity, operation, name, mimeType, delivery string, data []byte) (encoding string, inline string, ref *mcpcontract.MediaReference, err error) {
	wantInline := s.wantsInline(delivery)
	if !wantInline && (s == nil || s.artifacts == nil) {
		return "", "", nil, invalid("artifact delivery requires a media store")
	}

	if wantInline {
		return encodingBase64, encodeBase64(data), nil, nil
	}

	reference, err := s.artifacts.Put(ctx, id, operation, artifactName(name), firstNonEmpty(mimeType, mimeOctetStream), data)
	if err != nil {
		return "", "", nil, publicError(err)
	}

	return "", "", &reference, nil
}

func (s *service) wantsInline(delivery string) bool {
	switch strings.ToLower(strings.TrimSpace(delivery)) {
	case deliveryInline:
		return true
	case deliveryArtifact:
		return false
	default:
		return s == nil || s.artifacts == nil
	}
}

func validateID(name, value string) error {
	if value != strings.TrimSpace(value) {
		return invalid(name + " must not contain surrounding whitespace")
	}

	if value == "" {
		return invalid(name + " is required")
	}

	if len(value) > maxIDBytes || !utf8.ValidString(value) {
		return invalid(name + " is invalid")
	}

	if strings.ContainsRune(value, 0) || strings.ContainsAny(value, "/\\?#") {
		return invalid(name + " is invalid")
	}

	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return invalid(name + " is invalid")
		}
	}

	if containsCTL(value) {
		return invalid(name + " is invalid")
	}

	return nil
}

func validateName(value string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return invalid("name is required")
	}

	if value == "" {
		return nil
	}

	if len(value) > maxNameBytes || !utf8.ValidString(value) || containsCTL(value) {
		return invalid("name is invalid")
	}

	return nil
}

func validateMIME(value string) error {
	if value == "" {
		return nil
	}

	if containsCTL(value) {
		return invalid("mime_type is invalid")
	}

	return nil
}

func containsCTL(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < 0x20 || value[i] == 0x7f {
			return true
		}
	}

	return false
}

func artifactName(name string) string {
	if name == "" || len(name) > maxArtifactNameBytes || !utf8.ValidString(name) || containsCTL(name) {
		return ""
	}

	return name
}

func decodePayload(encoding, data string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", encodingUTF8:
		if !utf8.ValidString(data) {
			return nil, invalid("data must be valid utf8")
		}

		raw := []byte(data)
		if len(raw) == 0 {
			return nil, invalid("data is required")
		}

		if len(raw) > maxDecodedBytes {
			return nil, oversize()
		}

		return raw, nil
	case encodingBase64:
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, invalid("data must be standard base64")
		}

		if len(raw) == 0 {
			return nil, invalid("data is required")
		}

		if len(raw) > maxDecodedBytes {
			return nil, oversize()
		}

		return raw, nil
	default:
		return nil, invalid("encoding must be utf8 or base64")
	}
}

func encodeBase64(raw []byte) string {
	return base64.StdEncoding.EncodeToString(raw)
}

func oversize() error {
	return &mcpcontract.Error{Category: mcpcontract.BudgetExhausted, Message: "media exceeds the bounded size", Retryable: false}
}

func invalid(message string) error {
	return mcpcontract.Invalid(message) //nolint:wrapcheck // Typed public contract error.
}

func outcomeUnknown(message string) error {
	return &mcpcontract.Error{Category: mcpcontract.OutcomeUnknown, Message: message, Retryable: false}
}

func publicError(err error) error {
	if err == nil {
		return nil
	}

	return nativegoogleapi.NativePublicError(err)
}

func writeError(err error) error {
	if err == nil {
		return nil
	}

	return nativegoogleapi.NativeWritePublicError(err)
}

func requireBudget(ctx context.Context, calls int64) error {
	if remaining := nativegoogleapi.UpstreamBudgetRemaining(ctx); remaining >= 0 && remaining < calls {
		return &mcpcontract.Error{Category: mcpcontract.BudgetExhausted, Message: "insufficient remaining API budget for metadata and download", Retryable: false}
	}

	return nil
}

func (s *service) identityClient(ctx context.Context, id mcpcontract.Identity, name string) (*httpClientBundle, error) {
	if s == nil || s.provider == nil {
		return nil, invalid("client provider is required")
	}

	def, ok := mcpcontract.Lookup(name)
	if !ok {
		return nil, invalid("unknown Google operation")
	}

	client, err := s.provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: def.Name, Retry: def.Retry})
	if err != nil {
		return nil, publicError(err)
	}

	if client == nil {
		return nil, invalid("client provider returned no HTTP client")
	}

	return &httpClientBundle{client: pinClient(client), def: def}, nil
}
