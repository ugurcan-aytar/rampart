# ADR-0014: Anomaly → IoC bridge design

**Date:** 2026-04-27
**Status:** Accepted
**Authors:** ugurcan-aytar

## Context

Theme F2 (PR #39) shipped publisher anomaly detection but deferred IoC emission per the ADR-0011 amendment (commit `610ef6b`). Reason: existing `IoCKindPublisherAnomaly` body shape was maintainer-keyed (Theme D era), F2 anomalies are package-keyed (ADR-0013 Snapshot vs Profile split). Bridging required either semantic strain (PublisherName=packageRef) or a new IoC body variant.

Without IoC emission, anomalies sit in a separate `/v1/anomalies` endpoint and never flow into the incident state machine. Operators see anomalies but can't triage them through the same workflow as IoC-matched incidents (state transitions, remediation log, audit trail). Two parallel mental models defeat the unified-UX goal of Theme F.

## Decision

Add a new IoC body variant `IoCBodyAnomaly` to `schemas/openapi.yaml`. The variant is package-keyed and carries the F2 anomaly fields:

- `kind` — `AnomalyKind` enum (matches Theme F2: `new_maintainer_email` / `oidc_regression` / `version_jump`)
- `confidence` — `Confidence` enum (`high` / `medium` / `low`)
- `explanation` — string, human-readable
- `evidence` — object, detector-specific structured data
- `package_ref` — `<ecosystem>:<name>` (Theme F1 convention)

`IoCKindPublisherAnomaly` enum is reused as the dispatch key. A consumer (matcher, frontend) inspects which body variant slot is populated to decide rendering / matching behaviour:

- `IoC.publisherAnomaly` populated → maintainer-keyed (Theme D legacy, no shipping detector)
- `IoC.anomalyBody` populated → package-keyed (Theme F2)

Anomaly orchestrator gains an IoC emission step: after `SaveAnomaly`, the orchestrator builds an IoC with `Kind=PublisherAnomaly` + `AnomalyBody=...` and calls a small `engine/internal/iocforward` helper that upserts the IoC + forward-matches against every stored SBOM + opens incidents for matches. The `iocforward` package is the shared service `api.Server.SubmitIoC` already needed; this PR extracts it so the orchestrator can call it without depending on the api package.

The matcher's `IoCKindPublisherAnomaly` case (previously a no-op TODO) is updated: when `IoC.AnomalyBody` is populated, match SBOM packages whose `<ecosystem>:<name>` equals `AnomalyBody.PackageRef`. The maintainer-keyed `PublisherAnomaly` slot still no-ops — no shipping detector produces it.

## Alternatives considered

### Alternative B: Extend existing `IoCBodyPublisherAnomaly` to be package-keyed

Mutate the existing body shape to add `package_ref` + anomaly fields, deprecate maintainer-keyed fields.

Rejected because:
- Breaking shape change for consumers expecting maintainer-keyed payload (Theme D era code, even if no detector ships).
- No clean migration path — old data in postgres `iocs` table would need rewrite.
- Maintainer-keyed concerns (Profile aggregate from ADR-0013) still belong in some future IoC variant; deleting that shape now blocks v0.4.0+ work.

### Alternative C: Skip IoC emission, keep anomalies as a separate signal

Anomalies live only in `/v1/anomalies`; never flow into incidents.

Rejected because:
- Operators have two parallel triage workflows (incidents from IoC matcher, anomalies from `/v1/anomalies`). Mental model fragmentation.
- Remediation log + state machine + audit trail all live on incidents. Anomalies miss the entire response infrastructure.
- Frontend would need a second list view ("Anomalies dashboard") on top of `IncidentDashboard`. Doubles UI surface.

## Consequences

Positive:
- Anomalies flow through the standard incident workflow — single mental model for operators.
- Matcher dispatch unchanged at the enum level; the body-variant probe is local to the case.
- Frontend can render anomaly incidents distinctly via body-variant detection (the F3 frontend follow-up consumes this).
- F2 orchestrator emission completes the F1+F2+F3 chain at the API layer.
- The extracted `engine/internal/iocforward` package becomes the natural home for IoC-feed integrations (Theme J in the v0.3.0+ window).

Negative:
- Two `IoCBody*` variants for the same `IoCKind` is non-orthogonal — schema readers see redundancy. Acceptable: `kind` enum is the dispatch key, body variants are payload shapes; the variant selector lives next to the rendering logic, not the type system.
- Future IoC body variants for new anomaly types (Phase 3 publisher graph, Phase 4 multi-source feed) compound this pattern — needs disciplined naming. Convention to set: body variant name encodes its data model (`anomalyBody` = package-keyed time-series anomaly, `publisherAnomaly` = maintainer-keyed aggregate).

## References

- ADR-0011 — Theme F scope commitment
- ADR-0013 — Publisher Snapshot vs Profile split
- ADR-0011 amendment (commit `610ef6b`) — F2 IoC emission deferral
- PR #39 (Theme F2) — detection logic shipped without IoC emission
- This PR — IoC bridge implementation, completes the F1+F2+F3 chain
