package mcpserver

import (
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

var (
	errPrincipalRequired = errors.New("mcpserver: trusted principal is required")
	errAccountSource     = errors.New("mcpserver: account source is required")
	errDiscoveryMode     = errors.New("mcpserver: discovery mode must be expanded or compact")
	errMaxUpstreamCalls  = errors.New("mcpserver: max upstream calls must be 0 or between 1 and 256")
	errMaxBodyBytes      = errors.New("mcpserver: max body bytes must be non-negative and fit a bounded error with server metadata (at least 512 bytes)")
)

func publicError(err error) *mcpcontract.Error {
	if err == nil {
		return nil
	}

	if mapped := googleapi.NativePublicError(err); mapped != nil {
		return mapped
	}

	return &mcpcontract.Error{Category: mcpcontract.UpstreamFailure, Message: "upstream request failed", Retryable: false}
}

func toolErrorResult(err error) *mcp.CallToolResult {
	typed := publicError(err)

	return &mcp.CallToolResult{
		IsError:           true,
		StructuredContent: typed,
		Content: []mcp.Content{
			&mcp.TextContent{Text: typed.Error()},
		},
	}
}
