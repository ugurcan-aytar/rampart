# rampart

[![engine](https://github.com/ugurcan-aytar/rampart/actions/workflows/engine.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/engine.yml)
[![native](https://github.com/ugurcan-aytar/rampart/actions/workflows/native.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/native.yml)
[![parity](https://github.com/ugurcan-aytar/rampart/actions/workflows/parity.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/parity.yml)
[![e2e](https://github.com/ugurcan-aytar/rampart/actions/workflows/e2e.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/e2e.yml)
[![codeql](https://github.com/ugurcan-aytar/rampart/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/codeql.yml)
[![release](https://github.com/ugurcan-aytar/rampart/actions/workflows/release.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/release.yml)
[![license: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

> A rampart for your supply chain.

`rampart` is an open-source supply chain incident response engine for
five ecosystems — **npm, Go modules, Cargo, PyPI, and Maven/Gradle**.
It ingests Software Bill of Materials snapshots, matches them against
Indicators of Compromise, opens incidents on match, and tells you the
three things that matter when a `shai-hulud`-class event is breaking
on Hacker News at 3 a.m.:

1. **Which services are affected.** Every SBOM is pinned to a Backstage
   component reference; the blast radius is a query, not a hunt.
2. **Who owns them.** Component ownership flows from the catalog; the
   incident routes to the team before a human has to triage.
3. **What's the playbook.** The incident carries an append-only
   remediation log — `pin_version`, `rotate_secret`, `open_pr`,
   `notify`, `dismiss`. Audit-grade, not vibes-grade.

The engine is Go with a Postgres-backed storage layer; an optional
Rust sidecar parses lockfiles over a Unix Domain Socket for
deployments that want an extra layer of parser isolation (see
[ADR-0005](./docs/decisions/0005-no-cgo-rust-via-uds.md)).
Three consumers — a Backstage plugin, a GitHub Action, and a CLI —
speak the same OpenAPI contract; no one imports the engine's Go types
directly.

## Why rampart exists

There's an axe-shaped hole in supply chain tooling. Dependabot raises
PRs, Socket scores packages, CodeQL flags source-code issues. None of
them answer the "is my production affected right now, and by whom" of
a live incident. rampart is the glue that turns a feed entry into a
routed, triageable incident with the blast radius attached.

| Concern | Snyk / Socket | rampart |
|---|---|---|
| Self-hosted | Limited / paid tier | First-class, default |
| Telemetry | Cloud callbacks | Zero (ADR-0011 non-goal) |
| Backstage native | No | Yes — first-class plugin |
| Storage | Cloud DB | Local Postgres (or in-memory for dev) |
| Coverage | Source + supply chain | Supply-chain incident lifecycle |
| Cost | Per-dev SaaS | OSS + self-hosted infra |

Three user segments, three different shapes of the pain:

### Solo developer
Running `npm audit` weekly isn't enough when Shai-Hulud worms itself
across 187 packages in hours. You want to know at dinner that one of
your open-source projects pulled a compromised transitive, without
paying for an enterprise seat.

### Mid-size team (10–50 engineers, no platform org)
Dependabot covers the weekly grind; Socket or Semgrep cover the
per-package score. Nothing correlates "SBOM × IoC feed → who owns
what, who's on call, how's the incident tracked". rampart runs as a
self-hosted daemon; Slack gets the event, the team gets the incident.

### Platform team (Backstage-equipped or ready to adopt)
Your blast-radius lives in three places: SBOMs in one pipeline, owner
mappings in the catalog, incident history in PagerDuty. The rampart
Backstage plugin pulls all three into one `IncidentDashboard` inside
the developer portal your engineers already open fifty times a day.

## Quickstart — pick your path

Each path runs in under 5 minutes on a fresh checkout with Go 1.25
and Docker installed.

### Path 1 — Solo developer (CLI)

Scan a lockfile from the terminal. No daemon, no services, just a
binary. Auto-detects the ecosystem from the filename:

```bash
git clone https://github.com/ugurcan-aytar/rampart
cd rampart
go run ./cli/cmd/rampart scan path/to/your/package-lock.json     # or go.sum, Cargo.lock, requirements.txt, poetry.lock, uv.lock, pom.xml, gradle.lockfile
```

Pre-built binaries (linux / darwin / windows × amd64 / arm64) are
attached to every [GitHub Release](https://github.com/ugurcan-aytar/rampart/releases),
cosign-signed against the GitHub OIDC issuer. See [SECURITY.md](./SECURITY.md)
for the verification recipe.

The default `scan` output is the pure ParsedSBOM (no ID, no
timestamp). Add `--component-ref kind:Component/default/your-app
--commit-sha $(git rev-parse HEAD)` to get a full SBOM with a ULID +
`GeneratedAt` — feed that into your own storage, or pipe it to a
rampart daemon. Use `--format sarif` for the
[SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
shape consumed by the GitHub code-scanning API
(`integrations/github-action/` wraps this flow).

### Path 2 — Mid-size team (self-hosted)

Bring up the engine, Postgres, the mock npm registry, and the Slack
notifier. No Backstage, no demo scenarios — just the incident engine
and a subscriber:

```bash
docker compose up -d postgres engine mock-npm-registry slack-notifier
curl -sSf http://localhost:8080/healthz                    # {"status":"ok"}

# Register components + ingest SBOMs + publish an IoC
./scripts/seed-catalog.sh
./scripts/demo-scenarios/axios-compromise.sh

# Slack-notifier logs the payloads it would POST:
docker compose logs slack-notifier | grep 'would POST to Slack'
```

Set `SLACK_WEBHOOK_URL` in your shell (or a `.env`) to flip
`slack-notifier` out of dry-run and hit a real webhook. The engine
exposes `/v1/stream` (Server-Sent Events); anything that speaks SSE
can subscribe — PagerDuty routing, your own bridge, etc. State lives
in the Postgres volume (`rampart-pg-data`) and survives container
restarts.

### Path 3 — Platform team (Backstage)

Full stack: engine + Postgres + mock npm + Slack + a production
Backstage container that renders the rampart `IncidentDashboard` plus
the v0.2.0 incident detail drawer and blast-radius graph:

```bash
make demo-axios
open http://localhost:3000                                 # IncidentDashboard
```

Replay the 2026-03-31 `axios@1.11.0` compromise, the 2026-04-18
`rampage-*` worm, or the 2026-04-19 Vercel OAuth leak — each is a
separate make target:

```bash
make demo-axios          # 2 affected components
make demo-shai-hulud     # 1 component × 5 IoCs = 5 incidents
make demo-vercel         # 1 narrow-blast-radius incident
make demo-native         # axios scenario routed through the Rust sidecar
make demo-down           # clean teardown
```

## Features

**Five ecosystems, eight lockfile dialects.**

| Ecosystem | Lockfile(s) | Auto-detect filename(s) |
|---|---|---|
| npm | `package-lock.json` v3 (Go + Rust parity-tested) | `package-lock.json` |
| Go modules | `go.sum` + `go.mod` (replace directives, pseudo-versions) | `go.sum` |
| Cargo | `Cargo.lock` (workspaces, registry vs git source) | `Cargo.lock` |
| PyPI | `requirements.txt`, `poetry.lock`, `uv.lock` (PEP 503 name normalisation) | `requirements*.txt`, `poetry.lock`, `uv.lock` |
| Maven | `pom.xml` (property substitution), `gradle.lockfile` | `pom.xml`, `gradle.lockfile` |

PyPI and Maven (Theme C3+C4) ship CLI-only in v0.2.1 — Wasm-bridged
parity with the Rust sidecar lands in v0.5.0+.

**Matching and incident lifecycle.**
- Three IoC kinds: `packageVersion` (exact), `packageRange` (semver
  via `Masterminds/semver`), `publisherAnomaly` (Theme F3 + ADR-0014
  bridges to the anomaly detector).
- Forward matching when an IoC is published, retroactive when an
  SBOM is ingested; idempotent by `(IoCID, ComponentRef)`.
- Incident state machine: `pending → triaged → acknowledged →
  remediating → closed`, with `dismissed` reachable from any
  non-terminal state.
- Remediation audit log: append-only, typed kind enum.
- Blast radius (`POST /v1/blast-radius`) — hybrid lookup: cached
  `incidents` JOIN for ingested IoCs, live matcher for what-if
  queries (`shai-hulud@1.12 — who pages out?`).
- Server-Sent Events stream (`GET /v1/stream`) broadcasts five
  event types, with heartbeat framing for proxy compatibility.

**Publisher-anomaly detector (Theme F).** Three detectors against
publisher-graph snapshots ingested from npm + GitHub:
maintainer email drift, OIDC publishing regression, version jump.
Anomaly hits flow into the standard incident workflow via the
`IoCBodyAnomaly` bridge ([ADR-0014](./docs/decisions/0014-anomaly-ioc-bridge.md)).
Default off (`RAMPART_PUBLISHER_ENABLED`, `RAMPART_ANOMALY_ENABLED`).

**Production security posture (Theme A).**
- JWT middleware on `/v1/*` with HS256/RS256, scope-based
  authorisation (`read` / `write` / `admin`), token issuance via
  `POST /v1/auth/token`. Default off for backward compat —
  see [docs/operations/auth-providers.md](./docs/operations/auth-providers.md)
  for GitHub OAuth / Azure AD / Okta / generic-OIDC templates.
- Env-allow-list CORS (`RAMPART_CORS_ORIGINS`); v0.1.x permissive
  wildcard removed.
- Service-to-service auth between Backstage backend and engine via
  static JWT — see [ADR-0012](./docs/decisions/0012-auth-boundary-at-engine.md)
  for the auth-boundary-at-engine architecture.

**Postgres storage.** `pgx` + goose migrations. `docker-compose.yml`
includes `postgres:16-alpine` with healthcheck. Default backend;
in-memory mode preserved via `RAMPART_STORAGE=memory` for dev. The
shared `storagetest` contract suite enforces parity between
backends.

**Frontend depth (Theme E).** Backstage plugin ships an incident
detail drawer (timeline, matched IoC, affected components,
remediation log), a blast-radius graph (`reactflow` + `dagre`
auto-layout, double-click re-root), and a multi-state /
multi-ecosystem / time-range / owner search toolbar with URL state
for bookmarkable views.

**Consumers.**
- CLI (`cli/cmd/rampart`) — scan a lockfile in any of 8 dialects.
- GitHub Action (`integrations/github-action`) — lockfile → SARIF →
  code-scanning upload.
- Slack notifier (`integrations/slack-notifier`) — SSE subscriber,
  dry-run by default, real webhook behind `SLACK_WEBHOOK_URL`.
- Pre-commit hook (`integrations/precommit-hook`) — validate a
  lockfile before it lands.
- Backstage frontend plugin — `IncidentDashboard` + `IncidentDetail`
  routable extensions, blast-radius graph, anomaly panel.
- Backstage backend plugin — `/api/rampart/v1/*` proxy +
  `CatalogSync` tick.

**Infrastructure.**
- Docker Compose demo stack (`make demo-axios` — 5 s cold boot).
- GitHub Actions: per-PR engine / native / parity / gen-check /
  backstage / integrations / codeql / e2e + dependency-review +
  release.
- OpenAPI contract (`schemas/openapi.yaml`) drives Go + TS codegen;
  drift is a CI failure (`make gen-check`).
- `goreleaser` + `cargo` cross-compile matrix + multi-arch container
  images on `ghcr.io`, all cosign-signed with SBOM attestation.

## Architecture

```
  ┌───────────────┐      ┌───────────────────┐
  │  Backstage    │      │  GitHub Action    │
  │  plugin +     │      │  SARIF uploader   │
  │  backend      │      └────────┬──────────┘
  └───────┬───────┘               │
          │  HTTP + SSE           │
          ▼                       ▼
  ┌─────────────────────────────────────────┐
  │  engine (Go)                            │
  │    api/      OpenAPI server + JWT auth  │
  │    sbom/     parsers (5 ecosystems)     │
  │    matcher   IoC vs SBOM (hybrid cache) │
  │    publisher anomaly detector (3 kinds) │
  │    events    in-process SSE bus         │
  │    storage   postgres (default)         │
  └──────┬─────────────────────────┬────────┘
         │ UDS (opt-in, ADR-0005)  │ pgx
         ▼                         ▼
  ┌─────────────────┐     ┌───────────────────┐
  │  rampart-native │     │  postgres:16      │
  │  Rust parser    │     │  goose migrations │
  └─────────────────┘     └───────────────────┘
                          ┌───────────────────┐
                          │  slack-notifier   │
                          │  SSE subscriber   │
                          └───────────────────┘
```

**Domain entities.** Component, SBOM, IoC, Incident, Remediation,
Publisher, PublisherSnapshot, Anomaly. The publisher domain split
between snapshots (history) and profile (aggregate identity) is
covered by [ADR-0013](./docs/decisions/0013-publisher-domain-split.md).

**Contract.** [`schemas/openapi.yaml`](./schemas/openapi.yaml).
Everything downstream — the Backstage TS client, the engine's Go
server, the CLI, the SARIF shape in the GitHub Action — speaks this
single document. Drift is enforced at CI time by `make gen-check`.

## Performance

v0.2.1 baseline measured on Apple M3 Pro / `postgres:16-alpine` /
Go 1.25 against a 10 000-component × 10 000-SBOM × 500-IoC fixture
(see [`docs/performance/v020-load-test.md`](./docs/performance/v020-load-test.md)
for methodology, the harness in `test/load/`, and the v0.2.0 →
v0.2.1 delta table):

| Metric | Result |
|---|---|
| Total ingest | 86 s |
| Blast-radius p95 | 2.49 ms |
| Incident-detail p95 | 2.61 ms |
| Incidents opened (correctness sanity) | 5 047 |

Honest baseline; the v0.2.0 measurement, the v0.2.1 fix delta, and
the methodology notes (3-run median, single-run caveats) are all in
the doc. Reproducible end-to-end with `bash test/load/load_test.sh`
on the same host class.

## Configuration

Engine env vars (default in parens; see
[docs/operations/deployment-patterns.md](./docs/operations/deployment-patterns.md)
for the three supported deployment shapes):

| Variable | Default | Purpose |
|---|---|---|
| `RAMPART_STORAGE` | `postgres` | `postgres` or `memory` |
| `RAMPART_DB_DSN` | `postgres://rampart:rampart@postgres:5432/rampart?sslmode=disable` | pgx connection string |
| `RAMPART_DB_MAX_CONNS` | `10` | pgx pool size |
| `RAMPART_AUTH_ENABLED` | `false` | JWT middleware on `/v1/*` |
| `RAMPART_AUTH_ALGORITHM` | `HS256` | `HS256` (shared secret) or `RS256` (IdP public key) |
| `RAMPART_AUTH_SIGNING_KEY` | _(unset)_ | Shared secret or PEM-encoded public key |
| `RAMPART_AUTH_AUDIENCE` | _(unset)_ | Required when auth enabled — must match IdP's `aud` claim |
| `RAMPART_CORS_ORIGINS` | _(empty — deny all)_ | Comma-separated allow-list |
| `RAMPART_CORS_ALLOW_ALL` | `false` | Dev convenience; logs a warning when on |
| `RAMPART_PUBLISHER_ENABLED` | `false` | Theme F1 publisher-graph ingestion |
| `RAMPART_PUBLISHER_REFRESH_INTERVAL` | `1h` | Cron interval when publisher ingestion is on |
| `RAMPART_ANOMALY_ENABLED` | `false` | Theme F2 anomaly detectors |
| `RAMPART_ENGINE_AUTH_TOKEN` | _(unset)_ | Static JWT the Backstage backend forwards on every proxied call |

## Documentation

- [ARCHITECTURE.md](./ARCHITECTURE.md) — domain model, event flow, storage shape
- [CONTRIBUTING.md](./CONTRIBUTING.md) — dev setup, supply-chain rules, PR conventions
- [SECURITY.md](./SECURITY.md) — threat model, disclosure policy, cosign verification recipe
- [CHANGELOG.md](./CHANGELOG.md) — release notes
- [ROADMAP.md](./ROADMAP.md) — what's next; pre-1.0 cadence policy
- [DEPS.md](./DEPS.md) — every runtime dependency, justified
- [docs/decisions/](./docs/decisions/) — 11 ADRs (parser placement, auth boundary, publisher split, anomaly bridge, …)
- [docs/migration/v0.1.x-to-v0.2.0.md](./docs/migration/v0.1.x-to-v0.2.0.md) — upgrade guide if you're coming from v0.1.x
- [docs/operations/auth-providers.md](./docs/operations/auth-providers.md) — GitHub OAuth / Azure AD / Okta / generic-OIDC templates
- [docs/operations/deployment-patterns.md](./docs/operations/deployment-patterns.md) — Backstage-fronted, standalone, reverse-proxied
- [docs/performance/v020-load-test.md](./docs/performance/v020-load-test.md) — release-gate baseline + reproduction harness
- [docs/benchmarks/sbom-parser.md](./docs/benchmarks/sbom-parser.md) — Go vs Rust parser throughput (honest numbers)
- [schemas/openapi.yaml](./schemas/openapi.yaml) — API contract (single source of truth)
- [schemas/native-ipc.md](./schemas/native-ipc.md) — wire protocol for the opt-in Rust sidecar

## Status

rampart is **pre-1.0 (currently v0.2.1)**. Per
[ROADMAP.md](./ROADMAP.md), 1.0 stays deferred — release cadence
continues v0.3, v0.4, … v0.10 as feature themes ship. CLI flags,
public Go API surface, and SQL schema may shift between minor
versions; semver patch releases stay backwards-compatible.

## License

MIT. See [LICENSE](./LICENSE).
