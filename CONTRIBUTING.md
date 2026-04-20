# Contributing

This is a portfolio project maintained by [@ugurcan-aytar](https://github.com/ugurcan-aytar). External PRs are welcome after v0.1.0 ships.

Full contributing guide (PR template, testing expectations, review SLAs) lands in Adım 9.

## Quick dev setup

```bash
git clone https://github.com/ugurcan-aytar/rampart
cd rampart
make bootstrap
```

`make bootstrap` installs JS deps with Yarn 4 (corepack-managed), syncs the Go workspace, and fails fast if the supply-chain gates in `.yarnrc.yml` have drifted.

## Commit conventions

Conventional Commits — `type(scope): message`.

- `feat(engine): ...`
- `fix(backstage): ...`
- `chore: ...` (no scope for root-level changes)
- `docs: ...`
- `ci: ...`
- `refactor: ...`
- `test: ...`

## Branch model

Direct commits to `main` during bootstrap (Adım 1–9). Post-v0.1.0, a topic-branch + PR flow with `CODEOWNERS` review.

## AI attribution

This repository has a strict policy of **no AI attribution** in commits, PR descriptions, or review comments. Tooling artifacts (`CLAUDE.md`, `.claude/`, `.aider*`, `.cursor/`, …) are listed at the top of `.gitignore` and stay local.
