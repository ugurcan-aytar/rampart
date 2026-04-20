# rampart GitHub Action

Scans `package-lock.json` on every push / PR, uploads the SARIF to GitHub's
code-scanning API. Powered by the `rampart` CLI under the hood.

## Usage

```yaml
- uses: ugurcan-aytar/rampart-action@v1
  with:
    lockfile: package-lock.json
    component-ref: kind:Component/default/web-app
```

## Inputs

| Name           | Default              | Notes |
|----------------|----------------------|-------|
| `lockfile`     | `package-lock.json`  | Path relative to the repo root. |
| `component-ref`| —                    | Backstage Component ref to attach to the scan. |
| `commit-sha`   | `${{ github.sha }}`  | SHA the lockfile was taken at. |
| `fail-on`      | `high`               | Severity threshold. Phase 2 — currently scan always succeeds. |

## Outputs

| Name         | Notes |
|--------------|-------|
| `sarif-path` | Absolute path to the generated SARIF file. |

## Status

Phase 1 scaffold. Works end-to-end once:

- `ugurcan-aytar/rampart` publishes a tagged release so `go install
  .../cli/cmd/rampart@vX.Y.Z` resolves.
- IoC matching lands in the engine (the SARIF `results` array will then
  hold findings instead of being empty).

Until then the SARIF document is valid but carries zero findings — the
action succeeds, `upload-sarif` is happy, there's just nothing to flag yet.
