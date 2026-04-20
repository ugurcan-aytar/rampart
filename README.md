# rampart

> A rampart for your supply chain.

`rampart` is a supply chain incident response engine for the npm, pypi, cargo, and Go ecosystems. When a package is compromised, it answers the three questions that matter: **which services are affected**, **who owns them**, and **what's the playbook**.

The full README — including install instructions for the CLI, Backstage plugin, GitHub Action, and Slack notifier — lands in Adım 9. The sections below are a Phase-1 skeleton.

## Quickstart (default: embedded Go parser)

```bash
docker compose up -d
# engine listens on :8080 ; parser.strategy=go (no sidecar)
```

The default profile runs a single container (`engine`) with the
embedded Go lockfile parser. No UDS volume, no extra processes, no
sidecar to deploy. This is the recommended shape for most
deployments — benchmark data at [docs/benchmarks/sbom-parser.md](./docs/benchmarks/sbom-parser.md)
shows the Go path wins on every lockfile size Phase 1 measured.

## Experimental: Rust parser sidecar (opt-in)

The `native` profile swaps in `rampart-native`, a separate Rust
process that parses lockfiles over a Unix Domain Socket. It is **not**
a performance win (benchmarks show Go is ~1.8× faster at 100k
packages even with the binary-envelope optimisation) — its value is
**defense-in-depth parser isolation**: a malicious lockfile that
panics or OOMs the parser can only crash the sidecar, after which the
engine logs a warn line and falls back to the Go parser.

Enable it when ingesting SBOMs from untrusted sources:

```bash
RAMPART_PARSER_STRATEGY=native docker compose --profile native up -d
# engine + rampart-native are both up; engine talks UDS to the sidecar
```

Architecture: [ADR-0005](./docs/decisions/0005-no-cgo-rust-via-uds.md)
records both the original decision and the Final Decision (opt-in) the
benchmark drove. Wire protocol: [schemas/native-ipc.md](./schemas/native-ipc.md).

## Table of contents

- [ARCHITECTURE.md](./ARCHITECTURE.md) — domain model, state machine, storage design
- [DEPS.md](./DEPS.md) — every runtime dependency, justified
- [SECURITY.md](./SECURITY.md) — threat model, responsible disclosure
- [CONTRIBUTING.md](./CONTRIBUTING.md) — dev setup, commit conventions
- [ROADMAP.md](./ROADMAP.md) — Phase 1 / 2 / 3 scope
- [schemas/openapi.yaml](./schemas/openapi.yaml) — API contract (single source of truth)
- [docs/decisions/](./docs/decisions/) — ADRs 0001–0008

## License

MIT. See [LICENSE](./LICENSE).
