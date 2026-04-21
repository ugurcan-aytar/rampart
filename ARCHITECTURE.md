# Architecture

This document covers the shape of rampart at Phase 1 close: the
domain model, how components relate, the event flow from an IoC
arriving to an incident opening, and where the contract boundaries
sit. Design decisions with non-obvious rationale live as ADRs under
[docs/decisions/](./docs/decisions/); this file is the map.

## Domain model

Six first-class entities. Every field that lives on the wire is in
[`schemas/openapi.yaml`](./schemas/openapi.yaml); the Go types that
represent them internally live in `engine/internal/domain/`.

- **Component** — a Backstage-shaped reference (`kind:Component/<ns>/<name>`),
  an owner, and optional tags / lifecycle / annotations. The unit of
  ownership in blast-radius answers.
- **SBOM** — a historical snapshot of the dependencies at a specific
  `(componentRef, commitSHA)` pair. Packages are stored as a flat list
  of [PackageVersion] values; the engine doesn't reconstruct the
  dependency graph. New SBOMs are new records — nothing is mutated.
- **PackageVersion** — ecosystem + name + version + PURL + integrity +
  scope (`dev` / `optional` / `peer`, or nil for runtime prod).
- **IoC** — an Indicator of Compromise, tagged union over three
  kinds: `packageVersion` (exact match), `packageRange` (semver
  constraint via Masterminds/semver), `publisherAnomaly` (Phase 1
  no-op, Phase 3 signal source).
- **Incident** — an IoC × Component pairing after a successful
  match. Carries a state (`pending / triaged / acknowledged /
  remediating / closed / dismissed`), an append-only remediation
  log, and a frozen `AffectedComponentsSnapshot` that does not
  shift when new SBOMs arrive.
- **Remediation** — an audit-log entry on an incident:
  `pin_version / rotate_secret / open_pr / notify / dismiss`. Once
  appended, never deleted.

**What's not a first-class entity on purpose.** Repositories, teams,
users, secret rotations, Jira tickets, PagerDuty incidents — all of
these are scoped *into* rampart from the consumer side (CLI,
Backstage catalog, GitHub Action). The engine never learns them as
primitives; adapters translate at the edge.

## Incident state machine

```
     pending ──▶ triaged ──▶ acknowledged ──▶ remediating ──▶ closed
        │         │              │
        ▼         ▼              ▼
           ... any non-terminal state ...
                      │
                      ▼
                  dismissed
```

- `closed` and `dismissed` are terminal. No transitions out.
- Self-transitions are idempotent no-ops (same state → same state,
  no event emitted, no timestamp touched).
- Illegal transitions (`pending → closed`, for example) return
  409 Conflict with the domain error message included verbatim.
- State logic lives in `domain/incident.go` as `IncidentState.CanTransitionTo`
  + `Incident.Transition`. The HTTP handler is a thin wrapper.

## Event flow — the three useful scenarios

**A new IoC arrives** (feed → `POST /v1/iocs`):
```
POST /v1/iocs → domain.IoC.Validate → storage.UpsertIoC
             → matcher.Evaluate against every stored SBOM
             → for each unique (IoCID, ComponentRef) not already
               open: domain.NewIncident → storage.UpsertIncident
             → events.Publish(IncidentOpenedEvent)
             → events.Publish(IoCMatchedEvent)
SSE subscribers (Backstage dashboard, slack-notifier, custom bridge)
receive incident.opened + ioc.matched frames.
```

**A new SBOM arrives** (CI / scaffolder → `POST /v1/components/{ref}/sboms`):
```
POST /v1/components/{ref}/sboms
  → body.Content (base64 lockfile) → parser.Parse (Go or Rust sidecar)
  → ingestion.Ingest → storage.UpsertSBOM
  → events.Publish(SBOMIngestedEvent)
  → matcher.Evaluate against every stored IoC (retroactive)
  → for each new match: open incident + publish incident.opened
```

**An operator triages an incident** (`POST /v1/incidents/{id}/transition`):
```
POST /v1/incidents/{id}/transition
  → storage.GetIncident
  → domain.Incident.Transition — rejects illegal
  → storage.UpsertIncident
  → events.Publish(IncidentTransitionedEvent)
```

## Component boundaries + the OpenAPI contract

```
  ┌───────────────┐   ┌───────────────┐   ┌───────────────┐
  │ CLI           │   │ Backstage     │   │ GitHub Action │
  │ (Go)          │   │ plugins (TS)  │   │ (composite)   │
  └──────┬────────┘   └──────┬────────┘   └──────┬────────┘
         │                   │                    │
         │  HTTP + SSE       │  HTTP + SSE        │  HTTP
         └───────────┬───────┴────────────┬───────┘
                     ▼                    ▼
         ┌─────────────────────────────────────────┐
         │  schemas/openapi.yaml  (single source)  │
         │    Go server stub  ←  oapi-codegen      │
         │    TS client types ←  openapi-typescript│
         └─────────────────────────────────────────┘
                     │
                     ▼
         ┌─────────────────────────────────────────┐
         │ engine (Go)                             │
         │   internal/api     HTTP + SSE server    │
         │   internal/domain  type-safe core       │
         │   internal/matcher IoC vs SBOM          │
         │   sbom/npm         parser               │
         │   internal/events  in-process bus       │
         │   internal/trust   policy interface     │
         │   ingestion        SBOM lifecycle       │
         │   internal/storage pluggable contract   │
         └──────┬──────────────────────────────────┘
                │ UDS (opt-in, ADR-0005)
                ▼
         ┌─────────────────┐
         │ rampart-native  │
         │ Rust parser     │
         └─────────────────┘
```

**The contract is the file**, not the Go struct or the TS interface.
`schemas/openapi.yaml` is the single source of truth; drift is a CI
failure via `make gen-check`. Consumers only ever import from the
generated types, which keeps the internal Go domain package free to
evolve.

## Storage — pluggable interface

`engine/internal/storage.Storage` is the interface every backend
implements. Phase 1 ships only the in-memory implementation; Phase 2
adds Postgres. A shared contract suite at `storage/storagetest/`
runs against every backend — "passes storagetest" is the only test
for "is this storage correct".

The interface is deliberately shallow: list, get, upsert. No
transactions today; the engine's event bus handles causally-ordered
dispatch in-process. Transactions come when Postgres arrives and
multi-writer races become real.

## The event bus

`engine/internal/events.Bus` is a fan-out broadcaster with per-subscriber
channels. Non-blocking publish (slow subscribers get their channel
closed, not a stuck producer); context-scoped subscriptions that
auto-unsubscribe on `ctx.Done()`.

The bus is in-process — a single Go binary today. Phase 2's multi-repo
aggregation likely replaces this with NATS or Redis streams. The
bus interface is narrow enough that swap-in is localised.

## The Rust sidecar — opt-in, isolation-first

Throughput benchmarks falsified the "Rust is faster on big lockfiles"
hypothesis. The sidecar ships as an opt-in defense-in-depth layer:
parser runs in a separate process, separate language runtime, no
shared memory with the engine's storage. A hostile lockfile can crash
only the sidecar; the engine logs a warn line and falls back to the
embedded Go parser.

Details + measurements in
[ADR-0005](./docs/decisions/0005-no-cgo-rust-via-uds.md) and
[docs/benchmarks/sbom-parser.md](./docs/benchmarks/sbom-parser.md).

## Supply-chain posture (rampart's own)

- Yarn 4 + `enableScripts: false` — Shai-Hulud-class postinstall
  vectors disabled globally, no per-package allowlist (ADR-0008).
- `cargo-deny` with an explicit license allowlist; no unknown
  registries. Configuration at `native/deny.toml`.
- `npm install --ignore-scripts` per release job.
- `govulncheck` reports advisory-only (stdlib CVEs drop monthly;
  blocking every PR drowns the signal).
- `cosign` keyless signing on every release artifact via GitHub OIDC;
  no long-lived signing keys in the repo.
- SBOM attestation per artifact (`syft` → `cosign attest`). Rampart
  can scan its own releases.

## Performance targets (Phase 1 floor)

| Metric | Phase 1 target | Actual |
|---|---|---|
| npm parser, 50 MiB lockfile | < 500 ms | 110 ms (Go) / 199 ms (Rust+IPC) |
| Incident open, 10 k SBOMs | < 1 s end-to-end | sub-100 ms in chain_test.go |
| SSE fan-out, 100 subscribers | no drops under 1 msg/s | contract tested |
| Backstage cold boot | < 30 s | 1 s (post-slimming) |
| Image size (Backstage) | < 1 GiB | 773 MiB |

Phase 2 adds the scale floor (100 k-component fleet in < 500 ms
blast-radius). The bitmap index is the mechanical lever.
