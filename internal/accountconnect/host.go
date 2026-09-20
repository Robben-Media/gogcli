package accountconnect

import (
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
	normalize := func(host string) (string, bool) {
		h := strings.ToLower(strings.TrimSpace(host))
		h = strings.TrimSuffix(h, ".")
		isV4 := strings.HasPrefix(h, "127.0.0.1:")
		isV6 := strings.HasPrefix(h, "[::1]:")
		if !isV4 && !isV6 {
			return "", false
		}
		return h[strings.IndexByte(h, ':'):], true
	}
	gotPort, gotOK := normalize(got)
	wantPort, wantOK := normalize(want)
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
