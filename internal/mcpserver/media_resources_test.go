package mcpserver_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/googleops/media"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
	"github.com/steipete/gogcli/internal/mediaartifact"
)

func TestMediaResourceFreshAuthorization(t *testing.T) {
	t.Parallel()

	for _, change := range []string{"grant", "other_account_only", "scope", "scope_alternative", "scope_limited", "disconnect", "reconnect", "subject", "principal", "client"} {
		t.Run(change, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			accounts := fixtureAccounts()

			id, _, err := accounts.Get(ctx, "personal")
			if err != nil {
				t.Fatal(err)
			}

			operation, action := "gmail_get_attachment", "gmail:attachment"
			if change == "scope_limited" {
				operation, action = "drive_download_file", "drive:download"
				id.Scopes = []string{mcpcontract.DriveReadScope}
				accounts.Put(id)
			}

			store := mediaartifact.New()
			t.Cleanup(store.Close)

			ref, err := store.Put(ctx, id, operation, "", "application/pdf", []byte("private attachment"))
			if err != nil {
				t.Fatal(err)
			}
			cfg := fixtureConfig(nil)
			cfg.Accounts = accounts
			cfg.MediaArtifacts = store
			cfg.AllowOperations = []string{operation}
			cfg.Grants = []mcpcontract.Grant{{PrincipalID: "fixture", AccountIDs: []string{"personal", "work"}, ClientNames: []string{"app"}, Operations: []string{action}}}

			rt, err := mcpserver.New(cfg)
			if err != nil {
				t.Fatal(err)
			}

			session := connectRuntime(t, ctx, rt)
			defer session.Close()

			read, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: ref.URI})
			if err != nil || len(read.Contents) != 1 || string(read.Contents[0].Blob) != "private attachment" {
				t.Fatalf("resource read: %#v %v", read, err)
			}

			listed, err := session.ListResources(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}

			for _, r := range listed.Resources {
				if strings.HasPrefix(r.URI, mediaartifact.URIPrefix) {
					t.Fatal("sensitive reference listed")
				}
			}

			switch change {
			case "grant":
				rt.ReplaceAccess(access.Snapshot{AllowOperations: cfg.AllowOperations})
			case "other_account_only":
				work, _, workErr := accounts.Get(ctx, "work")
				if workErr != nil {
					t.Fatal(workErr)
				}

				workRef, workErr := store.Put(ctx, work, operation, "", "text/plain", []byte("work attachment"))
				if workErr != nil {
					t.Fatal(workErr)
				}

				cfg.Grants[0].AccountIDs = []string{"work"}
				if err := rt.ReloadAccess(access.Snapshot{AllowOperations: cfg.AllowOperations, Grants: cfg.Grants}); err != nil {
					t.Fatal(err)
				}

				workRead, workErr := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: workRef.URI})
				if workErr != nil || len(workRead.Contents) != 1 || string(workRead.Contents[0].Blob) != "work attachment" {
					t.Fatalf("authorized work read: %#v %v", workRead, workErr)
				}
			case "scope":
				id.Scopes = nil
				accounts.Put(id)
			case "scope_alternative":
				id.Scopes = []string{mcpcontract.GmailModifyScope}
				accounts.Put(id)
			case "scope_limited":
				id.Scopes = []string{"https://www.googleapis.com/auth/drive.file"}
				accounts.Put(id)
			case "disconnect":
				accounts.Delete(id.AccountID)
			case "reconnect":
				id.Generation++
				accounts.Put(id)
			case "subject":
				id.Subject = "new-subject"
				accounts.Put(id)
			case "principal":
				id.PrincipalID = "other"
				accounts.Put(id)
			case "client":
				id.ClientName = "other"
				accounts.Put(id)
			}

			if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: ref.URI}); err == nil {
				t.Fatal("stale authorization accepted")
			}
		})
	}
}

func TestMediaResourcePrincipalIsolationAndWireBound(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		principal string
		limit     int64
		payload   int
	}{
		{"wrong-principal", "other", 0, 10},
		{"wire-limit", "fixture", 600, 512},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			accounts := fixtureAccounts()
			id, _, _ := accounts.Get(ctx, "personal")

			store := mediaartifact.New()
			t.Cleanup(store.Close)

			ref, err := store.Put(ctx, id, "gmail_get_attachment", "", "text/plain", []byte(strings.Repeat("x", tc.payload)))
			if err != nil {
				t.Fatal(err)
			}
			cfg := fixtureConfig(nil)
			cfg.Accounts = accounts
			cfg.MediaArtifacts = store
			cfg.MaxBodyBytes = tc.limit
			cfg.Principal = mcpcontract.Principal{ID: tc.principal}
			cfg.AllowOperations = []string{"gmail_get_attachment"}
			cfg.Grants = []mcpcontract.Grant{{PrincipalID: tc.principal, AccountIDs: []string{"personal"}, ClientNames: []string{"app"}, Operations: []string{"gmail:attachment"}}}

			rt, err := mcpserver.New(cfg)
			if err != nil {
				t.Fatal(err)
			}

			session := connectRuntime(t, ctx, rt)
			defer session.Close()

			if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: ref.URI}); err == nil {
				t.Fatal("resource boundary bypass")
			}
		})
	}
}

type attachmentFixtureProvider struct{ calls atomic.Int64 }

func (p *attachmentFixtureProvider) HTTPClient(_ context.Context, id mcpcontract.Identity, _ mcpcontract.CallOptions) (*http.Client, error) {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		p.calls.Add(1)
		raw := []byte("attachment for " + id.AccountID)
		payload, _ := json.Marshal(map[string]any{"size": len(raw), "data": base64.RawURLEncoding.EncodeToString(raw)})

		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(payload))), Request: req}, nil
	})}, nil
}

func TestMediaToolArtifactTwoAccounts(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := mediaartifact.New()
	t.Cleanup(store.Close)
	provider := &attachmentFixtureProvider{}
	cfg := fixtureConfig(media.OperationsWithArtifacts(provider, store))
	cfg.MediaArtifacts = store
	cfg.DiscoveryMode = mcpserver.DiscoveryCompact
	cfg.AllowOperations = []string{"gmail_get_attachment"}
	cfg.Grants = []mcpcontract.Grant{{PrincipalID: "fixture", AccountIDs: []string{"personal", "work"}, ClientNames: []string{"app"}, Operations: []string{"gmail:attachment"}}}

	rt, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, ctx, rt)
	defer session.Close()

	for _, account := range []string{"personal", "work"} {
		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{"name": "gmail_get_attachment", "arguments": map[string]any{"account_id": account, "message_id": "m1", "attachment_id": "a1"}}})
		if callErr != nil || result.IsError {
			t.Fatalf("attachment: %#v %v", result, callErr)
		}

		wire, marshalErr := json.Marshal(result.StructuredContent)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}

		var decoded struct {
			Data struct {
				Artifact mcpcontract.MediaReference `json:"artifact"`
				Inline   string                     `json:"data"`
			} `json:"data"`
		}
		if decodeErr := json.Unmarshal(wire, &decoded); decodeErr != nil {
			t.Fatal(decodeErr)
		}

		if decoded.Data.Artifact.URI == "" || decoded.Data.Inline != "" {
			t.Fatalf("expected artifact reference: %s", wire)
		}
		before := provider.calls.Load()

		resource, readErr := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: decoded.Data.Artifact.URI})
		if readErr != nil || len(resource.Contents) != 1 || string(resource.Contents[0].Blob) != "attachment for "+account {
			t.Fatalf("resource: %#v %v", resource, readErr)
		}

		if provider.calls.Load() != before {
			t.Fatal("artifact read called provider")
		}
	}
	before := provider.calls.Load()

	denied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "capabilities_execute", Arguments: map[string]any{"name": "gmail_get_attachment", "arguments": map[string]any{"account_id": "secret", "message_id": "m1", "attachment_id": "a1"}}})
	if err == nil && !denied.IsError {
		t.Fatal("unauthorized account accepted")
	}

	if provider.calls.Load() != before {
		t.Fatal("denied account reached provider")
	}
}

func TestMediaMissingResourceDoesNotReflectUnboundedURI(t *testing.T) {
	t.Parallel()
	cfg := fixtureConfig(nil)
	cfg.MediaArtifacts = mediaartifact.New()
	t.Cleanup(cfg.MediaArtifacts.Close)
	cfg.MaxBodyBytes = 512

	rt, err := mcpserver.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	session := connectRuntime(t, t.Context(), rt)
	defer session.Close()
	uri := mediaartifact.URIPrefix + strings.Repeat("a", 600)

	_, err = session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
	if err == nil {
		t.Fatal("missing resource accepted")
	}

	encoded, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}

	if len(encoded) > 512 || strings.Contains(string(encoded), uri) {
		t.Fatalf("unbounded URI reflected: %d bytes", len(encoded))
	}
}
