package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

const schemeHTTPS = "https"

var errRedirect = errors.New("google API redirects are not followed")

type httpClientBundle struct {
	client *http.Client
	def    mcpcontract.Definition
}

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

func (b *httpClientBundle) do(ctx context.Context, method, rawURL string, body []byte, contentType string, maxBytes int) ([]byte, http.Header, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, nil, publicError(err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, nil, b.afterDispatchError(err)
	}
	defer resp.Body.Close()

	if resp.ContentLength > int64(maxBytes) {
		return nil, nil, b.oversizeResponse()
	}

	payload, err := readBounded(resp.Body, maxBytes)
	if err != nil {
		if errors.Is(err, errOversizeRead) {
			return nil, nil, b.oversizeResponse()
		}

		return nil, nil, b.afterDispatchError(err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if b.isWrite() && ambiguousWriteStatus(resp.StatusCode) {
			return nil, nil, writeError(&gapi.Error{Code: resp.StatusCode})
		}

		if checkErr := gapi.CheckResponseWithBody(resp, payload); checkErr != nil {
			return nil, nil, b.afterDispatchError(checkErr)
		}

		return nil, nil, publicError(&gapi.Error{Code: resp.StatusCode})
	}

	return payload, resp.Header, nil
}

func (b *httpClientBundle) isWrite() bool {
	return b.def.Retry != mcpcontract.SafeRead
}

func (b *httpClientBundle) afterDispatchError(err error) error {
	if b.isWrite() {
		return writeError(err)
	}

	return publicError(err)
}

func (b *httpClientBundle) oversizeResponse() error {
	if b.isWrite() {
		return writeError(errOversizeRead)
	}

	return oversize()
}

var errOversizeRead = errors.New("media response exceeded the bound")

func readBounded(r io.Reader, maxBytes int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(maxBytes)+1))
	if err != nil {
		if len(data) > maxBytes {
			return nil, errOversizeRead
		}

		return nil, fmt.Errorf("read google response: %w", err)
	}

	if len(data) > maxBytes {
		return nil, errOversizeRead
	}

	return data, nil
}

func ambiguousWriteStatus(code int) bool {
	return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500
}
