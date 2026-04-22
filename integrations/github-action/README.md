# rampart GitHub Action

Scans an npm `package-lock.json` on every push or pull request, emits
SARIF 2.1.0, and uploads the result to GitHub's code-scanning API.
Wraps the [rampart CLI](../../cli/) in a composite action.

## When to use this

Use the GitHub Action when you want lockfile findings to surface in
the **Security → Code scanning** tab of your repository, gated as
required PR checks. For other shapes:

- One-shot terminal scans → use the `rampart` CLI directly.
- Local pre-commit validation → use the
  [pre-commit hook](../precommit-hook/).
- Long-running daemon with Slack notifications → use the
  [self-hosted engine](https://github.com/ugurcan-aytar/rampart#path-2--mid-size-team-self-hosted)
  + Slack notifier.

## Usage

```yaml
name: Supply chain scan
on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read
  security-events: write   # required for upload-sarif

jobs:
  rampart-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: ugurcan-aytar/rampart/integrations/github-action@v0.1.0
        with:
          lockfile: package-lock.json
          component-ref: kind:Component/default/web-app
      - uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: ${{ steps.rampart-scan.outputs.sarif-path }}
```

## Inputs

| Name | Default | Notes |
|---|---|---|
| `lockfile` | `package-lock.json` | Path relative to the repo root. |
| `component-ref` | — | Backstage Component ref to attach to the scan (e.g. `kind:Component/default/web-app`). |
| `commit-sha` | `${{ github.sha }}` | SHA the lockfile was taken at; embedded in the SBOM. |
| `fail-on` | `high` | Severity threshold above which the action exits non-zero. |

## Outputs

| Name | Notes |
|---|---|
| `sarif-path` | Absolute path to the generated SARIF file. Pipe into `github/codeql-action/upload-sarif`. |

## What you get

The SARIF document contains:

- `runs[].tool.driver` populated with rampart version + scan
  properties.
- `runs[].results` populated with one entry per matched IoC, with
  `level`, `ruleId`, `locations`, and a `message` describing the
  affected `(package, version, transitive path)`.

If no IoCs match the lockfile's resolved tree, the `results` array
is empty and the action exits successfully.

## License

MIT — see [LICENSE](https://github.com/ugurcan-aytar/rampart/blob/main/LICENSE).

Source and issues:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
