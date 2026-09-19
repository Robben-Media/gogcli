package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

type recordingProvider struct {
	mu     sync.Mutex
	params []accountconnect.AuthCodeParams
}

func (p *recordingProvider) AuthCodeURL(params accountconnect.AuthCodeParams) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.params = append(p.params, params)

	return "https://accounts.google.com/o/oauth2/auth?state=" + url.QueryEscape(params.State), nil
}

func (p *recordingProvider) Exchange(context.Context, accountconnect.ExchangeParams) (accountconnect.TokenSet, error) {
	return accountconnect.TokenSet{}, nil
}

func (p *recordingProvider) Revoke(context.Context, string) error {
	return nil
}

func (p *recordingProvider) lastScopes() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.params) == 0 {
		return nil
	}

	return append([]string(nil), p.params[len(p.params)-1].Scopes...)
}

func TestRenderAccountsTwoAccountsFormsAndEscaping(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	page := accountconnect.PageModel{
		PrincipalID:           "principal <id>",
		CSRFToken:             `csrf <token> "autofocus"`,
		ClientName:            `trusted-bucket <client>`,
		RequestedCapabilities: []string{"gmail.read", "calendar.read", "unexpected <capability>"},
		ScopeChoices: []accountconnect.ScopeChoice{
			{Capability: "gmail.read", Scope: "https://www.googleapis.com/auth/gmail.readonly", Required: true},
			{Capability: "calendar.read", Scope: "https://www.googleapis.com/auth/calendar.readonly"},
			{Capability: "unexpected <capability>", Scope: "https://example.test/unexpected <scope>"},
		},
		Accounts: []accountconnect.AccountView{
			{
				AccountID:    `acct-personal <opaque> "autofocus"`,
				Email:        "personal@example.test <script>alert(1)</script>",
				Label:        `Personal <script>alert("label")</script>`,
				ClientName:   `trusted-personal <client>`,
				AuthMode:     "oauth",
				Scopes:       []string{"https://www.googleapis.com/auth/gmail.readonly"},
				Capabilities: []string{"gmail.read", "drive.read"},
				UpdatedAt:    time.Now(),
			},
			{
				AccountID:    "acct-work",
				Email:        "work@example.test",
				Label:        "Work",
				ClientName:   "trusted-work",
				AuthMode:     "oauth",
				Scopes:       []string{"https://www.googleapis.com/auth/calendar.readonly"},
				Capabilities: []string{"calendar.read"},
				UpdatedAt:    time.Now(),
			},
		},
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/accounts", nil)

	if err := renderer.RenderAccounts(recorder, request, page); err != nil {
		t.Fatalf("RenderAccounts: %v", err)
	}

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, body)
	}

	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}

	if got := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'none'") || !strings.Contains(got, "form-action 'self'") {
		t.Fatalf("unsafe content security policy = %q", got)
	}

	if !strings.Contains(body, "Personal &lt;script&gt;") || strings.Contains(body, "<script>alert") {
		t.Fatalf("account content was not escaped: %s", body)
	}

	if !strings.Contains(body, "unexpected &lt;capability&gt;") {
		t.Fatalf("capability content was not escaped: %s", body)
	}

	if !strings.Contains(body, `<input id="scope-0" type="checkbox" checked disabled>`) {
		t.Fatal("default Gmail capability is not shown as always included")
	}

	if !strings.Contains(body, `<input id="scope-1" type="checkbox" name="capabilities" value="calendar.read" checked>`) {
		t.Fatal("selected optional capability is not submitted with an opaque capability value")
	}

	if !strings.Contains(body, `<input id="reconnect-0-scope-0" type="checkbox" name="capabilities" value="calendar.read">`) {
		t.Fatal("reconnect form does not offer an ungranted capability")
	}

	if !strings.Contains(body, "Connect another account") || !strings.Contains(body, "Work") {
		t.Fatal("second-account state is missing")
	}

	if !strings.Contains(body, `Gmail read</span><span class="capability-status">Granted`) {
		t.Fatal("actual granted capabilities are missing")
	}

	if got := strings.Count(body, `name="csrf_token"`); got != 5 {
		t.Fatalf("csrf field count = %d, want 5", got)
	}

	if got := strings.Count(body, `name="account_id"`); got != 4 {
		t.Fatalf("account_id field count = %d, want 4", got)
	}

	if !strings.Contains(body, `value="csrf &lt;token&gt; &#34;autofocus&#34;"`) {
		t.Fatal("CSRF token was not escaped")
	}

	if !strings.Contains(body, `value="acct-personal &lt;opaque&gt; &#34;autofocus&#34;"`) {
		t.Fatal("opaque account ID was not escaped")
	}

	hiddenField := regexp.MustCompile(`<input type="hidden" name="([^"]+)"`)
	for _, match := range hiddenField.FindAllStringSubmatch(body, -1) {
		if match[1] != "csrf_token" && match[1] != "account_id" {
			t.Fatalf("unexpected hidden form field %q", match[1])
		}
	}

	for _, value := range []string{"client_name", "trusted-bucket", "trusted-personal", "trusted-work", "www.googleapis.com/auth", "example.test/unexpected"} {
		if strings.Contains(body, value) {
			t.Fatalf("server-owned value %q leaked to the page", value)
		}
	}

	if got := strings.Count(body, `<form method="post" action="/connect">`); got != 1 {
		t.Fatalf("connect form count = %d, want 1", got)
	}

	if got := strings.Count(body, `<form method="post" action="/reconnect">`); got != 2 {
		t.Fatalf("reconnect form count = %d, want 2", got)
	}

	if got := strings.Count(body, `<form method="post" action="/disconnect">`); got != 2 {
		t.Fatalf("disconnect form count = %d, want 2", got)
	}

	if strings.Contains(body, "<script") || strings.Contains(body, "localStorage") || strings.Contains(body, "sessionStorage") || strings.Contains(body, "document.cookie") {
		t.Fatal("page contains a script or browser storage reference")
	}

	if !strings.Contains(body, `<html lang="en">`) || !strings.Contains(body, `<label for="label">`) {
		t.Fatal("base accessibility markup is missing")
	}
}

func TestRenderAccountsEmptyAndServerScopeChoices(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	page := accountconnect.PageModel{
		CSRFToken:             "csrf",
		ClientName:            "trusted-bucket",
		RequestedCapabilities: []string{"gmail.read"},
		ScopeChoices: []accountconnect.ScopeChoice{
			{Capability: "gmail.read", Scope: "https://www.googleapis.com/auth/gmail.readonly", Required: true},
			{Capability: "calendar.read", Scope: "https://www.googleapis.com/auth/calendar.readonly"},
		},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/accounts", nil)

	if err := renderer.RenderAccounts(recorder, request, page); err != nil {
		t.Fatalf("RenderAccounts: %v", err)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "No Google accounts are connected.") || !strings.Contains(body, "Connect Google account") {
		t.Fatalf("empty state is missing: %s", body)
	}

	if !strings.Contains(body, "Access Google will request") || !strings.Contains(body, "Gmail read (always included)") {
		t.Fatalf("server scope choices are missing: %s", body)
	}

	if got := strings.Count(body, `<input id="scope-0" type="checkbox" checked disabled>`); got != 1 {
		t.Fatalf("included capability markup count = %d, want 1", got)
	}

	if !strings.Contains(body, `<input id="scope-1" type="checkbox" name="capabilities" value="calendar.read">`) {
		t.Fatal("optional capability is not selectable by default")
	}

	if strings.Contains(body, "capabilities[]") {
		t.Fatal("capability field name does not match handler form decoding")
	}

	if strings.Contains(body, "client_name") || strings.Contains(body, "trusted-bucket") || strings.Contains(body, "www.googleapis.com/auth") {
		t.Fatal("trusted app or raw scope data leaked to the page")
	}
}

func TestRenderAccountsRequiresCSRF(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/accounts", nil)

	if err := renderer.RenderAccounts(recorder, request, accountconnect.PageModel{}); err == nil {
		t.Fatal("RenderAccounts unexpectedly accepted an empty CSRF token")
	}
}

func TestRenderedCapabilityFieldsReachHandler(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	provider := &recordingProvider{}
	registry := accountconnect.NewMemoryRegistry()

	seedErr := registry.Upsert(t.Context(), accountconnect.Record{
		AccountID:   "acct-personal",
		Subject:     "sub-personal",
		Email:       "personal@example.test",
		Label:       "Personal",
		PrincipalID: "jeremy",
		ClientName:  "default",
		AuthMode:    accountconnect.AuthModeOAuth,
		Scopes:      accountconnect.DefaultConnectScopes(),
	})
	if seedErr != nil {
		t.Fatalf("seed account: %v", seedErr)
	}

	ctrl, err := accountconnect.NewController(accountconnect.Options{
		Registry: registry,
		Tokens:   accountconnect.NewMemoryTokenStore(),
		OAuth:    provider,
		Credentials: func(string) (accountconnect.ClientCredentials, error) {
			return accountconnect.ClientCredentials{ClientID: "client-id"}, nil
		},
		RedirectURL: "http://127.0.0.1:9/oauth/callback",
	})
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}

	handler, err := accountconnect.NewHandler(ctrl, accountconnect.HandlerOptions{
		Principal: mcpcontract.Principal{ID: "jeremy"},
		Renderer:  renderer,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	get := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/accounts", nil)
	get.Host = "127.0.0.1:9"
	get.Header.Set("Accept", "text/html")
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, get)

	body := getRec.Body.String()
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET accounts=%d body=%s", getRec.Code, body)
	}

	if !strings.Contains(body, `name="capabilities"`) || strings.Contains(body, "capabilities[]") {
		t.Fatalf("rendered capability field is incompatible with handler form decoding: %s", body)
	}

	csrf := getRec.Result().Cookies()[0]
	connectForm := url.Values{
		"csrf_token":   {csrf.Value},
		"capabilities": {"calendar.read"},
	}
	connectPost := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/connect", strings.NewReader(connectForm.Encode()))
	connectPost.Host = "127.0.0.1:9"
	connectPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	connectPost.AddCookie(csrf)
	connectRec := httptest.NewRecorder()
	handler.ServeHTTP(connectRec, connectPost)

	if connectRec.Code != http.StatusFound {
		t.Fatalf("POST connect=%d body=%s", connectRec.Code, connectRec.Body.String())
	}

	if !hasAllScopes(provider.lastScopes(), "openid", "email", "https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/calendar.readonly") {
		t.Fatalf("connect scopes=%v", provider.lastScopes())
	}

	reconnectForm := url.Values{
		"csrf_token":   {csrf.Value},
		"account_id":   {"acct-personal"},
		"capabilities": {"calendar.read"},
	}
	reconnectPost := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/reconnect", strings.NewReader(reconnectForm.Encode()))
	reconnectPost.Host = "127.0.0.1:9"
	reconnectPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reconnectPost.AddCookie(csrf)
	reconnectRec := httptest.NewRecorder()
	handler.ServeHTTP(reconnectRec, reconnectPost)

	if reconnectRec.Code != http.StatusFound {
		t.Fatalf("POST reconnect=%d body=%s", reconnectRec.Code, reconnectRec.Body.String())
	}

	if !hasAllScopes(provider.lastScopes(), "openid", "email", "https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/calendar.readonly") {
		t.Fatalf("reconnect scopes=%v", provider.lastScopes())
	}
}

func hasAllScopes(got []string, want ...string) bool {
	set := make(map[string]bool, len(got))
	for _, scope := range got {
		set[scope] = true
	}

	for _, scope := range want {
		if !set[scope] {
			return false
		}
	}

	return true
}

func TestRenderStatusFailureStatesWithoutRawError(t *testing.T) {
	tests := []struct {
		code    string
		heading string
		message string
	}{
		{"signin_rejected", "Sign-in rejected", "Google did not complete this sign-in. Start again from the Google accounts page."},
		{"missing_refresh_token", "Offline access missing", "Google did not grant the offline access this connection needs. Reconnect and grant access again."},
		{"account_mismatch", "Wrong Google account", "The Google account you selected does not match this connection. Start again and choose the original account."},
		{"unverified_identity", "Identity not verified", "Google did not return a verified account identity. Try connecting again."},
		{"access_denied", "Access denied", "Google access was denied. You can reconnect later if you want this account available."},
		{"unknown_account", "Connection unavailable", "That connection is no longer present. Open Google accounts to connect it again."},
		{"forbidden", "Connection unavailable", "That connection is not available for this account-management page."},
		{"unexpected <error>", "Sign-in failed", "Google sign-in did not complete. Try again from the Google accounts page."},
	}

	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			renderer, err := NewRenderer()
			if err != nil {
				t.Fatalf("NewRenderer: %v", err)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/status", nil)
			page := accountconnect.PageModel{
				ClientName: `trusted-bucket <client>`,
				Status: &accountconnect.StatusResponse{
					OK:     false,
					Error:  test.code,
					Detail: `raw <b>internal detail</b>`,
				},
			}

			if err := renderer.RenderStatus(recorder, request, page); err != nil {
				t.Fatalf("RenderStatus: %v", err)
			}

			body := recorder.Body.String()
			if strings.Contains(body, "raw <b>") || strings.Contains(body, "internal detail") || strings.Contains(body, "trusted-bucket") {
				t.Fatalf("raw failure data leaked: %s", body)
			}

			if !strings.Contains(body, test.heading) || !strings.Contains(body, test.message) {
				t.Fatalf("friendly copy is missing for %q: %s", test.code, body)
			}

			if !strings.Contains(body, `id="status"`) || !strings.Contains(body, "autofocus") || !strings.Contains(body, `role="status"`) || !strings.Contains(body, `aria-live="polite"`) {
				t.Fatalf("focused status landmark is missing: %s", body)
			}

			if !strings.Contains(body, `<a class="return" href="/accounts">`) {
				t.Fatal("status recovery link is missing")
			}
		})
	}
}

func TestRenderStatusSuccessEscapesAccount(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	page := accountconnect.PageModel{
		ClientName: `trusted-bucket <client>`,
		Status: &accountconnect.StatusResponse{
			OK: true,
			Account: &accountconnect.AccountView{
				AccountID:    `acct-personal <opaque>`,
				Email:        "personal@example.test <script>alert(1)</script>",
				Label:        `Personal <script>alert("label")</script>`,
				ClientName:   `trusted-account <client>`,
				AuthMode:     "oauth",
				Scopes:       []string{"https://www.googleapis.com/auth/gmail.readonly"},
				Capabilities: []string{"gmail.read"},
				UpdatedAt:    time.Now(),
			},
		},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/status", nil)

	if err := renderer.RenderStatus(recorder, request, page); err != nil {
		t.Fatalf("RenderStatus: %v", err)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "Google account connected") || !strings.Contains(body, "personal@example.test &lt;script&gt;") {
		t.Fatalf("success state is missing or unescaped: %s", body)
	}

	if !strings.Contains(body, "Personal &lt;script&gt;") {
		t.Fatalf("status account label was not escaped: %s", body)
	}

	for _, value := range []string{"acct-personal", "trusted-bucket", "trusted-account", "www.googleapis.com/auth", "csrf_token"} {
		if strings.Contains(body, value) {
			t.Fatalf("unnecessary value %q leaked to the status page", value)
		}
	}
}

func TestRenderStatusCompletedWithoutAccount(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/status?ok=1&disconnected=1", nil)
	page := accountconnect.PageModel{Status: &accountconnect.StatusResponse{OK: true}}

	if err := renderer.RenderStatus(recorder, request, page); err != nil {
		t.Fatalf("RenderStatus: %v", err)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "Google account updated") || !strings.Contains(body, "The account request completed.") {
		t.Fatalf("completed-without-account state is missing: %s", body)
	}
}

func TestRenderStatusDisconnectedWithoutDeletedRecord(t *testing.T) {
	renderer, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	tests := []struct {
		name   string
		status accountconnect.StatusResponse
		detail string
	}{
		{
			name: "action and detail",
			status: accountconnect.StatusResponse{
				OK:           true,
				Action:       "disconnected",
				Disconnected: true,
				Detail:       `Disconnected personal@example.test. <script>alert(1)</script>`,
			},
			detail: "Disconnected personal@example.test. &lt;script&gt;alert(1)&lt;/script&gt;",
		},
		{
			name: "flag fallback",
			status: accountconnect.StatusResponse{
				OK:           true,
				Disconnected: true,
			},
			detail: "The connection was removed. Other Google accounts are unchanged.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/status?ok=1&action=disconnected", nil)
			page := accountconnect.PageModel{Status: &test.status}

			if err := renderer.RenderStatus(recorder, request, page); err != nil {
				t.Fatalf("RenderStatus: %v", err)
			}

			body := recorder.Body.String()
			if !strings.Contains(body, "Google account disconnected") || !strings.Contains(body, test.detail) {
				t.Fatalf("disconnect state is missing: %s", body)
			}
		})
	}
}
