package googleapi

import (
	"bytes"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

func calculateBackoff(base time.Duration, attempt int, resp *http.Response) time.Duration {
	// Check Retry-After header
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			if seconds < 0 {
				return 0
			}

			return time.Duration(seconds) * time.Second
		}

		if t, err := http.ParseTime(retryAfter); err == nil {
			d := time.Until(t)
			if d < 0 {
				return 0
			}

			return d
		}
	}

	// Exponential backoff with jitter: 1s, 2s, 4s...
	if base <= 0 {
		return 0
	}

	var baseDelay time.Duration

	if bd := base * time.Duration(1<<attempt); bd <= 0 {
		return 0
	} else {
		baseDelay = bd
	}

	jitterRange := baseDelay / 2
	if jitterRange <= 0 {
		return baseDelay
	}
	jitter := time.Duration(rand.Int64N(int64(jitterRange))) //nolint:gosec // non-crypto jitter

	return baseDelay + jitter
}

func ensureReplayableBody(req *http.Request) error {
	if req == nil || req.Body == nil || req.GetBody != nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	_ = req.Body.Close()

	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	return nil
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 1<<20))
	_ = body.Close()
}
