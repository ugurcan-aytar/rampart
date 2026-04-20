# Security

rampart is a supply chain security tool. Its own supply chain has to be model-quality.

Full threat model, STRIDE analysis, and hall of fame land in Adım 9 alongside [docs/security/threat-model.md](./docs/security/threat-model.md). Controls already in force from Adım 1:

- **npm side:** `.yarnrc.yml` disables postinstall scripts (`enableScripts: false`) and enforces exact versions (`defaultSemverRangePrefix: ""`).
- **npm side (fallback):** `.npmrc` mirrors the same defenses for contributors who reach for `npm` directly.
- **Bootstrap gate:** `make bootstrap` fails if either of the above settings drifts.
- **72h release-age quarantine (Shai-Hulud defense):** Yarn 4.6 has no native equivalent of pnpm's `minimum-release-age`. We enforce this at PR-review time via Renovate's `minimumReleaseAge`, wired up in Adım 8. Open gap until then — interim mitigation is that Dependabot + manual review is the only ingress for new dependencies.
- **Go side:** `go.sum` integrity — strict pinning lands with `engine/go.mod` in Adım 2, `govulncheck` in CI in Adım 8.
- **GitHub Actions:** all action refs will be SHA-pinned (Adım 8 via `pinact`).
- **Releases:** OIDC-based publishing (npm trusted publishing, crates.io trusted publishing) — Phase 2.

## Responsible disclosure

Email: `aytarugurcan@gmail.com`. Subject prefix `[rampart-security]`. Please allow 72 hours for initial response.
