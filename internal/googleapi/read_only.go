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

type (
	readOnlyContextKey       struct{}
	readOnlyGrantsContextKey struct{}
)

// WriteGrant identifies a reviewed readonly-mode exception. Empty fields widen
// only the corresponding scope; callers should use the narrowest known scope.
type WriteGrant struct {
	Service   string
	Operation string
	Target    string
}

// WithReadOnly enables fail-closed outbound mutation protection for ctx.
func WithReadOnly(ctx context.Context, enabled bool) context.Context {
	if !enabled {
		return ctx
	}

	return context.WithValue(ctx, readOnlyContextKey{}, true)
}

// WithReadOnlyWriteGrants appends invocation/process lifetime grants to ctx.
func WithReadOnlyWriteGrants(ctx context.Context, grants ...WriteGrant) context.Context {
	existing, _ := ctx.Value(readOnlyGrantsContextKey{}).([]WriteGrant)
	copyOf := append(append([]WriteGrant{}, existing...), grants...)

	return context.WithValue(ctx, readOnlyGrantsContextKey{}, copyOf)
}

// ReadOnly reports whether runtime read-only protection is enabled.
func ReadOnly(ctx context.Context) bool {
	enabled, _ := ctx.Value(readOnlyContextKey{}).(bool)

	return enabled
}

type readOnlyTransport struct {
	base   http.RoundTripper
	grants []WriteGrant
}

func readOnlyTransportFromContext(ctx context.Context, base http.RoundTripper) http.RoundTripper {
	if !ReadOnly(ctx) {
		return base
	}

	if base == nil {
		base = http.DefaultTransport
	}

	grants, _ := ctx.Value(readOnlyGrantsContextKey{}).([]WriteGrant)

	return readOnlyTransport{base: base, grants: grants}
}

func (t readOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !ReadOnlyRequestAllowed(req) && !ReadOnlyWriteGrantsAllow(t.grants, req) {
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

// ReadOnlyWriteGrantsAllow permits only mutations on a service's exact owned
// hosts. Target scopes additionally require the target to be present in URL.
func ReadOnlyWriteGrantsAllow(grants []WriteGrant, req *http.Request) bool {
	if req == nil || req.URL == nil || req.URL.Scheme != "https" || req.Method == http.MethodGet || req.Method == http.MethodHead || req.Method == http.MethodOptions {
		return false
	}

	if strings.TrimSpace(req.Header.Get("X-HTTP-Method-Override")) != "" {
		return false
	}

	host := strings.ToLower(strings.TrimSuffix(req.URL.Hostname(), "."))
	for _, grant := range grants {
		if !serviceOwnsHost(grant.Service, host) {
			continue
		}

		if (host == "www.googleapis.com" || host == "www.mtls.googleapis.com") && !sharedHostPathMatchesService(grant.Service, req.URL.Path) {
			continue
		}

		if grant.Target != "" && !strings.Contains(req.URL.Path, grant.Target) {
			continue
		}

		if !operationMatchesRequest(grant.Operation, req) {
			continue
		}

		return true
	}

	return false
}

func operationMatchesRequest(operation string, req *http.Request) bool {
	if operation == "" {
		return true
	}

	last := operation
	if index := strings.LastIndex(last, "."); index >= 0 {
		last = last[index+1:]
	}

	if last == "send" {
		return req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/send")
	}

	if last == "delete" || last == "remove" || last == "unshare" {
		return req.Method == http.MethodDelete
	}

	if last == "update" || last == "patch" || last == "modify" || last == "rename" || last == "respond" || last == "write" || last == "format" || last == "clear" {
		return req.Method == http.MethodPatch || req.Method == http.MethodPut || req.Method == http.MethodPost
	}

	return true
}

func sharedHostPathMatchesService(service, path string) bool {
	prefixes := map[string][]string{
		"calendar": {"/calendar/"},
		"drive":    {"/drive/", "/upload/drive/"},
		"gmail":    {"/gmail/"},
		"sheets":   {"/v4/spreadsheets/"},
		"tasks":    {"/tasks/"},
		"youtube":  {"/youtube/"},
	}
	for _, prefix := range prefixes[service] {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

func serviceOwnsHost(service, host string) bool {
	service = strings.ToLower(strings.TrimSpace(service))
	for _, owned := range serviceHosts[service] {
		if host == owned || host == strings.Replace(owned, ".googleapis.com", ".mtls.googleapis.com", 1) {
			return true
		}
	}

	return false
}

var serviceHosts = map[string][]string{
	"analytics":       {"analyticsadmin.googleapis.com", "analyticsdata.googleapis.com"},
	"bigquery":        {"bigquery.googleapis.com"},
	"businessprofile": {"mybusinessaccountmanagement.googleapis.com", "mybusinessbusinessinformation.googleapis.com", "mybusiness.googleapis.com"},
	"calendar":        {"www.googleapis.com", "calendar-json.googleapis.com"},
	"chat":            {"chat.googleapis.com"},
	"classroom":       {"classroom.googleapis.com"},
	"contacts":        {"people.googleapis.com"},
	"docs":            {"docs.googleapis.com", "www.googleapis.com"},
	"drive":           {"www.googleapis.com", "drive.googleapis.com"},
	"gmail":           {"gmail.googleapis.com", "www.googleapis.com"},
	"groups":          {"admin.googleapis.com", "www.googleapis.com"},
	"keep":            {"keep.googleapis.com"},
	"people":          {"people.googleapis.com"},
	"searchconsole":   {"searchconsole.googleapis.com", "www.googleapis.com"},
	"sheets":          {"sheets.googleapis.com", "www.googleapis.com"},
	"slides":          {"slides.googleapis.com", "www.googleapis.com"},
	"tagmanager":      {"tagmanager.googleapis.com"},
	"tasks":           {"tasks.googleapis.com", "www.googleapis.com"},
	"youtube":         {"youtube.googleapis.com", "www.googleapis.com"},
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
		return strings.HasSuffix(path, "/searchAnalytics/query") || strings.HasSuffix(path, "/urlInspection/index:inspect") || strings.HasSuffix(path, "/mobileFriendlyTest:run")
	case "sheets.googleapis.com", "sheets.mtls.googleapis.com":
		return strings.HasSuffix(path, ":batchGetByDataFilter") || strings.HasSuffix(path, ":getByDataFilter") || strings.HasSuffix(path, ":search")
	case "driveactivity.googleapis.com", "driveactivity.mtls.googleapis.com":
		return strings.HasSuffix(path, "/v2/activity:query")
	case "analyticsdata.googleapis.com", "analyticsdata.mtls.googleapis.com":
		return strings.HasSuffix(path, ":runReport") || strings.HasSuffix(path, ":batchRunReports") || strings.HasSuffix(path, ":runPivotReport") || strings.HasSuffix(path, ":batchRunPivotReports") || strings.HasSuffix(path, ":runRealtimeReport")
	case "bigquery.googleapis.com", "bigquery.mtls.googleapis.com":
		return strings.HasPrefix(path, "/bigquery/v2/projects/") && strings.HasSuffix(path, "/queries")
	case "mybusinessbusinessinformation.googleapis.com", "mybusinessbusinessinformation.mtls.googleapis.com":
		return strings.HasSuffix(path, ":search")
	default:
		return false
	}
}
