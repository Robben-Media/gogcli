// Package apiexec executes pinned Google discovery methods over native HTTP.
package apiexec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	gapi "google.golang.org/api/googleapi"

	"github.com/google/jsonschema-go/jsonschema"

	nativegoogleapi "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googlecatalog"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	maxResponseBytes = 8 << 20
	maxRequestBytes  = 8 << 20
	schemeHTTPS      = "https"
)

func invalid(message string) error {
	return mcpcontract.Invalid(message) //nolint:wrapcheck // Typed public contract error.
}

// Operations returns native HTTP executors for the user-OAuth API catalogue.
// Media upload and download transports are omitted; JSON REST remains available.
func Operations(provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	allowed := make(map[string]struct{})
	for _, def := range mcpcontract.APIDefinitions() {
		allowed[def.Name] = struct{}{}
	}

	methods := make([]googlecatalog.Method, 0, len(allowed))
	for _, method := range googlecatalog.MustLoad().Methods {
		if _, ok := allowed[method.ToolName]; ok {
			methods = append(methods, method)
		}
	}

	return FromMethods(provider, methods)
}

// FromMethods builds executors for the supplied discovery methods. Tests use it
// to pin fixtures without loading the full catalogue.
func FromMethods(provider mcpcontract.ClientProvider, methods []googlecatalog.Method) []mcpcontract.Operation {
	operations := make([]mcpcontract.Operation, 0, len(methods))
	for _, method := range methods {
		if op, ok := newAPIOperation(provider, method); ok {
			operations = append(operations, op)
		}
	}

	return operations
}

type apiOperation struct {
	provider mcpcontract.ClientProvider
	method   googlecatalog.Method
	def      mcpcontract.Definition
	scope    schemaScope
	input    *jsonschema.Schema
	output   *jsonschema.Schema
}

func newAPIOperation(provider mcpcontract.ClientProvider, method googlecatalog.Method) (mcpcontract.Operation, bool) {
	if !executableMethod(method) {
		return mcpcontract.Operation{}, false
	}

	def, ok := mcpcontract.Lookup(method.ToolName)
	if !ok || def.Local {
		return mcpcontract.Operation{}, false
	}

	op := &apiOperation{
		provider: provider,
		method:   method,
		def:      def,
		scope:    scopeFor(method),
		output:   outputSchema(),
	}
	op.input = inputSchema(method, op.scope)

	return mcpcontract.Operation{
		Definition:   op.def,
		InputSchema:  op.input,
		OutputSchema: op.output,
		Decode:       op.decode,
	}, true
}

func executableMethod(method googlecatalog.Method) bool {
	if method.ToolName == "" || method.Action == "" || !supportedHTTPMethod(method.HTTPMethod) {
		return false
	}

	base, err := url.Parse(method.BaseURL)
	if err != nil || base.Scheme != schemeHTTPS || base.User != nil || !googleAPIHost(base.Host) {
		return false
	}

	if strings.TrimSpace(method.Path) == "" {
		return false
	}

	if requiresMediaTransport(method) || requiresStreamingTransport(method) {
		return false
	}

	return true
}

// UnsupportedReason explains why a catalogue method is omitted from Operations.
// An empty reason means JSON REST execution is available.
func UnsupportedReason(method googlecatalog.Method) string {
	if requiresMediaTransport(method) {
		return "media upload and download are not available"
	}

	if requiresStreamingTransport(method) {
		return "server-streamed methods are not available"
	}

	if !executableMethod(method) {
		return "method transport is not implemented"
	}

	return ""
}

var mediaOnlyMethodIDs = map[string]struct{}{
	"chat.media.download":           {},
	"chat.media.upload":             {},
	"drive.files.export":            {},
	"keep.media.download":           {},
	"youtube.captions.download":     {},
	"youtube.captions.insert":       {},
	"youtube.playlistImages.insert": {},
	"youtube.thumbnails.set":        {},
	"youtube.videos.insert":         {},
	"youtube.watermarks.set":        {},
}

var streamingMethodIDs = map[string]struct{}{
	"youtube.youtube.v3.liveChat.messages.stream": {},
}

func requiresStreamingTransport(method googlecatalog.Method) bool {
	_, ok := streamingMethodIDs[method.ID]

	return ok
}

func requiresMediaTransport(method googlecatalog.Method) bool {
	if _, ok := mediaOnlyMethodIDs[method.ID]; ok {
		return true
	}

	if method.Media == nil {
		return false
	}

	if method.Media.Upload != nil && method.Request == nil {
		return true
	}

	if method.Media.Download != nil && method.Response == nil {
		return true
	}

	return false
}

func supportedHTTPMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func (op *apiOperation) decode(raw json.RawMessage) (mcpcontract.Call, error) {
	prepared, err := decodeArgs(op.method, op.scope, raw)
	if err != nil {
		return mcpcontract.Call{}, err
	}

	return mcpcontract.Call{
		AccountID: prepared.accountID,
		Actions:   append([]string(nil), op.def.Actions...),
		Run: func(ctx context.Context, id mcpcontract.Identity) (any, error) {
			return op.run(ctx, id, prepared)
		},
	}, nil
}

func (op *apiOperation) run(ctx context.Context, id mcpcontract.Identity, prepared preparedCall) (any, error) {
	if ctx == nil {
		return nil, invalid("request context is required")
	}

	if err := ctx.Err(); err != nil {
		return nil, publicError(err)
	}

	if op.provider == nil {
		return nil, invalid("client provider is required")
	}

	target, err := buildRequestURL(op.method, prepared.path, prepared.query)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if len(prepared.body) > 0 {
		body = bytes.NewReader(prepared.body)
	}

	req, err := http.NewRequestWithContext(ctx, op.method.HTTPMethod, target.String(), body)
	if err != nil {
		return nil, publicError(err)
	}

	req.Header.Set("Accept", "application/json")

	if len(prepared.body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	client, err := op.provider.HTTPClient(ctx, id, mcpcontract.CallOptions{Operation: op.def.Name, Retry: op.def.Retry})
	if err != nil {
		return nil, publicError(err)
	}

	if client == nil {
		return nil, invalid("client provider returned no HTTP client")
	}

	resp, err := pinClient(client).Do(req)
	if err != nil {
		return nil, mapDoError(err, op.def.Retry)
	}
	defer resp.Body.Close()

	return readResult(id, op.def.Retry, resp)
}

func mapDoError(err error, retry mcpcontract.RetryClass) error {
	if err == nil {
		return nil
	}

	var safe *mcpcontract.Error
	if errors.As(err, &safe) {
		return safe
	}

	if retry != mcpcontract.SafeRead && writeOutcomeUnknown(err) {
		return writeUnknown()
	}

	return publicError(err)
}

func writeOutcomeUnknown(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	var urlErr *url.Error

	return errors.As(err, &urlErr)
}

func writeUnknown() error {
	return &mcpcontract.Error{Category: mcpcontract.OutcomeUnknown, Message: "Google write outcome is unknown", Retryable: false}
}

func publicError(err error) error {
	if err == nil {
		return nil
	}

	return nativegoogleapi.NativePublicError(err)
}

func readResult(id mcpcontract.Identity, retry mcpcontract.RetryClass, resp *http.Response) (mcpcontract.Result[json.RawMessage], error) {
	body, truncated, err := readBounded(resp.Body)
	if err != nil {
		if retry != mcpcontract.SafeRead {
			var safe *mcpcontract.Error
			if errors.As(err, &safe) && (safe.Category == mcpcontract.BudgetExhausted || safe.Category == mcpcontract.Forbidden) {
				return mcpcontract.Result[json.RawMessage]{}, safe
			}

			return mcpcontract.Result[json.RawMessage]{}, writeUnknown()
		}

		return mcpcontract.Result[json.RawMessage]{}, mapDoError(err, retry)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if retry != mcpcontract.SafeRead && ambiguousWriteStatus(resp.StatusCode) {
			return mcpcontract.Result[json.RawMessage]{}, writeUnknown()
		}

		if checkErr := gapi.CheckResponseWithBody(resp, body); checkErr != nil {
			return mcpcontract.Result[json.RawMessage]{}, publicError(checkErr)
		}

		return mcpcontract.Result[json.RawMessage]{}, publicError(&gapi.Error{Code: resp.StatusCode})
	}

	if truncated {
		if retry != mcpcontract.SafeRead {
			return mcpcontract.Result[json.RawMessage]{}, writeUnknown()
		}

		result := mcpcontract.NewResult(id, json.RawMessage("null"))
		result.Truncated = true

		return result, nil
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return mcpcontract.NewResult(id, json.RawMessage("{}")), nil
	}

	if !json.Valid(trimmed) {
		if retry != mcpcontract.SafeRead {
			return mcpcontract.Result[json.RawMessage]{}, writeUnknown()
		}

		return mcpcontract.Result[json.RawMessage]{}, &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "Google request failed"}
	}

	return envelope(id, trimmed), nil
}

func readBounded(r io.Reader) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err != nil {
		if len(data) >= maxResponseBytes {
			return data[:maxResponseBytes], true, nil
		}

		return nil, false, fmt.Errorf("read google response: %w", err)
	}

	if int64(len(data)) > maxResponseBytes {
		return data[:maxResponseBytes], true, nil
	}

	return data, false, nil
}

func envelope(id mcpcontract.Identity, body []byte) mcpcontract.Result[json.RawMessage] {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return mcpcontract.NewResult(id, json.RawMessage("{}"))
	}

	result := mcpcontract.NewResult(id, json.RawMessage(append([]byte(nil), trimmed...)))

	var payload map[string]any
	if err := json.Unmarshal(trimmed, &payload); err == nil {
		if token, ok := payload["nextPageToken"].(string); ok {
			result.NextPageToken = token
		}
	}

	return result
}

func ambiguousWriteStatus(code int) bool {
	return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500
}
