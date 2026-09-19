package mcpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

func (rt *Runtime) handlerFor(operation mcpcontract.Operation) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		started := time.Now()
		traceID := newTraceID()

		ctx, _ = googleapi.WithUpstreamCounter(ctx)

		ctx, cancel := rt.withTimeout(ctx)
		defer cancel()

		if err := rt.acquire(ctx); err != nil {
			result := toolErrorResult(err)
			rt.logCall(ctx, operation.Definition.Name, traceID, started, result)

			return result, nil
		}
		defer rt.release()

		raw := json.RawMessage(`{}`)
		if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
			raw = request.Params.Arguments
		}
		if err := rt.checkSize(int64(len(raw)), "request"); err != nil {
			result := toolErrorResult(err)
			rt.logCall(ctx, operation.Definition.Name, traceID, started, result)

			return result, nil
		}

		call, err := operation.Decode(raw)
		if err != nil {
			result := toolErrorResult(err)
			rt.logCall(ctx, operation.Definition.Name, traceID, started, result)

			return result, nil
		}

		identity, err := rt.authorizer.Authorize(ctx, rt.principal, call.AccountID, operation.Definition.Name, call.Actions)
		if err != nil {
			result := toolErrorResult(err)
			rt.logCall(ctx, operation.Definition.Name, traceID, started, result)

			return result, nil
		}

		out, err := call.Run(ctx, identity)
		if err != nil {
			result := toolErrorResult(err)
			rt.logCall(ctx, operation.Definition.Name, traceID, started, result)

			return result, nil
		}

		result, err := rt.resultFor(out)
		if err != nil {
			result = toolErrorResult(err)
		}

		rt.logCall(ctx, operation.Definition.Name, traceID, started, result)

		return result, nil
	}
}

func (rt *Runtime) resultFor(out any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(out)
	if err != nil {
		return nil, &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "encode result", Retryable: false}
	}

	result := &mcp.CallToolResult{
		StructuredContent: out,
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "encode result", Retryable: false}
	}

	if err := rt.checkSize(int64(len(encoded)), "response"); err != nil {
		return nil, err
	}

	return result, nil
}

func (rt *Runtime) checkSize(n int64, kind string) error {
	if rt.maxBodyBytes > 0 && n > rt.maxBodyBytes {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: kind + " exceeds configured size limit", Retryable: false}
	}

	return nil
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
