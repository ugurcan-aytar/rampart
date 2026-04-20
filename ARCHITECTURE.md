# Architecture

Written in full in Adım 9. Until then, see:

- [schemas/openapi.yaml](./schemas/openapi.yaml) — API contract (Adım 3)
- [docs/decisions/](./docs/decisions/) — ADRs 0001–0005 (Adım 9)
- [ROADMAP.md](./ROADMAP.md) — what's in scope per phase

## Planned sections

- Domain model — six first-class entities (Component, SBOM, PackageVersion, IoC, Incident, Remediation)
- State machine — `pending → triaged → acknowledged → remediating → closed / dismissed`, idempotent transitions, event-sourced
- Storage — pluggable interface with shared contract tests (memory, SQLite, Postgres)
- Adapter pattern — how Backstage, GitHub Action, CLI, and Slack notifier attach through the same OpenAPI contract
- Performance targets — >100k components indexed, <500 ms blast-radius query, <1 s SSE end-to-end
- Native parser — Rust over Unix Domain Socket, no cgo (ADR-0005)
