package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	nativegmail "github.com/steipete/gogcli/internal/googleops/gmail"
	"github.com/steipete/gogcli/internal/mcpcontract"
	"github.com/steipete/gogcli/internal/outfmt"
)

// This fixture benchmark compares handler work only. It excludes process startup,
// OAuth, MCP transport, agent reasoning, discovery and Google/network latency.
type nativeBenchmarkTransport struct{ calls atomic.Int64 }

func (r *nativeBenchmarkTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls.Add(1)
	var body string
	switch {
	case strings.HasSuffix(req.URL.Path, "/messages"):
		body = `{"messages":[{"id":"message-first","threadId":"thread-acme"},{"id":"message-second","threadId":"thread-acme"}]}`
	case strings.HasSuffix(req.URL.Path, "/labels"):
		body = `{"labels":[{"id":"INBOX","name":"INBOX"}]}`
	case strings.HasSuffix(req.URL.Path, "/message-first"):
		body = `{"id":"message-first","threadId":"thread-acme","internalDate":"1000","labelIds":["INBOX"],"payload":{"headers":[{"name":"Subject","value":"Review is Friday"},{"name":"From","value":"client@example.test"}]}}`
	default:
		body = `{"id":"message-second","threadId":"thread-acme","internalDate":"2000","labelIds":["INBOX"],"payload":{"headers":[{"name":"Subject","value":"Budget is 2500 USD"},{"name":"From","value":"client@example.test"}]}}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

type nativeBenchmarkProvider struct{ client *http.Client }

func (p nativeBenchmarkProvider) HTTPClient(context.Context, mcpcontract.Identity, mcpcontract.CallOptions) (*http.Client, error) {
	return p.client, nil
}

func BenchmarkNativeGmailHandlerComparison(b *testing.B) {
	transport := &nativeBenchmarkTransport{}
	client := &http.Client{Transport: transport}
	id := mcpcontract.Identity{AccountID: "work-fixture", Email: "work@example.test", Label: "Work", AuthMode: "oauth", Scopes: []string{mcpcontract.GmailReadScope}}
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
	svc, err := gmailapi.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		b.Fatal(err)
	}
	origService := newGmailService
	newGmailService = func(context.Context, string) (*gmailapi.Service, error) { return svc, nil }
	b.Cleanup(func() { newGmailService = origService })
	sink, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = sink
	b.Cleanup(func() { os.Stdout = origStdout; _ = sink.Close() })
	op := nativegmail.Operations(nativeBenchmarkProvider{client: client})[0]
	raw := json.RawMessage(`{"account_id":"work-fixture","query":"Acme","max_results":25}`)
	call, err := op.Decode(raw)
	if err != nil {
		b.Fatal(err)
	}
	result, err := call.Run(ctx, id)
	if err != nil {
		b.Fatal(err)
	}
	checked, ok := result.(mcpcontract.Result[nativegmail.SearchData])
	if !ok || len(checked.Data.Messages) != 2 || checked.Data.Messages[0].Subject != "Review is Friday" || checked.Data.Messages[1].Subject != "Budget is 2500 USD" {
		b.Fatal("native fixture facts differ")
	}
	for _, route := range []string{"CLI", "Native"} {
		b.Run(route, func(b *testing.B) {
			transport.calls.Store(0)
			durations := make([]int64, 0, b.N)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				start := time.Now()
				if route == "CLI" {
					command := GmailMessagesSearchCmd{Query: []string{"Acme"}, Max: 25, Timezone: "UTC"}
					if err := command.Run(ctx, &RootFlags{Account: id.Email}); err != nil {
						b.Fatal(err)
					}
				} else {
					decoded, err := op.Decode(raw)
					if err != nil {
						b.Fatal(err)
					}
					value, err := decoded.Run(ctx, id)
					if err != nil {
						b.Fatal(err)
					}
					if err := json.NewEncoder(io.Discard).Encode(value); err != nil {
						b.Fatal(err)
					}
				}
				durations = append(durations, time.Since(start).Nanoseconds())
			}
			b.StopTimer()
			sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
			if b.N > 0 {
				b.ReportMetric(float64(transport.calls.Load())/float64(b.N), "upstream/op")
				b.ReportMetric(float64(durations[len(durations)/2]), "p50-ns")
				b.ReportMetric(float64(durations[(len(durations)-1)*95/100]), "p95-ns")
			}
		})
	}
}
