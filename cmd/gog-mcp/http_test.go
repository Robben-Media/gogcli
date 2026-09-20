package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/mcpserver"
)

func TestParseHTTPStartupFlags(t *testing.T) {
	t.Parallel()

	var flags cli
	parser, err := kong.New(&flags, kong.Name("gog-mcp"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := parser.Parse([]string{
		"--http-addr", "0.0.0.0:8080",
		"--http-config", "/run/secrets/http-config.json",
	}); err != nil {
		t.Fatal(err)
	}

	if flags.HTTPAddr != "0.0.0.0:8080" || flags.HTTPConfig != "/run/secrets/http-config.json" {
		t.Fatalf("%+v", flags)
	}
}

func TestValidateHTTPModeFlags(t *testing.T) {
	t.Parallel()

	if err := validateHTTPModeFlags(cli{}); err != nil {
		t.Fatal(err)
	}

	if err := validateHTTPModeFlags(cli{HTTPConfig: "/tmp/http.json"}); !errors.Is(err, errHTTPConfigWithoutAddr) {
		t.Fatalf("got %v", err)
	}

	if err := validateHTTPModeFlags(cli{HTTPAddr: "0.0.0.0:8080"}); !errors.Is(err, errHTTPConfigRequired) {
		t.Fatalf("got %v", err)
	}

	if err := validateHTTPModeFlags(cli{HTTPAddr: "0.0.0.0:8080", HTTPConfig: "/tmp/http.json", AllowOperations: "accounts_list"}); !errors.Is(err, errHTTPStdioGrantFlags) {
		t.Fatalf("got %v", err)
	}

	if err := validateHTTPModeFlags(cli{HTTPAddr: "0.0.0.0:8080", HTTPConfig: "/tmp/http.json"}); err != nil {
		t.Fatal(err)
	}
}

func TestConnectPrincipalForHTTP(t *testing.T) {
	t.Parallel()

	one := mcpserver.HTTPConfig{Callers: []mcpserver.HTTPCaller{
		{ID: "a", PrincipalID: "jeremy"},
		{ID: "b", PrincipalID: "jeremy"},
	}}
	principal, err := connectPrincipalForHTTP(cli{Principal: "local"}, one)
	if err != nil || principal.ID != "jeremy" {
		t.Fatalf("%+v %v", principal, err)
	}

	if _, err := connectPrincipalForHTTP(cli{Principal: "other"}, one); !errors.Is(err, errHTTPConnectPrincipalMismatch) {
		t.Fatalf("got %v", err)
	}

	two := mcpserver.HTTPConfig{Callers: []mcpserver.HTTPCaller{
		{ID: "a", PrincipalID: "jeremy"},
		{ID: "b", PrincipalID: "other"},
	}}
	if _, err := connectPrincipalForHTTP(cli{ConnectAddr: "127.0.0.1:8787"}, two); !errors.Is(err, errHTTPConnectMultiPrincipal) {
		t.Fatalf("got %v", err)
	}
}

func TestIgnoreHTTPSIGHUP(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	reload := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		ignoreHTTPSIGHUP(ctx, reload, logger)
		close(done)
	}()

	reload <- os.Interrupt
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SIGHUP loop did not exit")
	}

	if !strings.Contains(buf.String(), httpSIGHUPMessage) {
		t.Fatalf("log %q", buf.String())
	}
}

func TestToolSyncInvalidatorStoresAllRuntimes(t *testing.T) {
	t.Parallel()

	syncer := &toolSyncInvalidator{}
	syncer.storeAll(nil)
	syncer.ConnectionsChanged()
}
