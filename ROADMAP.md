# Roadmap

Four horizons, in order of commitment.

- **v0.1.1** is housekeeping — release-engineering debt carried from
  v0.1.0. Small patch, 1–2 weeks.
- **v0.2.0** is the major feature release. Production-grade security,
  multi-ecosystem parsers, frontend depth, and the first real
  behavioural differentiator (publisher-anomaly detection).
- **v0.3.0–v0.9.0** are feature horizons with concrete scope per
  release. Multi-repo aggregation, LLM augmentation, sandboxing,
  integrations, distribution.
- **v1.0.0** is the production commitment: enterprise auth, plugin
  sandbox, performance index, SOC 2 / ISO 27001 readiness. API
  stability guarantee starts here.

Phase 2 / Phase 3 / Phase 4 blocks below describe *themes*; individual
line items are scheduled into specific releases. The **non-goals**
section is a discipline doc — every new proposal gets checked against
it before entering any release scope.

---

## Current state (v0.1.0 shipped)

v0.1.0 landed on 2026-04-21. What's running today:

- **Three quickstart paths**, one per user segment, each tested green
  on a fresh checkout: `rampart scan` CLI (solo developer),
  `docker compose up -d engine mock-npm-registry slack-notifier`
  (mid-size team), `make demo-axios` (platform team) with a five-service
  Backstage stack.
- **Engine** (Go) implementing the six domain entities (Component, SBOM,
  PackageVersion, IoC, Incident, Remediation), the incident state
  machine, forward + retroactive matching, an in-process event bus with
  five event types fanned out over SSE on `/v1/stream`, and a
  blast-radius query on `/v1/blast-radius`.
- **Rust sidecar** (rampart-native), opt-in via
  `docker compose --profile native`. Isolation feature, not a throughput
  win at Phase 1 (see ADR-0005).
- **Three Backstage plugins** — `@ugurcan-aytar/backstage-plugin-rampart`
  (frontend), `-backend` (engine proxy + CatalogSync), and
  `-scaffolder-rampart-actions` (scaffolder actions). Ship pre-built
  inside the `rampart-backstage` container image (not yet on npm — see
  v0.1.1).
- **Release artefacts on ghcr.io + GitHub Releases**:
  five multi-arch container images (engine, rampart-native,
  rampart-slack-notifier, rampart-mock-npm-registry, rampart-backstage),
  twenty Go archive bundles via goreleaser (4 tools × linux/darwin +
  windows where applicable), four cargo cross-compile Rust targets,
  all cosign-signed via GitHub OIDC (keyless, no long-lived signing
  keys in the repo) with syft-generated SPDX SBOMs attached as cosign
  attestations.
- **Ten CI/CD workflows** — see ADR-0009 for the gating-vs-advisory
  split and the per-package coverage gates.
- **Zero open Dependabot alerts** as of 2026-04-22 (35 → 0 via PR #7,
  #20, #29 bumps + six `tolerable_risk` dismissals for upstream-pinned
  transitives — see individual alert comments).

---

## v0.1.1 — housekeeping (target: 2–3 weeks)

Patch release that closes release-engineering debts explicitly deferred
from v0.1.0. No new user-facing features.

### Blockers (must ship before v0.1.1 tag)

#### 1. Backstage plugin npm trusted publishing

Three packages pending first publish:

- `@ugurcan-aytar/backstage-plugin-rampart`
- `@ugurcan-aytar/backstage-plugin-rampart-backend`
- `@ugurcan-aytar/backstage-scaffolder-rampart-actions`

Pre-requisites:
- Initial manual `npm publish` for each package (Trusted Publisher
  can't bootstrap a package that doesn't exist on the registry).
- Trusted Publisher configuration on `npmjs.com` side: bind each
  package to the `ugurcan-aytar/rampart` repository + the
  `release.yml` workflow path as the OIDC subject.
- Restore the `npm:` job in `release.yml` that v0.1.0 removed
  (see the `chore(release): defer npm publish to v0.1.1` commit
  for the exact shape).

**Acceptance**: pushing `v0.1.1` fires `release.yml`; all three
packages appear on `npmjs.com` within five minutes, with the
`--provenance` attestation visible in the npm web UI and verifiable
via `npm audit signatures`.

#### 2. SHA-pin sweep — remaining references

Every third-party GitHub Actions reference must be pinned to an
immutable commit SHA, with a trailing comment denoting the human-
readable semver tag.

Remaining references (as of 2026-04-22) still on semver tags:
- `actions/checkout` v4 → v5
- `actions/download-artifact` v4 → v5
- `docker/setup-qemu-action` v3 → v4
- `docker/setup-buildx-action` v3 → v4
- `docker/login-action` v3 → v4
- `docker/metadata-action` v5 → v6
- `github/codeql-action` v3 → v4 (if v4 exists by v0.1.1 tag)

**Acceptance**:
```
grep -rE "uses: [^ @]+@v[0-9]" .github/workflows/*.yml
```
returns zero matches (excluding rolling-pointer branch-refs like
`dtolnay/rust-toolchain@stable`). Dependabot continues to raise
`digest` updates on the SHA refs.

#### 3. Node.js 20 action-runtime deprecation finish

GitHub Actions drops Node 20 default on 2 June 2026. Bulk of the
Dependabot PRs merged during the 2026-04-22 cleanup session. Remaining
checks:

- Verify no workflow step still resolves to a Node-20 action version.
- Verify the latest green CI run on `main` emits no Node-20
  deprecation warning.

**Acceptance**: every third-party action runs on Node 22+. Verified
by: clean CI run on `main` post-merge of all SHA-pin updates.

### Nice-to-have (can slip to v0.1.2)

- **README release badge** —
  `![release](https://img.shields.io/github/v/release/ugurcan-aytar/rampart?sort=semver)`
  next to the CI badges.
- **Dependabot grouping extensions** — `dev-tools` group
  (typescript, @types/*, openapi-typescript, puppeteer-core,
  @playwright/*) and `actions-core` group (actions/*) added to
  `.github/dependabot.yml`. Collapses Monday PR bursts to ~3–5
  grouped PRs instead of the 24-PR first-run we saw on v0.1.0.

### Deferred ecosystem upgrades (re-evaluation triggers)

Closed Dependabot PRs pending upstream readiness. Listed here so
maintainers don't re-attempt prematurely.

- **Node 24 LTS migration** (currently on Node 20). Closed PRs
  #16 (`node:25-bookworm-slim`) and #23 (`@types/node@25`) — paired.
  - Blocked on: `node:24-bookworm-slim` image availability
    (Node 24 LTS ships October 2026).
  - Side finding from the Node 25 attempt: Node 25 dropped bundled
    corepack from official images, breaking Yarn 4 activation in
    `backstage/examples/app/Dockerfile`. Won't recur on Node 24 LTS.
  - Re-evaluation trigger: `docker manifest inspect node:24-bookworm-slim`
    succeeds AND `@backstage/cli` declares Node 24 in engines field.

- **React 19 + react-router 7 migration**. Closed PR #19.
  - Blocked on: `@backstage/core-components` upstream release
    accepting `react-markdown@10+`. `react-markdown@8.0.7` still
    references the JSX global namespace that React 19 removed.
  - Re-evaluation trigger: next `@backstage/core-components` major
    accepts `react-markdown@^10` in peerDependencies.

---

## v0.2.0 — major feature release (target: 6–10 weeks after v0.1.1)

This is the release where rampart goes from "scaffolded working
system" to "production-competitive supply-chain defense tool." Four
themes, shipping together so the release has a coherent story.

### Theme A — Production-grade security posture

Rampart's v0.1.x assumes a trusted reverse proxy upstream, permissive
CORS, and guest-auth Backstage. v0.2.0 removes every one of those
footnotes.

#### A1. First-party auth on `/v1/*`

- JWT-based request authentication middleware on the engine.
- Token issuance path: `POST /v1/auth/token` with a configured signing
  key (HS256 for operator simplicity; RS256 optional via config).
- All `/v1/*` mutation routes require a valid token; read-only routes
  configurable per deployment.
- Token claims: `sub` (service identity), `aud` (engine instance),
  `exp`, custom `scope` (read / write / admin).
- Middleware emits a `/v1/stream` event for every rejected token
  (signal for brute-force / replay detection).
- **Acceptance**: `curl -X POST /v1/components` without a token
  returns 401; with a valid token returns 201. Integration test
  covers expired token, wrong audience, and wrong signature.

#### A2. Engine CORS: permissive → env-configured allow-list

- `RAMPART_CORS_ORIGINS` env var accepts a comma-separated list.
- Default (empty var): deny all cross-origin requests (same-origin
  only). The dev-mode wildcard `*` that v0.1.x shipped is removed.
- `docker-compose.yml` documents the production pattern (empty var)
  and the dev pattern (explicit `http://localhost:3000`).
- **Acceptance**: `make demo-axios` works with the Backstage proxy
  wired (Theme B below) without setting any CORS origins. A cross-
  origin browser fetch against an un-allowlisted origin fails with
  the correct preflight rejection.

#### A3. Backstage guest auth → real provider

- `backstage/examples/app/app-config.yaml` no longer enables
  `guest.dangerouslyAllowOutsideDevelopment`.
- Ship a minimal GitHub OAuth provider config as the demo default
  (operators need a registered GitHub OAuth app — one-page README
  instruction).
- Alternative documented: `auth.providers.microsoft`,
  `auth.providers.okta`, `auth.providers.oidc` — configuration
  templates under `backstage/examples/app/config/auth-*.yaml`.
- `make demo-axios` smoke test updated: authenticate once, then
  run the scenario.
- **Acceptance**: the demo Backstage instance refuses unauthenticated
  access; after GitHub OAuth sign-in, all rampart plugin surfaces
  render.

### Theme B — Backstage architecture completion

v0.1.x mounted `/api/rampart/v1/*` in the backend plugin but the
frontend client still calls the engine directly (`localhost:8080`).
v0.2.0 closes the loop and makes "no CORS config needed" real.

#### B1. Frontend client → relative URLs

- `RampartClient` swaps `config.getString('rampart.baseUrl')` for
  `config.getOptionalString('backend.baseUrl')` + `/api/rampart/v1`.
- Dev-mode fallback documented for operators who want to test
  against a local engine without Backstage in the middle.
- Frontend plugin README updated — the `yarn add` snippet no longer
  needs the `rampart.baseUrl` config key.

#### B2. CatalogSync real push

- `rampart-backend` tick loop stops logging placeholder; starts
  pushing Backstage `Component` entities to `/v1/components` via
  the (now authenticated — see A1) engine API.
- Idempotent: tick with unchanged catalog produces zero engine writes
  (detected via `If-None-Match` on the component's `etag`).
- Ordering guarantee: `Component` with dependencies is created after
  its dependencies (topological push). Engine schema already supports
  this via `depends_on` foreign keys once Postgres storage lands
  (Theme D — included in v0.2.0 below).
- **Acceptance**: start the demo stack, add a Component to
  Backstage's catalog YAML, see it appear in `/v1/components`
  within one tick interval (default 30 s).

#### B3. "No CORS config needed" in install docs

- README's install section loses the CORS troubleshooting callout.
- The phrase "rampart works behind a standard reverse proxy without
  engine-side origin allow-list" appears in the install docs.

### Theme C — Multi-ecosystem parser expansion

v0.1.x ships npm `package-lock.json` v3 only. v0.2.0 adds Go, Rust,
Python, and Java/Kotlin. "Rampart is npm-only" is the most common
false impression from the v0.1.0 README skim; this theme kills it.

#### C1. Go modules parser

- `engine/sbom/gomod` package implementing the `Parser` interface.
- Reads `go.sum` (authoritative version list) and `go.mod` (for
  replace directives + module identity).
- Separate IoC namespace: `IoCKindGoModule`. The matcher's
  ecosystem-dispatch already supports per-ecosystem lookups.
- Rust sidecar parity implementation: `native/src/parsers/gomod.rs`,
  gated by the existing parity-test harness.
- Fixture pack: small (hello-world), medium (cobra-based CLI),
  large (kubernetes-level go.sum).
- **Acceptance**: `rampart scan path/to/go.sum --ecosystem=gomod`
  produces a valid SBOM; `parity-test gomod` passes on all fixtures.

#### C2. Cargo parser

- `engine/sbom/cargo` package, reads `Cargo.lock` (TOML).
- Rust sidecar implementation already has the TOML primitives (it
  parses its own `Cargo.toml` for version metadata).
- `IoCKindCargo`, fixture pack, parity harness — same shape as C1.
- **Acceptance**: `rampart scan path/to/Cargo.lock --ecosystem=cargo`;
  parity passes.

#### C3. PyPI parser

- `engine/sbom/pypi` package, reads three common lockfile formats:
  `requirements.txt` (flat), `poetry.lock` (nested TOML),
  `uv.lock` (TOML, similar to poetry).
- No Rust sidecar parity for PyPI in v0.2.0 — single-engine parser
  is acceptable; the isolation / parity story moves to Wasm in
  v0.4.0+.
- **Acceptance**: `rampart scan path/to/requirements.txt`,
  `rampart scan path/to/poetry.lock`, `rampart scan path/to/uv.lock`
  — all produce valid SBOMs, matched against a PyPI IoC fixture pack.

#### C4. Maven / Gradle parser

- `engine/sbom/maven` package, reads `pom.xml` (XML) and Gradle
  lockfile format.
- Broader coordinates: Maven uses `groupId:artifactId:version`
  instead of flat package names — engine's `PackageVersion` schema
  already tolerates this (the `ref` field is free-form).
- No Rust parity in v0.2.0.
- **Acceptance**: `rampart scan path/to/pom.xml` produces an SBOM;
  a dependency-confusion IoC matching `com.example:my-artifact:1.0`
  opens an incident.

#### C5. Install path refresh

- `rampart scan` auto-detects ecosystem from file name when
  `--ecosystem` is omitted (package-lock.json → npm, go.sum → gomod,
  etc.). Fallback: explicit flag required.
- Homepage demo section lists all five ecosystems with a one-line
  curl-able fixture each.

### Theme D — In-memory → Postgres storage

Required by CatalogSync idempotency (B2) and multi-day incident
timelines. The in-memory store is fine for demos; not fine for
"deploy this to production."

- `engine/internal/storage/postgres` package against the existing
  `Storage` interface.
- Shared `storagetest` contract suite — same tests run against memory
  and postgres; new backends guaranteed interface-compliant.
- Migrations via `goose` (SQL-first, operator-readable, no ORM).
- `docker-compose.yml` ships a `postgres:16-alpine` service (the
  deliberately-absent stub from v0.1.x). Default engine config
  points at it; override via `RAMPART_STORAGE=memory` for dev.
- Benchmarks: ingestion throughput (SBOMs/sec), query latency
  (p50/p95/p99 for blast-radius), footprint (MB per 1k components).
- **Acceptance**: `docker compose up -d postgres engine` boots
  clean. 1,000 components + 100 incidents persist across a full
  container restart. p95 blast-radius query < 50 ms on this fleet
  size.

### Theme E — Frontend depth

Screenshot-worthy UI depth. v0.1.x `IncidentDashboard` is a flat
table; v0.2.0 makes it feel like a real tool.

#### E1. Incident detail drawer

- Click an incident row → side drawer with:
  - **Timeline panel**: state transitions (opened → triaging →
    remediating → closed) with per-transition actor + timestamp.
  - **Matching IoC**: full IoC payload rendered (version ranges,
    publisher metadata, severity).
  - **Affected components**: list of Components + their blast-radius
    depth.
  - **Remediation log**: append-only log of remediation actions
    (pin version, rotate secret, open PR).
- Drawer reads from a new `/v1/incidents/{id}/detail` endpoint that
  joins incident + IoC + components in one query.
- **Acceptance**: click any incident, drawer opens within 200 ms,
  all four panels populated from a single API call.

#### E2. Blast-radius graph visualisation

- New `BlastRadiusGraph` component using `@react-flow/core` (already
  a transitive dep via Backstage core-components).
- Node = Component, edge = dependency, red outline = directly
  affected, orange outline = transitively affected, grey = safe.
- Interaction: click a node → drill into that component's own detail
  drawer. Double-click → re-root the graph at that component.
- **Acceptance**: a 100-component fleet with 10 affected components
  renders interactively on a standard laptop (60 fps drag / zoom).

#### E3. Search + filter

- Dashboard toolbar: search box (matches incident ID, IoC ID,
  component ref), state filter (multi-select), ecosystem filter,
  date-range picker, owner filter.
- URL state: filters serialise to query params for bookmarkable
  views.
- **Acceptance**: `?state=open&ecosystem=npm&owner=team-alpha`
  loads the dashboard pre-filtered; changes persist as the user
  refines filters.

### Theme F — Publisher-anomaly detector

The first real "rampart does something Socket/Snyk also sell" feature.
v0.1.x wired the `IoCKindPublisherAnomaly` enum but the matcher
returns no-op. v0.2.0 builds the publisher graph and ships three
concrete anomaly types.

#### F1. Publisher graph ingestion

- `engine/publisher` package — ingests:
  - **npm API**: `registry.npmjs.org/<package>` maintainer list +
    version publish timestamps.
  - **GitHub API**: release metadata (tag, author, signature) for
    packages that declare a GitHub repository in their manifest.
- Persisted in Postgres (new `publisher_history` table) so anomaly
  detection has historical context.
- Cron tick (configurable, default 1 h) refreshes the graph for
  packages in the active SBOM set.

#### F2. Three anomaly types

1. **Maintainer email drift** — a package's listed maintainer email
   changes within 7 days of a new version release. Classic takeover
   pattern.
2. **OIDC publishing regression** — a package that previously
   published with npm Trusted Publisher (OIDC) switches back to
   token-based publishing. Signal of credential compromise.
3. **Version jump anomaly** — semver jump that doesn't match the
   package's historical cadence (e.g. 1.2.3 → 47.0.0 from a package
   that's been doing patch releases for two years).

Each anomaly emits an `IoCKindPublisherAnomaly` with a confidence
score (high / medium / low) and a human-readable explanation.

#### F3. IncidentDashboard surface

- Publisher anomalies render with a distinct icon in the list view.
- Detail drawer (E1) shows the publisher-graph context: maintainer
  history, release cadence chart, OIDC status over time.
- **Acceptance**: a test fixture package with a simulated maintainer
  email change produces a visible publisher-anomaly incident within
  one cron tick.

### v0.2.0 release gate

- All six themes' acceptance criteria green.
- `make demo-axios` still works end-to-end (the auth changes in
  Theme A require demo config adjustment).
- Load test: 10k SBOMs + 500 IoCs + 200 publisher-anomaly records
  ingested, blast-radius p95 < 500 ms.
- Security posture documented: updated `SECURITY.md` reflects the
  new auth + CORS + Postgres surface.

---

## v0.3.0 — team scale (target: 10–14 weeks after v0.2.0)

Solo / mid-size team segments get their dedicated install paths.

### Theme G — Solo developer: `rampart watch` daemon

- Long-running local daemon; polls IoC feeds (OSV.dev, Socket.dev
  advisory stream) on a configurable interval (default hourly).
- Re-scans pointed lockfiles; diffs against last scan; opens a local
  incident on any new match.
- Desktop notifications: macOS Notification Center, Linux libnotify,
  Windows toast — via
  [beeep](https://github.com/gen2brain/beeep).
- CLI shape: `rampart watch --interval 1h --feeds osv,socket
  --lockfile path/to/package-lock.json`.
- **Acceptance**: `brew install rampart && rampart watch` works on
  a clean macOS laptop; a native notification fires within 60 s of
  an IoC being published in a test feed.

### Theme H — Mid-size team: multi-repo aggregation

- Single centralised daemon, org-wide monitoring.
- SBOM ingestion via webhook (GitHub `pull_request.closed` event)
  or scheduled poll of each repo's default branch.
- Single `IncidentDashboard` correlating across the fleet; blast
  radius becomes an org-level query.
- Per-repo `team_ref` on components; incidents route to the owning
  team before a human triages.
- **Acceptance**: ten repositories wired to a single daemon;
  blast-radius query returns the correct affected set in under
  500 ms on a 10-component-per-repo fleet.

### Theme I — Publishing: Homebrew + GitHub Marketplace

- **Homebrew tap**: `brew tap ugurcan-aytar/rampart` →
  `brew install rampart`. goreleaser emits the formula; the tap
  itself is a one-file repo.
- **GitHub Marketplace**: `integrations/github-action/` ships an
  `action.yml` ready for `@v1` tagging. Marketplace metadata (icon,
  colour, categories) picks up from the action's branding block.

### Theme J — IoC feed federation

- Phase 1 has a single internal feed. v0.3.0 aggregates OSV.dev,
  Socket.dev advisory stream, GitHub Advisory Database, Snyk
  Advisor.
- Conflict resolution: same CVE seen in multiple feeds, aggregate
  severity, per-feed credibility weighting.
- Feed source appears in the incident detail drawer (E1) so
  operators know provenance.

---

## v0.4.0 — LLM augmentation (opt-in) (target: 6–8 weeks after v0.3.0)

LLM features are **opt-in per deployment**, default off, local-only
by default (no remote API call unless the operator wires one in).
The core determinism stays intact — LLM outputs are enhancements, not
required inputs.

### Theme K — LLM features

- **Triage summary**: one-paragraph narrative of a fresh incident —
  affected services, likely remediation direction, similar prior
  incidents.
- **PR description generation**: "pin axios to 1.10.5 to block
  shai-hulud propagation" + rationale + test-plan checklist.
- **Incident narrative**: human-readable story assembled from the
  append-only event log.

Config surface: `rampart.llm.enabled=true`, `rampart.llm.backend=openai|anthropic|ollama`,
`rampart.llm.model=<model-id>`. Prompt templates configurable via
`rampart.llm.templates.*`.

### Theme L — Frontend LLM integration

- Detail drawer (E1) gets a "Summary" panel that appears when LLM
  is enabled. Falls back gracefully to the deterministic data when
  disabled.
- Remediation log entries auto-populate with LLM-generated rationale
  when operators run `rampart remediate --apply`.

---

## v0.5.0 — sandboxing (target: 6–10 weeks after v0.4.0)

Hostile lockfile protection beyond the Phase 1 rampart-native process
isolation.

### Theme M — Wasm-compiled parser

- Stronger alternative to process isolation.
- Hostile lockfile can't crash the engine (wasm memory safety +
  resource caps).
- Trade-off evaluation vs `SCM_RIGHTS` process isolation (Phase 2
  sidecar crossover work, part of v0.2.0 or deferred pending
  measurement).

### Theme N — rampart-native Phase 2 evaluation

- Phase 1 measured two wire shapes (JSON envelope, binary envelope)
  and found no throughput crossover (ADR-0005 kept the sidecar
  opt-in as an isolation feature). Phase 2 evaluates `SCM_RIGHTS` +
  mmap content handoff + binary response (FlatBuffers or bincode).
- Hypothesis: crossover emerges on 100+ MiB lockfiles where copy
  cost dominates parse cost.
- Decision tree:
  - **Crossover measured** → promote sidecar to default (drop the
    opt-in flag); keep it as the fast path.
  - **No crossover** → opt-in keeps the isolation value; update
    ADR-0005 with the Phase 2 verdict.
  - **Marginal improvement** → call it explicitly; ship the measured
    number, let operators decide.

### Theme O — Container-per-parse (contingent)

- If Theme M / N show that process isolation is insufficient against
  hostile supply-chain inputs, kernel-level sandboxing (gVisor /
  Kata) is the next rung. Heavier, but justifies itself against the
  `shai-hulud`-class threat model.
- Ship only if the Phase 2 verdict (Theme N) is "crossover observed
  but insufficient isolation".

---

## v0.6.0 — outbound integrations (target: 4–6 weeks after v0.5.0)

rampart becomes the source of truth; downstream systems consume its
events.

### Theme P — Webhook / SIEM integrations beyond Slack

- Splunk HEC.
- Datadog Events API.
- PagerDuty Events v2.
- Opsgenie.
- Microsoft Teams webhook.

Same pattern as `integrations/slack-notifier`: separate Go binary,
SSE consumer, dry-run default, per-integration README.

### Theme Q — IDE integrations

- **VS Code extension**: `rampart` panel showing active incidents
  for the currently-open lockfile; inline gutter decorations on
  versions with matching IoCs.
- **JetBrains plugin** (IntelliJ platform): same feature set,
  different distribution.

These are two distinct packages; either one on its own is a v0.6.0
shipping artefact.

---

## v0.9.0 — pre-1.0 hardening (target: 8–12 weeks after v0.6.0)

Everything that needs to be true before we commit to v1.0 API
stability.

### Theme R — Performance at fleet scale

- **Roaring-bitmap blast-radius index**: v0.1.x's query is
  O(IoCs × SBOMs). v0.9.0 builds a bitmap per
  `(ecosystem, package, version)` answering in O(|affected components|).
- **Target**: sub-500 ms on a 100k-component fleet. Benchmark
  included in the release notes with reproducible harness.

### Theme S — API stability audit

- Full `/v1/*` API review. Any endpoint that's likely to change
  post-1.0 either stabilises or moves to `/v1alpha/` with a
  documented promotion path.
- OpenAPI spec annotated with per-endpoint stability labels.
- SemVer contract: v1.x patches never break `/v1/*` behaviour;
  v2.0 required for any breaking change.

### Theme T — SOC 2 / ISO 27001 preparedness

- Audit log schema review (who did what, when, how).
- Secret handling review (SECURITY.md updated with the full chain:
  env → OIDC → short-lived tokens).
- Threat model document (docs/security/threat-model.md).
- No stored credentials on disk — every secret is either an env
  var or an OIDC-issued short-lived token.

---

## v1.0.0 — production commitment (target: 4–6 weeks after v0.9.0)

The 1.0 release commits to API stability and long-term support. API
`v1` is frozen; breaking changes require `v2`.

### Theme U — Enterprise auth

- OIDC / SAML provider integration (Keycloak, Okta, Auth0).
- RBAC scoping: components and incidents scoped by team.
- Full audit log: who transitioned which incident, when, who
  approved.
- SOC 2 / ISO 27001 preparedness (Theme T already ran in v0.9.0).

### Theme V — Wasm plugin sandbox

- Operators ship bespoke IoC logic as a wasm module with a
  limited-capability API.
- Rampart core stays insulated from plugin bugs.
- Opens the door to an opt-in community plugin marketplace
  (post-1.0 follow-up).

### Theme W — 1.0 release content

- Blog post with the v0.1.0 → v1.0 arc.
- Hacker News / Lobste.rs / r/programming launch set.
- Conference talk pitch (KubeCon, SREcon, FOSDEM).
- Comparison page vs Socket / Snyk / GitHub Advisory — feature
  matrix, honest about where rampart is equal / better / worse.

---

## Non-goals (what rampart will not do)

This is a discipline doc. New feature proposals get checked against
it. Saying no early is cheaper than walking it back later.

- **No cloud callbacks.** Rampart emits no telemetry — not to
  rampart maintainers, not to a cloud service, not anywhere. User
  data stays on the user's machine / cluster.
- **No vendor lock-in.** Rampart is OSS, self-hostable, portable
  across standard infrastructure. There will never be a cloud-only
  SaaS mode.
- **No LLM in the core path.** LLM features are strictly opt-in
  enhancements, never required for rampart's core function —
  ingest → match → open incident.
- **No stored secrets.** Rampart does not store webhook URLs, API
  keys, or credentials on disk. Secrets live in env vars or a
  trusted OIDC chain.
- **No silent fallbacks.** If a parser fails, the event is a warn
  log plus an operator-visible incident, never a silent skip.
  (See `PARSER_FALLBACK` events on `/v1/stream` for operator
  visibility.)
- **No proprietary wire protocols.** Contract is OpenAPI 3.1, SBOM
  format is SPDX / CycloneDX, IoC format is JSON against a published
  schema. No bespoke binary formats on public surfaces.
- **No training on rampart telemetry.** If a user chooses to enable
  LLM features pointing at a remote backend, rampart guarantees the
  prompts and responses never feed any training set.

---

## Release exit criteria (summary)

- **v0.1.1**: three blockers closed (npm trusted publishing, SHA-pin
  sweep tail, Node 20 deprecation verification).
- **v0.2.0**: six themes green (auth, Backstage wiring, multi-ecosystem
  parsers, Postgres, frontend depth, publisher anomaly). Load test
  passed. `SECURITY.md` updated.
- **v0.3.0**: solo `rampart watch` + mid-size multi-repo aggregation
  + Homebrew + GitHub Marketplace + IoC feed federation.
- **v0.4.0**: LLM features opt-in, local-only default, core
  determinism intact.
- **v0.5.0**: Wasm parser verdict documented. Process isolation
  verdict (Theme N) documented.
- **v0.6.0**: ≥3 SIEM/webhook integrations shipped + at least one
  IDE integration (VS Code or JetBrains).
- **v0.9.0**: fleet performance target hit. API stability audit
  complete. Threat model shipped.
- **v1.0.0**: enterprise auth shipped. Wasm plugin sandbox shipped.
  API `v1` frozen.

---

*Last updated: 2026-04-23.*
