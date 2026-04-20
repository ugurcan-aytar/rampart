# SBOM parser benchmark — Go in-process vs. Rust via UDS

Context: ADR-0005 argues the Rust sidecar pays for itself on large
lockfiles. This page holds the measurements that tell the real story.
The honest verdict lives at the bottom — and it's not the one the ADR
assumed at Phase 1 start.

## Methodology

Host: Apple M4 Pro, darwin/arm64 (macOS 25.3.0), Rust 1.95.0, Go 1.26.2.
`rampart-native` built with `cargo build --release` (`lto = "thin"`,
`codegen-units = 1`). UDS socket on local tmpfs
(`/tmp/rampart-native.sock`); the engine client opens one connection
per call (no pooling — Phase 1 behaviour).

Run command (from repo root):

```bash
cd native && cargo build --release && cd ..
./native/target/release/rampart-native &                # background
RAMPART_NATIVE_SOCKET=/tmp/rampart-native.sock \
  go test -bench 'ParseGo|ParseNative' -benchtime=2s \
          -run=^$ -benchmem ./engine/sbom/npm/...
```

Benchmarks live at `engine/sbom/npm/bench_test.go`. The `Native`
variants auto-skip when `RAMPART_NATIVE_SOCKET` is unset, so the same
file runs cleanly in pure-Go CI. The large synthetic fixtures come
from `go run ./cmd/gen-lockfile-fixture -packages N -output …` inside
`engine/` — deterministic for a fixed seed (42), not committed (sizes
grow into tens of MiB; see `.gitignore`).

## Results (2026-04-20, full 6-fixture sweep)

| Fixture                     |       Size | Packages |    Go ns/op |    Go MB/s | Native ns/op | Native MB/s | Winner |
|-----------------------------|-----------:|---------:|------------:|-----------:|-------------:|------------:|:-------|
| `axios-compromise.json`     |      765 B |        2 |       4 393 |    174 MB/s|       38 262 |    20 MB/s  | **Go 8.7×** |
| `simple-webapp.json`        |    2 045 B |       ~6 |      11 320 |    180 MB/s|       55 714 |    37 MB/s  | **Go 4.9×** |
| `with-scoped.json`          |      828 B |        3 |       4 683 |    177 MB/s|       40 851 |    20 MB/s  | **Go 8.7×** |
| `medium-200-pkgs.json`      |     32 KiB |      200 |     185 537 |    176 MB/s|      711 885 |    46 MB/s  | **Go 3.8×** |
| `medium-2k-pkgs.json`       |    317 KiB |    2 000 |   2 068 289 |    157 MB/s|    6 508 414 |    50 MB/s  | **Go 3.1×** |
| `large-20k-pkgs.json`       |    3.1 MiB |   20 000 |  21 374 802 |    152 MB/s|   67 867 609 |    48 MB/s  | **Go 3.2×** |
| `huge-100k-pkgs.json`       |     15 MiB |  100 000 | 109 453 131 |    148 MB/s|  342 840 444 |    47 MB/s  | **Go 3.1×** |

(`-benchtime=2s`, tight ±5 % across re-runs; full output is
reproducible by re-running the command above.)

Allocations per call tell the same story:

| Fixture                 | Go B/op  | Native B/op | Go allocs/op | Native allocs/op |
|-------------------------|---------:|------------:|-------------:|-----------------:|
| `medium-2k-pkgs.json`   |  1.7 MiB |     3.7 MiB |       16 052 |           11 191 |
| `large-20k-pkgs.json`   |   15 MiB |      42 MiB |      160 171 |          111 363 |
| `huge-100k-pkgs.json`   |   67 MiB |     212 MiB |      800 560 |          555 960 |

Rust's allocation **count** is lower (fewer objects), but the Rust
call's total **bytes** are ~3× higher because the wire envelope
materialises the whole `ParsedSBOM` as JSON inside the response
payload and the Go client has to decode it back into a domain struct.

## What the numbers mean

Two constants fall out of the table:

1. **Go holds ~150 MB/s** across every size — its stdlib `encoding/json`
   runs at a steady throughput with predictable per-package allocation.
2. **Native plateaus at ~47 MB/s** past the small-fixture regime —
   not a bug in the Rust parser, but the cost of the whole IPC round
   trip: `base64` encode on the Go client, `base64` decode on the
   Rust server, serde_json unmarshal of the lockfile, serde_json
   marshal of the response envelope, and a Go `encoding/json` unmarshal
   of that envelope back into `domain.ParsedSBOM`. Four JSON/base64
   boundaries the in-process Go path doesn't pay.

Normalised per package:

- Go processes ~950 k packages / sec (steady)
- Native processes ~300 k packages / sec (steady)

The Rust parser itself is almost certainly faster than Go's `encoding/json`
(measured in isolation, `serde_json` typically wins); we can't see that
win here because it's swamped by the round-trip cost.

## Verdict

**The Rust sidecar does not cross over within the tested range
(1 KiB → 15 MiB lockfiles, 2 → 100 000 packages).** The Go in-process
parser wins by a factor of **3–9× at every size we measured**. Claiming
otherwise on a portfolio page would be dishonest.

That's the Phase-1 baseline. Three Phase-2 levers exist to change the
picture:

1. **Drop the base64 + JSON envelope on the response.** Stream the
   `ParsedSBOM` as a length-prefixed binary (FlatBuffer, Cap'n Proto,
   or bincode over UDS) so Rust → Go doesn't re-enter JSON on the hot
   path. Expected gain: ~2× (rough, from the allocation table).
2. **Connection pooling + request pipelining.** Amortise the ~50 µs
   fixed UDS handshake across many lockfiles per connection. Helps
   small-fixture throughput but not the 3× steady-state gap.
3. **Zero-copy `content` handoff.** Memory-map the lockfile and pass
   a fd over `SCM_RIGHTS` instead of base64. Closes the input side of
   the four-transition cost.

None of these are Phase 1 work; they all need measurements of their
own. Until one of them lands, the ADR-0005 default of
`parser.strategy: go` is measurably correct at every input size this
benchmark covers.

## Operational takeaway

The `--parser` CLI flag and `ingestion.parser.strategy` config let an
operator pick per deployment:

- **`go` (default)** — the right answer for every workload this
  benchmark exercises.
- **`native`** — the right answer when Phase 2 lands one of the
  levers above, OR when the sidecar's isolation properties (separate
  process, separate FS mount, no shared memory with the engine's
  storage layer) are worth the 3× parse-cost tax. Note ADR-0005's
  security narrative: a compromised parser doesn't get read access
  to the engine's data.

The sidecar is not a performance win at Phase 1. It **is** a
compile-time architectural win (no cgo, independent release cadence,
wire-level debuggability) and a runtime isolation win. The rest is
future measurements.

## Reproducing

```bash
# From the repo root:
cd engine && go run ./cmd/gen-lockfile-fixture \
    -packages 200   -output testdata/lockfiles/medium-200-pkgs.json
go run ./cmd/gen-lockfile-fixture \
    -packages 2000  -output testdata/lockfiles/medium-2k-pkgs.json
go run ./cmd/gen-lockfile-fixture \
    -packages 20000 -output testdata/lockfiles/large-20k-pkgs.json
go run ./cmd/gen-lockfile-fixture \
    -packages 100000 -output testdata/lockfiles/huge-100k-pkgs.json
cd ..

cd native && cargo build --release && cd ..
./native/target/release/rampart-native &
sleep 1
RAMPART_NATIVE_SOCKET=/tmp/rampart-native.sock \
  go test -bench 'ParseGo|ParseNative' -benchtime=2s \
          -run=^$ -benchmem ./engine/sbom/npm/...
```

Shut down the sidecar with `SIGINT` (`Ctrl-C` or `kill`). Stale sockets
are cleaned up automatically on next bind.
