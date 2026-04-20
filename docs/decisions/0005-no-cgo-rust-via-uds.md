# ADR-0005: Rust parser lives in a sidecar process — cgo is off the table, Unix Domain Sockets carry the wire

## Status

Accepted — 2026-04-20. Implementation landed in Adım 6 (commit forthcoming).

## Context

rampart parses npm (and later pypi / cargo / go) lockfiles for the
incident engine. Large monorepo lockfiles can comfortably exceed 50 MiB;
the axios-compromise fixture is 765 bytes but simple-webapp is already
~2 KiB and a 50k-package Babel + Storybook monorepo routinely ships a
lockfile that saturates Node's JSON.parse. Two options for the parser:

1. **Pure Go parser everywhere.** Works — shipped in
   `engine/sbom/npm/parser.go` since Adım 2 — but throughput drops on
   huge lockfiles because the standard library's `encoding/json` is
   reflection-driven and eats memory proportional to the unmarshalled
   map.

2. **Rust parser for the hot path.** `serde_json` is fast in isolation
   and Rust's memory model allows zero-copy shapes `encoding/json`
   can't express. At the time of the decision the working assumption
   was "this will win on huge lockfiles"; the measurements at Adım 6
   close **falsified that** (see "Measured consequences" below). The
   current reason to keep the separate Rust parser is architectural
   isolation, not throughput — the decision below still holds on
   those grounds.

Three common integration shapes, all rejected in turn:

- **cgo static-link against a Rust crate compiled as `cdylib`.** Memory
  ownership is ours to manage across the FFI boundary (rust-lang/rfcs#2753
  summarises the constraints — https://rust-lang.github.io/rfcs/2753-ffi-safety.html);
  cross-compile cascades into pain (https://github.com/rust-embedded/cross
  exists *because* pure-Go's `GOOS=linux GOARCH=arm64` story breaks as
  soon as cgo is on). rampart's deploy targets (darwin-arm64, darwin-amd64,
  linux-amd64, linux-arm64) become four separate cross builds per
  release instead of four `go build` invocations.
- **Shared library via `dlopen`/`LoadLibrary`.** Windows support adds a
  third code path; symbol versioning adds runtime failure modes that
  don't exist in a pure-Go build.
- **gRPC over localhost TCP.** Codegen in two languages, proto schemas
  to maintain, a loopback port to choose, zero wire debuggability with
  `tcpdump`. Overkill for "hand the bytes to a parser and read the
  bytes back".

## Decision

`rampart-native` is a **separate process** written in Rust, and the Go
engine talks to it over a **Unix Domain Socket** using a **length-prefixed
JSON** protocol (documented at `schemas/native-ipc.md`).

The engine opens a connection, writes one request frame, reads one
response frame, closes. No cgo, no dynamic linking, no shared memory.
Each side ships its own binary on its own release cadence; cross-compile
stays single-toolchain per language.

**The parser is pure; ID generation is the caller's responsibility.**
Both parsers (Go in-process, Rust over UDS) return a `ParsedSBOM` —
`{Ecosystem, Packages, SourceFormat, SourceBytes}`, and nothing else.
Turning a `ParsedSBOM` into a full `SBOM` (stamping `ID`, `GeneratedAt`,
`ComponentRef`, `CommitSHA`) happens in the engine's ingestion layer
(`engine/ingestion.Ingest`). This split lets the parity test compare
parser outputs byte-for-byte with zero normalisation shims — which is
the strongest possible definition of "the two parsers are twins". The
earlier design stamped identity inside the parser and the parity test
worked around the per-call-volatile fields; that was a shim and is gone.

```
 ┌────────────┐   UDS SOCK_STREAM    ┌────────────────┐
 │ Go engine  │ ──────────────────▶  │ rampart-native │
 │ (client)   │   length-prefix JSON │ (Rust server)  │
 └────────────┘ ◀──────────────────  └────────────────┘
```

The strategy is configurable per engine: `parser.strategy: go | native`.
Default stays `go` — the native path is opt-in for deployments that
have the sidecar available.

## Consequences

**Positive**

- **No cgo.** `engine/go.mod` is zero-cgo and stays that way. `go build`
  cross-compiles to every rampart-supported target with `CGO_ENABLED=0`.
- **Independent release cadence.** A Rust parser bugfix doesn't rebuild
  the Go engine image; a Go engine change doesn't touch the Rust sidecar.
- **Language-native tooling.** Rust side gets `cargo clippy -D warnings`,
  `cargo fmt --check`, `cargo deny check` (Adım 8). Go side keeps
  `go vet` / `staticcheck` / `golangci-lint`.
- **Wire debuggability.** Every byte on the socket is human-readable
  JSON; `strace` / `tcpdump` / `socat -v` produce useful output without
  protobuf decoders.
- **Security story fits the narrative.** Two processes, isolated
  filesystems under Docker Compose, UDS permissions enforce mutual
  reachability — a compromised parser does not get direct read access
  to the engine's storage.

**Negative — must document**

- **One more deployable.** Docker Compose gains a `rampart-native`
  service; `depends_on.service_healthy` + shared volume for the UDS
  socket. Demo-stack (Adım 7) complexity grows one box.
- **IPC overhead is load-bearing at every size we measured.** Small
  lockfiles eat the ~50 µs UDS handshake; large ones eat the base64
  + JSON-envelope round-trip. Go wins 3–9× across the full fixture
  matrix (`docs/benchmarks/sbom-parser.md`). The strategy knob lets
  operators keep the Go path — which is today the right default on
  every workload this benchmark covers.
- **Platform scope: Linux + macOS only.** `tokio::net::UnixListener`
  and Go's `net.Listen("unix", …)` both require Unix. Windows clients
  route to named pipes (`\\.\pipe\...`) — different wire primitive,
  different Go stdlib surface. Phase 2 will add named-pipe support or
  accept that rampart's CLI-on-Windows works against a remote HTTP
  engine instead of a local UDS one. The fixture symlink
  (`native/testdata → ../engine/testdata`) also requires symlink-aware
  git checkout, which git-for-windows honours only with the
  `core.symlinks=true` setting (https://git-scm.com/docs/git-config#Documentation/git-config.txt-coresymlinks).
  Phase 2 concern; not blocking Adım 6.

**Neutral**

- **Frame size cap: 100 MiB.** Enforced on both sides; matches the
  largest benchmark fixture we anticipate. Hard error if exceeded.
- **Serde JSON, not simd-json / sonic.** Stock `serde_json` keeps the
  parity test's byte-for-byte promise easy to reason about. Performance
  headroom is available in Phase 3 if we find parse time dominates
  real workloads — currently it doesn't (see benchmarks).

## Alternatives considered — expanded

1. **cgo + static libnative.a.** Would remove the out-of-process hop
   but adds cross-compile overhead and cgo's runtime cost (goroutine
   pinned to OS thread during the call; std Go runtime docs at
   https://pkg.go.dev/cmd/cgo#hdr-Go_references_to_C are explicit about
   this). The serialisation we save is paid back in scheduler overhead
   on any multi-tenant engine.
2. **Embedded Rust via wazero (WebAssembly).** Rejected — WASM stops us
   from using `tokio::net` for UDS framing and forces another
   serialisation round-trip (host ↔ guest). Benchmarks elsewhere
   (bytecodealliance/wasmtime#6378 discussions) show that for
   bulk-parse workloads the sandbox boundary dominates.
3. **Pure Go with `github.com/bytedance/sonic`.** Relies on JIT codegen
   on x86-64 (project README at https://github.com/bytedance/sonic
   lists supported architectures); our arm64 Linux deploy target does
   not fall in the fast path.

## Measured consequences (2026-04-20, Adım 6 close, two wire revisions)

Benchmarks across a 1 KiB → 15 MiB range (7 fixtures, 2 → 100 000
packages; full table in `docs/benchmarks/sbom-parser.md`).

### First revision — JSON envelope (base64 request + JSON response)

- **The sidecar did not cross over in the tested range.** Go won at
  every input size — from 8.7× on the 765-byte axios fixture down to
  3.1× on the 15 MiB, 100 000-package fixture.
- **Native throughput plateaued at ~47 MB/s** past the small-fixture
  regime. Bottleneck: four JSON/base64 boundaries in the protocol
  (Go base64 encode → UDS → Rust base64 decode → serde_json parse →
  serde_json marshal of response → Go JSON unmarshal).

### Second revision — binary request envelope (this ADR's current wire)

Eliminated base64 + JSON-envelope on the request path. Kept JSON on
the (small) response. Details in `schemas/native-ipc.md`.

- **Native throughput improved from ~47 MB/s to ~82 MB/s** (~75 %
  gain) at steady state.
- **Go/Native ratio at 100 k packages dropped from 3.1× to 1.8×.**
- **Still no crossover.** Go holds ~150 MB/s; Native's new plateau
  at ~82 MB/s is a real improvement but not enough.

What's left as cost on the Native path: one UDS round-trip, plus the
response-side marshal (Rust `serde_json::to_vec` of ParsedSBOM) and
unmarshal (Go `encoding/json` into `domain.ParsedSBOM`). This
pencils out to ~2 µs/package of extra work the in-process Go path
never pays.

### Phase-2 lever status

| Lever | Status | Outcome |
|---|---|---|
| Binary wire envelope (this revision) | **Tested** | ~1.75× Native speed-up; still no crossover. |
| Connection pooling / pipelining | **Not yet tested** | Amortises the ~50 µs UDS connect cost. Helps small fixtures where that cost is visible; irrelevant past ~1 KiB lockfiles. Not the lever that closes the 1.8× steady-state gap. |
| `SCM_RIGHTS` + mmap handoff + binary response (FlatBuffers / bincode) | **Not yet tested** | The only remaining lever that could realistically close the gap — deletes both the response-JSON round-trip and the input copy. Two-day refactor minimum; deferred to Phase 2. |

### Operational consequence

`parser.strategy: go` remains **measurably the right default**. The
sidecar stays because its non-performance properties still hold:

- Zero cgo in the Go build (every rampart-supported target builds
  with `CGO_ENABLED=0`).
- Independent release cadence between engine and parser.
- Wire-level debuggability (`strace` / `tcpdump` show request opcodes
  + raw lockfile bytes + JSON SBOM — no protobuf decoders needed).
- Process-level isolation from the engine's storage layer.

If the remaining Phase-2 lever is measured and still doesn't produce
a crossover, the honest action is to delete `rampart-native` from the
default Docker Compose profile and keep it only as an opt-in for
deployments that value the isolation properties above.

## Verification

At Adım 6 close:

- `cargo test --workspace` — 16 unit tests green (parser / protocol / ipc).
- `go test -run TestParserParity ./engine/sbom/npm/...` — 5 valid fixtures
  × 2 parsers → byte-identical canonical JSON.
- `go test -run TestParserParity_Errors` — malformed + v2 lockfiles
  surface the same sentinel error class on each side.
- `docker compose --profile native up engine rampart-native` — both
  services report `healthy`; engine answers /readyz over HTTP, native
  answers `ping` over UDS.
- Real command chain:
  `engine parse-sbom --parser native engine/testdata/lockfiles/axios-compromise.json`
  prints the same SBOM as `--parser go`, with a stderr log line naming
  `strategy=native socket=/tmp/rampart-native.sock`.
