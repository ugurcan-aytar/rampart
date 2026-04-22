# rampart-native

Rust sidecar for the rampart engine. Parses npm `package-lock.json`
files (lockfileVersion 3) and answers the engine over a Unix Domain
Socket. Byte-identical to the Go parser at
`engine/sbom/npm/parser.go` — every fixture round-trips through the
parity test in `engine/sbom/npm/parity_test.go`.

The sidecar's value is **architectural isolation**: a separate
process, a separate filesystem mount, no cgo in the Go build, an
independent release cadence. It is not a throughput win — the
in-process Go parser wins at every input size measured. The full
table and the methodology live at
[`docs/benchmarks/sbom-parser.md`](../docs/benchmarks/sbom-parser.md);
the trade-off is documented in
[ADR-0005](../docs/decisions/0005-no-cgo-rust-via-uds.md).

Wire protocol: [`schemas/native-ipc.md`](../schemas/native-ipc.md).

## Installation

Pre-built binaries for every release at
`https://github.com/ugurcan-aytar/rampart/releases/latest`
(linux-x86_64-musl, linux-aarch64-musl, macOS x86_64 + aarch64).
Container image at
`ghcr.io/ugurcan-aytar/rampart-native:0.1.0` (multi-arch).

From source:

```bash
cd native
cargo build --workspace --release
```

Produces `target/release/rampart-native` — a single static-ish
binary (system libc + TLS roots are the only dynamic dependencies).

## Run

```bash
# Binds /tmp/rampart-native.sock by default.
./target/release/rampart-native

# Custom socket + JSON logs:
RAMPART_NATIVE_SOCKET=/var/run/rampart/native.sock \
RAMPART_LOG_FORMAT=json \
RUST_LOG=info \
./target/release/rampart-native
```

The process cleans up its socket file on `SIGINT` / `SIGTERM`.

In Docker Compose (opt-in via the `native` profile):

```bash
docker compose --profile native up engine rampart-native
```

The engine speaks the binary-envelope protocol on the UDS and falls
back to the in-process Go parser when the sidecar is unreachable
(failure modes are surfaced as `parser.fallback` events on
`/v1/stream`, never silent).

## Smoke test

```bash
# Terminal 1
./target/release/rampart-native

# Terminal 2 — drive a real round-trip parse through the engine:
go run ./engine/cmd/engine parse-sbom \
    --parser native \
    --native-socket /tmp/rampart-native.sock \
    engine/testdata/lockfiles/axios-compromise.json
```

stderr prints `parse-sbom: strategy=native socket=...
bytes=765`; stdout prints the parsed SBOM as indented JSON.

## Workspace layout

```
native/
├── Cargo.toml                       # workspace manifest, shared deps
├── crates/
│   ├── rampart-native/              # library — parser, protocol, IPC server
│   │   └── src/{lib,parser,protocol,ipc}.rs
│   └── rampart-native-cli/          # thin binary wrapper
│       └── src/main.rs
└── testdata -> ../engine/testdata   # symlink — one fixture set, two parsers
```

The symlink matters: the Go parity test reads from
`engine/testdata/lockfiles/`; Rust-side integration tests read from
`native/testdata/lockfiles/`. Both paths resolve to the same inode.
Linux + macOS only — Windows users need
`git config core.symlinks true` before cloning.

## Test

```bash
cargo test --workspace
cargo clippy --workspace -- -D warnings
cargo fmt --check
```

The CI gates on all three (see `.github/workflows/native.yml`),
plus `cargo audit` and `cargo deny check` against the workspace.

## Dependencies

Runtime crates (workspace-shared, declared in `native/Cargo.toml`):

| Crate | Purpose |
|---|---|
| `serde` | derive-macro serialisation |
| `serde_json` | JSON codec for wire + SBOM |
| `tokio` (current-thread) | UDS listener + per-connection tasks |
| `thiserror` | error enums for `ParseError` |
| `anyhow` | CLI-level error wrapping |
| `tracing` + `tracing-subscriber` | structured logging, JSON formatter opt |
| `base64` | payload encoding |

Every entry is justified in
[`DEPS.md`](../DEPS.md) alongside the Go dependencies.

## Scope

The sidecar today parses npm v3 lockfiles only. It does not perform
IoC matching, trust evaluation, or any state mutation — those stay
engine-side. The sidecar is a stateless parser. Multi-ecosystem
parsing, multi-process pooling, and Windows named-pipe transport
are planned.

## License

MIT — see [LICENSE](../LICENSE).

Source and issues:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
