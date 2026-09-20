package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/accountconnect/web"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleops"
	"github.com/steipete/gogcli/internal/googleops/apiexec"
	"github.com/steipete/gogcli/internal/googleops/authoring"
	"github.com/steipete/gogcli/internal/googleops/businessprofile"
	"github.com/steipete/gogcli/internal/googleops/mailworkflow"
	"github.com/steipete/gogcli/internal/googleops/media"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
	"github.com/steipete/gogcli/internal/mediaartifact"
	"github.com/steipete/gogcli/internal/secrets"
)

const (
	defaultPrincipal    = "local"
	defaultVersion      = "0.10.0"
	defaultMaxBodyBytes = 8 << 20
)

var (
	errAPICallBudget      = errors.New("gog-mcp: --max-upstream-calls must be between 1 and 256")
	errCompactCatalog     = errors.New("gog-mcp: --api-catalog requires --discovery=compact")
	errGrantsRequired     = errors.New("gog-mcp: --allow-operations or --grants-file is required")
	errGrantsPrincipal    = errors.New("gog-mcp: grants file principal_id does not match --principal")
	errNonLoopbackConnect = errors.New("gog-mcp: --connect-addr must be a loopback host")
)

type cli struct {
	Principal        string        `name:"principal" help:"Trusted caller principal ID" default:"local" env:"GOG_MCP_PRINCIPAL"`
	AllowOperations  string        `name:"allow-operations" help:"Comma-separated enabled MCP operations" env:"GOG_MCP_ALLOW_OPERATIONS"`
	GrantsFile       string        `name:"grants-file" help:"JSON file of caller account/client/action grants" env:"GOG_MCP_GRANTS_FILE"`
	HTTPAddr         string        `name:"http-addr" help:"Streamable HTTP listen address. Empty keeps stdio (example: 0.0.0.0:8080)" env:"GOG_MCP_HTTP_ADDR"`
	HTTPConfig       string        `name:"http-config" help:"JSON caller/token/grant file required with --http-addr" env:"GOG_MCP_HTTP_CONFIG"`
	RegistryFile     string        `name:"registry-file" help:"Persistent account registry JSON path (defaults to <config-dir>/mcp-accounts.json). Memory registries are test-only." env:"GOG_MCP_REGISTRY_FILE"`
	ClientName       string        `name:"client-name" help:"App-owned OAuth credential bucket" default:"native-mcp" env:"GOG_MCP_CLIENT_NAME"`
	ConnectAddr      string        `name:"connect-addr" help:"Optional loopback address for the account connect page" env:"GOG_MCP_CONNECT_ADDR"`
	RedirectURL      string        `name:"redirect-url" help:"Exact OAuth callback URL registered for the connect page" env:"GOG_MCP_REDIRECT_URL"`
	RequestTimeout   time.Duration `name:"request-timeout" help:"Per-tool deadline" default:"30s" env:"GOG_MCP_REQUEST_TIMEOUT"`
	MaxConcurrency   int           `name:"max-concurrency" help:"In-flight tool calls" default:"32" env:"GOG_MCP_MAX_CONCURRENCY"`
	ConnectScopes    string        `name:"connect-scopes" help:"Comma-separated additional OAuth scope URIs to offer explicitly in the account page" env:"GOG_MCP_CONNECT_SCOPES"`
	Discovery        string        `name:"discovery" help:"Tool discovery: expanded legacy tools or compact capability search" default:"expanded" enum:"expanded,compact" env:"GOG_MCP_DISCOVERY"`
	APICatalog       bool          `name:"api-catalog" help:"Load the extended Google API catalog and task workflows; requires --discovery=compact" env:"GOG_MCP_API_CATALOG"`
	EnableWrites     bool          `name:"enable-writes" help:"Permit explicitly granted write operations; disabled by default" env:"GOG_MCP_ENABLE_WRITES"`
	MaxUpstreamCalls int64         `name:"max-upstream-calls" help:"Maximum Google API attempts per tool request, including retries (1-256)" default:"32" env:"GOG_MCP_MAX_UPSTREAM_CALLS"`
	Version          bool          `name:"version" help:"Print version and exit"`
}

type grantsFile struct {
	PrincipalID     string              `json:"principal_id"`
	Grants          []mcpcontract.Grant `json:"grants"`
	AllowOperations []string            `json:"allow_operations"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	var flags cli
	parser, err := kong.New(&flags, kong.Name("gog-mcp"), kong.Description("Native Google MCP server (stdio by default; Streamable HTTP with --http-addr)."))
	if err != nil {
		logger.Error("parser", "error", err)
		return 1
	}
	if _, err := parser.Parse(args); err != nil {
		logger.Error("parse flags", "error", err)
		return 2
	}
	if flags.Version {
		_, _ = fmt.Fprintln(os.Stderr, defaultVersion)
		return 0
	}

	if err := serve(flags, logger); err != nil {
		logger.Error("gog-mcp", "error", err)
		return 1
	}

	return 0
}

func serve(flags cli, logger *slog.Logger) error {
	if flags.MaxUpstreamCalls < 1 || flags.MaxUpstreamCalls > 256 {
		return errAPICallBudget
	}
	if flags.APICatalog && flags.Discovery != string(mcpserver.DiscoveryCompact) {
		return errCompactCatalog
	}
	if strings.TrimSpace(flags.ClientName) == "" {
		flags.ClientName = "native-mcp"
	}

	if err := validateHTTPModeFlags(flags); err != nil {
		return err
	}

	httpMode := strings.TrimSpace(flags.HTTPAddr) != ""
	var httpCfg mcpserver.HTTPConfig
	var principalID string
	var principal mcpcontract.Principal
	var grants []mcpcontract.Grant
	var allow []string
	var err error

	if httpMode {
		httpCfg, err = mcpserver.LoadHTTPConfig(flags.HTTPConfig)
		if err != nil {
			return fmt.Errorf("load http config: %w", err)
		}
	} else {
		principalID = strings.TrimSpace(flags.Principal)
		if principalID == "" {
			principalID = defaultPrincipal
		}

		principal = mcpcontract.Principal{ID: principalID}

		grants, allow, err = loadGrants(flags, principalID)
		if err != nil {
			return err
		}
	}

	cfgFile, err := config.ReadConfig()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	store, err := secrets.OpenDefaultNonInteractive()
	if err != nil {
		return fmt.Errorf("open secret store: %w", err)
	}

	registry, err := openRegistry(flags.RegistryFile)
	if err != nil {
		return err
	}

	tokens, err := accountconnect.NewSecretsTokenStore(store)
	if err != nil {
		return fmt.Errorf("token store: %w", err)
	}

	lifecycle := accountconnect.NewLifecycle()
	provider, err := newClientProvider(registry, tokens, lifecycle, flags.RequestTimeout, defaultMaxBodyBytes, flags.MaxConcurrency)
	if err != nil {
		return err
	}

	syncer := &toolSyncInvalidator{inner: provider}
	var artifacts *mediaartifact.Store
	if flags.APICatalog {
		artifacts = mediaartifact.New()
		defer artifacts.Close()
	}

	if httpMode {
		return serveHTTP(flags, logger, httpCfg, httpServeDeps{
			policies:  cfgFile.Policies,
			registry:  registry,
			tokens:    tokens,
			lifecycle: lifecycle,
			provider:  provider,
			artifacts: artifacts,
			syncer:    syncer,
		})
	}

	runtime, err := mcpserver.New(mcpserver.Config{
		Name:            "gog-mcp",
		Version:         defaultVersion,
		Principal:       principal,
		Grants:          grants,
		Policies:        cfgFile.Policies,
		AllowOperations: allow,
		Operations:      configuredOperationsWithMedia(flags, provider, artifacts),
		MediaArtifacts:  artifacts,
		Accounts:        registryAccounts{registry: registry},

		Logger:           logger,
		RequestTimeout:   flags.RequestTimeout,
		MaxConcurrency:   flags.MaxConcurrency,
		MaxBodyBytes:     defaultMaxBodyBytes,
		DiscoveryMode:    mcpserver.DiscoveryMode(flags.Discovery),
		EnableWrites:     flags.EnableWrites,
		MaxUpstreamCalls: flags.MaxUpstreamCalls,
	})
	if err != nil {
		return fmt.Errorf("start mcp runtime: %w", err)
	}
	syncer.storeAll([]*mcpserver.Runtime{runtime})

	var connectSrv *http.Server
	if strings.TrimSpace(flags.ConnectAddr) != "" {
		if verr := validateConnectAddr(flags.ConnectAddr); verr != nil {
			return verr
		}

		connectSrv, err = startConnectServer(flags, principal, registry, tokens, syncer, lifecycle, logger)
		if err != nil {
			return err
		}

		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = connectSrv.Shutdown(shutdownCtx)
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)
	defer signal.Stop(reload)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-reload:
				next, nextAllow, loadErr := loadGrants(flags, principalID)
				if loadErr != nil {
					logger.Error("reload grants", "error", loadErr)
					continue
				}

				nextCfg, cfgErr := config.ReadConfig()
				if cfgErr != nil {
					logger.Error("reload policies", "error", cfgErr)
					continue
				}

				if reloadErr := runtime.ReloadAccess(access.Snapshot{Grants: next, Policies: nextCfg.Policies, AllowOperations: nextAllow}); reloadErr != nil { //nolint:contextcheck // ReloadAccess is a snapshot install, not a request.
					logger.Error("reload grants and policies", "error", reloadErr)
					continue
				}

				logger.Info("reloaded grants and policies")
			}
		}
	}()

	err = runtime.RunStdio(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("stdio mcp: %w", err)
	}

	return nil
}

func loadGrants(flags cli, principalID string) ([]mcpcontract.Grant, []string, error) {
	allow := splitCSV(flags.AllowOperations)
	if flags.GrantsFile == "" {
		if len(allow) == 0 {
			return nil, nil, errGrantsRequired
		}

		if err := (access.Snapshot{AllowOperations: allow}).Validate(); err != nil {
			return nil, nil, fmt.Errorf("validate operations: %w", err)
		}

		return nil, allow, nil
	}

	data, err := os.ReadFile(flags.GrantsFile)
	if err != nil {
		return nil, nil, fmt.Errorf("read grants file: %w", err)
	}

	var parsed grantsFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, nil, fmt.Errorf("decode grants file: %w", err)
	}
	if parsed.PrincipalID != "" && parsed.PrincipalID != principalID {
		return nil, nil, errGrantsPrincipal
	}
	if len(allow) == 0 {
		allow = parsed.AllowOperations
	}

	if err := (access.Snapshot{Grants: parsed.Grants, AllowOperations: allow}).Validate(); err != nil {
		return nil, nil, fmt.Errorf("validate operations: %w", err)
	}

	return parsed.Grants, allow, nil
}

func openRegistry(path string) (accountconnect.Registry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		dir, err := config.EnsureDir()
		if err != nil {
			return nil, fmt.Errorf("resolve account registry path: %w", err)
		}

		path = filepath.Join(dir, "mcp-accounts.json")
	}

	registry, err := accountconnect.NewFileRegistry(path)
	if err != nil {
		return nil, fmt.Errorf("account registry %s: %w (pass --registry-file to a writable JSON path; in-memory registries are test-only)", path, err)
	}

	return registry, nil
}

func startConnectServer(flags cli, principal mcpcontract.Principal, registry accountconnect.Registry, tokens accountconnect.RefreshTokenStore, invalidator accountconnect.Invalidator, lifecycle *accountconnect.Lifecycle, logger *slog.Logger) (*http.Server, error) {
	redirect := strings.TrimSpace(flags.RedirectURL)
	if redirect == "" {
		host := flags.ConnectAddr
		if strings.HasPrefix(host, ":") {
			host = "127.0.0.1" + host
		}
		redirect = "http://" + host + accountconnect.PathCallback
	}

	additionalScopes, err := configuredConnectScopes(flags.ConnectScopes)
	if err != nil {
		return nil, err
	}

	controller, err := accountconnect.NewController(accountconnect.Options{
		Registry:         registry,
		Tokens:           tokens,
		OAuth:            accountconnect.NewGoogleProvider(),
		Invalidator:      invalidator,
		RedirectURL:      redirect,
		ClientName:       flags.ClientName,
		Lifecycle:        lifecycle,
		AdditionalScopes: additionalScopes,
	})
	if err != nil {
		return nil, fmt.Errorf("oauth controller: %w", err)
	}

	renderer, err := web.NewRenderer()
	if err != nil {
		return nil, fmt.Errorf("account page renderer: %w", err)
	}

	handler, err := accountconnect.NewHandler(controller, accountconnect.HandlerOptions{Principal: principal, Renderer: renderer})
	if err != nil {
		return nil, fmt.Errorf("oauth handler: %w", err)
	}

	listenCfg := net.ListenConfig{}
	listener, err := listenCfg.Listen(context.Background(), "tcp", flags.ConnectAddr)
	if err != nil {
		return nil, fmt.Errorf("listen connect addr: %w", err)
	}

	srv := &http.Server{
		Handler:           http.MaxBytesHandler(handler, defaultMaxBodyBytes),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logger.Info("account connect page", "addr", listener.Addr().String(), "redirect_url", controller.RedirectURL())
		if serveErr := srv.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("connect page", "error", serveErr)
		}
	}()

	return srv, nil
}

func validateConnectAddr(addr string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("connect addr: %w", err)
	}

	if strings.TrimSpace(port) == "" {
		return errNonLoopbackConnect
	}

	if host == "localhost" {
		return nil
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errNonLoopbackConnect
	}

	return nil
}

type accountFencer interface {
	InvalidateAccount(accountID string)
}

type toolSyncInvalidator struct {
	inner    accountFencer
	runtimes atomic.Pointer[[]*mcpserver.Runtime]
}

func (t *toolSyncInvalidator) InvalidateAccount(accountID string) {
	if t.inner != nil {
		t.inner.InvalidateAccount(accountID)
	}
}

func (t *toolSyncInvalidator) storeAll(runtimes []*mcpserver.Runtime) {
	cloned := append([]*mcpserver.Runtime(nil), runtimes...)
	t.runtimes.Store(&cloned)
}

func (t *toolSyncInvalidator) ConnectionsChanged() {
	if runtimes := t.runtimes.Load(); runtimes != nil {
		for _, runtime := range *runtimes {
			if runtime != nil {
				runtime.ResyncTools()
			}
		}

		return
	}
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}

	return out
}

// configuredOperations preserves the original curated tool surface by default.
func configuredOperations(flags cli, provider mcpcontract.ClientProvider) []mcpcontract.Operation {
	return configuredOperationsWithMedia(flags, provider, nil)
}

func configuredOperationsWithMedia(flags cli, provider mcpcontract.ClientProvider, artifacts mcpcontract.MediaArtifacts) []mcpcontract.Operation {
	operations := googleops.Operations(provider)
	if flags.APICatalog {
		operations = append(operations, businessprofile.Operations(provider)...)
		operations = append(operations, media.OperationsWithArtifacts(provider, artifacts)...)
		operations = append(operations, authoring.Operations(provider)...)
		operations = append(operations, mailworkflow.Operations(provider)...)
		operations = append(operations, apiexec.Operations(provider)...)
	}
	return operations
}
