package httpx_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/ugurcan-aytar/rampart/engine/publisher/internal/httpx"
)

// fastClient produces a Client tuned for tests: tight timeout, generous
// rate limit, fast retry backoff so a 3-attempt loop fits in <100 ms.
func fastClient() *httpx.Client {
	c := httpx.New(2*time.Second, rate.Limit(1000), 1000)
	c.RetryBaseDelay = 1 * time.Millisecond
	c.RetryMaxDelay = 10 * time.Millisecond
	return c
}

func TestGet_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	body, err := fastClient().Get(context.Background(), srv.URL+"/x")
	require.NoError(t, err)
	require.Equal(t, `{"ok":true}`, string(body))
}

func TestGet_NotFoundReturnsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fastClient().Get(context.Background(), srv.URL+"/x")
	require.Error(t, err)
	require.True(t, errors.Is(err, httpx.ErrNotFound))
}

func TestGet_RetriesOn5xx_ThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`recovered`))
	}))
	defer srv.Close()

	body, err := fastClient().Get(context.Background(), srv.URL+"/x")
	require.NoError(t, err)
	require.Equal(t, "recovered", string(body))
	require.Equal(t, int32(3), hits.Load())
}

func TestGet_GivesUpAfterMaxRetriesOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := fastClient()
	c.MaxRetries = 2 // 1 initial + 2 retries = 3 total
	_, err := c.Get(context.Background(), srv.URL+"/x")
	require.Error(t, err)
	require.True(t, errors.Is(err, httpx.ErrServerFailure))
}

func TestGet_BackoffOn429HonoursRetryAfter(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1") // 1s
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := httpx.New(2*time.Second, rate.Limit(1000), 1000)
	c.RetryBaseDelay = 1 * time.Millisecond
	c.RetryMaxDelay = 5 * time.Second // allow honoring 1s Retry-After

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL+"/x")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Equal(t, "ok", string(body))
	require.Equal(t, int32(2), hits.Load())
	require.GreaterOrEqual(t, elapsed, 900*time.Millisecond,
		"Retry-After: 1 must produce ≥ ~1s wait")
}

func TestGet_GivesUpAfterMaxRetriesOn429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := fastClient()
	c.MaxRetries = 2
	_, err := c.Get(context.Background(), srv.URL+"/x")
	require.Error(t, err)
	require.True(t, errors.Is(err, httpx.ErrRateLimited))
}

func TestGet_RateLimiterEnforcesPerSecondCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// 5 req/s, burst 1 → 4 sequential calls take ≥ ~800 ms.
	c := httpx.New(2*time.Second, rate.Limit(5), 1)
	c.MaxRetries = 0

	start := time.Now()
	for i := 0; i < 5; i++ {
		_, err := c.Get(context.Background(), srv.URL+"/x")
		require.NoError(t, err)
	}
	elapsed := time.Since(start)
	require.GreaterOrEqual(t, elapsed, 700*time.Millisecond,
		"limiter must throttle: %v", elapsed)
}

func TestGet_4xxNonRetryableReturnsImmediately(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"err":"bad"}`))
	}))
	defer srv.Close()

	_, err := fastClient().Get(context.Background(), srv.URL+"/x")
	require.Error(t, err)
	require.Equal(t, int32(1), hits.Load(), "4xx other than 429 must not retry")
}

func TestGet_HeadersForwarded(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := fastClient()
	c.Header.Set("Authorization", "Bearer xyz")
	c.Header.Set("User-Agent", "rampart-test")

	_, err := c.Get(context.Background(), srv.URL+"/x")
	require.NoError(t, err)
	require.Equal(t, "Bearer xyz", got.Get("Authorization"))
	require.Equal(t, "rampart-test", got.Get("User-Agent"))
}

func TestGet_ContextCancelDuringBackoffAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := httpx.New(2*time.Second, rate.Limit(1000), 1000)
	c.MaxRetries = 5
	c.RetryBaseDelay = 200 * time.Millisecond // generous so we have time to cancel
	c.RetryMaxDelay = time.Second

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := c.Get(ctx, srv.URL+"/x")
	elapsed := time.Since(start)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled),
		"want context.Canceled, got %v", err)
	require.Less(t, elapsed, 600*time.Millisecond, "must abort promptly")
}

// silence unused import lint in the rare case strconv goes unused
var _ = strconv.Itoa
