package mcpserver_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
)

// TestNativeStdioHelper is only entered by the fixture subprocess below. It uses
// the real RunStdio transport, without opening a keyring or calling Google.
func TestNativeStdioHelper(t *testing.T) {
	if os.Getenv("GOG_MCP_TEST_STDIO") != "1" {
		t.Skip("fixture subprocess entrypoint")
	}

	provider := &countingProvider{}

	runtime, err := mcpserver.New(fixtureConfig([]mcpcontract.Operation{fakeSearch(provider)}))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = runtime.RunStdio(ctx)

	cancel()

	if err != nil {
		t.Fatal(err)
	}

	// The testing runner's PASS text is not part of the MCP process protocol.
	os.Exit(0)
}

func TestNativeStdioProcess(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestNativeStdioHelper$")

	command.Env = append(os.Environ(), "GOG_MCP_TEST_STDIO=1")
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics

	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = command.Start(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = stdin.Close()
		_ = stdout.Close()

		cancel()
		_ = command.Wait()
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-process-fixture", Version: "1"}, nil)

	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: stdout, Writer: stdin}, nil)
	if err != nil {
		t.Fatal(err)
	}

	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 2 {
		t.Fatalf("stdio discovery: %v, %v", tools, err)
	}

	result, err := session.CallTool(ctx, searchParams("fixture"))
	if err != nil || result.IsError {
		t.Fatalf("stdio call: %v, %v", result, err)
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	if err := command.Wait(); err != nil {
		t.Fatalf("stdio exit: %v; stderr: %s", err, diagnostics.String())
	}

	if !bytes.Contains(diagnostics.Bytes(), []byte("mcp tool ok")) {
		t.Fatalf("tool log missing from stderr: %s", diagnostics.String())
	}
}
