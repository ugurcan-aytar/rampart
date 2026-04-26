# ADR-0013: Publisher domain — Snapshot vs Profile split

**Date:** 2026-04-26
**Status:** Accepted
**Authors:** ugurcan-aytar

## Context

Theme F (publisher-anomaly detector) needs publisher metadata to detect three anomaly types: maintainer email drift, OIDC publishing regression, version jump anomaly.

During F1 (publisher graph ingestion) implementation, two distinct domain concerns emerged that initially looked like one:

1. **Per-package time-series**: every refresh tick captures the current publisher state (maintainers, latest version, publish method) for a specific package — `npm:axios`, `gomod:github.com/spf13/cobra`, `cargo:tokio`. Required for "did this package's maintainers change between yesterday and today?" queries.

2. **Per-maintainer aggregate identity**: a maintainer (`ugurcan@example.com`) publishes many packages over time. Required for "this maintainer's email just changed across 47 packages simultaneously" pattern detection (a stronger signal than per-package drift).

The naive design folds these into one "Publisher" table. The first ingestor implementation went down that path before the two concerns separated cleanly.

## Decision

Two separate domain types, two separate storage tables:

- **`domain.PublisherSnapshot`** + `publisher_history` table: package-keyed time-series. Append-only. Indexed by `(package_ref, snapshot_at DESC)` for fast "give me the last N snapshots of npm:axios" queries.
- **`domain.Publisher` / `domain.PublisherProfile`** + `publishers` table (added in Theme D): maintainer-keyed aggregate identity. Mutated as new snapshots arrive. Indexed by maintainer email + GitHub username.

F2 detector logic:
- Reads `PublisherSnapshot` time-series for a package → detects per-package drift.
- Updates `PublisherProfile` from each new snapshot → builds maintainer-aggregate baseline → detects multi-package patterns.

## Alternatives considered

### Alternative 1: Single Publisher table

Fold maintainer aggregate + package time-series into one table with a JSON column.

Rejected:
- Time-series queries become full table scans (no useful index across heterogeneous JSON).
- Two distinct mutation patterns (append-only vs mutated-on-update) fight each other for transaction semantics.
- Multi-package maintainer queries (Anomaly Type 2) require maintainer-keyed access; package-keyed table makes that a 47x package_ref scan.

### Alternative 2: Single table, dual indexes

Same table, separate `(package_ref, ts)` and `(maintainer_email)` indexes.

Rejected:
- Doesn't solve the append-only vs mutated semantic conflict.
- Index bloat: every new snapshot rewrites the maintainer-aggregate index entries.
- Schema becomes confused — what's the row's primary identity?

## Consequences

Positive:
- Each table optimised for its access pattern.
- F2 detector logic stays clean: snapshot reader + profile updater are separate code paths.
- Future "multi-package anomaly across maintainer" detection (Phase 2 / v0.4.0+) is unblocked — the maintainer-keyed table is already there.

Negative:
- Two write paths: every ingestion tick writes a snapshot AND updates relevant profile rows. Atomic in a single transaction (postgres) but adds complexity.
- Storagetest contract surface grew: 3 new methods for snapshot, plus the existing 2 for profile (Theme D).

Neutral:
- F1 ships with snapshot-only writes; profile update logic lands in F2. Profile rows from Theme D remain unused by F1 but available to F2.

## References

- Theme D (PR #36) — added `publishers` table for maintainer-keyed aggregate.
- Theme F1 (PR #38) — added `publisher_history` table for package-keyed time-series.
- ADR-0011 — Theme F scope commitment (anomaly types depend on this split).
