# ADR-0008: enableScripts=false compatible with the Backstage toolchain

## Status

Accepted — 2026-04-20

## Context

rampart's supply-chain posture requires `enableScripts: false` in
`.yarnrc.yml` (ADR-0003 dependency-locking discipline, and the broader
Shai-Hulud / `axios@1.11.0` class of postinstall-based attacks). Disabling
lifecycle scripts globally takes the most common npm delivery vector off
the table, and the yarn 4 docs are explicit that the setting can be
toggled per-install (ref: https://yarnpkg.com/configuration/yarnrc#enableScripts).

The standing assumption on the team was that installing Backstage with
`enableScripts: false` would be *incompatible* — packages like
`@swc/core`, `esbuild`, and `better-sqlite3` are commonly cited as needing
postinstall scripts to ship their native binaries, and the frequent
workaround across OSS projects is an `enableScripts: [<allowlist>]`
override. I explicitly wanted to avoid any allowlist: every entry is an
attack-surface addition.

My own local verification on 2026-04-20 (macOS arm64, Node 24.7, Yarn
4.6.0, Backstage 1.30+ era packages) contradicted the assumption.

## Decision

Install the full Backstage toolchain with `enableScripts: false` and no
per-package overrides.

`yarn install` completed (~1,100 packages, ~1.3 GiB node_modules) with
the expected `YN0004` warnings that certain packages had disabled build
scripts. The packages that warned during Adım 5.1:

- `@swc/core@npm:1.15.30`
- `esbuild@npm:0.24.2`, `esbuild@npm:0.27.7`
- `better-sqlite3@npm:11.10.0`
- `cpu-features@npm:0.0.10`
- `core-js-pure@npm:3.49.0`
- `protobufjs@npm:7.5.5`
- `ssh2@npm:1.17.0`

The critical toolchain consumers — `@backstage/cli`'s `package start`,
`package test`, and `package lint` — use esbuild / @swc for code
transformation at runtime. They proved runnable without the skipped
postinstall because the platform-specific native binaries ship as
separate *optionalDependencies* that yarn installs directly from tarballs
(no postinstall needed on the recipient side). The `@esbuild/darwin-arm64`
and `@swc/core-darwin-arm64` packages are examples of this shape; their
binaries landed at `node_modules/@esbuild/darwin-arm64/bin/esbuild` and
the bundler picked them up at invocation time.

Source citation — from `node_modules/@esbuild/darwin-arm64/package.json`:
```
"os": ["darwin"], "cpu": ["arm64"]
```
esbuild's top-level package `optionalDependencies` block lists the
per-platform packages directly, matching the pattern described in
https://esbuild.github.io/getting-started/#native-binary (the binary is
distributed in the platform-specific package, not downloaded post-install).

## Consequences

**Positive**

- The supply-chain gate is intact against a 1,100+ package transitive
  surface. No whitelist to audit.
- Interview argument: "rampart preserves Shai-Hulud defenses without
  compromising on toolchain choice — the enterprise-default posture
  doesn't force enableScripts concessions."
- Faster installs in CI (no postinstall phase).
- Reproducible builds: identical cache + lockfile yield identical
  `node_modules` content regardless of local Node/OS quirks that scripts
  would otherwise introduce.

**Negative**

- Coverage is bound to upstream ecosystems that continue shipping
  prebuilt binaries. If Backstage or a transitive dep later adopts a
  compilation-required package, this ADR reopens.
- Platform coverage matches whatever platforms the upstream packages
  prebuild. macOS `arm64` + `x64` and Linux `x64` + `arm64` are confirmed
  in the current lockfile's optional-dependency tree. Windows native is
  not confirmed — Phase 2 concern.
- Does not protect against attacks that don't rely on postinstall (e.g.
  runtime payloads shipped in the main package tarball). Those are the
  Renovate `minimumReleaseAge` + review surface (ADR-0006).

## Verification

Reproducible at commit `444b3ec`:

```
$ yarn config get enableScripts
false

$ yarn install
…
YN0004: @swc/core@npm:1.15.30 lists build scripts, but all build scripts have been disabled.
YN0004: esbuild@npm:0.24.2 lists build scripts, but all build scripts have been disabled.
…
Done in 15s

$ yarn workspace @ugurcan-aytar/backstage-plugin-rampart run typecheck && \
  yarn workspace @ugurcan-aytar/backstage-plugin-rampart run lint && \
  yarn workspace @ugurcan-aytar/backstage-plugin-rampart run test
…
exit 0 × 3

$ yarn workspace @ugurcan-aytar/backstage-plugin-rampart run dev     # background
$ node backstage/plugins/rampart/dev/verify.mjs
…
verdict: IncidentDashboard rendered end-to-end ✓
```

## Alternatives considered

1. **Per-package scripts allowlist via yarn `dependenciesMeta`.** Rejected
   — each entry adds an attack-surface opt-in. The whole point of the
   global gate is to make per-package audits explicit; the right place
   for that audit is the Renovate / PR review layer, not the toolchain
   config.
2. **Switch to pnpm with per-package script control.** Rejected on the
   same grounds as ADR-0006: Backstage ecosystem is yarn-centric and
   migrating for a feature already provided by the optional-dep pattern
   would be pure overhead.
3. **Accept yarn dev won't work, use production-only images.** Rejected —
   a working local dev harness is load-bearing for any contributor and
   the assumption that it was incompatible turned out to be false.
