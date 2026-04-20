package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

// mkSSETestServer spins up an httptest.Server with a memory backend, a
// fresh EventBus, and configurable heartbeat + subscriber buffer.
func mkSSETestServer(t *testing.T, heartbeat time.Duration, bufferSize int) (string, *events.Bus, func()) {
	t.Helper()
	bus := events.NewBus(bufferSize)
	srv := api.NewServer(memory.New(), bus, heartbeat)
	ts := httptest.NewServer(srv.Handler())
	return ts.URL, bus, ts.Close
}

type sseFrame struct {
	ID, Type, Data string
}

// readFrame pulls SSE lines off the reader and returns when it has
// assembled one full event frame (skips `:` comment heartbeats).
func readFrame(t *testing.T, r *bufio.Scanner) sseFrame {
	t.Helper()
	var ev sseFrame
	for r.Scan() {
		line := r.Text()
		if line == "" {
			if ev.Type != "" || ev.Data != "" {
				return ev
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "id: "):
			ev.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			ev.Type = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			ev.Data = strings.TrimPrefix(line, "data: ")
		}
	}
	t.Fatalf("scanner ended without full frame; err=%v", r.Err())
	return ev
}

// --- (a) publish → receive + schema ---------------------------------------

// TestSSE_EventPublishReceiveAndSchemaMatch is the schema-drift guard. The
// test unmarshals the received frame into gen.StreamEvent and asserts the
// discriminator + variant-specific fields, so any breaking change to
// schemas/openapi.yaml (or the envelope mapper in sse.go) fails the build.
func TestSSE_EventPublishReceiveAndSchemaMatch(t *testing.T) {
	url, bus, cleanup := mkSSETestServer(t, 10*time.Second, 16)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/v1/stream", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	require.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
	require.Equal(t, "keep-alive", resp.Header.Get("Connection"))

	// Wait for subscribe.
	require.Eventually(t, func() bool { return bus.SubscriberCount() == 1 },
		time.Second, 10*time.Millisecond)

	now := time.Now().UTC()
	bus.Publish(domain.IncidentOpenedEvent{
		IncidentID:                 "INC1",
		IoCID:                      "IOC1",
		AffectedComponentsSnapshot: []string{"kind:Component/default/web"},
		At:                         now,
	})

	sc := bufio.NewScanner(resp.Body)
	frame := readFrame(t, sc)
	require.Equal(t, "incident.opened", frame.Type)
	require.Equal(t, "INC1", frame.ID, "SSE id line must equal the event's aggregate id")

	// Drift guard: payload must unmarshal as gen.StreamEvent and
	// destructure into the matching variant without error.
	var se gen.StreamEvent
	require.NoError(t, json.Unmarshal([]byte(frame.Data), &se))

	disc, err := se.Discriminator()
	require.NoError(t, err)
	require.Equal(t, "incident.opened", disc)

	opened, err := se.AsIncidentOpenedEvent()
	require.NoError(t, err)
	require.Equal(t, "INC1", opened.IncidentId)
	require.Equal(t, "IOC1", opened.IocId)
	require.WithinDuration(t, now, opened.OccurredAt, time.Second)
}

// --- (b) heartbeat --------------------------------------------------------

func TestSSE_HeartbeatCommentEmitted(t *testing.T) {
	url, _, cleanup := mkSSETestServer(t, 80*time.Millisecond, 16)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/v1/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sawHeartbeat := false
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), ": keep-alive") {
			sawHeartbeat = true
			break
		}
	}
	require.True(t, sawHeartbeat,
		"server must emit ': keep-alive' comment within the heartbeat interval")
}

// --- (c) disconnect leaves no leaked goroutines ---------------------------

func TestSSE_DisconnectDoesNotLeakGoroutines(t *testing.T) {
	url, _, cleanup := mkSSETestServer(t, 10*time.Second, 16)
	defer cleanup()

	// Warm up HTTP client pool etc. so the baseline captures steady-state.
	for i := 0; i < 2; i++ {
		connectAndDisconnect(t, url)
	}
	require.Eventually(t, func() bool {
		runtime.GC()
		return true
	}, time.Second, 100*time.Millisecond)
	baseline := runtime.NumGoroutine()

	for i := 0; i < 10; i++ {
		connectAndDisconnect(t, url)
	}

	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= baseline+3
	}, 3*time.Second, 100*time.Millisecond,
		"goroutine leak: baseline=%d, current=%d", baseline, runtime.NumGoroutine())
}

func connectAndDisconnect(t *testing.T, url string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/v1/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	cancel()
	_ = resp.Body.Close()
}

// --- (d) slow consumer does not block the publisher -----------------------

func TestSSE_SlowConsumerDoesNotBlockPublisher(t *testing.T) {
	// Tiny subscriber buffer + an HTTP client that never drains the body.
	url, bus, cleanup := mkSSETestServer(t, 10*time.Second, 4)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/v1/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Eventually(t, func() bool { return bus.SubscriberCount() == 1 },
		time.Second, 10*time.Millisecond)

	start := time.Now()
	for i := 0; i < 256; i++ {
		bus.Publish(domain.SBOMIngestedEvent{
			SBOMID:       fmt.Sprintf("s-%d", i),
			ComponentRef: "kind:Component/default/web",
			At:           time.Now(),
		})
	}
	elapsed := time.Since(start)
	require.Less(t, elapsed, 500*time.Millisecond,
		"256 publishes against a slow HTTP client must not block the bus; took %v", elapsed)
}
