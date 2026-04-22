# ADR-0010: golangci-lint v2 migration

## Status

Accepted — 2026-04-22.

## Context

[ADR-0009](./0009-ci-cd-pipeline-architecture.md) explicitly deferred
the `golangci/golangci-lint-action` v6 → v9 upgrade (which requires
golangci-lint v2) to a separate ADR, because v2 ships a breaking
config-schema change. Dependabot PR #10 was left open against `main`
through the v0.1.0 release cycle so the config migration could be
reviewed as a standalone change.

Upstream references:
- v2 migration guide: <https://golangci-lint.run/product/migration-guide/>
- `golangci-lint migrate` subcommand (ships in v2) did ~95 % of the
  mechanical translation; the remaining edits are the two load-bearing
  exclusion comments the migrator dropped, plus an explicit `run.timeout`
  re-add (v2 defaults to no timeout).

## Decision

Migrate in one PR:

1. `engine/.golangci.yml` v1 → v2 format.
2. `.github/workflows/engine.yml` action ref v6 → v9.2.0 (SHA-pinned).
3. One source-code fix for the single new lint finding (see below).

### Config schema delta

| v1 field | v2 equivalent | Notes |
|---|---|---|
| `linters.disable-all: true` | `linters.default: none` | preset picker replaces the bool |
| `linters.enable: [gofmt, goimports, …]` | `formatters.enable: [gofmt, goimports]` + `linters.enable: […]` | formatters split out into their own block |
| `issues.exclude-rules` | `linters.exclusions.rules` | nested under `linters` |
| `issues.exclude-dirs` | `linters.exclusions.paths` | same semantics, new key |
| `run.deadline` | `run.timeout` | same semantics; also needs explicit set because v2 default is no timeout |
| (none) | `linters.exclusions.generated: lax` | new knob, defaults added by migrator |
| (none) | `linters.exclusions.presets` | `comments`, `common-false-positives`, `legacy`, `std-error-handling` added by migrator |

### Linter set

Kept unchanged:
`errcheck`, `gosec`, `govet`, `ineffassign`, `misspell`, `revive`,
`staticcheck`, `unused`. No deprecations hit us — the set was already
conservative.

`gofmt` and `goimports` stayed enabled but moved from
`linters.enable` to `formatters.enable`, reflecting v2's split.

### Exclusion rules

Both load-bearing exclusions carried over verbatim, their prose
comments preserved manually (the migrator strips comments):

- `_test\.go` × `gosec` — test fixtures construct malformed
  payloads + use unseeded randomness by design.
- `internal/domain/events\.go` × `revive` (`stutters`) — the
  `DomainEvent` type name is load-bearing across `events.Bus.Publish`
  consumers; renaming to `Event` collides with package-level
  `Event` symbols in every caller.

### Single new finding — fixed in place, not excluded

staticcheck's `QF1001` (De Morgan's law suggestion) fired once:
`engine/internal/api/handlers_sbom_test.go:117`. The `for !(a && b && c)`
spin-loop condition rewrote cleanly as `for !a || !b || !c`. No
semantic change, no exclusion added.

## Consequences

### Positive

- Lint path now on v2, aligned with dependabot's upgrade cadence.
- Future contributors running `golangci-lint run` locally must install
  v2 — the config is v2-formatted and v1 rejects `version: "2"` outright,
  so the error message is immediate and actionable.
- `formatters` split makes the distinction between "catches bugs" and
  "reshapes bytes" explicit — handy when adding new formatters like
  `gci` in the future.

### Neutral

- Migrator-added `exclusions.presets` are opt-in noise reducers. Kept
  all four defaults (`comments`, `common-false-positives`, `legacy`,
  `std-error-handling`). Can prune individually if a legitimate finding
  is ever hidden by one.
- `exclusions.paths` defaults (`third_party$`, `builtin$`, `examples$`)
  never matched anything in this repo pre-migration; harmless to keep.

### Negative

- Lockstep between local dev environments and CI: a contributor on
  v1 gets "unknown field: version" and must upgrade. Mitigation: the
  root `Makefile`'s `lint` target could pin the version, but that's a
  Phase 2 item — for now the error is clear.

## Alternatives considered

1. **Stay on v1 indefinitely.** Rejected: v1 stopped receiving new
   linter rule updates at 1.64; supply-chain-security rules (gosec,
   govulncheck integration) now ship v2-only.
2. **Enable all v2 presets aggressively** (e.g. add `godox`,
   `nolintlint`, `errorlint`). Rejected for this ADR: scope-creep.
   Each new linter deserves its own PR with a known-violations audit.
3. **Suppress `QF1001` globally** instead of fixing the one call
   site. Rejected: the fix is a one-line clarity improvement; a
   blanket suppression would also hide future violations.

## Verification

- `golangci-lint config verify --config .golangci.yml` → clean.
- `golangci-lint run ./...` → `0 issues.`
- `go test ./internal/api/...` (the package with the rewritten loop)
  → green.
- CI `lint` job on the feature-branch PR expected green; final
  verification lands on PR review before merge.
