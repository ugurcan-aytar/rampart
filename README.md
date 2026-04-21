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

The engine is Go; the optional Rust sidecar parses lockfiles over a
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

All three paths are first-class. Each is **tested to work in
under 5 minutes** on a fresh checkout with Go 1.24 and Docker
installed.

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

## What ships in Phase 1

| Surface | Status | Notes |
|---|---|---|
| npm package-lock.json v3 parser | ✅ | `engine/sbom/npm` — byte-identical Go + Rust implementations |
| SBOM ingest + retroactive match | ✅ | New SBOMs matched against every stored IoC |
| IoC publish + forward match | ✅ | Packet-version, packet-range (semver), publisherAnomaly (Phase 2) |
| Incident lifecycle | ✅ | State machine: pending → triaged → acknowledged → remediating → closed/dismissed |
| Blast radius query | ✅ | O(IoCs × SBOMs) scan — Phase 2 bitmap index |
| Remediation audit log | ✅ | Append-only, typed kind enum |
| Server-Sent Events stream | ✅ | Five event types (`incident.opened`, …), heartbeat |
| In-memory storage | ✅ | Single-node demo; Phase 2 ships Postgres |
| Backstage frontend plugin | ✅ | `IncidentDashboard` + `IncidentDetail` routable extensions |
| Backstage backend plugin | ✅ | Engine proxy + catalog-sync tick |
| GitHub Action | ✅ | `integrations/github-action` — SBOM → SARIF upload |
| Slack notifier | ✅ | SSE consumer, dry-run by default |
| Pre-commit hook | ✅ | `integrations/precommit-hook` — validate before push |
| Rust sidecar (opt-in) | ✅ | `--profile native`; defense-in-depth parser isolation, not a throughput win |
| Docker Compose demo stack | ✅ | `make demo-axios` → full stack up in < 5 s cold |
| CI/CD | ✅ | Ten GitHub Actions workflows (engine, native, parity, gen-check, backstage, integrations, codeql, e2e, dependency-review, release) |

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
  │    storage in-memory (Phase 2: pg)      │
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

This section is what `make demo-axios` deliberately doesn't cover —
the rough edges you'll hit when deploying rampart for real.

**CORS + base URLs.** The frontend's `RampartClient` reads
`rampart.baseUrl` and fetches the engine directly. In the demo stack
this is `http://localhost:8080` (browser-reachable). In production,
point the frontend at your Backstage backend's proxy
(`/api/rampart/v1/…`) — the `rampart-backend` plugin already mounts
it. The engine's current CORS middleware is a wildcard for demo
convenience; tighten for production.

**Storage is in-memory.** Phase 1 loses all state on restart. Postgres
lands in Phase 2 — see [ROADMAP.md](./ROADMAP.md). Deploy behind a
supervisor that can tolerate periodic restarts (k8s, nomad, etc.) in
the meantime; don't rely on incident continuity across restarts.

**Auth is guest.** Backstage ships with `auth.providers.guest`
enabled; swap in a real provider (OIDC, GitHub, etc.) before exposing
the stack outside a trust boundary. The dev-harness `AutoSignInPage`
bypass lives in `backstage/examples/app/packages/app/src/` — replace
it with Backstage's `SignInPage` under your provider.

**Image sizes.** The Backstage production image is ~770 MiB after
the Phase 1 slimming pass. Further reductions (alpine base, more
aggressive prune) are a Phase 2 ROI question; see the Dockerfile
comments at `backstage/examples/app/Dockerfile`.

**Release artifacts are cosign-signed.** Every tarball that leaves
the release workflow carries an SBOM attestation + a cosign keyless
signature verifiable against the GitHub OIDC issuer. See
[`release.yml`](./.github/workflows/release.yml) and
[`SECURITY.md`](./SECURITY.md) for the verification recipe.

## Documentation

- [ARCHITECTURE.md](./ARCHITECTURE.md) — domain model, event flow, storage shape
- [ROADMAP.md](./ROADMAP.md) — Phase 2 / 3 scope + reasoning
- [CONTRIBUTING.md](./CONTRIBUTING.md) — dev setup, supply-chain rules, PR conventions
- [SECURITY.md](./SECURITY.md) — threat model, disclosure policy, verification
- [DEPS.md](./DEPS.md) — every runtime dependency, justified
- [docs/decisions/](./docs/decisions/) — 9 ADRs (cgo + UDS, Yarn 4, parser placement, enableScripts hardening, CI/CD pipeline)
- [docs/benchmarks/sbom-parser.md](./docs/benchmarks/sbom-parser.md) — Go vs Rust parser throughput (honest numbers)
- [schemas/openapi.yaml](./schemas/openapi.yaml) — API contract (single source of truth)
- [schemas/native-ipc.md](./schemas/native-ipc.md) — wire protocol for the opt-in Rust sidecar

## License

MIT. See [LICENSE](./LICENSE).
