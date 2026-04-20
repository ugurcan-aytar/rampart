// Package native is the Go client for the rampart-native Rust sidecar.
// Wire protocol documented at schemas/native-ipc.md — binary envelope
// on the request path (raw lockfile bytes, no base64), JSON on the
// response path (ParsedSBOM). See ADR-0005 Measured Consequences for
// why the split is asymmetric.
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
	"context"
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

// Wire opcodes — kept in lock-step with native/crates/rampart-native/src/protocol.rs.
const (
	msgParseRequest byte = 0x01
	msgParseResult  byte = 0x02
	msgError        byte = 0x03
	msgPong         byte = 0xFE
	msgPing         byte = 0xFF
)

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

// Ping sends a `ping` opcode and returns on `pong`. Primarily used by
// parity tests and readiness probes.
func (c *Client) Ping(ctx context.Context) error {
	// Request body = opcode only (1 byte).
	frame := make([]byte, 0, 5)
	frame = binary.BigEndian.AppendUint32(frame, 1)
	frame = append(frame, msgPing)

	resp, err := c.roundTrip(ctx, frame)
	if err != nil {
		return err
	}
	if len(resp) != 1 || resp[0] != msgPong {
		return fmt.Errorf("%w: expected pong (single 0x%X byte), got %v",
			ErrMalformedResponse, msgPong, resp)
	}
	return nil
}

// ParseNPMLockfile asks the Rust parser to parse `content`. Returns a
// *domain.ParsedSBOM — identity fields (ID, GeneratedAt, ComponentRef,
// CommitSHA) are the caller's responsibility; wrap with
// engine/ingestion.Ingest when the engine wants a full SBOM.
func (c *Client) ParseNPMLockfile(ctx context.Context, content []byte) (*domain.ParsedSBOM, error) {
	// Request body layout:
	//   1 byte opcode
	// + 4-byte BE content_length + content bytes
	// + 4-byte BE metadata_length (0 in Phase 1) + 0 metadata bytes
	bodyLen := 1 + 4 + len(content) + 4
	if bodyLen > MaxFrameBytes {
		return nil, fmt.Errorf("request body %d exceeds MaxFrameBytes %d", bodyLen, MaxFrameBytes)
	}

	frame := make([]byte, 0, 4+bodyLen)
	frame = binary.BigEndian.AppendUint32(frame, uint32(bodyLen))
	frame = append(frame, msgParseRequest)
	frame = binary.BigEndian.AppendUint32(frame, uint32(len(content)))
	frame = append(frame, content...)
	frame = binary.BigEndian.AppendUint32(frame, 0) // metadata_length = 0

	respBody, err := c.roundTrip(ctx, frame)
	if err != nil {
		return nil, err
	}
	if len(respBody) < 1 {
		return nil, fmt.Errorf("%w: empty response body", ErrMalformedResponse)
	}
	opcode := respBody[0]
	rest := respBody[1:]

	switch opcode {
	case msgParseResult:
		return decodeParseResult(rest)
	case msgError:
		return nil, decodeErrorFrame(rest)
	default:
		return nil, fmt.Errorf("%w: unexpected opcode 0x%X", ErrMalformedResponse, opcode)
	}
}

// roundTrip opens a UDS connection, writes the whole request frame,
// and returns the response body (everything after the 4-byte outer
// length prefix). Single-shot — the connection is closed on return.
func (c *Client) roundTrip(ctx context.Context, frame []byte) ([]byte, error) {
	d := net.Dialer{Timeout: c.dialTimeout}
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrNativeUnavailable, c.socketPath, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if _, err := conn.Write(frame); err != nil {
		return nil, fmt.Errorf("write request frame: %w", err)
	}

	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, fmt.Errorf("read response header: %w", err)
	}
	respLen := binary.BigEndian.Uint32(header[:])
	if respLen == 0 || respLen > MaxFrameBytes {
		return nil, fmt.Errorf("response frame %d out of range [1, %d]", respLen, MaxFrameBytes)
	}
	respBody := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respBody); err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	return respBody, nil
}

func decodeParseResult(rest []byte) (*domain.ParsedSBOM, error) {
	if len(rest) < 4 {
		return nil, fmt.Errorf("%w: parse_result truncated at sbom length prefix", ErrMalformedResponse)
	}
	sbomLen := binary.BigEndian.Uint32(rest[:4])
	if int(sbomLen)+4 != len(rest) {
		return nil, fmt.Errorf("%w: parse_result inner length (%d) does not match remaining (%d)",
			ErrMalformedResponse, sbomLen, len(rest)-4)
	}
	var sbom domain.ParsedSBOM
	if err := json.Unmarshal(rest[4:], &sbom); err != nil {
		return nil, fmt.Errorf("%w: decode parsed_sbom: %v", ErrMalformedResponse, err)
	}
	return &sbom, nil
}

func decodeErrorFrame(rest []byte) error {
	if len(rest) < 4 {
		return fmt.Errorf("%w: error frame truncated at code length", ErrMalformedResponse)
	}
	codeLen := binary.BigEndian.Uint32(rest[:4])
	if 4+int(codeLen) > len(rest) {
		return fmt.Errorf("%w: error code overruns frame", ErrMalformedResponse)
	}
	code := string(rest[4 : 4+codeLen])
	after := 4 + int(codeLen)
	if len(rest) < after+4 {
		return fmt.Errorf("%w: error frame truncated at message length", ErrMalformedResponse)
	}
	msgLen := binary.BigEndian.Uint32(rest[after : after+4])
	if after+4+int(msgLen) != len(rest) {
		return fmt.Errorf("%w: error message overruns or underruns frame", ErrMalformedResponse)
	}
	message := string(rest[after+4 : after+4+int(msgLen)])
	return classifyRemoteError(code, message)
}

func classifyRemoteError(code, message string) error {
	base := fmt.Errorf("%w: [%s] %s", ErrRemoteError, code, message)
	switch code {
	case "MALFORMED_LOCKFILE":
		return fmt.Errorf("%w: %s: %w", ErrMalformedLockfile, message, base)
	case "UNSUPPORTED_LOCKFILE_VERSION":
		return fmt.Errorf("%w: %s: %w", ErrUnsupportedLockfileVersion, message, base)
	case "EMPTY_LOCKFILE":
		return fmt.Errorf("%w: %s: %w", ErrEmptyLockfile, message, base)
	}
	return base
}
