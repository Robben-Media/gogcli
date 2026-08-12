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

	response, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("send allowed request: %w", err)
	}

	return response, nil
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
	path := req.URL.EscapedPath()

	switch host {
	case "www.googleapis.com", "www.mtls.googleapis.com", "calendar-json.googleapis.com", "calendar-json.mtls.googleapis.com":
		return path == "/calendar/v3/freeBusy"
	case "searchconsole.googleapis.com", "searchconsole.mtls.googleapis.com":
		return pathMatches(path, "/webmasters/v3/sites/{site}/searchAnalytics/query") ||
			path == "/v1/urlInspection/index:inspect" ||
			path == "/v1/urlTestingTools/mobileFriendlyTest:run"
	case "sheets.googleapis.com", "sheets.mtls.googleapis.com":
		return pathMatches(path, "/v4/spreadsheets/{spreadsheet}:getByDataFilter") ||
			pathMatches(path, "/v4/spreadsheets/{spreadsheet}/developerMetadata:search") ||
			pathMatches(path, "/v4/spreadsheets/{spreadsheet}/values:batchGetByDataFilter")
	case "driveactivity.googleapis.com", "driveactivity.mtls.googleapis.com":
		return path == "/v2/activity:query"
	case "analyticsdata.googleapis.com", "analyticsdata.mtls.googleapis.com":
		return pathMatches(path, "/v1beta/properties/{property}:runReport") ||
			pathMatches(path, "/v1beta/properties/{property}:batchRunReports") ||
			pathMatches(path, "/v1beta/properties/{property}:runPivotReport") ||
			pathMatches(path, "/v1beta/properties/{property}:batchRunPivotReports") ||
			pathMatches(path, "/v1beta/properties/{property}:runRealtimeReport") ||
			pathMatches(path, "/v1beta/properties/{property}:checkCompatibility") ||
			pathMatches(path, "/v1beta/properties/{property}/audienceExports/{export}:query")
	case "mybusinessbusinessinformation.googleapis.com", "mybusinessbusinessinformation.mtls.googleapis.com":
		return path == "/v1/chains:search" || path == "/v1/googleLocations:search"
	default:
		return false
	}
}

// pathMatches compares a URL path to a reviewed route template. Placeholders
// match one nonempty path segment; literals (including action suffixes) must match exactly.
func pathMatches(path, template string) bool {
	pathParts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	templateParts := strings.Split(strings.TrimPrefix(template, "/"), "/")
	if len(pathParts) != len(templateParts) {
		return false
	}

	for i, want := range templateParts {
		got := pathParts[i]
		if got == "" {
			return false
		}

		for {
			start := strings.Index(want, "{")
			if start < 0 {
				break
			}

			end := strings.Index(want[start:], "}")
			if end < 0 {
				return false
			}

			end += start

			prefix, suffix := want[:start], want[end+1:]
			if !strings.HasPrefix(got, prefix) || !strings.HasSuffix(got, suffix) || len(got) <= len(prefix)+len(suffix) {
				return false
			}

			want = prefix + suffix
		}

		if got != want && !strings.Contains(templateParts[i], "{") {
			return false
		}
	}

	return true
}
