# ADR-0012: Auth boundary at engine, not at Backstage proxy

**Date:** 2026-04-23
**Status:** Accepted
**Authors:** ugurcan-aytar

## Context

Theme B1 (frontend client → backend proxy) introduced a question: where should authentication happen?

Two candidate auth points in the request chain:
1. Backstage frontend → Backstage backend `/api/rampart/v1/*` (Backstage's own auth policy)
2. Backstage backend → engine `/v1/*` (engine's A1 JWT middleware from ADR-0011 Theme A)

Initial implementation (commit d53dbdc) attempted to enforce Backstage auth at point 1, requiring signed-in users to access the proxy. This created a double-authentication pattern: Backstage auth at the proxy + engine JWT at the engine boundary.

Side effect: `make demo-axios` required guest auth bypass to work, which conflicts with Theme A3's future removal of `guest.dangerouslyAllowOutsideDevelopment`.

## Decision

The engine is the authoritative auth boundary. Backstage proxy routes use `addAuthPolicy({ allow: 'unauthenticated' })`.

Request flow:
1. Frontend calls `/api/rampart/v1/*` (same-origin, no Backstage auth required)
2. Backend proxy forwards to `http://engine:8080/v1/*` with service-to-service JWT token (RAMPART_ENGINE_AUTH_TOKEN)
3. Engine A1 middleware validates JWT, enforces scope, returns data or 401

Why engine is the right boundary:
- Engine is the single source of truth; its API must be secure regardless of what front-ends exist (Backstage today, standalone clients tomorrow, rampart CLI, third-party integrations).
- Backstage auth would duplicate enforcement without adding security — if the engine token is stolen, Backstage auth doesn't help.
- Cleaner deployment story: operators reason about one auth layer (engine's JWT), not two.
- Standalone engine deployments (no Backstage) retain the same auth guarantee.

## Alternatives considered

### Alternative 1: Backstage auth at proxy + engine JWT

Double-enforcement at both points. Users sign into Backstage; Backstage backend mints an engine token per request.

Rejected because:
- Duplicates enforcement without security gain.
- Couples engine's security posture to Backstage's auth configuration — a bug in Backstage auth would leak engine access.
- Breaks the non-Backstage client story (CLI, third-party integrations must pick a different auth path).

### Alternative 2: Backstage auth only, pass-through proxy

Backstage enforces auth, engine trusts Backstage implicitly (network-level trust).

Rejected because:
- Engine's A1 middleware exists precisely to eliminate implicit-trust assumptions. v0.1.x's "trusted reverse proxy upstream" footnote is exactly what Theme A removed.
- Standalone engine deployments have no Backstage to trust.

## Consequences

Positive:
- Single auth layer, clear boundary.
- Engine works identically whether fronted by Backstage or standalone.
- Aligns with Theme A3 (Backstage auth becomes operator's choice; engine auth is always on).

Negative:
- Service-to-service token (RAMPART_ENGINE_AUTH_TOKEN) is a new operational secret to manage.
- If an attacker gets the service-to-service token, they bypass Backstage user identity — audit log loses per-user attribution on Backstage-originated requests.

Mitigation for the per-user attribution gap:
- v0.7.0 (Theme S — RBAC + Theme T — Full audit log) will propagate signed-in user identity from Backstage to engine via a trusted-claims header pattern. For v0.2.0, the audit trail records the service identity; per-user attribution lands with RBAC.

## References

- ADR-0011 (v0.2.0 scope commitment) — Theme A and Theme B acceptance criteria.
- Commit a6ab62f (`fix: addAuthPolicy({ allow: 'unauthenticated' })`) — implementation of this decision.
- Commit d53dbdc — the rejected initial attempt at Backstage auth at proxy.
- ROADMAP.md v0.7.0 themes R/S/T — per-user identity propagation lands with RBAC.
