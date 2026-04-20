# Security

rampart is a supply chain security tool. Its own supply chain has to be model-quality.

Full threat model, STRIDE analysis, and hall of fame land in Adım 9 alongside [docs/security/threat-model.md](./docs/security/threat-model.md).

## Controls in force from Adım 1

- **Postinstall scripts disabled.** `.yarnrc.yml` sets `enableScripts: false`; `.npmrc` mirrors with `ignore-scripts=true`.
- **Exact versions only.** `.yarnrc.yml` sets `defaultSemverRangePrefix: ""`; `.npmrc` mirrors with `save-exact=true`, `save-prefix=`.
- **Bootstrap gate.** `make bootstrap` refuses to proceed if either of the above settings has drifted from the committed config.
- **No AI artifacts in repo.** `.gitignore` lists AI tooling directories (`CLAUDE.md`, `.claude/`, `.aider*`, `.cursor/`, `.continue/`) at the top; `CODEOWNERS` requires review from the project owner on every change.

## Controls that land in later steps

- **Go integrity:** `go.sum` pinning (Adım 2), `govulncheck` in CI (Adım 8), `trivy fs` (Adım 8).
- **GitHub Actions:** all action refs SHA-pinned via `pinact`; CodeQL, `dependency-review-action`, `zizmor` (Adım 8).
- **Container:** distroless base, non-root user, cosign signatures, SBOM attestation (Adım 7–8).
- **Releases:** OIDC-based publishing (npm trusted publishing, crates.io trusted publishing) — Phase 2.

## Known gaps

- **72h minimum-release-age quarantine.** Yarn 4.6 lacks a native minimum-release-age config. Enforced at PR-review time via Renovate's `minimumReleaseAge` setting (Phase 2, tracked in ROADMAP.md). See [ADR-0006](./docs/decisions/0006-yarn-4-vs-pnpm-minimum-release-age.md) for why this project did not switch to pnpm and for the migration path once Yarn 4.9 `npmMinimalAgeGate` leaves beta.

## Responsible disclosure

Email: `aytarugurcan@gmail.com`. Subject prefix `[rampart-security]`. Please allow 72 hours for initial response.
