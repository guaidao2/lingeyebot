package llm

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	llmMaxAttempts = 4
	llmBaseDelay   = 500 * time.Millisecond
	llmMaxDelay    = 15 * time.Second
)

// defaultLLMClient uses a response-header timeout, not a total deadline: a
// streamed generation may legitimately run longer than the header timeout.
func defaultLLMClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 60 * time.Second
	return &http.Client{Transport: transport}
}

func retryableStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if when.After(now) {
			return when.Sub(now)
		}
	}
	return 0
}

func retryDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > 30*time.Second {
			return 30 * time.Second
		}
		return retryAfter
	}
	delay := llmBaseDelay << attempt
	if delay > llmMaxDelay {
		delay = llmMaxDelay
	}
	jitter := time.Duration(0)
	if delay > 0 {
		jitter = time.Duration(rand.Int63n(int64(delay/2) + 1))
	}
	return delay + jitter
}

type llmStatusError struct {
	Status string
	Body   string
}

func (e *llmStatusError) Error() string {
	if e.Body == "" {
		return "llm " + e.Status
	}
	return fmt.Sprintf("llm %s: %s", e.Status, e.Body)
}

// doWithRetry retries a request whose body can be rebuilt. It only returns a
// response for a non-retryable status or success; retryable statuses are
// drained and retried, so streaming callers cannot duplicate tokens.
func doWithRetry(ctx context.Context, client *http.Client, build func() (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < llmMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := build()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err == nil && !retryableStatus(resp.StatusCode) {
			return resp, nil
		}
		var retryAfter time.Duration
		if err != nil {
			lastErr = err
		} else {
			retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			lastErr = &llmStatusError{Status: resp.Status, Body: truncate(string(raw), 1000)}
		}
		if attempt == llmMaxAttempts-1 {
			break
		}
		timer := time.NewTimer(retryDelay(attempt, retryAfter))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("request failed")
	}
	return nil, fmt.Errorf("llm retry exhausted after %d attempts: %w", llmMaxAttempts, lastErr)
}
