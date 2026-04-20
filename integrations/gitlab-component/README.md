# rampart GitLab CI component

Minimal GitLab Components spec that runs `rampart scan` on every pipeline
and uploads the SARIF as a SAST report.

## Usage

In your `.gitlab-ci.yml`:

```yaml
include:
  - component: $CI_SERVER_FQDN/ugurcan-aytar/rampart/rampart-scan@~latest
    inputs:
      lockfile: package-lock.json
      component_ref: kind:Component/default/web-app
```

## Status

Phase 1 scaffold — same constraints as the GitHub Action. End-to-end once
the `rampart` CLI module ships a tagged release and IoC matching lands.
