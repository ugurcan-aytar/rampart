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
4. **Unauthenticated API access** (added v0.2.0). The engine ships JWT
   middleware on `/v1/*` mutation routes. *Controls:* see
   "Authentication" + "CORS" below. Auth is opt-in via
   `RAMPART_AUTH_ENABLED=true` for backward-compat with v0.1.x; every
   production deployment is expected to flip it on. Failed auth emits
   `AuthRejected` events on `/v1/stream` for replay / brute-force
   detection.
5. **Postgres credential handling** (added v0.2.0). The engine's
   `RAMPART_DB_DSN` carries the Postgres password in the connection
   string. *Controls:* the DSN is read once at boot from env, never
   logged, never persisted into the storage layer. Operators are
   expected to source the env from a secret manager (Kubernetes
   Secret, Docker swarm secret, Vault), not from a plain
   `docker-compose.yml`.
6. **Outbound API rate limits** (added v0.2.0, Theme F1). The
   publisher metadata refresh path calls `registry.npmjs.org` and
   `api.github.com`. *Controls:* per-upstream token-bucket rate
   limiter, exponential backoff for 5xx, `Retry-After` honouring
   for 429s. Enforced inside `engine/publisher/internal/httpx`.
   `GITHUB_TOKEN` is optional; without it, the GitHub side caps at
   the unauthenticated 60 req/h.

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

### Authentication (v0.2.0+, Theme A1)

- **JWT middleware on `/v1/*`.** When `RAMPART_AUTH_ENABLED=true`,
  every mutation route (`POST` / `PUT` / `DELETE` / `PATCH`) requires
  a valid Bearer token. Read-only routes accept either authenticated
  or anonymous depending on per-deployment configuration. Algorithm
  is `HS256` (shared secret) or `RS256` (PEM-encoded public key);
  `RAMPART_AUTH_AUDIENCE`, when set, is asserted against the `aud`
  claim.
- **Token issuance.** `POST /v1/auth/token` mints internal/test
  tokens (HS256, 1-hour TTL by default). Production deployments
  typically front the engine with an external IdP that issues the
  JWTs; the engine just verifies. See
  [`docs/operations/auth-providers.md`](./docs/operations/auth-providers.md)
  for templates (GitHub OAuth, Azure AD, Okta, generic OIDC).
- **Scope enforcement.** Tokens carry a `scope` claim (`read` /
  `write` / `admin`). Mutation routes require `write`; admin-only
  routes (e.g. token issuance) require `admin`.
- **`AuthRejected` events.** Every rejected token is published on
  `/v1/stream` as an `AuthRejected` event with the reason
  (`expired`, `invalid_signature`, `audience_mismatch`,
  `scope_insufficient`). Operators tail this for replay / brute-
  force detection without reaching into engine logs.
- **Default off.** `RAMPART_AUTH_ENABLED` defaults `false` to
  preserve v0.1.x demo behaviour. Production deployments are
  expected to flip it on; the v0.2.0 migration guide documents
  the env-var contract.

### CORS (v0.2.0+, Theme A2)

- **Env allow-list.** `RAMPART_CORS_ORIGINS` is a comma-delimited
  list of origins permitted to call the engine from a browser. Empty
  (default) means deny-all cross-origin — the same-origin Backstage
  proxy path is unaffected.
- **`RAMPART_CORS_ALLOW_ALL=true`** echoes the request origin back
  as the `Access-Control-Allow-Origin` header (effectively wildcard).
  Exists for the v0.1.x demo path; the engine emits a warn log on
  every boot when this flag is on. Production deployments should
  use the explicit allow-list instead.
- **Backstage-fronted is the recommended posture.** When the
  Backstage backend plugin proxies the engine, no CORS config is
  needed — frontend requests are same-origin against Backstage. See
  [ADR-0012](./docs/decisions/0012-auth-boundary-at-engine.md) for
  why the engine is the auth boundary, not the proxy.

### Service-to-service auth (v0.2.0+, ADR-0012)

- **Backstage backend plugin token forwarding.** The
  `rampart-backend` Backstage plugin mints a service-to-service JWT
  at startup (HS256, signed with the shared `RAMPART_AUTH_SIGNING_KEY`)
  and attaches it as `Authorization: Bearer <token>` on every
  proxied request. The engine sees one identity (the Backstage
  service) regardless of which Backstage user originated the call —
  the user identity is captured separately in event metadata.
- **Why one boundary, not two.** Per ADR-0012: enforcing auth at
  both Backstage's `/api/rampart/*` proxy AND the engine's `/v1/*`
  middleware created a double-auth pattern that conflicted with
  Theme A3's planned guest-auth removal. The engine is the single
  authoritative auth boundary; Backstage proxy routes are
  configured `allow: 'unauthenticated'`.
- **Token rotation.** The Backstage plugin re-mints its token on
  every restart. There's no long-lived token persisted to disk;
  rotation is just a Backstage rolling restart. Operators with
  shorter rotation requirements should swap the Backstage plugin's
  token-mint helper for an external IdP integration (templates in
  `docs/operations/auth-providers.md`).

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

- **Backstage frontend auth is guest in the demo stack.** Backstage
  ships with `auth.providers.guest` enabled in the demo
  `app-config.yaml`. Theme A3 (replace guest with a real OAuth
  provider) is deferred from v0.2.0. Operators registering their
  own GitHub OAuth App should follow the v0.2.1 OAuth setup guide;
  templates for GitHub / Azure AD / Okta / generic OIDC live at
  [`docs/operations/auth-providers.md`](./docs/operations/auth-providers.md).
  Per ADR-0012 the engine itself enforces auth — the Backstage
  guest setting only affects who reaches the proxy, not who the
  engine accepts.
- **Postgres encryption-at-rest is the operator's responsibility.**
  The engine writes plain rows to whatever Postgres instance the
  DSN points at; operators who need at-rest encryption use cloud
  Postgres (RDS, Cloud SQL, Azure Database — all support
  transparent encryption) or a self-managed Postgres with disk-
  level encryption. The demo `docker-compose.yml` is not
  encrypted — it's a demo.
- **In-memory storage** (`RAMPART_STORAGE=memory`) has no
  persistence and no encryption — by design, for tests and demos.
  Production deployments use Postgres.
- **pinact SHA pinning on GitHub Actions** — v0.2.0 uses immutable
  semver tags (`actions/checkout@v4.2.2`); full commit SHA pins are
  a v0.3.0 sweep.
- **Theme A3 OAuth provider** is deferred. v0.2.0 ships the engine
  auth boundary (Theme A1) + CORS allow-list (Theme A2); Backstage
  guest auth removal lands in v0.2.1.
- **Outbound publisher API calls** (Theme F1) leak the deployment's
  IP to `registry.npmjs.org` and `api.github.com` if the publisher
  scheduler is enabled. Operators with strict egress policies
  should disable the scheduler (`RAMPART_PUBLISHER_ENABLED=false`,
  the default) or front the engine's outbound traffic with a known
  egress proxy.

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
