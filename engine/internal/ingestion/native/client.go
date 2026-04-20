// Package native is the Go client for the rampart-native Rust sidecar.
// Wire protocol documented at schemas/native-ipc.md.
//
// Scope (Phase 1):
//   - One connection per Parse call. Phase 2 adds pooling once we've
//     measured reconnect cost against steady-state SBOM ingest.
//   - Synchronous request → response. No pipelining; each call blocks
//     until the native side answers or the deadline fires.
//   - Request-scoped timeouts via context.Context; no client-level
//     timeout (the UDS dial itself gets a short fallback).
package native

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// MaxFrameBytes caps any single request or response. Matches the Rust
// server's 100 MiB hard cap; a mismatch would surface as a wire-framing
// error, so keep them in sync if either side moves.
const MaxFrameBytes = 100 * 1024 * 1024

// Sentinel errors. `errors.Is` works across the Go parser (engine/sbom/npm)
// and this client: callers can switch on `ErrUnsupportedLockfileVersion`
// once regardless of which parser produced it.
var (
	ErrNativeUnavailable          = errors.New("rampart-native: connect failed")
	ErrMalformedResponse          = errors.New("rampart-native: malformed response")
	ErrRemoteError                = errors.New("rampart-native: remote error")
	ErrMalformedLockfile          = errors.New("rampart-native: malformed lockfile")
	ErrUnsupportedLockfileVersion = errors.New("rampart-native: unsupported lockfile version")
	ErrEmptyLockfile              = errors.New("rampart-native: empty lockfile")
)

// Client talks to a single rampart-native instance over UDS.
type Client struct {
	socketPath  string
	dialTimeout time.Duration
}

// New builds a Client. socketPath is typically pulled from
// `config.Config.NativeSocketPath`; dial timeout is hard-set to 2 s to
// fail fast when the sidecar is missing.
func New(socketPath string) *Client {
	return &Client{
		socketPath:  socketPath,
		dialTimeout: 2 * time.Second,
	}
}

// LockfileMeta matches engine/sbom/npm.LockfileMeta (locally duplicated
// to avoid a circular import).
type LockfileMeta struct {
	ComponentRef string
	CommitSHA    string
	GeneratedAt  time.Time
	ID           string
}

// Ping sends a `ping` request and returns on `pong`. Primarily used by
// /readyz handlers and container healthchecks.
func (c *Client) Ping(ctx context.Context) error {
	req := request{
		ID:   "ping-" + nowID(),
		Kind: "ping",
	}
	resp, err := c.roundTrip(ctx, req)
	if err != nil {
		return err
	}
	if resp.Kind != "pong" {
		return fmt.Errorf("%w: expected `pong`, got %q", ErrMalformedResponse, resp.Kind)
	}
	return nil
}

// ParseNPMLockfile asks the Rust parser to parse `content`. Returns a
// *domain.SBOM reassembled from the wire payload. Errors are classified
// via the sentinels in this package so callers can errors.Is on them.
func (c *Client) ParseNPMLockfile(
	ctx context.Context,
	content []byte,
	meta LockfileMeta,
) (*domain.SBOM, error) {
	var genAt string
	if !meta.GeneratedAt.IsZero() {
		genAt = meta.GeneratedAt.UTC().Format(time.RFC3339Nano)
	}

	req := request{
		ID:   "parse-" + nowID(),
		Kind: "parse_npm_lockfile",
		Payload: &requestPayload{
			Content:      base64.StdEncoding.EncodeToString(content),
			ComponentRef: meta.ComponentRef,
			CommitSHA:    meta.CommitSHA,
			GeneratedAt:  genAt,
			ID:           meta.ID,
		},
	}
	resp, err := c.roundTrip(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, classifyRemoteError(resp.Error)
	}
	if resp.Kind != "parse_result" {
		return nil, fmt.Errorf("%w: expected `parse_result`, got %q", ErrMalformedResponse, resp.Kind)
	}
	if resp.Payload == nil {
		return nil, fmt.Errorf("%w: response payload missing", ErrMalformedResponse)
	}
	var pr parseResultPayload
	if err := json.Unmarshal(resp.Payload, &pr); err != nil {
		return nil, fmt.Errorf("%w: decode payload: %v", ErrMalformedResponse, err)
	}
	return &pr.SBOM, nil
}

// --- wire types (private, mirror schemas/native-ipc.md) ---

type request struct {
	ID      string          `json:"id"`
	Kind    string          `json:"type"`
	Payload *requestPayload `json:"payload,omitempty"`
}

type requestPayload struct {
	Content      string `json:"content"`
	ComponentRef string `json:"component_ref,omitempty"`
	CommitSHA    string `json:"commit_sha,omitempty"`
	GeneratedAt  string `json:"generated_at,omitempty"`
	ID           string `json:"id,omitempty"`
}

type responseEnvelope struct {
	ID      string          `json:"id"`
	Kind    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *remoteError    `json:"error,omitempty"`
}

type remoteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type parseResultPayload struct {
	SBOM  domain.SBOM `json:"sbom"`
	Stats parseStats  `json:"stats"`
}

type parseStats struct {
	ParseMS      int64 `json:"parse_ms"`
	PackageCount int   `json:"package_count"`
	BytesRead    int64 `json:"bytes_read"`
}

func (c *Client) roundTrip(ctx context.Context, req request) (*responseEnvelope, error) {
	d := net.Dialer{Timeout: c.dialTimeout}
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrNativeUnavailable, c.socketPath, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	if len(body) > MaxFrameBytes {
		return nil, fmt.Errorf("request frame %d exceeds MaxFrameBytes %d", len(body), MaxFrameBytes)
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(body)))
	if _, err := conn.Write(header[:]); err != nil {
		return nil, fmt.Errorf("write frame header: %w", err)
	}
	if _, err := conn.Write(body); err != nil {
		return nil, fmt.Errorf("write frame body: %w", err)
	}

	var respHeader [4]byte
	if _, err := io.ReadFull(conn, respHeader[:]); err != nil {
		return nil, fmt.Errorf("read response header: %w", err)
	}
	respLen := binary.BigEndian.Uint32(respHeader[:])
	if respLen > MaxFrameBytes {
		return nil, fmt.Errorf("response frame %d exceeds MaxFrameBytes %d", respLen, MaxFrameBytes)
	}
	respBody := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respBody); err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var env responseEnvelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return nil, fmt.Errorf("%w: decode envelope: %v — body %q",
			ErrMalformedResponse, err, truncate(string(respBody), 200))
	}
	return &env, nil
}

func classifyRemoteError(e *remoteError) error {
	base := fmt.Errorf("%w: [%s] %s", ErrRemoteError, e.Code, e.Message)
	switch e.Code {
	case "MALFORMED_LOCKFILE":
		return fmt.Errorf("%w: %s: %w", ErrMalformedLockfile, e.Message, base)
	case "UNSUPPORTED_LOCKFILE_VERSION":
		return fmt.Errorf("%w: %s: %w", ErrUnsupportedLockfileVersion, e.Message, base)
	case "EMPTY_LOCKFILE":
		return fmt.Errorf("%w: %s: %w", ErrEmptyLockfile, e.Message, base)
	}
	return base
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func nowID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Ensure io.Reader is wired; this variable is only here to catch
// accidental removal of the import by gofmt in future edits.
var _ = bytes.NewReader
