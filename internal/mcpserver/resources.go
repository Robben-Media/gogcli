// Package mcpserver adapts typed Google operations to MCP resources.
package mcpserver

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed workflows/*.md
var workflowFiles embed.FS

const workflowURIFormat = "gog://workflows/v1/%s"

type workflowResource struct {
	slug        string
	title       string
	description string
}

var workflowResources = []workflowResource{
	{
		slug:        "mail",
		title:       "Google mail workflow",
		description: "Select a mailbox, search metadata first, then retrieve bounded messages or threads.",
	},
	{
		slug:        "documents",
		title:       "Google documents workflow",
		description: "Find Drive sources, verify metadata, and extract bounded Google Doc text.",
	},
	{
		slug:        "calendar",
		title:       "Google calendar workflow",
		description: "Read calendar metadata and bounded intervals while preserving timezone and partial-failure semantics.",
	},
	{
		slug:        "reporting",
		title:       "Google reporting workflow",
		description: "Run bounded GA4 and Search Console reports with explicit date and metric semantics.",
	},
	{
		slug:        "sheets",
		title:       "Google Sheets workflow",
		description: "Inspect spreadsheet metadata, then read a bounded A1 range with explicit render options.",
	},
}

// RegisterWorkflowResources registers the versioned read-only workflow resources.
func RegisterWorkflowResources(server *mcp.Server) {
	if server == nil {
		return
	}

	for _, workflow := range workflowResources {
		uri := fmt.Sprintf(workflowURIFormat, workflow.slug)

		content, err := workflowFiles.ReadFile("workflows/" + workflow.slug + ".md")
		if err != nil {
			panic(fmt.Sprintf("read workflow %s: %v", workflow.slug, err))
		}
		digest := workflowDigest(content)
		metadata := mcp.Meta{"digest": digest}

		resource := &mcp.Resource{
			Meta:        metadata,
			Name:        workflow.slug,
			Title:       workflow.title,
			Description: workflow.description,
			MIMEType:    "text/markdown",
			Size:        int64(len(content)),
			URI:         uri,
		}
		server.AddResource(resource, func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			if request == nil || request.Params == nil || request.Params.URI != uri {
				return nil, mcp.ResourceNotFoundError(uri)
			}

			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					Meta:     metadata,
					URI:      request.Params.URI,
					MIMEType: "text/markdown",
					Text:     string(content),
				}},
			}, nil
		})
	}
}

func workflowDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
