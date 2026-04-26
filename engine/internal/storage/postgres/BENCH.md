# Postgres backend benchmarks

Run with:

```shell
cd engine && go test -run=NONE -bench=BenchmarkPostgres \
  -benchtime=500x -count=1 ./internal/storage/postgres/...
```

The benchmarks spin up a single `postgres:16-alpine` container via
testcontainers (~5 s amortised across the four cases), then each
benchmark provisions a dedicated database on the shared container so
numbers don't carry row-count residue from a previous case.

`p50 / p95 / p99` are in microseconds, attached as custom metrics via
`b.ReportMetric`. `ns/op` is Go's default mean-per-op, preserved so
`benchstat` can be run on the output.

## Typical results

Apple M3 Pro, Docker Desktop 4.30, Postgres 16-alpine, 500 iterations:

| Bench | ns/op | p50 | p95 | p99 |
|---|---|---|---|---|
| `UpsertSBOM`             | 751253 | 735 µs  | 920 µs  | 978 µs  |
| `ListSBOMsByComponent`   | 1609625 | 1520 µs | 2001 µs | 3936 µs |
| `UpsertIncident`         | 172708 | 163 µs  | 229 µs  | 290 µs  |
| `ListIncidents`          | 184341 | 171 µs  | 239 µs  | 308 µs  |

Fleet shape used by the read-side benchmarks:

- `ListSBOMsByComponent` seeds 100 components × 10 SBOMs (1000 rows),
  with 2 packages per SBOM; each list call hydrates 20 package rows.
- `ListIncidents` seeds 100 incidents without remediations.

## Acceptance target

The v0.2.0 acceptance criterion is `p95 < 50 ms` for the hot-path
read queries at this fleet size. All four benches clear that by an
order of magnitude; the tails we actually watch in production will
show up once multi-repo aggregation (Theme H) and publisher-history
ingest (Theme F) hit the same store.
