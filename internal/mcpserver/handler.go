package mcpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

func (rt *Runtime) handlerFor(operation mcpcontract.Operation) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		started := time.Now()
		traceID := newTraceID()

		ctx, _ = googleapi.WithUpstreamCounter(ctx)
		ctx = googleapi.WithUpstreamBudget(ctx, rt.maxUpstreamCalls)

		ctx, cancel := rt.withTimeout(ctx)
		defer cancel()

		if err := rt.acquire(ctx); err != nil {
			return rt.finish(ctx, operation.Definition.Name, traceID, started, toolErrorResult(err)), nil
		}
		defer rt.release()

		if !rt.enableWrites && writeOperation(operation) {
			return rt.finish(ctx, operation.Definition.Name, traceID, started, toolErrorResult(writeDisabledError())), nil
		}

		raw := json.RawMessage(`{}`)
		if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
			raw = request.Params.Arguments
		}

		if err := rt.checkSize(int64(len(raw)), "request"); err != nil {
			return rt.finish(ctx, operation.Definition.Name, traceID, started, toolErrorResult(err)), nil
		}

		call, err := operation.Decode(raw)
		if err != nil {
			return rt.finish(ctx, operation.Definition.Name, traceID, started, toolErrorResult(err)), nil
		}

		identity, err := rt.authorizer.Authorize(ctx, rt.principal, call.AccountID, operation.Definition.Name, call.Actions)
		if err != nil {
			return rt.finish(ctx, operation.Definition.Name, traceID, started, toolErrorResult(err)), nil
		}

		out, err := call.Run(ctx, identity)
		if err != nil {
			return rt.finish(ctx, operation.Definition.Name, traceID, started, toolErrorResult(err)), nil
		}

		result, err := rt.resultForSized(out, writeOperation(operation))
		if err != nil {
			result = toolErrorResult(err)
		}

		return rt.finish(ctx, operation.Definition.Name, traceID, started, result), nil
	}
}

func (rt *Runtime) resultFor(out any) (*mcp.CallToolResult, error) {
	return rt.resultForSized(out, false)
}

func (rt *Runtime) resultForSized(out any, write bool) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(out)
	if err != nil {
		return nil, encodeResultError(write, out)
	}

	result := &mcp.CallToolResult{
		StructuredContent: out,
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, encodeResultError(write, out)
	}

	if err := rt.checkSize(int64(len(encoded)), "response"); err != nil {
		return nil, oversizedResultError(write, out)
	}

	return result, nil
}

func (rt *Runtime) checkSize(n int64, kind string) error {
	if rt.maxBodyBytes > 0 && n > rt.maxBodyBytes {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: kind + " exceeds configured size limit", Retryable: false}
	}

	return nil
}

func encodeResultError(write bool, out any) error {
	if !write {
		return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "encode result", Retryable: false}
	}

	return writeResultUnknown(out, "Write completed but the result could not be encoded. Reconcile the target before repeating; do not retry this write.")
}

func oversizedResultError(write bool, out any) error {
	if !write {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "response exceeds configured size limit", Retryable: false}
	}

	return writeResultUnknown(out, "Write completed but the response exceeded the size limit. Reconcile the target before repeating; do not retry this write.")
}

func writeResultUnknown(out any, message string) error {
	accountID, resourceID := writeIdentity(out)
	if accountID != "" {
		message += " account_id=" + accountID + "."
	}

	if resourceID != "" {
		message += " resource_id=" + resourceID + "."
	}

	return &mcpcontract.Error{Category: mcpcontract.OutcomeUnknown, Message: message, Retryable: false}
}

func writeIdentity(out any) (accountID, resourceID string) {
	if out == nil {
		return "", ""
	}

	data, err := json.Marshal(out)
	if err != nil {
		return "", ""
	}

	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", ""
	}

	accountID = boundedIdentity(envelope["account_id"])
	if dataObj, ok := envelope["data"].(map[string]any); ok {
		for _, key := range []string{"id", "draft_id", "message_id", "documentId", "document_id", "spreadsheetId", "spreadsheet_id", "presentationId", "presentation_id", "name", "resourceName", "resource_id"} {
			if resourceID = boundedIdentity(dataObj[key]); resourceID != "" {
				break
			}
		}
	}

	if resourceID == "" {
		resourceID = boundedIdentity(envelope["id"])
	}

	return accountID, resourceID
}

func boundedIdentity(value any) string {
	s, ok := value.(string)
	if !ok {
		return ""
	}

	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > 128 || strings.Contains(s, "://") {
		return ""
	}

	return s
}

func (rt *Runtime) finish(ctx context.Context, operation string, traceID string, started time.Time, result *mcp.CallToolResult) *mcp.CallToolResult {
	write := false

	var out any
	if result != nil && !result.IsError {
		out = result.StructuredContent

		if op, ok := rt.lookupOperation(operation); ok {
			write = writeOperation(op)
		}
	} else if category, ok := resultCategory(result); ok && category == mcpcontract.OutcomeUnknown {
		write = true
		out = result.StructuredContent
	}

	rt.attachUsage(ctx, result)

	if rt.checkResultSize(result) != nil {
		result = rt.fitErrorResult(ctx, write, out)
	}

	rt.logCall(ctx, operation, traceID, started, result)

	return result
}

func (rt *Runtime) fitErrorResult(ctx context.Context, write bool, out any) *mcp.CallToolResult {
	var stages []error
	if write {
		stages = []error{
			writeResultUnknown(out, "Write outcome could not be returned within the size limit. Reconcile the target before repeating; do not retry this write."),
			&mcpcontract.Error{Category: mcpcontract.OutcomeUnknown, Message: "Write outcome is unknown. Do not retry.", Retryable: false},
		}
	} else {
		stages = []error{&mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "response exceeds configured size limit", Retryable: false}}
	}

	for _, err := range stages {
		result := toolErrorResult(err)
		rt.attachUsage(ctx, result)

		if rt.checkResultSize(result) == nil {
			return result
		}

		result.Meta = nil

		if rt.checkResultSize(result) == nil {
			return result
		}
	}

	result := toolErrorResult(stages[len(stages)-1])
	result.Meta = nil

	return result
}

func (rt *Runtime) checkResultSize(result *mcp.CallToolResult) error {
	if result == nil {
		return nil
	}

	wire := *result

	wire.Meta = make(mcp.Meta, len(result.Meta)+1)
	for key, value := range result.Meta {
		wire.Meta[key] = value
	}

	rt.ensureServerInfo(wire.Meta)

	encoded, err := json.Marshal(&wire)
	if err != nil {
		return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "encode result", Retryable: false}
	}

	// The SDK sets this private top-level field after the handler returns.
	// Always reserve it: a nested user field with the same name is unrelated.
	return rt.checkSize(int64(len(encoded)+len(`,"resultType":"complete"`)), "response")
}

func (rt *Runtime) attachUsage(ctx context.Context, result *mcp.CallToolResult) {
	if result == nil {
		return
	}

	if result.Meta == nil {
		result.Meta = mcp.Meta{}
	}

	rt.ensureServerInfo(result.Meta)
	result.Meta["upstream_calls"] = googleapi.UpstreamCalls(ctx)

	if remaining := googleapi.UpstreamBudgetRemaining(ctx); remaining >= 0 {
		result.Meta["upstream_budget_remaining"] = remaining
	}
}

func (rt *Runtime) ensureServerInfo(meta mcp.Meta) {
	if meta == nil {
		return
	}

	if _, exists := meta[mcp.MetaKeyServerInfo]; exists {
		return
	}

	implementation := rt.implementation
	if implementation == nil {
		implementation = &mcp.Implementation{Name: defaultName, Version: defaultVersion}
	}

	meta[mcp.MetaKeyServerInfo] = implementation
}

func (rt *Runtime) logCall(ctx context.Context, operation string, traceID string, started time.Time, result *mcp.CallToolResult) {
	attrs := []any{
		slog.String("operation", operation),
		slog.String("principal", rt.principal.ID),
		slog.Duration("elapsed", time.Since(started)),
		slog.String("trace_id", traceID),
		slog.Int64("upstream_calls", googleapi.UpstreamCalls(ctx)),
	}

	if result != nil && result.IsError {
		if typed, ok := result.StructuredContent.(*mcpcontract.Error); ok {
			attrs = append(attrs, slog.String("error_category", string(typed.Category)))
		}

		rt.logger.Info("mcp tool denied_or_failed", attrs...)

		return
	}

	rt.logger.Info("mcp tool ok", attrs...)
}

func resultCategory(result *mcp.CallToolResult) (mcpcontract.ErrorCategory, bool) {
	if result == nil {
		return "", false
	}

	if typed, ok := result.StructuredContent.(*mcpcontract.Error); ok && typed != nil {
		return typed.Category, true
	}

	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return "", false
	}

	var public mcpcontract.Error
	if err := json.Unmarshal(data, &public); err != nil || public.Category == "" {
		return "", false
	}

	return public.Category, true
}
