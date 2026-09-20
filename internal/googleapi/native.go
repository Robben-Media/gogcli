package googleapi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	defaultNativeConcurrency = 16
	maxNativeConcurrency     = 256
	defaultNativeMaxBytes    = 8 << 20

	nativeAuthMessage          = "Google authentication is required"
	nativeOAuthMessage         = "Google OAuth is required"
	nativeFailMessage          = "Google request failed"
	nativeDeadlineMessage      = "request deadline was exceeded"
	nativeCanceledMessage      = "request was canceled"
	nativeScopeMessage         = "additional Google OAuth scope is required"
	nativeQuotaMessage         = "Google quota was exhausted"
	nativeForbiddenMessage     = "Google denied access to this resource"
	nativeNotFoundMessage      = "Google resource was not found"
	nativeInvalidGoogleMessage = "Google rejected the request"
	nativeWriteUnknownMessage  = "Google write outcome is unknown"
)

var (
	errNilNativeRegistry     = errors.New("googleapi: account registry is required")
	errNilNativeTokens       = errors.New("googleapi: token store is required")
	errNilNativeCredentials  = errors.New("googleapi: credentials resolver is required")
	errInvalidNativeMaxBytes = errors.New("googleapi: max response bytes must be non-negative")
	errInvalidNativeConc     = errors.New("googleapi: max concurrency must be non-negative")
	errNativeRedirect        = errors.New("googleapi: refusing to follow HTTP redirect")
)

// RefreshTokenStore is the startup-opened refresh-token adapter.
// Callers must not open the secret store per request.
type RefreshTokenStore interface {
	Get(ctx context.Context, clientName, email string) (token string, scopes []string, err error)
	Put(ctx context.Context, clientName, email, token string, scopes []string) error
}

// NativeOptions constructs an OAuth-only Google client provider.
type NativeOptions struct {
	Registry         accountconnect.Registry
	Tokens           RefreshTokenStore
	Credentials      func(clientName string) (clientID, clientSecret string, err error)
	Endpoint         oauth2.Endpoint
	RefreshHTTP      *http.Client
	MaxConcurrency   int
	MaxResponseBytes int64
	RequestTimeout   time.Duration
	Now              func() time.Time
	Lifecycle        *accountconnect.Lifecycle
}

// NativeProvider caches identity-scoped token sources and HTTP transports.
type NativeProvider struct {
	registry    accountconnect.Registry
	tokens      RefreshTokenStore
	credentials func(clientName string) (clientID, clientSecret string, err error)
	endpoint    oauth2.Endpoint
	refreshHTTP *http.Client
	slots       chan struct{}
	maxBody     int64
	timeout     time.Duration
	now         func() time.Time
	pool        http.RoundTripper
	lifecycle   *accountconnect.Lifecycle

	mu     sync.Mutex
	cache  map[string]*nativeCachedClient
	fences map[string]uint64
}

type nativeCachedClient struct {
	source *nativeTokenSource
}

// NewNativeProvider validates injected dependencies. It never opens the secret store.
func NewNativeProvider(opts NativeOptions) (*NativeProvider, error) {
	if opts.Registry == nil {
		return nil, errNilNativeRegistry
	}

	if opts.Tokens == nil {
		return nil, errNilNativeTokens
	}

	if opts.Credentials == nil {
		return nil, errNilNativeCredentials
	}

	endpoint := opts.Endpoint
	if strings.TrimSpace(endpoint.TokenURL) == "" {
		endpoint = google.Endpoint
	}

	timeout := opts.RequestTimeout
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}

	if opts.MaxResponseBytes < 0 {
		return nil, errInvalidNativeMaxBytes
	}

	maxBody := opts.MaxResponseBytes
	if maxBody == 0 {
		maxBody = defaultNativeMaxBytes
	}

	if opts.MaxConcurrency < 0 {
		return nil, errInvalidNativeConc
	}

	concurrency := opts.MaxConcurrency
	if concurrency == 0 {
		concurrency = defaultNativeConcurrency
	}

	if concurrency > maxNativeConcurrency {
		concurrency = maxNativeConcurrency
	}

	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	refreshHTTP := opts.RefreshHTTP
	if refreshHTTP == nil {
		refreshHTTP = newNativeRefreshClient(timeout, maxBody)
	}

	return &NativeProvider{
		registry:    opts.Registry,
		tokens:      opts.Tokens,
		credentials: opts.Credentials,
		endpoint:    endpoint,
		refreshHTTP: refreshHTTP,
		slots:       make(chan struct{}, concurrency),
		maxBody:     maxBody,
		timeout:     timeout,
		now:         now,
		pool:        &nativeBodyLimitTransport{base: cloneNativeTransport(), max: maxBody},
		lifecycle:   opts.Lifecycle,
		cache:       make(map[string]*nativeCachedClient),
		fences:      make(map[string]uint64),
	}, nil
}

// HTTPClient returns an OAuth client for the resolved identity and catalog operation.
func (p *NativeProvider) HTTPClient(ctx context.Context, id mcpcontract.Identity, opts mcpcontract.CallOptions) (*http.Client, error) {
	if ctx == nil {
		return nil, &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "request context is required"}
	}

	if err := ctx.Err(); err != nil {
		return nil, NativePublicError(err)
	}

	def, err := nativeCallDefinition(opts)
	if err != nil {
		return nil, err
	}

	err = nativeValidateIdentity(id)
	if err != nil {
		return nil, err
	}

	err = p.lockLifecycle(ctx)
	if err != nil {
		return nil, NativePublicError(err)
	}

	rec, err := p.liveRecord(ctx, id)
	if err != nil {
		p.unlockLifecycle()

		return nil, err
	}

	if !mcpcontract.ScopesSatisfied(rec.Scopes, def) {
		p.unlockLifecycle()
		return nil, &mcpcontract.Error{Category: mcpcontract.InsufficientScope, Message: nativeScopeMessage}
	}

	cached, err := p.cachedClient(ctx, rec)
	p.unlockLifecycle()

	if err != nil {
		return nil, err
	}

	return &http.Client{
		Transport: &nativeAuthTransport{
			source: cached.source,
			base: &nativeSlotTransport{
				slots: p.slots,
				base: &NativeRetryTransport{
					Base:          p.pool,
					Class:         def.Retry,
					MaxRetries429: MaxRateLimitRetries,
					MaxRetries5xx: Max5xxRetries,
					BaseDelay:     RateLimitBaseDelay,
					now:           p.now,
				},
			},
		},
		Timeout:       p.timeout,
		CheckRedirect: rejectNativeRedirect,
	}, nil
}

// InvalidateAccount drops cached clients and token sources for every generation of accountID.
// It also fences the account so an in-flight TokenSource cannot persist a rotated refresh token.
func (p *NativeProvider) InvalidateAccount(accountID string) {
	if p == nil || accountID == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.fences[accountID]++
	for key, cached := range p.cache {
		if cached == nil || cached.source == nil || cached.source.accountID != accountID {
			continue
		}

		cached.source.invalidate()
		delete(p.cache, key)
	}
}

func (p *NativeProvider) accountFence(accountID string) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.fences[accountID]
}

func (p *NativeProvider) accountFenced(accountID string, fence uint64) bool {
	return p.accountFence(accountID) != fence
}

func (p *NativeProvider) lockLifecycle(ctx context.Context) error {
	if p == nil {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("googleapi: lifecycle: %w", err)
		}

		return nil
	}

	if err := p.lifecycle.Lock(ctx); err != nil {
		return fmt.Errorf("googleapi: lifecycle: %w", err)
	}

	if err := ctx.Err(); err != nil {
		p.lifecycle.Unlock()

		return fmt.Errorf("googleapi: lifecycle: %w", err)
	}

	return nil
}

func (p *NativeProvider) unlockLifecycle() {
	if p != nil {
		p.lifecycle.Unlock()
	}
}

func nativeCallDefinition(opts mcpcontract.CallOptions) (mcpcontract.Definition, error) {
	def, ok := mcpcontract.Lookup(opts.Operation)
	if !ok || def.Local {
		return mcpcontract.Definition{}, &mcpcontract.Error{
			Category:  mcpcontract.InvalidInput,
			Message:   "unknown Google operation",
			Retryable: false,
		}
	}

	if opts.Retry != def.Retry {
		return mcpcontract.Definition{}, &mcpcontract.Error{
			Category:  mcpcontract.InvalidInput,
			Message:   "retry class does not match the operation catalog",
			Retryable: false,
		}
	}

	return def, nil
}

func nativeValidateIdentity(id mcpcontract.Identity) error {
	if strings.TrimSpace(id.AccountID) == "" {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "account_id is required"}
	}

	if strings.TrimSpace(id.ClientName) == "" {
		return &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "client_name is required"}
	}

	if id.AuthMode != accountconnect.AuthModeOAuth {
		return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeOAuthMessage}
	}

	if strings.TrimSpace(id.Email) == "" || strings.TrimSpace(id.Subject) == "" {
		return &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	return nil
}

func (p *NativeProvider) liveRecord(ctx context.Context, id mcpcontract.Identity) (accountconnect.Record, error) {
	rec, ok, err := p.registry.Get(ctx, id.AccountID)
	if err != nil {
		return accountconnect.Record{}, NativePublicError(err)
	}

	if !ok {
		return accountconnect.Record{}, &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	if rec.Generation != id.Generation || rec.Subject != id.Subject || rec.ClientName != id.ClientName || rec.Email != id.Email || rec.AuthMode != id.AuthMode || rec.PrincipalID != id.PrincipalID {
		return accountconnect.Record{}, &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	if rec.AuthMode != accountconnect.AuthModeOAuth {
		return accountconnect.Record{}, &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeOAuthMessage}
	}

	if !rec.IsActive() {
		return accountconnect.Record{}, &mcpcontract.Error{Category: mcpcontract.AuthRequired, Message: nativeAuthMessage}
	}

	return rec, nil
}

func identityCacheKey(id mcpcontract.Identity) string {
	scopes := append([]string(nil), id.Scopes...)
	sort.Strings(scopes)

	return strings.Join([]string{
		id.AccountID,
		id.Subject,
		id.Email,
		id.PrincipalID,
		id.ClientName,
		id.AuthMode,
		strings.Join(scopes, " "),
		strconv.FormatUint(id.Generation, 10),
	}, "\x00")
}

func rejectNativeRedirect(_ *http.Request, _ []*http.Request) error {
	return errNativeRedirect
}

func cloneNativeTransport() *http.Transport {
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		cloned := defaultTransport.Clone()
		if cloned.TLSClientConfig == nil {
			cloned.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		} else if cloned.TLSClientConfig.MinVersion < tls.VersionTLS12 {
			cloned.TLSClientConfig = cloned.TLSClientConfig.Clone()
			cloned.TLSClientConfig.MinVersion = tls.VersionTLS12
		}

		return cloned
	}

	return &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
}

func newNativeRefreshClient(timeout time.Duration, maxBody int64) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     &nativeBodyLimitTransport{base: cloneNativeTransport(), max: maxBody},
		CheckRedirect: rejectNativeRedirect,
	}
}
