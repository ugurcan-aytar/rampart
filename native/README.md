# rampart-native

Rust sidecar for the rampart engine. Parses large npm lockfiles and
answers over a Unix Domain Socket. Byte-identical to the Go parser at
`engine/sbom/npm/parser.go` — every valid fixture round-trips through
the parity test in `engine/sbom/npm/parity_test.go`.

Architecture: ADR-0005 — `docs/decisions/0005-no-cgo-rust-via-uds.md`.
Wire protocol: `schemas/native-ipc.md`.

## Build

```bash
cd native
cargo build --workspace                  # debug
cargo build --workspace --release        # distribution
```

Produces `target/release/rampart-native` — a single static-ish binary
(with the system libc + TLS certs as the only dynamic deps).

Lints the project ships clean under:

```bash
cargo test --workspace
cargo clippy --workspace -- -D warnings
cargo fmt --check
```

## Run

```bash
# Binds /tmp/rampart-native.sock by default. Override with env.
./target/release/rampart-native

# Custom socket + JSON logs:
RAMPART_NATIVE_SOCKET=/var/run/rampart/native.sock \
RAMPART_LOG_FORMAT=json \
RUST_LOG=info \
./target/release/rampart-native
```

Stop with SIGINT or SIGTERM — the process cleans up its socket file on
exit.

## Smoke test

```bash
# Terminal 1:
./target/release/rampart-native

# Terminal 2 — Ping over UDS, via socat:
printf '%s\n' '{"id":"p1","type":"ping"}' | socat - UNIX-CONNECT:/tmp/rampart-native.sock
# (if you have python3, a length-prefix helper is in scripts/native-ping.py)
```

For a real round-trip parse, drive through the Go engine:

```bash
# With the server running in another terminal:
go run ./engine/cmd/engine parse-sbom \
    --parser native \
    --native-socket /tmp/rampart-native.sock \
    engine/testdata/lockfiles/axios-compromise.json
```

stderr prints `parse-sbom: strategy=native socket=/tmp/rampart-native.sock bytes=765`,
stdout prints the parsed SBOM as indented JSON.

## Workspace layout

```
native/
├── Cargo.toml                       # workspace manifest, shared deps
├── crates/
│   ├── rampart-native/              # library — parser, protocol, ipc server
│   │   └── src/{lib,parser,protocol,ipc}.rs
│   └── rampart-native-cli/          # thin binary wrapper
│       └── src/main.rs
└── testdata -> ../engine/testdata   # symlink — one fixture set, two parsers
```

The symlink matters: the Go parity test reads from
`engine/testdata/lockfiles/` while Rust-side integration tests (Phase 2)
can read from `native/testdata/lockfiles/`. Both paths resolve to the
same inode. Linux + macOS only — Windows users need
`git config core.symlinks true` before cloning.

## Dependencies

Runtime crates (workspace-shared, declared in `native/Cargo.toml`):

| Crate              | Purpose                                                 |
|--------------------|---------------------------------------------------------|
| `serde`            | derive-macro serialisation (https://serde.rs)           |
| `serde_json`       | JSON codec for wire + SBOM (not simd-json — see ADR-0005) |
| `tokio` (current-thread) | UDS listener + per-connection tasks               |
| `thiserror`        | error enums for ParseError                              |
| `anyhow`           | CLI-level error wrapping                                |
| `tracing` + `tracing-subscriber` | structured logging, json formatter opt |
| `base64`           | payload encoding                                        |

Every entry is justified in `DEPS.md` alongside the Go deps.

## What this crate does NOT do

- Multiple ecosystems — only `parse_npm_lockfile` today. pypi / cargo /
  go parsers land in Phase 3.
- Multi-process pool — single-threaded tokio runtime. Phase 2 lever.
- Windows named-pipe transport — Phase 2; see ADR-0005 consequences.
- IoC matching / trust evaluation — all stays engine-side. The sidecar
  is a stateless parser.
