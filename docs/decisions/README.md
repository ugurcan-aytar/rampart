# Architecture Decision Records

Records of architectural decisions taken during rampart's
development. Each ADR is numbered, dated, and follows the standard
format: `Status`, `Context`, `Decision`, `Consequences`,
`Alternatives considered`.

ADRs document the **why** behind a choice — the constraints and the
discarded alternatives — so a future contributor revisiting the
decision has the full picture, not just the resulting code.

## Index

| # | Title |
|---|---|
| [0005](./0005-no-cgo-rust-via-uds.md) | No cgo: Rust sidecar over a Unix Domain Socket |
| [0006](./0006-yarn-4-vs-pnpm-minimum-release-age.md) | Yarn 4 over pnpm: `minimum-release-age` for supply-chain hygiene |
| [0007](./0007-parser-packages-outside-internal.md) | npm parser package lives outside `internal/` |
| [0008](./0008-enablescripts-false-compatible-with-backstage.md) | `enableScripts: false` is compatible with Backstage |
| [0009](./0009-ci-cd-pipeline-architecture.md) | CI/CD pipeline architecture: 10 workflows, per-package coverage, govulncheck advisory, cosign keyless |
| [0010](./0010-golangci-lint-v2-migration.md) | golangci-lint v2 migration: `.golangci.yml` v1 → v2 format, action v6 → v9 |
| [0011](./0011-v020-scope-commitment.md) | v0.2.0 scope commitment: six themes ship together (auth, proxy wiring, multi-ecosystem parsers, Postgres, frontend depth, publisher anomaly) |
| [0012](./0012-auth-boundary-at-engine.md) | Auth boundary at engine, not at Backstage proxy (single enforcement layer; Backstage routes `allow: 'unauthenticated'`) |
| [0013](./0013-publisher-domain-split.md) | Publisher domain split: per-package time-series (`PublisherSnapshot`) vs per-maintainer aggregate (`PublisherProfile`) live in separate tables |

ADRs 0001–0004 are reserved for decisions that pre-date the formal
ADR practice; the Phase 1 design is captured in `ARCHITECTURE.md`
and the relevant ADRs from 0005 onward.

## Proposing a new ADR

1. Pick the next free number.
2. Copy an existing ADR as a starting template.
3. Fill in `Status: Proposed`, then walk through `Context`,
   `Decision`, `Consequences`, and `Alternatives considered`.
4. Open a PR. The PR description should link to the ADR file.
5. Once accepted (PR merged), update `Status` to `Accepted` with
   the merge date.

ADRs do not get edited after acceptance — they are a historical
record. A change in direction means a new ADR that supersedes the
prior one (the new ADR's `Status` references the superseded one).

## License

MIT — see [LICENSE](../../LICENSE).
