# rampart-native IPC protocol

Wire contract between the Go engine and the Rust `rampart-native` sidecar.
Referenced from `docs/decisions/0005-no-cgo-rust-via-uds.md`.

## Transport

Unix Domain Socket, SOCK_STREAM. Platform scope: Linux + macOS. Windows
named-pipe support is Phase 2 (same ADR).

| Environment | Default path | Override |
|---|---|---|
| Dev (local)  | `/tmp/rampart-native.sock`            | `RAMPART_NATIVE_SOCKET` env var |
| Container    | `/var/run/rampart/native.sock`        | mounted via a shared volume    |

## Framing

Every request / response is **outer-length-prefixed + binary body**:

```
┌─────────────────────────┬─────────────────────────────────┐
│ 4 bytes, big-endian u32 │  N bytes of body                │
│  = outer body length N  │  body always starts with 1-byte │
│                         │  opcode, then opcode-specific   │
└─────────────────────────┴─────────────────────────────────┘
```

Hard cap per frame: **100 MiB** (enforced by both sides — a bigger
prefix is rejected). This absorbs the largest synthetic benchmark
fixture (100 000 packages → ~15 MiB, see `docs/benchmarks/sbom-parser.md`)
with order-of-magnitude headroom for real-world enterprise monorepos.

A single connection can carry many frames back-to-back. The server
enforces a **30-second per-frame read timeout** once a frame has
started — idle connections between frames are unbounded, but a frame
that stalls mid-body gets the connection dropped (slowloris defence).

The Go client opens one connection per `ParseNPMLockfile` call and
closes after reading the response. Pooling is Phase 2 once we've
measured the reconnect cost.

### Why binary on requests, JSON on responses

Adım 6's first wire revision base64-encoded the lockfile into a JSON
envelope (`{ "content": "<base64>" }`). The benchmark at close showed
the envelope — not the Rust parser — dominating the round-trip. The
second revision (this document) made the **request** path fully binary
(raw lockfile bytes, no base64, no JSON escape) and kept the
**response** JSON (the SBOM is orders of magnitude smaller than a
large lockfile, JSON keeps `strace` / `tcpdump` output readable).
Measurement closed ~half of the remaining gap — details in
`docs/benchmarks/sbom-parser.md`.

## Request opcodes (client → server)

Every body's first byte is the opcode.

### `0x01` — parse_npm_lockfile

```
┌───────────┬────────────────────────┬─────────────────────────┬─────────────────────────┬──────────────────────────┐
│ opcode    │ 4-byte BE content_len  │ content_len raw bytes   │ 4-byte BE metadata_len  │ metadata_len raw bytes   │
│ = 0x01    │                        │ (the lockfile body)     │                         │ (reserved — Phase 1: 0)  │
└───────────┴────────────────────────┴─────────────────────────┴─────────────────────────┴──────────────────────────┘
```

`content` is the **raw** `package-lock.json` bytes — no base64, no
JSON escape. `metadata` is reserved: Phase 1 callers send 0 bytes,
the server ignores any payload. Phase 2 may use this as a JSON
options blob (e.g. `{ "skip_integrity_check": true }`) without a
wire revision.

### `0xFF` — ping

```
┌───────────┐
│ 0xFF      │
└───────────┘
```

Body is just the opcode byte (outer length = 1). Used by the Go
client's `Ping` method and, in Adım 7, by a dedicated prober binary
(distroless containers have no shell, so the compose healthcheck
can't open a UDS connection itself).

## Response opcodes (server → client)

### `0x02` — parse_result

```
┌───────────┬────────────────────────┬─────────────────────────┐
│ opcode    │ 4-byte BE sbom_len     │ sbom_len bytes of JSON  │
│ = 0x02    │                        │ = ParsedSBOM            │
└───────────┴────────────────────────┴─────────────────────────┘
```

The inner JSON is `domain.ParsedSBOM` as serialised by Go's default
`encoding/json` — Pascal-cased field names, `null` for empty `Scope`.
Example (whitespace added for readability):

```json
{
  "Ecosystem": "npm",
  "Packages": [
    {
      "Ecosystem": "npm",
      "Name": "axios",
      "Version": "1.11.0",
      "PURL": "pkg:npm/axios@1.11.0",
      "Scope": null,
      "Integrity": "sha512-..."
    }
  ],
  "SourceFormat": "npm-package-lock-v3",
  "SourceBytes": 765
}
```

### `0x03` — error

```
┌───────────┬─────────────────────┬──────────────────┬────────────────────────┬──────────────────┐
│ opcode    │ 4-byte BE code_len  │ code_len bytes   │ 4-byte BE message_len  │ message_len bytes│
│ = 0x03    │                     │ (ASCII error id) │                        │ (UTF-8 message)  │
└───────────┴─────────────────────┴──────────────────┴────────────────────────┴──────────────────┘
```

Error codes the server emits:

| Code                          | Cause                                                  |
|-------------------------------|--------------------------------------------------------|
| `MALFORMED_REQUEST`           | decode_request_body failed (unknown opcode, truncated, trailing bytes, or empty body) |
| `MALFORMED_LOCKFILE`          | `package-lock.json` JSON parse failed                  |
| `UNSUPPORTED_LOCKFILE_VERSION`| `lockfileVersion` missing or ≠ 3 (v1 / v2 land here)   |
| `EMPTY_LOCKFILE`              | `packages` key absent from the lockfile                |
| `INTERNAL_ERROR`              | response JSON marshal failed (should never happen)     |

These names mirror the Go side's sentinel errors
(`ErrMalformedLockfile`, `ErrUnsupportedLockfileVersion`,
`ErrEmptyLockfile`) so callers can switch on `code` regardless of
which parser produced the result.

### `0xFE` — pong

```
┌───────────┐
│ 0xFE      │
└───────────┘
```

Body is just the opcode byte (outer length = 1).

## Versioning

The shape above is v1. The first implementation shipped in Adım 6 used
a JSON-envelope request (base64 content + JSON metadata) and is
**not** wire-compatible with v1 — callers must upgrade in lockstep.
A future breaking change will add an explicit version handshake (a
`0x00 hello` opcode returning `{"version":"v2"}`) so older clients
can refuse gracefully.

## Why not protobuf / gRPC

Discussed and rejected in ADR-0005 at the decision point, and
reinforced by measurement: the request path's binary framing is
trivial to read (four fixed-width length prefixes + raw bytes) and
needs no generator in either language. The response's JSON is small
and human-readable in `strace` / `tcpdump`. A protobuf schema would
add generator toolchains to both sides with no measured win.
Revisiting is a Phase 3 option if later measurements change.
