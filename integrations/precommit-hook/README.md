# rampart pre-commit hook

A [pre-commit](https://pre-commit.com) hook that validates
`package-lock.json` shape — JSON well-formed + `lockfileVersion: 3` —
before a commit is accepted. Catches the "I committed a yarn-v1
lockfile by accident" class of mistake without requiring the rampart
engine to be running.

## When to use this

Use the pre-commit hook for fast local validation of lockfile
shape, with no engine dependency and no network calls. For full
IoC matching at commit time, run `rampart scan` in a separate hook
or wire the [GitHub Action](../github-action/) to your CI.

## Usage

In your repository's `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/ugurcan-aytar/rampart
    rev: v0.1.0
    hooks:
      - id: rampart-lockfile
```

Then:

```bash
pre-commit install
pre-commit run --all-files    # or just let it fire on commit
```

## What it checks

- The file at the configured path exists and is a valid JSON document.
- The top-level `lockfileVersion` field is `3` (npm 7+ format).
- The `packages` map is present and non-empty.

If any check fails, the commit is blocked with a message pointing at
the offending lockfile.

## What it does not check

- IoC matching against published advisories — that's the
  [`rampart` CLI](../../cli/) or the engine.
- Transitive dependency resolution — the hook reads the lockfile
  as data, it doesn't resolve.
- Other lockfile formats (`yarn.lock`, `pnpm-lock.yaml`) — npm
  v3 only at v0.1.0; other ecosystems planned.

## Build locally

```bash
cd integrations/precommit-hook
go build -o rampart-precommit ./cmd/rampart-precommit
./rampart-precommit ../../engine/testdata/lockfiles/axios-compromise.json
```

## License

MIT — see [LICENSE](https://github.com/ugurcan-aytar/rampart/blob/main/LICENSE).

Source and issues:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
