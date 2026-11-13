package googleapi

import (
	"context"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

func TestNativeHTTPClientStalledWriteIsNotReplayed(t *testing.T) {
	t.Parallel()

	const requestTimeout = 2 * time.Second

	fx := newNativeFixture(t, 3600)

	var hits atomic.Int32

	release := make(chan struct{})

	t.Cleanup(func() { close(release) })
	fx.apiHandler.set(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))

	provider, err := NewNativeProvider(NativeOptions{
		Registry:       fx.registry,
		Tokens:         fx.tokens,
		Credentials:    func(clientName string) (string, string, error) { return "id-" + clientName, "secret", nil },
		Endpoint:       oauth2.Endpoint{TokenURL: fx.token.URL, AuthURL: fx.token.URL + "/auth"},
		Lifecycle:      fx.life,
		RequestTimeout: requestTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}

	id := fx.putAccount(t, "acct-write", "sub-write", "write@example.com", "personal", []string{mcpcontract.DocsWriteScope}, "rt-write")

	client, err := provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "docs_create_document", Retry: mcpcontract.NonReplayableWrite})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ctx, counter := WithUpstreamCounter(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fx.api.URL+"/v1/documents", strings.NewReader(`{"title":"stalled"}`))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	// Write operations map errors after Do with NativeWritePublicError.
	public := NativeWritePublicError(err)
	if public == nil || public.Category != mcpcontract.OutcomeUnknown || public.Retryable {
		t.Fatalf("error = %#v (%v)", public, err)
	}

	if hits.Load() != 1 || counter.Load() != 1 {
		t.Fatalf("server saw %d writes, upstream_calls = %d, want exactly one attempt", hits.Load(), counter.Load())
	}
}

const (
	h2ReportPath     = "/v1beta/properties/375683505:runReport"
	h2ReportRequest  = `{"dateRanges":[{"startDate":"2026-10-27","endDate":"2026-11-12"}],"dimensions":[{"name":"date"}],"metrics":[{"name":"eventCount"}]}`
	h2ReportResponse = `{"dimensionHeaders":[{"name":"date"}],"metricHeaders":[{"name":"eventCount","type":"TYPE_INTEGER"}],"rows":[{"dimensionValues":[{"value":"20261027"}],"metricValues":[{"value":"431"}]}],"rowCount":1}`
)

// The #906 fault: Google leaves a runReport stream unanswered on one pooled
// HTTP/2 connection while that connection keeps serving other streams. The
// replay must go out on another connection and must not disturb those streams.
func TestNativeHTTPClientReplaysStalledHTTP2ReadOnAnotherConnection(t *testing.T) {
	t.Parallel()

	const requestTimeout = 3 * time.Second

	var (
		mu          sync.Mutex
		busyConn    string
		reportConns []string
		protocols   []string
	)

	busyDone := make(chan struct{})
	release := make(chan struct{})

	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()

		protocols = append(protocols, r.Proto)
		mu.Unlock()

		switch r.URL.Path {
		case "/busy":
			mu.Lock()
			busyConn = r.RemoteAddr
			mu.Unlock()

			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()

			select {
			case <-busyDone:
				_, _ = io.WriteString(w, "busy stream finished")
			case <-r.Context().Done():
			}
		case h2ReportPath:
			_, _ = io.ReadAll(r.Body)

			mu.Lock()

			reportConns = append(reportConns, r.RemoteAddr)
			stalledConn := reportConns[0]
			mu.Unlock()

			if r.RemoteAddr == stalledConn {
				select {
				case <-release:
				case <-r.Context().Done():
				}

				return
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, h2ReportResponse)
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	closedConns := make(chan string, 8)
	upstream.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closedConns <- conn.RemoteAddr().String()
		}
	}
	upstream.EnableHTTP2 = true
	upstream.StartTLS()
	t.Cleanup(upstream.Close)
	t.Cleanup(func() { close(release) })

	fx := newNativeFixture(t, 3600)

	provider, err := NewNativeProvider(NativeOptions{
		Registry:       fx.registry,
		Tokens:         fx.tokens,
		Credentials:    func(clientName string) (string, string, error) { return "id-" + clientName, "secret", nil },
		Endpoint:       oauth2.Endpoint{TokenURL: fx.token.URL, AuthURL: fx.token.URL + "/auth"},
		Lifecycle:      fx.life,
		RequestTimeout: requestTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The production pooled transport, trusting the test server's certificate.
	roots := x509.NewCertPool()
	roots.AddCert(upstream.Certificate())

	pooled := cloneNativeTransport()
	pooled.TLSClientConfig.RootCAs = roots
	t.Cleanup(pooled.CloseIdleConnections)

	provider.pool = &nativeBodyLimitTransport{base: pooled, max: provider.maxBody}

	busyCtx, cancelBusy := context.WithCancel(context.Background())
	t.Cleanup(cancelBusy)

	busyReq, err := http.NewRequestWithContext(busyCtx, http.MethodGet, upstream.URL+"/busy", nil)
	if err != nil {
		t.Fatal(err)
	}

	busy, err := provider.pool.RoundTrip(busyReq)
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Body.Close()

	id := fx.putAccount(t, "acct-ga4", "sub-ga4", "ga4@example.com", "personal", []string{mcpcontract.AnalyticsReadScope}, "rt-ga4")

	client, err := provider.HTTPClient(context.Background(), id, mcpcontract.CallOptions{Operation: "analytics_report", Retry: mcpcontract.SafeRead})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ctx, counter := WithUpstreamCounter(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream.URL+h2ReportPath, strings.NewReader(h2ReportRequest))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		mu.Lock()
		defer mu.Unlock()

		t.Fatalf("runReport error: %v (%+v); runReport connections = %v, busy connection = %s", err, NativePublicError(err), reportConns, busyConn)
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if err != nil || resp.StatusCode != http.StatusOK || resp.ProtoMajor != 2 || string(body) != h2ReportResponse {
		t.Fatalf("runReport = %s %d %q (%v)", resp.Proto, resp.StatusCode, body, err)
	}

	if counter.Load() != 2 {
		t.Fatalf("upstream_calls = %d, want 2", counter.Load())
	}

	close(busyDone)

	busyBody, err := io.ReadAll(busy.Body)
	if err != nil || string(busyBody) != "busy stream finished" {
		t.Fatalf("busy stream on the shared connection = %q (%v), want it to finish undisturbed", busyBody, err)
	}

	mu.Lock()
	defer mu.Unlock()

	for _, proto := range protocols {
		if proto != "HTTP/2.0" {
			t.Fatalf("server saw %v, want HTTP/2 on every request", protocols)
		}
	}

	if len(reportConns) != 2 || reportConns[0] != busyConn || reportConns[1] == reportConns[0] {
		t.Fatalf("runReport connections = %v, busy connection = %s; want the first on the busy connection and the replay on another", reportConns, busyConn)
	}

	// The replay's connection is not pooled: it closes once its body is closed.
	timeout := time.After(2 * time.Second)

	for {
		select {
		case closed := <-closedConns:
			if closed == reportConns[1] {
				return
			}
		case <-timeout:
			t.Fatalf("replay connection %s still open after its response body was closed", reportConns[1])
		}
	}
}

func TestCloneNativeTransportHealthChecksHTTP2Connections(t *testing.T) {
	t.Parallel()

	if got := cloneNativeTransport().HTTP2; got == nil || got.SendPingTimeout != 10*time.Second || got.PingTimeout != 5*time.Second {
		t.Fatalf("HTTP2 config = %+v, want PING after 10s idle with a 5s timeout", got)
	}
}

func TestNativeRetryTransportAttemptBoundTracksTimeLeft(t *testing.T) {
	t.Parallel()

	start := time.Now()

	tests := []struct {
		name           string
		attemptTimeout time.Duration
		timeLeft       time.Duration // zero means no request deadline
		budget         int64         // zero means no budget; otherwise one call is already charged
		stalls         int
		want           time.Duration
	}{
		{name: "full deadline", attemptTimeout: 12 * time.Second, timeLeft: 30 * time.Second, want: 12 * time.Second},
		{name: "late start leaves room for a replay", attemptTimeout: 12 * time.Second, timeLeft: 20 * time.Second, want: 8 * time.Second},
		{name: "late start at the floor", attemptTimeout: 12 * time.Second, timeLeft: 15 * time.Second, want: 6 * time.Second},
		{name: "too little time for a replay", attemptTimeout: 12 * time.Second, timeLeft: 14 * time.Second, want: 0},
		{name: "no request deadline", attemptTimeout: 12 * time.Second, want: 12 * time.Second},
		{name: "budget of one", attemptTimeout: 12 * time.Second, timeLeft: 30 * time.Second, budget: 1, want: 0},
		{name: "budget left for a replay", attemptTimeout: 12 * time.Second, timeLeft: 30 * time.Second, budget: 2, want: 12 * time.Second},
		{name: "attempt timeout zero", timeLeft: 30 * time.Second, want: 0},
		{name: "already one stall", attemptTimeout: 12 * time.Second, timeLeft: 30 * time.Second, stalls: 1, want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			if tc.timeLeft > 0 {
				var cancel context.CancelFunc

				ctx, cancel = context.WithDeadline(ctx, start.Add(tc.timeLeft))
				defer cancel()
			}

			if tc.budget > 0 {
				ctx = WithUpstreamBudget(ctx, tc.budget)
				if err := consumeUpstreamBudget(ctx); err != nil {
					t.Fatal(err)
				}
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://analyticsdata.googleapis.com"+h2ReportPath, nil)
			if err != nil {
				t.Fatal(err)
			}

			transport := &NativeRetryTransport{Class: mcpcontract.SafeRead, AttemptTimeout: tc.attemptTimeout, now: func() time.Time { return start }}
			if got := transport.attemptBound(req, tc.stalls); got != tc.want {
				t.Fatalf("attemptBound = %s, want %s", got, tc.want)
			}
		})
	}
}
