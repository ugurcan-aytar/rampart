# Schemas

Single source of truth for rampart's wire contracts. Two files:

| File | Role | Generated from this → |
|---|---|---|
| `openapi.yaml` | HTTP + SSE contract (live in Phase 1) | Go types at `engine/api/gen/`, TS types at `backstage/plugins/rampart/src/api/gen/schema.ts` |
| `cloudevents.yaml` | External event-bus envelope (Phase 2) | Not yet wired — reference only |

## Regenerate

```bash
make gen         # Go + TS
make gen-go      # Go types only (oapi-codegen)
make gen-ts      # TS types only (openapi-typescript)
```

CI gate: `make gen && git diff --exit-code` — if schema changed but the
generated artefacts weren't committed, the build fails.

## Policy

- **Every endpoint** has `operationId`, `summary`, `tags`, at least one
  response with a `$ref`'d schema. No inline schemas.
- **Every schema** has `required` + typed properties + `description` or
  `example` for the non-obvious ones. Enums are locked in — changing a
  value is a breaking change (mirror in `engine/internal/domain/*.go`).
- **No duplication.** Shared types (Severity, IncidentState, Error,
  PackageVersion, …) live once under `components/schemas` and are
  `$ref`'d everywhere.
- **SSE (`/v1/stream`)**: discriminated oneOf on `type`. TS narrows
  automatically; Go gets a union via oapi-codegen. The actual SSE framer
  is a hand-written adapter (see `engine/internal/api/sse.go`, Adım 3
  Part 2) because oapi-codegen does not produce `text/event-stream`
  handlers.
- **Breaking changes** bump `info.version` (semver-ish: `0.MINOR.PATCH`
  pre-1.0, `MAJOR.MINOR.PATCH` post-1.0) and add an ADR under
  `docs/decisions/`.
