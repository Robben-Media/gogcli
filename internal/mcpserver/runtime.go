package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

// Runtime is the stdio MCP adapter. HTTP and write tools stay disabled here.
type Runtime struct {
	principal      mcpcontract.Principal
	authorizer     *access.Authorizer
	server         *mcp.Server
	logger         *slog.Logger
	operations     []mcpcontract.Operation
	requestTimeout time.Duration
	maxBodyBytes   int64
	slots          chan struct{}
	toolsMu        sync.Mutex
	registered     map[string]struct{}
}

func New(cfg Config) (*Runtime, error) {
	principalID := strings.TrimSpace(cfg.Principal.ID)
	if principalID == "" {
		return nil, errPrincipalRequired
	}

	if cfg.Accounts == nil {
		return nil, errAccountSource
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = defaultName
	}

	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		version = defaultVersion
	}

	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}

	maxConcurrency := cfg.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = defaultMaxConcurrency
	}

	maxBody := cfg.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = defaultMaxBodyBytes
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	authorizer, err := access.NewAuthorizer(cfg.snapshot(), cfg.Accounts)
	if err != nil {
		return nil, fmt.Errorf("mcpserver: authorizer: %w", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: name, Version: version}, &mcp.ServerOptions{
		Logger:       logger,
		Capabilities: &mcp.ServerCapabilities{},
	})
	RegisterWorkflowResources(server)

	runtime := &Runtime{
		principal:      mcpcontract.Principal{ID: principalID},
		authorizer:     authorizer,
		server:         server,
		logger:         logger,
		operations:     append([]mcpcontract.Operation(nil), cfg.Operations...),
		requestTimeout: timeout,
		maxBodyBytes:   maxBody,
		slots:          make(chan struct{}, maxConcurrency),
		registered:     make(map[string]struct{}),
	}
	if err := runtime.syncTools(); err != nil {
		return nil, err
	}

	return runtime, nil
}

func (rt *Runtime) Server() *mcp.Server {
	return rt.server
}

func (rt *Runtime) Run(ctx context.Context, transport mcp.Transport) error {
	if err := rt.server.Run(ctx, transport); err != nil {
		return fmt.Errorf("mcp session: %w", err)
	}

	return nil
}

func (rt *Runtime) RunStdio(ctx context.Context) error {
	transport := &mcp.StdioTransport{MaxLineLength: int(rt.maxBodyBytes)}

	return rt.Run(ctx, transport)
}

func (rt *Runtime) ReplaceAccess(snapshot access.Snapshot) {
	if err := rt.ReloadAccess(snapshot); err != nil {
		rt.logger.Error("reload access", "error", err)
	}
}

// ReloadAccess validates the candidate snapshot against the live registry
// before publishing it. On failure the previous snapshot and tool catalog stay.
func (rt *Runtime) ReloadAccess(snapshot access.Snapshot) error {
	rt.toolsMu.Lock()
	defer rt.toolsMu.Unlock()

	probe, err := rt.authorizer.Preview(snapshot)
	if err != nil {
		return fmt.Errorf("preview access: %w", err)
	}

	wanted, wantedSet, err := rt.collectWanted(probe)
	if err != nil {
		return err
	}

	rt.authorizer.Replace(snapshot)

	return rt.applyWanted(wanted, wantedSet)
}

// ResyncTools rebuilds the advertised tool list from the current snapshot and
// registry. Call after a successful connect or disconnect so discovery matches
// durable account state.
func (rt *Runtime) ResyncTools() {
	if err := rt.syncTools(); err != nil {
		rt.logger.Error("tool discovery", "error", err)
	}
}

func (rt *Runtime) syncTools() error {
	rt.toolsMu.Lock()
	defer rt.toolsMu.Unlock()

	wanted, wantedSet, err := rt.collectWanted(rt.authorizer)
	if err != nil {
		return err
	}

	return rt.applyWanted(wanted, wantedSet)
}

func (rt *Runtime) collectWanted(authorizer *access.Authorizer) ([]string, map[string]struct{}, error) {
	wanted := make([]string, 0, len(rt.operations)+1)
	wantedSet := make(map[string]struct{}, len(rt.operations)+1)

	visible, err := authorizer.Visible(rt.principal, accountsListName)
	if err != nil {
		return nil, nil, fmt.Errorf("account tool visibility: %w", err)
	}

	if visible {
		wanted = append(wanted, accountsListName)
		wantedSet[accountsListName] = struct{}{}
	}

	for _, operation := range rt.operations {
		if operation.Definition.Local {
			continue
		}

		ok, visErr := authorizer.Visible(rt.principal, operation.Definition.Name)
		if visErr != nil {
			return nil, nil, fmt.Errorf("google tool visibility: %w", visErr)
		}

		if ok {
			wanted = append(wanted, operation.Definition.Name)
			wantedSet[operation.Definition.Name] = struct{}{}
		}
	}

	return wanted, wantedSet, nil
}

func (rt *Runtime) applyWanted(wanted []string, wantedSet map[string]struct{}) error {
	for _, name := range wanted {
		if _, ok := rt.registered[name]; ok {
			continue
		}

		rt.addNamedTool(name)
		rt.registered[name] = struct{}{}
	}

	for name := range rt.registered {
		if _, ok := wantedSet[name]; ok {
			continue
		}

		rt.server.RemoveTools(name)
		delete(rt.registered, name)
	}

	return nil
}

func (rt *Runtime) addNamedTool(name string) {
	if name == accountsListName {
		input, output := accountsListSchemas()
		def, _ := mcpcontract.Lookup(accountsListName)
		rt.server.AddTool(&mcp.Tool{
			Name:         def.Name,
			Description:  def.Description,
			InputSchema:  input,
			OutputSchema: output,
			Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: true, Title: "List Google accounts"},
		}, rt.handleAccountsList)

		return
	}

	for _, operation := range rt.operations {
		if operation.Definition.Name == name {
			rt.server.AddTool(rt.toolFor(operation), rt.handlerFor(operation))
			return
		}
	}
}

func (rt *Runtime) toolFor(operation mcpcontract.Operation) *mcp.Tool {
	readOnly := true

	return &mcp.Tool{
		Name:         operation.Definition.Name,
		Description:  operation.Definition.Description,
		InputSchema:  operation.InputSchema,
		OutputSchema: operation.OutputSchema,
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: readOnly, Title: operation.Definition.Name},
	}
}

func (rt *Runtime) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := rt.requestTimeout

	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return context.WithTimeout(ctx, time.Nanosecond)
		}

		if remaining < timeout {
			timeout = remaining
		}
	}

	return context.WithTimeout(ctx, timeout)
}

func (rt *Runtime) acquire(ctx context.Context) error {
	select {
	case rt.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("acquire slot: %w", ctx.Err())
	}
}

func (rt *Runtime) release() {
	select {
	case <-rt.slots:
	default:
	}
}

func newTraceID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "trace"
	}

	return hex.EncodeToString(buf[:])
}
