# Roadmap

Phase boundaries are about architecture, not calendar. Phase 1 is the
first working vertical slice that delivers value to all three user
segments (solo dev, mid-size team, platform team). Phase 2 is the
features that Phase 1 explicitly deferred because they're more work
than the core idea needs to prove itself. Phase 3 is speculation — items
that make sense if the project survives contact with real users.

---

## Phase 1 — the first vertical slice (shipped)

Everything in the [README status table](./README.md#what-ships-in-phase-1)
is in. The rough edges are documented in
[Production deployment notes](./README.md#production-deployment-notes).

---

## Phase 2 — pick-up list

Grouped by user segment because that's the shape of the actual work,
not by delivery order.

### Solo developer

1. **`rampart watch` daemon mode.** Phase 1's CLI is one-shot:
   `scan → SBOM`. A local daemon that polls an IoC feed (OSV.dev or
   equivalent) hourly, holds the same SBOMs scanned by
   `rampart scan`, and emits desktop notifications turns the CLI into
   something a developer can leave running. Implementation: extend
   `cli/cmd/rampart` with a `watch` subcommand that spawns a tiny
   local HTTP server, stores SBOMs in `~/.rampart/`, and uses
   [beeep](https://github.com/gen2brain/beeep) for cross-platform
   notifications.

### Mid-size team

2. **Multi-repo aggregation.** A single daemon ingests SBOMs from
   N repositories via webhook or pre-commit; a single
   `IncidentDashboard` correlates across the fleet. Requires real
   storage (Postgres) and a permissions model that scopes components
   to teams. The Backstage catalog-sync already pushes component
   refs with owners; Phase 2 adds `team_ref` filters on the list
   endpoints and an incident-routing rule engine.

3. **Postgres storage backend.** The engine's in-memory store is a
   Phase 1 honesty tradeoff — single process, lost on restart.
   Phase 2 ships `engine/internal/storage/postgres` against the
   existing `Storage` interface + `storagetest` contract suite,
   migration tooling (goose or atlas), and the `postgres` service
   that was deliberately absent from the Phase 1 Compose stack
   (see the comment at the top of `docker-compose.yml`).

### Platform team

4. **Backstage backend proxy fully wired.** Today the frontend's
   `RampartClient` calls the engine directly
   (`rampart.baseUrl = http://localhost:8080`); the `rampart-backend`
   plugin mounts `/api/rampart/v1/*` but the frontend doesn't route
   through it. Phase 2 swaps the client to relative URLs so
   cross-origin and browser-DNS concerns disappear. Removes the
   wildcard CORS middleware on the engine.

5. **Catalog-sync real push.** The `CatalogSync` tick loop in
   `rampart-backend` currently logs a placeholder; Phase 2 pushes
   Backstage `Component` entities to `/v1/components` on every tick,
   keeping the engine's catalog in step with Backstage's.

### Infrastructure / operations

6. **pinact SHA-pinning sweep.** Workflow action refs currently pin
   to immutable semver tags (`actions/checkout@v4.2.2`). zizmor-grade
   compliance wants full commit SHA pins. pinact is the mechanical
   sweep tool; Dependabot maintains freshness afterwards via its
   `digest` versioning mode.

7. **Backstage base image slimming pass 2.** The Phase 1 prune-stage
   refactor brought the image from 1.23 GiB to 773 MiB. Further
   reductions (alpine base, more aggressive
   `yarn workspaces focus --production --all`, multi-target prune)
   risk breaking better-sqlite3's native rebuild (ADR-0008 edge case).
   Deep ROI < 500 MiB is real work and not Phase 1-scoped.

8. **rampart-native Phase 2 re-evaluation.** Phase 1 measured two
   wire shapes (JSON envelope, binary envelope) and found no
   throughput crossover; ADR-0005 kept the sidecar opt-in as an
   isolation feature. Phase 2 evaluates `SCM_RIGHTS` + mmap content
   handoff + binary response (FlatBuffers / bincode) — the last
   lever that could realistically close the gap. If it does, the
   sidecar becomes default; if not, the honest action per the ADR
   is to delete it from the Compose profile and keep it only as a
   CLI opt-in for operators who want the isolation property.

### Publishing

9. **npm trusted publishing for the three Backstage plugins.** The
   release workflow already shape-wires `npm publish --provenance`
   under OIDC; waiting on the first tagged release to fire it for
   real. `@ugurcan-aytar/backstage-plugin-rampart` +
   `-backend` + `scaffolder-rampart-actions`.

10. **Homebrew tap for the CLI.** `brew tap ugurcan-aytar/rampart`
    → `brew install rampart`. goreleaser can emit the formula; the
    tap itself is a one-file repo.

11. **GitHub Marketplace listing for the Action.**
    `integrations/github-action/` ships an `action.yml` ready for
    `@v1` tagging. Marketplace metadata (icon, color, categories)
    picks up from the action's branding block.

12. **First release tag.** `v0.1.0` fires `release.yml`: four Go
    binaries via goreleaser, four cargo cross-compile targets, five
    container images to `ghcr.io`, three npm packages via trusted
    publishing, all cosign-signed and SBOM-attested per artifact.

---

## Phase 3 — speculation

Not commitments.

- **LLM incident triage (opt-in).** An LLM summarises an incident's
  blast radius, suggests a remediation playbook, drafts a PR message.
  Opt-in per deployment; local model first (vLLM + Llama 3.x), cloud
  via pluggable provider. No training on customer data, ever.
- **wasm-compiled parser.** Phase 2 sidecar evaluation's alternative:
  run the Rust parser as a wasm module under wasmtime inside the Go
  engine's process. Container-per-parse isolation without the IPC
  cost.
- **Publisher-anomaly detector (IoCKindPublisherAnomaly).** The IoC
  shape is already wired (Phase 1's matcher returns no-op for this
  kind). Phase 3 builds the publisher graph: maintainer email drift,
  OIDC regression, version-jump anomalies. Ingestion from the npm
  API + GitHub release metadata. Real signal source, not a toy.
- **Container-per-parse (gVisor / Kata).** If the Phase 2 sidecar
  evaluation shows that even process isolation is insufficient
  against hostile supply-chain inputs, kernel-level sandboxing is
  the next rung. Heavier, but justifies itself against the
  `shai-hulud`-class threat model.
- **Roaring bitmap blast-radius index.** Phase 1's blast-radius
  query is O(IoCs × SBOMs). Phase 3 builds a bitmap per
  (ecosystem, package, version) that answers the query in O(|affected
  components|). Target: sub-500 ms on a 100 k-component fleet.
- **Additional ecosystems.** pypi (`poetry.lock`, `Pipfile.lock`,
  `requirements.txt`), cargo (`Cargo.lock`), go (`go.sum`),
  yarn.lock, pnpm-lock.yaml. The parser package layout already
  separates `engine/sbom/npm` from the rest; each new ecosystem
  slots in as `engine/sbom/<eco>` with its own Go + (optional)
  Rust parity implementation.
- **IoC feed integrations beyond OSV.** GHSA, Socket, custom feeds.
  Each is a small Go client that POSTs to `/v1/iocs` on a
  subscription.
