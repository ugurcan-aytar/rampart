# SBOM parser benchmark — Go in-process vs. Rust via UDS

Context: ADR-0005 argues the Rust sidecar pays for itself on large
lockfiles. This page holds two measurement rounds against seven
fixtures — the first (JSON envelope) refuted the perf claim; the
second (binary envelope) closed about half of the gap but the Rust
path still loses on every size. The honest verdict lives at the
bottom.

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

## Results — 7 fixtures × 3 parsers

Both "Native" columns run against the same Rust parser and same
lockfile input; only the wire envelope differs.

- **Native-JSON** (Adım 6 refactor, previous column):
  `{ "content": "<base64>" }` request body, response
  `{ "parsed_sbom": {…}, "stats": {…} }`.
- **Native-Binary** (Adım 6 post-refactor, current column): raw
  lockfile bytes on the request (no base64, no JSON envelope — see
  `schemas/native-ipc.md`), response still JSON (ParsedSBOM only).

| Fixture                     |       Size |   Pkgs | Go ns/op | Go MB/s | Native-JSON ns/op | Native-JSON MB/s | Native-Binary ns/op | Native-Binary MB/s | Go vs. Native-Binary |
|-----------------------------|-----------:|-------:|---------:|--------:|------------------:|-----------------:|--------------------:|-------------------:|:---------------------|
| `axios-compromise.json`     |      765 B |      2 |    4 351 |     176 |            38 262 |               20 |              29 133 |                 26 | **Go 6.7×**          |
| `with-scoped.json`          |      828 B |      3 |    4 679 |     177 |            40 851 |               20 |              31 393 |                 26 | **Go 6.7×**          |
| `simple-webapp.json`        |    2 045 B |      6 |   11 364 |     180 |            55 714 |               37 |              40 070 |                 51 | **Go 3.5×**          |
| `medium-200-pkgs.json`      |     32 KiB |    200 |  186 785 |     174 |           711 885 |               46 |             407 348 |                 80 | **Go 2.2×**          |
| `medium-2k-pkgs.json`       |    317 KiB |  2 000 |  2.03 ms |     160 |           6.51 ms |               50 |             3.83 ms |                 85 | **Go 1.9×**          |
| `large-20k-pkgs.json`       |    3.1 MiB | 20 000 | 20.94 ms |     155 |          67.87 ms |               48 |            38.92 ms |                 83 | **Go 1.9×**          |
| `huge-100k-pkgs.json`       |     15 MiB |100 000 |  110 ms  |     148 |            343 ms |               47 |              199 ms |                 81 | **Go 1.8×**          |

(`-benchtime=2s`, tight ±5 % across re-runs; full output is
reproducible by re-running the command above.)

Normalised per package (steady-state, 100 k fixture):

| Path           | Packages / sec |
|----------------|---------------:|
| Go in-process  |    ~911 k      |
| Native-JSON    |    ~292 k      |
| Native-Binary  |    ~502 k      |

## What the binary envelope changed

- Native steady-state throughput moved from ~47 MB/s → ~82 MB/s
  (~75% improvement).
- Go/Native ratio at 100 k packages dropped from **3.1× → 1.8×**.
- Native per-package rate moved from ~300 k pkg/s → ~500 k pkg/s.

Where the gain came from: the JSON wire paid for (a) base64 encoding
on the Go client, (b) base64 decoding on the Rust server, and (c)
JSON parse of the outer envelope on both sides. The binary wire
deletes all three. What's left as cost: the `serde_json::to_vec`
marshal of the `ParsedSBOM` on the Rust side, the `json.Unmarshal`
on the Go side, one UDS round-trip, and the raw parse.

Why Native-Binary **still** loses by ~1.8× at steady state: Go's
`encoding/json` runs in-process with no handoff; the Native path
still pays a full response-JSON marshal + unmarshal round-trip.
Each ~910 k pkg/s vs. ~500 k pkg/s gap pencils out to roughly
`~2 µs / package` of extra cost in the Native path — which is
exactly the cost of materialising each package twice (once into
Rust's `PackageVersion`, once out via `serde_json`, once into Go's
`domain.PackageVersion` via `encoding/json`).

## Verdict — after two wire revisions

**The Rust sidecar does not cross over within the tested range
(1 KiB → 15 MiB lockfiles, 2 → 100 000 packages), even after the
binary-envelope refactor.** Go wins 6.7× on small fixtures and 1.8×
on the 100 000-package fixture. Improvement from the JSON envelope
is real (Native throughput up 75 %) but insufficient.

Two of ADR-0005's three Phase-2 levers are now tested or ruled out:

| Lever | Status | Outcome |
|---|---|---|
| Binary wire envelope (this change) | **Tested** | ~1.75× Native speed-up; still no crossover. |
| Connection pooling / pipelining | **Not yet tested** | Would amortise UDS connect cost (dominates small fixtures; irrelevant past 1 KiB). Estimated gain: marginal at steady state. |
| `SCM_RIGHTS` + mmap-based content handoff | **Not yet tested** | Kills the remaining response-JSON cost ONLY if paired with a binary response envelope (FlatBuffers, bincode). Two-day refactor at minimum. |

Until one of the remaining levers lands — and measures a genuine
crossover — the ADR-0005 default of `parser.strategy: go` remains
measurably correct at every input size this benchmark covers.

## Operational takeaway

Native sidecar is retained opt-in as a parser isolation feature.
Throughput gap (1.8× at 100k pkgs) does not justify default deployment
complexity; its value lies in defense-in-depth against malicious
lockfile payloads, not performance.

Concrete Phase-1 deployment shape (ADR-0005 Final Decision):

- **Default (`docker compose up`)** — engine only; the embedded Go
  parser is used (`ingestion.parser.strategy: go`). One binary, one
  runtime, no UDS volume to mount.
- **Opt-in (`docker compose --profile native up` +
  `RAMPART_PARSER_STRATEGY=native`)** — engine + `rampart-native`;
  parser runs in a separate process, separate language runtime, with
  no read access to the engine's storage layer. Recommended for
  deployments ingesting SBOMs from untrusted or third-party sources.

When the engine is configured to use `native` but the sidecar is
unreachable, it logs a warn line and falls back to the Go parser —
opt-in hardening must never prevent the engine from answering
requests. See `TestEffectiveStrategy_FallsBackWhenNativeUnavailable`.

## Reproducing

```bash
# From engine/ (fixtures are deterministic for seed=42):
cd engine
go run ./cmd/gen-lockfile-fixture -packages 200    -output testdata/lockfiles/medium-200-pkgs.json
go run ./cmd/gen-lockfile-fixture -packages 2000   -output testdata/lockfiles/medium-2k-pkgs.json
go run ./cmd/gen-lockfile-fixture -packages 20000  -output testdata/lockfiles/large-20k-pkgs.json
go run ./cmd/gen-lockfile-fixture -packages 100000 -output testdata/lockfiles/huge-100k-pkgs.json
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
