package analytics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/accountconnect"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/mcpcontract"
)

// stallRequestTimeout is the per-tool deadline in these tests. The native
// provider derives a 1.2s per-attempt header wait from it.
const stallRequestTimeout = 3 * time.Second

const stallReportArguments = `{
	"account_id":"ga4-account", "property":"properties/375683505", "metrics":["eventCount"],
	"dimensions":["date"], "start_date":"2026-10-27", "end_date":"2026-11-12"
}`

const stallReportResponse = `{
	"dimensionHeaders": [{"name": "date"}],
	"metricHeaders": [{"name": "eventCount", "type": "TYPE_INTEGER"}],
	"metadata": {"timeZone": "America/Chicago"},
	"rowCount": 2,
	"rows": [
		{"dimensionValues": [{"value": "20261027"}], "metricValues": [{"value": "431"}]},
		{"dimensionValues": [{"value": "20261028"}], "metricValues": [{"value": "512"}]}
	]
}`

var stallReportRows = []ReportRow{
	{Dimensions: []string{"20261027"}, Metrics: []string{"431"}},
	{Dimensions: []string{"20261028"}, Metrics: []string{"512"}},
}

// ga4Upstream is a fake GA4 Data API. respond handles the nth runReport
// (1-based); a handler that never writes models Google leaving a stream
// unanswered until the client gives up.
type ga4Upstream struct {
	mu      sync.Mutex
	bodies  []string
	release chan struct{}
	respond func(n int, w http.ResponseWriter, r *http.Request, release <-chan struct{})
}

func (u *ga4Upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	u.mu.Lock()
	u.bodies = append(u.bodies, string(body))
	n := len(u.bodies)
	u.mu.Unlock()

	if r.Method != http.MethodPost || r.URL.Path != "/v1beta/properties/375683505:runReport" {
		http.Error(w, "unexpected GA4 request", http.StatusBadRequest)

		return
	}

	u.respond(n, w, r, u.release)
}

func (u *ga4Upstream) requestBodies() []string {
	u.mu.Lock()
	defer u.mu.Unlock()

	return append([]string(nil), u.bodies...)
}

func stall(_ int, _ http.ResponseWriter, r *http.Request, release <-chan struct{}) {
	select {
	case <-r.Context().Done():
	case <-release:
	}
}

func answer(_ int, w http.ResponseWriter, _ *http.Request, _ <-chan struct{}) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, stallReportResponse)
}

type stallRun struct {
	result        mcpcontract.Result[ReportData]
	err           error
	elapsed       time.Duration
	upstreamCalls int64
}

// runStalledReport drives analytics_report through the real native provider
// stack (OAuth, concurrency slots, retry transport, pooled transport) against
// a fake GA4 server, under the same per-tool deadline the MCP runtime applies.
func runStalledReport(t *testing.T, upstream *ga4Upstream) stallRun {
	t.Helper()

	upstream.release = make(chan struct{})
	api := httptest.NewServer(upstream)
	t.Cleanup(api.Close)
	t.Cleanup(func() { close(upstream.release) })

	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"at-ga4","token_type":"Bearer","expires_in":3600}`)
	}))
	t.Cleanup(token.Close)

	registry := accountconnect.NewMemoryRegistry()
	tokens := accountconnect.NewMemoryTokenStore()
	record := accountconnect.Record{
		AccountID: "ga4-account", Subject: "subject-ga4", Email: "ga4@example.test", Label: "GA4",
		PrincipalID: "principal", ClientName: "client", AuthMode: accountconnect.AuthModeOAuth,
		Scopes: []string{mcpcontract.AnalyticsReadScope}, Generation: 1, UpdatedAt: time.Now().UTC(),
	}

	if err := registry.Upsert(context.Background(), record); err != nil {
		t.Fatal(err)
	}

	if err := tokens.Put(context.Background(), record.ClientName, record.Email, "rt-ga4", record.Scopes); err != nil {
		t.Fatal(err)
	}

	native, err := googleapi.NewNativeProvider(googleapi.NativeOptions{
		Registry:       registry,
		Tokens:         tokens,
		Credentials:    func(string) (string, string, error) { return "client-id", "client-secret", nil },
		Endpoint:       oauth2.Endpoint{TokenURL: token.URL, AuthURL: token.URL + "/auth"},
		Lifecycle:      accountconnect.NewLifecycle(),
		RequestTimeout: stallRequestTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}

	endpoint, err := url.Parse(api.URL)
	if err != nil {
		t.Fatal(err)
	}

	var operation mcpcontract.Operation

	for _, candidate := range Operations(&redirectProvider{native: native, endpoint: endpoint}) {
		if candidate.Definition.Name == "analytics_report" {
			operation = candidate
		}
	}

	call, err := operation.Decode(json.RawMessage(stallReportArguments))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), stallRequestTimeout)
	defer cancel()

	ctx, counter := googleapi.WithUpstreamCounter(ctx)
	start := time.Now()
	result, err := call.Run(ctx, record.Identity())
	run := stallRun{err: err, elapsed: time.Since(start), upstreamCalls: counter.Load()}

	if err == nil {
		run.result = result.(mcpcontract.Result[ReportData])
	}

	return run
}

// redirectProvider points the native client at the fake GA4 server. It wraps
// the complete native transport, so every native layer still runs.
type redirectProvider struct {
	native   *googleapi.NativeProvider
	endpoint *url.URL
}

func (p *redirectProvider) HTTPClient(ctx context.Context, id mcpcontract.Identity, options mcpcontract.CallOptions) (*http.Client, error) {
	client, err := p.native.HTTPClient(ctx, id, options)
	if err != nil {
		return nil, err //nolint:wrapcheck // test seam passes provider errors through
	}

	redirected := *client
	redirected.Transport = &redirectTransport{endpoint: p.endpoint, base: client.Transport}

	return &redirected, nil
}

type redirectTransport struct {
	endpoint *url.URL
	base     http.RoundTripper
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.endpoint.Scheme
	clone.URL.Host = t.endpoint.Host
	clone.Host = t.endpoint.Host

	return t.base.RoundTrip(clone) //nolint:wrapcheck // test seam passes transport errors through
}

func TestAnalyticsReportReplaysStalledRunReport(t *testing.T) {
	t.Parallel()

	upstream := &ga4Upstream{respond: func(n int, w http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		if n == 1 {
			stall(n, w, r, release)

			return
		}

		answer(n, w, r, release)
	}}

	run := runStalledReport(t, upstream)
	if run.err != nil {
		t.Fatalf("analytics_report error after %s: %v (%+v)", run.elapsed, run.err, googleapi.NativePublicError(run.err))
	}

	if !reflect.DeepEqual(run.result.Data.Rows, stallReportRows) {
		t.Fatalf("rows = %#v", run.result.Data.Rows)
	}

	if run.elapsed >= stallRequestTimeout {
		t.Fatalf("elapsed %s, want under the %s request deadline", run.elapsed, stallRequestTimeout)
	}

	if run.upstreamCalls != 2 {
		t.Fatalf("upstream_calls = %d, want 2", run.upstreamCalls)
	}

	bodies := upstream.requestBodies()
	if len(bodies) != 2 || bodies[0] == "" || bodies[1] != bodies[0] {
		t.Fatalf("runReport bodies = %q, want the first body replayed once", bodies)
	}
}

func TestAnalyticsReportEveryAttemptStalledIsDeadlineExceeded(t *testing.T) {
	t.Parallel()

	upstream := &ga4Upstream{respond: stall}

	run := runStalledReport(t, upstream)

	public := googleapi.NativePublicError(run.err)
	if public == nil || public.Category != mcpcontract.DeadlineExceeded || !public.Retryable {
		t.Fatalf("error = %#v (%v)", public, run.err)
	}

	if limit := stallRequestTimeout + 500*time.Millisecond; run.elapsed > limit {
		t.Fatalf("elapsed %s, want at most %s", run.elapsed, limit)
	}

	if run.upstreamCalls != 2 || len(upstream.requestBodies()) != 2 {
		t.Fatalf("upstream_calls = %d, server saw %d, want one replay", run.upstreamCalls, len(upstream.requestBodies()))
	}
}

func TestAnalyticsReportSlowHeadersAndBodyStillSucceed(t *testing.T) {
	t.Parallel()

	// Headers arrive 500ms before the 1.2s attempt bound, then the body takes
	// another second, finishing past the attempt bound but inside the deadline.
	upstream := &ga4Upstream{respond: func(_ int, w http.ResponseWriter, r *http.Request, _ <-chan struct{}) {
		flusher := w.(http.Flusher)

		time.Sleep(700 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		chunk := (len(stallReportResponse) + 3) / 4
		for start := 0; start < len(stallReportResponse); start += chunk {
			time.Sleep(250 * time.Millisecond)

			if r.Context().Err() != nil {
				return
			}

			_, _ = io.WriteString(w, stallReportResponse[start:min(start+chunk, len(stallReportResponse))])

			flusher.Flush()
		}
	}}

	run := runStalledReport(t, upstream)
	if run.err != nil {
		t.Fatalf("analytics_report error after %s: %v", run.elapsed, run.err)
	}

	if !reflect.DeepEqual(run.result.Data.Rows, stallReportRows) || run.upstreamCalls != 1 {
		t.Fatalf("rows = %#v upstream_calls = %d", run.result.Data.Rows, run.upstreamCalls)
	}
}
