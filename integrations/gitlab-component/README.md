# rampart GitLab CI component

A [GitLab Components](https://docs.gitlab.com/ee/ci/components/)
spec that runs `rampart scan` on every pipeline and uploads the SARIF
output as a GitLab SAST report.

## When to use this

The GitLab component is the equivalent of rampart's
[GitHub Action](../github-action/) for GitLab CI. Use it when you
want lockfile findings to surface in GitLab's
**Security & Compliance → Vulnerability report**, gated as a pipeline
job.

For other shapes, see the
[rampart README](https://github.com/ugurcan-aytar/rampart#quickstart--pick-your-path).

## Usage

In your `.gitlab-ci.yml`:

```yaml
include:
  - component: $CI_SERVER_FQDN/ugurcan-aytar/rampart/rampart-scan@v0.1.0
    inputs:
      lockfile: package-lock.json
      component_ref: kind:Component/default/web-app
```

## Inputs

| Input | Default | Notes |
|---|---|---|
| `lockfile` | `package-lock.json` | Path relative to the repo root. |
| `component_ref` | — | Backstage Component ref to attach to the scan. |
| `commit_sha` | `$CI_COMMIT_SHA` | SHA the lockfile was taken at. |
| `fail_on` | `high` | Severity threshold above which the job exits non-zero. |

## What you get

The job uploads the SARIF report as a `sast` artifact, which GitLab
ingests into the project's vulnerability dashboard. Findings are
deduplicated against existing vulnerabilities; new findings open
issues per the project's vulnerability management settings.

## License

MIT — see [LICENSE](https://github.com/ugurcan-aytar/rampart/blob/main/LICENSE).

Source and issues:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
