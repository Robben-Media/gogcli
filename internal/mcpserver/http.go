package mcpserver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/steipete/gogcli/internal/access"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/mediaartifact"
)

const (
	httpMCPPath             = "/mcp"
	httpUnauthorizedMessage = "unauthorized"
	httpForbiddenMessage    = "forbidden"
	httpBusyMessage         = "too many requests"
	minHTTPBearerBytes      = 32
	defaultHTTPGlobalSlots  = 64
	httpShutdownTimeout     = 5 * time.Second
	httpReadHeaderTimeout   = 10 * time.Second
	httpIdleTimeout         = 60 * time.Second
	httpBearerPrefix        = "Bearer "
)

type httpCallerContextKey struct{}

// HTTPHandlerConfig is trusted process configuration for Streamable HTTP.
// Caller identity is taken only from the hashed bearer token.
type HTTPHandlerConfig struct {
	HTTP              HTTPConfig
	MediaArtifacts    *mediaartifact.Store
	Name              string
	Version           string
	Policies          []config.Policy
	Operations        []mcpcontract.Operation
	Accounts          access.AccountSource
	Logger            *slog.Logger
	RequestTimeout    time.Duration
	MaxConcurrency    int
	GlobalConcurrency int
	MaxBodyBytes      int64
	DiscoveryMode     DiscoveryMode
	EnableWrites      bool
	MaxUpstreamCalls  int64
}

// HTTPGateway serves authenticated, stateless Streamable HTTP at /mcp.
type HTTPGateway struct {
	host           string
	allowedOrigins map[string]struct{}
	callers        []*httpCallerState
	mcp            *mcp.StreamableHTTPHandler
	requestTimeout time.Duration
	globalSlots    chan struct{}
}

type httpCallerState struct {
	tokenSHA256 [sha256ByteLength]byte
	runtime     *Runtime
	slots       chan struct{}
}

// NewHTTPGateway builds one Runtime per caller, sharing operations, the
// account registry, and the Google provider supplied through Operations.
func NewHTTPGateway(cfg HTTPHandlerConfig) (*HTTPGateway, error) {
	validated, err := cfg.HTTP.validated()
	if err != nil {
		return nil, err
	}

	cfg.HTTP = validated
	if cfg.MaxBodyBytes < 0 {
		return nil, errMaxBodyBytes
	}

	if cfg.Accounts == nil {
		return nil, errAccountSource
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}

	maxConcurrency := cfg.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = defaultMaxConcurrency
	}

	globalConcurrency := cfg.GlobalConcurrency
	if globalConcurrency <= 0 {
		globalConcurrency = defaultHTTPGlobalSlots
	}

	maxBody := cfg.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = defaultMaxBodyBytes
	}

	origins := make(map[string]struct{}, len(cfg.HTTP.AllowedOrigins))
	for _, origin := range cfg.HTTP.AllowedOrigins {
		origins[origin] = struct{}{}
	}

	gateway := &HTTPGateway{
		host:           cfg.HTTP.Host,
		allowedOrigins: origins,
		callers:        make([]*httpCallerState, 0, len(cfg.HTTP.Callers)),
		requestTimeout: timeout,
		globalSlots:    make(chan struct{}, globalConcurrency),
	}

	principalSlots := make(map[string]chan struct{})
	for _, caller := range cfg.HTTP.Callers {
		if principalSlots[caller.PrincipalID] == nil {
			principalSlots[caller.PrincipalID] = make(chan struct{}, maxConcurrency)
		}

		runtime, err := New(Config{
			MediaArtifacts:   cfg.MediaArtifacts,
			Name:             cfg.Name,
			Version:          cfg.Version,
			Principal:        mcpcontract.Principal{ID: caller.PrincipalID},
			Grants:           caller.Grants,
			Policies:         cfg.Policies,
			AllowOperations:  caller.AllowOperations,
			Operations:       cfg.Operations,
			Accounts:         cfg.Accounts,
			Logger:           logger.With("caller_id", caller.ID),
			RequestTimeout:   timeout,
			MaxConcurrency:   maxConcurrency,
			MaxBodyBytes:     maxBody,
			DiscoveryMode:    cfg.DiscoveryMode,
			EnableWrites:     cfg.EnableWrites,
			MaxUpstreamCalls: cfg.MaxUpstreamCalls,
		})
		if err != nil {
			return nil, fmt.Errorf("mcpserver: http caller %s: %w", caller.ID, err)
		}

		state := &httpCallerState{
			tokenSHA256: caller.TokenSHA256,
			runtime:     runtime,
			slots:       principalSlots[caller.PrincipalID],
		}
		gateway.callers = append(gateway.callers, state)
	}

	gateway.mcp = mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		caller, ok := callerFromRequest(req)
		if !ok {
			return nil
		}

		return caller.runtime.Server()
	}, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		Logger:                       logger,
		DisableLocalhostProtection:   true,
		MaxRequestBodyBytes:          maxBody,
		PropagateRequestCancellation: true,
	})

	return gateway, nil
}

// Runtimes returns the per-caller runtimes in config order.
func (g *HTTPGateway) Runtimes() []*Runtime {
	if g == nil {
		return nil
	}

	out := make([]*Runtime, len(g.callers))
	for i, caller := range g.callers {
		out[i] = caller.runtime
	}

	return out
}

func (g *HTTPGateway) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req == nil {
		writeUnauthorized(w)
		return
	}

	if req.Host != g.host {
		http.Error(w, httpForbiddenMessage, http.StatusForbidden)
		return
	}

	if !g.originAllowed(req) {
		http.Error(w, httpForbiddenMessage, http.StatusForbidden)
		return
	}

	caller, ok := g.authenticate(req)
	if !ok {
		writeUnauthorized(w)
		return
	}

	if req.URL == nil || req.URL.Path != httpMCPPath {
		http.NotFound(w, req)
		return
	}

	if !tryAcquire(g.globalSlots) {
		http.Error(w, httpBusyMessage, http.StatusTooManyRequests)
		return
	}
	defer releaseSlot(g.globalSlots)

	if !tryAcquire(caller.slots) {
		http.Error(w, httpBusyMessage, http.StatusTooManyRequests)
		return
	}
	defer releaseSlot(caller.slots)

	ctx, cancel := context.WithTimeout(req.Context(), g.requestTimeout)
	defer cancel()

	g.mcp.ServeHTTP(w, req.WithContext(withHTTPCaller(ctx, caller)))
}

func (g *HTTPGateway) originAllowed(req *http.Request) bool {
	origin := strings.TrimSpace(req.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	_, ok := g.allowedOrigins[origin]

	return ok
}

func (g *HTTPGateway) authenticate(req *http.Request) (*httpCallerState, bool) {
	token, ok := bearerToken(req.Header.Get("Authorization"))
	if !ok {
		return nil, false
	}

	sum := sha256.Sum256([]byte(token))

	var matched *httpCallerState

	for _, caller := range g.callers {
		if hmac.Equal(sum[:], caller.tokenSHA256[:]) {
			matched = caller
		}
	}

	return matched, matched != nil
}

func bearerToken(header string) (string, bool) {
	if len(header) < len(httpBearerPrefix) || !strings.EqualFold(header[:len(httpBearerPrefix)], httpBearerPrefix) {
		return "", false
	}

	token := strings.TrimSpace(header[len(httpBearerPrefix):])
	if len(token) < minHTTPBearerBytes {
		return "", false
	}

	return token, true
}

func withHTTPCaller(ctx context.Context, caller *httpCallerState) context.Context {
	return context.WithValue(ctx, httpCallerContextKey{}, caller)
}

func callerFromRequest(req *http.Request) (*httpCallerState, bool) {
	if req == nil {
		return nil, false
	}

	caller, ok := req.Context().Value(httpCallerContextKey{}).(*httpCallerState)

	return caller, ok && caller != nil && caller.runtime != nil
}

func tryAcquire(slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseSlot(slots chan struct{}) {
	select {
	case <-slots:
	default:
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, httpUnauthorizedMessage, http.StatusUnauthorized)
}

// ListenAndServeHTTP serves handler on addr until ctx is cancelled.
// The process is not tied to stdin. Cancellation stops active requests, then
// shutdown waits up to five seconds for handlers to exit.
func ListenAndServeHTTP(ctx context.Context, addr string, handler http.Handler, requestTimeout time.Duration) error {
	if strings.TrimSpace(addr) == "" {
		return fmt.Errorf("%w: listen address is required", ErrHTTPConfigInvalid)
	}

	if handler == nil {
		return fmt.Errorf("%w: handler is required", ErrHTTPConfigInvalid)
	}

	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}

	listenCfg := net.ListenConfig{}

	listener, err := listenCfg.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("mcpserver: http listen: %w", err)
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       requestTimeout,
		WriteTimeout:      requestTimeout,
		IdleTimeout:       httpIdleTimeout,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownCtx) //nolint:contextcheck // shutdown uses a bounded independent timer
		serveErr := <-errCh

		if shutdownErr != nil {
			return fmt.Errorf("mcpserver: http shutdown: %w", shutdownErr)
		}

		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("mcpserver: http serve: %w", serveErr)
		}

		return nil
	case serveErr := <-errCh:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("mcpserver: http serve: %w", serveErr)
		}

		return nil
	}
}
