package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mcpserver"
	"github.com/steipete/gogcli/internal/mediaartifact"
)

const httpSIGHUPMessage = "SIGHUP ignored in HTTP mode; restart gog-mcp to rotate tokens or grants"

var (
	errHTTPConfigRequired           = errors.New("gog-mcp: --http-config is required with --http-addr")
	errHTTPConfigWithoutAddr        = errors.New("gog-mcp: --http-config requires --http-addr")
	errHTTPStdioGrantFlags          = errors.New("gog-mcp: HTTP mode uses --http-config callers for grants; omit --allow-operations and --grants-file")
	errHTTPConnectMultiPrincipal    = errors.New("gog-mcp: --connect-addr requires exactly one principal_id in --http-config; refuse to bind the account page across owners")
	errHTTPConnectPrincipalMismatch = errors.New("gog-mcp: --principal does not match the single --http-config principal_id")
)

type httpServeDeps struct {
	cfgFile   config.File
	registry  accountconnect.Registry
	tokens    accountconnect.RefreshTokenStore
	lifecycle *accountconnect.Lifecycle
	provider  *googleapi.NativeProvider
	artifacts *mediaartifact.Store
	syncer    *toolSyncInvalidator
}

func validateHTTPModeFlags(flags cli) error {
	httpAddr := strings.TrimSpace(flags.HTTPAddr)
	httpConfig := strings.TrimSpace(flags.HTTPConfig)

	if httpAddr == "" {
		if httpConfig != "" {
			return errHTTPConfigWithoutAddr
		}

		return nil
	}

	if httpConfig == "" {
		return errHTTPConfigRequired
	}

	if strings.TrimSpace(flags.AllowOperations) != "" || strings.TrimSpace(flags.GrantsFile) != "" {
		return errHTTPStdioGrantFlags
	}

	return nil
}

func connectPrincipalForHTTP(flags cli, cfg mcpserver.HTTPConfig) (mcpcontract.Principal, error) {
	ids := cfg.UniquePrincipalIDs()
	if len(ids) != 1 {
		return mcpcontract.Principal{}, errHTTPConnectMultiPrincipal
	}

	flagPrincipal := strings.TrimSpace(flags.Principal)
	if flagPrincipal != "" && flagPrincipal != defaultPrincipal && flagPrincipal != ids[0] {
		return mcpcontract.Principal{}, errHTTPConnectPrincipalMismatch
	}

	return mcpcontract.Principal{ID: ids[0]}, nil
}

func serveHTTP(flags cli, logger *slog.Logger, httpCfg mcpserver.HTTPConfig, deps httpServeDeps) error {
	gateway, err := mcpserver.NewHTTPGateway(mcpserver.HTTPHandlerConfig{
		HTTP:             httpCfg,
		MediaArtifacts:   deps.artifacts,
		Name:             "gog-mcp",
		Version:          defaultVersion,
		Policies:         deps.cfgFile.Policies,
		Operations:       configuredOperationsWithMedia(flags, deps.provider, deps.artifacts),
		Accounts:         registryAccounts{registry: deps.registry},
		Logger:           logger,
		RequestTimeout:   flags.RequestTimeout,
		MaxConcurrency:   flags.MaxConcurrency,
		MaxBodyBytes:     defaultMaxBodyBytes,
		DiscoveryMode:    mcpserver.DiscoveryMode(flags.Discovery),
		EnableWrites:     flags.EnableWrites,
		MaxUpstreamCalls: flags.MaxUpstreamCalls,
	})
	if err != nil {
		return fmt.Errorf("start http mcp: %w", err)
	}

	deps.syncer.storeAll(gateway.Runtimes())

	var connectSrv *http.Server
	if strings.TrimSpace(flags.ConnectAddr) != "" {
		if verr := validateConnectAddr(flags.ConnectAddr); verr != nil {
			return verr
		}

		principal, perr := connectPrincipalForHTTP(flags, httpCfg)
		if perr != nil {
			return perr
		}

		connectSrv, err = startConnectServer(flags, principal, deps.registry, deps.tokens, deps.syncer, deps.lifecycle, logger)
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

	go ignoreHTTPSIGHUP(ctx, reload, logger)

	logger.Info("mcp http listen", "addr", flags.HTTPAddr, "path", "/mcp", "callers", len(httpCfg.Callers))

	if err := mcpserver.ListenAndServeHTTP(ctx, flags.HTTPAddr, gateway, flags.RequestTimeout); err != nil {
		return fmt.Errorf("http mcp serve: %w", err)
	}

	return nil
}

func ignoreHTTPSIGHUP(ctx context.Context, reload <-chan os.Signal, logger *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-reload:
			logger.Warn(httpSIGHUPMessage)
		}
	}
}
