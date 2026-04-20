// Package receiver is a minimal SSE client for the engine's /v1/stream.
// The stream lines are parsed into Frame values and passed to the caller's
// handler. Connection drop → reconnect with 1s backoff.
//
// This package deliberately has NO dependency on the engine's Go module.
// slack-notifier is a consumer that speaks HTTP; the engine's internal
// types stay internal. This is the first concrete instance of rampart's
// adapter pattern — new consumers attach to the wire contract, not to
// rampart's internals.
package receiver

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Frame is one assembled SSE event. ID / EventType / Data correspond to
// the `id: / event: / data:` lines, respectively.
type Frame struct {
	ID        string
	EventType string
	Data      string
}

// Client holds the SSE subscription. A Client is safe for single-goroutine
// use; create one per subscription.
type Client struct {
	url  string
	log  *slog.Logger
	http *http.Client
}

// New constructs a Client pointed at an SSE URL.
func New(url string, log *slog.Logger) *Client {
	return &Client{
		url: url,
		log: log,
		// SSE is long-lived — no client-level timeout; per-request timeouts
		// come from ctx deadlines.
		http: &http.Client{Timeout: 0},
	}
}

// Run opens the subscription and delivers every assembled Frame to
// handle. When the underlying connection drops, Run reconnects after
// a 1 s backoff. Returns ctx.Err() when ctx is done.
func (c *Client) Run(ctx context.Context, handle func(Frame)) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := c.once(ctx, handle); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.log.Warn("sse: reconnecting in 1s", "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
			continue
		}
		// clean EOF (server closed) — reconnect.
		c.log.Info("sse: server closed stream; reconnecting")
	}
}

func (c *Client) once(ctx context.Context, handle func(Frame)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sse: unexpected status %d from %s", resp.StatusCode, c.url)
	}
	return ReadStream(resp.Body, handle)
}

// ReadStream is exported for tests: given an io.Reader over the SSE body
// content, it assembles frames and dispatches handle until EOF.
func ReadStream(r io.Reader, handle func(Frame)) error {
	sc := bufio.NewScanner(r)
	// SSE payloads can be larger than the default 64 KB scan buffer.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var cur Frame
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if cur.EventType != "" || cur.Data != "" {
				handle(cur)
				cur = Frame{}
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // comment / heartbeat
		}
		switch {
		case strings.HasPrefix(line, "id: "):
			cur.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			cur.EventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.Data = strings.TrimPrefix(line, "data: ")
		}
	}
	return sc.Err()
}
