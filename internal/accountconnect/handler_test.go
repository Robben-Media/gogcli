package accountconnect

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func withHost(r *http.Request) *http.Request {
	r.Host = "127.0.0.1:9"
	r.Header.Set("Origin", "http://127.0.0.1:9")

	return r
}

func TestHandlerCSRFAndTrustedPrincipal(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)

	h, err := NewHandler(ctrl, HandlerOptions{Principal: mcpcontract.Principal{ID: "jeremy"}})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	get := withHost(httptest.NewRequestWithContext(t.Context(), http.MethodGet, PathAccounts, nil))
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, get)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET accounts=%d %s", getRec.Code, getRec.Body.String())
	}

	var page PageModel
	if err := json.Unmarshal(getRec.Body.Bytes(), &page); err != nil {
		t.Fatalf("page: %v", err)
	}

	if page.ClientName != "default" || len(page.ScopeChoices) == 0 {
		t.Fatalf("page=%+v", page)
	}

	cookie := cookieNamed(getRec, CSRFCookieName)
	if cookie == nil {
		t.Fatal("missing csrf cookie")
	}

	post := withHost(httptest.NewRequestWithContext(t.Context(), http.MethodPost, PathConnect, bytes.NewBufferString(`{"principal_id":"attacker","client_name":"other"}`)))
	post.Header.Set("Content-Type", "application/json")
	post.Header.Set("Accept", "application/json")
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, post)

	if postRec.Code != http.StatusForbidden {
		t.Fatalf("missing csrf=%d", postRec.Code)
	}

	post = withHost(httptest.NewRequestWithContext(t.Context(), http.MethodPost, PathConnect, bytes.NewBufferString(`{"label":"Personal"}`)))
	post.Header.Set("Content-Type", "application/json")
	post.Header.Set("Accept", "application/json")
	post.Header.Set(CSRFHeaderName, cookie.Value)
	post.AddCookie(cookie)
	postRec = httptest.NewRecorder()
	h.ServeHTTP(postRec, post)

	if postRec.Code != http.StatusOK {
		t.Fatalf("connect=%d %s", postRec.Code, postRec.Body.String())
	}

	var started StartResult
	if err := json.Unmarshal(postRec.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if started.AuthURL == "" || cookieNamed(postRec, OAuthSessionCookieName) == nil {
		t.Fatalf("start=%+v cookies=%v", started, postRec.Result().Cookies())
	}

	if provider.last.SelectAccount != true {
		t.Fatal("connect should use account chooser")
	}
}

func TestHandlerFormConnectAndHostGuard(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)

	h, err := NewHandler(ctrl, HandlerOptions{Principal: mcpcontract.Principal{ID: "jeremy"}})
	if err != nil {
		t.Fatal(err)
	}
	get := withHost(httptest.NewRequestWithContext(t.Context(), http.MethodGet, PathAccounts, nil))
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, get)

	csrf := cookieNamed(getRec, CSRFCookieName)
	if csrf == nil {
		t.Fatal("csrf")
	}

	form := url.Values{"csrf_token": {csrf.Value}, "label": {"Personal"}, "scopes": {DefaultConnectScopes()[2]}}
	post := withHost(httptest.NewRequestWithContext(t.Context(), http.MethodPost, PathConnect, strings.NewReader(form.Encode())))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(csrf)
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, post)

	if postRec.Code != http.StatusFound {
		t.Fatalf("form connect=%d %s", postRec.Code, postRec.Body.String())
	}

	bad := httptest.NewRequestWithContext(t.Context(), http.MethodGet, PathAccounts, nil)
	bad.Host = "evil.example"
	badRec := httptest.NewRecorder()
	h.ServeHTTP(badRec, bad)

	if badRec.Code != http.StatusForbidden {
		t.Fatalf("rebinding=%d", badRec.Code)
	}
}

func TestHandlerCallbackRequiresBrowserCookie(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)

	h, err := NewHandler(ctrl, HandlerOptions{Principal: mcpcontract.Principal{ID: "jeremy"}})
	if err != nil {
		t.Fatal(err)
	}

	started, err := ctrl.StartConnect(t.Context(), ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}
	cb := withHost(httptest.NewRequestWithContext(t.Context(), http.MethodGet, PathCallback+"?code=c&state="+started.SessionID, nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, cb)

	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "signin_rejected") {
		t.Fatalf("missing browser cookie: %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}

	return nil
}

func TestHandlerDisconnectStatusIndependentOfRecord(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)

	h, err := NewHandler(ctrl, HandlerOptions{Principal: mcpcontract.Principal{ID: "jeremy"}})
	if err != nil {
		t.Fatal(err)
	}
	req := withHost(httptest.NewRequestWithContext(t.Context(), http.MethodGet, PathStatus+"?ok=1&action=disconnected", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}

	var status StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}

	if !status.OK || !status.Disconnected || status.Action != ActionDisconnected || status.Detail == "" || status.Account != nil {
		t.Fatalf("status=%+v", status)
	}
}

func TestHandlerCapabilitiesFormOmitsUnselectedGmail(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)

	h, err := NewHandler(ctrl, HandlerOptions{Principal: mcpcontract.Principal{ID: "jeremy"}})
	if err != nil {
		t.Fatal(err)
	}
	get := withHost(httptest.NewRequestWithContext(t.Context(), http.MethodGet, PathAccounts, nil))
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, get)
	csrf := cookieNamed(getRec, CSRFCookieName)
	form := url.Values{"csrf_token": {csrf.Value}, "label": {"Work"}, "capabilities": {"calendar.read"}}
	post := withHost(httptest.NewRequestWithContext(t.Context(), http.MethodPost, PathConnect, strings.NewReader(form.Encode())))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(csrf)
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, post)

	if postRec.Code != http.StatusFound {
		t.Fatalf("connect=%d %s", postRec.Code, postRec.Body.String())
	}
	foundGmail := false
	foundCal := false

	for _, scope := range provider.last.Scopes {
		if scope == "https://www.googleapis.com/auth/gmail.readonly" {
			foundGmail = true
		}

		if scope == "https://www.googleapis.com/auth/calendar.readonly" {
			foundCal = true
		}
	}

	if foundGmail || !foundCal {
		t.Fatalf("calendar-only connect scopes=%v", provider.last.Scopes)
	}
}

func TestHandlerReconnectCapabilitiesForm(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)
	ctx := t.Context()

	started, err := ctrl.StartConnect(ctx, ConnectRequest{PrincipalID: "jeremy"})
	if err != nil {
		t.Fatal(err)
	}
	provider.exchange = func(ExchangeParams) (TokenSet, error) {
		return TokenSet{RefreshToken: "rt", Subject: "sub", Email: "me@gmail.com", Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}, nil
	}

	first, err := ctrl.CompleteCallback(ctx, CallbackRequest{Code: "c", State: started.SessionID, BrowserID: started.SessionID, RedirectURL: ctrl.RedirectURL()})
	if err != nil {
		t.Fatal(err)
	}

	h, err := NewHandler(ctrl, HandlerOptions{Principal: mcpcontract.Principal{ID: "jeremy"}})
	if err != nil {
		t.Fatal(err)
	}
	get := withHost(httptest.NewRequestWithContext(ctx, http.MethodGet, PathAccounts, nil))
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, get)
	csrf := cookieNamed(getRec, CSRFCookieName)
	form := url.Values{"csrf_token": {csrf.Value}, "account_id": {first.Account.AccountID}, "capabilities": {"calendar.read"}}
	post := withHost(httptest.NewRequestWithContext(ctx, http.MethodPost, PathReconnect, strings.NewReader(form.Encode())))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(csrf)
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, post)

	if postRec.Code != http.StatusFound {
		t.Fatalf("reconnect=%d %s", postRec.Code, postRec.Body.String())
	}
	foundGmail := false
	foundCal := false

	for _, scope := range provider.last.Scopes {
		if scope == "https://www.googleapis.com/auth/gmail.readonly" {
			foundGmail = true
		}

		if scope == "https://www.googleapis.com/auth/calendar.readonly" {
			foundCal = true
		}
	}

	if !foundGmail || !foundCal {
		t.Fatalf("reconnect scopes=%v", provider.last.Scopes)
	}
}

func TestHandlerJSONCalendarOnlyCapabilities(t *testing.T) {
	provider := &fakeProvider{}
	ctrl := testController(t, provider)

	h, err := NewHandler(ctrl, HandlerOptions{Principal: mcpcontract.Principal{ID: "jeremy"}})
	if err != nil {
		t.Fatal(err)
	}

	get := withHost(httptest.NewRequestWithContext(t.Context(), http.MethodGet, PathAccounts, nil))
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, get)
	csrf := cookieNamed(getRec, CSRFCookieName)

	post := withHost(httptest.NewRequestWithContext(t.Context(), http.MethodPost, PathConnect, strings.NewReader(`{"label":"Work","capabilities":["calendar.read"]}`)))
	post.Header.Set("Content-Type", "application/json")
	post.Header.Set("Accept", "application/json")
	post.Header.Set(CSRFHeaderName, csrf.Value)
	post.AddCookie(csrf)
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, post)

	if postRec.Code != http.StatusOK {
		t.Fatalf("connect=%d %s", postRec.Code, postRec.Body.String())
	}

	foundGmail := false
	foundCal := false

	for _, scope := range provider.last.Scopes {
		if scope == mcpcontract.GmailReadScope {
			foundGmail = true
		}

		if scope == mcpcontract.CalendarReadScope {
			foundCal = true
		}
	}

	if foundGmail || !foundCal {
		t.Fatalf("json calendar-only scopes=%v", provider.last.Scopes)
	}
}
