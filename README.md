# rampart

> A rampart for your supply chain.

`rampart` is a supply chain incident response engine for the npm, pypi, cargo, and Go ecosystems. When a package is compromised, it answers the three questions that matter: **which services are affected**, **who owns them**, and **what's the playbook**.

The full README — including install instructions for the CLI, Backstage plugin, GitHub Action, and Slack notifier — lands in Adım 9.

## Table of contents

- [ARCHITECTURE.md](./ARCHITECTURE.md) — domain model, state machine, storage design
- [DEPS.md](./DEPS.md) — every runtime dependency, justified
- [SECURITY.md](./SECURITY.md) — threat model, responsible disclosure
- [CONTRIBUTING.md](./CONTRIBUTING.md) — dev setup, commit conventions
- [ROADMAP.md](./ROADMAP.md) — Phase 1 / 2 / 3 scope
- [schemas/openapi.yaml](./schemas/openapi.yaml) — API contract (single source of truth)
- [docs/decisions/](./docs/decisions/) — ADRs 0001–0005

## License

MIT. See [LICENSE](./LICENSE).
