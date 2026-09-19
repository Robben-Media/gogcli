package accountconnect

import (
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

// AuthModeOAuth is the only supported connection mode for the Gmail/Workspace pilot.
const AuthModeOAuth = "oauth"

// HTTP paths the UI worker must use. Tokens never appear in these responses.
const (
	PathAccounts   = "/accounts"
	PathConnect    = "/connect"
	PathReconnect  = "/reconnect"
	PathCallback   = "/oauth/callback"
	PathDisconnect = "/disconnect"
	PathStatus     = "/status"
)

// Record is the durable connection registry entry. Refresh tokens are stored
// separately in the protected token store, never on this record or in UI JSON.
type RecordState string

const (
	RecordStateActive        RecordState = "active"
	RecordStatePending       RecordState = "pending"
	RecordStateDisconnecting RecordState = "disconnecting"
	CleanupTokenKey                      = "token_key"
	CleanupRevoke                        = "revoke"
	CleanupStageLocal                    = "local"
)

// Cleanup is durable retry metadata for an interrupted connect or disconnect.
type Cleanup struct {
	Kind       string `json:"kind"`
	Stage      string `json:"stage,omitempty"`
	ClientName string `json:"client_name,omitempty"`
	Email      string `json:"email,omitempty"`
}

type Record struct {
	AccountID   string
	Subject     string
	Email       string
	Label       string
	PrincipalID string
	ClientName  string
	AuthMode    string
	Scopes      []string
	Generation  uint64
	State       RecordState
	RevokeEpoch uint64
	Cleanup     *Cleanup
	UpdatedAt   time.Time
}

func (r Record) IsActive() bool {
	return r.State == "" || r.State == RecordStateActive
}

// Identity returns the frozen MCP identity snapshot for this record.
func (r Record) Identity() mcpcontract.Identity {
	scopes := append([]string(nil), r.Scopes...)

	return mcpcontract.Identity{
		AccountID:   r.AccountID,
		Subject:     r.Subject,
		Email:       r.Email,
		Label:       r.Label,
		PrincipalID: r.PrincipalID,
		ClientName:  r.ClientName,
		AuthMode:    r.AuthMode,
		Scopes:      scopes,
		Generation:  r.Generation,
		UpdatedAt:   r.UpdatedAt,
	}
}

// AccountView is the UI/MCP-safe connection listing. No tokens, subjects may
// be omitted from page copy; AccountID is the only selector.
type AccountView struct {
	AccountID      string      `json:"account_id"`
	Email          string      `json:"email"`
	Label          string      `json:"label"`
	ClientName     string      `json:"client_name"`
	AuthMode       string      `json:"auth_mode"`
	Scopes         []string    `json:"scopes"`
	Capabilities   []string    `json:"capabilities"`
	State          RecordState `json:"state,omitempty"`
	CleanupPending bool        `json:"cleanup_pending,omitempty"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// ConnectRequest starts a new Google account connection for a trusted principal.
// The controller uses the server-configured OAuth app, not a caller-selected client.
type ConnectRequest struct {
	PrincipalID string   `json:"principal_id"`
	Label       string   `json:"label"`
	Scopes      []string `json:"scopes"`
}

// ScopeChoice is a server-provided readonly capability the UI may offer.
type ScopeChoice struct {
	Capability string `json:"capability"`
	Scope      string `json:"scope"`
	Required   bool   `json:"required"`
}

// ReconnectRequest starts consent again for an existing AccountID.
type ReconnectRequest struct {
	PrincipalID string   `json:"principal_id"`
	AccountID   string   `json:"account_id"`
	Scopes      []string `json:"scopes,omitempty"`
}

// StartResult is returned to the UI so it can redirect the browser to Google.
// SessionID is the OAuth state value. The handler copies it into the HttpOnly
// browser cookie so the callback can bind the redirect to the initiating
// browser. The PKCE verifier and pending session record stay server-side and
// are not in this payload.
type StartResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
}

// CallbackRequest is the OAuth redirect query. RedirectURL must equal the
// registered URL used when the session was created. BrowserID is the initiating
// browser session cookie and must equal State.
type CallbackRequest struct {
	Code        string
	State       string
	Error       string
	RedirectURL string
	BrowserID   string
}

// CallbackResult is shown after a successful connect/reconnect.
type CallbackResult struct {
	Account        AccountView `json:"account"`
	Created        bool        `json:"created"`
	CleanupPending bool        `json:"cleanup_pending,omitempty"`
	Message        string      `json:"message,omitempty"`
}

// DisconnectRequest removes one connection owned by the principal.
type DisconnectRequest struct {
	PrincipalID string `json:"principal_id"`
	AccountID   string `json:"account_id"`
}

// DisconnectResult describes local deletion and whether Google-side revocation
// can affect other deployments that share the same app grant.
type DisconnectResult struct {
	AccountID                     string      `json:"account_id"`
	RevokedRemote                 bool        `json:"revoked_remote"`
	OtherDeploymentsMayBeAffected bool        `json:"other_deployments_may_be_affected"`
	Retryable                     bool        `json:"retryable,omitempty"`
	State                         RecordState `json:"state,omitempty"`
	Message                       string      `json:"message"`
}

// StatusResponse is the JSON body for GET /status used by the UI copy.
const (
	ActionConnected    = "connected"
	ActionDisconnected = "disconnected"
	ActionError        = "error"
)

type StatusResponse struct {
	OK           bool         `json:"ok"`
	Action       string       `json:"action,omitempty"`
	Disconnected bool         `json:"disconnected,omitempty"`
	Account      *AccountView `json:"account,omitempty"`
	Error        string       `json:"error,omitempty"`
	Detail       string       `json:"detail,omitempty"`
}

// ClientCredentials are app-owned OAuth client values. Never send these to the browser.
type ClientCredentials struct {
	ClientID     string
	ClientSecret string
}
