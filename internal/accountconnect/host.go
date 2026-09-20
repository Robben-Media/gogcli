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
		_, port, err := net.SplitHostPort(h)
		if err != nil {
			return "", false
		}
		ip := net.ParseIP(h)
		isLoopbackIP := ip != nil && ip.IsLoopback()
		return port, isLoopbackIP || h == "localhost"
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
	if got == "" || (!strings.EqualFold(got, wantURL.Host) && !loopbackHostEquivalent(got, wantURL.Host)) {
		return ErrInvalidHost
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return nil
	}

	parsed, parseErr := url.Parse(origin)
	if parseErr != nil || parsed.Host == "" {
		return ErrInvalidOrigin
	}

	if !strings.EqualFold(parsed.Host, wantURL.Host) && !loopbackHostEquivalent(parsed.Host, wantURL.Host) {
		return ErrInvalidOrigin
	}

	if parsed.Scheme != "" && wantURL.Scheme != "" && parsed.Scheme != wantURL.Scheme {
		return ErrInvalidOrigin
	}

	return nil
}
