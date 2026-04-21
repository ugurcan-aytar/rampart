# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
  container images on `ghcr.io` + npm trusted publishing for the
  three Backstage plugins. Every artifact cosign-signed via
  GitHub OIDC (keyless) with a syft-generated SPDX SBOM attached
  as a cosign attestation.
- `.github/workflows/*` action refs pinned to commit SHAs via
  `pinact`.

**Documentation.**

- `README.md`, `ARCHITECTURE.md`, `ROADMAP.md`, `CONTRIBUTING.md`,
  `SECURITY.md`, `DEPS.md`.
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

Documented in `SECURITY.md` and scoped on the `ROADMAP.md`:

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

### Breaking changes

None — this is the first release.

[0.1.0]: https://github.com/ugurcan-aytar/rampart/releases/tag/v0.1.0
