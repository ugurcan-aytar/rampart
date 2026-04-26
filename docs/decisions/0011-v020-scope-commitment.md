# ADR-0011: v0.2.0 scope commitment

**Date:** 2026-04-23
**Status:** Accepted
**Authors:** ugurcan-aytar

## Context

v0.1.0 shipped 2026-04-21 as a feature-complete but posture-light release. The 2026-04-22 housekeeping session closed 21 Dependabot PRs + 35 security alerts, establishing a clean baseline. With v0.1.1 narrowed to pure release-engineering debt (npm trusted publishing, SHA-pin tail, Node 20 deprecation), the next major question was "what ships in v0.2.0?"

Rampart's competitive position at v0.1.0:
- Behind Socket/Snyk on: publisher-anomaly detection, multi-ecosystem coverage, production-grade auth, persistent storage.
- Level with them on: self-hosted deployment, blast-radius queries, SBOM attestation.
- Ahead of them on: incident-based workflow (state machine vs alert list), OSS + no-telemetry posture.

The competitive gap is real and closeable. v0.2.0 is the release that closes it.

## Decision

v0.2.0 ships six coherent themes simultaneously:

- **Theme A — Production-grade security posture.** JWT auth on `/v1/*`, env-configured CORS allow-list, Backstage guest auth replaced with real OAuth provider.
- **Theme B — Backstage architecture completion.** Frontend client routes through backend proxy (relative URLs), CatalogSync real push, "no CORS needed" install story.
- **Theme C — Multi-ecosystem parser expansion.** Go modules, Cargo, PyPI, Maven. Rust sidecar parity for Go and Cargo (TOML + Go modules already fit native). PyPI and Maven single-engine (Wasm parity moves to v0.5.0).

  **Implementation note (PR #37, 2026-04-26)**: Theme C ships without `IoCKindGoModule` / `IoCKindCargo` enum constants originally listed in this ADR. Rationale: rampart's existing type system separates *behavioural* IoC kinds (PackageVersion / Range / Anomaly) from the *ecosystem* dimension (a separate `Ecosystem` field on the IoC). The matcher already dispatches on ecosystem; adding ecosystem-named IoC kinds would duplicate the dimension. The ADR's enum proposal reflected an earlier draft of the type system; PR #37 preserves the cleaner factoring instead of mechanically applying the spec.
- **Theme D — Postgres storage backend.** In-memory store becomes a dev-only mode; production deployments get persistent storage with migration tooling (goose).
- **Theme E — Frontend depth.** IncidentDashboard gets detail drawer (timeline + IoC + affected components + remediation log), blast-radius graph visualisation (@react-flow), search + filter with URL state.
- **Theme F — Publisher-anomaly detector.** Three concrete anomaly types (maintainer email drift, OIDC regression, version jump). npm API + GitHub API ingestion, Postgres-backed publisher history table.

Six themes ship together because:
1. Theme B (proxy wiring) depends on Theme A (CORS can be removed only if proxy replaces direct-engine calls).
2. Theme F (publisher anomaly) depends on Theme D (publisher history needs persistence).
3. Theme E (detail drawer) is best demoed against the richer data model Themes A, D, F produce.
4. Partial ship (e.g. A + B without C) produces a release story that's hard to narrate — "rampart now has auth" is weak compared to "rampart is now production-competitive."

## Alternatives considered

### Alternative 1: Ship themes incrementally (v0.2.0 → v0.2.5)

Each theme as its own minor release. Faster individual ship cycles.

Rejected because:
- No coherent narrative per release ("v0.2.0 adds auth" doesn't move the needle vs Socket/Snyk).
- Dependency chain between themes creates sequencing pain — B before A is wrong, F before D is wrong. Stacking five dependent minor releases over 3 months is more overhead than one larger release over 8 weeks.
- User-facing upgrade path is noisier — operators re-upgrade 5x instead of 1x.

### Alternative 2: Fold v0.2.0 themes into a distant 1.0 release

Ship all v0.2.0 themes + enterprise auth + sandboxing + everything else together as the first production-committed release.

Rejected because:
- 1.0 implies an API stability guarantee (SemVer contract). We haven't stress-tested `/v1/*` under load; the roaring-bitmap work (v0.9.0 theme) may reshape the blast-radius query shape. Committing to API stability before that measurement is premature.
- Enterprise auth, plugin sandboxing, roaring-bitmap indexing are each distinct bodies of work. Folding them all into one mega-release turns a bounded 8-week push into an open-ended multi-quarter one.
- Adoption pattern: users should be able to try v0.2.0 without committing to "this is my production tool with a support contract." v0.2.x through v0.9.x preserve experimentation space. 1.0 stays deferred per the ROADMAP's "no premature 1.0" discipline.

### Alternative 3: Defer publisher-anomaly detector to v0.3.0

Ship Themes A–E in v0.2.0; publisher anomaly is v0.3.0's headline.

Rejected because:
- Publisher anomaly is rampart's most visible behavioural differentiator. Without it, v0.2.0's story is "Snyk-like but self-hosted" instead of "Snyk-like AND has novel detection."
- The publisher-graph data model is best designed against the same Postgres schema as Theme D; splitting them produces a migration in v0.3.0 that could be avoided.
- Pairing F with E is valuable: the detail drawer in E surfaces publisher-anomaly context (maintainer history chart, release cadence). Shipping E without F leaves a empty-looking panel.

## Consequences

Positive:
- v0.2.0 release story is competitive against Socket/Snyk on a feature-matrix basis.
- Postgres + JWT auth unblock every downstream theme (multi-repo aggregation, CatalogSync, LLM features all assume persistent storage and per-user auth).
- Frontend depth (E) produces screenshot-worthy artefacts for launch content.

Negative:
- 6-theme release is larger than v0.1.0 was. Risk of scope creep — each theme's acceptance criteria must be frozen before theme work starts, and in-release change requests go to v0.2.1.
- Upgrade path for v0.1.x operators is non-trivial (guest auth → real OAuth, in-memory → Postgres). Migration guide in the release notes is a hard requirement.

Neutral:
- LLM features (v0.4.0) stay out of v0.2.0 scope. Pressure to "add AI" before the deterministic foundation is solid is deliberately resisted.

## References

- ROADMAP.md v0.2.0 section (authoritative acceptance criteria).
- ADR-0005 (rampart-native opt-in default) — the isolation-vs-throughput decision that Theme N (v0.5.0) re-evaluates.
- ADR-0008 (Backstage image prune stage) — referenced by Theme D migration path.
- ADR-0009 (CI/CD pipeline architecture) — gating vs advisory split that the auth changes (Theme A) must preserve.
