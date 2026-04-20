# rampart CLI

`rampart` is the CLI facade over the engine. Phase 1 ships one live
subcommand — `scan` — plus stubs for `ingest` / `status` / `serve` that
surface "not yet implemented" until the engine's publishing HTTP endpoints
come online.

## Build

```bash
go build ./cli/cmd/rampart
# or, in the workspace:
go install ./cli/cmd/rampart
```

## Subcommands

### `rampart scan [--format text|json|sarif] [--component-ref ref] [--commit-sha sha] <lockfile>`

Parses a `package-lock.json` (lockfileVersion 3) and prints the SBOM in
the requested format. Defaults to `text`.

Examples:

```bash
rampart scan ./package-lock.json
rampart scan --format json engine/testdata/lockfiles/axios-compromise.json
rampart scan --format sarif \
  --component-ref kind:Component/default/web-app \
  --commit-sha $(git rev-parse HEAD) \
  ./package-lock.json > rampart.sarif
```

Output formats:

- `text` — human-readable dump; one package per line.
- `json` — indented JSON; identical shape to the engine's SBOM wire type.
- `sarif` — SARIF 2.1.0; ready for `github/codeql-action/upload-sarif`.
  Results array is empty until IoC matching comes online; tool metadata
  and scan properties are populated.

## What's not implemented yet

- `rampart ingest <file>` — POST an SBOM or IoC to a running engine.
  Awaits `POST /v1/iocs` / `POST /v1/components/{ref}/sboms` (engine
  currently returns 501 stubs).
- `rampart status <incident-id>` — `GET /v1/incidents/{id}` likewise 501.
- `rampart serve` — for now, run the engine binary directly
  (`go run ./engine/cmd/engine`) or the container image.

## Architecture notes

- `cmd/rampart/main.go` is ≤25 lines — signal plumbing + `commands.Dispatch`.
- Every subcommand lives in `internal/commands/<name>.go`.
- Output formatters implement a single-method interface in
  `internal/output/`; the CLI's `output.SBOM` / `output.PackageVersion`
  types are deliberately decoupled from the engine's internal domain types,
  so the engine module stays free to evolve its internals.
