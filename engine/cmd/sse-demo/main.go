// sse-demo is a one-shot tool that stands up the engine on an ephemeral
// port, subscribes to /v1/stream as an HTTP client, publishes three events
// onto the internal bus, and prints the raw wire format to stdout.
//
// Run with: go run ./cmd/sse-demo (from engine/).
//
// It's the "terminal 1 + terminal 2 + terminal 3" demo from the SSE design
// review, folded into a single process for self-contained reproduction.
// The wire format is identical to what `curl -N http://localhost:8080/v1/stream`
// observes against the live engine.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
)

func main() {
	bus := events.NewBus(256)
	srv := api.NewServer(memory.New(), bus, 5*time.Second)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	fmt.Fprintf(os.Stderr, "engine URL:   %s\n", ts.URL)
	fmt.Fprintf(os.Stderr, "subscribing:  GET %s/v1/stream\n", ts.URL)
	fmt.Fprintln(os.Stderr, "--- wire output below ---")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	fmt.Fprintln(os.Stderr, "response headers:")
	for k, v := range resp.Header {
		fmt.Fprintf(os.Stderr, "  %s: %s\n", k, v)
	}

	// Publish three events mid-stream while the reader is copying.
	go func() {
		time.Sleep(150 * time.Millisecond)
		bus.Publish(domain.IncidentOpenedEvent{
			IncidentID:                 "INC-axios-2026-03-31",
			IoCID:                      "IOC-axios-1-11-0",
			AffectedComponentsSnapshot: []string{"kind:Component/default/web-app", "kind:Component/default/billing"},
			At:                         time.Now().UTC(),
		})
		time.Sleep(200 * time.Millisecond)
		bus.Publish(domain.SBOMIngestedEvent{
			SBOMID:       "01KPN-demo-sbom",
			ComponentRef: "kind:Component/default/web-app",
			At:           time.Now().UTC(),
		})
		time.Sleep(200 * time.Millisecond)
		bus.Publish(domain.IncidentTransitionedEvent{
			IncidentID: "INC-axios-2026-03-31",
			From:       domain.StatePending,
			To:         domain.StateTriaged,
			At:         time.Now().UTC(),
		})
	}()

	_, _ = io.Copy(os.Stdout, resp.Body)
}
