package apiexec

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/steipete/gogcli/internal/googlecatalog"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

func googleAPIHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || strings.HasPrefix(host, "[") {
		return false
	}

	if _, _, err := net.SplitHostPort(host); err == nil {
		return false
	}

	if ip := net.ParseIP(host); ip != nil {
		return false
	}

	return host == "googleapis.com" || host == "www.googleapis.com" || strings.HasSuffix(host, ".googleapis.com")
}

func buildRequestURL(method googlecatalog.Method, pathValues map[string]string, query url.Values) (*url.URL, error) {
	base, err := url.Parse(method.BaseURL)
	if err != nil || base.Scheme != schemeHTTPS || base.User != nil || !googleAPIHost(base.Host) {
		return nil, invalid("method endpoint is not a pinned Google API URL")
	}

	expanded, err := expandPath(method.Path, pathValues)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(expanded, "/") {
		return nil, invalid("method path is invalid")
	}

	joined := strings.TrimSuffix(base.String(), "/") + "/" + strings.TrimPrefix(expanded, "/")

	parsed, err := url.Parse(joined)
	if err != nil {
		return nil, invalid("method path is invalid")
	}

	if parsed.Scheme != schemeHTTPS || parsed.User != nil || parsed.Host != base.Host || !googleAPIHost(parsed.Host) {
		return nil, invalid("method endpoint is not a pinned Google API URL")
	}

	if err := validateExpandedPath(parsed.Path); err != nil {
		return nil, err
	}
	parsed.Fragment = ""
	parsed.RawQuery = query.Encode()

	return parsed, nil
}

func expandPath(path string, values map[string]string) (string, error) {
	var out strings.Builder

	remaining := path
	for {
		start := strings.IndexByte(remaining, '{')
		if start < 0 {
			out.WriteString(remaining)
			return out.String(), nil
		}

		end := strings.IndexByte(remaining[start:], '}')
		if end < 0 {
			return "", invalid("method path is invalid")
		}
		end += start
		out.WriteString(remaining[:start])
		token := remaining[start+1 : end]
		remaining = remaining[end+1:]

		reserved := false
		if strings.HasPrefix(token, "+") {
			reserved = true
			token = token[1:]
		}

		if token == "" || strings.ContainsAny(token, "{}/") {
			return "", invalid("method path is invalid")
		}

		value, ok := values[token]
		if !ok {
			return "", invalid(token + " is required")
		}

		if err := validatePathValue(token, value, reserved); err != nil {
			return "", err
		}

		if reserved {
			out.WriteString(encodeReserved(value))
		} else {
			out.WriteString(url.PathEscape(value))
		}
	}
}

func validatePathValue(name, value string, reserved bool) error {
	if strings.TrimSpace(value) == "" {
		return invalid(name + " is required")
	}

	if len(value) > maxPathBytes || !utf8.ValidString(value) {
		return invalid(name + " is invalid")
	}

	if strings.ContainsRune(value, 0) || strings.ContainsAny(value, "\\\r\n") {
		return invalid(name + " is invalid")
	}

	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return invalid(name + " is invalid")
		}
	}

	if reserved {
		if strings.Contains(value, "://") || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "?#") {
			return invalid(name + " is invalid")
		}
	}

	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return invalid(name + " is invalid")
		}
	}

	return nil
}

func validateExpandedPath(path string) error {
	if strings.Contains(path, "\\") || strings.Contains(path, "://") || strings.HasPrefix(path, "//") {
		return invalid("method path is invalid")
	}

	for _, segment := range strings.Split(path, "/") {
		if segment == ".." {
			return invalid("method path is invalid")
		}
	}

	return nil
}

func encodeReserved(value string) string {
	var out strings.Builder

	for i := 0; i < len(value); i++ {
		c := value[i]
		if isUnreserved(c) || c == '/' {
			out.WriteByte(c)
			continue
		}

		out.WriteByte('%')
		out.WriteByte(upperHex[c>>4])
		out.WriteByte(upperHex[c&15])
	}

	return out.String()
}

const upperHex = "0123456789ABCDEF"

func isUnreserved(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~'
}

var errRedirect = errors.New("google API redirects are not followed")

func pinClient(client *http.Client) *http.Client {
	cloned := *client
	cloned.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errRedirect
	}
	cloned.Transport = &pinnedTransport{base: client.Transport}

	return &cloned
}

type pinnedTransport struct {
	base http.RoundTripper
}

func (t *pinnedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, invalid("request is required")
	}

	if req.URL.Scheme != schemeHTTPS || req.URL.User != nil || !googleAPIHost(req.URL.Host) {
		return nil, invalid("request endpoint is not a pinned Google API URL")
	}

	if err := validateExpandedPath(req.URL.Path); err != nil {
		return nil, err
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	response, err := base.RoundTrip(req)
	if err != nil {
		var safe *mcpcontract.Error
		if errors.As(err, &safe) {
			return nil, err //nolint:wrapcheck // Preserve typed contract errors.
		}

		return nil, fmt.Errorf("google API request: %w", err)
	}

	return response, nil
}
