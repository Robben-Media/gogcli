package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mediaartifact"
)

func (rt *Runtime) registerMediaResources(store *mediaartifact.Store) {
	rt.server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: mediaartifact.URIPrefix + "{id}",
		Name:        "temporary-google-media",
		Description: "Read a temporary media reference with fresh account authorization. References expire after 15 minutes and never expose local files.",
	}, func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		ctx, cleanup, err := rt.beginLocal(ctx)
		if err != nil {
			return nil, fmt.Errorf("media resource: %w", err)
		}
		defer cleanup()

		if request == nil || request.Params == nil {
			return nil, mcp.ResourceNotFoundError("")
		}
		uri := request.Params.URI

		binding, ok := store.Lookup(uri)
		if !ok {
			return nil, mcp.ResourceNotFoundError(mediaartifact.URIPrefix + "unavailable")
		}

		def, ok := mcpcontract.Lookup(binding.Operation)
		if !ok {
			return nil, mcp.ResourceNotFoundError(mediaartifact.URIPrefix + "unavailable")
		}

		id, err := rt.authorizer.Authorize(ctx, rt.principal, binding.Identity.AccountID, binding.Operation, def.Actions)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(mediaartifact.URIPrefix + "unavailable")
		}

		data, ref, err := store.Read(ctx, uri, id)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(mediaartifact.URIPrefix + "unavailable")
		}
		result := &mcp.ReadResourceResult{
			Meta: mcp.Meta{},
			Contents: []*mcp.ResourceContents{{
				URI: uri, MIMEType: ref.MIMEType, Blob: data,
				Meta: mcp.Meta{"sha256": ref.SHA256, "size_bytes": ref.SizeBytes, "expires_at": ref.ExpiresAt},
			}},
		}
		rt.ensureServerInfo(result.Meta)

		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("encode media resource: %w", err)
		}
		// Account for the SDK's private field added after the handler returns.
		if err := rt.checkSize(int64(len(encoded)+len(`,"resultType":"complete"`)), "response"); err != nil {
			return nil, fmt.Errorf("media resource size: %w", err)
		}

		return result, nil
	})
}
