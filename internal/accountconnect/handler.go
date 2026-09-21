package accountconnect

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const (
	// CSRFCookieName is the HttpOnly cookie holding the CSRF secret.
	CSRFCookieName = "gog_csrf"
	// CSRFHeaderName is required on mutating JSON requests.
	CSRFHeaderName = "X-CSRF-Token"
	// CSRFFieldName is accepted on HTML form posts as a fallback.
	CSRFFieldName = "csrf_token"
	// OAuthSessionCookieName binds the OAuth state to the initiating browser.
	OAuthSessionCookieName = "gog_oauth_session"
	maxRequestBody         = 64 << 10
)

// PageModel is the only data the web package may render. It never contains tokens.
type PageModel struct {
	PrincipalID           string          `json:"principal_id"`
	CSRFToken             string          `json:"csrf_token"`
	ClientName            string          `json:"client_name,omitempty"`
	RequestedCapabilities []string        `json:"requested_capabilities"`
	ScopeChoices          []ScopeChoice   `json:"scope_choices"`
	Accounts              []AccountView   `json:"accounts"`
	Status                *StatusResponse `json:"status,omitempty"`
}

// Renderer is implemented by internal/accountconnect/web. Nil means JSON only.
type Renderer interface {
	RenderAccounts(w http.ResponseWriter, r *http.Request, page PageModel) error
	RenderStatus(w http.ResponseWriter, r *http.Request, page PageModel) error
}

// HandlerOptions binds the HTTP surface to a trusted principal. Principal is
// process startup configuration, never a query parameter or MCP client name.
type HandlerOptions struct {
	Principal mcpcontract.Principal
	Renderer  Renderer
}

// Handler serves the account-management JSON/HTTP API for the UI worker.
type Handler struct {
	controller *Controller
	principal  mcpcontract.Principal
	renderer   Renderer
	mux        *http.ServeMux
}

func NewHandler(controller *Controller, opts HandlerOptions) (*Handler, error) {
	if controller == nil {
		return nil, ErrNilController
	}

	if strings.TrimSpace(opts.Principal.ID) == "" {
		return nil, ErrInvalidPrincipal
	}

	h := &Handler{controller: controller, principal: opts.Principal, renderer: opts.Renderer}
	mux := http.NewServeMux()
	mux.HandleFunc(PathAccounts, h.accounts)
	mux.HandleFunc(PathConnect, h.connect)
	mux.HandleFunc(PathReconnect, h.reconnect)
	mux.HandleFunc(PathCallback, h.callback)
	mux.HandleFunc(PathDisconnect, h.disconnect)
	mux.HandleFunc(PathStatus, h.status)
	h.mux = mux

	return h, nil
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// Mux mounts the stable UI paths. The web package may wrap this mux with HTML
// assets but must not add a second OAuth callback handler.
func (h *Handler) Mux() *http.ServeMux {
	return h.mux
}

func (h *Handler) accounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.guard(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if h.redirectLoopbackAlias(w, r) {
		return
	}

	token, err := h.ensureCSRF(w, r)
	if err != nil {
		http.Error(w, "csrf", http.StatusInternalServerError)
		return
	}

	views, err := h.controller.ListAccounts(r.Context(), h.principal.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err)
		return
	}

	page := h.page(token, views, nil)
	if h.wantHTML(r) && h.renderer != nil {
		if err := h.renderer.RenderAccounts(w, r, page); err != nil {
			http.Error(w, "render", http.StatusInternalServerError)
		}

		return
	}

	h.writeJSON(w, http.StatusOK, page)
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	if !h.beginMutation(w, r) {
		return
	}

	req, err := decodeConnectRequest(r, h.controller.scopeForCapability)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err)
		return
	}

	req.PrincipalID = h.principal.ID

	result, err := h.controller.StartConnect(r.Context(), req)
	if err != nil {
		h.writeError(w, statusFor(err), err)
		return
	}

	h.writeStartResult(w, r, result)
}

func (h *Handler) reconnect(w http.ResponseWriter, r *http.Request) {
	if !h.beginMutation(w, r) {
		return
	}

	req, err := decodeReconnectRequest(r, h.controller.scopeForCapability)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err)
		return
	}

	req.PrincipalID = h.principal.ID

	result, err := h.controller.StartReconnect(r.Context(), req)
	if err != nil {
		h.writeError(w, statusFor(err), err)
		return
	}

	h.writeStartResult(w, r, result)
}

func (h *Handler) beginMutation(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}

	h.limitBody(w, r)

	if err := h.guard(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}

	if err := h.requireCSRF(r); err != nil {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return false
	}

	return true
}

func (h *Handler) writeStartResult(w http.ResponseWriter, r *http.Request, result StartResult) {
	h.setOAuthSessionCookie(w, result.SessionID)

	if h.wantJSON(r) {
		h.writeJSON(w, http.StatusOK, result)
		return
	}

	http.Redirect(w, r, result.AuthURL, http.StatusFound)
}

func (h *Handler) disconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.limitBody(w, r)

	if err := h.guard(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := h.requireCSRF(r); err != nil {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	req, err := decodeDisconnectRequest(r)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err)
		return
	}

	req.PrincipalID = h.principal.ID

	result, err := h.controller.Disconnect(r.Context(), req)
	if err != nil {
		h.writeError(w, statusFor(err), err)
		return
	}

	if h.wantJSON(r) {
		h.writeJSON(w, http.StatusOK, result)
		return
	}

	http.Redirect(w, r, PathStatus+"?"+url.Values{"ok": {"1"}, "action": {ActionDisconnected}}.Encode(), http.StatusSeeOther)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.guard(r); err != nil {
		http.Redirect(w, r, PathStatus+"?"+url.Values{"ok": {"0"}, "error": {"signin_rejected"}}.Encode(), http.StatusSeeOther)
		return
	}

	browserID := ""
	if cookie, err := r.Cookie(OAuthSessionCookieName); err == nil {
		browserID = cookie.Value
	}

	q := r.URL.Query()

	result, err := h.controller.CompleteCallback(r.Context(), CallbackRequest{
		Code:        q.Get("code"),
		State:       q.Get("state"),
		Error:       q.Get("error"),
		RedirectURL: h.controller.RedirectURL(),
		BrowserID:   browserID,
	})
	if err != nil {
		http.Redirect(w, r, PathStatus+"?"+url.Values{"ok": {"0"}, "error": {publicError(err)}}.Encode(), http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: clearing a loopback session cookie
		Name: OAuthSessionCookieName, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, PathStatus+"?"+url.Values{"ok": {"1"}, "account_id": {result.Account.AccountID}}.Encode(), http.StatusSeeOther)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.guard(r); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if h.redirectLoopbackAlias(w, r) {
		return
	}

	token, err := h.ensureCSRF(w, r)
	if err != nil {
		http.Error(w, "csrf", http.StatusInternalServerError)
		return
	}

	status := StatusResponse{OK: r.URL.Query().Get("ok") != "0", Action: r.URL.Query().Get("action")}
	if errText := r.URL.Query().Get("error"); errText != "" {
		status.OK = false
		status.Action = ActionError
		status.Error = errText
		status.Detail = statusCopy(errText)
	}

	if status.Action == ActionDisconnected {
		status.OK = true
		status.Disconnected = true
		status.Detail = "Disconnected. Other Google accounts are unchanged."
	}

	if accountID := r.URL.Query().Get("account_id"); accountID != "" && status.OK && !status.Disconnected {
		rec, getErr := h.controller.GetRecord(r.Context(), accountID)
		if getErr == nil && rec.PrincipalID == h.principal.ID {
			view := h.controller.accountView(rec)
			status.Account = &view
			status.Action = ActionConnected
			status.Detail = "Connected " + view.Email + "."
		}
	}

	page := h.page(token, nil, &status)
	if h.wantHTML(r) && h.renderer != nil {
		if err := h.renderer.RenderStatus(w, r, page); err != nil {
			http.Error(w, "render", http.StatusInternalServerError)
		}

		return
	}

	h.writeJSON(w, http.StatusOK, status)
}

func (h *Handler) page(token string, accounts []AccountView, status *StatusResponse) PageModel {
	return PageModel{
		PrincipalID:           h.principal.ID,
		CSRFToken:             token,
		ClientName:            h.controller.ClientName(),
		RequestedCapabilities: capabilitiesForScopes(DefaultConnectScopes()),
		ScopeChoices:          h.controller.ScopeChoices(),
		Accounts:              accounts,
		Status:                status,
	}
}

// redirectLoopbackAlias keeps browser cookies on the registered callback host.
func (h *Handler) redirectLoopbackAlias(w http.ResponseWriter, r *http.Request) bool {
	callback, err := url.Parse(h.controller.RedirectURL())
	if err != nil || strings.EqualFold(r.Host, callback.Host) || !loopbackHostEquivalent(r.Host, callback.Host) {
		return false
	}
	callback.Path = r.URL.Path
	callback.RawQuery = r.URL.RawQuery
	callback.Fragment = ""
	http.Redirect(w, r, callback.String(), http.StatusSeeOther)

	return true
}

func (h *Handler) guard(r *http.Request) error {
	return checkRequestHost(r, h.controller.RedirectURL())
}

func (h *Handler) limitBody(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	}
}

func (h *Handler) setOAuthSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: local HTTP loopback cannot set Secure
		Name:     OAuthSessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.controller.SessionTTL().Seconds()),
	})
}

func (h *Handler) ensureCSRF(w http.ResponseWriter, r *http.Request) (string, error) {
	if cookie, err := r.Cookie(CSRFCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, nil
	}

	token, err := randomID()
	if err != nil {
		return "", err
	}

	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: local HTTP loopback cannot set Secure
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return token, nil
}

func (h *Handler) requireCSRF(r *http.Request) error {
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil || cookie.Value == "" {
		return ErrInvalidState
	}

	provided := r.Header.Get(CSRFHeaderName)
	if provided == "" {
		provided = r.FormValue(CSRFFieldName)
	}

	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(provided)) != 1 {
		return ErrInvalidState
	}

	return nil
}

func (h *Handler) wantJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

func (h *Handler) wantHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html") && !h.wantJSON(r)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, err error) {
	h.writeJSON(w, status, StatusResponse{OK: false, Error: publicError(err), Detail: statusCopy(publicError(err))})
}

var errEmptyBody = errors.New("empty body")

func decodeJSONBody(r *http.Request, dest any) error {
	if r.Body == nil {
		return errEmptyBody
	}

	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dest); err != nil {
		if errors.Is(err, io.EOF) {
			return errEmptyBody
		}

		return fmt.Errorf("decode request: %w", err)
	}

	return nil
}

func isJSONRequest(r *http.Request) bool {
	return strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json")
}

type capabilityPayload struct {
	Label        string          `json:"label"`
	AccountID    string          `json:"account_id"`
	Scopes       json.RawMessage `json:"scopes"`
	Capabilities json.RawMessage `json:"capabilities"`
}

func decodeConnectRequest(r *http.Request, lookup func(string) (string, bool)) (ConnectRequest, error) {
	if isJSONRequest(r) {
		var payload capabilityPayload
		if err := decodeJSONBody(r, &payload); err != nil && !errors.Is(err, errEmptyBody) {
			return ConnectRequest{}, err
		}

		scopes, err := scopesFromPayload(payload.Scopes, payload.Capabilities, lookup)
		if err != nil {
			return ConnectRequest{}, err
		}

		return ConnectRequest{Label: payload.Label, Scopes: scopes}, nil
	}

	if err := r.ParseForm(); err != nil {
		return ConnectRequest{}, fmt.Errorf("parse form: %w", err)
	}

	return ConnectRequest{
		Label:  r.FormValue("label"),
		Scopes: formScopes(r, lookup),
	}, nil
}

func decodeReconnectRequest(r *http.Request, lookup func(string) (string, bool)) (ReconnectRequest, error) {
	if isJSONRequest(r) {
		var payload capabilityPayload
		if err := decodeJSONBody(r, &payload); err != nil {
			return ReconnectRequest{}, err
		}

		scopes, err := scopesFromPayload(payload.Scopes, payload.Capabilities, lookup)
		if err != nil {
			return ReconnectRequest{}, err
		}

		return ReconnectRequest{AccountID: payload.AccountID, Scopes: scopes}, nil
	}

	if err := r.ParseForm(); err != nil {
		return ReconnectRequest{}, fmt.Errorf("parse form: %w", err)
	}

	return ReconnectRequest{AccountID: r.FormValue("account_id"), Scopes: formScopes(r, lookup)}, nil
}

func jsonFieldPresent(raw json.RawMessage) bool {
	trim := strings.TrimSpace(string(raw))

	return trim != "" && trim != "null"
}

func scopesFromPayload(scopesRaw, capsRaw json.RawMessage, lookup func(string) (string, bool)) ([]string, error) {
	hasScopes := jsonFieldPresent(scopesRaw)
	hasCaps := jsonFieldPresent(capsRaw)

	if !hasScopes && !hasCaps {
		return nil, nil
	}

	out := make([]string, 0)

	if hasScopes {
		var scopes []string
		if err := json.Unmarshal(scopesRaw, &scopes); err != nil {
			return nil, fmt.Errorf("decode scopes: %w", err)
		}

		out = append(out, scopes...)
	}

	if hasCaps {
		var caps []string
		if err := json.Unmarshal(capsRaw, &caps); err != nil {
			return nil, fmt.Errorf("decode capabilities: %w", err)
		}

		for _, cap := range caps {
			if scope, ok := lookup(cap); ok {
				out = append(out, scope)
			}
		}
	}

	return out, nil
}

func decodeDisconnectRequest(r *http.Request) (DisconnectRequest, error) {
	if isJSONRequest(r) {
		var req DisconnectRequest
		if err := decodeJSONBody(r, &req); err != nil {
			return DisconnectRequest{}, err
		}

		return req, nil
	}

	if err := r.ParseForm(); err != nil {
		return DisconnectRequest{}, fmt.Errorf("parse form: %w", err)
	}

	return DisconnectRequest{AccountID: r.FormValue("account_id")}, nil
}

func formScopes(r *http.Request, lookup func(string) (string, bool)) []string {
	_, hasCapabilities := r.Form["capabilities"]

	_, hasScopes := r.Form["scopes"]
	if !hasCapabilities && !hasScopes {
		return nil
	}

	out := make([]string, 0, len(r.Form["scopes"])+len(r.Form["capabilities"]))

	out = append(out, r.Form["scopes"]...)
	for _, cap := range r.Form["capabilities"] {
		if scope, ok := lookup(cap); ok {
			out = append(out, scope)
		}
	}

	return out
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrInvalidPrincipal), errors.Is(err, ErrInvalidClientName), errors.Is(err, ErrMissingScopes), errors.Is(err, ErrMissingCode):
		return http.StatusBadRequest
	case errors.Is(err, ErrForbiddenAccount), errors.Is(err, ErrInvalidState), errors.Is(err, ErrInvalidPKCE), errors.Is(err, ErrInvalidRedirect), errors.Is(err, ErrInvalidHost), errors.Is(err, ErrInvalidOrigin):
		return http.StatusForbidden
	case errors.Is(err, ErrUnknownAccount):
		return http.StatusNotFound
	case errors.Is(err, ErrSubjectMismatch), errors.Is(err, ErrMissingRefresh), errors.Is(err, ErrUnverifiedIdentity), errors.Is(err, ErrProviderDenied):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func publicError(err error) string {
	switch {
	case errors.Is(err, ErrInvalidState), errors.Is(err, ErrInvalidPKCE), errors.Is(err, ErrInvalidRedirect), errors.Is(err, ErrSessionExpired), errors.Is(err, ErrInvalidHost), errors.Is(err, ErrInvalidOrigin):
		return "signin_rejected"
	case errors.Is(err, ErrMissingRefresh):
		return "missing_refresh_token"
	case errors.Is(err, ErrSubjectMismatch):
		return "account_mismatch"
	case errors.Is(err, ErrUnverifiedIdentity):
		return "unverified_identity"
	case errors.Is(err, ErrProviderDenied):
		return "access_denied"
	case errors.Is(err, ErrUnknownAccount):
		return "unknown_account"
	case errors.Is(err, ErrForbiddenAccount):
		return "forbidden"
	default:
		return "signin_failed"
	}
}

func statusCopy(code string) string {
	switch code {
	case "signin_rejected":
		return "Sign-in was rejected. Start connect again from this page."
	case "missing_refresh_token":
		return "Google did not return offline access. Reconnect and grant access again."
	case "account_mismatch":
		return "That Google account does not match this connection. Pick the original account."
	case "unverified_identity":
		return "Google did not return a verified account identity."
	case "access_denied":
		return "Google access was denied."
	case "unknown_account":
		return "That connection is gone."
	case "forbidden":
		return "That connection is not available."
	default:
		return "Sign-in failed. Try again."
	}
}
