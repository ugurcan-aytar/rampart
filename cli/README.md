# rampart CLI

`rampart` is the command-line front end for the rampart supply-chain
incident response engine. The CLI binary parses lockfiles, emits
SBOMs, and produces SARIF reports without requiring a running engine.

## Installation

Pre-built binaries are published with every release at
`https://github.com/ugurcan-aytar/rampart/releases/latest` (Linux,
macOS, Windows × amd64/arm64). Pick the archive matching your
platform, extract `rampart`, and place it on `$PATH`.

From source:

```bash
go install github.com/ugurcan-aytar/rampart/cli/cmd/rampart@v0.1.0
```

From the workspace:

```bash
git clone https://github.com/ugurcan-aytar/rampart && cd rampart
go build -o rampart ./cli/cmd/rampart
```

## `rampart scan`

Parses a `package-lock.json` (lockfileVersion 3) and prints the SBOM
in the requested format.

```bash
rampart scan [--format text|json|sarif]
             [--component-ref ref]
             [--commit-sha sha]
             <lockfile>
```

### Examples

```bash
# Plain text — one package per line.
rampart scan ./package-lock.json

# Machine-readable SBOM JSON.
rampart scan --format json ./package-lock.json

# Full SBOM with stable identity (ULID + GeneratedAt) for ingestion
# into a rampart engine, plus SARIF for upload to GitHub
# code-scanning:
rampart scan --format sarif \
             --component-ref kind:Component/default/web-app \
             --commit-sha "$(git rev-parse HEAD)" \
             ./package-lock.json > rampart.sarif
```

### Output formats

| Format | Shape | When to use |
|---|---|---|
| `text` | Human-readable, one package per line | Terminal inspection |
| `json` | Indented JSON; same shape as the engine's SBOM wire type | Pipeline consumption, downstream tools |
| `sarif` | SARIF 2.1.0 | `github/codeql-action/upload-sarif`, GitLab SAST report |

`--component-ref` and `--commit-sha` together promote the output
from a `ParsedSBOM` (no identity) to a full `SBOM` (ULID +
`GeneratedAt` + component binding) suitable for posting to the
engine's `/v1/components/{ref}/sboms`.

## Other subcommands

`ingest`, `status`, and `serve` are stubs at v0.1.0. For ingestion
today, POST the JSON output of `rampart scan` directly to the
engine; for serving, run the engine binary
(`go run ./engine/cmd/engine`) or the
`ghcr.io/ugurcan-aytar/engine` container image.

## Architecture notes

For contributors: the CLI is intentionally decoupled from the
engine's internal Go domain types. `cmd/rampart/main.go` is ~25
lines (signal plumbing + `commands.Dispatch`); each subcommand lives
in `internal/commands/<name>.go`; output formatters implement a
single-method interface in `internal/output/`. The CLI's
`output.SBOM` and `output.PackageVersion` types are deliberate
copies of the engine's wire shape so the engine module can evolve
its internals without breaking CLI consumers.

## License

MIT — see [LICENSE](https://github.com/ugurcan-aytar/rampart/blob/main/LICENSE).

Source and issues:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
