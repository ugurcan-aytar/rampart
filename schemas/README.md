# Schemas

Single source of truth for rampart's wire contracts. Generated
artefacts (Go server stub + types, TypeScript types) are written
back into the consumer packages and committed to the repo; CI fails
if the schema changes without regenerating.

## Files

| File | Role | Generated artefacts |
|---|---|---|
| `openapi.yaml` | HTTP + SSE contract for the engine | Go server stub at `engine/api/gen/`, Go client types, TypeScript types at `backstage/plugins/rampart/src/api/gen/schema.ts` |
| `native-ipc.md` | Wire protocol between the engine and the Rust sidecar over a Unix Domain Socket | hand-rolled Rust + Go codecs (`engine/internal/native/`, `native/crates/rampart-native/`) |
| `cloudevents.yaml` | External event-bus envelope, reference only — not yet wired into a producer or consumer | none |

## Regenerate

```bash
make gen         # Go + TypeScript
make gen-go      # Go types only (oapi-codegen)
make gen-ts      # TypeScript types only (openapi-typescript)
```

The CI gate `gen-check.yml` runs `make gen` and asserts
`git diff --exit-code` — if `openapi.yaml` was edited but the
generated artefacts weren't committed, the build fails with a diff
summary.

## Policy

- **Every endpoint** has `operationId`, `summary`, `tags`, and at
  least one response with a `$ref`'d schema. No inline schemas in
  responses.
- **Every schema** has `required` + typed properties + a
  `description` or `example` for the non-obvious fields. Enums are
  locked in — changing a value is a breaking change and must be
  mirrored in `engine/internal/domain/*.go`.
- **No duplication.** Shared types (`Severity`, `IncidentState`,
  `Error`, `PackageVersion`, …) live once under
  `components/schemas` and are `$ref`'d everywhere they appear.
- **SSE (`/v1/stream`)** uses a discriminated `oneOf` on `type`.
  TypeScript narrows automatically; Go gets a generated union via
  oapi-codegen. The actual `text/event-stream` framer is hand-rolled
  in `engine/internal/api/sse.go` because oapi-codegen does not
  produce SSE handlers.
- **Breaking changes** bump `info.version` (semver-ish:
  `0.MINOR.PATCH` pre-1.0, `MAJOR.MINOR.PATCH` post-1.0) and add
  an ADR under `docs/decisions/`.

## License

MIT — see [LICENSE](../LICENSE).
