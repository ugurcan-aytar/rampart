# v0.2.0 load test — performance baseline

Establishes the performance baseline for v0.3.0+ regression
detection. The orchestrator + fixture generator under
[`test/load/`](../../test/load/) are the canonical reproduction.

## Targets (v0.2.0 release gate)

- **`blast-radius p95 < 500 ms`** — the dashboard's incident-detail
  click can spawn a blast-radius lookup; we want the round-trip
  invisible to operators.
- **`incident-detail p95 < 200 ms`** — drawer-open latency budget.
  Joined endpoint introduced in PR #40 / Theme E1.
- **`total ingest < 5 min`** for 10 000 components + 10 000 SBOMs +
  500 IoCs — onboarding a fresh fleet should not be a coffee-break
  exercise.

## Methodology

- **Host**: Apple M3 Pro, darwin/arm64 (macOS 25.3.0), Docker
  Desktop 28.3.2.
- **Engine**: Go 1.25, postgres backend, default
  `RAMPART_DB_MAX_CONNS=10`. No Backstage, no Rust sidecar — only
  `postgres + engine` services from `docker-compose.yml`. Auth
  disabled (`RAMPART_AUTH_ENABLED=false`) to match v0.1.x baseline.
- **Postgres**: `postgres:16-alpine`, default config (no tuning).
  16 GB host RAM available; the engine's 10-conn pool stayed
  comfortably below Postgres' 100-conn default ceiling.
- **Fixture corpus**: 10 000 components × 10 000 SBOMs (one each,
  ≈25 npm packages per SBOM). 500 IoCs split 350 packageVersion +
  100 packageRange + 50 publisher-anomaly variants (Theme F3 /
  ADR-0014). 200 publisher snapshots.
- **Sampling**: 1 000 blast-radius calls (random IoC payload per
  call), 200 incident-detail calls (random incident ID per call).
- **Variance control**: 3 cold-cache runs (engine + postgres
  restarted between runs); table below reports the median run.
  Per-run JSON kept locally in `test/load/results.json` for diff'ing.

## Results (median of 3 runs)

> **Run history**: this section is populated by the load_test.sh
> orchestrator on each release-gate run. Numbers below are from the
> v0.2.0 release gate (2026-04-27).

| Metric | Target | Run 1 | Run 2 | Run 3 | Median | Pass? |
|---|---|---|---|---|---|---|
| Components ingest | n/a | 26 s | 25 s | 27 s | 26 s | — |
| SBOMs ingest | n/a | 137 s | 112 s | 115 s | 115 s | — |
| IoCs ingest | n/a | 427 s | 355 s | 376 s | 376 s | — |
| **Total ingest** | **< 300 s** | 590 s | 492 s | 518 s | **518 s** | ❌ |
| Incidents opened | n/a (sanity) | 5 047 | 5 047 | 5 047 | 5 047 | — |
| **Blast-radius p95** | **< 500 ms** | 3 445 ms | 2 977 ms | 2 829 ms | **2 977 ms** | ❌ |
| Blast-radius p99 | n/a | 4 197 ms | 3 306 ms | 3 500 ms | 3 500 ms | — |
| **Incident-detail p95** | **< 200 ms** | 1.80 ms | 2.97 ms | 3.04 ms | **2.97 ms** | ✅ |
| Incident-detail p99 | n/a | 5.40 ms | 3.54 ms | 3.66 ms | 3.66 ms | — |

> Blast-radius sample completion across the three runs: 929 / 908 /
> 885 of 1 000 attempts. The 7–11 % shortfall is samples that hit
> the harness' 30 s `curl --max-time` cap; the percentile numbers
> above are over the completed samples and therefore *under-state*
> the true tail. Treat the p95/p99 as a lower bound, not a ceiling.

### Verdict — v0.2.0 ships with two missed targets

- **Ingest target missed by ~1.7×** (518 s median vs 300 s target).
  The IoC ingest stage alone is 376 s — every IoC submission
  forward-matches against all 10 000 SBOMs synchronously, with no
  batched insert path. Components (26 s) and SBOMs (115 s) are well
  inside budget; the matcher is the dominant cost.
- **Blast-radius target missed by ~6×** (2 977 ms p95 vs 500 ms
  target). The endpoint re-runs the same forward-match sweep on
  every call instead of joining against the already-computed
  `incidents` table. Not an I/O problem — see the next bullet.
- **Incident-detail comfortably inside budget** (2.97 ms p95 vs
  200 ms target — 67× headroom). This endpoint *is* a JOIN against
  the indexed `incidents` table; it confirms Postgres I/O and the
  Theme E1 schema design are not the bottleneck. The matcher's
  live-execution path is.

The fixture corpus is realistic — 50 IoC-target packages × 5 %
injection rate produces ~10 affected components per IoC (in line
with mid-severity GHSA advisories). Earlier iterations of the
generator picked 10 targets at 30 % injection, which produced 270
matches per IoC and 135 000 incidents — pathological for the matcher
and unrepresentative of real fleets. The current numbers reflect a
realistic workload, not a synthetic worst case.

### v0.2.1 perf-fix scope (separate PR)

The two missed targets share a single root cause: the matcher's
forward-match runs synchronously on the request path (both at IoC
ingest and at blast-radius query). v0.2.1 will:

1. **Cache IoC → affected-components mapping** at IoC ingest time
   (already partly populated via the `incidents` write); rewrite
   `POST /v1/blast-radius` to JOIN `incidents` by `ioc_id` instead
   of re-running the matcher. Expected: blast-radius p95 drops to
   the same order of magnitude as `incident-detail` (single-digit
   ms — both are JOINs over the same indexed table).
2. **Bulk IoC ingest endpoint** (`POST /v1/iocs:bulk`) so a 500-IoC
   submission is one transaction, not 500. Combined with moving
   the matcher fan-out to a background worker (returning 202
   Accepted with a job ID), per-IoC latency stops gating ingest
   wall-clock.
3. **Index on `incidents(ioc_id)` + `incidents(component_ref)`** to
   keep the new JOIN paths within the latency budget under load.

This load-test infra (script + fixture generator + this doc) is
the regression harness for the v0.2.1 work — same corpus, same
host, before/after numbers go in the same table.

## Reproducing locally

```bash
# Cold start
docker system prune -f --volumes  # optional — only if the host is dirty

# Run (regenerates fixtures if absent or size mismatch)
bash test/load/load_test.sh
```

For a smaller smoke that validates the script without spending
~25 minutes on the full corpus (8–10 min ingest plus ~15 min
blast-radius sampling at the current matcher latency):

```bash
bash test/load/load_test.sh --components 200 --iocs 50 \
                            --blast-samples 50 --detail-samples 20 \
                            --workers 5
```

The orchestrator exits 2 if any target misses; CI consumers (or a
release-gate script) can rely on the exit code.

## Why not a CI lane

This load test is **not wired into per-PR CI**. Reasons:

1. **Wall-clock cost.** A full run on the maintainer's M3 Pro is
   ~25 minutes; on a GitHub-hosted runner (slower disk, shared
   CPU) closer to 40+ — per-PR feedback time stays cheap by
   leaving this to release-gate.
2. **Variance.** Cloud runners share noisy neighbours; the same
   corpus produces noticeably different p99s across runs. The
   3-run median plus a known host hardware spec is the only way
   to make these numbers comparable across releases.
3. **Resource spike.** 10k SBOM ingest pushes the engine pool to
   saturation; running this on every PR's CI agent would slow the
   queue noticeably.

The release-gate flow runs `bash test/load/load_test.sh` 3× on the
maintainer's local hardware before tagging `v0.x.0` and updates
this doc with the median. v0.3.0 baselines will reuse this same
script + corpus generator, so regression deltas will be apples to
apples.

## Known limitations

- **No publisher-anomaly orchestrator timing.** The Theme F1+F2
  scheduler runs against `RAMPART_PUBLISHER_ENABLED=true` and hits
  external APIs (npm registry, GitHub) — not reproducible without
  network access + a real fixture set. The 200 synthetic snapshots
  in the corpus only exercise the post-detection persistence path,
  not the live-fetch path. End-to-end orchestrator timing lives in
  the v0.3.0 publisher-graph load test.
- **Single-node Postgres.** v0.2.0 ships the single-node deployment;
  multi-node Postgres + read replicas is in the v0.10.0 candidate
  themes (per ROADMAP.md).
- **No matcher concurrency stress.** The orchestrator submits IoCs
  with 4 parallel workers; the matcher's per-IoC fanout against
  10k SBOMs is the dominant cost. A higher-parallelism IoC ingest
  would surface different p99 numbers; left to a v0.3.0 deeper
  perf pass.

## Provenance

- Fixture generator: `test/load/generate_fixtures.go`
- Orchestrator: `test/load/load_test.sh`
- Per-run output schema: see `results.json` example in
  `test/load/README.md`.
