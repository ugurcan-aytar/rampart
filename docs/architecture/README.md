# Architecture documentation

Reserved for extended architecture references — domain model
deep-dive, state machine diagrams, storage adapter pattern, the
"how to add a new consumer" walkthrough.

The directory is intentionally empty at v0.1.0. The authoritative
architecture references for v0.1.0 are:

- Root [`ARCHITECTURE.md`](../../ARCHITECTURE.md) — narrative
  overview of the engine, parser, sidecar, and consumer plugins.
- The OpenAPI contract at
  [`schemas/openapi.yaml`](../../schemas/openapi.yaml) — the
  authoritative wire shape for every consumer.
- Architecture Decision Records under
  [`docs/decisions/`](../decisions/) — the **why** behind
  load-bearing choices.
- The benchmark at
  [`docs/benchmarks/sbom-parser.md`](../benchmarks/sbom-parser.md)
  — Go vs Rust parser throughput across seven fixture sizes.

Extended architecture documentation is planned for a future
release.
