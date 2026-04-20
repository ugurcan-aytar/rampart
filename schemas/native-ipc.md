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

Every request / response is one **length-prefixed JSON frame**:

```
┌─────────────────────────┬─────────────────────────────────┐
│ 4 bytes, big-endian u32 │ exactly that many bytes of JSON │
└─────────────────────────┴─────────────────────────────────┘
```

Hard cap per frame: **100 MiB** (enforced by the server — a bigger prefix
is refused as `InvalidData`). This absorbs the 15 MiB `huge-100k-pkgs.json`
benchmark fixture with order-of-magnitude headroom for the larger
synthetic fixtures Adım 8 CI may generate.

A single TCP connection can carry many frames back-to-back. The server
handles each request in turn; the client currently opens one connection
per `ParseNPMLockfile` call and closes after reading the response
(connection pooling is Phase 2 once we have measured reconnect cost).

## Request types

Every request carries `id` (correlation — echoed back on the response)
and `type` (discriminator). Payload shape is `type`-specific.

### `ping`

```json
{"id": "r-any", "type": "ping"}
```

Health probe. Response:

```json
{"id": "r-any", "type": "pong", "payload": {}}
```

Used by the Go client's `Ping` method (and, in Adım 7, by a dedicated
prober binary — distroless containers have no shell, so the compose
healthcheck can't speak the protocol directly).

### `parse_npm_lockfile`

```json
{
  "id": "req-abc123",
  "type": "parse_npm_lockfile",
  "payload": {
    "content": "<base64 of package-lock.json bytes>"
  }
}
```

`content` is **base64**-encoded lockfile bytes so the wire remains bare
JSON regardless of the lockfile's own UTF-8 content. **No other fields.**

ADR-0005 makes the parser pure: identity (`id`, `generated_at`,
`component_ref`, `commit_sha`) is stamped by the engine's ingestion
layer — see `engine/ingestion`. The earlier revision of this payload
carried those fields and the parity test normalised them out; the new
shape lets Go and Rust outputs diff byte-for-byte with zero shims.

Success response:

```json
{
  "id": "req-abc123",
  "type": "parse_result",
  "payload": {
    "parsed_sbom": {
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
    },
    "stats": {
      "parse_ms": 12,
      "package_count": 1,
      "bytes_read": 765
    }
  }
}
```

The `parsed_sbom` shape matches `engine/internal/domain.ParsedSBOM` as
serialised by Go's default `encoding/json` — Pascal-cased field names,
no rename tags, `null` for empty `Scope` — so Go and Rust outputs are
byte-identical out of the parser.

### `shutdown`

```json
{"id": "r-any", "type": "shutdown"}
```

Reserved for the test harness. Server acknowledges with `shutdown_ack`;
the process continues running (SIGTERM is the real stop signal). Kept
so tests can verify the server round-trips unknown-but-documented
requests without surprises.

## Error responses

Any failure returns:

```json
{
  "id": "req-abc123",
  "type": "error",
  "error": {
    "code": "UNSUPPORTED_LOCKFILE_VERSION",
    "message": "unsupported lockfile version: got 2, expected 3"
  }
}
```

Error codes the server emits:

| Code                          | Cause                                                  |
|-------------------------------|--------------------------------------------------------|
| `MALFORMED_REQUEST`           | JSON decode failed or missing `payload`                |
| `UNKNOWN_REQUEST`             | `type` not one of the three above                      |
| `INVALID_BASE64`              | `payload.content` is not decodable base64              |
| `MALFORMED_LOCKFILE`          | package-lock.json JSON parse failed                    |
| `UNSUPPORTED_LOCKFILE_VERSION`| `lockfileVersion` missing or ≠ 3 (v1 / v2 land here)   |
| `EMPTY_LOCKFILE`              | `packages` key absent from the lockfile                |

These names mirror the Go side's sentinel errors (`ErrMalformedLockfile`,
`ErrUnsupportedLockfileVersion`, `ErrEmptyLockfile`) so callers can
switch on `code` regardless of which parser produced the result.

## Versioning

`info.version` on the wire is today `"0.1.0"` (tracked implicitly by the
crate version). This is the first tagged shape; a later breaking change
to the payload bumps the minor version and adds a `version_required`
field to `ping` responses so older clients can refuse gracefully —
deferred to Phase 2.

## Why not protobuf / gRPC

Discussed and rejected in ADR-0005: JSON on the wire is readable in
`tcpdump` / `strace`, needs no generator in either language, and
performance sits comfortably below the parse cost on every lockfile
we've measured (see `docs/benchmarks/sbom-parser.md`). Revisiting is a
Phase 3 option if the proto overhead ever stops being dominated by
lockfile parsing.
