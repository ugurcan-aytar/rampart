# Roadmap

Written in full in Adım 9. Current skeleton:

## Phase 1 — this sprint (scaffolding + one working vertical slice)

- [x] Repo scaffolding, Yarn 4, Go workspace, Makefile, doc placeholders (Adım 1)
- [ ] Engine: domain types, storage interface + memory impl, contract tests, API scaffolding (Adım 2)
- [ ] npm `lockfileVersion: 3` parser — real Go implementation + fixtures (Adım 2)
- [ ] Publisher trust engine: interface + domain types + `AlwaysTrust` default (Adım 2)
- [ ] OpenAPI schema + Go / TS codegen pipeline (Adım 3)
- [ ] CLI + four integration adapters (Adım 4)
- [ ] Backstage frontend, backend, scaffolder plugins (Adım 5)
- [ ] Rust native parser via UDS + parity tests + benchmarks (Adım 6)
- [ ] Docker Compose demo stack + Playwright e2e scenarios (Adım 7)
- [ ] CI workflows: engine, native, parity, backstage, integrations, e2e, codeql (Adım 8)
- [ ] Full README + ARCHITECTURE + DEPS + SECURITY + CONTRIBUTING + ADRs (Adım 9)

## Phase 2 — publishing

- [ ] npm trusted publishing for the three Backstage plugins
- [ ] Homebrew tap for CLI (`ugurcan-aytar/tap`)
- [ ] GitHub Marketplace listing for the Action
- [ ] First container images on `ghcr.io` (engine + native + slack-notifier), cosign-signed
- [ ] SBOM attestations per release

## Phase 3 — real capabilities

- [ ] SBOM parsers for pypi, cargo, go (Go + Rust, byte-identical parity)
- [ ] SBOM parsers for npm v1/v2, yarn.lock, pnpm-lock.yaml
- [ ] Roaring bitmap blast-radius index (<500 ms on 100k components)
- [ ] Publisher trust engine: statistical baseline + seven detectors
- [ ] IoC feed integrations beyond OSV (GHSA, Socket, custom)
- [ ] SQLite + Postgres storage backends (memory ships in Phase 1, these follow)
