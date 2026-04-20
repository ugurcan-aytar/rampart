# rampart pre-commit hook

Tiny Go binary wired into the [pre-commit](https://pre-commit.com) framework.
Validates `package-lock.json` JSON structure + `lockfileVersion: 3` before
a commit is accepted. Catches the "I committed a yarn v1 lockfile by
accident" class of mistake without requiring the engine to be running.

## Usage

In your repo's `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/ugurcan-aytar/rampart
    rev: vX.Y.Z   # pin to a tagged release once one ships
    hooks:
      - id: rampart-lockfile
```

Then:

```bash
pre-commit install
pre-commit run --all-files    # or just let it fire on commit
```

## Build locally

```bash
cd integrations/precommit-hook
go build -o rampart-precommit ./cmd/rampart-precommit
./rampart-precommit ../../engine/testdata/lockfiles/axios-compromise.json
./rampart-precommit ../../engine/testdata/lockfiles/wrong-version.json && echo oops
```

## Scope

Phase 1 validates shape only — no IoC matching, no network calls, no
engine dependency. For full scanning at commit time, use the `rampart
scan` CLI in a separate hook (Phase 2).
