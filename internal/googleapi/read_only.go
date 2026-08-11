package googleapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrReadOnly reports an outbound request denied by runtime read-only mode.
var ErrReadOnly = errors.New("request blocked by --readonly")

type readOnlyContextKey struct{}

// WithReadOnly enables fail-closed outbound mutation protection for ctx.
func WithReadOnly(ctx context.Context, enabled bool) context.Context {
	if !enabled {
		return ctx
	}
	return context.WithValue(ctx, readOnlyContextKey{}, true)
}

// ReadOnly reports whether runtime read-only protection is enabled.
func ReadOnly(ctx context.Context) bool {
	enabled, _ := ctx.Value(readOnlyContextKey{}).(bool)
	return enabled
}

type readOnlyTransport struct{ base http.RoundTripper }

func readOnlyTransportFromContext(ctx context.Context, base http.RoundTripper) http.RoundTripper {
	if !ReadOnly(ctx) {
		return base
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return readOnlyTransport{base: base}
}

func (t readOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !ReadOnlyRequestAllowed(req) {
		method, path := "", ""
		if req != nil {
			method = req.Method
			if req.URL != nil {
				path = req.URL.Path
			}
		}
		return nil, fmt.Errorf("%w: %s %s", ErrReadOnly, method, path)
	}
	return t.base.RoundTrip(req)
}

// ReadOnlyRequestAllowed identifies requests that cannot mutate remote state.
func ReadOnlyRequestAllowed(req *http.Request) bool {
	if req == nil || req.URL == nil || strings.TrimSpace(req.Header.Get("X-HTTP-Method-Override")) != "" {
		return false
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	case http.MethodPost:
		return readOnlyPOSTRequest(req)
	default:
		return false
	}
}

func readOnlyPOSTRequest(req *http.Request) bool {
	if req.URL.Scheme != "https" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(req.URL.Hostname(), "."))
	path := req.URL.Path
	switch host {
	case "www.googleapis.com", "www.mtls.googleapis.com", "calendar-json.googleapis.com", "calendar-json.mtls.googleapis.com":
		return strings.HasSuffix(path, "/calendar/v3/freeBusy")
	case "searchconsole.googleapis.com", "searchconsole.mtls.googleapis.com":
		return strings.HasSuffix(path, "/searchAnalytics/query") || strings.HasSuffix(path, "/urlInspection/index:inspect")
	case "sheets.googleapis.com", "sheets.mtls.googleapis.com":
		return strings.HasSuffix(path, ":batchGetByDataFilter") || strings.HasSuffix(path, ":getByDataFilter")
	case "driveactivity.googleapis.com", "driveactivity.mtls.googleapis.com":
		return strings.HasSuffix(path, "/v2/activity:query")
	case "analyticsdata.googleapis.com", "analyticsdata.mtls.googleapis.com":
		return strings.HasSuffix(path, ":runReport") || strings.HasSuffix(path, ":batchRunReports") || strings.HasSuffix(path, ":runPivotReport") || strings.HasSuffix(path, ":runRealtimeReport")
	default:
		return false
	}
}
