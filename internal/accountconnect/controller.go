package accountconnect

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/config"
)

const defaultSessionTTL = 10 * time.Minute

const disconnectLocalRetryMessage = "Google revoke finished, but local cleanup needs a retry."

// Options constructs a Controller. RedirectURL must be the exact registered callback.
type Options struct {
	Registry    Registry
	Tokens      RefreshTokenStore
	OAuth       Provider
	Credentials func(clientName string) (ClientCredentials, error)
	Invalidator Invalidator
	Now         func() time.Time
	NewID       func() (string, error)
	SessionTTL  time.Duration
	RedirectURL string
	ClientName  string
	Lifecycle   *Lifecycle
}

// Controller owns connect, reconnect, callback, and disconnect. The web package
// must not duplicate this logic.
type Controller struct {
	registry    Registry
	tokens      RefreshTokenStore
	oauth       Provider
	credentials func(clientName string) (ClientCredentials, error)
	invalidator Invalidator
	now         func() time.Time
	newID       func() (string, error)
	sessionTTL  time.Duration
	redirectURL string
	clientName  string
	lifecycle   *Lifecycle

	mu       sync.Mutex
	sessions map[string]pendingSession
}

type pendingSession struct {
	ID          string
	PrincipalID string
	ClientName  string
	Label       string
	Scopes      []string
	AccountID   string
	Verifier    string
	RedirectURL string
	ExpiresAt   time.Time
}

func NewController(opts Options) (*Controller, error) {
	if opts.Registry == nil {
		return nil, ErrNilRegistry
	}

	if opts.Tokens == nil {
		return nil, ErrNilTokens
	}

	if opts.OAuth == nil {
		return nil, ErrNilOAuth
	}

	if strings.TrimSpace(opts.RedirectURL) == "" {
		return nil, ErrInvalidRedirect
	}

	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	newID := opts.NewID
	if newID == nil {
		newID = randomID
	}

	creds := opts.Credentials
	if creds == nil {
		creds = defaultCredentials
	}

	ttl := opts.SessionTTL
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}

	inv := opts.Invalidator
	if inv == nil {
		inv = NopInvalidator{}
	}

	clientName, err := config.NormalizeClientNameOrDefault(opts.ClientName)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidClientName, err)
	}

	life := opts.Lifecycle
	if life == nil {
		life = NewLifecycle()
	}

	return &Controller{
		registry:    opts.Registry,
		tokens:      opts.Tokens,
		oauth:       opts.OAuth,
		credentials: creds,
		invalidator: inv,
		now:         now,
		newID:       newID,
		sessionTTL:  ttl,
		redirectURL: strings.TrimSpace(opts.RedirectURL),
		clientName:  clientName,
		lifecycle:   life,
		sessions:    make(map[string]pendingSession),
	}, nil
}

func defaultCredentials(clientName string) (ClientCredentials, error) {
	raw, err := config.ReadClientCredentialsFor(clientName)
	if err != nil {
		return ClientCredentials{}, fmt.Errorf("read OAuth client credentials: %w", err)
	}

	return ClientCredentials{ClientID: raw.ClientID, ClientSecret: raw.ClientSecret}, nil
}

func randomID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}

	return hex.EncodeToString(buf[:]), nil
}

// ListAccounts returns caller-visible connections. Tokens are never included.
func (c *Controller) ListAccounts(ctx context.Context, principalID string) ([]AccountView, error) {
	if strings.TrimSpace(principalID) == "" {
		return nil, ErrInvalidPrincipal
	}

	if err := c.lifecycle.Lock(ctx); err != nil {
		return nil, err
	}

	records, err := c.registry.List(ctx, principalID)
	if err != nil {
		c.lifecycle.Unlock()
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	views := make([]AccountView, 0, len(records))
	for _, rec := range records {
		if rec.Cleanup != nil && rec.Cleanup.Kind == CleanupTokenKey {
			rec, _ = c.settleTokenCleanup(ctx, rec)
		}

		views = append(views, accountView(rec))
	}

	c.lifecycle.Unlock()

	sort.Slice(views, func(i, j int) bool {
		if views[i].Email == views[j].Email {
			return views[i].AccountID < views[j].AccountID
		}

		return views[i].Email < views[j].Email
	})

	return views, nil
}

// GetRecord returns a registry snapshot for MCP identity resolution.
func (c *Controller) GetRecord(ctx context.Context, accountID string) (Record, error) {
	rec, ok, err := c.registry.Get(ctx, accountID)
	if err != nil {
		return Record{}, fmt.Errorf("get account: %w", err)
	}

	if !ok {
		return Record{}, ErrUnknownAccount
	}

	if !rec.IsActive() {
		return Record{}, ErrAccountUnavailable
	}

	return rec, nil
}

// StartConnect begins a new Google account chooser flow. Connecting another
// account does not replace existing records.
func (c *Controller) StartConnect(ctx context.Context, req ConnectRequest) (StartResult, error) {
	principalID := strings.TrimSpace(req.PrincipalID)
	if principalID == "" {
		return StartResult{}, ErrInvalidPrincipal
	}

	scopes := filterRequestedScopes(req.Scopes)

	return c.start(ctx, principalID, c.clientName, strings.TrimSpace(req.Label), scopes, "", true, false)
}

// StartReconnect re-consents the same Google subject. The callback rejects a
// different subject rather than silently rebinding the record.
func (c *Controller) StartReconnect(ctx context.Context, req ReconnectRequest) (StartResult, error) {
	principalID := strings.TrimSpace(req.PrincipalID)
	if principalID == "" {
		return StartResult{}, ErrInvalidPrincipal
	}

	rec, err := c.ownedRecord(ctx, principalID, req.AccountID)
	if err != nil {
		return StartResult{}, err
	}

	if rec.State == RecordStateDisconnecting {
		return StartResult{}, ErrRevokeInProgress
	}

	scopes := reconnectScopes(rec.Scopes, req.Scopes)

	return c.start(ctx, principalID, rec.ClientName, rec.Label, scopes, rec.AccountID, false, true)
}

func (c *Controller) start(ctx context.Context, principalID, clientName, label string, scopes []string, accountID string, selectAccount, forceConsent bool) (StartResult, error) {
	if len(scopes) == 0 {
		return StartResult{}, ErrMissingScopes
	}

	creds, err := c.credentials(clientName)
	if err != nil {
		return StartResult{}, err
	}

	state, err := c.newID()
	if err != nil {
		return StartResult{}, err
	}

	verifier := oauth2.GenerateVerifier()
	loginHint := ""

	if accountID != "" {
		rec, ok, getErr := c.registry.Get(ctx, accountID)
		if getErr != nil {
			return StartResult{}, fmt.Errorf("load reconnect account: %w", getErr)
		}

		if ok {
			loginHint = rec.Email
		}
	}

	authURL, err := c.oauth.AuthCodeURL(AuthCodeParams{
		ClientID:      creds.ClientID,
		ClientSecret:  creds.ClientSecret,
		RedirectURL:   c.redirectURL,
		Scopes:        append([]string(nil), scopes...),
		State:         state,
		Verifier:      verifier,
		LoginHint:     loginHint,
		SelectAccount: selectAccount,
		ForceConsent:  forceConsent,
	})
	if err != nil {
		return StartResult{}, fmt.Errorf("auth URL: %w", err)
	}

	c.mu.Lock()
	c.sessions[state] = pendingSession{
		ID:          state,
		PrincipalID: principalID,
		ClientName:  clientName,
		Label:       label,
		Scopes:      append([]string(nil), scopes...),
		AccountID:   accountID,
		Verifier:    verifier,
		RedirectURL: c.redirectURL,
		ExpiresAt:   c.now().Add(c.sessionTTL),
	}
	c.mu.Unlock()

	return StartResult{AuthURL: authURL, SessionID: state}, nil
}

// CompleteCallback validates state, PKCE, and the exact redirect URL, then
// binds the connection to the verified Google subject.
func (c *Controller) CompleteCallback(ctx context.Context, req CallbackRequest) (CallbackResult, error) {
	if req.Error != "" {
		return CallbackResult{}, fmt.Errorf("%w: %s", ErrProviderDenied, req.Error)
	}

	if strings.TrimSpace(req.Code) == "" {
		return CallbackResult{}, ErrMissingCode
	}

	if strings.TrimSpace(req.RedirectURL) != c.redirectURL {
		return CallbackResult{}, ErrInvalidRedirect
	}

	if subtle.ConstantTimeCompare([]byte(req.BrowserID), []byte(req.State)) != 1 {
		return CallbackResult{}, ErrInvalidState
	}

	sess, err := c.takeSession(req.State)
	if err != nil {
		return CallbackResult{}, err
	}

	creds, err := c.credentials(sess.ClientName)
	if err != nil {
		return CallbackResult{}, err
	}

	beforeEpochs, err := c.registry.Epochs()
	if err != nil {
		return CallbackResult{}, fmt.Errorf("read revoke epochs: %w", err)
	}

	tok, err := c.oauth.Exchange(ctx, ExchangeParams{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		RedirectURL:  sess.RedirectURL,
		Code:         req.Code,
		Verifier:     sess.Verifier,
	})
	if err != nil {
		return CallbackResult{}, fmt.Errorf("exchange code: %w", err)
	}

	if tok.Subject == "" || tok.Email == "" {
		return CallbackResult{}, ErrUnverifiedIdentity
	}

	if len(tok.Scopes) == 0 {
		return CallbackResult{}, ErrMissingGrantedScopes
	}

	if lockErr := c.lifecycle.Lock(ctx); lockErr != nil {
		return CallbackResult{}, lockErr
	}

	currentEpoch, epochErr := c.registry.RevokeEpoch(sess.ClientName, tok.Subject)
	if epochErr != nil {
		c.lifecycle.Unlock()
		return CallbackResult{}, fmt.Errorf("read revoke epoch: %w", epochErr)
	}

	if beforeEpochs[epochKey(sess.ClientName, tok.Subject)] != currentEpoch {
		c.lifecycle.Unlock()
		return CallbackResult{}, ErrRevokeInProgress
	}

	existing, created, err := c.matchRecord(ctx, sess, tok.Subject)
	if err != nil {
		c.lifecycle.Unlock()
		return CallbackResult{}, err
	}

	if !created && existing.State == RecordStateDisconnecting {
		c.lifecycle.Unlock()
		return CallbackResult{}, ErrRevokeInProgress
	}

	if !created && existing.Subject != tok.Subject {
		c.lifecycle.Unlock()
		return CallbackResult{}, ErrSubjectMismatch
	}

	previousEmail := ""
	refresh := tok.RefreshToken

	if !created {
		previousEmail = existing.Email
	}

	if refresh == "" {
		lookupEmail := tok.Email
		if previousEmail != "" {
			lookupEmail = previousEmail
		}

		stored, _, getErr := c.tokens.Get(ctx, sess.ClientName, lookupEmail)
		if getErr != nil || stored == "" {
			c.lifecycle.Unlock()
			return CallbackResult{}, ErrMissingRefresh
		}

		refresh = stored
	}

	now := c.now()
	rec := existing

	if created {
		id, idErr := c.newID()
		if idErr != nil {
			c.lifecycle.Unlock()
			return CallbackResult{}, idErr
		}

		rec = Record{
			AccountID:   id,
			Subject:     tok.Subject,
			Email:       tok.Email,
			Label:       sess.Label,
			PrincipalID: sess.PrincipalID,
			ClientName:  sess.ClientName,
			AuthMode:    AuthModeOAuth,
			Generation:  0,
		}
	}

	if rec.Label == "" {
		rec.Label = sess.Label
	}

	if rec.Label == "" {
		rec.Label = tok.Email
	}

	rec.Email = tok.Email
	rec.Subject = tok.Subject
	rec.Scopes = append([]string(nil), tok.Scopes...)
	rec.UpdatedAt = now
	rec.AuthMode = AuthModeOAuth
	rec.State = RecordStatePending

	rec.Generation = 0
	if rec.Cleanup != nil && rec.Cleanup.Kind != CleanupTokenKey {
		rec.Cleanup = nil
	}

	committed, cleanupPending, err := c.persistConnection(ctx, rec, refresh, previousEmail)
	c.lifecycle.Unlock()
	c.notifyConnectionsChanged()

	if err != nil {
		return CallbackResult{}, err
	}

	result := CallbackResult{Account: accountView(committed), Created: created, CleanupPending: cleanupPending}
	if cleanupPending {
		result.Message = "Connected. Previous credential cleanup is pending."
	}

	return result, nil
}

func (c *Controller) persistConnection(ctx context.Context, rec Record, refresh, previousEmail string) (Record, bool, error) {
	if rec.AccountID != "" {
		live, ok, err := c.registry.Get(ctx, rec.AccountID)
		if err != nil {
			return Record{}, false, fmt.Errorf("load account: %w", err)
		}

		if ok && live.Cleanup != nil && live.Cleanup.Kind == CleanupTokenKey {
			settled, pending := c.settleTokenCleanup(ctx, live)
			if pending {
				return settled, true, ErrCleanupPending
			}

			rec.Cleanup = settled.Cleanup
		}

		c.invalidator.InvalidateAccount(rec.AccountID)
	}

	rec.State = RecordStatePending
	if previousEmail != "" && previousEmail != rec.Email {
		rec.Cleanup = &Cleanup{Kind: CleanupTokenKey, ClientName: rec.ClientName, Email: previousEmail}
	}

	committed, _, existed, err := c.registry.Commit(ctx, rec)
	if err != nil {
		return Record{}, false, fmt.Errorf("persist account: %w", err)
	}

	if existed && committed.AccountID != rec.AccountID {
		c.invalidator.InvalidateAccount(committed.AccountID)
	}

	if putErr := c.tokens.Put(ctx, committed.ClientName, committed.Email, refresh, committed.Scopes); putErr != nil {
		c.invalidator.InvalidateAccount(committed.AccountID)

		// Protected stores can mutate before returning an error. Keep the
		// committed pending row and cleanup intent until reconnect recovers it.
		return committed, true, fmt.Errorf("store refresh token: %w", putErr)
	}

	committed.State = RecordStateActive
	if previousEmail == "" || previousEmail == committed.Email {
		if committed.Cleanup != nil && committed.Cleanup.Kind != CleanupTokenKey {
			committed.Cleanup = nil
		}
	}

	active, _, _, err := c.registry.Commit(ctx, committed)
	if err != nil {
		return Record{}, false, fmt.Errorf("activate account: %w", err)
	}

	active, cleanupPending := c.settleTokenCleanup(ctx, active)
	c.invalidator.InvalidateAccount(active.AccountID)

	return active, cleanupPending, nil
}

func (c *Controller) settleTokenCleanup(ctx context.Context, rec Record) (Record, bool) {
	if rec.Cleanup == nil || rec.Cleanup.Kind != CleanupTokenKey || rec.Cleanup.Email == "" {
		return rec, rec.Cleanup != nil
	}

	cleanup := rec.Cleanup
	if delErr := c.tokens.Delete(ctx, cleanup.ClientName, cleanup.Email); delErr != nil && !IsTokenNotFound(delErr) {
		c.persistCleanupDebt(ctx, rec)
		return rec, true
	}

	rec.Cleanup = nil
	if _, _, _, err := c.registry.Commit(ctx, rec); err != nil {
		rec.Cleanup = cleanup
		return rec, true
	}

	return rec, false
}

func (c *Controller) persistCleanupDebt(ctx context.Context, rec Record) {
	if _, _, _, err := c.registry.Commit(ctx, rec); err != nil {
		return
	}
}

func (c *Controller) matchRecord(ctx context.Context, sess pendingSession, subject string) (Record, bool, error) {
	if sess.AccountID != "" {
		rec, err := c.ownedRecord(ctx, sess.PrincipalID, sess.AccountID)
		if err != nil {
			return Record{}, false, err
		}

		if rec.Subject != subject {
			return Record{}, false, ErrSubjectMismatch
		}

		return rec, false, nil
	}

	records, err := c.registry.List(ctx, sess.PrincipalID)
	if err != nil {
		return Record{}, false, fmt.Errorf("list accounts: %w", err)
	}

	for _, rec := range records {
		if rec.ClientName == sess.ClientName && rec.Subject == subject {
			return rec, false, nil
		}
	}

	return Record{}, true, nil
}

// Disconnect deletes one local connection and attempts to revoke the Google grant.
func (c *Controller) Disconnect(ctx context.Context, req DisconnectRequest) (DisconnectResult, error) {
	if err := c.lifecycle.Lock(ctx); err != nil {
		return DisconnectResult{}, err
	}

	rec, err := c.ownedRecord(ctx, req.PrincipalID, req.AccountID)
	if err != nil {
		c.lifecycle.Unlock()
		return DisconnectResult{}, err
	}

	refresh, _, tokenErr := c.tokens.Get(ctx, rec.ClientName, rec.Email)
	alreadyLocal := rec.State == RecordStateDisconnecting && rec.Cleanup != nil && rec.Cleanup.Stage == CleanupStageLocal
	tokenMissing := refresh == "" || IsTokenNotFound(tokenErr)
	tokenReadFailed := tokenErr != nil && !IsTokenNotFound(tokenErr)

	if rec.State != RecordStateDisconnecting {
		if rec.Cleanup != nil && rec.Cleanup.Kind == CleanupTokenKey {
			var pending bool

			rec, pending = c.settleTokenCleanup(ctx, rec)
			if pending {
				c.lifecycle.Unlock()

				return DisconnectResult{
					AccountID: rec.AccountID,
					Retryable: true,
					State:     rec.State,
					Message:   "Previous credential cleanup is still pending. Retry disconnect after it completes.",
				}, ErrCleanupPending
			}
		}

		epoch, bumpErr := c.registry.BumpRevokeEpoch(rec.ClientName, rec.Subject)
		if bumpErr != nil {
			c.lifecycle.Unlock()
			return DisconnectResult{}, fmt.Errorf("bump revoke epoch: %w", bumpErr)
		}

		rec.State = RecordStateDisconnecting
		rec.RevokeEpoch = epoch
		rec.Cleanup = &Cleanup{Kind: CleanupRevoke, ClientName: rec.ClientName, Email: rec.Email}
		rec.UpdatedAt = c.now()

		rec.Generation = 0
		if _, _, _, err := c.registry.Commit(ctx, rec); err != nil {
			c.lifecycle.Unlock()
			return DisconnectResult{}, fmt.Errorf("mark disconnecting: %w", err)
		}
	}

	c.invalidator.InvalidateAccount(rec.AccountID)
	c.lifecycle.Unlock()
	c.notifyConnectionsChanged()

	if tokenReadFailed && !alreadyLocal {
		return DisconnectResult{
			AccountID:                     rec.AccountID,
			OtherDeploymentsMayBeAffected: true,
			Retryable:                     true,
			State:                         RecordStateDisconnecting,
			Message:                       "Google revoke did not finish. The local connection is disabled; retry disconnect to keep the protected retry token.",
		}, fmt.Errorf("read refresh token: %w", tokenErr)
	}

	revoked := alreadyLocal
	revokeErr := error(nil)

	needRevoke := !alreadyLocal && !tokenMissing
	if needRevoke {
		revokeErr = c.oauth.Revoke(ctx, refresh)
		revoked = revokeErr == nil
	}

	if needRevoke && !revoked {
		return DisconnectResult{
			AccountID:                     rec.AccountID,
			OtherDeploymentsMayBeAffected: true,
			Retryable:                     true,
			State:                         RecordStateDisconnecting,
			Message:                       "Google revoke did not finish. The local connection is disabled; retry disconnect to keep the protected retry token.",
		}, fmt.Errorf("revoke Google grant: %w", revokeErr)
	}

	if err := c.lifecycle.Lock(ctx); err != nil {
		return DisconnectResult{
			AccountID:                     rec.AccountID,
			RevokedRemote:                 revoked,
			OtherDeploymentsMayBeAffected: true,
			Retryable:                     true,
			State:                         RecordStateDisconnecting,
			Message:                       disconnectLocalRetryMessage,
		}, err
	}

	current, ok, getErr := c.registry.Get(ctx, rec.AccountID)
	if getErr != nil {
		c.lifecycle.Unlock()

		return DisconnectResult{
			AccountID:                     rec.AccountID,
			RevokedRemote:                 revoked,
			OtherDeploymentsMayBeAffected: true,
			Retryable:                     true,
			State:                         RecordStateDisconnecting,
			Message:                       disconnectLocalRetryMessage,
		}, fmt.Errorf("reload disconnecting account: %w", getErr)
	}

	if !ok || current.State != RecordStateDisconnecting {
		c.lifecycle.Unlock()
		c.notifyConnectionsChanged()

		return DisconnectResult{AccountID: rec.AccountID, RevokedRemote: revoked, OtherDeploymentsMayBeAffected: true, Message: "Local connection removed. Other connected Google accounts were not changed."}, nil
	}

	current.Cleanup = &Cleanup{Kind: CleanupRevoke, Stage: CleanupStageLocal, ClientName: current.ClientName, Email: current.Email}
	if _, _, _, err := c.registry.Commit(ctx, current); err != nil {
		c.lifecycle.Unlock()

		return DisconnectResult{
			AccountID: rec.AccountID, RevokedRemote: revoked, OtherDeploymentsMayBeAffected: true,
			Retryable: true, State: RecordStateDisconnecting,
			Message: disconnectLocalRetryMessage,
		}, fmt.Errorf("record revoke stage: %w", err)
	}

	if _, err := c.registry.BumpRevokeEpoch(current.ClientName, current.Subject); err != nil {
		c.lifecycle.Unlock()

		return DisconnectResult{
			AccountID: rec.AccountID, RevokedRemote: revoked, OtherDeploymentsMayBeAffected: true,
			Retryable: true, State: RecordStateDisconnecting,
			Message: disconnectLocalRetryMessage,
		}, fmt.Errorf("bump completion epoch: %w", err)
	}

	if err := c.tokens.Delete(ctx, current.ClientName, current.Email); err != nil && !IsTokenNotFound(err) {
		c.lifecycle.Unlock()

		return DisconnectResult{
			AccountID: rec.AccountID, RevokedRemote: revoked, OtherDeploymentsMayBeAffected: true,
			Retryable: true, State: RecordStateDisconnecting,
			Message: disconnectLocalRetryMessage,
		}, fmt.Errorf("delete refresh token: %w", err)
	}

	if err := c.registry.Delete(ctx, current.AccountID); err != nil {
		c.lifecycle.Unlock()

		return DisconnectResult{
			AccountID: rec.AccountID, RevokedRemote: revoked, OtherDeploymentsMayBeAffected: true,
			Retryable: true, State: RecordStateDisconnecting,
			Message: disconnectLocalRetryMessage,
		}, fmt.Errorf("delete account: %w", err)
	}

	c.lifecycle.Unlock()
	c.notifyConnectionsChanged()

	msg := "Local connection removed. Other connected Google accounts were not changed."
	if revoked {
		msg += " The Google grant for this app was revoked; other deployments using the same Google OAuth client may need to reconnect this mailbox."
	} else {
		msg += " Google-side revocation did not complete; the mailbox may still be authorized in other deployments of this app."
	}

	return DisconnectResult{
		AccountID:                     rec.AccountID,
		RevokedRemote:                 revoked,
		OtherDeploymentsMayBeAffected: true,
		Message:                       msg,
	}, nil
}

func (c *Controller) notifyConnectionsChanged() {
	c.invalidator.ConnectionsChanged()
}

func (c *Controller) ownedRecord(ctx context.Context, principalID, accountID string) (Record, error) {
	if strings.TrimSpace(principalID) == "" {
		return Record{}, ErrInvalidPrincipal
	}

	if strings.TrimSpace(accountID) == "" {
		return Record{}, ErrUnknownAccount
	}

	rec, ok, err := c.registry.Get(ctx, accountID)
	if err != nil {
		return Record{}, fmt.Errorf("get account: %w", err)
	}

	if !ok {
		return Record{}, ErrUnknownAccount
	}

	if rec.PrincipalID != principalID {
		return Record{}, ErrForbiddenAccount
	}

	return rec, nil
}

func (c *Controller) takeSession(state string) (pendingSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	sess, ok := c.sessions[state]
	if !ok {
		return pendingSession{}, ErrInvalidState
	}

	delete(c.sessions, state)

	if !c.now().Before(sess.ExpiresAt) {
		return pendingSession{}, ErrSessionExpired
	}

	if sess.Verifier == "" {
		return pendingSession{}, ErrInvalidPKCE
	}

	return sess, nil
}

func (c *Controller) RedirectURL() string {
	return c.redirectURL
}

// ClientName is the server-selected OAuth credential bucket. The UI must not pick this.
func (c *Controller) ClientName() string {
	return c.clientName
}

func (c *Controller) SessionTTL() time.Duration {
	return c.sessionTTL
}

func (c *Controller) Lifecycle() *Lifecycle {
	return c.lifecycle
}

func filterRequestedScopes(requested []string) []string {
	if requested == nil {
		return append([]string(nil), DefaultConnectScopes()...)
	}

	return unionScopes(identityScopes(), requested)
}

func reconnectScopes(existing, requested []string) []string {
	if requested == nil {
		return unionScopes(identityScopes(), existing)
	}

	return unionScopes(identityScopes(), existing, requested)
}

func identityScopes() []string {
	return []string{scopeOpenID, scopeEmail}
}

func unionScopes(sets ...[]string) []string {
	allowed := make(map[string]bool, len(AllowedConnectScopes()))
	for _, scope := range AllowedConnectScopes() {
		allowed[scope] = true
	}

	out := make([]string, 0)
	seen := map[string]bool{}

	for _, set := range sets {
		for _, scope := range set {
			scope = strings.TrimSpace(scope)
			if scope == "" || seen[scope] {
				continue
			}

			if !allowed[scope] && scope != scopeOpenID && scope != scopeEmail {
				continue
			}

			seen[scope] = true
			out = append(out, scope)
		}
	}

	return out
}
