# ADR-0006: Yarn 4.6 vs pnpm for minimum-release-age quarantine

## Status

Accepted — 2026-04-20

## Context

Two recent incidents made the *publish-to-detection* window the highest-leverage defense in the npm supply chain:

- **Shai-Hulud worm (September 2025)** — self-propagating npm worm; the window between publish and community detection was the window in which victims were infected.
- **axios compromise (31 March 2026)** — attacker-controlled `axios@1.11.0` live for roughly three hours. Researchers flagged it within six minutes, but every `npm install` inside the window pulled a RAT.

The standard defense is *minimum release age*: refuse to install any version younger than N hours (typical value 72 h, which covers the "detected but not yet advisory-published" zone).

**pnpm** implements this natively as `minimum-release-age`. **Yarn 4.6** (the current stable as of 2026-04-20) does not. Yarn 4.9 adds `npmMinimalAgeGate` in beta but 4.9 is not yet released.

`rampart`'s identity as a supply-chain security tool requires this defense to be present — and to be *visibly* present, because the project is demonstrative.

## Decision

Stay on Yarn 4.6 for Phase 1. Enforce the 72 h quarantine at PR-review time via Renovate's `minimumReleaseAge` setting, wired up in Adım 8. Migrate to Yarn 4.9+ `npmMinimalAgeGate` once 4.9 reaches stable.

## Consequences

- **Open gap in local install:** Between an IoC's publish timestamp and Renovate's PR gate firing, a local `yarn install` can still pull a freshly-published version. In practice the window is narrow because (a) this repo is solo-maintained; (b) Dependabot + Renovate are the only automated ingress; (c) every new dependency requires an entry in `DEPS.md` before the PR can merge.
- **Shifted enforcement point:** The defense lives in CI/PR instead of in the package manager. This is honest but adds a moving part.
- **Easy migration:** When Yarn 4.9 ships, flipping one key in `.yarnrc.yml` replaces the Renovate-side check. No code elsewhere has to change.
- **Explicitly documented:** Tracked in `SECURITY.md` under *Known gaps*.

## Alternatives considered

1. **Switch to pnpm.** Rejected. The Backstage ecosystem is Yarn-centric — the `create-app` template ships a `yarn.lock`, all official Backstage tooling and recipes assume Yarn workspaces, and two of the three published Backstage plugins in this repo depend on Yarn workspace protocol (`workspace:*`). pnpm compatibility is achievable (hoisting config, Nx pnpm driver, `shamefully-hoist`), but the cost is real engineering work for a single feature. Not worth it.
2. **Wait for Yarn 4.9 stable.** Rejected. 4.9 is in beta; blocking Phase 1 on an unreleased version is not an option.
3. **Adopt a community Yarn plugin (e.g. `@vidavidorra/yarn-plugin-minimum-release-age`).** Rejected. Installing a maintainer-of-one Yarn plugin *into the tool chain that is supposed to defend against maintainer-of-one compromises* inverts the threat model. The cure is a larger attack surface than the disease.
4. **Enforce only via a git pre-commit hook.** Rejected. Pre-commit hooks are bypassable locally (`--no-verify`), and they fire after the dependency has already been installed. Renovate is the right enforcement point: it gates the PR before merge, and it cannot be silently bypassed.
