# rampart

[![engine](https://github.com/ugurcan-aytar/rampart/actions/workflows/engine.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/engine.yml)
[![native](https://github.com/ugurcan-aytar/rampart/actions/workflows/native.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/native.yml)
[![parity](https://github.com/ugurcan-aytar/rampart/actions/workflows/parity.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/parity.yml)
[![e2e](https://github.com/ugurcan-aytar/rampart/actions/workflows/e2e.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/e2e.yml)
[![codeql](https://github.com/ugurcan-aytar/rampart/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/ugurcan-aytar/rampart/actions/workflows/codeql.yml)
[![license: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

> A rampart for your supply chain.

`rampart` is an open-source supply chain incident response engine for
the npm, pypi, cargo, and Go ecosystems. It ingests Software Bill of
Materials snapshots, matches them against Indicators of Compromise,
opens incidents on match, and tells you the three things that matter
when a `shai-hulud`-class event is breaking on Hacker News at 3 a.m.:

1. **Which services are affected.** Every SBOM is pinned to a Backstage
   component reference; the blast radius is a query, not a hunt.
2. **Who owns them.** Component ownership flows from the catalog; the
   incident routes to the team before a human has to triage.
3. **What's the playbook.** The incident carries an append-only
   remediation log — `pin_version`, `rotate_secret`, `open_pr`,
   `notify`, `dismiss`. Audit-grade, not vibes-grade.

The engine is Go; an optional Rust sidecar parses lockfiles over a
Unix Domain Socket for deployments that want an extra layer of parser
isolation (see [ADR-0005](./docs/decisions/0005-no-cgo-rust-via-uds.md)).
Three consumers — a Backstage plugin, a GitHub Action, and a CLI —
speak the same OpenAPI contract; no one imports the engine's Go types
directly.

## Why rampart exists

There's an axe-shaped hole in supply chain tooling. Dependabot raises
PRs, Socket scores packages, CodeQL flags source-code issues. None of
them answer the "is my production affected right now, and by whom" of
a live incident. rampart is the glue that turns a feed entry into a
routed, triageable incident with the blast radius attached.

Three user segments, three different shapes of that pain:

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

Each path runs in under 5 minutes on a fresh checkout with Go 1.24
and Docker installed.

### Path 1 — Solo developer (CLI)

Scan a lockfile from the terminal. No daemon, no services, just a
binary:

```bash
git clone https://github.com/ugurcan-aytar/rampart
cd rampart
go run ./cli/cmd/rampart scan --format json path/to/your/package-lock.json
```

The default `scan` output is a pure ParsedSBOM (no ID, no timestamp).
Add `--component-ref kind:Component/default/your-app --commit-sha $(git rev-parse HEAD)`
to get a full SBOM with a ULID + `GeneratedAt` — feed that into your
own storage, or pipe it to a rampart daemon running somewhere.

Use `--format sarif` to emit the [SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html)
shape for upload to the GitHub code-scanning API
(`integrations/github-action/` wraps this flow).

### Path 2 — Mid-size team (self-hosted)

Bring up the engine, the mock npm registry, and the Slack notifier.
No Backstage, no demo scenarios — just the incident engine and a
subscriber:

```bash
docker compose up -d engine mock-npm-registry slack-notifier
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
can subscribe — PagerDuty routing, your own bridge, etc.

### Path 3 — Platform team (Backstage)

Full stack: engine + mock npm + Slack + a production Backstage
container that renders the rampart `IncidentDashboard`:

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

Parsing and ingestion:
- npm `package-lock.json` v3 parser (byte-identical Go + Rust
  implementations, parity-tested on every PR)
- SBOM submission via `POST /v1/components/{ref}/sboms` with
  retroactive matching against every stored IoC
- Rust sidecar (opt-in via `docker compose --profile native`) —
  defense-in-depth parser isolation, engine falls back to the Go
  parser if the sidecar is unreachable

Matching and incident lifecycle:
- Three IoC kinds: `packageVersion` (exact), `packageRange`
  (semver via Masterminds/semver), `publisherAnomaly` (wire shape
  landed, detector on the roadmap)
- Forward matching when an IoC is published, retroactive matching
  when an SBOM is ingested; idempotent by `(IoCID, ComponentRef)`
- Incident state machine: `pending → triaged → acknowledged →
  remediating → closed`, with `dismissed` reachable from any
  non-terminal state
- Remediation audit log: append-only, typed kind enum
  (`pin_version`, `rotate_secret`, `open_pr`, `notify`, `dismiss`)
- Blast radius query (`POST /v1/blast-radius`) — given IoCs,
  return affected component refs
- Server-Sent Events stream (`GET /v1/stream`) broadcasts five
  event types, with heartbeat framing for proxy compatibility

Consumers:
- CLI (`cli/cmd/rampart`) — scan a lockfile, emit ParsedSBOM /
  SBOM / SARIF
- GitHub Action (`integrations/github-action`) — lockfile → SARIF
  → code-scanning upload
- Slack notifier (`integrations/slack-notifier`) — SSE subscriber,
  dry-run by default, real webhook behind `SLACK_WEBHOOK_URL`
- Pre-commit hook (`integrations/precommit-hook`) — validate a
  lockfile before it lands
- Backstage frontend plugin — `IncidentDashboard` +
  `IncidentDetail` routable extensions
- Backstage backend plugin — `/api/rampart/v1/*` proxy +
  `CatalogSync` tick

Infrastructure:
- Docker Compose demo stack (`make demo-axios` — 5 s cold boot)
- Ten GitHub Actions workflows (engine, native, parity,
  gen-check, backstage, integrations, codeql, e2e,
  dependency-review, release) — every PR runs the right subset
- OpenAPI contract (`schemas/openapi.yaml`) drives Go + TS
  codegen; drift is a CI failure (`make gen-check`)
- `goreleaser` + `cargo` cross-compile matrix + multi-arch
  container images on `ghcr.io`, all cosign-signed with SBOM
  attestation

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
  │    api/    OpenAPI server               │
  │    sbom/   parser (Go)                  │
  │    matcher IoC vs SBOM                  │
  │    events  in-process SSE bus           │
  │    trust   policy evaluation            │
  │    storage in-memory                    │
  └──────┬──────────────────────────────────┘
         │ UDS (opt-in, ADR-0005)
         ▼
  ┌─────────────────┐     ┌────────────────┐
  │  rampart-native │     │  slack-notifier│
  │  Rust parser    │     │  SSE subscriber│
  └─────────────────┘     └────────────────┘
```

Contract: [`schemas/openapi.yaml`](./schemas/openapi.yaml). Everything
downstream — the Backstage TS client, the engine's Go server, the CLI,
the SARIF shape in the GitHub Action — speaks this single document.
Drift is enforced at CI time by `make gen-check`.

## Production deployment notes

Rough edges `make demo-axios` deliberately doesn't cover — read
before deploying rampart for real.

**No CORS configuration required when fronted by Backstage.** From
v0.2.0, the frontend resolves `${backend.baseUrl}/api/rampart` via
`discoveryApiRef` and goes through the `rampart-backend` proxy —
same-origin against Backstage, no browser-side CORS handshake, no
`rampart.baseUrl` on the frontend. The Backstage proxy attaches a
static service JWT (`rampart.engine.authToken`) so the engine's A1
middleware accepts upstream calls when `RAMPART_AUTH_ENABLED=true`.

If you deploy the engine standalone (without Backstage in front),
narrow CORS via `RAMPART_CORS_ORIGINS=https://your-frontend.example`
and enable auth via `RAMPART_AUTH_ENABLED=true` +
`RAMPART_AUTH_SIGNING_KEY`. See
[docs/operations/deployment-patterns.md](docs/operations/deployment-patterns.md)
for the three supported shapes (Backstage-fronted, standalone,
reverse-proxied).

**Storage is in-memory.** The engine loses all state on restart.
Deploy behind a supervisor that can tolerate periodic restarts (k8s,
nomad, etc.); don't rely on incident continuity across restarts. A
Postgres backend is planned.

**Auth is guest.** Backstage ships with `auth.providers.guest`
enabled; swap in a real provider (OIDC, GitHub, etc.) before exposing
the stack outside a trust boundary. The dev-harness `AutoSignInPage`
bypass lives in `backstage/examples/app/packages/app/src/` — replace
it with Backstage's `SignInPage` under your provider.

**Image sizes.** The Backstage production image is ~773 MiB. Further
reductions (alpine base, more aggressive prune) are planned but carry
tradeoffs around better-sqlite3's native rebuild; see the Dockerfile
comments at `backstage/examples/app/Dockerfile` for the current
shape.

**Release artifacts are cosign-signed.** Every tarball that leaves
the release workflow carries an SBOM attestation + a cosign keyless
signature verifiable against the GitHub OIDC issuer. See
[`release.yml`](./.github/workflows/release.yml) and
[`SECURITY.md`](./SECURITY.md) for the verification recipe.

## Documentation

- [ARCHITECTURE.md](./ARCHITECTURE.md) — domain model, event flow, storage shape
- [CONTRIBUTING.md](./CONTRIBUTING.md) — dev setup, supply-chain rules, PR conventions
- [SECURITY.md](./SECURITY.md) — threat model, disclosure policy, verification
- [CHANGELOG.md](./CHANGELOG.md) — release notes
- [DEPS.md](./DEPS.md) — every runtime dependency, justified
- [docs/decisions/](./docs/decisions/) — 9 ADRs (cgo + UDS, Yarn 4, parser placement, enableScripts hardening, CI/CD pipeline)
- [docs/migration/v0.1.x-to-v0.2.0.md](./docs/migration/v0.1.x-to-v0.2.0.md) — upgrade guide if you're coming from v0.1.x
- [docs/benchmarks/sbom-parser.md](./docs/benchmarks/sbom-parser.md) — Go vs Rust parser throughput (honest numbers)
- [schemas/openapi.yaml](./schemas/openapi.yaml) — API contract (single source of truth)
- [schemas/native-ipc.md](./schemas/native-ipc.md) — wire protocol for the opt-in Rust sidecar

## License

MIT. See [LICENSE](./LICENSE).
