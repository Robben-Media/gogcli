package accountconnect

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestLoopbackAliasesKeepPortAndOriginBoundary(t *testing.T) {
	for _, want := range []string{"localhost:9", "127.0.0.1:9", "[::1]:9"} {
		for _, got := range []string{"localhost:9", "127.0.0.1:9", "[::1]:9"} {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+got+PathAccounts, nil)
			r.Header.Set("Origin", "http://"+got)

			if err := checkRequestHost(r, "http://"+want+PathCallback); err != nil {
				t.Fatalf("%s -> %s: %v", got, want, err)
			}
		}
	}

	for _, got := range []string{"evil.test:9", "localhost:10", "127.0.0.1:10", "127.0.0.2:9", "localhost.evil.test:9"} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+got+PathAccounts, nil)
		if err := checkRequestHost(r, "http://localhost:9"+PathCallback); err == nil {
			t.Fatalf("accepted host %s", got)
		}
	}

	for _, origin := range []string{"http://evil.test:9", "http://localhost:10", "https://localhost:9", "//localhost:9", "http://localhost:9/path", "http://user@localhost:9", "http://localhost:9?x=1", "http://localhost:9#fragment"} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost:9"+PathAccounts, nil)
		r.Header.Set("Origin", origin)

		if err := checkRequestHost(r, "http://localhost:9"+PathCallback); err == nil {
			t.Fatalf("accepted origin %s", origin)
		}
	}
}

func TestAccountPageCanonicalizesBeforeSettingCookies(t *testing.T) {
	ctrl := testController(t, &fakeProvider{})

	h, err := NewHandler(ctrl, HandlerOptions{Principal: mcpcontract.Principal{ID: "jeremy"}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost:9"+PathAccounts, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "http://127.0.0.1:9/accounts" || len(w.Result().Cookies()) != 0 {
		t.Fatalf("alias response: %d %v", w.Code, w.Header())
	}
	r = httptest.NewRequestWithContext(t.Context(), http.MethodGet, w.Header().Get("Location"), nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK || cookieNamed(w, CSRFCookieName) == nil {
		t.Fatalf("canonical page failed: %d", w.Code)
	}
}

func TestAliasMutationAndCallbackRejected(t *testing.T) {
	for _, path := range []string{PathConnect, PathReconnect, PathDisconnect, PathCallback} {
		method := http.MethodPost
		if path == PathCallback {
			method = http.MethodGet
		}

		r := httptest.NewRequestWithContext(t.Context(), method, "http://localhost:9"+path, nil)
		if err := checkRequestHost(r, "http://127.0.0.1:9"+PathCallback); err == nil {
			t.Fatalf("alias accepted for %s", path)
		}
	}
}

func TestStatusCanonicalizesBeforeSettingCookies(t *testing.T) {
	h, err := NewHandler(testController(t, &fakeProvider{}), HandlerOptions{Principal: mcpcontract.Principal{ID: "jeremy"}})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost:9/status?ok=0", nil))

	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "http://127.0.0.1:9/status?ok=0" || len(w.Result().Cookies()) != 0 {
		t.Fatalf("unsafe status response: %d %v", w.Code, w.Header())
	}
}

func TestAliasBrowserConnectCookieContinuity(t *testing.T) {
	for _, host := range []string{"localhost:9", "127.0.0.1:9"} {
		t.Run(host, func(t *testing.T) {
			ctrl := testController(t, &fakeProvider{})
			ctrl.redirectURL = "http://" + host + PathCallback

			h, err := NewHandler(ctrl, HandlerOptions{Principal: mcpcontract.Principal{ID: "jeremy"}})
			if err != nil {
				t.Fatal(err)
			}

			alias := "localhost:9"
			if host == alias {
				alias = "127.0.0.1:9"
			}
			first := httptest.NewRecorder()
			h.ServeHTTP(first, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+alias+PathAccounts, nil))

			if first.Code != http.StatusSeeOther {
				t.Fatalf("alias status %d", first.Code)
			}

			pageURL, err := url.Parse(first.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			page := httptest.NewRecorder()
			h.ServeHTTP(page, httptest.NewRequestWithContext(t.Context(), http.MethodGet, pageURL.String(), nil))

			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}

			jar.SetCookies(pageURL, page.Result().Cookies())

			csrf := cookieNamed(page, CSRFCookieName)
			if csrf == nil {
				t.Fatal("no CSRF cookie")
			}
			connectURL := "http://" + host + PathConnect
			post := httptest.NewRequestWithContext(t.Context(), http.MethodPost, connectURL, strings.NewReader(`{}`))
			post.Header.Set("Content-Type", "application/json")
			post.Header.Set("Accept", "application/json")
			post.Header.Set("Origin", "http://"+host)
			post.Header.Set(CSRFHeaderName, csrf.Value)

			for _, cookie := range jar.Cookies(pageURL) {
				post.AddCookie(cookie)
			}
			started := httptest.NewRecorder()
			h.ServeHTTP(started, post)

			if started.Code != http.StatusOK {
				t.Fatalf("connect status %d: %s", started.Code, started.Body.String())
			}

			jar.SetCookies(pageURL, started.Result().Cookies())

			var result StartResult
			if decodeErr := json.Unmarshal(started.Body.Bytes(), &result); decodeErr != nil {
				t.Fatal(decodeErr)
			}

			cbURL, err := url.Parse(ctrl.RedirectURL() + "?code=fake&state=" + url.QueryEscape(result.SessionID))
			if err != nil {
				t.Fatal(err)
			}

			cb := httptest.NewRequestWithContext(t.Context(), http.MethodGet, cbURL.String(), nil)
			for _, cookie := range jar.Cookies(cbURL) {
				cb.AddCookie(cookie)
			}
			done := httptest.NewRecorder()
			h.ServeHTTP(done, cb)

			if done.Code != http.StatusSeeOther || !strings.Contains(done.Header().Get("Location"), "ok=1") {
				t.Fatalf("callback failed: %d %s", done.Code, done.Header().Get("Location"))
			}

			accounts, err := ctrl.ListAccounts(t.Context(), "jeremy")
			if err != nil || len(accounts) != 1 {
				t.Fatalf("account not stored: %d %v", len(accounts), err)
			}
		})
	}
}
