package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegisterWorkflowResourcesListAndRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test-gog-mcp", Version: "test"}, nil)
	RegisterWorkflowResources(server)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	listed, err := clientSession.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}

	wantURIs := []string{
		"gog://workflows/v1/mail",
		"gog://workflows/v1/documents",
		"gog://workflows/v1/calendar",
		"gog://workflows/v1/reporting",
		"gog://workflows/v1/sheets",
	}
	if len(listed.Resources) != len(wantURIs) {
		t.Fatalf("resource count = %d, want %d", len(listed.Resources), len(wantURIs))
	}

	listedByURI := make(map[string]*mcp.Resource, len(listed.Resources))
	for _, resource := range listed.Resources {
		listedByURI[resource.URI] = resource
	}

	for _, want := range wantURIs {
		resource := listedByURI[want]
		if resource == nil {
			t.Fatalf("resource %s was not listed", want)
		}

		if resource.URI != want {
			t.Fatalf("resource URI = %q, want %q", resource.URI, want)
		}

		content, readErr := workflowFiles.ReadFile("workflows/" + strings.TrimPrefix(want, "gog://workflows/v1/") + ".md")
		if readErr != nil {
			t.Fatalf("read workflow fixture: %v", readErr)
		}
		sum := sha256.Sum256(content)

		wantDigest := "sha256:" + hex.EncodeToString(sum[:])
		if got := resource.Meta["digest"]; got != wantDigest {
			t.Fatalf("resource %s digest = %v, want %q", want, got, wantDigest)
		}

		if resource.MIMEType != "text/markdown" || resource.Size != int64(len(content)) {
			t.Fatalf("resource %s metadata = %#v", want, resource)
		}
	}

	read, err := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: wantURIs[0]})
	if err != nil {
		t.Fatalf("read mail workflow: %v", err)
	}

	if len(read.Contents) != 1 {
		t.Fatalf("content count = %d, want 1", len(read.Contents))
	}
	content := read.Contents[0]

	mail, err := workflowFiles.ReadFile("workflows/mail.md")
	if err != nil {
		t.Fatalf("read mail fixture: %v", err)
	}

	if content.URI != wantURIs[0] || content.MIMEType != "text/markdown" || content.Text != string(mail) {
		t.Fatalf("unexpected mail content: %#v", content)
	}

	if content.Meta["digest"] != listedByURI[wantURIs[0]].Meta["digest"] {
		t.Fatalf("read digest %v does not match discovery digest %v", content.Meta["digest"], listedByURI[wantURIs[0]].Meta["digest"])
	}

	_, err = clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: "gog://workflows/v1/unknown"})
	if err == nil {
		t.Fatal("expected unknown workflow URI to fail")
	}

	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("unknown URI error = %v, want resource-not-found code", err)
	}
}

func TestWorkflowContentUsesFrozenSafetyConventions(t *testing.T) {
	for _, workflow := range workflowResources {
		content, err := workflowFiles.ReadFile("workflows/" + workflow.slug + ".md")
		if err != nil {
			t.Fatalf("read %s: %v", workflow.slug, err)
		}

		text := string(content)
		if !strings.Contains(text, "`accounts_list`") || !strings.Contains(text, "`account_id`") {
			t.Fatalf("%s does not require explicit account selection", workflow.slug)
		}

		for _, forbidden := range []string{"shell", "credential", "updater", "skills", "CLI"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains forbidden instruction term %q", workflow.slug, forbidden)
			}
		}
	}

	calendar, err := workflowFiles.ReadFile("workflows/calendar.md")
	if err != nil {
		t.Fatalf("read calendar: %v", err)
	}

	if !strings.Contains(string(calendar), "`calendar_id`") || !strings.Contains(string(calendar), "`partial_failures`") {
		t.Fatal("calendar workflow omits frozen calendar or partial-failure semantics")
	}

	reporting, err := workflowFiles.ReadFile("workflows/reporting.md")
	if err != nil {
		t.Fatalf("read reporting: %v", err)
	}

	if !strings.Contains(string(reporting), "`offset`") || !strings.Contains(string(reporting), "Pacific") {
		t.Fatal("reporting workflow omits GA4 offset or Search Console timezone semantics")
	}
}

const testTimeout = 5 * time.Second
