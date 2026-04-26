// Package httpx is the shared HTTP machinery for publisher ingestors:
// rate-limited GET with exponential backoff for 5xx and 429 Retry-After.
// Lives under engine/publisher/internal/ so only the ecosystem
// sub-packages can import it.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

// Sentinel errors. Callers use errors.Is to distinguish HTTP-status
// classes without unwrapping a *http.Response.
var (
	ErrNotFound      = errors.New("publisher httpx: not found")
	ErrRateLimited   = errors.New("publisher httpx: rate limited (gave up after retries)")
	ErrServerFailure = errors.New("publisher httpx: upstream 5xx (gave up after retries)")
)

// Client is a rate-limited HTTP client tuned for ingestor traffic.
// One Client per upstream — npm and GitHub get separate buckets so
// neither exhausts the other's quota.
type Client struct {
	HTTP    *http.Client
	Limiter *rate.Limiter
	// Header is applied to every request before send (Authorization,
	// User-Agent, Accept). nil-safe — empty headers add nothing.
	Header http.Header
	// MaxRetries caps the number of retries (excluding the first
	// attempt) for 5xx and 429 responses. Defaults to 3 if zero.
	MaxRetries int
	// RetryBaseDelay seeds the exponential backoff. Doubled per retry,
	// capped at RetryMaxDelay. Defaults to 500ms / 30s.
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
}

// New builds a Client with sensible defaults. Callers tweak fields on
// the returned struct (e.g. set Header for auth tokens).
func New(timeout time.Duration, perSecond rate.Limit, burst int) *Client {
	return &Client{
		HTTP:           &http.Client{Timeout: timeout},
		Limiter:        rate.NewLimiter(perSecond, burst),
		Header:         http.Header{},
		MaxRetries:     3,
		RetryBaseDelay: 500 * time.Millisecond,
		RetryMaxDelay:  30 * time.Second,
	}
}

// Get fetches url with the client's rate limit + retry policy. The
// caller owns the returned body and must Close it. 4xx (except 429)
// returns the response as-is — non-retryable.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	maxRetries := c.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	baseDelay := c.RetryBaseDelay
	if baseDelay <= 0 {
		baseDelay = 500 * time.Millisecond
	}
	maxDelay := c.RetryMaxDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Rate-limit gate. Wait blocks until a token is available or
		// ctx fires. If ctx fires we surface the error verbatim — the
		// scheduler interprets ctx.Err() as "shutting down".
		if err := c.Limiter.Wait(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("publisher httpx: build request: %w", err)
		}
		for k, vs := range c.Header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			// Transport error — treat as retryable.
			lastErr = fmt.Errorf("%w: transport: %v", ErrServerFailure, err)
			if !sleepBackoff(ctx, attempt, baseDelay, maxDelay) {
				return nil, ctx.Err()
			}
			continue
		}

		// Drain the body once per branch so we don't leak connections.
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusOK:
			if readErr != nil {
				return nil, fmt.Errorf("publisher httpx: read body: %w", readErr)
			}
			return body, nil

		case resp.StatusCode == http.StatusNotFound:
			return nil, fmt.Errorf("%w: %s", ErrNotFound, url)

		case resp.StatusCode == http.StatusTooManyRequests:
			lastErr = fmt.Errorf("%w: %s", ErrRateLimited, url)
			delay := retryAfterOrBackoff(resp.Header.Get("Retry-After"), attempt, baseDelay, maxDelay)
			if !sleepFor(ctx, delay) {
				return nil, ctx.Err()
			}
			continue

		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("%w: status %d", ErrServerFailure, resp.StatusCode)
			if !sleepBackoff(ctx, attempt, baseDelay, maxDelay) {
				return nil, ctx.Err()
			}
			continue

		default:
			// Other 4xx — non-retryable.
			return nil, fmt.Errorf("publisher httpx: unexpected status %d for %s: %s",
				resp.StatusCode, url, snippet(body))
		}
	}
	return nil, lastErr
}

// retryAfterOrBackoff honours an integer-seconds Retry-After header
// when present; otherwise falls back to exponential backoff.
func retryAfterOrBackoff(header string, attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	if header != "" {
		if n, err := strconv.Atoi(header); err == nil && n >= 0 {
			d := time.Duration(n) * time.Second
			if d > maxDelay {
				return maxDelay
			}
			return d
		}
	}
	return backoff(attempt, baseDelay, maxDelay)
}

// backoff returns baseDelay * 2^attempt, capped at maxDelay.
func backoff(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	d := baseDelay << attempt //nolint:gosec // attempt is small (≤ MaxRetries)
	if d <= 0 || d > maxDelay {
		return maxDelay
	}
	return d
}

func sleepBackoff(ctx context.Context, attempt int, baseDelay, maxDelay time.Duration) bool {
	return sleepFor(ctx, backoff(attempt, baseDelay, maxDelay))
}

func sleepFor(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func snippet(b []byte) string {
	const limit = 200
	if len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + "…"
}
