# Contributing

This is a portfolio project maintained by
[@ugurcan-aytar](https://github.com/ugurcan-aytar). External PRs are
welcome after v0.1.0 ships; issue reports, benchmark contributions,
and new ecosystem parsers are welcome any time.

## Quick dev setup

```bash
git clone https://github.com/ugurcan-aytar/rampart
cd rampart
make bootstrap
```

`make bootstrap` installs JS deps with Yarn 4 (corepack-managed),
syncs the Go workspace, and fails fast if the supply-chain gates in
`.yarnrc.yml` have drifted. Required tooling on the host:

- Go 1.24+
- Node 20.18+ (corepack will fetch Yarn 4.6)
- Rust stable 1.75+ (only if you touch `native/`)
- Docker + Docker Compose (only for the demo stack)

## Running the stack

```bash
make demo-axios           # engine + mock-npm + slack + backstage, seeded
make demo-shai-hulud      # same stack, different scenario
make demo-native          # axios scenario through the Rust sidecar
make demo-down            # clean teardown + volume prune
```

For the three Phase 1 quickstart paths (CLI, self-hosted, Backstage),
see the [README Quickstart](./README.md#quickstart--pick-your-path).

## OpenAPI contract

`schemas/openapi.yaml` is the single source of truth. Go types live at
`engine/api/gen/api.gen.go` (oapi-codegen); TS types at
`backstage/plugins/rampart/src/api/gen/schema.ts` (openapi-typescript).

Regenerate both with `make gen`. Individual targets: `make gen-go`,
`make gen-ts`.

CI gate: `make gen-check` runs both generators then asserts
`git diff --exit-code`. If you change `schemas/openapi.yaml` and
forget to regenerate, this fails. See
[schemas/README.md](./schemas/README.md) for the full policy.

## Testing expectations

| Layer | What | How |
|---|---|---|
| Engine | Go unit + integration | `cd engine && go test ./...` |
| Engine | Coverage gates | per-package (domain 80%+, api 70%+, trust 85%+, matcher 80%+) |
| Native | Rust unit | `cd native && cargo test --workspace` |
| Parity | Go parser == Rust parser | `go test -run Parity ./engine/sbom/npm/...` |
| CLI | Go unit | `cd cli && go test ./...` |
| Backstage | TS typecheck + lint + test | `yarn workspaces foreach --all run test` |
| E2E | Playwright vs live demo stack | `cd e2e && yarn exec playwright test` |

Every PR runs the matching CI workflow (`engine.yml`, `native.yml`,
`parity.yml`, `backstage.yml`, `integrations.yml`, `e2e.yml`). The
[`gen-check.yml`](.github/workflows/gen-check.yml) guard fires on
any edit to `schemas/openapi.yaml`.

## Adding a new IoC source

1. Write a tiny Go client in `integrations/` that polls or subscribes
   to the source (OSV.dev API, GHSA webhook, custom feed).
2. Translate each entry into a `domain.IoC` + `POST /v1/iocs`.
3. The engine's forward-match pipeline will open incidents against
   any already-ingested SBOMs that match; nothing else to wire up.
4. If the source publishes the same IoC multiple times (dedupe by ID
   upstream or in your client), the engine's idempotency key
   `(IoCID, ComponentRef)` handles replay cleanly.

Reference: `scripts/demo-scenarios/*.sh` show the simplest
possible "client" (curl against a canned JSON file).

## Adding a new parser (new ecosystem)

The parser package layout keeps each ecosystem isolated:

```
engine/sbom/
├── npm/          parser.go, parser_test.go, types.go
├── pypi/         (Phase 3 — your work)
├── cargo/        (Phase 3)
└── go/           (Phase 3)
```

Steps:

1. `engine/sbom/<ecosystem>/parser.go` — implement `Parse(ctx, content) (*domain.ParsedSBOM, error)`
   following the shape of `engine/sbom/npm/parser.go`. Output must
   carry `Ecosystem`, `Packages`, `SourceFormat`, `SourceBytes`.
2. Test fixtures under `engine/testdata/lockfiles/`.
3. Wire the strategy dispatcher at `engine/sbom/<ecosystem>/strategy.go`
   if you also ship a Rust implementation (mirrors the npm pattern).
4. Extend the OpenAPI `SBOMSubmission.sourceFormat` enum; regenerate.
5. If you add a Rust parser, run the parity test template from
   `engine/sbom/npm/parity_test.go`.

## Supply-chain policy

These are non-negotiable. PRs that relax them need an ADR first.

- **`.yarnrc.yml` → `enableScripts: false`.** No postinstall /
  preinstall / prepare. Enforced by `backstage.yml` pre-check.
- **Yarn lockfile = exact pins.** `defaultSemverRangePrefix: ""`;
  no caret, no tilde.
- **`cargo-deny` license allowlist.** New crates with non-allowlisted
  licenses need an ADR. Config at `native/deny.toml`.
- **Every dep justified in `DEPS.md`.** Entry template in the file.
- **Minimum release age (72 h).** New package versions wait 72 h
  before Renovate can pin them (ADR-0006).
- **No Claude / AI attribution.** Anywhere. `CLAUDE.md`, `.claude/`,
  `.aider*`, `.cursor/`, and friends are `.gitignore`'d at the top
  of the file and stay local.

## Commit conventions

Conventional Commits — `type(scope): message`.

- `feat(engine): …`
- `fix(backstage): …`
- `perf(native): …`
- `refactor(api): …`
- `chore: …` (no scope for root-level changes)
- `docs: …`
- `ci: …`
- `test: …`

Commit subjects under 70 characters. Body wraps at 72. Write the
**why** in the body — the diff already shows the *what*.

## Branch + PR model

Direct commits to `main` during bootstrap (Adım 1–9). Post-v0.1.0:

- topic branches (`feat/…`, `fix/…`, `docs/…`)
- PRs require `CODEOWNERS` review (owners defined in `./CODEOWNERS`)
- CI must be green (all non-advisory workflows)
- Squash merge; the squashed commit message should read as a
  standalone changelog entry

## Security disclosures

See [SECURITY.md](./SECURITY.md). Short version: email
`aytarugurcan@gmail.com` with a clear reproduction before opening
an issue if the bug is security-relevant.
