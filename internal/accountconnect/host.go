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

func checkRequestHost(r *http.Request, redirectURL string) error {
	wantURL, err := url.Parse(redirectURL)
	if err != nil || wantURL.Host == "" {
		return ErrInvalidRedirect
	}

	got := requestHost(r)
	if got == "" || !strings.EqualFold(got, wantURL.Host) {
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

	if !strings.EqualFold(parsed.Host, wantURL.Host) {
		return ErrInvalidOrigin
	}

	if parsed.Scheme != "" && wantURL.Scheme != "" && parsed.Scheme != wantURL.Scheme {
		return ErrInvalidOrigin
	}

	return nil
}
