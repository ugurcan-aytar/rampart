# ADR-0009: CI/CD pipeline architecture — 10 workflows, per-package coverage gates, govulncheck advisory, cosign keyless

## Status

Accepted — 2026-04-21. Implementation landed across Adım 8 and 9
(commits `c1f2244 → c3faa91`).

## Context

By Adım 8 rampart had enough surface (Go engine, Rust sidecar, three
Backstage plugins, four integrations, five Docker images, three
codegen artefacts) that hand-verified green was infeasible and
PR-level regressions went unnoticed between `Adım N` and `Adım N+1`.

The question wasn't "do we need CI" — it was the shape. Four
decisions drive this ADR:

1. **What's gating vs what's advisory?** Blocking every PR on every
   lint nit drives noise; letting everything slide makes CI
   decorative. Where's the line?
2. **Coverage thresholds — global or per-package?** A single global
   floor (e.g. 80 %) gets diluted by generated code (api/gen/*) and
   test helpers (storagetest) that show as 0 % coverage. A per-
   package matrix costs more config but enforces the right thing.
3. **Release signing — long-lived keys or keyless?** A cosign
   private key secret in the repo is the obvious path; a keyless
   OIDC flow is the supply-chain-honest path. Both work.
4. **Schema drift — runtime contract or human discipline?** The
   OpenAPI schema is the contract across four consumer languages.
   "Remember to run `make gen`" is a request the CI shouldn't trust.

## Decision

Ten workflows under `.github/workflows/`, organised by language /
concern, with the gating-vs-advisory split explicit in comments.

### Workflow inventory

| File | Scope | Trigger | Gating? |
|---|---|---|---|
| `engine.yml` | Go engine tests + lint + coverage + govulncheck | PR + main push, paths filter | **yes** (lint, test, coverage), **advisory** (govulncheck) |
| `native.yml` | Rust test + clippy + fmt + cargo-audit + cargo-deny | PR + main push, paths filter | **yes** |
| `parity.yml` | Go parser ↔ Rust parser byte-identical output | PR + main push when either side changes | **yes** |
| `gen-check.yml` | OpenAPI schema drift (`make gen-check`) | PR + main push when schema changes | **yes** |
| `backstage.yml` | Yarn workspaces typecheck + lint + test + production image build | PR + main push, paths filter | **yes** |
| `integrations.yml` | Four Go modules + github-action yamllint + gitlab-component yamllint | PR + main push, paths filter | **yes** |
| `e2e.yml` | Playwright against live compose stack | PR + main push | **yes** |
| `codeql.yml` | Go + JS/TS + Actions SAST | PR + weekly schedule | **yes** |
| `dependency-review.yml` | License + advisory diff on deps | PR only | **yes** |
| `release.yml` | Tag-triggered goreleaser + cargo cross + images + npm | Tag push (`v*`) | n/a (release artefact) |

### Per-package coverage gates

Single-threshold coverage was measurably misleading. The engine's
total coverage sits at ~48 % because:

- `engine/api/gen/*` is ~1000 lines of generated OpenAPI code with
  no tests by design (the contract is enforced at the handler layer).
- `engine/internal/storage/storagetest/*` ships shared contract
  helpers called from downstream backends' `*_test.go`; Go reports
  it as 0 % because the package itself has no `_test.go`.

Instead, `engine.yml` enforces per-package floors on the load-bearing
code, plus a low total floor as a tripwire:

- `domain`     ≥ 80 %
- `api`        ≥ 70 %
- `trust`      ≥ 85 %
- `matcher`    ≥ 80 %
- total        ≥ 45 %

A regression in any of these fails the build. Dilution from generated
packages stays allowed; code that operators depend on stays gated.

### govulncheck — advisory

`govulncheck ./...` runs on every engine PR but with
`continue-on-error: true`. Rationale: Go standard library CVEs drop
monthly (a new encoding/pem quadratic parse advisory, a new
crypto/tls ALPN leak, etc.); most don't touch rampart's hot path
(we don't parse hostile PEM, don't initiate TLS against attacker-
controlled hosts, etc.).

Blocking on each new stdlib advisory forces a Go minor-version bump
per PR cycle, which drowns the signal. The advisory mode keeps the
findings visible in the Actions summary — security review reads them
and decides if a minor bump is warranted.

**Install-step resilience.** The govulncheck run step carries
`continue-on-error: true`. The install step
(`go install golang.org/x/vuln/cmd/govulncheck@<pinned>`) does too,
and pins to a specific version rather than `@latest`. Rationale:
upstream tool releases can silently bump Go-version floors — e.g.
`golang.org/x/vuln` v1.2.0 required Go ≥ 1.25, which hard-failed
under CI's pinned Go 1.24 + `GOTOOLCHAIN=local`. Pinning + making
the install step advisory preserves govulncheck's advisory nature
even when upstream drops break compatibility with our toolchain.
Currently pinned to `v1.1.4` (last Go 1.22+ compatible release,
covering engine's 1.24 toolchain). Re-evaluate when the engine Go
toolchain bumps to 1.25+.

### Schema drift — `make gen-check`

Contract edits cascade into four generated artefacts: Go server stub,
Go client types, TS client types, and the internal domain mappers.
Hand-running `make gen` before every schema PR is an ask CI shouldn't
trust.

`gen-check.yml` runs `make gen` in CI then asserts `git diff
--exit-code`. Any edit to `schemas/openapi.yaml` without the matching
regeneration fails the PR with the diff summary inlined.

Proven with a deliberate drift PR at Phase 1 close (see Adım 8
validation). A trivial `tags[0].description` edit without `make gen`
failed CI in 3 seconds with: *"FAIL: generated artefacts drift from
schemas/openapi.yaml. Run `make gen` and commit the result."*

### Release signing — cosign keyless

No long-lived signing keys in the repo. Every artefact
(`release.yml`) is signed via GitHub OIDC — the workflow runtime
acquires a short-lived Sigstore certificate, signs, and publishes
both to the Rekor transparency log.

- `permissions.id-token: write` is the OIDC switch.
- `cosign sign-blob --yes` on every Go binary tarball.
- `cosign sign` on every container image with
  `ghcr.io/ugurcan-aytar/<name>@<digest>`.
- `cosign attest --type spdx` attaches the syft-generated SBOM as
  a release attestation.

Verification recipe in [SECURITY.md](../../SECURITY.md).

The same OIDC flow covers npm trusted publishing for the three
Backstage plugins: no NPM_TOKEN secret, `--provenance` attaches the
workflow's provenance to the registry entry.

## Consequences

### Positive

- **Every Phase 1 regression surfaces at PR time.** No more
  "noticed in Adım N+2 that Adım N broke the parity test". Chain
  test + gen-check + per-package coverage gates form a wall.
- **No long-lived secrets.** `GITHUB_TOKEN` is the only secret
  the repo consumes; everything else runs through OIDC (cosign,
  npm trusted publishing). A Shai-Hulud-class compromise of the
  repo's build environment can't pivot to signing a bad release
  because there's no signing key to steal.
- **The advisory-vs-gating split is explicit in every workflow.**
  Reviewers know by reading the yaml what will block a merge.
- **Release artefacts are self-auditable.** rampart can scan its
  own releases — the SBOM attestation is queryable by any tool
  that speaks cosign attest.
- **Per-package coverage gates focus review effort.** When
  `domain` coverage drops from 99 % to 78 %, a reviewer knows
  something material changed. A global threshold dilutes that
  signal.

### Negative — must document

- **Matrix of 10 workflows is a lot of yaml.** A fresh contributor
  opening a PR across three subsystems can trip three different
  workflows. Mitigation: paths filters on every workflow so a
  PR touching only `engine/` doesn't pay the native+backstage
  cost.
- **govulncheck advisory means CVEs can sit unfixed.** The
  workflow summary surfaces them but doesn't paginate them. An
  operator deploying rampart at a paranoia level higher than
  Phase 1 needs their own `govulncheck` gate in their
  deployment pipeline.
- **Coverage tuning is per-package.** New packages need a new
  threshold entry in `engine.yml`. Someone who adds `engine/sbom/pypi`
  without a coverage gate will dilute the total floor quietly.
  Mitigation: CODEOWNERS requires review on `.github/workflows/`
  edits, so adding the gate alongside the package is an
  automatic reminder.
- **SHA pinning on action refs is deferred.** Phase 1 workflows
  use immutable semver tags (`actions/checkout@v4.2.2`). Full
  commit SHA pinning via `pinact` is a Phase 2 mechanical sweep.
  A malicious tag rewrite by an action maintainer between Phase 1
  and Phase 2 would land in a rampart build. Partial mitigation:
  semver-pinned versions require the attacker to overwrite a
  specific tag AFTER dependabot runs, which is a visible pattern.

### Neutral

- **paths filters bias first-time contributors' experience.** A
  PR that only edits `docs/` never sees `engine.yml` run. This
  is intentional (don't burn CI minutes on no-op runs) but can
  mislead someone's "my PR is green" into "therefore the code
  change I also made is green".

## Alternatives considered

1. **Single monolithic CI workflow.** Simpler yaml, simpler UX.
   Rejected: on a PR touching only `backstage/`, paying the Rust
   cross-compile cost is ~8 minutes of runner time wasted.
2. **golangci-lint v2 config format.** v2 ships new defaults + a
   breaking yaml shape. Staying on v1 (`v1.64.6`) because the
   default rule set fits rampart's code style and a repo-wide
   v2 migration is a separate ADR — now accepted, see
   [ADR-0010](./0010-golangci-lint-v2-migration.md).
3. **Signed key in repo via sops or sealed-secrets.** Simpler
   than OIDC for the cosign case — drop a key, encrypt it. Rejected:
   any in-repo key leaks the moment the repo is compromised, which
   is the exact threat model we're defending against.
4. **SBOM attestation at build time, attached to the binary.**
   Conceptually cleaner than a sidecar attestation. Rejected for
   Phase 1 because the binary's build reproducibility isn't
   verified yet — attaching a non-reproducible SBOM invites
   "the attestation lied" review loops. Re-visit in Phase 2
   alongside rebuildable release artefacts.

## Verification

The workflow inventory at Phase 1 close: all 8 primary workflows
pass on commit `c3faa91` without `continue-on-error` flags
(Playwright e2e came off its advisory flag after the Dockerfile
slimming reduced cold-boot flake).

A deliberate drift PR (#26, closed no-merge) proved `gen-check.yml`
catches a schema edit without `make gen` in < 3 s.

Coverage gates fire in the expected direction: deleting one
`TestParse_AxiosCompromise` case drops `domain` coverage from 99 %
to 94 % — still above the 80 % floor. Deleting the package's entire
`_test.go` drops it to 0 % and fails the build with
`FAIL: engine/internal/domain below 80%`.
