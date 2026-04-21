# mock-npm-registry

Demo-only HTTP server that stands in for `registry.npmjs.org` inside
the Adım 7.5 Docker Compose stack. Serves a small set of canned
lockfile fixtures and an IoC feed JSON.

**This is not a general-purpose npm registry double.** It only
answers the specific routes the rampart demo scenarios hit.

## Routes

- `GET /healthz` — liveness probe.
- `GET /-/lockfile/{component}` — returns a canned
  `package-lock.json` for the demo component. Known components:
  - `web-app` → `axios-compromise.json` (axios@1.11.0)
  - `billing` → `axios-compromise.json`
  - `reporting` → `simple-webapp.json` (clean, axios-free)
  - `shai-hulud` → `shai-hulud.json` (10 worm packages)
  - `vercel-oauth` → `vercel-oauth.json` (leaked OAuth token package)
- `GET /-/iocs` — returns the canned IoC list scenarios publish.
  The top-level keys are `axios_compromise`, `shai_hulud`,
  `vercel_oauth`; each is an array of
  [`IoC`](../../schemas/openapi.yaml) objects ready to POST to
  `/v1/iocs`.

## Run standalone

```bash
go run ./cmd/mock-npm-registry -addr :8081
curl http://localhost:8081/healthz
curl http://localhost:8081/-/lockfile/web-app
curl http://localhost:8081/-/iocs | jq .axios_compromise
```

## Run in the demo stack

Built and started by `docker compose up` at Adım 7.5. The scenario
scripts (`scripts/demo-scenarios/*.sh`) fetch the fixtures from this
service, submit them to the engine's `/v1/sboms` endpoint, then POST
the matching IoC list to `/v1/iocs`.
