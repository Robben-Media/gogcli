package googleapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/mcpcontract"
)

// NativeRetryTransport applies catalog retry classes. Safe reads may retry
// 429, 5xx, and 403 rateLimitExceeded. Write and unknown classes never replay.
type NativeRetryTransport struct {
	Base          http.RoundTripper
	Class         mcpcontract.RetryClass
	MaxRetries429 int
	MaxRetries5xx int
	BaseDelay     time.Duration
	now           func() time.Time
}

func (t *NativeRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, &mcpcontract.Error{Category: mcpcontract.InvalidInput, Message: "request is required"}
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	if t.Class != mcpcontract.SafeRead {
		return t.roundTripOnce(base, req)
	}

	if err := ensureReplayableBody(req); err != nil {
		return nil, fmt.Errorf("googleapi: read request body: %w", err)
	}

	retries429 := 0
	retries5xx := 0

	max429 := t.MaxRetries429
	if max429 <= 0 {
		max429 = MaxRateLimitRetries
	}

	max5xx := t.MaxRetries5xx
	if max5xx <= 0 {
		max5xx = Max5xxRetries
	}

	for {
		if err := resetReplayableBody(req); err != nil {
			return nil, err
		}

		AddUpstreamCall(req.Context())

		resp, err := base.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("googleapi: google request: %w", err)
		}

		retry, delay := t.safeReadRetry(req, resp, retries429, retries5xx, max429, max5xx)
		if !retry {
			return resp, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
			retries429++
		} else {
			retries5xx++
		}

		drainAndClose(resp.Body)

		if err := t.sleep(req.Context(), delay); err != nil {
			return nil, NativePublicError(err)
		}
	}
}

func (t *NativeRetryTransport) roundTripOnce(base http.RoundTripper, req *http.Request) (*http.Response, error) {
	AddUpstreamCall(req.Context())

	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, &mcpcontract.Error{
			Category:  mcpcontract.OutcomeUnknown,
			Message:   nativeWriteUnknownMessage,
			Retryable: false,
		}
	}

	return resp, nil
}

func (t *NativeRetryTransport) safeReadRetry(req *http.Request, resp *http.Response, retries429, retries5xx, max429, max5xx int) (bool, time.Duration) {
	if resp == nil || resp.StatusCode < 400 {
		return false, 0
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		if retries429 >= max429 {
			return false, 0
		}

		if resp.StatusCode == http.StatusForbidden && !nativePeekRateLimit(resp) {
			return false, 0
		}

		delay := t.retryDelay(retries429, resp)
		if t.exceedsDeadline(req, delay) {
			return false, 0
		}

		return true, delay
	}

	if resp.StatusCode >= 500 {
		if retries5xx >= max5xx {
			return false, 0
		}

		delay := ServerErrorRetryDelay
		if t.BaseDelay > 0 {
			delay = t.BaseDelay
		}

		if t.exceedsDeadline(req, delay) {
			return false, 0
		}

		return true, delay
	}

	return false, 0
}

func nativePeekRateLimit(resp *http.Response) bool {
	if resp == nil || resp.Body == nil {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))

	if err != nil {
		return false
	}

	text := string(body)

	return strings.Contains(text, "rateLimitExceeded") || strings.Contains(text, "userRateLimitExceeded")
}

func (t *NativeRetryTransport) retryDelay(attempt int, resp *http.Response) time.Duration {
	helper := RetryTransport{BaseDelay: t.BaseDelay}
	if helper.BaseDelay <= 0 {
		helper.BaseDelay = RateLimitBaseDelay
	}

	return helper.calculateBackoff(attempt, resp)
}

func (t *NativeRetryTransport) exceedsDeadline(req *http.Request, delay time.Duration) bool {
	if req == nil || delay <= 0 {
		return false
	}

	deadline, ok := req.Context().Deadline()
	if !ok {
		return false
	}

	now := time.Now()
	if t != nil && t.now != nil {
		now = t.now()
	}

	return !now.Add(delay).Before(deadline)
}

func (t *NativeRetryTransport) sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("googleapi: retry wait: %w", ctx.Err())
	}
}

func resetReplayableBody(req *http.Request) error {
	if req == nil || req.GetBody == nil {
		return nil
	}

	if req.Body != nil {
		_ = req.Body.Close()
	}

	body, err := req.GetBody()
	if err != nil {
		return fmt.Errorf("googleapi: reset request body: %w", err)
	}

	req.Body = body

	return nil
}

type nativeSlotTransport struct {
	slots chan struct{}
	base  http.RoundTripper
}

func (t *nativeSlotTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.slots != nil {
		select {
		case t.slots <- struct{}{}:
			defer func() { <-t.slots }()
		case <-req.Context().Done():
			return nil, NativePublicError(req.Context().Err())
		}
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("googleapi: bounded request: %w", err)
	}

	return resp, nil
}

type nativeBodyLimitTransport struct {
	base http.RoundTripper
	max  int64
}

func (t *nativeBodyLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("googleapi: google transport: %w", err)
	}

	if resp != nil && resp.Body != nil && t.max > 0 {
		resp.Body = &nativeLimitedBody{ReadCloser: resp.Body, remaining: t.max}
	}

	return resp, nil
}

type nativeLimitedBody struct {
	io.ReadCloser
	remaining int64
}

var errNativeResponseLimit = errors.New("googleapi: response exceeds bounded read limit")

func (b *nativeLimitedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		probe := make([]byte, 1)

		n, err := b.ReadCloser.Read(probe)
		if n > 0 {
			return 0, errNativeResponseLimit
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0, io.EOF
			}

			return 0, fmt.Errorf("googleapi: read response: %w", err)
		}

		return 0, nil
	}

	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}

	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)

	if err != nil {
		if errors.Is(err, io.EOF) {
			return n, io.EOF
		}

		return n, fmt.Errorf("googleapi: read response: %w", err)
	}

	return n, nil
}
