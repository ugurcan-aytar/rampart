# test/load — v0.2.0 load test

Establishes a performance baseline for v0.3.0+ regression detection.

## Layout

```
test/load/
├── generate_fixtures.go   — deterministic fixture writer
├── load_test.sh           — orchestrator (postgres + engine + ingest + sample)
├── README.md              — you are here
├── results.json           — last run's output (gitignored when sensitive)
└── fixtures/              — generated; gitignored
    ├── manifest.json
    ├── components.jsonl
    ├── iocs.jsonl
    ├── snapshots.jsonl
    └── sboms/<idx>.json   — one SBOM per component
```

## What it measures

For each run:

- **Ingest timing**: components, SBOMs, IoCs (each stage timed separately + total).
- **Blast-radius latency**: 1000 random IoC payloads → `POST /v1/blast-radius`. p50 / p95 / p99 / avg.
- **Incident-detail latency**: 200 random incident IDs → `GET /v1/incidents/{id}/detail`. p50 / p95 / p99 / avg.
- **Incidents opened**: total count after ingest (sanity check that the matcher fired).

## Targets (v0.2.0 release gate)

- `blast-radius p95 < 500 ms`
- `incident-detail p95 < 200 ms`
- `total ingest < 5 min` (10k components + 10k SBOMs + 500 IoCs)

The orchestrator exits 2 if any target misses.

## Run

```bash
# Default: 10k components, 500 IoCs, 200 snapshots, 1000 blast samples,
# 200 detail samples, 10 ingest workers
bash test/load/load_test.sh

# Smaller for quick smoke
bash test/load/load_test.sh --components 200 --iocs 50 --workers 5
```

Wipes any prior demo state, brings up `postgres + engine` only (no
Backstage / no native sidecar — keep the variable budget for the
engine itself), runs the full pipeline, writes `results.json`,
prints PASS/FAIL per target, exits.

## Determinism

`generate_fixtures.go` takes a `-seed` flag (default `1`); same seed
→ byte-identical fixtures across runs and machines. The orchestrator
reuses fixtures when `manifest.json` matches the requested
`--components`. Delete `manifest.json` to force regeneration.

## Reproducing the v0.2.0 published numbers

See [`docs/performance/v020-load-test.md`](../../docs/performance/v020-load-test.md)
for the published results, the host hardware, and the methodology
notes. The orchestrator + generator in this directory are the
canonical reproduction.
