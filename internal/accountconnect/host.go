package accountconnect

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func requestHost(r *http.Request) string {
	if r == nil {
		return ""
	}

	return strings.TrimSpace(r.Host)
}

func loopbackHostEquivalent(got, want string) bool {
	ports := func(host string) (string, bool) {
		h := strings.ToLower(strings.TrimSpace(host))

		name, port, err := net.SplitHostPort(h)
		if err != nil {
			return "", false
		}

		return port, name == "localhost" || name == "127.0.0.1" || name == "::1"
	}
	gotPort, gotOK := ports(got)
	wantPort, wantOK := ports(want)

	return gotOK && wantOK && gotPort == wantPort
}

func checkRequestHost(r *http.Request, redirectURL string) error {
	wantURL, err := url.Parse(redirectURL)
	if err != nil || wantURL.Host == "" {
		return ErrInvalidRedirect
	}

	got := requestHost(r)
	if got == "" {
		return ErrInvalidHost
	}

	allowAlias := r.Method == http.MethodGet && (r.URL.Path == PathAccounts || r.URL.Path == PathStatus)
	if !strings.EqualFold(got, wantURL.Host) && !(allowAlias && loopbackHostEquivalent(got, wantURL.Host)) {
		return ErrInvalidHost
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return nil
	}

	parsed, parseErr := url.Parse(origin)
	if parseErr != nil || parsed.Host == "" || parsed.Scheme == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return ErrInvalidOrigin
	}

	if !strings.EqualFold(parsed.Host, wantURL.Host) && !(allowAlias && loopbackHostEquivalent(parsed.Host, wantURL.Host)) {
		return ErrInvalidOrigin
	}

	if !strings.EqualFold(parsed.Scheme, wantURL.Scheme) {
		return ErrInvalidOrigin
	}

	return nil
}
