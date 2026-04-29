# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.1] — 2026-04-28

Performance + multi-ecosystem release. Closes the deferred items
from v0.2.0's release notes (PyPI + Maven parsers, blast-radius
performance fix), with honest measured deltas against the v0.2.0
load test baseline.

### Performance — matcher + storage hot-path

The v0.2.0 baseline missed two of three performance targets (total
ingest 518s vs 300s target; blast-radius p95 2977ms vs 500ms target).
v0.2.1 closes both gaps with measured margin.

Root cause was N+1 query patterns in two places, not missing indexes:

- `forwardMatch` looped over every component fetching SBOMs one-by-one
  (10k components × 500 IoCs = 5M postgres roundtrips per ingest stage).
- `BlastRadius` re-executed the matcher live on every request, making
  the same 10k roundtrips per call.

Fix landed in PR #47 as two storage methods plus a hybrid handler:

- `MatchedComponentRefsByIoC` — cached lookup against the incidents
  table for already-ingested IoCs (uses existing `incidents_ioc_idx`).
- `ListSBOMPackages` — single JOIN replacing the per-component fetch
  loop (uses existing `sbom_packages_name_version_idx`).
- `BlastRadius` handler routes ingested IoCs to the cache path and
  hypothetical IoCs to the live matcher (preserves what-if semantics
  documented in the handler docstring + integration test).

| Metric | v0.2.0 | v0.2.1 | Target | Status |
|---|---|---|---|---|
| Total ingest | 518s | 86s | <300s | ✅ |
| IoC ingest stage | 376s | 3s | n/a | ~125× |
| Blast-radius p95 | 2977ms | 2.49ms | <500ms | ✅ |
| Incident-detail p95 | 2.97ms | 2.61ms | <200ms | ✅ no regression |
| Incidents opened | 5047 | 5047 | — | correctness preserved |

Single-run measurement against the v0.2.0 baseline corpus (5,047
incidents, 10k components, 500 IoCs, 200 anomalies). Methodology
and EXPLAIN ANALYZE evidence in `docs/performance/v020-load-test.md`.

### Cancelled / skipped from original v0.2.1 plan

- **PR 1 — Postgres index migration: cancelled.** Initial plan
  assumed missing indexes were the bottleneck. Investigation showed
  all required indexes already existed from migration 0004; the
  baseline gap was N+1 query patterns, not missing indexes. PR 1
  would have been cargo-cult work producing no measurable delta.
- **PR 3 — Bulk IoC ingest endpoint + background worker: skipped.**
  PR 2 (above) closed all performance targets with margin. Bulk
  endpoint scope (202 Accepted + status polling + matcher fan-out
  worker) was deferred under YAGNI discipline; revisit when
  multi-repo aggregation (v0.3.0 Theme H) increases fleet size.

### Multi-ecosystem — Theme C3 + C4

Five ecosystems / eight lockfile dialects now production-validated.
Closes the "PyPI + Maven parsers ship in v0.2.1" deferral from the
v0.2.0 release notes.

PR #48 added:

- **PyPI** — three lockfile formats: `requirements.txt` (extras +
  env markers stripped, VCS/local lines skipped), `poetry.lock`,
  `uv.lock`.
- **Maven / Gradle** — two formats: `pom.xml` (property
  substitution, test scope visibility, unresolved properties kept
  verbatim), `gradle.lockfile`.
- Auto-detect by filename (7 of 8 dialects); cargo's legacy
  `simple.toml` fixture still requires `--ecosystem cargo` flag.

Per ADR-0011 amendment: no new `IoCKind` enums; ecosystem dimension
stays in the existing `Ecosystem` field. Per spec: no Rust sidecar
parity for PyPI / Maven (Wasm scope, v0.5.0+).

### Documentation

- **README v0.2.1 refresh** (PR #49) — three-segment audience
  framing preserved (solo / mid-size team / platform team), 5
  ecosystems documented in a single Features table, factual
  comparison vs Snyk / Socket, performance section linking to the
  honest baseline doc, configuration env-var table, architecture
  diagram refresh, ADR count updated 9 → 11, pre-1.0 status callout.
- ADR count: 9 → 11 (0010 golangci-lint v2 migration, 0011 v0.2.0
  scope commitment + amendments, 0012 auth boundary at engine, 0013
  publisher Snapshot vs Profile split, 0014 anomaly → IoC bridge).

### Dependency hygiene

Merged:

- `BurntSushi/toml` 1.3.2 → 1.6.0 (#50, minor)
- `dtolnay/rust-toolchain` SHA refresh (#52)
- Go workspace 1.25.0 → 1.25.7 + `goose/v3` 3.27.0 → 3.27.1 (#59,
  manual replacement of #54 — dependabot can't bump `go.work`
  directives)

Deferred to v0.2.2:

- 5 GitHub Action major bumps (`cosign-installer` 3 → 4,
  `docker/login-action` 3 → 4, `actions/checkout` 4 → 6,
  `goreleaser-action` 6 → 7, `dtolnay/rust-toolchain` toml 0.x → 1.x)
  — release.yml core surface, requires release engineering session.
- `kin-openapi` 0.135 → 0.137 (#55) — codegen-only transitive,
  zero functional value, would add new transitive supply-chain
  dep; revisit when oapi-codegen requires it.

### Known issues

- **Backstage transitive security alerts:** `uuid <14.0.0`
  (GHSA-w5hq-g745-h8pq) and `fast-xml-parser <5.7.0`
  (GHSA-gh4j-gqv2-49f6) appear in `yarn.lock` via transitive
  Backstage dependencies. Both vulnerable + safe versions present;
  pinned by downstream consumers. Resolution requires
  `package.json` resolutions block + Backstage e2e validation
  (the MUI 9 / Recharts breakage pattern); deferred to v0.2.2.
- **TestPostgresContract flake:** Postgres testcontainers
  shared-DB counter race produces intermittent
  `database "rampart_contract_<pid>_N" does not exist` failures
  on CI. Re-run passes deterministically. Fix (per-test DB
  isolation) tracked for v0.2.2.

### Migration

No breaking changes from v0.2.0. New ecosystems (PyPI, Maven) are
additive; existing npm / gomod / cargo flows unchanged.

If you bump rampart from v0.2.0 → v0.2.1:

- `go.work` and `go.mod` workspaces require Go 1.25.7+ if you
  build from source.
- No database migrations required (existing migrations 0001-0007
  apply unchanged; v0.2.1 adds zero new migrations).
- No API contract changes (`schemas/openapi.yaml` unchanged).
- No configuration changes (existing `RAMPART_*` env vars work
  as-is).

## [0.2.0] — 2026-04-27

Major feature release. Six themes ship together: production-grade
security, Backstage architecture completion, multi-ecosystem parser
expansion (Go + Cargo), Postgres storage, frontend depth, and
publisher-anomaly detection.

### Added

**Production security posture (Theme A)**

- JWT authentication middleware on `/v1/*` routes with HS256/RS256
  signing, scope-based authorization (`read` / `write` / `admin`),
  and token issuance via `POST /v1/auth/token`. Default off
  (`RAMPART_AUTH_ENABLED=false`) for backward compat — operators
  opt in per deployment.
- Env-configured CORS allow-list (`RAMPART_CORS_ORIGINS`). Default
  deny-all; v0.1.x permissive wildcard removed. Dev convenience
  flag `RAMPART_CORS_ALLOW_ALL=true` warns in logs.

**Backstage architecture completion (Theme B)**

- Frontend client routes through the Backstage backend proxy
  (relative URLs). `rampart.baseUrl` config removed; `backend.baseUrl`
  + `/api/rampart/v1` is the new pattern.
- `CatalogSync` real push: tick loop pushes Backstage `Component`
  entities to the engine with topological ordering and a client-side
  memo for idempotency.
- Service-to-service auth between Backstage backend and engine via
  static JWT token (`RAMPART_ENGINE_AUTH_TOKEN`). See ADR-0012 for
  the auth-boundary-at-engine architecture.
- "No CORS config needed" install posture documented in
  `docs/operations/deployment-patterns.md`.

**Multi-ecosystem parsers (Theme C)**

- Go modules parser (`engine/sbom/gomod`) — `go.sum` + `go.mod`
  parsing, `replace` directive handling, pseudo-version
  normalisation.
- Cargo parser (`engine/sbom/cargo`) — `Cargo.lock` TOML, workspace-
  aware, registry-vs-git source distinction.
- CLI auto-detects ecosystem from filename (`go.sum` → gomod,
  `Cargo.lock` → cargo, `package-lock.json` → npm).
- Rust sidecar parity for both ecosystems via existing parity-test
  harness.
- PyPI + Maven parsers ship in v0.2.1 (Theme C3+C4).

**Postgres storage backend (Theme D)**

- `engine/internal/storage/postgres` package implements the
  `Storage` interface against `pgx` + goose migrations.
- Seven migration files (`components`, `sboms`, `iocs`, `incidents`,
  `publishers`, `publisher_history`, `anomalies`).
- `docker-compose.yml` includes `postgres:16-alpine` service with
  healthcheck. Default backend; in-memory mode preserved via
  `RAMPART_STORAGE=memory` for development.
- Shared `storagetest` contract suite ensures interface parity
  between backends.

**Frontend depth (Theme E)**

- Incident detail drawer — MUI `Drawer` with timeline, matched IoC,
  affected components, and remediation log panels. New
  `GET /v1/incidents/{id}/detail` joined endpoint.
- Blast-radius graph visualisation — `reactflow` + `dagre` auto-
  layout, red/orange/grey color encoding, double-click re-root.
- Search + filter toolbar — multi-state, multi-ecosystem, date-
  range, search substring, owner. URL state via `useSearchParams`
  for bookmarkable views.
- `IncidentDashboard` list-view filter expansion (multi-state,
  ecosystem array, time-range, search, owner) on
  `GET /v1/incidents`.

**Publisher-anomaly detector (Theme F)**

- Publisher graph ingestion (F1): npm registry + GitHub API
  ingestors with rate-limit awareness, retry/backoff, structured
  logging. Publisher history persisted in `publisher_history`
  table. Cron tick configurable via
  `RAMPART_PUBLISHER_REFRESH_INTERVAL` (default 1h, default OFF).
- Three anomaly detectors (F2): maintainer email drift, OIDC
  publishing regression, version jump. Confidence levels
  (`high` / `medium` / `low`) with explicit thresholds. Pre-1.0 +
  pre-release semver edge cases handled.
- Anomaly orchestrator emits IoCs via the new `IoCBodyAnomaly` body
  variant (F3 + ADR-0014). Anomalies flow through the standard
  incident workflow: detect → IoC emit → matcher dispatch →
  incident open.
- Default off (`RAMPART_PUBLISHER_ENABLED`,
  `RAMPART_ANOMALY_ENABLED`).

### Performance baseline

Honest baseline measured on M3 Pro / `postgres:16-alpine` (see
[`docs/performance/v020-load-test.md`](docs/performance/v020-load-test.md)
for the harness, methodology, and the 3-run table):

- Total ingest 518 s (vs 300 s target — miss ~1.7×)
- Blast-radius p95 2 977 ms (vs 500 ms target — miss ~6×)
- Incident-detail p95 2.97 ms (vs 200 ms target — pass with 67×
  headroom)

Both ingest and blast-radius misses share a single root cause: the
matcher's forward-match runs synchronously on the request path
instead of joining against the cached `incidents` table. The
`incident-detail` endpoint *is* a JOIN against the indexed table —
its 67× headroom confirms the schema design is sound and isolates
the bottleneck to matcher live-execution.

v0.2.1 targets the fix (cache + bulk endpoint + index migration);
the same load-test harness will measure the before/after delta on
the same fixture corpus.

### Changed

- `IncidentDashboard` row click opens the detail drawer (was: no
  detail surface in v0.1.x).
- `RampartClient` defaults to relative URLs against the Backstage
  backend; explicit engine URL override only for non-Backstage
  deployments.
- `make demo-axios` includes the Postgres service; in-memory mode
  is no longer the default but preserved as `RAMPART_STORAGE=memory`.

### Deprecated

- `rampart.baseUrl` config in Backstage `app-config.yaml` —
  superseded by `backend.baseUrl` + `/api/rampart/v1` relative URL
  routing. Targeted for removal in v1.0.

### Removed

- v0.1.x permissive CORS wildcard. Operators must explicitly
  configure `RAMPART_CORS_ORIGINS` or run behind the Backstage
  proxy.

### Migration

See [`docs/migration/v0.1.x-to-v0.2.0.md`](docs/migration/v0.1.x-to-v0.2.0.md)
for the upgrade guide. Three required actions: provision Postgres
(or set `RAMPART_STORAGE=memory`), configure JWT signing if auth
enabled, update Backstage plugin config.

### Architecture decision records

New ADRs documenting v0.2.0 design decisions:

- [ADR-0011](docs/decisions/0011-v020-scope-commitment.md) — v0.2.0
  scope commitment (six-theme major release) plus amendments for
  the Theme C `IoCKind` ecosystem-naming deviation and the Theme F2
  IoC-emission deferral.
- [ADR-0012](docs/decisions/0012-auth-boundary-at-engine.md) —
  auth boundary at the engine, not the Backstage proxy.
- [ADR-0013](docs/decisions/0013-publisher-domain-split.md) —
  publisher domain `Snapshot` vs `Profile` split.
- [ADR-0014](docs/decisions/0014-anomaly-ioc-bridge.md) — anomaly →
  IoC bridge (`IoCBodyAnomaly` variant).

### Deferred

- **Theme A3** — Backstage guest auth → real OAuth provider.
  Requires a GitHub OAuth App registration; ships in v0.2.1 with a
  setup guide.
- **Theme C3+C4** — PyPI + Maven/Gradle parsers. Separate PR;
  targets v0.2.1.
- **Performance fix scope** (matcher cache + bulk ingest + index
  migration). v0.2.1, with measured before/after delta on the
  v0.2.0 baseline harness.

### Known limitations

- BlastRadiusGraph 100-component performance not yet measured —
  Playwright e2e benchmark planned for the v0.10.0+ frontend
  hardening pass.
- Distinct anomaly icon in `IncidentDashboard` list view requires
  N+1 IoC fetch; rendered correctly in the detail drawer; the
  list-view distinguisher is targeted for v0.10.0+.
- MUI 9 + Recharts 3.x are incompatible with Backstage's webpack
  bundle; pinned to MUI 5 / Recharts 2.x. Re-evaluation triggers
  documented in `ROADMAP.md`.

## [0.1.0] — 2026-04-21

Initial public release. Three quickstart paths tested green on the
maintainer's machine; eight CI workflows deterministic green on the
same commit.

### Added

**Quickstart paths.** Three tested-to-work install flows, one per
user segment:

- Solo developer: `go run ./cli/cmd/rampart scan <lockfile>` — CLI
  emits `ParsedSBOM`, `SBOM` (with ULID + timestamp when
  `--component-ref` + `--commit-sha` are supplied), or SARIF 2.1.0.
- Mid-size team: `docker compose up -d engine mock-npm-registry slack-notifier`
  brings the incident engine up with a dry-run Slack subscriber on
  `/v1/stream`. Real webhook delivery behind `SLACK_WEBHOOK_URL`.
- Platform team: `make demo-axios` brings up a five-service stack
  (engine + mock-npm + Slack notifier + Backstage production
  image) and replays the 2026-03-31 axios compromise scenario.
  `make demo-shai-hulud`, `make demo-vercel`, and `make demo-native`
  replay the other scenarios.

**Engine and domain.** Go engine implementing:

- npm `package-lock.json` v3 parser with byte-identical Go and Rust
  implementations (the Rust sidecar is opt-in — see ADR-0005).
- Six first-class domain entities: Component, SBOM, PackageVersion,
  IoC, Incident, Remediation.
- Incident state machine:
  `pending → triaged → acknowledged → remediating → closed`,
  with `dismissed` reachable from any non-terminal state.
- IoC matcher supporting `packageVersion` (exact) and `packageRange`
  (semver via `Masterminds/semver/v3`). `publisherAnomaly` wire
  shape landed; detector implementation on the roadmap.
- Forward matching (IoC publish → scan SBOMs), retroactive matching
  (SBOM ingest → scan IoCs), idempotent by
  `(IoCID, ComponentRef)` pair.
- In-process event bus with five event types (`incident.opened`,
  `incident.transitioned`, `remediation.added`, `sbom.ingested`,
  `ioc.matched`) fanned out over Server-Sent Events on `/v1/stream`.
- Blast-radius query (`POST /v1/blast-radius`) returning affected
  component refs for a given IoC list.
- Remediation audit log, append-only, typed kind enum.
- In-memory storage backend implementing a pluggable `Storage`
  interface. Shared contract test suite at `storagetest/` accepts
  future backends.

**Consumers.**

- CLI (`cli/cmd/rampart`) with `scan` subcommand and text / JSON /
  SARIF formatters.
- GitHub Action (`integrations/github-action/`) wrapping the CLI +
  SARIF upload for code-scanning.
- Slack notifier (`integrations/slack-notifier/`) — SSE consumer
  with dry-run default.
- Pre-commit hook (`integrations/precommit-hook/`) for local
  lockfile validation.
- Mock npm registry (`integrations/mock-npm-registry/`) — demo
  support, serves canned fixtures + the IoC feed scenarios publish.
- Backstage frontend plugin
  (`@ugurcan-aytar/backstage-plugin-rampart`) — `IncidentDashboard`
  and `IncidentDetail` routable extensions.
- Backstage backend plugin
  (`@ugurcan-aytar/backstage-plugin-rampart-backend`) —
  `/api/rampart/v1/*` engine proxy and `CatalogSync` tick loop.
- Backstage `examples/app` — production Backstage container image
  (773 MiB) that renders the rampart UI end-to-end.

**CI/CD.** Ten GitHub Actions workflows:

- `engine.yml`, `native.yml`, `parity.yml`, `gen-check.yml`,
  `backstage.yml`, `integrations.yml`, `codeql.yml`, `e2e.yml` —
  per-PR guards with per-package coverage gates, supply-chain
  audits (cargo-deny, govulncheck advisory, dependency-review),
  and Playwright e2e against the live compose stack.
- `dependency-review.yml` — PR-only license + advisory diff gate.
- `release.yml` — tag-triggered orchestrator: `goreleaser` Go
  binaries + `cargo` cross-compile Rust targets + five multi-arch
  container images on `ghcr.io`. Every artifact cosign-signed via
  GitHub OIDC (keyless) with a syft-generated SPDX SBOM attached
  as a cosign attestation.
- `.github/workflows/*` action refs pinned to commit SHAs via
  `pinact`.

**Documentation.**

- `README.md`, `ARCHITECTURE.md`, `CONTRIBUTING.md`, `SECURITY.md`,
  `DEPS.md`.
- Nine Architecture Decision Records under `docs/decisions/`
  (0005 through 0009 — Rust sidecar via UDS, Yarn 4 vs pnpm,
  parser placement, `enableScripts=false` compatibility, CI/CD
  pipeline architecture).
- `docs/benchmarks/sbom-parser.md` — honest Go vs Rust parser
  throughput measurements across seven lockfile fixtures.
- `schemas/openapi.yaml` — single-source contract; drift is a CI
  failure.
- `schemas/native-ipc.md` — wire protocol for the opt-in Rust
  sidecar.

### Known gaps

Documented in `SECURITY.md`:

- In-memory storage only — engine loses all state on restart.
  Postgres backend is on the roadmap.
- Engine CORS middleware is a permissive wildcard to let the demo
  Backstage frontend hit the engine directly. Production deploys
  should route through the `rampart-backend` proxy and tighten CORS
  accordingly.
- Backstage ships with guest auth enabled for the demo. Swap in a
  real provider (OIDC, GitHub, etc.) before exposing the stack
  outside a trust boundary.
- No first-party auth on `/v1/*` endpoints. Engine assumes it runs
  behind a trusted reverse proxy that enforces auth upstream.
- `govulncheck` is advisory, not gating. New Go stdlib CVEs do not
  block PRs; the workflow summary surfaces them for security
  review.
- Backstage image is 773 MiB. Further slimming risks breaking
  better-sqlite3's native rebuild (ADR-0008 edge case).
- The Rust sidecar does not currently beat the Go parser at any
  tested lockfile size; it ships as an opt-in isolation feature,
  not a throughput win. Phase 2 evaluates `SCM_RIGHTS` + binary
  response to revisit this.
- Backstage plugins (`@ugurcan-aytar/backstage-plugin-rampart`,
  `-rampart-backend`, `-scaffolder-rampart-actions`) are not yet
  published to npm — scheduled for v0.1.1 once npm trusted
  publishing is configured on the registry side. Install via
  workspace path for now; the release container image
  (`ghcr.io/ugurcan-aytar/rampart-backstage:v0.1.0`) bundles them
  pre-built for the `make demo-*` quickstart paths.

### Breaking changes

None — this is the first release.

[0.2.1]: https://github.com/ugurcan-aytar/rampart/releases/tag/v0.2.1
[0.2.0]: https://github.com/ugurcan-aytar/rampart/releases/tag/v0.2.0
[0.1.0]: https://github.com/ugurcan-aytar/rampart/releases/tag/v0.1.0
