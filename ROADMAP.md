# Roadmap

Three horizons, in order of commitment. v0.1.1 is scheduled work with
acceptance criteria. Phase 2 is work we intend to do once v0.1.1 ships,
grouped by the user segment it benefits. Phase 3 is vision — direction,
not a backlog. The **non-goals** section is a discipline doc: any
new feature proposal gets checked against it.

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

---

## v0.1.1 — housekeeping (target: 2–3 weeks)

Patch release that closes three release-engineering debts explicitly
deferred from v0.1.0. No new user-facing features.

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

#### 2. SHA-pin sweep completion

Every third-party GitHub Actions reference must be pinned to an
immutable commit SHA, with a trailing comment denoting the human-
readable semver tag (so Dependabot can track updates).

Current state: the Phase 1 `pinact` sweep (commit `245695e`) covered
most references. Any that slipped since need a pass.

**Acceptance**:
```
grep -rE "uses: [^ @]+@v[0-9]" .github/workflows/*.yml
```
returns zero matches (excluding branch-ref cases like
`dtolnay/rust-toolchain@stable`, which is a rolling pointer by
design). Dependabot continues to raise `digest` updates on the
SHA refs.

#### 3. Node.js 20 action-runtime deprecation

GitHub's Actions runner drops Node 20 as default on 2 June 2026.
Four Dependabot PRs already cover the bulk of the exposed actions
(as of 2026-04-22):
- `#9` actions/upload-artifact v4 → v7
- `#12` docker/build-push-action v6 → v7
- `#13` actions/setup-go v5 → v6
- `#14` actions/setup-node v4 → v6

Not yet PR'd (next Dependabot cycle):
- `actions/checkout` v4 → v5
- `actions/download-artifact` v4 → v5
- `docker/setup-qemu-action` v3 → v4
- `docker/setup-buildx-action` v3 → v4
- `docker/login-action` v3 → v4
- `docker/metadata-action` v5 → v6
- `golangci/golangci-lint-action` v6 → v9 (PR `#10` open; blocked
  on the v2 config migration — see ADR-0009)
- `github/codeql-action` v3 → v4 (if v4 exists by deprecation date)

**Acceptance**: every third-party action in `.github/workflows/*`
runs on Node 22+. Verified by: no Node-20 deprecation warning in
the most recent green CI run on `main`.

### Nice-to-have (can slip to v0.1.2)

- **B-tier Dependabot PR review + merge** (17 PRs open as of v0.1.0
  ship, some now merged via the SHA-pin / Node-20 work above).
  - `golangci-lint` v2 config migration (ADR-0009 lists this as a
    separate ADR).
  - `node:20-bookworm-slim` → `node:25-bookworm-slim` for the
    Backstage image + `@types/node` 20 → 25 (Backstage
    compatibility check required — `backstage-cli` still targets
    Node 20 LTS as of 2026-04).
  - React 18 → 19, TypeScript 5 → 6 (breaking-change analysis per
    Backstage plugin).
  - Backstage-core 7-update group (canary churn; merge in a window
    where the `make demo-*` paths can be re-run end-to-end).
  - Strategy: small batches, CI green before each merge, rollback
    to previous SHA if demo scenarios regress.
- **README release badge** — `![release](https://img.shields.io/github/v/release/ugurcan-aytar/rampart?sort=semver)`
  next to the existing CI badges.
- **Dependabot grouping extensions** — add `dev-tools`
  (typescript, @types/*, openapi-typescript, puppeteer-core,
  @playwright/*) and `actions-core` (actions/*) groups to
  `.github/dependabot.yml`. Collapses Monday PR bursts to ~3–5
  grouped PRs instead of the 24-PR first-run we saw.

### Known gaps (acknowledged, deferred beyond v0.1.1)

All four are tracked in [SECURITY.md](./SECURITY.md); listing here
because v0.1.1 consciously does not close them:

- **Backstage guest auth** → replace with basic auth middleware or a
  real provider. Targets v0.2.0 (Mid-size team segment of Phase 2).
- **Permissive CORS** on the engine → env-configured origin list.
  Resolves naturally when the Backstage backend proxy lands (Phase 2,
  Platform team segment).
- **In-memory storage only** → Postgres backend is a Phase 2 item
  (Mid-size team segment).
- **No first-party auth on `/v1/*`** → v0.1.x assumes a trusted
  reverse proxy upstream. Full auth layer is Phase 2 cross-cutting
  work.

### Deferred ecosystem upgrades (re-evaluation triggers)

Closed Dependabot PRs that need specific upstream readiness before
re-opening. Listed here so future maintainers don't re-attempt them
prematurely.

- **Node 24 LTS migration** (currently on Node 20). Closed PRs
  #16 (`node:25-bookworm-slim`) and #23 (`@types/node@25`) — paired.
  - Blocked on: `node:24-bookworm-slim` Docker image availability
    (Node 24 LTS ships October 2026; image typically lands within
    a week of release).
  - Why we skipped Node 25: odd-numbered, ~6-month support window,
    EoL April 2027 — not viable for a production supply-chain tool.
  - Side issue surfaced during the test: Node 25 dropped bundled
    corepack from official images, breaking our Yarn 4 activation
    in `backstage/examples/app/Dockerfile`. Won't recur on Node 24
    LTS (corepack restored).
  - Re-evaluation trigger: `docker manifest inspect node:24-bookworm-slim`
    succeeds AND `@backstage/cli` declares Node 24 in its
    engines field.

- **React 19 + react-router 7 migration**. Closed PR #19 (the react
  group bump bundling `react`, `react-dom`, `@types/react`,
  `@types/react-dom`, `react-router-dom`).
  - Blocked on: `@backstage/core-components` upstream release that
    accepts `react-markdown@10+`. React 19 removed the global `JSX`
    namespace; `react-markdown@8.0.7` (current pin via
    core-components `^8.0.0`) still references it, producing 3 tsc
    errors in node_modules with no in-repo fix path.
  - Forcing `react-markdown@10` via `yarn resolutions` risks
    breaking Backstage's own markdown rendering (component prop
    shape changed across the major).
  - Re-evaluation trigger: the next `@backstage/core-components`
    major (currently 0.18.x) accepts `react-markdown@^10` in its
    peerDependencies.

---

## Phase 2 — expansion (target: Q2–Q3 2026)

Grouped by the user segment the work benefits, plus a cross-cutting
section for infrastructure that touches all three.

### Solo developer segment

- **`rampart watch` daemon mode.** Phase 1's CLI is one-shot
  (`scan → SBOM`). Phase 2 adds a long-running local daemon that:
  - Polls IoC feeds (OSV.dev, Socket.dev advisory stream) on a
    configurable interval (default hourly).
  - Re-scans the lockfiles it was pointed at; diffs against the last
    scan; opens a local incident on any new match.
  - Emits desktop notifications (macOS Notification Center, Linux
    libnotify, Windows toast — via
    [beeep](https://github.com/gen2brain/beeep)).
  - CLI shape: `rampart watch --interval 1h --feeds osv,socket
    --lockfile path/to/package-lock.json`.
  - **Acceptance**: `brew install rampart && rampart watch` works
    on a clean macOS laptop, emits a native notification within
    60 seconds of an IoC being published in a test feed.

### Mid-size team segment

- **Multi-repo aggregation.** Single centralised daemon, org-wide
  monitoring.
  - SBOM ingestion via webhook (GitHub `pull_request.closed` event)
    or scheduled poll of each repo's default branch.
  - Single `IncidentDashboard` correlating across the fleet; blast
    radius becomes an org-level query.
  - Per-repo `team_ref` on components; incidents route to the team
    that owns the component before a human triages.
  - **Acceptance**: ten repositories wired to a single daemon;
    blast-radius query returns the correct affected set in under
    500 ms on a 10-component-per-repo fleet.

- **Postgres storage backend.** The Phase 1 in-memory store is a
  single-process, lost-on-restart design (by deliberate scope).
  Phase 2 ships:
  - `engine/internal/storage/postgres` against the existing
    `Storage` interface + the shared `storagetest` contract suite.
  - Migration tooling (goose or atlas).
  - The `postgres` service that was deliberately absent from the
    Phase 1 Compose stack (see the comment at the top of
    `docker-compose.yml`).
  - Benchmarks: ingestion rate, query latency at p50/p95/p99,
    storage footprint on realistic fleet sizes.
  - **Acceptance**: `docker compose up postgres engine` boots and
    persists 1,000 components + 100 incidents across a full
    container restart with zero data loss.

### Platform team segment

- **Backstage backend proxy fully wired.** Today the frontend's
  `RampartClient` calls the engine directly
  (`rampart.baseUrl = http://localhost:8080`); the `rampart-backend`
  plugin mounts `/api/rampart/v1/*` but the frontend doesn't route
  through it.
  - Swap the client to relative URLs (`/api/rampart/v1/...`) so
    cross-origin and browser-DNS concerns disappear.
  - Remove the wildcard CORS middleware on the engine.
  - **Acceptance**: Backstage install docs carry the phrase "no
    CORS config needed" and production deployments work behind a
    standard reverse proxy without engine-side origin allow-list.

- **Backstage image slimming pass 2.** Current image is 773 MiB
  (Phase 1 prune-stage refactor brought it down from 1.23 GiB).
  Target: under 400 MiB.
  - Techniques to evaluate: `node:20-alpine` base (weighed against
    better-sqlite3's native rebuild — see ADR-0008 edge case),
    aggressive `@backstage/cli` dep-tree prune, multi-target
    `yarn workspaces focus --production --all`.
  - **Acceptance**: `docker images rampart-backstage` reports
    < 400 MiB; cold-boot-to-200-on-`/` in under 2 seconds on a
    GitHub Actions runner.

- **Catalog-sync real push.** The `CatalogSync` tick loop in
  `rampart-backend` currently logs a placeholder. Phase 2 pushes
  Backstage `Component` entities to `/v1/components` on every tick,
  keeping the engine's catalog in step with Backstage's.

### Cross-cutting

- **rampart-native Phase 2 re-evaluation.** Phase 1 measured two
  wire shapes (JSON envelope, binary envelope) and found no
  throughput crossover (ADR-0005 kept the sidecar opt-in as an
  isolation feature). Phase 2 evaluates `SCM_RIGHTS` + mmap
  content handoff + binary response (FlatBuffers or bincode).
  - Hypothesis: crossover emerges on 100+ MiB lockfiles where the
    copy cost dominates parse cost.
  - Method: same seven-fixture benchmark methodology as Phase 1,
    Phase 2 implementation as the IUT.
  - Decision tree:
    - **Crossover measured** → promote sidecar to default (drop
      the opt-in flag); keep it as the fast path.
    - **No crossover** → opt-in keeps the isolation value; update
      ADR-0005 with the Phase 2 verdict.
    - **Marginal improvement** → call it explicitly; ship the
      measured number, let operators decide.

- **Parser ecosystem expansion.** Phase 1 shipped npm
  `package-lock.json` v3. Phase 2 adds:
  - Go modules (`go.sum` + `go.mod`).
  - Cargo (`Cargo.lock`) — the Rust sidecar crate already speaks
    TOML.
  - PyPI (`requirements.txt`, `poetry.lock`, `uv.lock`).
  - Maven (`pom.xml`, Gradle lockfile).
  - Each ecosystem: separate IoC namespace, separate matcher
    extension, parity test if a Rust implementation exists. Parser
    layout (`engine/sbom/<eco>`) already separates npm from the
    rest; new ecosystems slot in as peers.

- **Frontend `IncidentDashboard` depth.** Phase 1 ships a table
  view with ID / state / opened-at / matching IoC. Phase 2 adds:
  - Detail drawer per incident with timeline (state transitions,
    remediation log).
  - Blast-radius visualisation (affected-component graph).
  - Search + filter (by state, ecosystem, date range, component
    owner).

### Publishing (Phase 2)

- **Homebrew tap for the CLI.** `brew tap ugurcan-aytar/rampart`
  → `brew install rampart`. goreleaser can emit the formula; the
  tap itself is a one-file repo.
- **GitHub Marketplace listing for the Action.**
  `integrations/github-action/` ships an `action.yml` ready for
  `@v1` tagging. Marketplace metadata (icon, color, categories)
  picks up from the action's branding block.

---

## Phase 3 — vision (exploratory, 3–12 months)

Direction, not a backlog. Selection depends on community traction and
observed usage patterns. None of these are commitments.

- **LLM integration (opt-in).** Three concrete uses:
  - *Triage summary*: one-paragraph narrative of a fresh incident —
    affected services, likely remediation direction, similar prior
    incidents.
  - *PR description generation*: "pin axios to 1.10.5 to block
    shai-hulud propagation" + rationale + test-plan checklist.
  - *Incident narrative*: human-readable story assembled from the
    append-only event log.
  - **Constraints**: core determinism must stay intact; feature is
    opt-in per deployment (`rampart.llm.enabled=true`, default
    false); local-only by default (no remote API call unless the
    operator wires one in); no training on rampart telemetry, ever.

- **Wasm-compiled parser.** Stronger alternative to process isolation.
  - Hostile lockfile can't crash the engine (wasm memory safety +
    resource caps).
  - Trade-off: wasm runtime overhead vs the `SCM_RIGHTS` process
    isolation Phase 2 evaluates. Pick after the Phase 2 sidecar
    verdict is in.

- **Container-per-parse (gVisor / Kata).** If Phase 2 shows process
  isolation is insufficient against hostile supply-chain inputs,
  kernel-level sandboxing is the next rung. Heavier, but justifies
  itself against the `shai-hulud`-class threat model.

- **Webhook / SIEM integrations beyond Slack.** Splunk HEC,
  Datadog Events API, PagerDuty Events v2, Opsgenie. Same pattern
  as `integrations/slack-notifier`: separate Go binary, SSE consumer,
  dry-run default.

- **Multi-source IoC feed federation.** Phase 1 has a single
  internal feed. Phase 3 aggregates OSV.dev, Socket.dev, Snyk
  Advisor, GitHub Advisory Database. Conflict resolution: same
  CVE seen in multiple feeds, aggregate severity, per-feed
  credibility weighting.

- **Publisher-anomaly detector** (`IoCKindPublisherAnomaly`).
  The IoC shape is already wired (Phase 1's matcher returns no-op
  for this kind). Phase 3 builds the publisher graph:
  maintainer email drift, OIDC regression, version-jump anomalies.
  Ingestion from the npm API + GitHub release metadata. Real signal
  source, not a toy.

- **Enterprise auth.** OIDC / SAML provider integration
  (Keycloak, Okta, Auth0). RBAC scoping components and incidents
  by team. Audit log of who transitioned which incident, when,
  who approved. SOC 2 / ISO 27001 preparedness.

- **Wasm plugin sandbox for third-party detectors.** Operators ship
  bespoke IoC logic as a wasm module with a limited-capability API.
  Rampart core stays insulated from plugin bugs. Opens the door to
  an opt-in community plugin marketplace.

- **Roaring-bitmap blast-radius index.** Phase 1's blast-radius
  query is O(IoCs × SBOMs). Phase 3 builds a bitmap per
  `(ecosystem, package, version)` that answers the query in
  O(|affected components|). Target: sub-500 ms on a 100k-component
  fleet.

---

## Non-goals (what rampart will not do)

This is a discipline doc. New feature proposals get checked against
it. Saying no early is cheaper than walking it back later.

- **No cloud callbacks.** Rampart emits no telemetry — not to
  rampart maintainers, not to a cloud service, not anywhere. User
  data stays on the user's machine / cluster.
- **No vendor lock-in.** Rampart is OSS, self-hostable, portable
  across standard infrastructure. There will never be a
  cloud-only SaaS mode.
- **No LLM in the core path.** LLM features are strictly opt-in
  enhancements (Phase 3), never required for rampart's core
  function — ingest → match → open incident.
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

---

## Phase exit criteria

- **v0.1.1 ships** when all three blockers (npm trusted publishing,
  SHA-pin sweep, Node 20 deprecation path) are done + the four
  known gaps are tracked in their respective issue tickets.
- **Phase 2 ends** when each user segment has a production-ready
  install path (solo `rampart watch`, mid-size Postgres + multi-repo,
  platform Backstage backend proxy) + the rampart-native crossover
  verdict is documented (promote / keep opt-in / remove).
- **Phase 3 ends** — N/A. Ongoing, adoption-driven. Items graduate
  into Phase 4+ scoping when the signal is strong enough.

---

*Last updated: 2026-04-22.*
