# ADR-0007: SBOM parser packages live outside `engine/internal/`

## Status

Accepted — 2026-04-20

## Context

FIRST.md prescribed `engine/internal/ingestion/sbom/npm` as the home for the
npm lockfile parser and had the CLI (a separate Go module at `cli/`) import
it directly. Go's `/internal/` rule blocks that:

> An import of a path containing the element "internal" is disallowed if
> the importing code is outside the tree rooted at the parent of the
> "internal" directory.

Since `cli/` is a separate go.mod rooted at `github.com/ugurcan-aytar/rampart/cli`,
it lives *outside* the tree rooted at `github.com/ugurcan-aytar/rampart/engine`.
Anything under `engine/internal/` is unreachable. The CLI's `scan`
subcommand therefore could not be wired the way FIRST.md sketched.

## Decision

SBOM parser packages live at `engine/sbom/*`, not `engine/internal/*`.
Specifically: `engine/sbom/npm/` holds the npm lockfile parser. Any future
parsers (`engine/sbom/pypi/`, `engine/sbom/cargo/`, `engine/sbom/go/`) will
follow the same pattern.

The parser's return type (`*domain.SBOM`) references a struct defined in
`engine/internal/domain/`. Go permits this because `engine/sbom/` is under
`engine/` and can reach `engine/internal/`; the CLI (outside that tree)
cannot *name* the `domain.SBOM` type but can read its fields through the
value returned by `Parser.Parse`. `cli/internal/commands/scan.go` copies
those fields into `cli/internal/output.SBOM` — CLI's own public shape —
and hands the copy to the formatter. The engine's domain package stays
internal.

## Consequences

- **Parser signature is now a public API boundary.** Any change to
  `Parser.Parse` or the returned struct's shape is a breaking change for
  external consumers (CLI today, Rust/UDS client in Adım 6, third-party
  integrations later).
- **`domain.SBOM` leaks through the return type.** Consumers can read its
  fields via value access but cannot call domain methods on it (they'd need
  the import, which the internal rule blocks). This is a small ergonomic
  wart — if it bites, the fix is to replace the return type with a
  dedicated public struct in `engine/sbom/` and keep `domain.SBOM` purely
  internal. Deferred until a consumer actually needs domain methods.
- **`engine/pkg/sbom/*` was the classic Go alternative** (the `pkg/`
  convention) but adds no substance over `engine/sbom/*`; shorter path
  wins.

## Alternatives considered

1. **CLI talks only to the engine's HTTP API.** Rejected: kills
   `rampart scan` as an offline tool; every developer running a pre-push
   scan would need the engine running. Breaks the CI story (GitHub
   Action installs the CLI in a bare runner).
2. **Copy the parser code into `cli/`.** Rejected: duplication, parity
   drift, two places to fix any bug.
3. **Merge `cli/` into the engine's go.mod.** Rejected: FIRST.md and the
   architecture narrative lean on independently-versioned modules so each
   artefact (engine library, CLI, integrations) ships on its own
   schedule.
4. **Move `domain/` out of `internal/` alongside the parser.** Rejected
   for Phase 1: domain types (Component, Incident, Remediation, …) are
   still evolving and kept private on purpose. Revisit when Phase 3
   makes storage backends concrete — at that point, a stable public
   `engine/types/` subpackage may be worth the commitment.
