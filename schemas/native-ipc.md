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
is refused as `InvalidData`). This absorbs the 50 MiB `huge-monorepo.json`
benchmark fixture with headroom.

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

Used by the container health check and by the Go client's `Ping` method.

### `parse_npm_lockfile`

```json
{
  "id": "req-abc123",
  "type": "parse_npm_lockfile",
  "payload": {
    "content": "<base64 of package-lock.json bytes>",
    "component_ref": "kind:Component/default/web-app",
    "commit_sha": "abc123",
    "generated_at": "2026-04-20T12:00:00Z",
    "id": "01KPAR-optional-sbom-ulid"
  }
}
```

`content` is **base64**-encoded lockfile bytes so the wire remains bare
JSON regardless of the lockfile's own UTF-8 content. `component_ref`,
`commit_sha`, `generated_at`, and `id` are optional — when omitted the
server stamps empty strings / its own UTC `now()` / a freshly-generated
ULID. Parity tests provide all four explicitly so Go and Rust outputs
are bit-identical.

Success response:

```json
{
  "id": "req-abc123",
  "type": "parse_result",
  "payload": {
    "sbom": {
      "ID": "01KPAR-...",
      "ComponentRef": "kind:Component/default/web-app",
      "CommitSHA": "abc123",
      "Ecosystem": "npm",
      "GeneratedAt": "2026-04-20T12:00:00Z",
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

The SBOM shape matches `engine/internal/domain.SBOM` as serialised by
Go's default `encoding/json` — Pascal-cased field names, no rename
tags, `null` for empty `Scope` — so Go/Rust outputs compare
byte-identical through the shared normalisation in `parity_test.go`.

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
crate version). A breaking change to payload shape bumps the minor
version and adds a `version_required` field to `ping` responses so older
clients can refuse gracefully — deferred to Phase 2.

## Why not protobuf / gRPC

Discussed and rejected in ADR-0005: JSON on the wire is readable in
`tcpdump` / `strace`, needs no generator in either language, and
performance sits comfortably below the parse cost on every lockfile
we've measured (see `docs/benchmarks/sbom-parser.md`). Revisiting is a
Phase 3 option if the proto overhead ever stops being dominated by
lockfile parsing.
