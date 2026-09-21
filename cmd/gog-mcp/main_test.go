package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/googleops/gmail"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

func TestLoadGrantsRequiresAllowOrFile(t *testing.T) {
	t.Parallel()

	_, _, err := loadGrants(cli{}, "local")
	if !errors.Is(err, errGrantsRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestLoadGrantsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "grants.json")
	body, err := json.Marshal(grantsFile{
		PrincipalID:     "local",
		AllowOperations: []string{"accounts_list", "gmail_get_message"},
		Grants: []mcpcontract.Grant{{
			PrincipalID: "local",
			AccountIDs:  []string{"personal"},
			ClientNames: []string{"default"},
			Operations:  []string{"gmail:get"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	writeErr := os.WriteFile(path, body, 0o600)
	if writeErr != nil {
		t.Fatal(writeErr)
	}

	grants, allow, err := loadGrants(cli{GrantsFile: path}, "local")
	if err != nil {
		t.Fatal(err)
	}

	if len(grants) != 1 || grants[0].AccountIDs[0] != "personal" {
		t.Fatalf("grants %#v", grants)
	}

	if len(allow) != 2 || allow[0] != "accounts_list" {
		t.Fatalf("allow %#v", allow)
	}
}

func TestLoadGrantsPrincipalMismatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "grants.json")
	if err := os.WriteFile(path, []byte(`{"principal_id":"other","allow_operations":["accounts_list"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := loadGrants(cli{GrantsFile: path}, "local")
	if !errors.Is(err, errGrantsPrincipal) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateConnectAddr(t *testing.T) {
	t.Parallel()

	if err := validateConnectAddr("127.0.0.1:8787"); err != nil {
		t.Fatal(err)
	}

	if err := validateConnectAddr("localhost:8787"); err != nil {
		t.Fatal(err)
	}

	if err := validateConnectAddr("0.0.0.0:8787"); !errors.Is(err, errNonLoopbackConnect) {
		t.Fatalf("got %v", err)
	}

	if err := validateConnectAddr(":8787"); !errors.Is(err, errNonLoopbackConnect) {
		t.Fatalf("empty host got %v", err)
	}
}

func TestParseStartupFlags(t *testing.T) {
	t.Parallel()

	var flags cli
	parser, err := kong.New(&flags, kong.Name("gog-mcp"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := parser.Parse([]string{
		"--principal", "fixture",
		"--allow-operations", "accounts_list,gmail_get_message",
		"--grants-file", "/tmp/grants.json",
		"--registry-file", "/tmp/accounts.json",
		"--client-name", "default",
		"--connect-addr", "127.0.0.1:8787",
		"--redirect-url", "http://127.0.0.1:8787/oauth/callback",
	}); err != nil {
		t.Fatal(err)
	}

	if flags.Principal != "fixture" || flags.ClientName != "default" {
		t.Fatalf("%+v", flags)
	}
}

func TestDefaultCredentialBucketIsNativeMCP(t *testing.T) {
	t.Setenv("GOG_MCP_CLIENT_NAME", "")
	if err := os.Unsetenv("GOG_MCP_CLIENT_NAME"); err != nil {
		t.Fatal(err)
	}
	var flags cli
	parser, err := kong.New(&flags, kong.Name("gog-mcp"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if flags.ClientName != "native-mcp" {
		t.Fatalf("native server selected credential bucket %q", flags.ClientName)
	}
}

func TestOpenRegistryDefaultsToPersistentFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	registry, err := openRegistry("")
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := registry.(*accountconnect.MemoryRegistry); ok {
		t.Fatal("production serve must not use a memory registry")
	}
}

func TestOpenRegistryExplicitPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "accounts.json")
	registry, err := openRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := registry.(*accountconnect.MemoryRegistry); ok {
		t.Fatal("explicit path must be a file registry")
	}
}

func TestLoadGrantsUnknownOperation(t *testing.T) {
	t.Parallel()

	_, _, err := loadGrants(cli{AllowOperations: "gmail_serach"}, "local")
	if err == nil {
		t.Fatal("unknown allow operation must fail")
	}
}

func TestRuntimePublishedBeforeConnectListener(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	registry := accountconnect.NewMemoryRegistry()
	syncer := &toolSyncInvalidator{}
	runtime, err := mcpserver.New(mcpserver.Config{
		Principal: mcpcontract.Principal{ID: "fixture"},
		Grants: []mcpcontract.Grant{{
			PrincipalID: "fixture", AccountIDs: []string{"account"}, ClientNames: []string{"native-mcp"},
			Operations: []string{"gmail_search"},
		}},
		AllowOperations: []string{"accounts_list", "gmail_search"},
		Operations:      gmail.Operations(nil),
		Accounts:        registryAccounts{registry: registry},
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	syncer.storeAll([]*mcpserver.Runtime{runtime})
	if stored := syncer.runtimes.Load(); stored == nil || len(*stored) != 1 || (*stored)[0] != runtime {
		t.Fatal("runtime was not published before connect startup")
	}

	if err = registry.Upsert(ctx, accountconnect.Record{
		AccountID: "account", Subject: "subject-account", Email: "account@example.test", Label: "Account",
		PrincipalID: "fixture", ClientName: "native-mcp", AuthMode: accountconnect.AuthModeOAuth,
		Scopes: []string{mcpcontract.GmailReadScope}, Generation: 1, State: accountconnect.RecordStateActive,
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	syncer.ConnectionsChanged()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := runtime.Server().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "startup-fixture", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sawGmailSearch bool
	for _, tool := range tools.Tools {
		sawGmailSearch = sawGmailSearch || tool.Name == "gmail_search"
	}
	if !sawGmailSearch {
		t.Fatalf("connect event dropped after runtime publication: tools %#v", tools.Tools)
	}
}
