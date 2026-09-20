package accountconnect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
)

const (
	scopeOpenID        = "openid"
	scopeEmail         = "email"
	revokeURL          = "https://oauth2.googleapis.com/revoke"
	defaultHTTPTimeout = 30 * time.Second
	maxResponseBytes   = 1 << 20
)

// AuthCodeParams is everything needed to build a Google consent URL.
type AuthCodeParams struct {
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	Scopes        []string
	State         string
	Verifier      string
	LoginHint     string
	SelectAccount bool
	ForceConsent  bool
}

// ExchangeParams completes the authorization code flow with PKCE.
type ExchangeParams struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Code         string
	Verifier     string
}

// TokenSet is the provider result. Tokens must not be returned through MCP or UI JSON.
type TokenSet struct {
	RefreshToken string
	Scopes       []string
	Subject      string
	Email        string
}

// Provider is the mockable Google OAuth boundary.
type Provider interface {
	AuthCodeURL(params AuthCodeParams) (string, error)
	Exchange(ctx context.Context, params ExchangeParams) (TokenSet, error)
	Revoke(ctx context.Context, refreshToken string) error
}

// GoogleProvider talks to Google's authorization server.
type GoogleProvider struct {
	Endpoint        oauth2.Endpoint
	HTTP            *http.Client
	ValidateIDToken func(context.Context, string, string) (*idtoken.Payload, error)

	mu        sync.Mutex
	validator *idtoken.Validator
}

func NewGoogleProvider() *GoogleProvider {
	return &GoogleProvider{
		Endpoint: oauth2.Endpoint{ //nolint:gosec // G101: Google OAuth endpoint URLs, not credentials
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
		HTTP: newBoundedClient(nil),
	}
}

func newBoundedClient(base *http.Client) *http.Client {
	timeout := defaultHTTPTimeout
	transport := http.DefaultTransport

	if base != nil {
		if base.Timeout > 0 {
			timeout = base.Timeout
		}

		if base.Transport != nil {
			transport = base.Transport
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: &limitedTransport{base: transport, max: maxResponseBytes},
	}
}

func (p *GoogleProvider) client() *http.Client {
	if p.HTTP != nil {
		if p.HTTP.Timeout == 0 && p.HTTP.Transport != nil {
			return newBoundedClient(p.HTTP)
		}

		if p.HTTP.Timeout == 0 {
			cloned := *p.HTTP

			cloned.Timeout = defaultHTTPTimeout
			if cloned.Transport == nil {
				cloned.Transport = &limitedTransport{base: http.DefaultTransport, max: maxResponseBytes}
			}

			return &cloned
		}

		return p.HTTP
	}

	return newBoundedClient(nil)
}

func (p *GoogleProvider) config(clientID, clientSecret, redirectURL string, scopes []string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     p.Endpoint,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
	}
}

func (p *GoogleProvider) AuthCodeURL(params AuthCodeParams) (string, error) {
	if params.ClientID == "" || params.RedirectURL == "" || params.State == "" || params.Verifier == "" {
		return "", ErrIncompleteAuth
	}

	cfg := p.config(params.ClientID, params.ClientSecret, params.RedirectURL, params.Scopes)
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(params.Verifier),
		// The controller carries this connection's existing scopes on reconnect.
		// Do not merge unrelated project grants (including other OAuth clients).
		oauth2.SetAuthURLParam("include_granted_scopes", "false"),
	}

	prompt := make([]string, 0, 2)
	if params.ForceConsent {
		prompt = append(prompt, "consent")
	}

	if params.SelectAccount {
		prompt = append(prompt, "select_account")
	}

	if len(prompt) > 0 {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", strings.Join(prompt, " ")))
	}

	if params.LoginHint != "" {
		opts = append(opts, oauth2.SetAuthURLParam("login_hint", params.LoginHint))
	}

	return cfg.AuthCodeURL(params.State, opts...), nil
}

func (p *GoogleProvider) Exchange(ctx context.Context, params ExchangeParams) (TokenSet, error) {
	if params.Code == "" || params.Verifier == "" {
		return TokenSet{}, ErrInvalidPKCE
	}

	cfg := p.config(params.ClientID, params.ClientSecret, params.RedirectURL, nil)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, p.client())

	tok, err := cfg.Exchange(ctx, params.Code, oauth2.VerifierOption(params.Verifier))
	if err != nil {
		return TokenSet{}, fmt.Errorf("exchange authorization code: %w", err)
	}

	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return TokenSet{}, ErrUnverifiedIdentity
	}

	payload, err := p.validateIDToken(ctx, raw, params.ClientID)
	if err != nil {
		return TokenSet{}, err
	}

	subject, email, err := identityFromPayload(payload)
	if err != nil {
		return TokenSet{}, err
	}

	return TokenSet{
		RefreshToken: tok.RefreshToken,
		Scopes:       splitScopes(tok.Extra("scope")),
		Subject:      subject,
		Email:        email,
	}, nil
}

func (p *GoogleProvider) validateIDToken(ctx context.Context, raw, audience string) (*idtoken.Payload, error) {
	if p.ValidateIDToken != nil {
		payload, err := p.ValidateIDToken(ctx, raw, audience)
		if err != nil {
			return nil, errors.Join(ErrUnverifiedIdentity, err)
		}

		return payload, nil
	}

	v, err := p.getValidator(ctx)
	if err != nil {
		return nil, err
	}

	payload, err := v.Validate(ctx, raw, audience)
	if err != nil {
		return nil, errors.Join(ErrUnverifiedIdentity, err)
	}

	return payload, nil
}

func (p *GoogleProvider) getValidator(ctx context.Context) (*idtoken.Validator, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.validator != nil {
		return p.validator, nil
	}

	v, err := idtoken.NewValidator(ctx, option.WithHTTPClient(p.client()), option.WithoutAuthentication())
	if err != nil {
		return nil, fmt.Errorf("create ID token validator: %w", err)
	}

	p.validator = v

	return v, nil
}

func (p *GoogleProvider) Revoke(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}

	form := url.Values{"token": {refreshToken}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create revoke request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client().Do(req)
	if err != nil {
		return fmt.Errorf("revoke Google grant: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 256))
		return fmt.Errorf("%w: status %d", ErrRevokeFailed, resp.StatusCode)
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))

	return nil
}

func splitScopes(extra any) []string {
	switch v := extra.(type) {
	case string:
		return strings.Fields(v)
	case []string:
		return append([]string(nil), v...)
	default:
		return nil
	}
}

type limitedTransport struct {
	base http.RoundTripper
	max  int64
}

func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("round trip: %w", err)
	}

	if resp != nil && resp.Body != nil {
		resp.Body = &limitedCloser{r: io.LimitReader(resp.Body, t.max), c: resp.Body}
	}

	return resp, nil
}

type limitedCloser struct {
	r io.Reader
	c io.Closer
}

func (l *limitedCloser) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	if err == nil {
		return n, nil
	}

	if errors.Is(err, io.EOF) {
		return n, io.EOF
	}

	return n, fmt.Errorf("read response: %w", err)
}

func (l *limitedCloser) Close() error {
	if err := l.c.Close(); err != nil {
		return fmt.Errorf("close response: %w", err)
	}

	return nil
}
