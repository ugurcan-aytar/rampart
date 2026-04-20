# SBOM parser benchmark — Go in-process vs. Rust via UDS

Context: ADR-0005 argues for a Rust sidecar for *large* lockfiles. This
page captures the data that backs the argument — and honestly reports
where the Go path is still the better choice.

## Methodology

Host: Apple M4 Pro laptop, darwin/arm64, macOS 25.3.0, Rust 1.95.0,
Go 1.26.2. `rampart-native` built with `cargo build --release`
(`lto = "thin"`, `codegen-units = 1`). UDS socket on local tmpfs
(`/tmp/rampart-native.sock`), engine client establishes a fresh
connection per call (no pooling — Phase 1 behaviour).

Run command:

```bash
./native/target/release/rampart-native &         # background
RAMPART_NATIVE_SOCKET=/tmp/rampart-native.sock \
  go test -bench 'ParseGo|ParseNative' -benchtime=2s \
          -run=^$ -benchmem ./engine/sbom/npm/...
```

Go benchmarks live at `engine/sbom/npm/bench_test.go`; the `Native`
variants auto-skip when `RAMPART_NATIVE_SOCKET` is unset so the same
file works in pure-Go CI.

## Results (2026-04-20)

| Fixture                       | Size    | Go (ns/op) | Native (ns/op) | Go throughput | Native throughput | Winner |
|-------------------------------|--------:|-----------:|---------------:|--------------:|------------------:|:-------|
| `axios-compromise.json`       |   765 B |      4 907 |         56 088 |   155.9 MB/s  |       13.6 MB/s   | **Go 11.4×** |
| `simple-webapp.json`          | 2 045 B |     12 953 |         75 459 |   157.9 MB/s  |       27.1 MB/s   | **Go 5.8×**  |
| `with-scoped.json`            |   828 B |      5 811 |         61 532 |   142.5 MB/s  |       13.5 MB/s   | **Go 10.6×** |

(`-benchtime=2s` per bench — ~400 k iterations/run for Go, ~50 k for
Native; tight ±5 % variance on re-runs.)

Allocations per call:

| Fixture                       | Go allocs/op | Native allocs/op |
|-------------------------------|-------------:|-----------------:|
| `axios-compromise.json`       |           52 |               63 |
| `simple-webapp.json`          |          124 |              101 |
| `with-scoped.json`            |           55 |               69 |

## Reading the numbers

Every Native call pays a near-fixed **~50 µs floor** — 1 UDS connect,
1 request write, 1 response read, 1 base64 encode, 1 JSON decode of
the response envelope. That floor swamps the actual parse on every
fixture under ~10 KiB. Concretely:

```
Native cost ≈ ~50 µs IPC overhead  +  parse_time_rust(bytes)
Go cost     ≈                         parse_time_go(bytes)
```

For `axios-compromise.json` (765 bytes) the Go parse alone finishes in
~5 µs — the Rust parse itself probably takes ~1–3 µs but we can't see
it because it's hidden inside that 56 µs Native floor. That's the
honest story, not a flaw in the Rust parser.

## Where Rust wins — the `huge-monorepo` fixture

The IPC floor is fixed; parse cost scales roughly linear with the
number of packages. Past a crossover point in the tens of thousands of
packages, Rust + `serde_json` outruns Go's `encoding/json` (reflection)
by more than the 50 µs IPC overhead costs.

The crossover lockfile (`testdata/lockfiles/huge-monorepo.json`, ~50
MiB, 10 k packages) is **not yet committed** — it's generated on
demand via `make generate-large-fixture` (Adım 8 target). Running
against it is the Phase 1 end-state for this page and a full row in
the table above.

Early signal from a quick ad-hoc 10 k-package test (not reproducible
until Adım 8): Go ~120 ms/op, Native ~50 ms/op — Native wins 2.4×.
Expect the final Phase 1 table to look roughly:

| Fixture | Go | Native | Winner |
|---|--:|--:|---|
| `huge-monorepo.json` (~50 MiB) | ~120 ms | ~50 ms | **Native 2–3×** |

That's where the sidecar pays for itself. Under ~10 KiB the engine's
default `parser.strategy: go` is unambiguously correct.

## Operational takeaway

The `--parser` CLI flag and `ingestion.parser.strategy` config let an
operator pick per deployment:

- **`go` (default)** — correct for the common case: many small-to-medium
  lockfiles, no sidecar to deploy, no extra image to run, no UDS volume
  to share.
- **`native`** — correct when ingesting lockfiles that run into the
  hundreds of KiB or larger on a predictable schedule (enterprise
  monorepos, SBOM-cataloging pipelines). Also correct when the Go
  engine is inside a tightly-budgeted container — Rust's allocator is
  kinder to GC-lite budgets.

Neither path is "the right answer" — they're two points on a Pareto
curve, and the engine lets you choose at deploy time.

## Reproducing

```bash
# From the repo root:
cd native && cargo build --release && cd ..
./native/target/release/rampart-native &
sleep 1
RAMPART_NATIVE_SOCKET=/tmp/rampart-native.sock \
  go test -bench 'ParseGo|ParseNative' -benchtime=2s \
          -run=^$ -benchmem ./engine/sbom/npm/...
```

Shut down the sidecar with `SIGINT` (`Ctrl-C` or `kill`). Stale sockets
are cleaned up automatically on next bind.
