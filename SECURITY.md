# Security

rampart is a supply chain security tool. Its own supply chain has to
be model-quality. This document covers the controls, the known gaps,
and the responsible-disclosure path.

## Responsible disclosure

**Email:** `aytarugurcan@gmail.com`. Subject prefix
`[rampart-security]`.

Please allow **72 hours** for initial acknowledgement. If the bug is
actively being exploited, say so in the subject — that opens a faster
lane. Do not open a public GitHub issue until we've confirmed a fix
path together.

## Threat model

rampart sits between *feeds that publish indicators of compromise*
and *teams that own the code under attack*. Three classes of threat
drive the control set:

1. **Hostile lockfile parsing.** A crafted `package-lock.json` arrives
   as an SBOM submission or a CLI scan target. The parser is Go; a
   crash or OOM would take down the ingestion path for everyone.
   *Control:* the optional Rust sidecar (ADR-0005) runs the parser in
   a separate process, separate language runtime, no shared memory
   with the engine. A hostile lockfile crashes only the sidecar; the
   engine logs a fallback warn and keeps serving. Opt-in per deploy.
2. **Malicious transitive in rampart's own build.** The Shai-Hulud /
   `axios@1.11.0` class attack: a dependency ships a postinstall
   script that runs during `yarn install`. *Controls:* see below.
3. **Stolen release artifacts.** Someone pushes a compromised
   `rampart-engine:v0.1.2` image to ghcr.io. *Controls:* cosign
   keyless signing on every artifact + SBOM attestation. Verification
   recipe below.

## Controls in force

### Build-time

- **Postinstall scripts disabled.** `.yarnrc.yml` sets
  `enableScripts: false`; `.npmrc` mirrors with `ignore-scripts=true`.
  No allowlist — the whole class is off. Verified against the full
  1,100-package Backstage toolchain in [ADR-0008](./docs/decisions/0008-enablescripts-false-compatible-with-backstage.md).
- **Exact versions only.** `.yarnrc.yml` sets
  `defaultSemverRangePrefix: ""`; `.npmrc` mirrors with
  `save-exact=true`. No caret, no tilde — a lockfile resolution is
  a git diff worth reviewing.
- **Minimum release age (72 h).** New package versions wait 72 h
  before Renovate can pin them ([ADR-0006](./docs/decisions/0006-yarn-4-vs-pnpm-minimum-release-age.md)).
  Migration path to Yarn's native `npmMinimalAgeGate` is tracked.
- **Bootstrap gate.** `make bootstrap` refuses to proceed if either
  of the above settings has drifted from the committed config.
- **Every dep justified.** [DEPS.md](./DEPS.md) lists every runtime
  dependency with **source**, **why**, **alternatives considered**,
  **risk assessment**, and **upgrade policy**.

### CI

- **CodeQL SAST** on Go + JS/TS + GitHub Actions, weekly schedule +
  per-PR.
- **govulncheck** (Go stdlib + deps) — advisory mode, CVE signal
  without blocking on stdlib-patch-version churn (ADR-0009).
- **cargo-audit + cargo-deny** on the Rust sidecar, with a license
  allowlist scoped to MIT / Apache-2 / BSD / ISC / Unicode / MPL.
- **Dependency review** on every PR — AGPL / GPL / SSPL deps are
  refused outright.
- **Schema drift guard.** `gen-check.yml` re-runs `make gen` and
  fails on any diff — a contract edit can't be merged with the
  generated client out of sync.
- **Supply-chain gate guards.** The `enableScripts=false` +
  `defaultSemverRangePrefix=""` settings are re-checked by
  `backstage.yml` on every PR. Drift is a CI failure, not a code
  review.

### Release

- **Cosign keyless signing on every artifact.** Release workflow
  uses GitHub OIDC — no long-lived signing key anywhere in the repo.
  Every Go binary, every Rust cross-compile target, every container
  image gets a signature + certificate deposited in the Sigstore
  transparency log.
- **SBOM attestation per artifact.** `syft` produces an SPDX SBOM
  per archive; `cosign attest --type spdx` attaches it. Verifiable
  with `cosign verify-attestation`.
- **npm trusted publishing.** The three Backstage plugins publish
  via OIDC (no long-lived NPM_TOKEN); `--provenance` attaches the
  build's provenance to the registry entry.

### Verifying a release

After v0.1.0 ships, any release artifact can be verified like so
(example for a binary tarball):

```bash
cosign verify-blob \
  --certificate   rampart_v0.1.0_linux_amd64.tar.gz.pem \
  --signature     rampart_v0.1.0_linux_amd64.tar.gz.sig \
  --certificate-identity-regexp '.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  rampart_v0.1.0_linux_amd64.tar.gz
```

For a container image:

```bash
cosign verify \
  --certificate-identity-regexp '.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/ugurcan-aytar/rampart-engine:v0.1.0

cosign verify-attestation \
  --type spdx \
  --certificate-identity-regexp '.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/ugurcan-aytar/rampart-engine:v0.1.0
```

## Known gaps

- **Auth is guest in the demo stack.** Backstage ships with
  `auth.providers.guest` enabled; swap in a real provider (OIDC,
  GitHub, etc.) before exposing the stack outside a trust boundary.
  See [README Production deployment notes](./README.md#production-deployment-notes).
- **Engine CORS is permissive.** The demo middleware allows any
  origin so the Backstage frontend (on `:3000`) can hit the engine
  (on `:8080`) directly. Production deploys route the frontend
  through the `rampart-backend` proxy; CORS tightens then. Phase 2
  ships this wiring.
- **In-memory storage has no encryption-at-rest.** It also has no
  persistence — the point is moot until Postgres ships (Phase 2).
- **No first-party auth on `/v1/*` endpoints.** Engine assumes it
  runs behind a trusted reverse proxy. Production deployments should
  put an auth-enforcing proxy (Envoy + OIDC, nginx + auth subrequest,
  etc.) in front of the engine container.
- **pinact SHA pinning on GitHub Actions** — Phase 1 uses immutable
  semver tags (`actions/checkout@v4.2.2`); full commit SHA pins are
  a Phase 2 sweep.

## What rampart will *not* do

- **No cloud callbacks.** The engine does not phone home, does not
  send telemetry, does not call out to any rampart-hosted service.
  Every network request is one you asked for (IoC feed subscription,
  Slack webhook you configured, Backstage backend proxy on your
  network).
- **No stored secrets.** Slack webhooks, OIDC tokens, API keys — all
  come from environment variables or the runtime config. Nothing is
  persisted into the storage layer.
- **No LLM calls in the core engine.** Phase 3 speculates about an
  opt-in LLM triage feature; if it ships, it will be a separate
  binary behind an explicit on-switch, with no data flowing unless
  you configure it.

## Previous vulnerabilities

None reported. First release is still pending (v0.1.0).
