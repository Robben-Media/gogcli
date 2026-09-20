package googleapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

var errConnectionResetAfterSend = errors.New("connection reset after send")

type scriptedTransport struct {
	responses []*http.Response
	errors    []error
	calls     int
	bodies    []string
}

func (s *scriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body string

	if req != nil && req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		body = string(raw)
	}

	s.bodies = append(s.bodies, body)

	idx := s.calls
	s.calls++

	if idx < len(s.errors) && s.errors[idx] != nil {
		return nil, s.errors[idx]
	}

	if idx < len(s.responses) {
		return s.responses[idx], nil
	}

	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
}

func TestNativeRetryTransportSafeReadRetries4295xxAndRateLimit403(t *testing.T) {
	script := &scriptedTransport{
		responses: []*http.Response{
			{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("slow")), Header: http.Header{"Retry-After": []string{"0"}}},
			{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(`{"error":{"errors":[{"reason":"rateLimitExceeded"}]}}`))},
			{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("boom"))},
			{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}
	rt := &NativeRetryTransport{Base: script, Class: mcpcontract.SafeRead, MaxRetries429: 3, MaxRetries5xx: 1, BaseDelay: time.Millisecond}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.invalid/read", nil)

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	if script.calls != 4 {
		t.Fatalf("calls = %d, want 4", script.calls)
	}
}

func TestNativeRetryTransportSafeReadPOSTReplaysBody(t *testing.T) {
	script := &scriptedTransport{
		responses: []*http.Response{
			{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("slow")), Header: http.Header{"Retry-After": []string{"0"}}},
			{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}
	rt := &NativeRetryTransport{Base: script, Class: mcpcontract.SafeRead, MaxRetries429: 3, BaseDelay: time.Millisecond}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.invalid/report", bytes.NewReader([]byte(`{"start":"1"}`)))

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if script.calls != 2 {
		t.Fatalf("calls = %d", script.calls)
	}

	if script.bodies[0] != `{"start":"1"}` || script.bodies[1] != `{"start":"1"}` {
		t.Fatalf("bodies = %#v", script.bodies)
	}
}

func TestNativeRetryTransportSafeReadDoesNotRetryForbidden(t *testing.T) {
	script := &scriptedTransport{
		responses: []*http.Response{
			{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(`{"error":{"errors":[{"reason":"insufficientPermissions"}]}}`))},
		},
	}
	rt := &NativeRetryTransport{Base: script, Class: mcpcontract.SafeRead, MaxRetries429: 3, BaseDelay: time.Millisecond}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.invalid/read", nil)

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	if script.calls != 1 {
		t.Fatalf("calls = %d", script.calls)
	}
}

func TestNativeRetryTransportObeysDeadlineRetryAfter(t *testing.T) {
	script := &scriptedTransport{
		responses: []*http.Response{
			{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("slow")), Header: http.Header{"Retry-After": []string{"30"}}},
		},
	}
	rt := &NativeRetryTransport{Base: script, Class: mcpcontract.SafeRead, MaxRetries429: 3, BaseDelay: time.Second, now: func() time.Time { return time.Unix(0, 0) }}

	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid/read", nil)

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	if script.calls != 1 {
		t.Fatalf("calls = %d, want no retry past deadline", script.calls)
	}
}

func TestNativeRetryTransportNonReplayableWriteOutcomeUnknown(t *testing.T) {
	script := &scriptedTransport{errors: []error{errConnectionResetAfterSend}}
	rt := &NativeRetryTransport{Base: script, Class: mcpcontract.NonReplayableWrite}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.invalid/send", strings.NewReader("payload"))

	resp, err := rt.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) {
		t.Fatalf("err = %v", err)
	}

	if safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable {
		t.Fatalf("error = %#v", safe)
	}

	if script.calls != 1 {
		t.Fatalf("write retried: %d", script.calls)
	}

	if strings.Contains(safe.Error(), "example.invalid") || strings.Contains(safe.Error(), "payload") {
		t.Fatalf("leaked details: %s", safe.Error())
	}
}

func TestNativeRetryTransportUnknownClassOutcomeUnknown(t *testing.T) {
	script := &scriptedTransport{errors: []error{io.ErrUnexpectedEOF}}
	rt := &NativeRetryTransport{Base: script, Class: ""}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.invalid/maybe-write", nil)

	resp, err := rt.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable {
		t.Fatalf("err = %v", err)
	}

	if script.calls != 1 {
		t.Fatalf("unknown class retried: %d", script.calls)
	}
}

func TestNativeRetryTransportWriteDoesNotRetry429(t *testing.T) {
	script := &scriptedTransport{
		responses: []*http.Response{
			{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("slow"))},
		},
	}
	rt := &NativeRetryTransport{Base: script, Class: mcpcontract.ProviderKeyedWrite, MaxRetries429: 3, BaseDelay: time.Millisecond}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.invalid/write", strings.NewReader("x"))

	resp, err := rt.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	var safe *mcpcontract.Error
	if !errors.As(err, &safe) || safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable {
		t.Fatalf("err=%v", err)
	}

	if script.calls != 1 {
		t.Fatalf("calls=%d", script.calls)
	}
}

func TestNativeRetryTransportCountsUpstreamAttempts(t *testing.T) {
	script := &scriptedTransport{
		responses: []*http.Response{
			{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("slow")), Header: http.Header{"Retry-After": []string{"0"}}},
			{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}
	rt := &NativeRetryTransport{Base: script, Class: mcpcontract.SafeRead, MaxRetries429: 3, BaseDelay: time.Millisecond}
	ctx, counter := WithUpstreamCounter(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid/read", nil)

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if counter.Load() != 2 || UpstreamCalls(ctx) != 2 {
		t.Fatalf("calls=%d counter=%d", script.calls, counter.Load())
	}
}

func TestNativeRetryTransportNonReplayableAmbiguousHTTP(t *testing.T) {
	for _, code := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError} {
		script := &scriptedTransport{
			responses: []*http.Response{
				{StatusCode: code, Body: io.NopCloser(strings.NewReader("maybe"))},
				{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))},
			},
		}
		rt := &NativeRetryTransport{Base: script, Class: mcpcontract.NonReplayableWrite, MaxRetries429: 3, MaxRetries5xx: 3, BaseDelay: time.Millisecond}
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.invalid/send", strings.NewReader("payload"))

		resp, err := rt.RoundTrip(req)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		public := NativePublicError(err)
		if public == nil || public.Category != mcpcontract.OutcomeUnknown || public.Retryable {
			t.Fatalf("status %d: %#v", code, public)
		}

		if script.calls != 1 {
			t.Fatalf("status %d retried: %d", code, script.calls)
		}
	}
}

func TestNativeWriteBodyLimitAfterSuccessIsUnknown(t *testing.T) {
	script := &scriptedTransport{responses: []*http.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"committed"}`))}}}
	transport := &NativeRetryTransport{Class: mcpcontract.NonReplayableWrite, Base: &nativeBodyLimitTransport{base: script, max: 4}}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://docs.googleapis.com/v1/documents", nil)
	if err != nil {
		t.Fatal(err)
	}

	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, err = io.ReadAll(response.Body)

	safe := NativePublicError(err)
	if safe == nil || safe.Category != mcpcontract.OutcomeUnknown || safe.Retryable || script.calls != 1 {
		t.Fatalf("error=%+v calls=%d", safe, script.calls)
	}
}
